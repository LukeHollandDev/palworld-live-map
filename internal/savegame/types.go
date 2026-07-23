// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import "time"

const (
	defaultMaxSaveBytes int64 = 512 << 20
	hardMaxSaveBytes    int64 = 2 << 30
	defaultMaxPlayers         = 10_000
	hardMaxPlayers            = 100_000
	maxSkippedDetails         = 20
)

// Options configures a read-only save reader.
type Options struct {
	// MaxSaveBytes bounds both compressed files and their declared decompressed
	// sizes. Zero uses 512 MiB; values above 2 GiB are rejected.
	MaxSaveBytes int64
	// MaxPlayers bounds player character entries and individual player saves.
	// Zero uses 10,000; values above 100,000 are rejected.
	MaxPlayers int
}

// Snapshot is the typed, bounded view extracted from one immutable save copy.
type Snapshot struct {
	SnapshotAt time.Time `json:"snapshotAt"`
	Players    []Player  `json:"players"`
	Stats      Stats     `json:"stats"`
}

// Player contains only fields needed by the live map. PlayerID is Palworld's
// internal player GUID, not an EOS/Steam account identifier.
type Player struct {
	PlayerID    string     `json:"playerId"`
	DisplayName string     `json:"displayName"`
	Level       int32      `json:"level"`
	GuildID     string     `json:"guildId,omitempty"`
	GuildName   string     `json:"guildName,omitempty"`
	X           *float64   `json:"x"`
	Y           *float64   `json:"y"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`
	// Progress fields come from the per-player SaveData.RecordData block.
	// Pointers distinguish an authoritative zero from an unavailable field.
	CaptureTotal       *int64 `json:"captureTotal,omitempty"`
	UniquePalsCaptured *int   `json:"uniquePalsCaptured,omitempty"`
	PaldeckUnlocked    *int   `json:"paldeckUnlocked,omitempty"`
}

// Vector is an Unreal world-space position in centimetres.
type Vector struct {
	X float64
	Y float64
	Z float64
}

// Stats makes tolerated save-format drift observable without exposing save
// contents, player names, or identifiers in diagnostics.
type Stats struct {
	SkippedProperties int            `json:"skippedProperties"`
	SkippedStructs    int            `json:"skippedStructs"`
	DecodeFailures    map[string]int `json:"decodeFailures,omitempty"`
	SkippedDetails    []string       `json:"skippedDetails,omitempty"`
	PlayerFiles       int            `json:"playerFiles"`
	DuplicatePlayers  int            `json:"duplicatePlayers"`
	GuildConflicts    int            `json:"guildConflicts"`
	propertyCount     uint64
	decodedNodes      uint64
	decodedBytes      uint64
}

func newStats() Stats { return Stats{DecodeFailures: make(map[string]int)} }

func (s *Stats) recordSkip(path, typ string) {
	if s == nil {
		return
	}
	s.SkippedProperties++
	if len(s.SkippedDetails) < maxSkippedDetails {
		s.SkippedDetails = append(s.SkippedDetails, path+" ("+typ+")")
	}
}
