package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
)

const claimTestOrigin = "https://map.example.test"

var claimTestNow = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

type claimHTTPClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *claimHTTPClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *claimHTTPClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}

type claimHTTPProver struct {
	mu sync.Mutex

	prepareErr  error
	verifySteps []claimHTTPVerifyStep
	progress    playerclaim.PrivateProgress
	progressErr error

	prepareCalls int
	verifyCalls  int
	lastTarget   string
	lastSelector uint64
}

type claimHTTPVerifyStep struct {
	err          error
	instructions playerclaim.Instructions
}

type mutableClaimSnapshotSource struct {
	revision uint64
	players  palworld.PlayerSnapshot
}

func (s *mutableClaimSnapshotSource) Snapshot() palworld.Snapshot {
	return palworld.Snapshot{Players: append([]palworld.Player{}, s.players.Players...), Objects: []palworld.WorldObject{}}
}

func (s *mutableClaimSnapshotSource) PlayerSnapshotSince(revision uint64) (palworld.PlayerSnapshot, uint64, bool) {
	if revision == s.revision {
		return palworld.PlayerSnapshot{}, s.revision, false
	}
	value := s.players
	value.Players = append([]palworld.Player{}, value.Players...)
	return value, s.revision, true
}

func (s *mutableClaimSnapshotSource) ObjectSnapshotSince(revision uint64) (palworld.ObjectSnapshot, uint64, bool) {
	if revision == 1 {
		return palworld.ObjectSnapshot{}, 1, false
	}
	return palworld.ObjectSnapshot{Objects: []palworld.WorldObject{}}, 1, true
}

func (p *claimHTTPProver) Prepare(_ context.Context, target string, selector uint64) (playerclaim.Prepared, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	p.lastTarget = target
	p.lastSelector = selector
	if p.prepareErr != nil {
		return playerclaim.Prepared{}, p.prepareErr
	}
	return playerclaim.Prepared{
		Subject:        "private-world-subject:" + target,
		PublicPlayerID: target,
		Evidence: struct {
			Secret string
		}{Secret: "never disclose this evidence"},
	}, nil
}

func (p *claimHTTPProver) Verify(_ context.Context, prepared *playerclaim.Prepared) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.verifyCalls++
	if len(p.verifySteps) == 0 {
		return nil
	}
	step := p.verifySteps[0]
	p.verifySteps = p.verifySteps[1:]
	if step.instructions.Kind != "" && prepared != nil {
		prepared.Instructions = step.instructions
	}
	return step.err
}

func (p *claimHTTPProver) Progress(_ context.Context, subject string) (playerclaim.PrivateProgress, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !strings.HasPrefix(subject, "private-world-subject:") {
		return playerclaim.PrivateProgress{}, playerclaim.ErrUnavailable
	}
	if p.progressErr != nil {
		return playerclaim.PrivateProgress{}, p.progressErr
	}
	return p.progress, nil
}

func (p *claimHTTPProver) calls() (prepare, verify int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prepareCalls, p.verifyCalls
}

func claimHTTPSequenceInstructions(phase playerclaim.ProofPhase) playerclaim.Instructions {
	step := 1
	snapshotAt := claimTestNow.Add(time.Minute)
	pairs := []playerclaim.SlotPair{
		{SlotA: 2, SlotB: 7}, {SlotA: 2, SlotB: 8}, {SlotA: 2, SlotB: 9}, {SlotA: 2, SlotB: 10},
		{SlotA: 2, SlotB: 11}, {SlotA: 2, SlotB: 12}, {SlotA: 2, SlotB: 13},
	}
	if phase == playerclaim.ProofPhaseRestore {
		step = 2
		snapshotAt = claimTestNow.Add(2 * time.Minute)
		pairs = []playerclaim.SlotPair{
			{SlotA: 2, SlotB: 13}, {SlotA: 2, SlotB: 12}, {SlotA: 2, SlotB: 11}, {SlotA: 2, SlotB: 10},
			{SlotA: 2, SlotB: 9}, {SlotA: 2, SlotB: 8}, {SlotA: 2, SlotB: 7},
		}
	}
	return playerclaim.Instructions{
		Kind: playerclaim.InventorySwapSequence, Phase: phase, Step: step, TotalSteps: 2,
		Pairs:      pairs,
		SnapshotAt: snapshotAt,
	}
}

func newClaimHTTPServer(t *testing.T, prover *claimHTTPProver, clock *claimHTTPClock) (*Server, *playerclaim.Service) {
	t.Helper()
	if prover == nil {
		prover = &claimHTTPProver{}
	}
	if clock == nil {
		clock = &claimHTTPClock{now: claimTestNow}
	}
	claims, err := playerclaim.NewService(prover, playerclaim.Options{Now: clock.Now})
	if err != nil {
		t.Fatalf("playerclaim.NewService() error = %v", err)
	}
	cfg := testConfig()
	cfg.PlayerClaimsEnabled = true
	cfg.PlayerClaimsOrigin = claimTestOrigin
	for index := range cfg.PlayerClaimsSecret {
		cfg.PlayerClaimsSecret[index] = 0xab
	}
	server, err := NewWithClaims(cfg, fixedSnapshot{}, claims)
	if err != nil {
		t.Fatalf("NewWithClaims() error = %v", err)
	}
	server.source = fixedSnapshot{value: palworld.Snapshot{Players: claimTestLivePlayers()}}
	return server, claims
}

func claimTestLivePlayers() []palworld.Player {
	ids := []string{"public-player", "session-owner", "different-player"}
	for _, prefix := range []string{"public-player-", "ipv6-player-", "proxy-player-"} {
		for index := 1; index <= 7; index++ {
			ids = append(ids, prefix+string(rune('0'+index)))
		}
	}
	players := make([]palworld.Player, 0, len(ids))
	for _, id := range ids {
		players = append(players, palworld.Player{ID: id, Name: id, Online: true})
	}
	players = append(players, palworld.Player{ID: "offline-player", Name: "Offline", Online: false})
	return players
}

