// Package playerclaim defines the private boundary between save-backed proof
// collection and the HTTP claim/session service. Implementations must never
// expose Prepared.Subject or Prepared.Evidence to a browser or a log.
package playerclaim

import (
	"context"
	"errors"
	"time"
)

type ProofKind string

const (
	InventorySwapSequence ProofKind = "inventory_swap_sequence"
)

type ProofPhase string

const (
	ProofPhaseProve   ProofPhase = "prove"
	ProofPhaseRestore ProofPhase = "restore"
)

var (
	// ErrUnavailable indicates that the save-backed proof source cannot safely
	// prepare or verify a claim at the moment.
	ErrUnavailable = errors.New("player claim proof is unavailable")
	// ErrPending is returned by a Prover when no post-challenge immutable save
	// generation is available yet. Service.Verify translates it into the public
	// Pending status rather than returning it to the caller as an error.
	ErrPending = errors.New("player claim proof is pending")
	// ErrReady means a post-start immutable baseline is armed and the updated
	// Prepared instructions may now be revealed to the claimant.
	ErrReady = errors.New("player claim proof is ready")
)

// SlotPair is one reversible in-game swap. Slots are one-based to match the
// visible inventory grid.
type SlotPair struct {
	SlotA int `json:"slotA"`
	SlotB int `json:"slotB"`
}

// Instructions are the only save-derived proof details that may be returned
// to a claimant. A proof uses seven nonce-selected anchored swaps followed by
// a separately observed reverse-order restore phase. Item identifiers, counts,
// instance IDs, and the private save PlayerUID remain in Evidence on the server.
type Instructions struct {
	Kind       ProofKind  `json:"kind"`
	Phase      ProofPhase `json:"phase"`
	Step       int        `json:"step"`
	TotalSteps int        `json:"totalSteps"`
	Pairs      []SlotPair `json:"pairs"`
	SnapshotAt time.Time  `json:"snapshotAt"`
}

// Prepared is an opaque, server-only proof. Subject is a stable world-scoped
// HMAC and Evidence is implementation-private baseline state.
type Prepared struct {
	Subject        string       `json:"-"`
	PublicPlayerID string       `json:"-"`
	Instructions   Instructions `json:"-"`
	Evidence       any          `json:"-"`
}

// Prover prepares a reversible, nonce-selected state transition and later
// verifies it against a newly published immutable save generation.
type Prover interface {
	Prepare(context.Context, string, uint64) (Prepared, error)
	Verify(context.Context, *Prepared) error
}

// PrivateProgress is exact save evidence for the authenticated subject only.
// Its keys are intentionally excluded from JSON; the HTTP layer must project
// them to safe catalogue location IDs before returning a response.
type PrivateProgress struct {
	SnapshotAt     time.Time `json:"-"`
	FastTravelKeys []string  `json:"-"`
	AreaKeys       []string  `json:"-"`
	NoteKeys       []string  `json:"-"`
	NormalBossKeys []string  `json:"-"`
	TowerBossKeys  []string  `json:"-"`
}

type ProgressSource interface {
	Progress(context.Context, string) (PrivateProgress, error)
}
