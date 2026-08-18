package playerclaim

import (
	"context"
	"errors"
	"testing"
	"time"
)

type testProver struct {
	prepareErr error
	verifyErr  error
	cycleErr   error
}

func testQuiz(id string) Instructions {
	return Instructions{Kind: InventoryQuiz, SnapshotAt: time.Unix(100, 0), Questions: []QuizQuestion{{
		ID: id, Prompt: "What was equipped?", Options: []string{"A", "B", "C"}, CanCycle: true,
	}}}
}

func (p *testProver) Prepare(_ context.Context, playerID string, _ uint64) (Prepared, error) {
	if p.prepareErr != nil {
		return Prepared{}, p.prepareErr
	}
	return Prepared{Subject: "private-subject", PublicPlayerID: playerID, Instructions: testQuiz("q1")}, nil
}

func (p *testProver) Verify(_ context.Context, prepared *Prepared) error {
	if p.verifyErr != nil {
		return p.verifyErr
	}
	if len(prepared.Answers) != 1 || prepared.Answers[0].QuestionID != "q1" || prepared.Answers[0].Option != 1 {
		return ErrIncorrectAnswer
	}
	return nil
}

func (p *testProver) CycleQuestion(_ context.Context, prepared *Prepared, questionID string) error {
	if p.cycleErr != nil {
		return p.cycleErr
	}
	if questionID != "q1" {
		return ErrNoAlternateQuestion
	}
	prepared.Instructions = testQuiz("q2")
	return nil
}

func TestQuestionClaimIssuesAndValidatesEphemeralSession(t *testing.T) {
	service, err := NewService(&testProver{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := service.Start(context.Background(), "player-public")
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Status != VerificationReady || challenge.Instructions == nil || len(challenge.Instructions.Questions) != 1 {
		t.Fatalf("challenge = %+v", challenge)
	}
	verification, err := service.Verify(context.Background(), challenge.Bearer, QuizAnswer{QuestionID: "q1", Option: 1})
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != VerificationVerified || verification.Session == nil || verification.Session.Bearer == "" {
		t.Fatalf("verification = %+v", verification)
	}
	principal, err := service.ValidateSession(verification.Session.Bearer)
	if err != nil || principal.PublicPlayerID() != "player-public" || principal.Subject() != "private-subject" {
		t.Fatalf("principal = %+v, %v", principal, err)
	}
	service.RevokeSession(verification.Session.Bearer)
	if _, err := service.ValidateSession(verification.Session.Bearer); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ValidateSession() = %v", err)
	}
}

func TestIncorrectAnswerConsumesChallenge(t *testing.T) {
	service, _ := NewService(&testProver{}, Options{})
	challenge, _ := service.Start(context.Background(), "player-public")
	_, err := service.Verify(context.Background(), challenge.Bearer, QuizAnswer{QuestionID: "q1", Option: 0})
	if !errors.Is(err, ErrIncorrectAnswer) {
		t.Fatalf("Verify() = %v", err)
	}
	if _, err := service.Verify(context.Background(), challenge.Bearer); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("second Verify() = %v", err)
	}
}

func TestCycleQuestionReplacesOnlyQuestion(t *testing.T) {
	service, _ := NewService(&testProver{}, Options{})
	challenge, _ := service.Start(context.Background(), "player-public")
	result, err := service.CycleQuestion(context.Background(), challenge.Bearer, "q1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Instructions == nil || result.Instructions.Questions[0].ID != "q2" {
		t.Fatalf("result = %+v", result)
	}
}

func TestStartPropagatesNoSuitableQuestion(t *testing.T) {
	service, _ := NewService(&testProver{prepareErr: ErrNoSuitableQuestion}, Options{})
	_, err := service.Start(context.Background(), "player-public")
	if !errors.Is(err, ErrNoSuitableQuestion) {
		t.Fatalf("Start() = %v", err)
	}
}

func TestPreparedProofMustBeOneQuestion(t *testing.T) {
	if validQuizInstructions(Instructions{}) {
		t.Fatal("empty instructions accepted")
	}
	instructions := testQuiz("q1")
	instructions.Questions[0].Options = []string{"A", "B"}
	if validQuizInstructions(instructions) {
		t.Fatal("two-option question accepted")
	}
}
