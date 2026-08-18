package playerclaim

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	KnowledgeChallengeTTL     = 10 * time.Minute
	DefaultSessionIdleTTL     = 24 * time.Hour
	DefaultSessionAbsoluteTTL = 7 * 24 * time.Hour
	DefaultMaxChallenges      = 1024
	DefaultMaxSessions        = 4096
	bearerBytes               = 32
)

var (
	ErrInvalidConfiguration  = errors.New("invalid player claim service configuration")
	ErrInvalidTarget         = errors.New("invalid player claim target")
	ErrInvalidProof          = errors.New("invalid prepared player claim proof")
	ErrChallengeNotFound     = errors.New("player claim challenge not found")
	ErrChallengeExpired      = errors.New("player claim challenge expired")
	ErrVerificationInFlight  = errors.New("player claim verification is already in progress")
	ErrStoreFull             = errors.New("player claim store is full")
	ErrSessionNotFound       = errors.New("player claim session not found")
	ErrSessionExpired        = errors.New("player claim session expired")
	ErrRandomnessUnavailable = errors.New("secure randomness is unavailable")
	ErrBearerCollision       = errors.New("player claim bearer collision")
)

type VerificationStatus string

const (
	VerificationReady    VerificationStatus = "ready"
	VerificationVerified VerificationStatus = "verified"
)

type Challenge struct {
	Bearer       string             `json:"challengeToken"`
	Status       VerificationStatus `json:"status"`
	Instructions *Instructions      `json:"instructions,omitempty"`
	ExpiresAt    time.Time          `json:"expiresAt"`
}