func newClaimMutation(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Origin", claimTestOrigin)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set(claimCSRFHeader, "1")
	request.Header.Set("Content-Type", "application/json")
	return request
}

func serveClaim(t *testing.T, server *Server, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func startClaimHTTP(t *testing.T, server *Server, playerID string) (string, *httptest.ResponseRecorder) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"playerId": playerID})
	if err != nil {
		t.Fatal(err)
	}
	response := serveClaim(t, server, newClaimMutation(http.MethodPost, "/api/player-claims", string(body)))
	if response.Code != http.StatusCreated {
		t.Fatalf("start claim = status %d, body %s", response.Code, response.Body.String())
	}
	var payload struct {
		ChallengeToken string `json:"challengeToken"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return payload.ChallengeToken, response
}

func verifyClaimHTTP(t *testing.T, server *Server, token string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"challengeToken": token})
	if err != nil {
		t.Fatal(err)
	}
	return serveClaim(t, server, newClaimMutation(http.MethodPost, "/api/player-claims/verify", string(body)))
}

func claimSessionFromResponse(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == claimSessionCookie {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie: %v", claimSessionCookie, response.Header().Values("Set-Cookie"))
	return nil
}

func TestHTTPClaimsUseDistinctHostOnlyCookie(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	server.settings.playerClaimsHTTP = true
	response := httptest.NewRecorder()
	server.setClaimCookie(response, strings.Repeat("a", 43), claimTestNow.Add(time.Hour))
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != httpClaimSessionCookie || cookie.Path != "/" || cookie.Domain != "" || cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("HTTP claim cookie = %+v", cookie)
	}
	if cookie.Name == claimSessionCookie {
		t.Fatal("HTTP claim cookie reused the HTTPS __Host- name")
	}
}

func assertPrivateClaimResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	cacheControl := response.Header().Get("Cache-Control")
	if !strings.Contains(cacheControl, "private") || !strings.Contains(cacheControl, "no-store") {
		t.Errorf("private response Cache-Control = %q, want private and no-store", cacheControl)
	}
	if got := response.Header().Get("Pragma"); got != "no-cache" {
		t.Errorf("private response Pragma = %q, want no-cache", got)
	}
	if got := response.Header().Get("ETag"); got != "" {
		t.Errorf("private response ETag = %q, want empty", got)
	}
}

func assertReadyClaimResponse(t *testing.T, response *httptest.ResponseRecorder, want playerclaim.Instructions) {
	t.Helper()
	if response.Code != http.StatusAccepted {
		t.Fatalf("ready response = status %d, body %s", response.Code, response.Body.String())
	}
	assertPrivateClaimResponse(t, response)
	if len(response.Header().Values("Set-Cookie")) != 0 {
		t.Fatalf("ready response set cookies: %v", response.Header().Values("Set-Cookie"))
	}
	var payload struct {
		Status       playerclaim.VerificationStatus `json:"status"`
		Instructions *playerclaim.Instructions      `json:"instructions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != playerclaim.VerificationReady || payload.Instructions == nil || !reflect.DeepEqual(*payload.Instructions, want) {
		t.Fatalf("ready response = %+v, want instructions %+v", payload, want)
	}
	if len(payload.Instructions.Pairs) != 7 {
		t.Fatalf("ready response pairs = %v, want exactly seven ordered swaps", payload.Instructions.Pairs)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 3 || raw["status"] == nil || raw["instructions"] == nil || raw["expiresAt"] == nil {
		t.Fatalf("ready response fields = %v", raw)
	}
	var rawInstructions map[string]json.RawMessage
	if err := json.Unmarshal(raw["instructions"], &rawInstructions); err != nil {
		t.Fatal(err)
	}
	if len(rawInstructions) != 6 {
		t.Fatalf("instruction fields = %v, want only kind, phase, step, totalSteps, pairs, and snapshotAt", rawInstructions)
	}
	var rawPairs []map[string]json.RawMessage
	if err := json.Unmarshal(rawInstructions["pairs"], &rawPairs); err != nil {
		t.Fatal(err)
	}
	for index, pair := range rawPairs {
		if len(pair) != 2 || pair["slotA"] == nil || pair["slotB"] == nil {
			t.Fatalf("pair %d fields = %v, want only slotA and slotB", index, pair)
		}
	}
	for _, privateValue := range []string{"private-world-subject", "never disclose", "subject", "evidence"} {
		if strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(privateValue)) {
			t.Errorf("ready response exposes %q: %s", privateValue, response.Body.String())
		}
	}
}

