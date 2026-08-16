package playerclaim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var testNow = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}

type incrementReader struct {
	mu   sync.Mutex
	next byte
}

func (r *incrementReader) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range buffer {
		buffer[index] = r.next
		r.next++
	}
	return len(buffer), nil
}

type fakeProver struct {
	mu sync.Mutex

	subjects   map[string]string
	prepareErr error
	verifyFn   func(context.Context, Prepared) error

	prepareCalls int
	verifyCalls  int
	lastTarget   string
	lastSelector uint64
}

type blockingPrepareProver struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (p *blockingPrepareProver) Prepare(_ context.Context, target string, _ uint64) (Prepared, error) {
	if p.calls.Add(1) == 1 {
		close(p.started)
		<-p.release
	}
	prepared := validPrepared("subject:" + target)
	prepared.PublicPlayerID = target
	return prepared, nil
}

func (*blockingPrepareProver) Verify(context.Context, *Prepared) error { return ErrPending }

func (p *fakeProver) Prepare(_ context.Context, target string, selector uint64) (Prepared, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	p.lastTarget = target
	p.lastSelector = selector
	if p.prepareErr != nil {
		return Prepared{}, p.prepareErr
	}
	subject := "subject:" + target
	if mapped, exists := p.subjects[target]; exists {
		subject = mapped
	}
	prepared := validPrepared(subject)
	prepared.PublicPlayerID = target
	return prepared, nil
}

func (p *fakeProver) Verify(ctx context.Context, prepared *Prepared) error {
	p.mu.Lock()
	p.verifyCalls++
	verifyFn := p.verifyFn
	p.mu.Unlock()
	if emptyInstructions(prepared.Instructions) {
		prepared.Instructions = testInstructions(ProofPhaseProve)
		return ErrReady
	}
	if verifyFn != nil {
		if err := verifyFn(ctx, *prepared); err != nil {
			return err
		}
	}
	if prepared.Instructions.Phase == ProofPhaseProve {
		prepared.Instructions = testInstructions(ProofPhaseRestore)
		return ErrReady
	}
	return nil
}

func (p *fakeProver) counts() (prepare, verify int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prepareCalls, p.verifyCalls
}

func validPrepared(subject string) Prepared {
	return Prepared{
		Subject: subject, PublicPlayerID: "player",
		Evidence: struct {
			Secret string
		}{Secret: "never disclose this evidence"},
	}
}

func testInstructions(phase ProofPhase) Instructions {
	pairs := []SlotPair{
		{SlotA: 2, SlotB: 7}, {SlotA: 2, SlotB: 8}, {SlotA: 2, SlotB: 9},
		{SlotA: 2, SlotB: 10}, {SlotA: 2, SlotB: 11}, {SlotA: 2, SlotB: 12}, {SlotA: 2, SlotB: 13},
	}
	step := 1
	if phase == ProofPhaseRestore {
		step = 2
		for left, right := 0, len(pairs)-1; left < right; left, right = left+1, right-1 {
			pairs[left], pairs[right] = pairs[right], pairs[left]
		}
	}
	return Instructions{
		Kind: InventorySwapSequence, Phase: phase, Step: step, TotalSteps: 2,
		Pairs: pairs, SnapshotAt: testNow.Add(-time.Minute),
	}
}

func newTestService(t *testing.T, prover Prover, clock *fakeClock, mutate func(*Options)) *Service {
	t.Helper()
	options := Options{
		Now:    clock.Now,
		Random: &incrementReader{},
	}
	if mutate != nil {
		mutate(&options)
	}
	service, err := NewService(prover, options)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestPrivateClaimBoundaryTypesMarshalWithoutEvidence(t *testing.T) {
	prepared := validPrepared("private-world-subject")
	prepared.PublicPlayerID = "public-player"
	prepared.Instructions = testInstructions(ProofPhaseProve)
	progress := PrivateProgress{
		SnapshotAt:     testNow,
		FastTravelKeys: []string{"private-fast-travel-key"},
		AreaKeys:       []string{"private-area-key"},
		NoteKeys:       []string{"private-note-key"},
		NormalBossKeys: []string{"private-normal-boss-key"},
		TowerBossKeys:  []string{"private-tower-boss-key"},
	}

	for name, value := range map[string]any{
		"prepared proof":   prepared,
		"private progress": progress,
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if got, want := string(encoded), `{}`; got != want {
				t.Fatalf("private boundary JSON = %s, want %s", got, want)
			}
		})
	}
}

