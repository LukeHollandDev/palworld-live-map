// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
//
// Package savegame provides bounded, read-only extraction of persisted player
// roster data from an immutable Palworld save snapshot.
package savegame

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Reader is immutable after construction and safe for concurrent use.
type Reader struct {
	maxSaveBytes int64
	maxPlayers   int
}

// NewReader validates options. It performs no downloads and never writes to
// the save tree.
func NewReader(opts Options) (*Reader, error) {
	maxBytes := opts.MaxSaveBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxSaveBytes
	}
	if maxBytes <= 0 || maxBytes > hardMaxSaveBytes {
		return nil, fmt.Errorf("savegame: MaxSaveBytes must be within 1..%d", hardMaxSaveBytes)
	}
	maxPlayers := opts.MaxPlayers
	if maxPlayers == 0 {
		maxPlayers = defaultMaxPlayers
	}
	if maxPlayers <= 0 || maxPlayers > hardMaxPlayers {
		return nil, fmt.Errorf("savegame: MaxPlayers must be within 1..%d", hardMaxPlayers)
	}
	return &Reader{maxSaveBytes: maxBytes, maxPlayers: maxPlayers}, nil
}

// ReadSnapshot reads Level.sav and Players/*.sav from snapshotDir without
// modifying them. Callers should provide an immutable copy made by their
// snapshot layer, never the actively-written server save directory.
func (r *Reader) ReadSnapshot(ctx context.Context, snapshotDir string) (*Snapshot, error) {
	if r == nil {
		return nil, fmt.Errorf("savegame: Reader is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := filepath.Abs(snapshotDir)
	if err != nil {
		return nil, fmt.Errorf("savegame: resolve snapshot directory: %w", err)
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("savegame: stat snapshot directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("savegame: snapshot path is not a regular directory")
	}
	levelPath := filepath.Join(dir, "Level.sav")
	raw, levelInfo, err := readSave(levelPath, r.maxSaveBytes)
	if err != nil {
		return nil, fmt.Errorf("savegame: Level.sav: %w", err)
	}
	stats := newStats()
	gvas, err := parseGVAS(raw, &stats)
	if err != nil {
		return nil, fmt.Errorf("savegame: parse Level.sav: %w", err)
	}
	players, err := extractLevelPlayers(gvas, r.maxPlayers, &stats)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		SnapshotAt: levelInfo.ModTime().UTC(),
		Players:    players,
		Stats:      stats,
	}
	if err = r.loadPlayerSaves(ctx, filepath.Join(dir, "Players"), snapshot); err != nil {
		return nil, err
	}
	// Catch a snapshot source that changed while its player files were parsed.
	after, err := os.Lstat(levelPath)
	if err != nil || after.Size() != levelInfo.Size() || !after.ModTime().Equal(levelInfo.ModTime()) {
		return nil, fmt.Errorf("savegame: Level.sav changed during snapshot extraction")
	}
	sort.Slice(snapshot.Players, func(i, j int) bool {
		return snapshot.Players[i].PlayerID < snapshot.Players[j].PlayerID
	})
	return snapshot, nil
}

func (r *Reader) loadPlayerSaves(ctx context.Context, dir string, snapshot *Snapshot) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("savegame: read Players directory: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if entry.IsDir() || !strings.HasSuffix(lower, ".sav") || strings.HasSuffix(lower, "_dps.sav") {
			continue
		}
		files = append(files, name)
	}
	if len(files) > r.maxPlayers {
		return &parseLimitError{Kind: "player save files", Value: uint64(len(files)), Limit: uint64(r.maxPlayers)}
	}
	sort.Strings(files)
	byID := make(map[string]int, len(snapshot.Players))
	for i := range snapshot.Players {
		byID[strings.ToLower(snapshot.Players[i].PlayerID)] = i
	}
	merged := make(map[string]time.Time)
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		snapshot.Stats.PlayerFiles++
		raw, _, readErr := readSave(filepath.Join(dir, name), r.maxSaveBytes)
		if readErr != nil {
			snapshot.Stats.DecodeFailures["playerSaves"]++
			continue
		}
		gvas, parseErr := parseGVAS(raw, &snapshot.Stats)
		if parseErr != nil {
			snapshot.Stats.DecodeFailures["playerSaves"]++
			continue
		}
		state, stateErr := playerStateFromGVAS(gvas)
		if stateErr != nil {
			snapshot.Stats.DecodeFailures["playerSaves"]++
			continue
		}
		index, exists := byID[state.uid]
		if !exists {
			snapshot.Stats.DecodeFailures["unmatchedPlayerSaves"]++
			continue
		}
		if previous, exists := merged[state.uid]; exists {
			snapshot.Stats.DuplicatePlayers++
			if state.lastSeen == nil || (!previous.IsZero() && !state.lastSeen.After(previous)) {
				continue
			}
		}
		player := &snapshot.Players[index]
		if state.location != nil {
			x, y := state.location.X, state.location.Y
			player.X, player.Y = &x, &y
		}
		if state.lastSeen != nil {
			t := state.lastSeen.UTC()
			player.LastSeenAt = &t
			merged[state.uid] = t
		} else {
			merged[state.uid] = time.Time{}
		}
		player.CaptureTotal = state.captureTotal
		player.UniquePalsCaptured = state.uniquePalsCaptured
		player.PaldeckUnlocked = state.paldeckUnlocked
	}
	return nil
}