func assertPendingClaimReplay(t *testing.T, response *httptest.ResponseRecorder, want playerclaim.Instructions) {
	t.Helper()
	if response.Code != http.StatusAccepted {
		t.Fatalf("pending replay response = status %d, body %s", response.Code, response.Body.String())
	}
	assertPrivateClaimResponse(t, response)
	if len(response.Header().Values("Set-Cookie")) != 0 {
		t.Fatalf("pending replay response set cookies: %v", response.Header().Values("Set-Cookie"))
	}
	var payload struct {
		Status       playerclaim.VerificationStatus `json:"status"`
		Instructions *playerclaim.Instructions      `json:"instructions"`
		ExpiresAt    time.Time                      `json:"expiresAt"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != playerclaim.VerificationPending || payload.Instructions == nil ||
		!reflect.DeepEqual(*payload.Instructions, want) || payload.ExpiresAt.IsZero() {
		t.Fatalf("pending replay response = %+v, want current instructions %+v", payload, want)
	}
	for _, privateValue := range []string{"private-world-subject", "never disclose", "subject", "evidence"} {
		if strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(privateValue)) {
			t.Errorf("pending replay response exposes %q: %s", privateValue, response.Body.String())
		}
	}
}

func TestPlayerClaimsConfigurationAndServiceMustMatch(t *testing.T) {
	prover := &claimHTTPProver{}
	claims, err := playerclaim.NewService(prover, playerclaim.Options{})
	if err != nil {
		t.Fatal(err)
	}

	disabled := testConfig()
	if _, err := NewWithClaims(disabled, fixedSnapshot{}, claims); err == nil {
		t.Fatal("NewWithClaims() accepted a service while player claims were disabled")
	}

	enabled := testConfig()
	enabled.PlayerClaimsEnabled = true
	enabled.PlayerClaimsOrigin = claimTestOrigin
	if _, err := NewWithClaims(enabled, fixedSnapshot{}, nil); err == nil {
		t.Fatal("NewWithClaims() accepted enabled player claims without a service")
	}
	if _, err := New(enabled, fixedSnapshot{}); err == nil {
		t.Fatal("New() accepted enabled player claims without a service")
	}
}

func TestPublicConfigExposesOnlyPlayerClaimCapability(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	response := serveClaim(t, server, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("config = status %d, body %s", response.Code, response.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if got := string(payload["playerClaimsEnabled"]); got != "true" {
		t.Fatalf("playerClaimsEnabled = %s, want true", got)
	}
	for _, forbiddenKey := range []string{"playerClaimsOrigin", "playerClaimsSecret", "claimsOrigin", "claimsSecret"} {
		if _, exists := payload[forbiddenKey]; exists {
			t.Errorf("public config exposes private key %q", forbiddenKey)
		}
	}
	body := response.Body.String()
	for _, forbiddenValue := range []string{claimTestOrigin, strings.Repeat("ab", 32), "admin-secret-never-expose"} {
		if strings.Contains(body, forbiddenValue) {
			t.Errorf("public config exposes private value %q: %s", forbiddenValue, body)
		}
	}
}

func TestClaimsModePublicProjectionExcludesSavedRosterAndProgress(t *testing.T) {
	captures := int64(9001)
	paldeck := 150
	updated := claimTestNow.Add(-time.Minute)
	players := []palworld.Player{
		{ID: "save-guild", Name: "Save Guild", Online: true, GuildKey: "guild:save", GuildName: "Private Save Guild", Level: 55, X: 10, Y: 20, Map: "palpagos", LastSeenAt: updated, CaptureTotal: &captures, PaldeckUnlocked: &paldeck},
		{ID: "live-guild", Name: "Live Guild", Online: true, GuildKey: "guild:live", GuildName: "Current Live Guild", GuildFromLive: true, Level: 55, X: 12, Y: 22, Map: "palpagos"},
		{ID: "offline", Name: "Offline Save", Online: false, Level: 60, X: 30, Y: 40, Map: "palpagos", LastSeenAt: updated, CaptureTotal: &captures, PaldeckUnlocked: &paldeck},
	}
	projected := publicPlayerSnapshot(palworld.PlayerSnapshot{
		Players: players, SaveEnabled: true, SaveAvailable: true, SaveStale: true,
		SaveUpdatedAt: updated, SaveSnapshotAt: updated, SaveLastError: "private decoder detail",
	})
	if len(projected.Players) != 2 || projected.Players[0].ID != "save-guild" || projected.Players[1].ID != "live-guild" {
		t.Fatalf("public players = %+v", projected.Players)
	}
	player := projected.Players[0]
	if player.GuildKey != "" || player.GuildName != "" || !player.LastSeenAt.IsZero() || player.CaptureTotal != nil || player.PaldeckUnlocked != nil ||
		projected.SaveEnabled || projected.SaveAvailable || projected.SaveStale ||
		!projected.SaveUpdatedAt.IsZero() || !projected.SaveSnapshotAt.IsZero() || projected.SaveLastError != "" {
		t.Fatalf("public projection retained saved data: player=%+v snapshot=%+v", player, projected)
	}
	if live := projected.Players[1]; live.GuildKey != "guild:live" || live.GuildName != "Current Live Guild" || !live.GuildFromLive {
		t.Fatalf("public projection removed live guild evidence: %+v", live)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Offline Save", "Private Save Guild", "guild:save", "private decoder detail", "captureTotal", "paldeckUnlocked", "lastSeenAt"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("public projection exposed %q: %s", forbidden, encoded)
		}
	}
}

func TestClaimsModePublicEndpointsCannotBypassSavedDataBoundary(t *testing.T) {
	captures := int64(42)
	lastSeen := claimTestNow.Add(-time.Hour)
	server, _ := newClaimHTTPServer(t, nil, nil)
	server.source = fixedSnapshot{value: palworld.Snapshot{
		SaveEnabled: true, SaveAvailable: true, SaveStale: true, SaveLastError: "private save failure",
		SaveUpdatedAt: lastSeen, SaveSnapshotAt: lastSeen,
		Players: []palworld.Player{
			{ID: "online", Name: "Online", Online: true, Level: 10, X: 1, Y: 2, Map: "palpagos", CaptureTotal: &captures, LastSeenAt: lastSeen},
			{ID: "offline", Name: "Offline Secret", Online: false, Level: 60, X: 3, Y: 4, Map: "palpagos", CaptureTotal: &captures, LastSeenAt: lastSeen},
		},
	}}
	for _, endpoint := range []string{"/api/players", "/api/state"} {
		response := serveClaim(t, server, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s = status %d, body %s", endpoint, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(body, `"id":"online"`) {
			t.Fatalf("%s omitted live player: %s", endpoint, body)
		}
		for _, forbidden := range []string{"Offline Secret", "private save failure", "captureTotal", "lastSeenAt", `"saveAvailable":true`, `"saveEnabled":true`} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s exposed %q: %s", endpoint, forbidden, body)
			}
		}
	}
}

func TestPrivateSaveOnlyChangeDoesNotChangePublicPlayersETag(t *testing.T) {
	firstCaptures, secondCaptures := int64(10), int64(11)
	source := &mutableClaimSnapshotSource{revision: 1, players: palworld.PlayerSnapshot{
		Players:     []palworld.Player{{ID: "online", Name: "Online", Online: true, X: 1, Y: 2, Map: "palpagos", CaptureTotal: &firstCaptures}},
		SaveEnabled: true, SaveAvailable: true, SaveUpdatedAt: claimTestNow,
	}}
	server, _ := newClaimHTTPServer(t, nil, nil)
	server.source = source
	first := serveClaim(t, server, httptest.NewRequest(http.MethodGet, "/api/players", nil))
	if first.Code != http.StatusOK || first.Header().Get("ETag") == "" {
		t.Fatalf("first players = status %d, etag %q, body %s", first.Code, first.Header().Get("ETag"), first.Body.String())
	}
	etag := first.Header().Get("ETag")
	source.revision = 2
	source.players.Players[0].CaptureTotal = &secondCaptures
	source.players.SaveUpdatedAt = claimTestNow.Add(time.Minute)
	request := httptest.NewRequest(http.MethodGet, "/api/players", nil)
	request.Header.Set("If-None-Match", etag)
	second := serveClaim(t, server, request)
	if second.Code != http.StatusNotModified || second.Header().Get("ETag") != etag || second.Body.Len() != 0 {
		t.Fatalf("private-only update = status %d, etag %q, body %s", second.Code, second.Header().Get("ETag"), second.Body.String())
	}
}

func TestClaimMutationsRequireStrictBrowserAndJSONHeaders(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*http.Request)
		wantStatus int
	}{
		{name: "missing origin", mutate: func(r *http.Request) { r.Header.Del("Origin") }, wantStatus: http.StatusForbidden},
		{name: "wrong origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }, wantStatus: http.StatusForbidden},
		{name: "uppercase equivalent origin", mutate: func(r *http.Request) { r.Header.Set("Origin", "https://MAP.example.test") }, wantStatus: http.StatusForbidden},
		{name: "default port equivalent origin", mutate: func(r *http.Request) { r.Header.Set("Origin", claimTestOrigin+":443") }, wantStatus: http.StatusForbidden},
		{name: "missing fetch metadata", mutate: func(r *http.Request) { r.Header.Del("Sec-Fetch-Site") }, wantStatus: http.StatusForbidden},
		{name: "same site is not same origin", mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-site") }, wantStatus: http.StatusForbidden},
		{name: "cross site", mutate: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, wantStatus: http.StatusForbidden},
		{name: "missing csrf header", mutate: func(r *http.Request) { r.Header.Del(claimCSRFHeader) }, wantStatus: http.StatusForbidden},
		{name: "wrong csrf header", mutate: func(r *http.Request) { r.Header.Set(claimCSRFHeader, "true") }, wantStatus: http.StatusForbidden},
		{name: "missing content type", mutate: func(r *http.Request) { r.Header.Del("Content-Type") }, wantStatus: http.StatusUnsupportedMediaType},
		{name: "non json content type", mutate: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, wantStatus: http.StatusUnsupportedMediaType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prover := &claimHTTPProver{}
			server, _ := newClaimHTTPServer(t, prover, nil)
			request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player"}`)
			test.mutate(request)
			response := serveClaim(t, server, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), `"error":`) {
				t.Fatalf("response = status %d, body %s; want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			if prepare, verify := prover.calls(); prepare != 0 || verify != 0 {
				t.Fatalf("rejected request reached prover: prepare=%d verify=%d", prepare, verify)
			}
			assertPrivateClaimResponse(t, response)
		})
	}

	t.Run("json content type parameters are accepted", func(t *testing.T) {
		server, _ := newClaimHTTPServer(t, nil, nil)
		request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player"}`)
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
		response := serveClaim(t, server, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("response = status %d, body %s", response.Code, response.Body.String())
		}
	})

	t.Run("canonical non-default port origin is accepted exactly", func(t *testing.T) {
		server, _ := newClaimHTTPServer(t, nil, nil)
		server.settings.playerClaimsOrigin = claimTestOrigin + ":8443"
		request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player"}`)
		request.Header.Set("Origin", claimTestOrigin+":8443")
		response := serveClaim(t, server, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("response = status %d, body %s", response.Code, response.Body.String())
		}
	})
}

