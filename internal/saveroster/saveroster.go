// Package saveroster adapts immutable Palworld native backup generations to
// the public roster consumed by the live-map poller. It selects a completed
// backup generation on disk and hands its path to a snapshot reader (the
// external save-decoder sidecar), then projects the decoded players into the
// opaque public identity space.
package saveroster

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
	"github.com/LukeHollandDev/palworld-live-map/internal/savesidecar"
)

const (
	maxWorldEntries      = 128
	maxGenerationEntries = 2048
	maxPublicIDBytes     = 256
	maxNameBytes         = 96
	claimCandidateFloor  = 16
	claimCycleSlots      = 8
	claimQuizQuestions   = 2
	claimQuizOptions     = 8
	maxClaimPlayerCache  = 32
	maxCachedPlayerBytes = 1 << 20
)

// SnapshotReader is the narrow part of the save decoder used by the adapter.
// The concrete implementation is expected to be *savesidecar.Reader; keeping
// the contract narrow lets selection and projection be tested without a real
// decoder binary.
type SnapshotReader interface {
	ReadSnapshot(context.Context, string) (*savesidecar.Snapshot, error)
}

// ClaimReader is deliberately separate from SnapshotReader: enabling save
// roster enrichment does not implicitly enable private per-player reads.
type ClaimReader interface {
	ReadClaimPlayer(context.Context, string, string) (savesidecar.ClaimPlayer, error)
}

type knowledgeQuizReader interface {
	KnowledgeQuizEnabled() bool
}

// IDProjector turns a private persistent save GUID into an opaque public key.
// Implementations must not return the input GUID or otherwise encode it in the
// result. palworld.Client.PublicPlayerID and PublicGuildKey satisfy this API.
type IDProjector func(string) (string, bool)

type Options struct {
	// Root is the read-only SaveGames/0 directory, not the Saved directory or
	// an individual world directory. It must be mounted read-only: the static
	// no-symlink checks are defense in depth, not confinement against a writer
	// racing pathname inspection.
	Root string
	// WorldID optionally selects one exact 32-character hexadecimal world
	// directory. When empty, exactly one usable world must be discoverable.
	WorldID string
	// Timeout applies a context deadline to discovery and decoding. Readers
	// must cooperate with context cancellation; a decoder subprocess or
	// filesystem syscall already in progress cannot be preempted. Zero relies
	// on the caller's context deadline.
	Timeout time.Duration
	Reader  SnapshotReader

	ProjectPlayerID IDProjector
	// ProjectGuildKey must land save guild GUIDs in the same opaque keyspace the
	// REST path publishes, or save-derived members will form guilds parallel to
	// the REST ones instead of joining them.
	ProjectGuildKey IDProjector

	// ClaimReader and ClaimSecret must either both be provided or both omitted.
	// ClaimSecret is a dedicated persistent installation secret and must not be
	// derived from the Palworld REST password.
	ClaimReader ClaimReader
	ClaimSecret []byte
}

// Source implements palworld.RosterSource.
type Source struct {
	root            string
	worldID         string
	timeout         time.Duration
	reader          SnapshotReader
	projectPlayerID IDProjector
	projectGuildKey IDProjector
	claimReader     ClaimReader
	claimSecret     [sha256.Size]byte
	claimMu         sync.RWMutex
	claimTargets    map[string]claimTarget
	claimPlayers    map[string]claimPlayerCache
	claimCacheClock uint64
}

var (
	_ SnapshotReader             = (*savesidecar.Reader)(nil)
	_ ClaimReader                = (*savesidecar.Reader)(nil)
	_ palworld.RosterSource      = (*Source)(nil)
	_ playerclaim.Prover         = (*Source)(nil)
	_ playerclaim.ProgressSource = (*Source)(nil)
)

func New(options Options) (*Source, error) {
	root := filepath.Clean(strings.TrimSpace(options.Root))
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("save roster root must be an absolute SaveGames/0 path")
	}
	if options.Timeout < 0 {
		return nil, errors.New("save roster timeout cannot be negative")
	}
	worldID := ""
	if options.WorldID != "" {
		if options.WorldID != strings.TrimSpace(options.WorldID) {
			return nil, errors.New("save roster world ID must be exactly 32 hexadecimal characters")
		}
		var ok bool
		worldID, ok = canonicalWorldID(options.WorldID)
		if !ok {
			return nil, errors.New("save roster world ID must be exactly 32 hexadecimal characters")
		}
	}
	if options.Reader == nil {
		return nil, errors.New("save roster requires a snapshot reader")
	}
	if options.ProjectPlayerID == nil {
		return nil, errors.New("save roster requires a player ID projector")
	}
	if options.ProjectGuildKey == nil {
		return nil, errors.New("save roster requires a guild key projector")
	}
	if (options.ClaimReader == nil) != (len(options.ClaimSecret) == 0) {
		return nil, errors.New("save roster claim reader and secret must be configured together")
	}
	if len(options.ClaimSecret) != 0 && len(options.ClaimSecret) != sha256.Size {
		return nil, errors.New("save roster claim secret must be exactly 32 bytes")
	}
	var claimSecret [sha256.Size]byte
	copy(claimSecret[:], options.ClaimSecret)
	return &Source{
		root: root, worldID: worldID, timeout: options.Timeout, reader: options.Reader,
		projectPlayerID: options.ProjectPlayerID, projectGuildKey: options.ProjectGuildKey,
		claimReader: options.ClaimReader, claimSecret: claimSecret, claimTargets: make(map[string]claimTarget),
		claimPlayers: make(map[string]claimPlayerCache),
	}, nil
}