type playerState struct {
	uid                string
	location           *Vector
	lastSeen           *time.Time
	captureTotal       *int64
	uniquePalsCaptured *int
	paldeckUnlocked    *int
}

func playerStateFromGVAS(gvas *gvasFile) (playerState, error) {
	data, ok := propertyProperties(gvas.Properties, "SaveData")
	if !ok {
		return playerState{}, fmt.Errorf("missing SaveData")
	}
	uid := strings.ToLower(firstString(data, "PlayerUId", "PlayerUID", "PlayerUid"))
	if !validGUID(uid) || uid == zeroGUID {
		return playerState{}, fmt.Errorf("invalid PlayerUId")
	}
	state := playerState{uid: uid}
	if transform, ok := propertyProperties(data, "LastTransform"); ok {
		if location, ok := firstVector(transform, "Translation"); ok && finiteLocation(location) {
			state.location = &location
		}
	}
	if ticks, ok := propertyDateTime(data, "LastOnlineDateTime"); ok {
		if lastSeen, ok := unrealDateTime(ticks); ok {
			state.lastSeen = &lastSeen
		}
	}
	decodePlayerProgress(data, &state)
	return state, nil
}

func decodePlayerProgress(data propertyMap, state *playerState) {
	record, ok := propertyProperties(data, "RecordData")
	if !ok {
		return
	}
	// Palworld's TribeCaptureCount is the authoritative distinct Pal-species
	// count. PalCaptureCount also contains a Human key on servers where people
	// have been captured, so counting that map directly is not equivalent.
	if value, ok := propertyInt(record, "TribeCaptureCount"); ok && value >= 0 && value <= math.MaxInt32 {
		state.uniquePalsCaptured = intPointer(int(value))
	}
	if entries, ok := propertyMapValues(record, "PalCaptureCount"); ok {
		seen := make(map[string]struct{}, len(entries))
		var total int64
		valid := true
		for _, entry := range entries {
			key, keyOK := entry.Key.(string)
			value, valueOK := numericValue(entry.Value)
			key = strings.TrimSpace(key)
			if !keyOK || key == "" || !valueOK || value < 0 || value > math.MaxInt32 {
				valid = false
				break
			}
			canonicalKey := strings.ToLower(key)
			if _, duplicate := seen[canonicalKey]; duplicate {
				valid = false
				break
			}
			seen[canonicalKey] = struct{}{}
			if canonicalKey == "human" {
				continue
			}
			total += value
		}
		if valid {
			state.captureTotal = int64Pointer(total)
		}
	}
	if entries, ok := propertyMapValues(record, "PaldeckUnlockFlag"); ok {
		seen := make(map[string]struct{}, len(entries))
		count := 0
		valid := true
		for _, entry := range entries {
			key, keyOK := entry.Key.(string)
			unlocked, valueOK := entry.Value.(bool)
			key = strings.TrimSpace(key)
			if !keyOK || key == "" || !valueOK {
				valid = false
				break
			}
			canonicalKey := strings.ToLower(key)
			if _, duplicate := seen[canonicalKey]; duplicate {
				valid = false
				break
			}
			seen[canonicalKey] = struct{}{}
			if unlocked && canonicalKey != "human" {
				count++
			}
		}
		if valid {
			state.paldeckUnlocked = intPointer(count)
		}
	}
}

func propertyMapValues(properties propertyMap, name string) ([]mapEntry, bool) {
	property := properties[name]
	if property == nil {
		return nil, false
	}
	values, ok := property.Value.([]mapEntry)
	return values, ok
}

func numericValue(value any) (int64, bool) {
	switch value := value.(type) {
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value <= math.MaxInt64 {
			return int64(value), true
		}
	}
	return 0, false
}

func int64Pointer(value int64) *int64 { return &value }
func intPointer(value int) *int       { return &value }

func propertyDateTime(p propertyMap, name string) (uint64, bool) {
	q := p[name]
	if q == nil {
		return 0, false
	}
	s, ok := q.Value.(structData)
	if !ok || s.Type != "DateTime" {
		return 0, false
	}
	v, ok := s.Value.(uint64)
	return v, ok
}

func finiteLocation(v Vector) bool {
	const maxWorldCoordinate = 1e10
	for _, value := range []float64{v.X, v.Y, v.Z} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -maxWorldCoordinate || value > maxWorldCoordinate {
			return false
		}
	}
	return true
}

const unrealUnixEpochTicks uint64 = 621355968000000000

func unrealDateTime(ticks uint64) (time.Time, bool) {
	if ticks < unrealUnixEpochTicks {
		return time.Time{}, false
	}
	delta := ticks - unrealUnixEpochTicks
	seconds := delta / 10_000_000
	if seconds > 253402300799 { // 9999-12-31T23:59:59Z
		return time.Time{}, false
	}
	nanos := (delta % 10_000_000) * 100
	return time.Unix(int64(seconds), int64(nanos)).UTC(), true
}