func TestStartCreatesOpaque256BitChallenge(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProver{subjects: map[string]string{"public-player": "private-world-subject"}}
	service := newTestService(t, prover, clock, nil)

	challenge, err := service.Start(context.Background(), "public-player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	rawBearer, err := base64.RawURLEncoding.DecodeString(challenge.Bearer)
	if err != nil {
		t.Fatalf("challenge bearer is not base64url: %v", err)
	}
	if got, want := len(rawBearer), bearerBytes; got != want {
		t.Fatalf("challenge bearer bytes = %d, want %d", got, want)
	}
	if got, want := challenge.ExpiresAt, testNow.Add(ChallengePhaseTTL); !got.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", got, want)
	}
	if challenge.Status != VerificationArming || challenge.Instructions != nil {
		t.Fatalf("challenge = %+v, want unarmed challenge", challenge)
	}

	prover.mu.Lock()
	if got, want := prover.lastTarget, "public-player"; got != want {
		t.Errorf("Prepare target = %q, want %q", got, want)
	}
	if got, want := prover.lastSelector, uint64(0x0001020304050607); got != want {
		t.Errorf("Prepare selector = %#x, want %#x", got, want)
	}
	prover.mu.Unlock()

	encoded, err := json.Marshal(challenge)
	if err != nil {
		t.Fatalf("json.Marshal(challenge) error = %v", err)
	}
	for _, privateValue := range []string{"private-world-subject", "never disclose", "Evidence", "Subject"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Errorf("challenge JSON disclosed %q: %s", privateValue, encoded)
		}
	}
	if got := len(service.challenges); got != 1 {
		t.Fatalf("stored challenges = %d, want 1", got)
	}
	for _, entry := range service.challenges {
		if entry.prepared.Subject != "private-world-subject" {
			t.Fatalf("private stored subject = %q", entry.prepared.Subject)
		}
	}
}

func TestStartAllowsIndependentChallengesForSamePrivateSubject(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProver{subjects: map[string]string{
		"first-public-id":  "same-private-subject",
		"second-public-id": "same-private-subject",
	}}
	service := newTestService(t, prover, clock, nil)

	if _, err := service.Start(context.Background(), "first-public-id"); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if _, err := service.Start(context.Background(), "second-public-id"); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if got := len(service.challenges); got != 2 {
		t.Fatalf("stored challenges = %d, want 2", got)
	}
}

func TestVerifyPendingKeepsChallengeForRetry(t *testing.T) {
	clock := &fakeClock{now: testNow}
	var attempts atomic.Int32
	prover := &fakeProver{verifyFn: func(context.Context, Prepared) error {
		if attempts.Add(1) == 1 {
			return ErrPending
		}
		return nil
	}}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseProve)

	pending, err := service.Verify(context.Background(), challenge.Bearer)
	if err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}
	if pending.Status != VerificationPending || pending.Session != nil ||
		pending.Instructions == nil || pending.Instructions.Phase != ProofPhaseProve {
		t.Fatalf("first Verify() = %+v, want pending without session", pending)
	}

	restore, err := service.Verify(context.Background(), challenge.Bearer)
	if err != nil {
		t.Fatalf("third Verify() error = %v", err)
	}
	if restore.Status != VerificationReady || restore.Instructions == nil || restore.Instructions.Phase != ProofPhaseRestore {
		t.Fatalf("third Verify() = %+v, want restore instructions", restore)
	}
	verified, err := service.Verify(context.Background(), challenge.Bearer)
	if err != nil || verified.Status != VerificationVerified || verified.Session == nil {
		t.Fatalf("fourth Verify() = %+v, error %v; want verified session", verified, err)
	}
	if _, err := service.Verify(context.Background(), challenge.Bearer); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("fifth Verify() error = %v, want ErrChallengeNotFound", err)
	}
}

func TestPendingReplaysInstructionsAfterLostReadyResponse(t *testing.T) {
	for _, phase := range []ProofPhase{ProofPhaseProve, ProofPhaseRestore} {
		t.Run(string(phase), func(t *testing.T) {
			clock := &fakeClock{now: testNow}
			prover := &fakeProver{}
			service := newTestService(t, prover, clock, nil)
			challenge, err := service.Start(context.Background(), "player")
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			ready, err := service.Verify(context.Background(), challenge.Bearer)
			if err != nil || ready.Status != VerificationReady || ready.Instructions == nil {
				t.Fatalf("prove ready Verify() = %+v, error %v", ready, err)
			}
			if phase == ProofPhaseRestore {
				ready, err = service.Verify(context.Background(), challenge.Bearer)
				if err != nil || ready.Status != VerificationReady || ready.Instructions == nil {
					t.Fatalf("restore ready Verify() = %+v, error %v", ready, err)
				}
			}
			if ready.Instructions.Phase != phase {
				t.Fatalf("ready phase = %q, want %q", ready.Instructions.Phase, phase)
			}
			lostInstructions := *ready.Instructions

			prover.mu.Lock()
			prover.verifyFn = func(context.Context, Prepared) error { return ErrPending }
			prover.mu.Unlock()
			replayed, err := service.Verify(context.Background(), challenge.Bearer)
			if err != nil {
				t.Fatalf("replay Verify() error = %v", err)
			}
			if replayed.Status != VerificationPending || replayed.Instructions == nil ||
				!reflect.DeepEqual(*replayed.Instructions, lostInstructions) ||
				!replayed.ExpiresAt.Equal(ready.ExpiresAt) {
				t.Fatalf("replay Verify() = %+v, want pending with current instructions %+v", replayed, lostInstructions)
			}
		})
	}
}

