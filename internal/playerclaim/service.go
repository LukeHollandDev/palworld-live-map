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
	// ChallengePhaseTTL applies independently to arming, proof, and restore.
	// Native backups can be infrequent and the safe reader intentionally lags
	// one generation, so each phase needs room for two backup intervals.
	ChallengePhaseTTL     = 90 * time.Minute
	KnowledgeChallengeTTL = 10 * time.Minute

	DefaultSessionIdleTTL     = 24 * time.Hour
	DefaultSessionAbsoluteTTL = 7 * 24 * time.Hour
	DefaultMaxChallenges      = 1024
	DefaultMaxSessions        = 4096

	bearerBytes = 32
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

// VerificationStatus is safe to expose to a claimant. Prover errors other
// than ErrPending are intentionally not represented as statuses; the HTTP
// boundary can map those typed errors to a generic response.
type VerificationStatus string

const (
	VerificationArming   VerificationStatus = "arming"
	VerificationReady    VerificationStatus = "ready"
	VerificationPending  VerificationStatus = "pending"
	VerificationVerified VerificationStatus = "verified"
)

// Challenge contains only the bearer and instructions needed by a claimant.
// The stable subject and baseline Evidence remain solely in Service's private
// challenge store.
type Challenge struct {
	Bearer       string             `json:"challengeToken"`
	Status       VerificationStatus `json:"status"`
	Instructions *Instructions      `json:"instructions,omitempty"`
	ExpiresAt    time.Time          `json:"expiresAt"`
}

// IssuedSession is returned once, after a challenge is successfully consumed.
// Its bearer is intended for an HttpOnly cookie. It deliberately contains no
// save-derived identity or proof data.
type IssuedSession struct {
	Bearer            string    `json:"-"`
	IdleExpiresAt     time.Time `json:"idleExpiresAt"`
	AbsoluteExpiresAt time.Time `json:"absoluteExpiresAt"`
}

// Verification describes a completed verification attempt. Session is nil
// while pending and non-nil only when Status is VerificationVerified.
type Verification struct {
	Status       VerificationStatus `json:"status"`
	Instructions *Instructions      `json:"instructions,omitempty"`
	ExpiresAt    time.Time          `json:"expiresAt,omitzero"`
	Session      *IssuedSession     `json:"-"`
}

// Principal is the authenticated, private result of validating a session.
// All fields are unexported so it cannot accidentally disclose the subject if
// marshalled as an HTTP response.
type Principal struct {
	subject           string
	publicPlayerID    string
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

// Subject returns the stable private subject for server-side progress lookup.
// Callers must not include it in responses or logs.
func (p Principal) Subject() string { return p.subject }

func (p Principal) PublicPlayerID() string { return p.publicPlayerID }

func (p Principal) IdleExpiresAt() time.Time { return p.idleExpiresAt }

func (p Principal) AbsoluteExpiresAt() time.Time { return p.absoluteExpiresAt }

// Options controls session lifetimes and memory bounds. Zero values select
// conservative defaults. Now and Random exist primarily for deterministic
// tests; production callers should leave them nil.
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
	phase          challengeProofPhase
	expiresAt      time.Time
	verifying      bool
}

type challengeProofPhase uint8

const (
	challengeArming challengeProofPhase = iota
	challengeProving
	challengeRestoring
)

type sessionEntry struct {
	subject           string
	publicPlayerID    string
	idleExpiresAt     time.Time
	absoluteExpiresAt time.Time
}

// Service owns the ephemeral claim and login lifecycle. It stores only hashes
// of bearer values; plaintext bearers are returned once to the caller.
type Service struct {
	prover   Prover
	progress ProgressSource
	now      func() time.Time
	random   io.Reader

	sessionIdleTTL     time.Duration
	sessionAbsoluteTTL time.Duration
	maxChallenges      int
	maxSessions        int

	randomMu sync.Mutex
	mu       sync.Mutex

	challenges    map[bearerKey]*challengeEntry
	pendingStarts int
	sessions      map[bearerKey]*sessionEntry
}