// Roster selects a completed native backup before decoding it. When at least
// two complete generations exist, it deliberately uses the second newest: a
// directory for the newest generation can become visible while Palworld is
// still publishing its files, whereas the preceding generation is immutable.
func (s *Source) Roster(ctx context.Context) (palworld.RosterSnapshot, error) {
	if ctx == nil {
		return palworld.RosterSnapshot{}, errors.New("save roster requires a context")
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	generation, err := s.selectGeneration(ctx)
	if err != nil {
		return palworld.RosterSnapshot{}, err
	}
	snapshot, err := s.reader.ReadSnapshot(ctx, generation.path)
	if err != nil {
		return palworld.RosterSnapshot{}, fmt.Errorf("decode save roster snapshot: %w", err)
	}
	if snapshot == nil {
		return palworld.RosterSnapshot{}, errors.New("decode save roster snapshot: reader returned no snapshot")
	}
	if err := ctx.Err(); err != nil {
		return palworld.RosterSnapshot{}, err
	}

	snapshotAt := snapshot.SnapshotAt.UTC()
	if snapshotAt.IsZero() {
		snapshotAt = generation.snapshotAt.UTC()
	}
	players, claimTargets := s.projectPlayers(ctx, generation.worldID, generation.worldPath, snapshot.Players)
	if err := ctx.Err(); err != nil {
		return palworld.RosterSnapshot{}, err
	}
	if s.claimReader != nil {
		s.claimMu.Lock()
		s.claimTargets = claimTargets
		for subject := range s.claimPlayers {
			if _, exists := s.claimTargetsForSubjectLocked(subject); !exists {
				delete(s.claimPlayers, subject)
			}
		}
		s.claimMu.Unlock()
	}
	var partialError error
	if snapshot.Stats.ResolveFailed {
		// ResolveError may contain decoder stderr, filesystem paths, or private
		// save-authored identifiers. Only a stable category may cross the roster
		// boundary into the poller.
		partialError = errors.New("save decoder resolve failed")
	}
	return palworld.RosterSnapshot{
		SnapshotAt:   snapshotAt,
		Players:      players,
		PartialError: partialError,
	}, nil
}

type generation struct {
	path       string
	name       string
	worldID    string
	worldPath  string
	snapshotAt time.Time
	nameTime   bool
}

type worldCandidate struct {
	generation generation
}

func (s *Source) selectGeneration(ctx context.Context) (generation, error) {
	return s.selectGenerationWithPolicy(ctx, false)
}

// selectClaimGeneration is deliberately stricter than the public roster
// selector. A complete generation is eligible for private claim decoding only
// when another complete generation sorts strictly newer, making the selected
// backup the safely lagged immutable one even when native retention removes
// older generations.
func (s *Source) selectClaimGeneration(ctx context.Context) (generation, error) {
	return s.selectGenerationWithPolicy(ctx, true)
}

func (s *Source) selectGenerationWithPolicy(ctx context.Context, requireNewerComplete bool) (generation, error) {
	entries, err := readDirectoryBounded(ctx, s.root, maxWorldEntries)
	if err != nil {
		return generation{}, fmt.Errorf("inspect save roster root: %w", err)
	}

	worlds := make([]worldCandidate, 0, 1)
	explicitMatches := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return generation{}, err
		}
		candidateID, guid := canonicalWorldID(entry.Name())
		if !guid || (s.worldID != "" && candidateID != s.worldID) {
			continue
		}
		if s.worldID != "" {
			explicitMatches++
		}
		isDirectory, err := directoryEntryWithoutSymlink(entry)
		if err != nil {
			return generation{}, fmt.Errorf("inspect save world directory: %w", err)
		}
		if !isDirectory {
			continue
		}
		worldPath := filepath.Join(s.root, entry.Name())
		complete, err := completeGenerations(ctx, worldPath)
		if err != nil {
			return generation{}, fmt.Errorf("inspect save backup generations: %w", err)
		}
		if len(complete) == 0 || (requireNewerComplete && len(complete) < 2) {
			continue
		}
		selected := complete[0]
		if len(complete) >= 2 {
			selected = complete[1]
		}
		selected.worldID = candidateID
		selected.worldPath = worldPath
		worlds = append(worlds, worldCandidate{generation: selected})
	}

	if s.worldID != "" {
		if explicitMatches > 1 {
			return generation{}, errors.New("save roster world ID matches multiple directories")
		}
		if explicitMatches == 0 {
			return generation{}, errors.New("configured save roster world was not found")
		}
		if len(worlds) != 1 {
			return generation{}, errors.New("configured save roster world has no complete backup generation")
		}
		return worlds[0].generation, nil
	}
	if len(worlds) == 0 {
		return generation{}, errors.New("no world with a complete save backup was found")
	}
	if len(worlds) != 1 {
		return generation{}, errors.New("save roster world discovery is ambiguous; configure a world ID")
	}
	return worlds[0].generation, nil
}

func completeGenerations(ctx context.Context, worldPath string) ([]generation, error) {
	if ok, err := directoryWithoutSymlink(worldPath); err != nil || !ok {
		return nil, err
	}
	backupPath := filepath.Join(worldPath, "backup")
	if ok, err := directoryWithoutSymlink(backupPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	} else if !ok {
		return nil, nil
	}
	generationsPath := filepath.Join(backupPath, "world")
	if ok, err := directoryWithoutSymlink(generationsPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	} else if !ok {
		return nil, nil
	}

	entries, err := readDirectoryBounded(ctx, generationsPath, maxGenerationEntries)
	if err != nil {
		return nil, err
	}
	complete := make([]generation, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		isDirectory, err := directoryEntryWithoutSymlink(entry)
		if err != nil {
			return nil, fmt.Errorf("inspect save backup generation: %w", err)
		}
		if !isDirectory {
			continue
		}
		path := filepath.Join(generationsPath, entry.Name())
		ok, err := completeGeneration(path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect save backup generation: %w", err)
		}
		snapshotAt, nameTime := generationTime(entry.Name(), info.ModTime())
		complete = append(complete, generation{
			path: path, name: entry.Name(), snapshotAt: snapshotAt, nameTime: nameTime,
		})
	}
	sort.Slice(complete, func(i, j int) bool {
		left, right := complete[i], complete[j]
		// Native generations use a sortable timestamp name. If both names are
		// understood, prefer it over mutable directory metadata.
		if left.nameTime && right.nameTime && !left.snapshotAt.Equal(right.snapshotAt) {
			return left.snapshotAt.After(right.snapshotAt)
		}
		if !left.snapshotAt.Equal(right.snapshotAt) {
			return left.snapshotAt.After(right.snapshotAt)
		}
		return left.name > right.name
	})
	return complete, nil
}