func TestSuccessfulVerifyConsumesChallengeAndAuthenticatesPrivateSubject(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProver{subjects: map[string]string{"player": "private-subject"}}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	verification := completeChallenge(t, service, challenge.Bearer)
	if verification.Status != VerificationVerified || verification.Session == nil {
		t.Fatalf("Verify() = %+v", verification)
	}
	rawBearer, err := base64.RawURLEncoding.DecodeString(verification.Session.Bearer)
	if err != nil || len(rawBearer) != bearerBytes {
		t.Fatalf("session bearer decoded to %d bytes with error %v", len(rawBearer), err)
	}
	if verification.Session.Bearer == challenge.Bearer {
		t.Fatal("session and challenge bearers must differ")
	}
	if got, want := verification.Session.IdleExpiresAt, testNow.Add(DefaultSessionIdleTTL); !got.Equal(want) {
		t.Fatalf("idle expiry = %v, want %v", got, want)
	}
	if got, want := verification.Session.AbsoluteExpiresAt, testNow.Add(DefaultSessionAbsoluteTTL); !got.Equal(want) {
		t.Fatalf("absolute expiry = %v, want %v", got, want)
	}

	principal, err := service.ValidateSession(verification.Session.Bearer)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if got, want := principal.Subject(), "private-subject"; got != want {
		t.Fatalf("principal subject = %q, want %q", got, want)
	}
	encoded, err := json.Marshal(principal)
	if err != nil {
		t.Fatalf("json.Marshal(principal) error = %v", err)
	}
	if got, want := string(encoded), "{}"; got != want {
		t.Fatalf("principal JSON = %s, want %s", got, want)
	}
	encoded, err = json.Marshal(verification)
	if err != nil {
		t.Fatalf("json.Marshal(verification) error = %v", err)
	}
	if strings.Contains(string(encoded), "private-subject") || strings.Contains(string(encoded), verification.Session.Bearer) {
		t.Fatalf("verification JSON disclosed private data: %s", encoded)
	}
}

func TestConcurrentVerifyIssuesAtMostOneSession(t *testing.T) {
	clock := &fakeClock{now: testNow}
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	prover := &fakeProver{}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseProve)
	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseRestore)
	prover.mu.Lock()
	prover.verifyFn = func(context.Context, Prepared) error {
		startOnce.Do(func() { close(started) })
		<-release
		return nil
	}
	prover.mu.Unlock()

	type outcome struct {
		verification Verification
		err          error
	}
	first := make(chan outcome, 1)
	go func() {
		verification, verifyErr := service.Verify(context.Background(), challenge.Bearer)
		first <- outcome{verification: verification, err: verifyErr}
	}()
	<-started

	const competitors = 32
	results := make(chan outcome, competitors)
	var wait sync.WaitGroup
	for range competitors {
		wait.Add(1)
		go func() {
			defer wait.Done()
			verification, verifyErr := service.Verify(context.Background(), challenge.Bearer)
			results <- outcome{verification: verification, err: verifyErr}
		}()
	}
	wait.Wait()
	close(results)
	for result := range results {
		if !errors.Is(result.err, ErrVerificationInFlight) {
			t.Errorf("competing Verify() error = %v, want ErrVerificationInFlight", result.err)
		}
		if result.verification.Session != nil {
			t.Error("competing Verify() unexpectedly issued a session")
		}
	}
	close(release)
	winning := <-first
	if winning.err != nil || winning.verification.Status != VerificationVerified || winning.verification.Session == nil {
		t.Fatalf("winning Verify() = %+v, error %v", winning.verification, winning.err)
	}
	if _, verifyCalls := prover.counts(); verifyCalls != 3 {
		t.Fatalf("Prover.Verify calls = %d, want 3", verifyCalls)
	}
	if got := len(service.sessions); got != 1 {
		t.Fatalf("stored sessions = %d, want 1", got)
	}
}