func TestClaimStartRejectsMalformedAndAmbiguousBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "truncated", body: `{"playerId":`},
		{name: "empty object", body: `{}`},
		{name: "empty player", body: `{"playerId":""}`},
		{name: "whitespace player", body: `{"playerId":"  "}`},
		{name: "padded player", body: `{"playerId":" public-player"}`},
		{name: "control character", body: `{"playerId":"public\u0000player"}`},
		{name: "unknown field", body: `{"playerId":"public-player","subject":"stolen"}`},
		{name: "trailing document", body: `{"playerId":"public-player"} {"playerId":"second"}`},
		{name: "wrong shape", body: `[]`},
		{name: "player id too long", body: `{"playerId":"` + strings.Repeat("a", maxClaimPlayerID+1) + `"}`},
		{name: "body too large", body: `{"playerId":"` + strings.Repeat("a", maxClaimBody) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prover := &claimHTTPProver{}
			server, _ := newClaimHTTPServer(t, prover, nil)
			response := serveClaim(t, server, newClaimMutation(http.MethodPost, "/api/player-claims", test.body))
			if response.Code != http.StatusBadRequest || response.Body.String() != "{\"error\":\"invalid_request\"}\n" {
				t.Fatalf("response = status %d, body %s", response.Code, response.Body.String())
			}
			if prepare, _ := prover.calls(); prepare != 0 {
				t.Fatalf("invalid body reached prover %d times", prepare)
			}
			assertPrivateClaimResponse(t, response)
		})
	}
}