type IssuedSession struct {
	Bearer            string    `json:"-"`
	IdleExpiresAt     time.Time `json:"idleExpiresAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
}

type Verification struct {
	Status       VerificationStatus `json:"status"`
	Instructions *Instructions      `json:"instructions,omitempty"`
	ExpiresAt    time.Time          `json:"expiresAt,omitzero"`
	Session      *IssuedSession     `json:"-"`
}

type Principal struct {
	subject           string
	publicPlayerID    string
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

func (p Principal) Subject() string              { return p.subject }
func (p Principal) PublicPlayerID() string       { return p.publicPlayerID }
func (p Principal) IdleExpiresAt() time.Time     { return p.idleExpiresAt }
func (p Principal) AbsoluteExpiresAt() time.Time { return p.absoluteExpiresAt }

type Options struct {
	SessionIdleTTL     time.Duration
	SessionAbsoluteTTL time.Duration
	MaxChallenges      int
	MaxSessions        int
	Now                func() time.Time
	Random             io.Reader
}

type bearerKey [sha256.Size]byte

type challengeEntry struct {
	prepared       Prepared
	subject        string
	publicPlayerID string
	expiresAt      time.Time
	verifying      bool
}

type sessionEntry struct {
	subject           string
	publicPlayerID    string
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

type Service struct {
	prover             Prover
	progress           ProgressSource
	now                func() time.Time
	random             io.Reader
	sessionIdleTTL     time.Duration
	sessionAbsoluteTTL time.Duration
	maxChallenges      int
	maxSessions        int
	randomMu           sync.Mutex
	mu                 sync.Mutex
	challenges         map[bearerKey]*challengeEntry
	pendingStarts      int
	sessions           map[bearerKey]*sessionEntry
}

func NewService(prover Prover, options Options) (*Service, error) {
	if prover == nil || options.SessionIdleTTL < 0 || options.SessionAbsoluteTTL < 0 || options.MaxChallenges < 0 || options.MaxSessions < 0 {
		return nil, ErrInvalidConfiguration
	}
	idleTTL := options.SessionIdleTTL
	if idleTTL == 0 {
		idleTTL = DefaultSessionIdleTTL
	}
	absoluteTTL := options.SessionAbsoluteTTL
	if absoluteTTL == 0 {
		absoluteTTL = DefaultSessionAbsoluteTTL
	}
	if idleTTL > absoluteTTL {
		return nil, fmt.Errorf("%w: idle TTL exceeds absolute TTL", ErrInvalidConfiguration)
	}
	maxChallenges := options.MaxChallenges
	if maxChallenges == 0 {
		maxChallenges = DefaultMaxChallenges
	}
	maxSessions := options.MaxSessions
	if maxSessions == 0 {
		maxSessions = DefaultMaxSessions
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	random := options.Random
	if random == nil {
		random = rand.Reader
	}
	progress, _ := prover.(ProgressSource)
	return &Service{prover: prover, progress: progress, now: now, random: random, sessionIdleTTL: idleTTL,
		sessionAbsoluteTTL: absoluteTTL, maxChallenges: maxChallenges, maxSessions: maxSessions,
		challenges: make(map[bearerKey]*challengeEntry), sessions: make(map[bearerKey]*sessionEntry)}, nil
}

func (s *Service) Start(ctx context.Context, publicPlayerID string) (Challenge, error) {
	if strings.TrimSpace(publicPlayerID) == "" || strings.TrimSpace(publicPlayerID) != publicPlayerID {
		return Challenge{}, ErrInvalidTarget
	}
	if !s.reserveChallengeSlot() {
		return Challenge{}, ErrStoreFull
	}
	reserved := true
	defer func() {
		if reserved {
			s.releaseChallengeSlot()
		}
	}()
	selector, err := s.randomUint64()
	if err != nil {
		return Challenge{}, err
	}
	bearer, key, err := s.randomBearer()
	if err != nil {
		return Challenge{}, err
	}
	prepared, err := s.prover.Prepare(ctx, publicPlayerID, selector)
	if err != nil {
		return Challenge{}, fmt.Errorf("prepare player claim: %w", err)
	}
	if err := validatePrepared(prepared, publicPlayerID); err != nil {
		return Challenge{}, err
	}
	expiresAt := s.now().Add(KnowledgeChallengeTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingStarts--
	reserved = false
	if _, exists := s.challenges[key]; exists {
		return Challenge{}, ErrBearerCollision
	}
	if _, exists := s.sessions[key]; exists {
		return Challenge{}, ErrBearerCollision
	}
	s.challenges[key] = &challengeEntry{prepared: clonePrepared(prepared), subject: prepared.Subject,
		publicPlayerID: prepared.PublicPlayerID, expiresAt: expiresAt}
	instructions := cloneInstructions(prepared.Instructions)
	return Challenge{Bearer: bearer, Status: VerificationReady, Instructions: &instructions, ExpiresAt: expiresAt}, nil
}

func (s *Service) CycleQuestion(ctx context.Context, challengeBearer, questionID string) (Verification, error) {
	cycler, ok := s.prover.(QuestionCycler)
	if !ok || strings.TrimSpace(questionID) == "" {
		return Verification{}, ErrUnavailable
	}
	key, now := hashBearer(challengeBearer), s.now()
	s.mu.Lock()
	entry, ok := s.challenges[key]
	if !ok {
		s.mu.Unlock()
		return Verification{}, ErrChallengeNotFound
	}
	if expired(now, entry.expiresAt) {
		delete(s.challenges, key)
		s.mu.Unlock()
		return Verification{}, ErrChallengeExpired
	}
	if entry.verifying {
		s.mu.Unlock()
		return Verification{}, ErrVerificationInFlight
	}
	entry.verifying = true
	prepared := clonePrepared(entry.prepared)
	s.mu.Unlock()
	err := cycler.CycleQuestion(ctx, &prepared, questionID)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok = s.challenges[key]
	if !ok || expired(s.now(), entry.expiresAt) {
		delete(s.challenges, key)
		return Verification{}, ErrChallengeExpired
	}
	if err != nil {
		entry.verifying = false
		return Verification{}, err
	}
	if !matchesPreparedBinding(prepared, entry) || !validQuizReplacement(entry.prepared.Instructions, prepared.Instructions, questionID) {
		entry.verifying = false
		return Verification{}, ErrInvalidProof
	}
	entry.prepared, entry.verifying = clonePrepared(prepared), false
	instructions := cloneInstructions(prepared.Instructions)
	return Verification{Status: VerificationReady, Instructions: &instructions, ExpiresAt: entry.expiresAt}, nil
}

func (s *Service) Verify(ctx context.Context, challengeBearer string, answers ...QuizAnswer) (Verification, error) {
	key, now := hashBearer(challengeBearer), s.now()
	s.mu.Lock()
	entry, ok := s.challenges[key]
	if !ok {
		s.mu.Unlock()
		return Verification{}, ErrChallengeNotFound
	}
	if expired(now, entry.expiresAt) {
		delete(s.challenges, key)
		s.mu.Unlock()
		return Verification{}, ErrChallengeExpired
	}
	if entry.verifying {
		s.mu.Unlock()
		return Verification{}, ErrVerificationInFlight
	}
	entry.verifying = true
	prepared := clonePrepared(entry.prepared)
	prepared.Answers = append([]QuizAnswer(nil), answers...)
	s.mu.Unlock()

	verifyErr := s.prover.Verify(ctx, &prepared)
	var sessionBearer string
	var sessionKey bearerKey
	if verifyErr == nil {
		sessionBearer, sessionKey, verifyErr = s.randomBearer()
	}

	now = s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok = s.challenges[key]
	if !ok || expired(now, entry.expiresAt) {
		delete(s.challenges, key)
		return Verification{}, ErrChallengeExpired
	}
	if !matchesPreparedBinding(prepared, entry) || !sameInstructions(prepared.Instructions, entry.prepared.Instructions) {
		entry.verifying = false
		return Verification{}, ErrInvalidProof
	}
	if errors.Is(verifyErr, ErrIncorrectAnswer) {
		delete(s.challenges, key)
		return Verification{}, ErrIncorrectAnswer
	}
	if verifyErr != nil {
		entry.verifying = false
		return Verification{}, fmt.Errorf("verify player claim: %w", verifyErr)
	}
	if len(s.sessions) >= s.maxSessions {
		s.cleanupSessionsLocked(now)
	}
	if len(s.sessions) >= s.maxSessions {
		entry.verifying = false
		return Verification{}, ErrStoreFull
	}
	if _, exists := s.challenges[sessionKey]; exists {
		entry.verifying = false
		return Verification{}, ErrBearerCollision
	}
	if _, exists := s.sessions[sessionKey]; exists {
		entry.verifying = false
		return Verification{}, ErrBearerCollision
	}
	absolute := now.Add(s.sessionAbsoluteTTL)
	idle := cappedExpiry(now.Add(s.sessionIdleTTL), absolute)
	delete(s.challenges, key)
	s.sessions[sessionKey] = &sessionEntry{subject: entry.subject, publicPlayerID: entry.publicPlayerID,
		idleExpiresAt: idle, absoluteExpiresAt: absolute}
	return Verification{Status: VerificationVerified, Session: &IssuedSession{Bearer: sessionBearer,
		IdleExpiresAt: idle, AbsoluteExpiresAt: absolute}}, nil
}

func (s *Service) ValidateSession(sessionBearer string) (Principal, error) {
	key, now := hashBearer(sessionBearer), s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[key]
	if !ok {
		return Principal{}, ErrSessionNotFound
	}
	if expired(now, entry.idleExpiresAt) || expired(now, entry.absoluteExpiresAt) {
		delete(s.sessions, key)
		return Principal{}, ErrSessionExpired
	}
	entry.idleExpiresAt = cappedExpiry(now.Add(s.sessionIdleTTL), entry.absoluteExpiresAt)
	return Principal{subject: entry.subject, publicPlayerID: entry.publicPlayerID,
		idleExpiresAt: entry.idleExpiresAt, absoluteExpiresAt: entry.absoluteExpiresAt}, nil
}

func (s *Service) RevokeSession(sessionBearer string) {
	s.mu.Lock()
	delete(s.sessions, hashBearer(sessionBearer))
	s.mu.Unlock()
}

func (s *Service) Progress(ctx context.Context, principal Principal) (PrivateProgress, error) {
	if s.progress == nil || principal.subject == "" {
		return PrivateProgress{}, ErrUnavailable
	}
	return s.progress.Progress(ctx, principal.subject)
}

func (s *Service) Cleanup() { now := s.now(); s.mu.Lock(); s.cleanupLocked(now); s.mu.Unlock() }

func (s *Service) reserveChallengeSlot() bool {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.challenges)+s.pendingStarts >= s.maxChallenges {
		s.cleanupLocked(now)
	}
	if len(s.challenges)+s.pendingStarts >= s.maxChallenges {
		return false
	}
	s.pendingStarts++
	return true
}

func (s *Service) releaseChallengeSlot() {
	s.mu.Lock()
	if s.pendingStarts > 0 {
		s.pendingStarts--
	}
	s.mu.Unlock()
}

func (s *Service) randomBearer() (string, bearerKey, error) {
	var raw [bearerBytes]byte
	s.randomMu.Lock()
	_, err := io.ReadFull(s.random, raw[:])
	s.randomMu.Unlock()
	if err != nil {
		return "", bearerKey{}, fmt.Errorf("%w: %v", ErrRandomnessUnavailable, err)
	}
	bearer := base64.RawURLEncoding.EncodeToString(raw[:])
	return bearer, hashBearer(bearer), nil
}

func (s *Service) randomUint64() (uint64, error) {
	var raw [8]byte
	s.randomMu.Lock()
	_, err := io.ReadFull(s.random, raw[:])
	s.randomMu.Unlock()
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrRandomnessUnavailable, err)
	}
	return binary.BigEndian.Uint64(raw[:]), nil
}

func (s *Service) cleanupLocked(now time.Time) {
	for key, entry := range s.challenges {
		if expired(now, entry.expiresAt) {
			delete(s.challenges, key)
		}
	}
	s.cleanupSessionsLocked(now)
}

func (s *Service) cleanupSessionsLocked(now time.Time) {
	for key, entry := range s.sessions {
		if expired(now, entry.idleExpiresAt) || expired(now, entry.absoluteExpiresAt) {
			delete(s.sessions, key)
		}
	}
}

func validatePrepared(prepared Prepared, requestedPublicPlayerID string) error {
	if strings.TrimSpace(prepared.Subject) == "" || prepared.PublicPlayerID != requestedPublicPlayerID || !validQuizInstructions(prepared.Instructions) {
		return ErrInvalidProof
	}
	return nil
}

func validQuizInstructions(instructions Instructions) bool {
	if instructions.Kind != InventoryQuiz || len(instructions.Questions) != 1 || instructions.SnapshotAt.IsZero() {
		return false
	}
	question := instructions.Questions[0]
	if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Prompt) == "" || len(question.Options) < 3 || len(question.Options) > 8 {
		return false
	}
	seen := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		if strings.TrimSpace(option) == "" || len(option) > 96 {
			return false
		}
		if _, exists := seen[option]; exists {
			return false
		}
		seen[option] = struct{}{}
	}
	return true
}

func validQuizReplacement(previous, next Instructions, questionID string) bool {
	return validQuizInstructions(previous) && validQuizInstructions(next) && previous.SnapshotAt.Equal(next.SnapshotAt) &&
		previous.Questions[0].ID == questionID && next.Questions[0].ID != questionID
}

func matchesPreparedBinding(prepared Prepared, entry *challengeEntry) bool {
	return prepared.Subject == entry.subject && prepared.PublicPlayerID == entry.publicPlayerID
}

func clonePrepared(prepared Prepared) Prepared {
	prepared.Instructions = cloneInstructions(prepared.Instructions)
	prepared.Answers = append([]QuizAnswer(nil), prepared.Answers...)
	return prepared
}

func cloneInstructions(instructions Instructions) Instructions {
	instructions.Questions = append([]QuizQuestion(nil), instructions.Questions...)
	for index := range instructions.Questions {
		instructions.Questions[index].Options = append([]string(nil), instructions.Questions[index].Options...)
	}
	return instructions
}

func sameInstructions(left, right Instructions) bool {
	if left.Kind != right.Kind || !left.SnapshotAt.Equal(right.SnapshotAt) || len(left.Questions) != len(right.Questions) {
		return false
	}
	for index := range left.Questions {
		lq, rq := left.Questions[index], right.Questions[index]
		if lq.ID != rq.ID || lq.Prompt != rq.Prompt || lq.CanCycle != rq.CanCycle || len(lq.Options) != len(rq.Options) {
			return false
		}
		for option := range lq.Options {
			if lq.Options[option] != rq.Options[option] {
				return false
			}
		}
	}
	return true
}

func hashBearer(bearer string) bearerKey   { return sha256.Sum256([]byte(bearer)) }
func expired(now, deadline time.Time) bool { return !now.Before(deadline) }
func cappedExpiry(candidate, absolute time.Time) time.Time {
	if candidate.After(absolute) {
		return absolute
	}
	return candidate
}