func TestVerifyErrorReleasesChallengeForRetry(t *testing.T) {
	clock := &fakeClock{now: testNow}
	transient := errors.New("temporary decoder error")
	var attempts atomic.Int32
	prover := &fakeProver{verifyFn: func(context.Context, Prepared) error {
		if attempts.Add(1) == 1 {
			return transient
		}
		return nil
	}}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseProve)
	if _, err := service.Verify(context.Background(), challenge.Bearer); !errors.Is(err, transient) {
		t.Fatalf("first Verify() error = %v, want transient error", err)
	}
	verification, err := service.Verify(context.Background(), challenge.Bearer)
	if err != nil || verification.Status != VerificationReady || verification.Instructions == nil || verification.Instructions.Phase != ProofPhaseRestore {
		t.Fatalf("second Verify() = %+v, error %v; want restore", verification, err)
	}
	_ = completeRestore(t, service, challenge.Bearer)
}

func TestChallengeExpiresAtExactDeadline(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProver{}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	clock.Advance(ChallengePhaseTTL)

	if _, err := service.Verify(context.Background(), challenge.Bearer); !errors.Is(err, ErrChallengeExpired) {
		t.Fatalf("Verify() error = %v, want ErrChallengeExpired", err)
	}
	if _, verifyCalls := prover.counts(); verifyCalls != 0 {
		t.Fatalf("Prover.Verify calls = %d, want 0", verifyCalls)
	}
	if got := len(service.challenges); got != 0 {
		t.Fatalf("stored challenges = %d, want 0", got)
	}
}

func TestReadyTransitionsRefreshPhaseDeadline(t *testing.T) {
	clock := &fakeClock{now: testNow}
	service := newTestService(t, &fakeProver{}, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(80 * time.Minute)
	prove := assertReadyPhase(t, service, challenge.Bearer, ProofPhaseProve)
	if want := clock.Now().Add(ChallengePhaseTTL); !prove.ExpiresAt.Equal(want) {
		t.Fatalf("prove expiry = %v, want %v", prove.ExpiresAt, want)
	}
	clock.Advance(80 * time.Minute)
	restore := assertReadyPhase(t, service, challenge.Bearer, ProofPhaseRestore)
	if want := clock.Now().Add(ChallengePhaseTTL); !restore.ExpiresAt.Equal(want) {
		t.Fatalf("restore expiry = %v, want %v", restore.ExpiresAt, want)
	}
	clock.Advance(80 * time.Minute)
	_ = completeRestore(t, service, challenge.Bearer)
}

func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	t.Run("idle deadline refreshes", func(t *testing.T) {
		clock := &fakeClock{now: testNow}
		service := newTestService(t, &fakeProver{}, clock, func(options *Options) {
			options.SessionIdleTTL = 10 * time.Minute
			options.SessionAbsoluteTTL = time.Hour
		})
		sessionBearer := claimSession(t, service)

		clock.Advance(9 * time.Minute)
		principal, err := service.ValidateSession(sessionBearer)
		if err != nil {
			t.Fatalf("ValidateSession() before idle deadline error = %v", err)
		}
		if got, want := principal.IdleExpiresAt(), testNow.Add(19*time.Minute); !got.Equal(want) {
			t.Fatalf("refreshed idle deadline = %v, want %v", got, want)
		}
		clock.Advance(10 * time.Minute)
		if _, err := service.ValidateSession(sessionBearer); !errors.Is(err, ErrSessionExpired) {
			t.Fatalf("ValidateSession() at idle deadline error = %v, want ErrSessionExpired", err)
		}
	})

	t.Run("absolute deadline caps refresh", func(t *testing.T) {
		clock := &fakeClock{now: testNow}
		service := newTestService(t, &fakeProver{}, clock, func(options *Options) {
			options.SessionIdleTTL = 10 * time.Minute
			options.SessionAbsoluteTTL = 25 * time.Minute
		})
		sessionBearer := claimSession(t, service)

		clock.Advance(9 * time.Minute)
		if _, err := service.ValidateSession(sessionBearer); err != nil {
			t.Fatalf("first ValidateSession() error = %v", err)
		}
		clock.Advance(9 * time.Minute)
		principal, err := service.ValidateSession(sessionBearer)
		if err != nil {
			t.Fatalf("second ValidateSession() error = %v", err)
		}
		if got, want := principal.IdleExpiresAt(), testNow.Add(25*time.Minute); !got.Equal(want) {
			t.Fatalf("capped idle deadline = %v, want %v", got, want)
		}
		clock.Advance(7 * time.Minute)
		if _, err := service.ValidateSession(sessionBearer); !errors.Is(err, ErrSessionExpired) {
			t.Fatalf("ValidateSession() at absolute deadline error = %v, want ErrSessionExpired", err)
		}
	})
}