func TestClaimStartReturnsOnlyArmingStateAndOpaqueToken(t *testing.T) {
	prover := &claimHTTPProver{}
	server, _ := newClaimHTTPServer(t, prover, nil)
	token, response := startClaimHTTP(t, server, "public-player")
	assertPrivateClaimResponse(t, response)

	decodedToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decodedToken) != 32 {
		t.Fatalf("challengeToken decodes to %d bytes with error %v", len(decodedToken), err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 3 || payload["challengeToken"] == nil || payload["status"] == nil || payload["expiresAt"] == nil {
		t.Fatalf("start response fields = %v", payload)
	}
	if string(payload["status"]) != `"arming"` {
		t.Fatalf("start status = %s, want arming", payload["status"])
	}
	if payload["instructions"] != nil {
		t.Fatalf("unarmed challenge exposed instructions: %s", payload["instructions"])
	}
	if len(response.Header().Values("Set-Cookie")) != 0 {
		t.Fatalf("unarmed challenge set cookies: %v", response.Header().Values("Set-Cookie"))
	}
	for _, privateValue := range []string{"private-world-subject", "never disclose", "evidence", "subject"} {
		if strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(privateValue)) {
			t.Errorf("start response exposes %q: %s", privateValue, response.Body.String())
		}
	}
}

func TestUnknownClaimTargetHasGenericUnavailableResponse(t *testing.T) {
	tests := []struct {
		name        string
		playerID    string
		prepareErr  error
		wantPrepare int
	}{
		{name: "unknown", playerID: "unknown-player", wantPrepare: 0},
		{name: "offline", playerID: "offline-player", wantPrepare: 0},
		{name: "live unavailable", playerID: "public-player", prepareErr: playerclaim.ErrUnavailable, wantPrepare: 1},
		{name: "live decoder detail", playerID: "public-player", prepareErr: errors.New("private decoder detail: player does not exist"), wantPrepare: 1},
	}
	var wantBody string
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prover := &claimHTTPProver{prepareErr: test.prepareErr}
			server, _ := newClaimHTTPServer(t, prover, nil)
			body, err := json.Marshal(map[string]string{"playerId": test.playerID})
			if err != nil {
				t.Fatal(err)
			}
			response := serveClaim(t, server, newClaimMutation(http.MethodPost, "/api/player-claims", string(body)))
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("error %v = status %d, body %s", test.prepareErr, response.Code, response.Body.String())
			}
			if index == 0 {
				wantBody = response.Body.String()
			} else if response.Body.String() != wantBody {
				t.Fatalf("unknown target responses differ: %q != %q", response.Body.String(), wantBody)
			}
			if response.Body.String() != "{\"error\":\"claim_unavailable\"}\n" || strings.Contains(response.Body.String(), "decoder") {
				t.Fatalf("non-generic unknown target response: %s", response.Body.String())
			}
			if prepare, _ := prover.calls(); prepare != test.wantPrepare {
				t.Fatalf("Prover.Prepare calls = %d, want %d", prepare, test.wantPrepare)
			}
			assertPrivateClaimResponse(t, response)
		})
	}
}

func TestClaimOrderedLifecycleIssuesSessionOnlyAfterAllActions(t *testing.T) {
	clock := &claimHTTPClock{now: claimTestNow}
	firstInstructions := claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve)
	secondInstructions := claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore)
	prover := &claimHTTPProver{verifySteps: []claimHTTPVerifyStep{
		{err: playerclaim.ErrPending},
		{err: playerclaim.ErrReady, instructions: firstInstructions},
		{err: playerclaim.ErrReady, instructions: secondInstructions},
		{err: playerclaim.ErrPending},
		{},
	}}
	server, _ := newClaimHTTPServer(t, prover, clock)
	challengeToken, started := startClaimHTTP(t, server, "public-player")
	if !strings.Contains(started.Body.String(), `"status":"arming"`) || strings.Contains(started.Body.String(), `"instructions"`) {
		t.Fatalf("start response = %s", started.Body.String())
	}

	arming := verifyClaimHTTP(t, server, challengeToken)
	if arming.Code != http.StatusAccepted || arming.Body.String() != "{\"status\":\"arming\"}\n" {
		t.Fatalf("arming response = status %d, body %s", arming.Code, arming.Body.String())
	}
	if len(arming.Header().Values("Set-Cookie")) != 0 {
		t.Fatalf("arming response set cookies: %v", arming.Header().Values("Set-Cookie"))
	}
	assertPrivateClaimResponse(t, arming)

	firstReady := verifyClaimHTTP(t, server, challengeToken)
	assertReadyClaimResponse(t, firstReady, firstInstructions)

	secondReady := verifyClaimHTTP(t, server, challengeToken)
	assertReadyClaimResponse(t, secondReady, secondInstructions)
	if firstReady.Body.String() == secondReady.Body.String() {
		t.Fatalf("ordered proof repeated the same instruction: %s", secondReady.Body.String())
	}
	for index, pair := range firstInstructions.Pairs {
		if restore := secondInstructions.Pairs[len(secondInstructions.Pairs)-1-index]; restore != pair {
			t.Fatalf("restore pair %d = %+v, want reverse of prove pair %+v", index, restore, pair)
		}
	}

	pending := verifyClaimHTTP(t, server, challengeToken)
	assertPendingClaimReplay(t, pending, secondInstructions)

	verified := verifyClaimHTTP(t, server, challengeToken)
	if verified.Code != http.StatusOK {
		t.Fatalf("verified response = status %d, body %s", verified.Code, verified.Body.String())
	}
	assertPrivateClaimResponse(t, verified)
	cookie := claimSessionFromResponse(t, verified)
	decodedBearer, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(decodedBearer) != 32 {
		t.Fatalf("session cookie decodes to %d bytes with error %v", len(decodedBearer), err)
	}
	if cookie.Name != "__Host-palworld_live_map_session" || cookie.Path != "/" || cookie.Domain != "" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe session cookie: %+v", cookie)
	}
	if !cookie.Expires.Equal(claimTestNow.Add(playerclaim.DefaultSessionAbsoluteTTL)) {
		t.Fatalf("cookie expiry = %v, want %v", cookie.Expires, claimTestNow.Add(playerclaim.DefaultSessionAbsoluteTTL))
	}
	for _, privateValue := range []string{cookie.Value, challengeToken, "private-world-subject", "never disclose", "bearer", "token", "subject", "evidence"} {
		if strings.Contains(strings.ToLower(verified.Body.String()), strings.ToLower(privateValue)) {
			t.Errorf("verified response exposes %q: %s", privateValue, verified.Body.String())
		}
	}

	meRequest := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meRequest.AddCookie(cookie)
	me := serveClaim(t, server, meRequest)
	if me.Code != http.StatusOK {
		t.Fatalf("me response = status %d, body %s", me.Code, me.Body.String())
	}
	assertPrivateClaimResponse(t, me)
	var mePayload map[string]json.RawMessage
	if err := json.Unmarshal(me.Body.Bytes(), &mePayload); err != nil {
		t.Fatal(err)
	}
	if string(mePayload["authenticated"]) != "true" || string(mePayload["playerId"]) != `"public-player"` {
		t.Fatalf("me response = %s", me.Body.String())
	}
	for _, privateValue := range []string{cookie.Value, "private-world-subject", "never disclose", "subject", "evidence"} {
		if strings.Contains(strings.ToLower(me.Body.String()), strings.ToLower(privateValue)) {
			t.Errorf("me response exposes %q: %s", privateValue, me.Body.String())
		}
	}

	logoutRequest := newClaimMutation(http.MethodPost, "/api/logout", `{}`)
	logoutRequest.AddCookie(cookie)
	logout := serveClaim(t, server, logoutRequest)
	if logout.Code != http.StatusOK || logout.Body.String() != "{\"authenticated\":false}\n" {
		t.Fatalf("logout response = status %d, body %s", logout.Code, logout.Body.String())
	}
	assertPrivateClaimResponse(t, logout)
	cleared := claimSessionFromResponse(t, logout)
	if cleared.Value != "" || cleared.MaxAge >= 0 || !cleared.Secure || !cleared.HttpOnly || cleared.SameSite != http.SameSiteStrictMode || cleared.Path != "/" {
		t.Fatalf("unsafe clearing cookie: %+v", cleared)
	}

	reusedRequest := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	reusedRequest.AddCookie(cookie)
	reused := serveClaim(t, server, reusedRequest)
	if reused.Code != http.StatusUnauthorized || reused.Body.String() != "{\"error\":\"authentication_required\"}\n" {
		t.Fatalf("revoked session response = status %d, body %s", reused.Code, reused.Body.String())
	}
	assertPrivateClaimResponse(t, reused)
}

