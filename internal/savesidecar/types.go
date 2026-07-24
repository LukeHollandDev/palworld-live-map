// Package savesidecar decodes the roster JSON emitted by the external
// palworld-save-reader "savedecode" binary. The decoder is a separate,
// GPL-licensed static binary the server execs; the server never links it.
// Keeping it at arm's length over a subprocess boundary is aggregation, not
// linking, so it does not relicense this MIT server.
//
// The types below mirror the decoder's stdout contract (savegame.Snapshot).
// They are deliberately first-party so this package carries no dependency on
// the decoder's Go module.
package savesidecar

import "time"

// Snapshot mirrors the decoder's roster JSON document.
type Snapshot struct {
	SnapshotAt time.Time `json:"snapshotAt"`
	Players    []Player  `json:"players"`
	Stats      Stats     `json:"stats"`
}

// Player mirrors one entry of the decoder's roster JSON. Pointer fields are
// nil when the save did not carry that datum; the projection layer treats
// nil as "unknown".
type Player struct {
	PlayerID    string     `json:"playerId"`
	DisplayName string     `json:"displayName"`
	Level       int32      `json:"level"`
	GuildID     string     `json:"guildId,omitempty"`
	GuildName   string     `json:"guildName,omitempty"`
	X           *float64   `json:"x"`
	Y           *float64   `json:"y"`
	LastSeenAt  *time.Time `json:"lastSeenAt"`

	CaptureTotal       *int64 `json:"captureTotal,omitempty"`
	UniquePalsCaptured *int   `json:"uniquePalsCaptured,omitempty"`
	PaldeckUnlocked    *int   `json:"paldeckUnlocked,omitempty"`
}

// Stats mirrors the decoder's decode-quality counters. It is retained for
// observability; the projection layer does not depend on it.
type Stats struct {
	SkippedProperties int            `json:"skippedProperties"`
	SkippedStructs    int            `json:"skippedStructs"`
	DecodeFailures    map[string]int `json:"decodeFailures,omitempty"`
	PlayerFiles       int            `json:"playerFiles"`
	DuplicatePlayers  int            `json:"duplicatePlayers"`
	GuildConflicts    int            `json:"guildConflicts"`
}