func TestRevokeSessionIsIdempotent(t *testing.T) {
	clock := &fakeClock{now: testNow}
	service := newTestService(t, &fakeProver{}, clock, nil)
	sessionBearer := claimSession(t, service)

	service.RevokeSession(sessionBearer)
	service.RevokeSession(sessionBearer)
	if _, err := service.ValidateSession(sessionBearer); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ValidateSession() error = %v, want ErrSessionNotFound", err)
	}
}

type fakeProgressProver struct {
	*fakeProver
	progress PrivateProgress
	subject  string
}

func (p *fakeProgressProver) Progress(_ context.Context, subject string) (PrivateProgress, error) {
	p.subject = subject
	return p.progress, nil
}

func TestProgressUsesAuthenticatedPrivateSubject(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProgressProver{
		fakeProver: &fakeProver{subjects: map[string]string{"player": "private-subject"}},
		progress:   PrivateProgress{SnapshotAt: testNow, FastTravelKeys: []string{"secret-key"}},
	}
	service := newTestService(t, prover, clock, nil)
	sessionBearer := claimSession(t, service)
	principal, err := service.ValidateSession(sessionBearer)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := service.Progress(context.Background(), principal)
	if err != nil || progress.FastTravelKeys[0] != "secret-key" || prover.subject != "private-subject" {
		t.Fatalf("Progress() = %+v, subject %q, error %v", progress, prover.subject, err)
	}
	if _, err := service.Progress(context.Background(), Principal{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Progress(empty principal) error = %v, want ErrUnavailable", err)
	}
}

func TestBoundedStoresCleanExpiredEntries(t *testing.T) {
	t.Run("challenges", func(t *testing.T) {
		clock := &fakeClock{now: testNow}
		prover := &fakeProver{}
		service := newTestService(t, prover, clock, func(options *Options) {
			options.MaxChallenges = 1
		})
		if _, err := service.Start(context.Background(), "first"); err != nil {
			t.Fatalf("first Start() error = %v", err)
		}
		if _, err := service.Start(context.Background(), "second"); !errors.Is(err, ErrStoreFull) {
			t.Fatalf("second Start() error = %v, want ErrStoreFull", err)
		}
		if prepare, _ := prover.counts(); prepare != 1 {
			t.Fatalf("full challenge store called Prover.Prepare %d times, want 1", prepare)
		}
		clock.Advance(ChallengePhaseTTL)
		if _, err := service.Start(context.Background(), "second"); err != nil {
			t.Fatalf("Start() after cleanup error = %v", err)
		}
		if got := len(service.challenges); got != 1 {
			t.Fatalf("stored challenges = %d, want 1", got)
		}
	})

	t.Run("in-flight challenge reservation", func(t *testing.T) {
		clock := &fakeClock{now: testNow}
		prover := &blockingPrepareProver{started: make(chan struct{}), release: make(chan struct{})}
		service := newTestService(t, prover, clock, func(options *Options) {
			options.MaxChallenges = 1
		})

		first := make(chan error, 1)
		go func() {
			_, err := service.Start(context.Background(), "first")
			first <- err
		}()
		<-prover.started
		if _, err := service.Start(context.Background(), "second"); !errors.Is(err, ErrStoreFull) {
			t.Fatalf("Start() during reserved preparation error = %v, want ErrStoreFull", err)
		}
		if calls := prover.calls.Load(); calls != 1 {
			t.Fatalf("reserved challenge called Prover.Prepare %d times, want 1", calls)
		}
		close(prover.release)
		if err := <-first; err != nil {
			t.Fatalf("reserved Start() error = %v", err)
		}
	})

	t.Run("sessions", func(t *testing.T) {
		clock := &fakeClock{now: testNow}
		service := newTestService(t, &fakeProver{}, clock, func(options *Options) {
			options.MaxSessions = 1
		})
		firstSession := claimSessionFor(t, service, "first")
		secondChallenge, err := service.Start(context.Background(), "second")
		if err != nil {
			t.Fatalf("second Start() error = %v", err)
		}
		assertReadyPhase(t, service, secondChallenge.Bearer, ProofPhaseProve)
		assertReadyPhase(t, service, secondChallenge.Bearer, ProofPhaseRestore)
		if _, err := service.Verify(context.Background(), secondChallenge.Bearer); !errors.Is(err, ErrStoreFull) {
			t.Fatalf("second Verify() error = %v, want ErrStoreFull", err)
		}
		service.RevokeSession(firstSession)
		verification, err := service.Verify(context.Background(), secondChallenge.Bearer)
		if err != nil || verification.Status != VerificationVerified {
			t.Fatalf("Verify() after capacity released = %+v, error %v", verification, err)
		}
	})
}

func TestCleanupRemovesExpiredEntries(t *testing.T) {
	clock := &fakeClock{now: testNow}
	service := newTestService(t, &fakeProver{}, clock, nil)
	challenge, err := service.Start(context.Background(), "unclaimed")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	_ = challenge
	claimSessionFor(t, service, "claimed")
	clock.Advance(DefaultSessionAbsoluteTTL)
	service.Cleanup()
	if len(service.challenges) != 0 || len(service.sessions) != 0 {
		t.Fatalf("stores after Cleanup: challenges=%d sessions=%d", len(service.challenges), len(service.sessions))
	}
}

func TestNewServiceAndPreparedValidation(t *testing.T) {
	if _, err := NewService(nil, Options{}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewService(nil) error = %v, want ErrInvalidConfiguration", err)
	}
	for name, options := range map[string]Options{
		"negative idle TTL":     {SessionIdleTTL: -time.Second},
		"negative absolute TTL": {SessionAbsoluteTTL: -time.Second},
		"negative challenges":   {MaxChallenges: -1},
		"negative sessions":     {MaxSessions: -1},
		"idle exceeds absolute": {SessionIdleTTL: time.Hour, SessionAbsoluteTTL: time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewService(&fakeProver{}, options); !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("NewService() error = %v, want ErrInvalidConfiguration", err)
			}
		})
	}

	for name, prepared := range map[string]Prepared{
		"empty subject": validPrepared(""),
		"mismatched public player": func() Prepared {
			value := validPrepared("subject")
			value.PublicPlayerID = "different-player"
			return value
		}(),
		"pre-armed proof": func() Prepared {
			value := validPrepared("subject")
			value.Instructions = testInstructions(ProofPhaseProve)
			return value
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			clock := &fakeClock{now: testNow}
			prover := &preparedProver{prepared: prepared}
			service := newTestService(t, prover, clock, nil)
			if _, err := service.Start(context.Background(), "player"); !errors.Is(err, ErrInvalidProof) {
				t.Fatalf("Start() error = %v, want ErrInvalidProof", err)
			}
			if len(service.challenges) != 0 {
				t.Fatal("malformed proof was stored")
			}
		})
	}
}