func TestClaimPendingReplaysLostReadyInstructions(t *testing.T) {
	prove := claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve)
	restore := claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore)
	prover := &claimHTTPProver{verifySteps: []claimHTTPVerifyStep{
		{err: playerclaim.ErrReady, instructions: prove},
		{err: playerclaim.ErrPending},
		{err: playerclaim.ErrReady, instructions: restore},
		{err: playerclaim.ErrPending},
	}}
	server, _ := newClaimHTTPServer(t, prover, nil)
	challengeToken, _ := startClaimHTTP(t, server, "public-player")

	// Treat each ready response as dropped after the service has advanced its
	// private phase. The next request must replay the bearer-protected sequence.
	_ = verifyClaimHTTP(t, server, challengeToken)
	assertPendingClaimReplay(t, verifyClaimHTTP(t, server, challengeToken), prove)
	_ = verifyClaimHTTP(t, server, challengeToken)
	assertPendingClaimReplay(t, verifyClaimHTTP(t, server, challengeToken), restore)
}

func TestAuthenticatedProgressProjectsPrivateKeysToCatalogueIDs(t *testing.T) {
	prover := &claimHTTPProver{verifySteps: []claimHTTPVerifyStep{
		{err: playerclaim.ErrReady, instructions: claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve)},
		{err: playerclaim.ErrReady, instructions: claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore)},
		{},
	}}
	server, _ := newClaimHTTPServer(t, prover, nil)
	var waypointKey, waypointID, journalKey, journalID string
	for _, record := range server.worldCatalogue.Completion {
		switch record.Category {
		case "waypoints":
			if waypointKey == "" {
				waypointKey, waypointID = record.StateKey, record.LocationID
			}
		case "journals":
			if journalKey == "" {
				journalKey, journalID = record.StateKey, record.LocationID
			}
		}
	}
	if waypointKey == "" || journalKey == "" {
		t.Fatal("embedded catalogue has no completion records for progress test")
	}
	prover.mu.Lock()
	prover.progress = playerclaim.PrivateProgress{
		SnapshotAt: claimTestNow, FastTravelKeys: []string{waypointKey, "private-unmatched-fast-travel-key"},
		NoteKeys: []string{journalKey, "private-unmatched-note-key"},
	}
	prover.mu.Unlock()

	token, _ := startClaimHTTP(t, server, "public-player")
	assertReadyClaimResponse(t, verifyClaimHTTP(t, server, token), claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve))
	assertReadyClaimResponse(t, verifyClaimHTTP(t, server, token), claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore))
	verified := verifyClaimHTTP(t, server, token)
	if verified.Code != http.StatusOK {
		t.Fatalf("verified response = status %d, body %s", verified.Code, verified.Body.String())
	}
	cookie := claimSessionFromResponse(t, verified)

	request := httptest.NewRequest(http.MethodGet, "/api/me/progress", nil)
	request.AddCookie(cookie)
	response := serveClaim(t, server, request)
	if response.Code != http.StatusOK {
		t.Fatalf("progress response = status %d, body %s", response.Code, response.Body.String())
	}
	assertPrivateClaimResponse(t, response)
	var payload claimProgressResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.SnapshotAt.Equal(claimTestNow) || payload.CatalogueVersion != server.worldCatalogue.ContentHash || len(payload.Domains) != 2 {
		t.Fatalf("progress response = %+v", payload)
	}
	completed := map[string][]string{}
	for _, domain := range payload.Domains {
		if domain.Coverage != "complete" || domain.Total <= 0 {
			t.Fatalf("domain = %+v", domain)
		}
		completed[domain.ID] = domain.CompletedIDs
	}
	if !reflect.DeepEqual(completed["waypoints"], []string{waypointID}) || !reflect.DeepEqual(completed["journals"], []string{journalID}) {
		t.Fatalf("completed IDs = %v", completed)
	}
	body := response.Body.String()
	for _, forbidden := range []string{waypointKey, "private-unmatched-fast-travel-key", "private-unmatched-note-key", "stateKey", "private-world-subject"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("progress response exposed %q: %s", forbidden, body)
		}
	}
}

