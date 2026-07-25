// Package savesidecar adapts the projected JSON emitted by the external
// palworld-save-reader "palsave" binary. The GPL decoder remains a separate
// process; this package owns the application-specific aggregation and derived
// progress calculations.
package savesidecar

import "time"

// Snapshot is the bounded set of per-player details decoded from one immutable
// Palworld backup generation.
type Snapshot struct {
	SnapshotAt time.Time
	Players    []Player
	Stats      Stats
}

// Player contains the fields available from the reader's player-details
// preset. Identity is private until saveroster projects it into the public ID
// space. The refactored reader no longer supplies names, levels, or guilds.
type Player struct {
	PlayerID string
	X        *float64
	Y        *float64

	LastSeenAt         *time.Time
	CaptureTotal       *int64
	UniquePalsCaptured *int
	PaldeckUnlocked    *int
}

// Stats records per-generation aggregation quality without retaining save
// contents, player names, or private identifiers.
type Stats struct {
	PlayerFiles      int
	DecodeFailures   int
	DuplicatePlayers int
}

type playerProjection struct {
	PlayerUID     string     `json:"PlayerUId"`
	LastTransform *transform `json:"LastTransform"`
	RecordData    *progress  `json:"RecordData"`
	LastOnline    *uint64    `json:"LastOnlineDateTime"`
}

type transform struct {
	Translation *translation `json:"Translation"`
}

type translation struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
}

type progress struct {
	TribeCaptureCount *int64       `json:"TribeCaptureCount"`
	PalCaptureCount   []countEntry `json:"PalCaptureCount"`
	PaldeckUnlockFlag []flagEntry  `json:"PaldeckUnlockFlag"`
}

type countEntry struct {
	Key   string `json:"Key"`
	Value int64  `json:"Value"`
}

type flagEntry struct {
	Key   string `json:"Key"`
	Value bool   `json:"Value"`
}

type preset struct {
	Name        string `json:"name"`
	GameVersion string `json:"gameVersion"`
	SaveType    string `json:"saveType"`
}