func TestRandomnessFailureDoesNotCallProverOrStoreChallenge(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &fakeProver{}
	service, err := NewService(prover, Options{Now: clock.Now, Random: errorReader{}})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if _, err := service.Start(context.Background(), "player"); !errors.Is(err, ErrRandomnessUnavailable) {
		t.Fatalf("Start() error = %v, want ErrRandomnessUnavailable", err)
	}
	if prepare, _ := prover.counts(); prepare != 0 {
		t.Fatalf("Prover.Prepare calls = %d, want 0", prepare)
	}
	if len(service.challenges) != 0 {
		t.Fatal("challenge stored after random failure")
	}
}

type preparedProver struct{ prepared Prepared }

func (p *preparedProver) Prepare(context.Context, string, uint64) (Prepared, error) {
	return p.prepared, nil
}

func (p *preparedProver) Verify(context.Context, *Prepared) error { return nil }

type scriptedProver struct {
	step   int
	verify func(int, *Prepared) error
}

func (p *scriptedProver) Prepare(_ context.Context, target string, _ uint64) (Prepared, error) {
	prepared := validPrepared("subject:" + target)
	prepared.PublicPlayerID = target
	return prepared, nil
}

func (p *scriptedProver) Verify(_ context.Context, prepared *Prepared) error {
	p.step++
	return p.verify(p.step, prepared)
}