func TestProgressRequiresAuthenticatedSession(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	response := serveClaim(t, server, httptest.NewRequest(http.MethodGet, "/api/me/progress", nil))
	if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"authentication_required\"}\n" {
		t.Fatalf("progress response = status %d, body %s", response.Code, response.Body.String())
	}
	assertPrivateClaimResponse(t, response)
}

func TestInvalidAndExpiredChallengesShareGenericResponse(t *testing.T) {
	clock := &claimHTTPClock{now: claimTestNow}
	server, _ := newClaimHTTPServer(t, nil, clock)
	actualToken, _ := startClaimHTTP(t, server, "public-player")
	unknownToken := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 32))

	unknown := verifyClaimHTTP(t, server, unknownToken)
	clock.Advance(playerclaim.ChallengePhaseTTL)
	expired := verifyClaimHTTP(t, server, actualToken)
	for name, response := range map[string]*httptest.ResponseRecorder{"unknown": unknown, "expired": expired} {
		if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"error\":\"invalid_or_expired_challenge\"}\n" {
			t.Errorf("%s response = status %d, body %s", name, response.Code, response.Body.String())
		}
		assertPrivateClaimResponse(t, response)
		if cookies := response.Header().Values("Set-Cookie"); len(cookies) != 0 {
			t.Errorf("%s response changed an unrelated session cookie: %v", name, cookies)
		}
	}
	if unknown.Code != expired.Code || unknown.Body.String() != expired.Body.String() {
		t.Fatalf("unknown and expired challenge responses differ: %d %q != %d %q", unknown.Code, unknown.Body.String(), expired.Code, expired.Body.String())
	}

	malformed := verifyClaimHTTP(t, server, "not-a-valid-token")
	if malformed.Code != http.StatusBadRequest || malformed.Body.String() != "{\"error\":\"invalid_request\"}\n" {
		t.Fatalf("malformed response = status %d, body %s", malformed.Code, malformed.Body.String())
	}
}

