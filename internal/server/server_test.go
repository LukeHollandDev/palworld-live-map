package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		WorldDataEnabled: true, SiteTitle: "Test Map",
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

func TestServerRestrictsMapArtworkToKnownFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "palpagos.webp"), []byte("private-map"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("do-not-serve"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.MapAssetDir = dir
	service, err := New(cfg, fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	allowed := httptest.NewRecorder()
	service.Handler().ServeHTTP(allowed, httptest.NewRequest(http.MethodGet, "/map-assets/palpagos.webp", nil))
	if allowed.Code != http.StatusOK || allowed.Body.String() != "private-map" {
		t.Fatalf("map response = %d %q", allowed.Code, allowed.Body.String())
	}
	for _, path := range []string{"/map-assets/secret.txt", "/map-assets/"} {
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