func TestVerifyEnforcesBindingAndProofOrder(t *testing.T) {
	tests := []struct {
		name         string
		firstInvalid bool
		verify       func(int, *Prepared) error
	}{
		{
			name: "restore cannot be first", firstInvalid: true,
			verify: func(_ int, prepared *Prepared) error {
				prepared.Instructions = testInstructions(ProofPhaseRestore)
				return ErrReady
			},
		},
		{
			name: "restore cannot be skipped",
			verify: func(step int, prepared *Prepared) error {
				if step == 1 {
					prepared.Instructions = testInstructions(ProofPhaseProve)
					return ErrReady
				}
				return nil
			},
		},
		{
			name: "restore must exactly reverse proof",
			verify: func(step int, prepared *Prepared) error {
				if step == 1 {
					prepared.Instructions = testInstructions(ProofPhaseProve)
					return ErrReady
				}
				restore := testInstructions(ProofPhaseRestore)
				restore.Pairs[0], restore.Pairs[1] = restore.Pairs[1], restore.Pairs[0]
				prepared.Instructions = restore
				return ErrReady
			},
		},
		{
			name: "subject cannot change",
			verify: func(step int, prepared *Prepared) error {
				if step == 1 {
					prepared.Instructions = testInstructions(ProofPhaseProve)
					return ErrReady
				}
				prepared.Subject = "different-private-subject"
				prepared.Instructions = testInstructions(ProofPhaseRestore)
				return ErrReady
			},
		},
		{
			name: "public player cannot change",
			verify: func(step int, prepared *Prepared) error {
				if step == 1 {
					prepared.Instructions = testInstructions(ProofPhaseProve)
					return ErrReady
				}
				prepared.PublicPlayerID = "different-public-player"
				prepared.Instructions = testInstructions(ProofPhaseRestore)
				return ErrReady
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clock := &fakeClock{now: testNow}
			service := newTestService(t, &scriptedProver{verify: test.verify}, clock, nil)
			challenge, err := service.Start(context.Background(), "player")
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.Verify(context.Background(), challenge.Bearer)
			if test.firstInvalid {
				if !errors.Is(err, ErrInvalidProof) || first.Session != nil {
					t.Fatalf("first Verify() = %+v, error %v; want ErrInvalidProof", first, err)
				}
				return
			}
			if err != nil || first.Status != VerificationReady || first.Instructions == nil || first.Instructions.Phase != ProofPhaseProve {
				t.Fatalf("first Verify() = %+v, error %v; want prove instructions", first, err)
			}
			second, err := service.Verify(context.Background(), challenge.Bearer)
			if !errors.Is(err, ErrInvalidProof) || second.Session != nil {
				t.Fatalf("second Verify() = %+v, error %v; want ErrInvalidProof", second, err)
			}
		})
	}
}

