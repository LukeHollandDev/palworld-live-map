// Package savesidecar adapts the projected JSON emitted by the external
// palworld-save-reader binary.
//
// The decoder is invoked rather than imported. This package owns the
// application-specific aggregation and derived progress calculations on its
// output.
//
// Two decoder calls make up the contract. The player-details preset runs once
// per player.sav and yields capture and Paldeck progress. A single compact
// roster pass over the generation yields identity, Arena RP, exploration, and
// clear counters without resolving owned collections; without its Level.sav
// identity data every save record is anonymous and the poller cannot publish
// offline players.
package savesidecar

import "time"

// Snapshot is the bounded set of per-player details decoded from one immutable
// Palworld backup generation.
type Snapshot struct {
	SnapshotAt time.Time
	Players    []Player
	Stats      Stats
}

// ClaimPlayer is the deliberately narrow private projection used to prove
// control of one character. It is never part of a public snapshot. Stack item
// identifiers and Pal instance identifiers are save-authored secrets and must
// remain server-side.
type ClaimPlayer struct {
	PlayerID  string        `json:"-"`
	Common    []ClaimStack  `json:"-"`
	DropSlot  []ClaimStack  `json:"-"`
	Essential []ClaimStack  `json:"-"`
	Weapons   []ClaimStack  `json:"-"`
	Armor     []ClaimStack  `json:"-"`
	Food      []ClaimStack  `json:"-"`
	Party     []ClaimPal    `json:"-"`
	Progress  ClaimProgress `json:"-"`
}

// ClaimProgress contains exact, private state keys used only after a character
// has been proven. Available is true only when every domain was decoded and
// validated; non-nil empty slices then mean authoritative zero completion.
type ClaimProgress struct {
	Available    bool     `json:"-"`
	FastTravel   []string `json:"-"`
	Areas        []string `json:"-"`
	Notes        []string `json:"-"`
	Relics       []string `json:"-"`
	ItemPickups  []string `json:"-"`
	NormalBosses []string `json:"-"`
	TowerBosses  []string `json:"-"`
}

// ClaimStack identifies one occupied private inventory-container slot. Common
// stacks also retain enough information for the ordered proof and restore.
type ClaimStack struct {
	Slot          uint32 `json:"-"`
	ItemID        string `json:"-"`
	Count         uint32 `json:"-"`
	DynamicItemID string `json:"-"`
}

// ClaimPal identifies one occupied party slot retained by the narrow private
// resolver. Species is used only as a knowledge-question candidate.
type ClaimPal struct {
	Slot       int32  `json:"-"`
	InstanceID string `json:"-"`
	Species    string `json:"-"`
}

// Player combines both decoder calls for one save record. Identity is private
// until saveroster projects it into the public ID space: PlayerID and GuildID
// are raw save GUIDs and must never reach a public snapshot unprojected.
//
// Name, Level, guild, ArenaRankPoints, and the exploration/clear counters come
// from the roster pass and are empty when it fails or omits the record. Position,
// capture, Paldeck, and last-seen fields come from the player-details preset.
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
	ArenaRankPoints    *int
	FastTravelUnlocked *int
	AreasDiscovered    *int
	BossDefeats        *int
	TowerDefeats       *int
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
	// data rather than discarding it. ResolveError remains inside this private
	// decoder boundary for diagnosis and tests; callers must collapse it to a
	// stable category because it can contain paths or save-authored text.
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
	PlayerUID          string             `json:"playerUId"`
	Character          *resolvedCharacter `json:"character"`
	Guild              *resolvedGuild     `json:"guild"`
	FastTravelUnlocked *int               `json:"fastTravelUnlocked"`
	AreasDiscovered    *int               `json:"areasDiscovered"`
	BossDefeats        *int               `json:"bossDefeats"`
	TowerDefeats       *int               `json:"towerDefeats"`
}

type resolvedCharacter struct {
	Nickname        string `json:"nickname"`
	Level           int    `json:"level"`
	ArenaRankPoints *int   `json:"arenaRankPoints"`
}

type resolvedGuild struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type resolvedPlayerDocument struct {
	ResolveVersion int                  `json:"resolveVersion"`
	Kind           string               `json:"kind"`
	Player         *resolvedClaimPlayer `json:"player"`
}

type resolvedClaimPlayer struct {
	PlayerUID string                 `json:"playerUId"`
	Inventory resolvedClaimInventory `json:"inventory"`
	Pals      []resolvedClaimPal     `json:"pals"`
	Warnings  []string               `json:"warnings"`
	Progress  *resolvedClaimProgress `json:"progress"`
}

type resolvedClaimProgress struct {
	FastTravel   []string `json:"fastTravel"`
	Areas        []string `json:"areas"`
	Notes        []string `json:"notes"`
	Relics       []string `json:"relics"`
	ItemPickups  []string `json:"itemPickups"`
	NormalBosses []string `json:"normalBosses"`
	TowerBosses  []string `json:"towerBosses"`
}

type resolvedClaimInventory struct {
	Common    []resolvedClaimStack `json:"common"`
	DropSlot  []resolvedClaimStack `json:"dropSlot"`
	Essential []resolvedClaimStack `json:"essential"`
	Weapons   []resolvedClaimStack `json:"weapons"`
	Armor     []resolvedClaimStack `json:"armor"`
	Food      []resolvedClaimStack `json:"food"`
}

type resolvedClaimStack struct {
	Slot          uint32  `json:"slot"`
	ItemID        string  `json:"itemId"`
	Count         uint32  `json:"count"`
	DynamicItemID *string `json:"dynamicItemId"`
}

type resolvedClaimPal struct {
	InstanceID string `json:"instanceId"`
	Species    string `json:"species"`
	Location   string `json:"location"`
	Slot       int32  `json:"slot"`
}