func TestInvalidChallengeDoesNotClearOrRevokeUnrelatedSession(t *testing.T) {
	clock := &claimHTTPClock{now: claimTestNow}
	prover := &claimHTTPProver{verifySteps: []claimHTTPVerifyStep{
		{err: playerclaim.ErrReady, instructions: claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve)},
		{err: playerclaim.ErrReady, instructions: claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore)},
		{},
	}}
	server, _ := newClaimHTTPServer(t, prover, clock)
	sessionChallenge, _ := startClaimHTTP(t, server, "session-owner")
	assertReadyClaimResponse(t, verifyClaimHTTP(t, server, sessionChallenge), claimHTTPSequenceInstructions(playerclaim.ProofPhaseProve))
	assertReadyClaimResponse(t, verifyClaimHTTP(t, server, sessionChallenge), claimHTTPSequenceInstructions(playerclaim.ProofPhaseRestore))
	verified := verifyClaimHTTP(t, server, sessionChallenge)
	if verified.Code != http.StatusOK {
		t.Fatalf("session verification = status %d, body %s", verified.Code, verified.Body.String())
	}
	sessionCookie := claimSessionFromResponse(t, verified)

	assertSessionWorks := func(label string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		request.AddCookie(sessionCookie)
		response := serveClaim(t, server, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"playerId":"session-owner"`) {
			t.Fatalf("%s session = status %d, body %s", label, response.Code, response.Body.String())
		}
	}
	assertSessionWorks("initial")

	unknownToken := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xee}, 32))
	unknownBody, err := json.Marshal(map[string]string{"challengeToken": unknownToken})
	if err != nil {
		t.Fatal(err)
	}
	unknownRequest := newClaimMutation(http.MethodPost, "/api/player-claims/verify", string(unknownBody))
	unknownRequest.AddCookie(sessionCookie)
	unknown := serveClaim(t, server, unknownRequest)
	if unknown.Code != http.StatusUnauthorized || unknown.Body.String() != "{\"error\":\"invalid_or_expired_challenge\"}\n" {
		t.Fatalf("unknown challenge = status %d, body %s", unknown.Code, unknown.Body.String())
	}
	if cookies := unknown.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("unknown challenge changed session cookie: %v", cookies)
	}
	assertSessionWorks("after unknown challenge")

	expiringToken, _ := startClaimHTTP(t, server, "different-player")
	clock.Advance(playerclaim.ChallengePhaseTTL)
	expiredBody, err := json.Marshal(map[string]string{"challengeToken": expiringToken})
	if err != nil {
		t.Fatal(err)
	}
	expiredRequest := newClaimMutation(http.MethodPost, "/api/player-claims/verify", string(expiredBody))
	expiredRequest.AddCookie(sessionCookie)
	expired := serveClaim(t, server, expiredRequest)
	if expired.Code != http.StatusUnauthorized || expired.Body.String() != unknown.Body.String() {
		t.Fatalf("expired challenge = status %d, body %s", expired.Code, expired.Body.String())
	}
	if cookies := expired.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("expired challenge changed session cookie: %v", cookies)
	}
	assertSessionWorks("after expired challenge")
}

func TestPrivateClaimEndpointsIgnoreConditionalCaching(t *testing.T) {
	prover := &claimHTTPProver{verifySteps: []claimHTTPVerifyStep{{err: playerclaim.ErrPending}}}
	server, _ := newClaimHTTPServer(t, prover, nil)

	startRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player"}`)
	startRequest.Header.Set("If-None-Match", "*")
	started := serveClaim(t, server, startRequest)
	if started.Code != http.StatusCreated {
		t.Fatalf("start = status %d, body %s", started.Code, started.Body.String())
	}
	assertPrivateClaimResponse(t, started)
	var challenge struct {
		Token string `json:"challengeToken"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}

	verifyRequest := newClaimMutation(http.MethodPost, "/api/player-claims/verify", `{"challengeToken":"`+challenge.Token+`"}`)
	verifyRequest.Header.Set("If-None-Match", "*")
	pending := serveClaim(t, server, verifyRequest)
	if pending.Code != http.StatusAccepted {
		t.Fatalf("verify = status %d, body %s", pending.Code, pending.Body.String())
	}
	assertPrivateClaimResponse(t, pending)

	meRequest := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	meRequest.Header.Set("If-None-Match", "*")
	unauthenticated := serveClaim(t, server, meRequest)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("me = status %d, body %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	assertPrivateClaimResponse(t, unauthenticated)

	logoutRequest := newClaimMutation(http.MethodPost, "/api/logout", `{}`)
	logoutRequest.Header.Set("If-None-Match", "*")
	logout := serveClaim(t, server, logoutRequest)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout = status %d, body %s", logout.Code, logout.Body.String())
	}
	assertPrivateClaimResponse(t, logout)
}

func TestClaimStartRateLimitUsesDirectIPv4SourceAndIgnoresForwardingHeaders(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	for attempt := 1; attempt <= 5; attempt++ {
		request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player-`+string(rune('0'+attempt))+`"}`)
		request.RemoteAddr = "203.0.113.7:4321"
		request.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('0'+attempt)))
		response := serveClaim(t, server, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("attempt %d = status %d, body %s", attempt, response.Code, response.Body.String())
		}
	}

	limitedRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player-6"}`)
	limitedRequest.RemoteAddr = "203.0.113.7:9999"
	limitedRequest.Header.Set("X-Forwarded-For", "192.0.2.200")
	limited := serveClaim(t, server, limitedRequest)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") != "60" || limited.Body.String() != "{\"error\":\"claim_unavailable\"}\n" {
		t.Fatalf("limited response = status %d, retry %q, body %s", limited.Code, limited.Header().Get("Retry-After"), limited.Body.String())
	}
	assertPrivateClaimResponse(t, limited)

	distinctRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"public-player-7"}`)
	distinctRequest.RemoteAddr = "203.0.113.8:4321"
	distinctRequest.Header.Set("X-Forwarded-For", "203.0.113.7")
	distinct := serveClaim(t, server, distinctRequest)
	if distinct.Code != http.StatusCreated {
		t.Fatalf("distinct source = status %d, body %s", distinct.Code, distinct.Body.String())
	}
}

func TestClaimStartRateLimitGroupsIPv6By64(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	addresses := []string{
		"[2001:db8:1234:5678::1]:1001",
		"[2001:db8:1234:5678::2]:1002",
		"[2001:db8:1234:5678:1111::1]:1003",
		"[2001:db8:1234:5678:2222::1]:1004",
		"[2001:db8:1234:5678:ffff:ffff:ffff:ffff]:1005",
	}
	for index, address := range addresses {
		request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"ipv6-player-`+string(rune('1'+index))+`"}`)
		request.RemoteAddr = address
		response := serveClaim(t, server, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("source %s = status %d, body %s", address, response.Code, response.Body.String())
		}
	}

	limitedRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"ipv6-player-6"}`)
	limitedRequest.RemoteAddr = "[2001:db8:1234:5678:abcd::1]:1006"
	limited := serveClaim(t, server, limitedRequest)
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") != "60" {
		t.Fatalf("same /64 = status %d, retry %q, body %s", limited.Code, limited.Header().Get("Retry-After"), limited.Body.String())
	}

	distinctRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"ipv6-player-7"}`)
	distinctRequest.RemoteAddr = "[2001:db8:1234:5679::1]:1007"
	distinct := serveClaim(t, server, distinctRequest)
	if distinct.Code != http.StatusCreated {
		t.Fatalf("different /64 = status %d, body %s", distinct.Code, distinct.Body.String())
	}
}

func TestClaimStartRateLimitUsesForwardedClientOnlyFromTrustedProxy(t *testing.T) {
	server, _ := newClaimHTTPServer(t, nil, nil)
	server.settings.playerClaimsTrustedProxies = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("192.0.2.0/24"),
	}
	for attempt := 1; attempt <= 5; attempt++ {
		request := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"proxy-player-`+string(rune('0'+attempt))+`"}`)
		request.RemoteAddr = "10.0.0.5:443"
		// The right-most trusted hop is stripped. The untrusted address before it
		// is the client; the left-most value is an attacker-supplied spoof.
		request.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('0'+attempt))+", 198.51.100.9, 192.0.2.4")
		response := serveClaim(t, server, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("attempt %d = status %d, body %s", attempt, response.Code, response.Body.String())
		}
	}
	limitedRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"proxy-player-6"}`)
	limitedRequest.RemoteAddr = "10.0.0.5:443"
	limitedRequest.Header.Set("X-Forwarded-For", "192.0.2.200, 198.51.100.9, 192.0.2.4")
	limited := serveClaim(t, server, limitedRequest)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("shared forwarded client = status %d, body %s", limited.Code, limited.Body.String())
	}

	distinctRequest := newClaimMutation(http.MethodPost, "/api/player-claims", `{"playerId":"proxy-player-7"}`)
	distinctRequest.RemoteAddr = "10.0.0.5:443"
	distinctRequest.Header.Set("X-Forwarded-For", "198.51.100.10, 192.0.2.4")
	distinct := serveClaim(t, server, distinctRequest)
	if distinct.Code != http.StatusCreated {
		t.Fatalf("distinct forwarded client = status %d, body %s", distinct.Code, distinct.Body.String())
	}
}
