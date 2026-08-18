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
	InventoryQuiz ProofKind = "inventory_quiz"
)

var (
	// ErrUnavailable indicates that the save-backed proof source cannot safely
	// prepare or verify a claim at the moment.
	ErrUnavailable = errors.New("player claim proof is unavailable")
	// ErrIncorrectAnswer means a one-shot knowledge challenge did not match its
	// private save evidence. Callers must consume the challenge on this result.
	ErrIncorrectAnswer = errors.New("player claim answer is incorrect")
	// ErrNoSuitableQuestion means the save was readable but did not contain
	// enough distinct private facts to construct a useful multiple-choice check.
	ErrNoSuitableQuestion = errors.New("player claim has no suitable question")
	// ErrNoAlternateQuestion means an otherwise valid knowledge challenge has
	// exhausted its private question pool.
	ErrNoAlternateQuestion = errors.New("player claim has no alternate question")
)

type QuizQuestion struct {
	ID       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Options  []string `json:"options"`
	CanCycle bool     `json:"canCycle"`
}

type QuizAnswer struct {
	QuestionID string `json:"questionId"`
	Option     int    `json:"option"`
}

// Instructions contain the single safe question shown to the claimant. Item
// identifiers, counts, instance IDs, and the private save PlayerUID remain in
// Evidence on the server.
type Instructions struct {
	Kind       ProofKind      `json:"kind"`
	Questions  []QuizQuestion `json:"questions"`
	SnapshotAt time.Time      `json:"snapshotAt"`
}

// Prepared is an opaque, server-only proof. Subject is a process-scoped,
// world-bound HMAC and Evidence is implementation-private baseline state.
type Prepared struct {
	Subject        string       `json:"-"`
	PublicPlayerID string       `json:"-"`
	Instructions   Instructions `json:"-"`
	Answers        []QuizAnswer `json:"-"`
	Evidence       any          `json:"-"`
}

// Prover prepares one knowledge question and verifies a one-shot answer.
type Prover interface {
	Prepare(context.Context, string, uint64) (Prepared, error)
	Verify(context.Context, *Prepared) error
}

// QuestionCycler optionally replaces the current knowledge question.
// Implementations must clone private evidence before mutation because Service
// retains the previous Prepared value until the replacement has been validated.
type QuestionCycler interface {
	CycleQuestion(context.Context, *Prepared, string) error
}

// PrivateProgress is exact save evidence for the authenticated subject only.
// Its keys are intentionally excluded from JSON; the HTTP layer must project
// them to safe catalogue location IDs before returning a response.
type PrivateProgress struct {
	SnapshotAt     time.Time `json:"-"`
	FastTravelKeys []string  `json:"-"`
	AreaKeys       []string  `json:"-"`
	NoteKeys       []string  `json:"-"`
	RelicKeys      []string  `json:"-"`
	ItemPickupKeys []string  `json:"-"`
	NormalBossKeys []string  `json:"-"`
	TowerBossKeys  []string  `json:"-"`
}

type ProgressSource interface {
	Progress(context.Context, string) (PrivateProgress, error)
}
