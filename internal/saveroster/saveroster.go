// Package saveroster adapts immutable Palworld native backup generations to
// the public roster consumed by the live-map poller.
package saveroster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/savegame"
)

const (
	maxWorldEntries      = 128
	maxGenerationEntries = 512
	maxPublicIDBytes     = 256
	maxNameBytes         = 96
	maxPlayerLevel       = 999
)

// SnapshotReader is the narrow part of savegame.Reader used by the adapter.
// The concrete implementation is expected to be *savegame.Reader; keeping the
// contract narrow lets selection and projection be tested without Oodle.
type SnapshotReader interface {
	ReadSnapshot(context.Context, string) (*savegame.Snapshot, error)
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
	// must cooperate with context cancellation; a native decompression call or
	// filesystem syscall already in progress cannot be preempted. Zero relies
	// on the caller's context deadline.
	Timeout time.Duration
	Reader  SnapshotReader

	ProjectPlayerID IDProjector
	ProjectGuildID  IDProjector
}

// Source implements palworld.RosterSource.
type Source struct {
	root            string
	worldID         string
	timeout         time.Duration
	reader          SnapshotReader
	projectPlayerID IDProjector
	projectGuildID  IDProjector
}

var (
	_ SnapshotReader        = (*savegame.Reader)(nil)
	_ palworld.RosterSource = (*Source)(nil)
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
	if options.ProjectPlayerID == nil || options.ProjectGuildID == nil {
		return nil, errors.New("save roster requires player and guild ID projectors")
	}
	return &Source{
		root: root, worldID: worldID, timeout: options.Timeout, reader: options.Reader,
		projectPlayerID: options.ProjectPlayerID, projectGuildID: options.ProjectGuildID,
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
	players := s.projectPlayers(ctx, snapshot.Players)
	if err := ctx.Err(); err != nil {
		return palworld.RosterSnapshot{}, err
	}
	return palworld.RosterSnapshot{
		SnapshotAt: snapshotAt,
		Players:    players,
	}, nil
}

type generation struct {
	path       string
	name       string
	snapshotAt time.Time
	nameTime   bool
}

type worldCandidate struct {
	generation generation
}

func (s *Source) selectGeneration(ctx context.Context) (generation, error) {
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
		if len(complete) == 0 {
			continue
		}
		selected := complete[0]
		if len(complete) >= 2 {
			selected = complete[1]
		}
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

func (s *Source) projectPlayers(ctx context.Context, players []savegame.Player) []palworld.Player {
	type candidate struct {
		id     string
		player palworld.Player
	}
	candidates := make([]candidate, 0, len(players))
	idCounts := make(map[string]int, len(players))
	for _, raw := range players {
		if ctx.Err() != nil {
			break
		}
		name := cleanName(raw.DisplayName)
		id, ok := projectID(s.projectPlayerID, raw.PlayerID)
		if !ok || name == "" {
			continue
		}
		player := palworld.Player{
			ID: id, Name: name, Level: sanitizeLevel(raw.Level), Online: false,
		}
		if raw.GuildID != "" {
			if guildID, ok := projectID(s.projectGuildID, raw.GuildID); ok {
				player.GuildKey = guildID
				player.GuildName = cleanName(raw.GuildName)
			}
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
		idCounts[id]++
		candidates = append(candidates, candidate{id: id, player: player})
	}

	result := make([]palworld.Player, 0, len(candidates))
	for _, candidate := range candidates {
		// A projector collision must not transfer identity, map position, or
		// guild state between two private save records.
		if idCounts[candidate.id] == 1 {
			result = append(result, candidate.player)
		}
	}
	return result
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

func cleanName(value string) string {
	value = strings.TrimSpace(strings.Map(func(character rune) rune {
		// Format controls include bidi overrides/isolates. Line and paragraph
		// separators can likewise spoof otherwise single-line labels and logs.
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return character
	}, value))
	if len(value) <= maxNameBytes {
		return value
	}
	value = value[:maxNameBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return strings.TrimSpace(value)
}

func canonicalPrivateGUID(value string) (string, bool) {
	value = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "")
	return canonicalWorldID(value)
}

func sanitizeLevel(level int32) int {
	if level < 0 {
		return 0
	}
	if level > maxPlayerLevel {
		return maxPlayerLevel
	}
	return int(level)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