func NewService(prover Prover, options Options) (*Service, error) {
	if prover == nil {
		return nil, fmt.Errorf("%w: prover is required", ErrInvalidConfiguration)
	}
	if options.SessionIdleTTL < 0 || options.SessionAbsoluteTTL < 0 || options.MaxChallenges < 0 || options.MaxSessions < 0 {
		return nil, fmt.Errorf("%w: options cannot be negative", ErrInvalidConfiguration)
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
		return nil, fmt.Errorf("%w: session idle TTL cannot exceed absolute TTL", ErrInvalidConfiguration)
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
	return &Service{
		prover:             prover,
		progress:           progress,
		now:                now,
		random:             random,
		sessionIdleTTL:     idleTTL,
		sessionAbsoluteTTL: absoluteTTL,
		maxChallenges:      maxChallenges,
		maxSessions:        maxSessions,
		challenges:         make(map[bearerKey]*challengeEntry),
		sessions:           make(map[bearerKey]*sessionEntry),
	}, nil
}

// Start asks the Prover to prepare a challenge for a public player identifier.
// Capacity is reserved before the potentially expensive proof preparation so
// rejected starts cannot continue walking save generations after the store is
// already full.
func (s *Service) Start(ctx context.Context, publicPlayerID string) (Challenge, error) {
	if strings.TrimSpace(publicPlayerID) == "" || strings.TrimSpace(publicPlayerID) != publicPlayerID {
		return Challenge{}, ErrInvalidTarget
	}
	if !s.reserveChallengeSlot() {
		return Challenge{}, ErrStoreFull
	}
	reservationHeld := true
	defer func() {
		if reservationHeld {
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
		return Challenge{}, fmt.Errorf("prepare player claim proof: %w", err)
	}
	if err := validatePrepared(prepared, publicPlayerID); err != nil {
		return Challenge{}, err
	}

	now := s.now()
	phase := challengeArming
	status := VerificationArming
	var instructions *Instructions
	expiresAt := now.Add(ChallengePhaseTTL)
	if prepared.Instructions.Kind == InventoryQuiz {
		phase = challengeProving
		status = VerificationReady
		expiresAt = now.Add(KnowledgeChallengeTTL)
		value := cloneInstructions(prepared.Instructions)
		instructions = &value
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingStarts--
	reservationHeld = false
	if _, exists := s.challenges[key]; exists {
		return Challenge{}, ErrBearerCollision
	}
	if _, exists := s.sessions[key]; exists {
		return Challenge{}, ErrBearerCollision
	}

	s.challenges[key] = &challengeEntry{
		prepared: clonePrepared(prepared), subject: prepared.Subject, publicPlayerID: prepared.PublicPlayerID,
		phase: phase, expiresAt: expiresAt,
	}
	return Challenge{
		Bearer: bearer, Status: status, Instructions: instructions, ExpiresAt: expiresAt,
	}, nil
}

// Verify checks an active challenge. ErrPending becomes a Pending status so a
// client may poll without learning why the next safe save is not ready. A
// successful transition consumes the challenge and creates exactly one
// session in the same critical section.
func (s *Service) Verify(ctx context.Context, challengeBearer string, answers ...QuizAnswer) (Verification, error) {
	key := hashBearer(challengeBearer)
	now := s.now()

	s.mu.Lock()
	entry, exists := s.challenges[key]
	if !exists {
		s.mu.Unlock()
		return Verification{}, ErrChallengeNotFound
	}
	if expired(now, entry.expiresAt) {
		s.deleteChallengeLocked(key, entry)
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
		var randomErr error
		sessionBearer, sessionKey, randomErr = s.randomBearer()
		if randomErr != nil {
			verifyErr = randomErr
		}
	}

	now = s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists = s.challenges[key]
	if !exists {
		return Verification{}, ErrChallengeExpired
	}
	if expired(now, entry.expiresAt) {
		s.deleteChallengeLocked(key, entry)
		return Verification{}, ErrChallengeExpired
	}
	if !matchesPreparedBinding(prepared, entry) {
		entry.verifying = false
		return Verification{}, ErrInvalidProof
	}
	if errors.Is(verifyErr, ErrIncorrectAnswer) {
		s.deleteChallengeLocked(key, entry)
		return Verification{}, ErrIncorrectAnswer
	}

	if errors.Is(verifyErr, ErrPending) {
		if !validPendingProof(entry, prepared.Instructions) {
			entry.verifying = false
			return Verification{}, ErrInvalidProof
		}
		entry.prepared = clonePrepared(prepared)
		entry.verifying = false
		status := VerificationArming
		var instructions *Instructions
		var expiresAt time.Time
		if entry.phase != challengeArming {
			status = VerificationPending
			// A ready response can be lost after the prover has already advanced
			// its private phase. Replay the current bearer-protected instructions
			// on later pending checks so the claimant can recover both the proof
			// and restore sequences without persisting them in the browser.
			value := cloneInstructions(prepared.Instructions)
			instructions = &value
			expiresAt = entry.expiresAt
		}
		return Verification{Status: status, Instructions: instructions, ExpiresAt: expiresAt}, nil
	}
	if errors.Is(verifyErr, ErrReady) {
		nextPhase, valid := readyTransition(entry, prepared.Instructions)
		if !valid {
			entry.verifying = false
			return Verification{}, ErrInvalidProof
		}
		entry.prepared = clonePrepared(prepared)
		entry.phase = nextPhase
		entry.expiresAt = now.Add(ChallengePhaseTTL)
		entry.verifying = false
		instructions := cloneInstructions(prepared.Instructions)
		return Verification{Status: VerificationReady, Instructions: &instructions, ExpiresAt: entry.expiresAt}, nil
	}
	if verifyErr != nil {
		entry.verifying = false
		return Verification{}, fmt.Errorf("verify player claim proof: %w", verifyErr)
	}
	quizProof := entry.phase == challengeProving && entry.prepared.Instructions.Kind == InventoryQuiz &&
		sameInstructions(prepared.Instructions, entry.prepared.Instructions)
	swapProof := entry.phase == challengeRestoring && entry.prepared.Instructions.Kind == InventorySwapSequence &&
		sameInstructions(prepared.Instructions, entry.prepared.Instructions)
	if !quizProof && !swapProof {
		entry.verifying = false
		return Verification{}, ErrInvalidProof
	}
	entry.prepared = clonePrepared(prepared)

	if len(s.sessions) >= s.maxSessions {
		s.cleanupSessionsLocked(now)
		if len(s.sessions) >= s.maxSessions {
			entry.verifying = false
			return Verification{}, ErrStoreFull
		}
	}
	if _, exists := s.challenges[sessionKey]; exists {
		entry.verifying = false
		return Verification{}, ErrBearerCollision
	}
	if _, exists := s.sessions[sessionKey]; exists {
		entry.verifying = false
		return Verification{}, ErrBearerCollision
	}

	absoluteExpiresAt := now.Add(s.sessionAbsoluteTTL)
	idleExpiresAt := cappedExpiry(now.Add(s.sessionIdleTTL), absoluteExpiresAt)
	s.deleteChallengeLocked(key, entry)
	s.sessions[sessionKey] = &sessionEntry{
		subject:           entry.subject,
		publicPlayerID:    entry.publicPlayerID,
		idleExpiresAt:     idleExpiresAt,
		absoluteExpiresAt: absoluteExpiresAt,
	}

	return Verification{
		Status: VerificationVerified,
		Session: &IssuedSession{
			Bearer:            sessionBearer,
			IdleExpiresAt:     idleExpiresAt,
			AbsoluteExpiresAt: absoluteExpiresAt,
		},
	}, nil
}

// ValidateSession authenticates a bearer, refreshes its idle deadline, and
// returns a non-serializable Principal for server-side authorization.
func (s *Service) ValidateSession(sessionBearer string) (Principal, error) {
	key := hashBearer(sessionBearer)
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.sessions[key]
	if !exists {
		return Principal{}, ErrSessionNotFound
	}
	if expired(now, entry.idleExpiresAt) || expired(now, entry.absoluteExpiresAt) {
		delete(s.sessions, key)
		return Principal{}, ErrSessionExpired
	}

	entry.idleExpiresAt = cappedExpiry(now.Add(s.sessionIdleTTL), entry.absoluteExpiresAt)
	return Principal{
		subject:           entry.subject,
		publicPlayerID:    entry.publicPlayerID,
		idleExpiresAt:     entry.idleExpiresAt,
		absoluteExpiresAt: entry.absoluteExpiresAt,
	}, nil
}

// RevokeSession implements logout. It is intentionally idempotent so callers
// do not gain a session-validity oracle from its result.
func (s *Service) RevokeSession(sessionBearer string) {
	key := hashBearer(sessionBearer)
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
}

// Progress returns exact private completion evidence for an authenticated
// principal. HTTP callers must project its raw keys to public catalogue IDs.
func (s *Service) Progress(ctx context.Context, principal Principal) (PrivateProgress, error) {
	if s.progress == nil || principal.subject == "" {
		return PrivateProgress{}, ErrUnavailable
	}
	return s.progress.Progress(ctx, principal.subject)
}

// Cleanup removes expired challenges and sessions. Capacity checks also clean
// their respective bounded stores without making arbitrary bearer misses scan
// every entry.
func (s *Service) Cleanup() {
	now := s.now()
	s.mu.Lock()
	s.cleanupLocked(now)
	s.mu.Unlock()
}

func (s *Service) reserveChallengeSlot() bool {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.challenges)+s.pendingStarts >= s.maxChallenges {
		s.cleanupLocked(now)
		if len(s.challenges)+s.pendingStarts >= s.maxChallenges {
			return false
		}
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
			s.deleteChallengeLocked(key, entry)
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

func (s *Service) deleteChallengeLocked(key bearerKey, entry *challengeEntry) {
	delete(s.challenges, key)
}

func validatePrepared(prepared Prepared, requestedPublicPlayerID string) error {
	if strings.TrimSpace(prepared.Subject) == "" {
		return fmt.Errorf("%w: subject is empty", ErrInvalidProof)
	}
	if prepared.PublicPlayerID != requestedPublicPlayerID {
		return fmt.Errorf("%w: public player binding changed", ErrInvalidProof)
	}
	if !emptyInstructions(prepared.Instructions) && !validQuizInstructions(prepared.Instructions) {
		return fmt.Errorf("%w: proof must begin unarmed", ErrInvalidProof)
	}
	return nil
}

func emptyInstructions(instructions Instructions) bool {
	return instructions.Kind == "" && instructions.Phase == "" && instructions.Step == 0 &&
		instructions.TotalSteps == 0 && len(instructions.Pairs) == 0 && len(instructions.Questions) == 0 && instructions.SnapshotAt.IsZero()
}

func validQuizInstructions(instructions Instructions) bool {
	if instructions.Kind != InventoryQuiz || instructions.Phase != "" || instructions.Step != 0 ||
		instructions.TotalSteps != 0 || len(instructions.Pairs) != 0 || len(instructions.Questions) != 2 ||
		instructions.SnapshotAt.IsZero() {
		return false
	}
	seen := make(map[string]struct{}, len(instructions.Questions))
	for _, question := range instructions.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Prompt) == "" || len(question.Options) != 8 {
			return false
		}
		if _, exists := seen[question.ID]; exists {
			return false
		}
		seen[question.ID] = struct{}{}
		optionSeen := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			if strings.TrimSpace(option) == "" || len(option) > 96 {
				return false
			}
			if _, exists := optionSeen[option]; exists {
				return false
			}
			optionSeen[option] = struct{}{}
		}
	}
	return true
}

func validInstructions(instructions Instructions) bool {
	if instructions.Kind != InventorySwapSequence ||
		(instructions.Phase != ProofPhaseProve && instructions.Phase != ProofPhaseRestore) ||
		instructions.TotalSteps != 2 || len(instructions.Pairs) != 7 || instructions.SnapshotAt.IsZero() {
		return false
	}
	if (instructions.Phase == ProofPhaseProve && instructions.Step != 1) ||
		(instructions.Phase == ProofPhaseRestore && instructions.Step != 2) {
		return false
	}
	anchor := instructions.Pairs[0].SlotA
	seen := map[int]struct{}{anchor: {}}
	for _, pair := range instructions.Pairs {
		if pair.SlotA != anchor || pair.SlotB <= 0 || pair.SlotA == pair.SlotB {
			return false
		}
		if _, exists := seen[pair.SlotB]; exists {
			return false
		}
		seen[pair.SlotB] = struct{}{}
	}
	return true
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
	instructions.Pairs = append([]SlotPair(nil), instructions.Pairs...)
	instructions.Questions = append([]QuizQuestion(nil), instructions.Questions...)
	for index := range instructions.Questions {
		instructions.Questions[index].Options = append([]string(nil), instructions.Questions[index].Options...)
	}
	return instructions
}

func validPendingProof(entry *challengeEntry, instructions Instructions) bool {
	if entry.phase == challengeArming {
		return emptyInstructions(instructions)
	}
	return sameInstructions(instructions, entry.prepared.Instructions)
}

func readyTransition(entry *challengeEntry, instructions Instructions) (challengeProofPhase, bool) {
	if !validInstructions(instructions) {
		return entry.phase, false
	}
	switch entry.phase {
	case challengeArming:
		return challengeProving, instructions.Phase == ProofPhaseProve
	case challengeProving:
		return challengeRestoring, instructions.Phase == ProofPhaseRestore &&
			reverseInstructions(entry.prepared.Instructions, instructions)
	default:
		return entry.phase, false
	}
}

func sameInstructions(left, right Instructions) bool {
	if left.Kind != right.Kind || left.Phase != right.Phase || left.Step != right.Step ||
		left.TotalSteps != right.TotalSteps || !left.SnapshotAt.Equal(right.SnapshotAt) ||
		len(left.Pairs) != len(right.Pairs) || len(left.Questions) != len(right.Questions) {
		return false
	}
	for index := range left.Pairs {
		if left.Pairs[index] != right.Pairs[index] {
			return false
		}
	}
	for index := range left.Questions {
		if left.Questions[index].ID != right.Questions[index].ID ||
			left.Questions[index].Prompt != right.Questions[index].Prompt ||
			len(left.Questions[index].Options) != len(right.Questions[index].Options) {
			return false
		}
		for option := range left.Questions[index].Options {
			if left.Questions[index].Options[option] != right.Questions[index].Options[option] {
				return false
			}
		}
	}
	return true
}

func reverseInstructions(prove, restore Instructions) bool {
	if prove.Kind != InventorySwapSequence || prove.Phase != ProofPhaseProve ||
		restore.Kind != InventorySwapSequence || restore.Phase != ProofPhaseRestore ||
		prove.TotalSteps != restore.TotalSteps || len(prove.Pairs) != len(restore.Pairs) {
		return false
	}
	for index, pair := range prove.Pairs {
		if restore.Pairs[len(restore.Pairs)-1-index] != pair {
			return false
		}
	}
	return true
}

func hashBearer(bearer string) bearerKey { return sha256.Sum256([]byte(bearer)) }

func expired(now, deadline time.Time) bool { return !now.Before(deadline) }

func cappedExpiry(candidate, absolute time.Time) time.Time {
	if candidate.After(absolute) {
		return absolute
	}
	return candidate
}