func completeGeneration(path string) (bool, error) {
	if ok, err := directoryWithoutSymlink(path); err != nil || !ok {
		return false, err
	}
	for _, name := range []string{"Level.sav", "LevelMeta.sav"} {
		info, err := os.Lstat(filepath.Join(path, name))
		if err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("inspect save backup artifact: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return false, nil
		}
	}
	players, err := directoryWithoutSymlink(filepath.Join(path, "Players"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect save backup players directory: %w", err)
	}
	return players, nil
}

func readDirectoryBounded(ctx context.Context, path string, limit int) ([]os.DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ok, err := directoryWithoutSymlink(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("path is not a non-symlink directory")
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, fmt.Errorf("directory contains more than %d entries", limit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func directoryWithoutSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	return true, nil
}

func directoryEntryWithoutSymlink(entry os.DirEntry) (bool, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir(), nil
}

func generationTime(name string, fallback time.Time) (time.Time, bool) {
	for _, layout := range []string{
		"2006.01.02-15.04.05",
		"2006-01-02_15-04-05",
		time.RFC3339,
	} {
		if parsed, err := time.ParseInLocation(layout, name, time.UTC); err == nil {
			return parsed.UTC(), true
		}
	}
	return fallback.UTC(), false
}

func canonicalWorldID(value string) (string, bool) {
	if len(value) != 32 {
		return "", false
	}
	for _, character := range []byte(value) {
		if !((character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f') ||
			(character >= 'A' && character <= 'F')) {
			return "", false
		}
	}
	return strings.ToLower(value), true
}

type claimTarget struct {
	playerID  string
	worldID   string
	worldPath string
	subject   string
}

func (s *Source) projectPlayers(ctx context.Context, worldID, worldPath string, players []savesidecar.Player) ([]palworld.Player, map[string]claimTarget) {
	type candidate struct {
		id     string
		player palworld.Player
		target claimTarget
	}
	candidates := make([]candidate, 0, len(players))
	idCounts := make(map[string]int, len(players))
	for _, raw := range players {
		if ctx.Err() != nil {
			break
		}
		id, ok := projectID(s.projectPlayerID, raw.PlayerID)
		if !ok {
			continue
		}
		privateID, privateOK := canonicalPrivateGUID(raw.PlayerID)
		player := palworld.Player{ID: id, Online: false, Name: cleanName(raw.Name)}
		if raw.Level > 0 {
			player.Level = raw.Level
		}
		// A guild name without a key cannot be grouped and would render as a
		// guild of its own, so publish the pair or neither.
		if guildKey, ok := projectID(s.projectGuildKey, raw.GuildID); ok {
			player.GuildKey = guildKey
			player.GuildName = cleanName(raw.GuildName)
		}
		if raw.X != nil && raw.Y != nil && finite(*raw.X) && finite(*raw.Y) {
			if mapID, ok := mapdata.LayerID(*raw.X, *raw.Y); ok {
				player.X, player.Y, player.Map = *raw.X, *raw.Y, mapID
			}
		}
		if raw.LastSeenAt != nil && !raw.LastSeenAt.IsZero() {
			player.LastSeenAt = raw.LastSeenAt.UTC()
		}
		player.CaptureTotal = nonNegativeInt64(raw.CaptureTotal)
		player.UniquePalsCaptured = nonNegativeInt(raw.UniquePalsCaptured)
		player.PaldeckUnlocked = nonNegativeInt(raw.PaldeckUnlocked)
		player.ArenaRankPoints = nonNegativeInt(raw.ArenaRankPoints)
		player.FastTravelUnlocked = nonNegativeInt(raw.FastTravelUnlocked)
		player.AreasDiscovered = nonNegativeInt(raw.AreasDiscovered)
		player.BossDefeats = nonNegativeInt(raw.BossDefeats)
		player.TowerDefeats = nonNegativeInt(raw.TowerDefeats)
		idCounts[id]++
		target := claimTarget{}
		if privateOK && s.claimReader != nil {
			target = claimTarget{
				playerID: privateID, worldID: worldID, worldPath: worldPath,
				subject: s.claimSubject(worldID, privateID),
			}
		}
		candidates = append(candidates, candidate{id: id, player: player, target: target})
	}

	result := make([]palworld.Player, 0, len(candidates))
	targets := make(map[string]claimTarget, len(candidates))
	for _, candidate := range candidates {
		// A projector collision must not transfer identity, map position, or
		// guild state between two private save records.
		if idCounts[candidate.id] == 1 {
			result = append(result, candidate.player)
			if candidate.target.subject != "" {
				targets[candidate.id] = candidate.target
			}
		}
	}
	return result, targets
}

func (s *Source) claimSubject(worldID, playerID string) string {
	digest := hmac.New(sha256.New, s.claimSecret[:])
	_, _ = digest.Write([]byte("palworld-live-map/player-claim-subject/v1\x00"))
	_, _ = digest.Write([]byte(worldID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(playerID))
	return hex.EncodeToString(digest.Sum(nil))
}

type claimEvidence struct {
	target            claimTarget
	issuedGenerations map[string]struct{}
	selector          uint64
	phase             claimPhase
	inventory         []savesidecar.ClaimStack
}

type claimQuizEvidence struct {
	target    claimTarget
	correct   map[string]int
	remaining []claimQuizCandidate
}

type claimQuizCandidate struct {
	question playerclaim.QuizQuestion
	correct  int
}

type claimPhase uint8

const (
	claimArming claimPhase = iota
	claimAwaitingProof
	claimAwaitingRestore
)

// claimPlayerCache keeps one immutable decoded generation for a bounded set of
// private subjects. It is never serialized or logged. Reusing this projection
// prevents independent challenges for the same character from multiplying
// sidecar work against an identical save generation.
type claimPlayerCache struct {
	generationPath string
	player         savesidecar.ClaimPlayer
	lastUsed       uint64
}

// Prepare records every generation visible at challenge start but deliberately
// does not read a slot baseline or return instructions yet. Verify first waits
// for a post-start safely immutable generation, then selects a nonce-rich
// eight-slot cycle from at least sixteen distinct stacks. The claimant must
// make the ordered cycle and later restore it across separate safe generations.
func (s *Source) Prepare(ctx context.Context, publicPlayerID string, selector uint64) (playerclaim.Prepared, error) {
	if ctx == nil {
		return playerclaim.Prepared{}, errors.New("prepare save claim: context is required")
	}
	if s.claimReader == nil {
		return playerclaim.Prepared{}, playerclaim.ErrUnavailable
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	s.claimMu.RLock()
	target, ok := s.claimTargets[strings.TrimSpace(publicPlayerID)]
	s.claimMu.RUnlock()
	if !ok {
		return playerclaim.Prepared{}, playerclaim.ErrUnavailable
	}
	baseline, err := s.selectClaimGeneration(ctx)
	if err != nil || baseline.worldID != target.worldID {
		return playerclaim.Prepared{}, playerclaim.ErrUnavailable
	}
	if quizReader, ok := s.claimReader.(knowledgeQuizReader); ok && quizReader.KnowledgeQuizEnabled() {
		player, readErr := s.readClaimPlayerCached(ctx, baseline, target)
		if readErr == nil && player.PlayerID == target.playerID {
			instructions, correct, remaining, selected := selectKnowledgeQuiz(player, selector, baseline.snapshotAt)
			if selected {
				return playerclaim.Prepared{
					Subject: target.subject, PublicPlayerID: strings.TrimSpace(publicPlayerID), Instructions: instructions,
					Evidence: &claimQuizEvidence{target: target, correct: correct, remaining: remaining},
				}, nil
			}
		}
	}
	issued, err := visibleGenerationPaths(ctx, baseline.worldPath)
	if err != nil || len(issued) == 0 {
		return playerclaim.Prepared{}, playerclaim.ErrUnavailable
	}
	return playerclaim.Prepared{
		Subject: target.subject, PublicPlayerID: strings.TrimSpace(publicPlayerID),
		Evidence: &claimEvidence{
			target: target, issuedGenerations: issued, selector: selector, phase: claimArming,
		},
	}, nil
}

// Verify succeeds only when a safely selected immutable generation that did
// not exist at issuance contains the exact requested swap. Unrelated inventory
// activity does not invalidate the proof; it simply never becomes public.
func (s *Source) Verify(ctx context.Context, prepared *playerclaim.Prepared) error {
	if ctx == nil {
		return errors.New("verify save claim: context is required")
	}
	if prepared == nil {
		return playerclaim.ErrUnavailable
	}
	if quiz, ok := prepared.Evidence.(*claimQuizEvidence); ok {
		if quiz == nil || prepared.Subject == "" || prepared.Subject != quiz.target.subject ||
			prepared.Instructions.Kind != playerclaim.InventoryQuiz || len(prepared.Answers) != len(prepared.Instructions.Questions) {
			return playerclaim.ErrUnavailable
		}
		s.claimMu.RLock()
		current, exists := s.claimTargetsForSubjectLocked(quiz.target.subject)
		s.claimMu.RUnlock()
		if !exists || current.playerID != quiz.target.playerID || current.worldID != quiz.target.worldID {
			return playerclaim.ErrUnavailable
		}
		for index, answer := range prepared.Answers {
			correct, exists := quiz.correct[answer.QuestionID]
			if !exists || answer.QuestionID != prepared.Instructions.Questions[index].ID || answer.Option != correct {
				return playerclaim.ErrIncorrectAnswer
			}
		}
		return nil
	}
	evidence, ok := prepared.Evidence.(*claimEvidence)
	if !ok || evidence == nil || prepared.Subject == "" || prepared.Subject != evidence.target.subject {
		return playerclaim.ErrUnavailable
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	s.claimMu.RLock()
	current, exists := s.claimTargetsForSubjectLocked(evidence.target.subject)
	s.claimMu.RUnlock()
	if !exists || current.playerID != evidence.target.playerID || current.worldID != evidence.target.worldID || current.worldPath != evidence.target.worldPath {
		return playerclaim.ErrUnavailable
	}
	generation, err := s.selectClaimGeneration(ctx)
	if err != nil || generation.worldID != evidence.target.worldID || generation.worldPath != evidence.target.worldPath {
		return playerclaim.ErrPending
	}
	if _, existedAtIssue := evidence.issuedGenerations[generation.path]; existedAtIssue {
		return playerclaim.ErrPending
	}
	player, err := s.readClaimPlayerCached(ctx, generation, evidence.target)
	if err != nil || player.PlayerID != evidence.target.playerID {
		return playerclaim.ErrPending
	}
	// A complete selected generation is immutable. Once it has decoded
	// successfully, observing the same non-matching state again cannot advance
	// this challenge and would only repeat expensive sidecar work on every poll.
	evidence.issuedGenerations[generation.path] = struct{}{}
	if evidence.phase == claimArming {
		sequence, ok := selectInventorySequence(player.Common, evidence.selector)
		if !ok {
			return playerclaim.ErrUnavailable
		}
		visibleAfterRead, err := visibleGenerationPaths(ctx, generation.worldPath)
		if err != nil {
			return playerclaim.ErrUnavailable
		}
		evidence.issuedGenerations = visibleAfterRead
		evidence.phase = claimAwaitingProof
		evidence.inventory = sequence
		prepared.Instructions = claimInstructions(sequence, playerclaim.ProofPhaseProve, generation.snapshotAt)
		return playerclaim.ErrReady
	}
	if prepared.Instructions.Kind != playerclaim.InventorySwapSequence {
		return playerclaim.ErrUnavailable
	}
	switch evidence.phase {
	case claimAwaitingProof:
		if !matchesClaimCycle(player.Common, evidence.inventory) {
			return playerclaim.ErrPending
		}
		visibleAfterRead, err := visibleGenerationPaths(ctx, generation.worldPath)
		if err != nil {
			return playerclaim.ErrUnavailable
		}
		evidence.issuedGenerations = visibleAfterRead
		evidence.phase = claimAwaitingRestore
		prepared.Instructions = claimInstructions(evidence.inventory, playerclaim.ProofPhaseRestore, generation.snapshotAt)
		return playerclaim.ErrReady
	case claimAwaitingRestore:
		if matchesClaimBaseline(player.Common, evidence.inventory) {
			return nil
		}
		return playerclaim.ErrPending
	default:
		return playerclaim.ErrUnavailable
	}
}

// CycleQuestion replaces only the requested knowledge question. The remaining
// questions and their answer keys stay unchanged, so a claimant can skip one
// uncertain memory without re-answering the other cards.
func (s *Source) CycleQuestion(ctx context.Context, prepared *playerclaim.Prepared, questionID string) error {
	if ctx == nil || prepared == nil || strings.TrimSpace(questionID) == "" {
		return playerclaim.ErrUnavailable
	}
	evidence, ok := prepared.Evidence.(*claimQuizEvidence)
	if !ok || evidence == nil || prepared.Instructions.Kind != playerclaim.InventoryQuiz {
		return playerclaim.ErrUnavailable
	}
	questionIndex := -1
	for index, question := range prepared.Instructions.Questions {
		if question.ID == questionID {
			questionIndex = index
			break
		}
	}
	if questionIndex < 0 {
		return playerclaim.ErrUnavailable
	}
	if len(evidence.remaining) == 0 {
		return playerclaim.ErrNoAlternateQuestion
	}

	nextEvidence := cloneClaimQuizEvidence(evidence)
	next := nextEvidence.remaining[0]
	nextEvidence.remaining = nextEvidence.remaining[1:]
	delete(nextEvidence.correct, questionID)
	nextEvidence.correct[next.question.ID] = next.correct
	canCycle := len(nextEvidence.remaining) > 0

	questions := append([]playerclaim.QuizQuestion(nil), prepared.Instructions.Questions...)
	for index := range questions {
		questions[index].Options = append([]string(nil), questions[index].Options...)
		questions[index].CanCycle = canCycle
	}
	next.question.Options = append([]string(nil), next.question.Options...)
	next.question.CanCycle = canCycle
	questions[questionIndex] = next.question
	prepared.Instructions.Questions = questions
	prepared.Evidence = nextEvidence
	return nil
}

func cloneClaimQuizEvidence(source *claimQuizEvidence) *claimQuizEvidence {
	result := &claimQuizEvidence{
		target: source.target, correct: make(map[string]int, len(source.correct)),
		remaining: make([]claimQuizCandidate, len(source.remaining)),
	}
	for id, correct := range source.correct {
		result.correct[id] = correct
	}
	for index, candidate := range source.remaining {
		candidate.question.Options = append([]string(nil), candidate.question.Options...)
		result.remaining[index] = candidate
	}
	return result
}

func selectKnowledgeQuiz(player savesidecar.ClaimPlayer, selector uint64, snapshotAt time.Time) (playerclaim.Instructions, map[string]int, []claimQuizCandidate, bool) {
	if snapshotAt.IsZero() {
		return playerclaim.Instructions{}, nil, nil, false
	}
	facts := make([]claimQuizFact, 0, len(player.Common)+len(player.DropSlot)+len(player.Essential)+len(player.Weapons)+len(player.Armor)+len(player.Food)+len(player.Party))
	appendStackQuizFacts(&facts, player.Common, "Which item was in common-inventory slot %d?", itemQuizDecoys)
	appendStackQuizFacts(&facts, player.DropSlot, "Which item was in dropped-items slot %d?", itemQuizDecoys)
	appendStackQuizFacts(&facts, player.Essential, "Which key item was in key-items slot %d?", essentialQuizDecoys)
	appendStackQuizFacts(&facts, player.Weapons, "Which weapon was equipped in slot %d?", weaponQuizDecoys)
	appendStackQuizFacts(&facts, player.Armor, "Which armor or accessory was equipped in slot %d?", armorQuizDecoys)
	appendStackQuizFacts(&facts, player.Food, "Which food was equipped in slot %d?", foodQuizDecoys)
	for _, pal := range player.Party {
		label := humanizeItemID(pal.Species)
		if pal.Slot < 0 || label == "" {
			continue
		}
		facts = append(facts, claimQuizFact{
			prompt: fmt.Sprintf("Which Pal was in party slot %d?", pal.Slot+1), value: label, decoys: palQuizDecoys,
		})
	}
	if len(facts) < claimQuizQuestions {
		return playerclaim.Instructions{}, nil, nil, false
	}
	state := selector ^ 0xd1b54a32d192ed03
	for index := len(facts) - 1; index > 0; index-- {
		state = claimRandom(state)
		other := int(state % uint64(index+1))
		facts[index], facts[other] = facts[other], facts[index]
	}
	if len(facts) > 24 {
		facts = facts[:24]
	}
	candidates := make([]claimQuizCandidate, 0, len(facts))
	for index, fact := range facts {
		candidate, ok := makeQuizCandidate(fact, fmt.Sprintf("q%d", index+1), &state)
		if ok {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) < claimQuizQuestions {
		return playerclaim.Instructions{}, nil, nil, false
	}
	questions := make([]playerclaim.QuizQuestion, claimQuizQuestions)
	correct := make(map[string]int, claimQuizQuestions)
	remaining := append([]claimQuizCandidate(nil), candidates[claimQuizQuestions:]...)
	canCycle := len(remaining) > 0
	for index, candidate := range candidates[:claimQuizQuestions] {
		candidate.question.CanCycle = canCycle
		questions[index] = candidate.question
		correct[candidate.question.ID] = candidate.correct
	}
	return playerclaim.Instructions{Kind: playerclaim.InventoryQuiz, Questions: questions, SnapshotAt: snapshotAt}, correct, remaining, true
}

type claimQuizFact struct {
	prompt string
	value  string
	decoys []string
}

func appendStackQuizFacts(destination *[]claimQuizFact, stacks []savesidecar.ClaimStack, prompt string, decoys []string) {
	for _, stack := range stacks {
		label := humanizeItemID(stack.ItemID)
		if !validClaimStack(stack) || label == "" {
			continue
		}
		*destination = append(*destination, claimQuizFact{
			prompt: fmt.Sprintf(prompt, stack.Slot+1), value: label, decoys: decoys,
		})
	}
}

func makeQuizCandidate(fact claimQuizFact, id string, state *uint64) (claimQuizCandidate, bool) {
	options := make([]string, 0, claimQuizOptions)
	seen := make(map[string]struct{}, claimQuizOptions)
	appendOption := func(option string) {
		option = strings.TrimSpace(option)
		key := strings.ToLower(option)
		if option == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		options = append(options, option)
	}
	appendOption(fact.value)
	decoys := append([]string(nil), fact.decoys...)
	for index := len(decoys) - 1; index > 0; index-- {
		*state = claimRandom(*state)
		other := int(*state % uint64(index+1))
		decoys[index], decoys[other] = decoys[other], decoys[index]
	}
	for _, decoy := range decoys {
		appendOption(decoy)
		if len(options) == claimQuizOptions {
			break
		}
	}
	if len(options) != claimQuizOptions {
		return claimQuizCandidate{}, false
	}
	for index := len(options) - 1; index > 0; index-- {
		*state = claimRandom(*state)
		other := int(*state % uint64(index+1))
		options[index], options[other] = options[other], options[index]
	}
	correct := -1
	for index, option := range options {
		if strings.EqualFold(option, fact.value) {
			correct = index
			break
		}
	}
	if correct < 0 {
		return claimQuizCandidate{}, false
	}
	return claimQuizCandidate{question: playerclaim.QuizQuestion{ID: id, Prompt: fact.prompt, Options: options}, correct: correct}, true
}

var itemQuizDecoys = []string{
	"Wood", "Stone", "Fiber", "Paldium Fragment", "Ore", "Coal", "Sulfur", "Quartz", "Polymer",
	"High Quality Pal Oil", "Ancient Civilization Parts", "Dog Coin", "Gold Coin", "Pal Sphere",
	"Mega Pal Sphere", "Giga Pal Sphere", "Hyper Pal Sphere", "Ultra Pal Sphere", "Legendary Sphere",
	"Repair Kit", "Medical Supplies", "Gunpowder", "Circuit Board", "Carbon Fiber",
}

var weaponQuizDecoys = []string{
	"Old Bow", "Crossbow", "Handgun", "Makeshift Handgun", "Single Shot Rifle", "Double Barreled Shotgun",
	"Pump Action Shotgun", "Assault Rifle", "Rocket Launcher", "Laser Rifle", "Gatling Gun", "Grenade Launcher",
}

var armorQuizDecoys = []string{
	"Cloth Outfit", "Pelt Armor", "Metal Armor", "Refined Metal Armor", "Pal Metal Armor", "Heat Resistant Armor",
	"Cold Resistant Armor", "Plasteel Armor", "Lightweight Plasteel Armor", "Life Pendant", "Attack Pendant", "Defense Pendant",
}

var foodQuizDecoys = []string{
	"Baked Berries", "Jam Filled Bun", "Salad", "Omelet", "Pancake", "Pizza", "Cake", "Grilled Lamball",
	"Fried Chikipi", "Marinated Mushrooms", "Mozzarina Cheeseburger", "Carbonara",
}

var essentialQuizDecoys = []string{
	"Normal Parachute", "Mega Glider", "Giga Glider", "Hyper Glider", "Grappling Gun", "Mega Grappling Gun",
	"Giga Grappling Gun", "Hyper Grappling Gun", "Lockpicking Tool", "Feed Bag", "Lantern", "Pal Essence Condenser",
}

var palQuizDecoys = []string{
	"Lamball", "Cattiva", "Chikipi", "Foxparks", "Lifmunk", "Pengullet", "Tanzee", "Daedream", "Direhowl",
	"Tocotoco", "Eikthyrdeer", "Nitewing", "Dumud", "Dinossom", "Mossanda", "Anubis", "Grizzbolt",
	"Jetragon", "Frostallion", "Orserk", "Lyleen", "Shadowbeak", "Blazamut", "Knocklem",
}

func humanizeItemID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var result strings.Builder
	var previous rune
	for _, current := range value {
		if current == '_' || current == '-' {
			if result.Len() > 0 && previous != ' ' {
				result.WriteByte(' ')
				previous = ' '
			}
			continue
		}
		if result.Len() > 0 && unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			result.WriteByte(' ')
		}
		result.WriteRune(current)
		previous = current
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

// Progress resolves exact keys for one already-authenticated private subject.
// Results are cached by immutable generation path and never enter the public
// roster snapshot.
func (s *Source) Progress(ctx context.Context, subject string) (playerclaim.PrivateProgress, error) {
	if ctx == nil {
		return playerclaim.PrivateProgress{}, errors.New("read save claim progress: context is required")
	}
	if s.claimReader == nil || strings.TrimSpace(subject) == "" {
		return playerclaim.PrivateProgress{}, playerclaim.ErrUnavailable
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	s.claimMu.RLock()
	target, exists := s.claimTargetsForSubjectLocked(subject)
	s.claimMu.RUnlock()
	if !exists {
		return playerclaim.PrivateProgress{}, playerclaim.ErrUnavailable
	}
	generation, err := s.selectClaimGeneration(ctx)
	if err != nil || generation.worldID != target.worldID || generation.worldPath != target.worldPath {
		return playerclaim.PrivateProgress{}, playerclaim.ErrUnavailable
	}
	player, err := s.readClaimPlayerCached(ctx, generation, target)
	if err != nil || player.PlayerID != target.playerID || !player.Progress.Available {
		return playerclaim.PrivateProgress{}, playerclaim.ErrUnavailable
	}
	return privateProgress(generation.snapshotAt, player.Progress), nil
}

func (s *Source) readClaimPlayerCached(ctx context.Context, generation generation, target claimTarget) (savesidecar.ClaimPlayer, error) {
	s.claimMu.Lock()
	if cached, ok := s.claimPlayers[target.subject]; ok && cached.generationPath == generation.path {
		s.claimCacheClock++
		cached.lastUsed = s.claimCacheClock
		s.claimPlayers[target.subject] = cached
		player := cloneCachedClaimPlayer(cached.player)
		s.claimMu.Unlock()
		return player, nil
	}
	s.claimMu.Unlock()

	player, err := s.claimReader.ReadClaimPlayer(ctx, generation.path, target.playerID)
	if err != nil || player.PlayerID != target.playerID || !cacheableClaimPlayer(player) {
		return player, err
	}

	s.claimMu.Lock()
	if s.claimPlayers == nil {
		s.claimPlayers = make(map[string]claimPlayerCache)
	}
	if _, exists := s.claimPlayers[target.subject]; !exists && len(s.claimPlayers) >= maxClaimPlayerCache {
		victim := ""
		var oldest uint64
		for subject, cached := range s.claimPlayers {
			if victim == "" || cached.lastUsed < oldest || (cached.lastUsed == oldest && subject < victim) {
				victim, oldest = subject, cached.lastUsed
			}
		}
		delete(s.claimPlayers, victim)
	}
	s.claimCacheClock++
	s.claimPlayers[target.subject] = claimPlayerCache{
		generationPath: generation.path,
		player:         cloneCachedClaimPlayer(player),
		lastUsed:       s.claimCacheClock,
	}
	s.claimMu.Unlock()
	return player, nil
}

func cacheableClaimPlayer(player savesidecar.ClaimPlayer) bool {
	containers := [][]savesidecar.ClaimStack{
		player.Common, player.DropSlot, player.Essential, player.Weapons, player.Armor, player.Food,
	}
	size := len(player.PlayerID) + len(player.Party)*32
	for _, container := range containers {
		size += len(container) * 64
		for _, stack := range container {
			size += len(stack.ItemID) + len(stack.DynamicItemID)
		}
	}
	for _, pal := range player.Party {
		size += len(pal.InstanceID) + len(pal.Species)
	}
	for _, keys := range [][]string{
		player.Progress.FastTravel, player.Progress.Areas, player.Progress.Notes,
		player.Progress.Relics, player.Progress.ItemPickups,
		player.Progress.NormalBosses, player.Progress.TowerBosses,
	} {
		size += len(keys) * 16
		for _, key := range keys {
			size += len(key)
		}
	}
	return size <= maxCachedPlayerBytes
}

func cloneCachedClaimPlayer(player savesidecar.ClaimPlayer) savesidecar.ClaimPlayer {
	player.Common = append([]savesidecar.ClaimStack(nil), player.Common...)
	player.DropSlot = append([]savesidecar.ClaimStack(nil), player.DropSlot...)
	player.Essential = append([]savesidecar.ClaimStack(nil), player.Essential...)
	player.Weapons = append([]savesidecar.ClaimStack(nil), player.Weapons...)
	player.Armor = append([]savesidecar.ClaimStack(nil), player.Armor...)
	player.Food = append([]savesidecar.ClaimStack(nil), player.Food...)
	player.Party = append([]savesidecar.ClaimPal(nil), player.Party...)
	player.Progress.FastTravel = append([]string(nil), player.Progress.FastTravel...)
	player.Progress.Areas = append([]string(nil), player.Progress.Areas...)
	player.Progress.Notes = append([]string(nil), player.Progress.Notes...)
	player.Progress.Relics = append([]string(nil), player.Progress.Relics...)
	player.Progress.ItemPickups = append([]string(nil), player.Progress.ItemPickups...)
	player.Progress.NormalBosses = append([]string(nil), player.Progress.NormalBosses...)
	player.Progress.TowerBosses = append([]string(nil), player.Progress.TowerBosses...)
	return player
}

func privateProgress(snapshotAt time.Time, progress savesidecar.ClaimProgress) playerclaim.PrivateProgress {
	return playerclaim.PrivateProgress{
		SnapshotAt:     snapshotAt.UTC(),
		FastTravelKeys: append([]string{}, progress.FastTravel...),
		AreaKeys:       append([]string{}, progress.Areas...),
		NoteKeys:       append([]string{}, progress.Notes...),
		RelicKeys:      append([]string{}, progress.Relics...),
		ItemPickupKeys: append([]string{}, progress.ItemPickups...),
		NormalBossKeys: append([]string{}, progress.NormalBosses...),
		TowerBossKeys:  append([]string{}, progress.TowerBosses...),
	}
}

func visibleGenerationPaths(ctx context.Context, worldPath string) (map[string]struct{}, error) {
	if ok, err := directoryWithoutSymlink(worldPath); err != nil || !ok {
		return nil, err
	}
	generationsPath := filepath.Join(worldPath, "backup", "world")
	if ok, err := directoryWithoutSymlink(generationsPath); err != nil || !ok {
		return nil, err
	}
	entries, err := readDirectoryBounded(ctx, generationsPath, maxGenerationEntries)
	if err != nil {
		return nil, err
	}
	paths := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		isDirectory, err := directoryEntryWithoutSymlink(entry)
		if err != nil {
			return nil, err
		}
		if isDirectory {
			paths[filepath.Join(generationsPath, entry.Name())] = struct{}{}
		}
	}
	return paths, nil
}

func (s *Source) claimTargetsForSubjectLocked(subject string) (claimTarget, bool) {
	var result claimTarget
	found := false
	for _, target := range s.claimTargets {
		if target.subject != subject {
			continue
		}
		if found {
			return claimTarget{}, false
		}
		result, found = target, true
	}
	return result, found
}

// selectInventorySequence chooses an eight-slot cycle from at least sixteen
// stacks whose complete save fingerprints are unique. There are more than
// 2^25 possible labelled cycles at the minimum candidate count, so an unseen
// pre-instruction inventory state has negligible chance of matching the
// nonce-selected target. Selection is O(n), bounded by sidecar validation.
func selectInventorySequence(stacks []savesidecar.ClaimStack, selector uint64) ([]savesidecar.ClaimStack, bool) {
	counts := make(map[claimStackKey]int, len(stacks))
	for _, stack := range stacks {
		if validClaimStack(stack) {
			counts[claimStackFingerprint(stack)]++
		}
	}
	candidates := make([]savesidecar.ClaimStack, 0, len(stacks))
	for _, stack := range stacks {
		if validClaimStack(stack) && counts[claimStackFingerprint(stack)] == 1 {
			candidates = append(candidates, stack)
		}
	}
	if len(candidates) < claimCandidateFloor {
		return nil, false
	}
	state := selector ^ 0x9e3779b97f4a7c15
	for index := len(candidates) - 1; index > 0; index-- {
		state = claimRandom(state)
		other := int(state % uint64(index+1))
		candidates[index], candidates[other] = candidates[other], candidates[index]
	}
	return append([]savesidecar.ClaimStack{}, candidates[:claimCycleSlots]...), true
}

func claimRandom(state uint64) uint64 {
	state ^= state >> 12
	state ^= state << 25
	state ^= state >> 27
	return state * 0x2545f4914f6cdd1d
}

func validClaimStack(stack savesidecar.ClaimStack) bool {
	return stack.ItemID != "" && stack.Count > 0
}

type claimStackKey struct {
	itemID        string
	count         uint32
	dynamicItemID string
}

func claimStackFingerprint(stack savesidecar.ClaimStack) claimStackKey {
	return claimStackKey{itemID: stack.ItemID, count: stack.Count, dynamicItemID: stack.DynamicItemID}
}

func claimInstructions(sequence []savesidecar.ClaimStack, phase playerclaim.ProofPhase, snapshotAt time.Time) playerclaim.Instructions {
	pairs := make([]playerclaim.SlotPair, 0, len(sequence)-1)
	if len(sequence) == claimCycleSlots {
		for index := 1; index < len(sequence); index++ {
			pairs = append(pairs, playerclaim.SlotPair{SlotA: int(sequence[0].Slot) + 1, SlotB: int(sequence[index].Slot) + 1})
		}
		if phase == playerclaim.ProofPhaseRestore {
			for left, right := 0, len(pairs)-1; left < right; left, right = left+1, right-1 {
				pairs[left], pairs[right] = pairs[right], pairs[left]
			}
		}
	}
	step := 1
	if phase == playerclaim.ProofPhaseRestore {
		step = 2
	}
	return playerclaim.Instructions{
		Kind: playerclaim.InventorySwapSequence, Phase: phase, Step: step, TotalSteps: 2,
		Pairs: pairs, SnapshotAt: snapshotAt.UTC(),
	}
}

func matchesClaimCycle(stacks, sequence []savesidecar.ClaimStack) bool {
	if len(sequence) != claimCycleSlots {
		return false
	}
	for index, original := range sequence {
		want := sequence[len(sequence)-1]
		if index > 0 {
			want = sequence[index-1]
		}
		current, ok := claimStackAt(stacks, original.Slot)
		if !ok || !sameStack(current, want) {
			return false
		}
	}
	return true
}

func matchesClaimBaseline(stacks, sequence []savesidecar.ClaimStack) bool {
	if len(sequence) != claimCycleSlots {
		return false
	}
	for _, original := range sequence {
		current, ok := claimStackAt(stacks, original.Slot)
		if !ok || !sameStack(current, original) {
			return false
		}
	}
	return true
}

func claimStackAt(stacks []savesidecar.ClaimStack, slot uint32) (savesidecar.ClaimStack, bool) {
	for _, stack := range stacks {
		if stack.Slot == slot {
			return stack, true
		}
	}
	return savesidecar.ClaimStack{}, false
}

func sameStack(left, right savesidecar.ClaimStack) bool {
	return left.ItemID == right.ItemID && left.Count == right.Count && left.DynamicItemID == right.DynamicItemID
}

func nonNegativeInt64(value *int64) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	copy := *value
	return &copy
}

func nonNegativeInt(value *int) *int {
	if value == nil || *value < 0 {
		return nil
	}
	copy := *value
	return &copy
}

func projectID(project IDProjector, raw string) (string, bool) {
	value, ok := project(raw)
	value = strings.TrimSpace(value)
	if !ok || value == "" || len(value) > maxPublicIDBytes || !utf8.ValidString(value) {
		return "", false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	// Treat projectors as a security boundary, but also fail closed on the
	// most dangerous configuration mistakes: identity and prefix+identity
	// projectors. Production projectors are keyed HMACs and never contain the
	// canonical private GUID.
	if private, ok := canonicalPrivateGUID(raw); ok {
		publicComparable := strings.ToLower(strings.ReplaceAll(value, "-", ""))
		if strings.Contains(publicComparable, private) {
			return "", false
		}
	}
	return value, true
}

func canonicalPrivateGUID(value string) (string, bool) {
	value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "")
	return canonicalWorldID(value)
}

// cleanName bounds and strips control characters from a save-authored string.
// Names reach browsers, and the save is player-controlled input, so it gets the
// same treatment as the REST path's names.
func cleanName(value string) string {
	value = strings.TrimSpace(strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, value))
	if len(value) > maxNameBytes {
		value = value[:maxNameBytes]
		for len(value) > 0 && !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	if !utf8.ValidString(value) {
		return ""
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
