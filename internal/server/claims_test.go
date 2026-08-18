package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
)

type claimTestProver struct{ prepareErr error }

func claimTestInstructions(id string) playerclaim.Instructions {
	return playerclaim.Instructions{Kind: playerclaim.InventoryQuiz, SnapshotAt: time.Unix(100, 0), Questions: []playerclaim.QuizQuestion{{
		ID: id, Prompt: "What was equipped?", Options: []string{"A", "B", "C"}, CanCycle: true,
	}}}
}

func (p *claimTestProver) Prepare(_ context.Context, playerID string, _ uint64) (playerclaim.Prepared, error) {
	if p.prepareErr != nil {
		return playerclaim.Prepared{}, p.prepareErr
	}
	return playerclaim.Prepared{Subject: "private-subject", PublicPlayerID: playerID, Instructions: claimTestInstructions("q1")}, nil
}

func (*claimTestProver) Verify(_ context.Context, prepared *playerclaim.Prepared) error {
	if len(prepared.Answers) != 1 || prepared.Answers[0].QuestionID != "q1" || prepared.Answers[0].Option != 1 {
		return playerclaim.ErrIncorrectAnswer
	}
	return nil
}

func (*claimTestProver) CycleQuestion(_ context.Context, prepared *playerclaim.Prepared, questionID string) error {
	if questionID != "q1" {
		return playerclaim.ErrNoAlternateQuestion
	}
	prepared.Instructions = claimTestInstructions("q2")
	return nil
}

func (*claimTestProver) Progress(context.Context, string) (playerclaim.PrivateProgress, error) {
	return playerclaim.PrivateProgress{SnapshotAt: time.Unix(200, 0)}, nil
}

type claimTestSource struct{ players []palworld.Player }

func (s claimTestSource) Snapshot() palworld.Snapshot { return palworld.Snapshot{Players: s.players} }
func (s claimTestSource) PlayerSnapshotSince(uint64) (palworld.PlayerSnapshot, uint64, bool) {
	return palworld.PlayerSnapshot{Players: s.players}, 1, true
}
func (claimTestSource) ObjectSnapshotSince(uint64) (palworld.ObjectSnapshot, uint64, bool) {
	return palworld.ObjectSnapshot{}, 1, true
}

func newQuestionClaimServer(t *testing.T, prover *claimTestProver) *Server {
	t.Helper()
	if prover == nil {
		prover = &claimTestProver{}
	}
	service, err := playerclaim.NewService(prover, playerclaim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.PlayerClaimsEnabled = true
	source := claimTestSource{players: []palworld.Player{{ID: "online", Name: "Online", Online: true}, {ID: "offline", Name: "Offline", Online: false}}}
	server, err := NewWithClaims(cfg, source, service)
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func claimRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func serveClaim(server *Server, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func startQuestionClaim(t *testing.T, server *Server, playerID string) string {
	t.Helper()
	response := serveClaim(server, claimRequest(http.MethodPost, "/api/player-claims", `{"playerId":"`+playerID+`"}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		ChallengeToken string `json:"challengeToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.ChallengeToken
}

func TestQuestionClaimNeedsOnlyEnabledFlagAndWorksForOfflinePlayer(t *testing.T) {
	server := newQuestionClaimServer(t, nil)
	for _, playerID := range []string{"online", "offline"} {
		t.Run(playerID, func(t *testing.T) {
			request := claimRequest(http.MethodPost, "/api/player-claims", `{"playerId":"`+playerID+`"}`)
			request.Header.Set("Origin", "http://unrelated.example")
			response := serveClaim(server, request)
			if response.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if response.Header().Get("Set-Cookie") != "" {
				t.Fatalf("unexpected cookie: %s", response.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestNoSuitableQuestionHasDistinctActionableCode(t *testing.T) {
	server := newQuestionClaimServer(t, &claimTestProver{prepareErr: playerclaim.ErrNoSuitableQuestion})
	response := serveClaim(server, claimRequest(http.MethodPost, "/api/player-claims", `{"playerId":"offline"}`))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "no_suitable_question") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestVerifiedSessionUsesBearerHeaderAndNoCookie(t *testing.T) {
	server := newQuestionClaimServer(t, nil)
	token := startQuestionClaim(t, server, "offline")
	body := `{"challengeToken":"` + token + `","answers":[{"questionId":"q1","option":1}]}`
	verified := serveClaim(server, claimRequest(http.MethodPost, "/api/player-claims/verify", body))
	if verified.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", verified.Code, verified.Body.String())
	}
	if verified.Header().Get("Set-Cookie") != "" {
		t.Fatalf("unexpected cookie: %s", verified.Header().Get("Set-Cookie"))
	}
	var payload struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := json.Unmarshal(verified.Body.Bytes(), &payload); err != nil || payload.SessionToken == "" {
		t.Fatalf("payload = %+v, %v", payload, err)
	}

	unauthorized := serveClaim(server, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized = %d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer "+payload.SessionToken)
	authorized := serveClaim(server, request)
	if authorized.Code != http.StatusOK || !strings.Contains(authorized.Body.String(), `"playerId":"offline"`) {
		t.Fatalf("authorized = %d %s", authorized.Code, authorized.Body.String())
	}

	logout := claimRequest(http.MethodPost, "/api/logout", `{}`)
	logout.Header.Set("Authorization", "Bearer "+payload.SessionToken)
	if response := serveClaim(server, logout); response.Code != http.StatusOK {
		t.Fatalf("logout = %d", response.Code)
	}
	if response := serveClaim(server, request); response.Code != http.StatusUnauthorized {
		t.Fatalf("after logout = %d", response.Code)
	}
}

func TestIncorrectAnswerConsumesQuestion(t *testing.T) {
	server := newQuestionClaimServer(t, nil)
	token := startQuestionClaim(t, server, "online")
	body := `{"challengeToken":"` + token + `","answers":[{"questionId":"q1","option":0}]}`
	response := serveClaim(server, claimRequest(http.MethodPost, "/api/player-claims/verify", body))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "verification_failed") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	response = serveClaim(server, claimRequest(http.MethodPost, "/api/player-claims/verify", body))
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "invalid_or_expired_challenge") {
		t.Fatalf("second = %d %s", response.Code, response.Body.String())
	}
}

func TestClaimsConfigurationMustMatchService(t *testing.T) {
	service, _ := playerclaim.NewService(&claimTestProver{}, playerclaim.Options{})
	disabled := testConfig()
	if _, err := NewWithClaims(disabled, fixedSnapshot{}, service); err == nil {
		t.Fatal("accepted service while disabled")
	}
	enabled := testConfig()
	enabled.PlayerClaimsEnabled = true
	if _, err := NewWithClaims(enabled, fixedSnapshot{}, nil); err == nil {
		t.Fatal("accepted enabled claims without service")
	}
}

func TestNoSuitableQuestionErrorSurvivesWrapping(t *testing.T) {
	err := errors.Join(errors.New("prepare"), playerclaim.ErrNoSuitableQuestion)
	if !errors.Is(err, playerclaim.ErrNoSuitableQuestion) {
		t.Fatal("sentinel not discoverable")
	}
}
