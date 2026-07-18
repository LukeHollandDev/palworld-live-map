package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
)

type fixedSnapshot struct{ value palworld.Snapshot }

func (s fixedSnapshot) Snapshot() palworld.Snapshot { return s.value }

func testConfig() config.Config {
	return config.Config{
		RESTURL: "http://palworld:8212", AdminPassword: "admin-secret-never-expose",
		PollInterval: 5 * time.Second, UpstreamTimeout: 4 * time.Second,
		WorldPollInterval: 15 * time.Second, WorldTimeout: 10 * time.Second,
		WorldDataEnabled: true,
	}
}

func TestStateIsPublicAndSanitized(t *testing.T) {
	cfg := testConfig()
	source := fixedSnapshot{value: palworld.Snapshot{
		Connected: true,
		Players:   []palworld.Player{{Name: "Luke", Level: 55, X: 10, Y: -20, Map: "palpagos"}},
		Objects:   []palworld.WorldObject{{Kind: "bases", Name: "Home", X: 5, Y: 6, Map: "palpagos"}},
	}}
	service, err := New(cfg, source)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("state status = %d", response.Code)
	}
	body := response.Body.String()
	if strings.Contains(body, cfg.AdminPassword) || strings.Contains(body, "user_id") || !strings.Contains(body, `"name":"Luke"`) {
		t.Fatalf("unexpected state body: %s", body)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("state response may be cached")
	}
}

func TestSplitStateEndpointsDoNotRepeatUnrelatedData(t *testing.T) {
	cfg := testConfig()
	source := fixedSnapshot{value: palworld.Snapshot{
		Server:           palworld.ServerInfo{Name: "The Chaos"},
		Metrics:          palworld.ServerMetrics{ServerFPS: 59, MaxPlayers: 20},
		MetricsAvailable: true,
		Connected:        true,
		Players:          []palworld.Player{{Name: "Luke", Level: 55, X: 10, Y: -20, Map: "palpagos"}},
		ObjectsAvailable: true,
		Objects:          []palworld.WorldObject{{Kind: "bases", Name: "Home", X: 5, Y: 6, Map: "palpagos"}},
	}}
	service, err := New(cfg, source)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	players := httptest.NewRecorder()
	service.Handler().ServeHTTP(players, httptest.NewRequest(http.MethodGet, "/api/players", nil))
	if players.Code != http.StatusOK || !strings.Contains(players.Body.String(), `"name":"Luke"`) {
		t.Fatalf("players response = status %d, body %s", players.Code, players.Body.String())
	}
	if !strings.Contains(players.Body.String(), `"serverFps":59`) || !strings.Contains(players.Body.String(), `"maxPlayers":20`) {
		t.Fatalf("players response has no metrics: %s", players.Body.String())
	}
	if strings.Contains(players.Body.String(), `"name":"Home"`) || strings.Contains(players.Body.String(), `"objects"`) {
		t.Fatalf("players response contains world objects: %s", players.Body.String())
	}

	objects := httptest.NewRecorder()
	service.Handler().ServeHTTP(objects, httptest.NewRequest(http.MethodGet, "/api/objects", nil))
	if objects.Code != http.StatusOK || !strings.Contains(objects.Body.String(), `"name":"Home"`) {
		t.Fatalf("objects response = status %d, body %s", objects.Code, objects.Body.String())
	}
	if strings.Contains(objects.Body.String(), `"name":"Luke"`) || strings.Contains(objects.Body.String(), `"players"`) {
		t.Fatalf("objects response contains players: %s", objects.Body.String())
	}
}

func TestPublicConfigAndObjectsExposeDisabledWorldData(t *testing.T) {
	cfg := testConfig()
	cfg.WorldDataEnabled = false
	service, err := New(cfg, fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, path := range []string{"/api/config", "/api/objects"} {
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"enabled":false`) && !strings.Contains(response.Body.String(), `"worldDataEnabled":false`) {
			t.Fatalf("%s response = status %d, body %s", path, response.Code, response.Body.String())
		}
	}
}

func TestServerServesOnlyKnownEmbeddedMapArtwork(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allowed := httptest.NewRecorder()
	service.Handler().ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/assets/map/palpagos.jpg", nil))
	if allowed.Code != http.StatusOK || allowed.Header().Get("Content-Type") != "image/jpeg" || allowed.Body.Len() < 1_000_000 {
		t.Fatalf("map response = status %d, type %q, size %d", allowed.Code, allowed.Header().Get("Content-Type"), allowed.Body.Len())
	}
	for _, path := range []string{"/assets/map/secret.txt", "/assets/map/"} {
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", path, response.Code)
		}
	}
}

func TestEmptySnapshotOmitsZeroTimestamps(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if strings.Contains(response.Body.String(), "lastSuccessAt") || strings.Contains(response.Body.String(), "objectsUpdatedAt") {
		t.Fatalf("zero timestamp was serialized: %s", response.Body.String())
	}
}
