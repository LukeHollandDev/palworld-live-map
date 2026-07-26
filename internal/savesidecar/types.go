// Package savesidecar adapts the projected JSON emitted by the external
// palworld-save-reader binary.
//
// The decoder is invoked rather than imported. This package owns the
// application-specific aggregation and derived progress calculations on its
// output.
//
// Two decoder calls make up the contract. The player-details preset runs once
// per player.sav and yields progress counters. A single compact roster pass over the
// generation yields the display name, level, and guild that live in Level.sav
// and appear in no preset; without it every save record is anonymous and the
// poller cannot publish offline players.
package savesidecar

import "time"

// Snapshot is the bounded set of per-player details decoded from one immutable
// Palworld backup generation.
type Snapshot struct {
	SnapshotAt time.Time
	Players    []Player
	Stats      Stats
}

// Player combines both decoder calls for one save record. Identity is private
// until saveroster projects it into the public ID space: PlayerID and GuildID
// are raw save GUIDs and must never reach a public snapshot unprojected.
//
// Name, Level, and the guild fields come from the roster pass and are empty
// when it fails or omits the record; the remaining fields come from the
// player-details preset.
type Player struct {
	PlayerID string
	X        *float64
	Y        *float64

	Name      string
	Level     int
	GuildID   string
	GuildName string

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
	RosterRecords    int
	UnnamedPlayers   int
	// NamesResolved counts records the roster pass named. ResolveFailed marks
	// the pass itself failing, which downgrades the generation to preset-only
	// data rather than discarding it. ResolveError retains the private
	// diagnostic for server-side logging; it must never be sent to browsers
	// because decoder errors can contain paths or save-authored text.
	NamesResolved int
	ResolveFailed bool
	ResolveError  error
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

// resolveDocument is the compact `--resolve roster` output. Its version is
// checked before use so an incompatible reader cannot silently mislabel data.
type resolveDocument struct {
	ResolveVersion int              `json:"resolveVersion"`
	Kind           string           `json:"kind"`
	Roster         []resolvedPlayer `json:"roster"`
}

type resolvedPlayer struct {
	PlayerUID string             `json:"playerUId"`
	Character *resolvedCharacter `json:"character"`
	Guild     *resolvedGuild     `json:"guild"`
}

type resolvedCharacter struct {
	Nickname string `json:"nickname"`
	Level    int    `json:"level"`
}

type resolvedGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