func TestVerifyCannotMutatePinnedInstructionsThroughSharedPairSlice(t *testing.T) {
	clock := &fakeClock{now: testNow}
	prover := &scriptedProver{verify: func(step int, prepared *Prepared) error {
		switch step {
		case 1:
			prepared.Instructions = testInstructions(ProofPhaseProve)
			return ErrReady
		case 2:
			prepared.Instructions = testInstructions(ProofPhaseRestore)
			return ErrReady
		default:
			// Mutate the verifier's copy in place. Without a defensive slice copy,
			// this also rewrites the service's pinned restore instructions and makes
			// the final equality check compare the attacker-controlled slice to itself.
			for left, right := 0, len(prepared.Instructions.Pairs)-1; left < right; left, right = left+1, right-1 {
				prepared.Instructions.Pairs[left], prepared.Instructions.Pairs[right] =
					prepared.Instructions.Pairs[right], prepared.Instructions.Pairs[left]
			}
			return nil
		}
	}}
	service := newTestService(t, prover, clock, nil)
	challenge, err := service.Start(context.Background(), "player")
	if err != nil {
		t.Fatal(err)
	}
	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseProve)
	assertReadyPhase(t, service, challenge.Bearer, ProofPhaseRestore)
	verification, err := service.Verify(context.Background(), challenge.Bearer)
	if !errors.Is(err, ErrInvalidProof) || verification.Session != nil {
		t.Fatalf("Verify() = %+v, error %v; want ErrInvalidProof", verification, err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type knowledgeQuizProver struct{}

func TestValidQuizInstructionsAcceptsThreeToEightGroundedOptions(t *testing.T) {
	base := Instructions{
		Kind: InventoryQuiz, SnapshotAt: time.Now(),
		Questions: []QuizQuestion{
			{ID: "q1", Prompt: "First?", Options: []string{"A", "B", "C"}},
			{ID: "q2", Prompt: "Second?", Options: []string{"D", "E", "F"}},
		},
	}
	if !validQuizInstructions(base) {
		t.Fatal("three real options should be accepted")
	}
	base.Questions[0].Options = []string{"A", "B"}
	if validQuizInstructions(base) {
		t.Fatal("two options should not meet the guessing floor")
	}
	base.Questions[0].Options = []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"}
	if validQuizInstructions(base) {
		t.Fatal("more than eight options should exceed the disclosure bound")
	}
}

func (knowledgeQuizProver) Prepare(_ context.Context, target string, _ uint64) (Prepared, error) {
	return Prepared{
		Subject: "subject:" + target, PublicPlayerID: target,
		Instructions: Instructions{
			Kind: InventoryQuiz, SnapshotAt: time.Now().Add(-time.Minute),
			Questions: []QuizQuestion{
				{ID: "q1", Prompt: "First?", Options: []string{"A", "B", "C", "D", "E", "F", "G", "H"}},
				{ID: "q2", Prompt: "Second?", Options: []string{"I", "J", "K", "L", "M", "N", "O", "P"}},
			},
		},
		Evidence: "private answers",
	}, nil
}

func (knowledgeQuizProver) Verify(_ context.Context, prepared *Prepared) error {
	if len(prepared.Answers) != 2 || prepared.Answers[0] != (QuizAnswer{QuestionID: "q1", Option: 1}) ||
		prepared.Answers[1] != (QuizAnswer{QuestionID: "q2", Option: 3}) {
		return ErrIncorrectAnswer
	}
	return nil
}

func (knowledgeQuizProver) CycleQuestion(_ context.Context, prepared *Prepared, questionID string) error {
	if prepared == nil || questionID != "q1" {
		return ErrNoAlternateQuestion
	}
	prepared.Instructions.Questions = append([]QuizQuestion(nil), prepared.Instructions.Questions...)
	prepared.Instructions.Questions[0] = QuizQuestion{
		ID: "q4", Prompt: "Replacement?", Options: []string{"AA", "BB", "CC", "DD", "EE", "FF", "GG", "HH"},
	}
	return nil
}

func TestKnowledgeQuizIsReadyImmediatelyAndConsumesIncorrectAttempt(t *testing.T) {
	service, err := NewService(knowledgeQuizProver{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	incorrect, err := service.Start(context.Background(), "offline-player")
	if err != nil || incorrect.Status != VerificationReady || incorrect.Instructions == nil || incorrect.Instructions.Kind != InventoryQuiz {
		t.Fatalf("Start() = %+v, %v", incorrect, err)
	}
	if _, err := service.Verify(context.Background(), incorrect.Bearer,
		QuizAnswer{QuestionID: "q1", Option: 0}, QuizAnswer{QuestionID: "q2", Option: 3}); !errors.Is(err, ErrIncorrectAnswer) {
		t.Fatalf("incorrect Verify() error = %v", err)
	}
	if _, err := service.Verify(context.Background(), incorrect.Bearer); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("consumed Verify() error = %v", err)
	}

	correct, err := service.Start(context.Background(), "offline-player")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Verify(context.Background(), correct.Bearer,
		QuizAnswer{QuestionID: "q1", Option: 1}, QuizAnswer{QuestionID: "q2", Option: 3})
	if err != nil || verified.Status != VerificationVerified || verified.Session == nil {
		t.Fatalf("correct Verify() = %+v, %v", verified, err)
	}
}

func TestKnowledgeQuizCyclesOnlyRequestedQuestion(t *testing.T) {
	service, err := NewService(knowledgeQuizProver{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := service.Start(context.Background(), "offline-player")
	if err != nil || challenge.Instructions == nil {
		t.Fatalf("Start() = %+v, %v", challenge, err)
	}
	q2 := challenge.Instructions.Questions[1]
	cycled, err := service.CycleQuestion(context.Background(), challenge.Bearer, "q1")
	if err != nil || cycled.Status != VerificationReady || cycled.Instructions == nil {
		t.Fatalf("CycleQuestion() = %+v, %v", cycled, err)
	}
	if cycled.Instructions.Questions[0].ID != "q4" ||
		cycled.Instructions.Questions[1].ID != q2.ID {
		t.Fatalf("cycled questions = %+v", cycled.Instructions.Questions)
	}
	if _, err := service.CycleQuestion(context.Background(), challenge.Bearer, "q4"); !errors.Is(err, ErrNoAlternateQuestion) {
		t.Fatalf("second CycleQuestion() error = %v", err)
	}
}

func claimSession(t *testing.T, service *Service) string {
	t.Helper()
	return claimSessionFor(t, service, "player")
}

func claimSessionFor(t *testing.T, service *Service, player string) string {
	t.Helper()
	challenge, err := service.Start(context.Background(), player)
	if err != nil {
		t.Fatalf("Start(%q) error = %v", player, err)
	}
	verification := completeChallenge(t, service, challenge.Bearer)
	if verification.Session == nil {
		t.Fatalf("Verify(%q) did not issue a session", player)
	}
	return verification.Session.Bearer
}

func assertReadyPhase(t *testing.T, service *Service, bearer string, phase ProofPhase) Verification {
	t.Helper()
	verification, err := service.Verify(context.Background(), bearer)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verification.Status != VerificationReady || verification.Instructions == nil || verification.Instructions.Phase != phase {
		t.Fatalf("Verify() = %+v, want ready %s instructions", verification, phase)
	}
	return verification
}

func completeRestore(t *testing.T, service *Service, bearer string) Verification {
	t.Helper()
	verification, err := service.Verify(context.Background(), bearer)
	if err != nil || verification.Status != VerificationVerified || verification.Session == nil {
		t.Fatalf("Verify() = %+v, error %v; want verified session", verification, err)
	}
	return verification
}

func completeChallenge(t *testing.T, service *Service, bearer string) Verification {
	t.Helper()
	assertReadyPhase(t, service, bearer, ProofPhaseProve)
	assertReadyPhase(t, service, bearer, ProofPhaseRestore)
	return completeRestore(t, service, bearer)
}
