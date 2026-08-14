package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
)

type fixedSnapshot struct{ value palworld.Snapshot }

func (s fixedSnapshot) PlayerSnapshotSince(revision uint64) (palworld.PlayerSnapshot, uint64, bool) {
	if revision == 1 {
		return palworld.PlayerSnapshot{}, 1, false
	}
	return s.PlayerSnapshot(), 1, true
}

func (s fixedSnapshot) ObjectSnapshotSince(revision uint64) (palworld.ObjectSnapshot, uint64, bool) {
	if revision == 1 {
		return palworld.ObjectSnapshot{}, 1, false
	}
	return s.ObjectSnapshot(), 1, true
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

type trackingSnapshotSource struct {
	fullCalls   int
	playerCalls int
	objectCalls int
}

type revisionSnapshotSource struct {
	playerBuilds int
	objectBuilds int
	objects      []palworld.WorldObject
}

func (s *revisionSnapshotSource) Snapshot() palworld.Snapshot {
	return palworld.Snapshot{Players: []palworld.Player{}, Objects: []palworld.WorldObject{}}
}

func (s *revisionSnapshotSource) PlayerSnapshotSince(revision uint64) (palworld.PlayerSnapshot, uint64, bool) {
	if revision == 1 {
		return palworld.PlayerSnapshot{}, 1, false
	}
	s.playerBuilds++
	return palworld.PlayerSnapshot{Players: []palworld.Player{}}, 1, true
}

func (s *revisionSnapshotSource) ObjectSnapshotSince(revision uint64) (palworld.ObjectSnapshot, uint64, bool) {
	if revision == 1 {
		return palworld.ObjectSnapshot{}, 1, false
	}
	s.objectBuilds++
	objects := s.objects
	if objects == nil {
		objects = []palworld.WorldObject{{ID: "base", Kind: "bases", Name: "Home"}}
	}
	return palworld.ObjectSnapshot{Available: true, Total: len(objects), Objects: objects}, 1, true
}

func (s *trackingSnapshotSource) Snapshot() palworld.Snapshot {
	s.fullCalls++
	return palworld.Snapshot{Players: []palworld.Player{}, Objects: []palworld.WorldObject{}}
}

func (s *trackingSnapshotSource) PlayerSnapshot() palworld.PlayerSnapshot {
	s.playerCalls++
	return palworld.PlayerSnapshot{Players: []palworld.Player{}}
}

func (s *trackingSnapshotSource) PlayerSnapshotSince(uint64) (palworld.PlayerSnapshot, uint64, bool) {
	return s.PlayerSnapshot(), uint64(s.playerCalls), true
}

func (s *trackingSnapshotSource) ObjectSnapshot() palworld.ObjectSnapshot {
	s.objectCalls++
	return palworld.ObjectSnapshot{Objects: []palworld.WorldObject{}}
}

func (s *trackingSnapshotSource) ObjectSnapshotSince(uint64) (palworld.ObjectSnapshot, uint64, bool) {
	return s.ObjectSnapshot(), uint64(s.objectCalls), true
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.deadline = deadline
	return nil
}

func (s fixedSnapshot) Snapshot() palworld.Snapshot {
	result := s.value
	result.Players = append([]palworld.Player{}, result.Players...)
	result.Objects = append([]palworld.WorldObject{}, result.Objects...)
	return result
}

func (s fixedSnapshot) PlayerSnapshot() palworld.PlayerSnapshot {
	players := make([]palworld.Player, len(s.value.Players))
	copy(players, s.value.Players)
	return palworld.PlayerSnapshot{
		Server: s.value.Server, Connected: s.value.Connected, Stale: s.value.Stale,
		LastSuccessAt: s.value.LastSuccessAt, Players: players,
		Metrics: s.value.Metrics, MetricsAvailable: s.value.MetricsAvailable,
		MetricsStale: s.value.MetricsStale, MetricsUpdatedAt: s.value.MetricsUpdatedAt,
	}
}

func (s fixedSnapshot) ObjectSnapshot() palworld.ObjectSnapshot {
	objects := make([]palworld.WorldObject, len(s.value.Objects))
	copy(objects, s.value.Objects)
	return palworld.ObjectSnapshot{
		Available: s.value.ObjectsAvailable, Stale: s.value.ObjectsStale,
		Unsupported: s.value.ObjectsUnsupported, Truncated: s.value.ObjectsTruncated,
		Total: s.value.ObjectsTotal, LastError: s.value.ObjectsLastError,
		UpdatedAt: s.value.ObjectsUpdatedAt, Objects: objects,
	}
}

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
	captureTotal := int64(4321)
	uniqueCaptured, paldeckUnlocked := 117, 119
	arenaRankPoints, fastTravelUnlocked, areasDiscovered := 875, 64, 23
	bossDefeats, towerDefeats := 31, 7
	source := fixedSnapshot{value: palworld.Snapshot{
		Server:           palworld.ServerInfo{Name: "The Chaos"},
		Metrics:          palworld.ServerMetrics{ServerFPS: 59, MaxPlayers: 20},
		MetricsAvailable: true,
		Connected:        true,
		Players: []palworld.Player{{
			Name: "Luke", Level: 55, X: 10, Y: -20, Map: "palpagos",
			CaptureTotal: &captureTotal, UniquePalsCaptured: &uniqueCaptured, PaldeckUnlocked: &paldeckUnlocked,
			ArenaRankPoints: &arenaRankPoints, FastTravelUnlocked: &fastTravelUnlocked, AreasDiscovered: &areasDiscovered,
			BossDefeats: &bossDefeats, TowerDefeats: &towerDefeats,
		}},
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
	for _, progress := range []string{
		`"captureTotal":4321`, `"uniquePalsCaptured":117`, `"paldeckUnlocked":119`,
		`"arenaRankPoints":875`, `"fastTravelUnlocked":64`, `"areasDiscovered":23`,
		`"bossDefeats":31`, `"towerDefeats":7`,
	} {
		if !strings.Contains(players.Body.String(), progress) {
			t.Fatalf("players response has no save progress %s: %s", progress, players.Body.String())
		}
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

func TestSplitStateEndpointsUseNarrowSnapshotAccessors(t *testing.T) {
	source := &trackingSnapshotSource{}
	service, err := New(testConfig(), source)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, endpoint := range []string{"/api/players", "/api/objects"} {
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", endpoint, response.Code)
		}
	}
	if source.fullCalls != 0 || source.playerCalls != 1 || source.objectCalls != 1 {
		t.Fatalf("snapshot calls = full:%d player:%d object:%d", source.fullCalls, source.playerCalls, source.objectCalls)
	}
}

func TestJSONEndpointsSupportGzip(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{value: palworld.Snapshot{
		ObjectsAvailable: true,
		Objects:          []palworld.WorldObject{{ID: "object:one", Kind: "npcs", Name: "Merchant"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/objects", nil)
	request.Header.Set("Accept-Encoding", "br, gzip")
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	if response.Header().Get("Content-Encoding") != "gzip" || response.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("gzip headers = encoding %q, vary %q", response.Header().Get("Content-Encoding"), response.Header().Get("Vary"))
	}
	compressed, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"name":"Merchant"`) {
		t.Fatalf("decompressed response = %s", body)
	}

	tests := []struct {
		value string
		want  bool
	}{
		{value: "gzip;q=0.00", want: false},
		{value: "br", want: false},
		{value: "gzip;q=0.5", want: true},
		{value: "gzip;q=bogus", want: false},
		{value: "gzip;q=1.1", want: false},
		{value: "*;q=0.5, identity;q=0", want: true},
		{value: "gzip;q=0, *;q=1", want: false},
	}
	for _, test := range tests {
		if got := acceptsGzip(test.value); got != test.want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestDynamicEndpointsReuseCachedRepresentationsAndReturn304(t *testing.T) {
	source := &revisionSnapshotSource{}
	service, err := New(testConfig(), source)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/objects", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	first := httptest.NewRecorder()
	service.Handler().ServeHTTP(first, request)
	if first.Code != http.StatusOK || first.Header().Get("ETag") == "" || first.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("first response = status %d, etag %q, encoding %q", first.Code, first.Header().Get("ETag"), first.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(first.Body)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	identity := httptest.NewRecorder()
	service.Handler().ServeHTTP(identity, httptest.NewRequest(http.MethodGet, "/api/objects", nil))
	if !bytes.Equal(decompressed, identity.Body.Bytes()) {
		t.Fatalf("gzip and identity representations differ: %q != %q", decompressed, identity.Body.Bytes())
	}
	conditional := httptest.NewRequest(http.MethodGet, "/api/objects", nil)
	conditional.Header.Set("If-None-Match", first.Header().Get("ETag"))
	second := httptest.NewRecorder()
	service.Handler().ServeHTTP(second, conditional)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("conditional response = status %d, body %q", second.Code, second.Body.String())
	}
	if source.objectBuilds != 1 {
		t.Fatalf("object snapshots built %d times, want once", source.objectBuilds)
	}
}

func BenchmarkCachedObjectResponses(b *testing.B) {
	objects := make([]palworld.WorldObject, 8_000)
	for index := range objects {
		objects[index] = palworld.WorldObject{
			ID: fmt.Sprintf("object:%d", index), Kind: "workers", Name: "Assigned Pal",
			BaseID: fmt.Sprintf("base:%d", index/20), X: float64(index), Y: -float64(index), Map: "palpagos",
		}
	}
	source := &revisionSnapshotSource{objects: objects}
	service := &Server{settings: serverSettings{worldDataEnabled: true}, source: source}
	service.handler = service.securityHeaders(service.routes())
	warm := httptest.NewRecorder()
	service.Handler().ServeHTTP(warm, httptest.NewRequest(http.MethodGet, "/api/objects", nil))
	etag := warm.Header().Get("ETag")

	for _, benchmark := range []struct {
		name     string
		encoding string
		etag     string
	}{
		{name: "identity"},
		{name: "gzip", encoding: "gzip"},
		{name: "not-modified", etag: etag},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				request := httptest.NewRequest(http.MethodGet, "/api/objects", nil)
				request.Header.Set("Accept-Encoding", benchmark.encoding)
				request.Header.Set("If-None-Match", benchmark.etag)
				response := httptest.NewRecorder()
				service.Handler().ServeHTTP(response, request)
			}
		})
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

func TestPublicConfigUsesManifestAssetHash(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("config response = status %d, body %s", response.Code, response.Body.String())
	}
	if strings.Contains(body, `"demoMode"`) {
		t.Fatalf("config response exposes backend-only demo mode: %s", body)
	}
	if !strings.Contains(body, `/assets/map/palpagos.jpg?v=`) || strings.Contains(body, `?v=8192`) {
		t.Fatalf("config response does not use manifest asset hash: %s", body)
	}
	if !strings.Contains(body, `"catalogueUrl":"/api/catalogue?v=`+service.worldCatalogue.ContentHash+`"`) {
		t.Fatalf("config response does not use world catalogue content hash: %s", body)
	}
	if !strings.Contains(body, `"kind":"alpha-pals"`) || !strings.Contains(body, `"name":"Penking"`) || !strings.Contains(body, `"id":"landmark:tower:REGION_Grass_Boss"`) {
		t.Fatalf("config response does not include embedded encounter landmarks: %s", body)
	}
	if !strings.Contains(body, `"landmarkCatalogue":{"gameVersion":"1.0.3.101283","generator":"palworld-asset-exporter/4"`) {
		t.Fatalf("config response does not expose landmark provenance: %s", body)
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
	if allowed.Header().Get("Cache-Control") != "no-cache" || allowed.Header().Get("ETag") == "" {
		t.Fatalf("unversioned map cache headers = cache %q, etag %q", allowed.Header().Get("Cache-Control"), allowed.Header().Get("ETag"))
	}

	versioned := httptest.NewRecorder()
	service.Handler().ServeHTTP(versioned, httptest.NewRequest(http.MethodGet, service.layers[0].ImageURL, nil))
	if versioned.Code != http.StatusOK || !strings.Contains(versioned.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("versioned map response = status %d, cache %q", versioned.Code, versioned.Header().Get("Cache-Control"))
	}

	tileTemplate := service.layers[0].TilePyramid.URLTemplate
	tileURL := strings.NewReplacer("{size}", strconv.Itoa(service.layers[0].TilePyramid.Levels[0]), "{x}", "0", "{y}", "0").Replace(tileTemplate)
	tile := httptest.NewRecorder()
	service.Handler().ServeHTTP(tile, httptest.NewRequest(http.MethodGet, tileURL, nil))
	if tile.Code != http.StatusOK || tile.Header().Get("Content-Type") != "image/webp" || !strings.Contains(tile.Header().Get("Cache-Control"), "immutable") || tile.Header().Get("ETag") == "" {
		t.Fatalf("tile response = status %d, type %q, cache %q, etag %q", tile.Code, tile.Header().Get("Content-Type"), tile.Header().Get("Cache-Control"), tile.Header().Get("ETag"))
	}

	wrongVersion := httptest.NewRecorder()
	service.Handler().ServeHTTP(wrongVersion, httptest.NewRequest(http.MethodGet, "/assets/map/palpagos.jpg?v=wrong", nil))
	if wrongVersion.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("wrong-version map cache policy = %q", wrongVersion.Header().Get("Cache-Control"))
	}

	rangeRequest := httptest.NewRequest(http.MethodGet, service.layers[0].ImageURL, nil)
	rangeRequest.Header.Set("Range", "bytes=0-15")
	ranged := httptest.NewRecorder()
	service.Handler().ServeHTTP(ranged, rangeRequest)
	if ranged.Code != http.StatusPartialContent || ranged.Body.Len() != 16 || ranged.Header().Get("Content-Range") == "" {
		t.Fatalf("range response = status %d, size %d, content-range %q", ranged.Code, ranged.Body.Len(), ranged.Header().Get("Content-Range"))
	}

	notModifiedRequest := httptest.NewRequest(http.MethodGet, service.layers[0].ImageURL, nil)
	notModifiedRequest.Header.Set("If-None-Match", versioned.Header().Get("ETag"))
	notModified := httptest.NewRecorder()
	service.Handler().ServeHTTP(notModified, notModifiedRequest)
	if notModified.Code != http.StatusNotModified {
		t.Fatalf("conditional map status = %d", notModified.Code)
	}
	for _, path := range []string{"/assets/map/secret.txt", "/assets/map/"} {
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d", path, response.Code)
		}
	}
}

func TestServerServesVersionedWorldCatalogue(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if service.catalogueURL != "/api/catalogue?v="+service.worldCatalogue.ContentHash {
		t.Fatalf("catalogue URL = %q", service.catalogueURL)
	}

	unversioned := httptest.NewRecorder()
	service.Handler().ServeHTTP(unversioned, httptest.NewRequest(http.MethodGet, "/api/catalogue", nil))
	if unversioned.Code != http.StatusOK ||
		unversioned.Header().Get("Content-Type") != "application/json" ||
		unversioned.Header().Get("Cache-Control") != "no-cache" ||
		unversioned.Header().Get("ETag") == "" {
		t.Fatalf(
			"unversioned catalogue = status %d, type %q, cache %q, etag %q",
			unversioned.Code,
			unversioned.Header().Get("Content-Type"),
			unversioned.Header().Get("Cache-Control"),
			unversioned.Header().Get("ETag"),
		)
	}
	var public map[string]json.RawMessage
	if err := json.Unmarshal(unversioned.Body.Bytes(), &public); err != nil {
		t.Fatal(err)
	}
	if len(public) != 4 ||
		public["gameVersion"] == nil ||
		public["generator"] == nil ||
		public["decoder"] == nil ||
		public["locations"] == nil {
		t.Fatalf("catalogue response fields = %v", public)
	}
	if !strings.Contains(unversioned.Body.String(), `"id":"catalogue:`) ||
		!strings.Contains(unversioned.Body.String(), `"kind":"bounties"`) ||
		strings.Contains(unversioned.Body.String(), `"sourcePackage"`) {
		t.Fatalf("unexpected catalogue projection: %s", unversioned.Body.String())
	}

	versioned := httptest.NewRecorder()
	service.Handler().ServeHTTP(versioned, httptest.NewRequest(http.MethodGet, service.catalogueURL, nil))
	if versioned.Code != http.StatusOK ||
		!strings.Contains(versioned.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf(
			"versioned catalogue = status %d, cache %q",
			versioned.Code, versioned.Header().Get("Cache-Control"),
		)
	}

	wrongVersion := httptest.NewRecorder()
	service.Handler().ServeHTTP(
		wrongVersion,
		httptest.NewRequest(http.MethodGet, "/api/catalogue?v=wrong", nil),
	)
	if wrongVersion.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("wrong-version catalogue cache policy = %q", wrongVersion.Header().Get("Cache-Control"))
	}

	notModifiedRequest := httptest.NewRequest(http.MethodGet, service.catalogueURL, nil)
	notModifiedRequest.Header.Set("If-None-Match", versioned.Header().Get("ETag"))
	notModified := httptest.NewRecorder()
	service.Handler().ServeHTTP(notModified, notModifiedRequest)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("conditional catalogue = status %d, body %q", notModified.Code, notModified.Body.String())
	}

	gzipRequest := httptest.NewRequest(http.MethodGet, service.catalogueURL, nil)
	gzipRequest.Header.Set("Accept-Encoding", "gzip")
	compressed := httptest.NewRecorder()
	service.Handler().ServeHTTP(compressed, gzipRequest)
	if compressed.Header().Get("Content-Encoding") != "gzip" ||
		compressed.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf(
			"catalogue gzip headers = encoding %q, vary %q",
			compressed.Header().Get("Content-Encoding"), compressed.Header().Get("Vary"),
		)
	}
	reader, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decompressed), `"locations":[`) {
		t.Fatalf("decompressed catalogue = %s", decompressed)
	}
}

func TestServerServesViteFrontendAssets(t *testing.T) {
	service, err := New(testConfig(), fixedSnapshot{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	index := httptest.NewRecorder()
	service.Handler().ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("index response = status %d, body %s", index.Code, index.Body.String())
	}
	assetPath := regexp.MustCompile(`/assets/[^"']+\.js`).FindString(index.Body.String())
	if assetPath == "" {
		t.Fatalf("index has no JavaScript asset: %s", index.Body.String())
	}
	asset := httptest.NewRecorder()
	service.Handler().ServeHTTP(asset, httptest.NewRequest(http.MethodGet, assetPath, nil))
	if asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Content-Type"), "javascript") {
		t.Fatalf("asset response = status %d, type %q", asset.Code, asset.Header().Get("Content-Type"))
	}
	if !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("built asset cache policy = %q", asset.Header().Get("Cache-Control"))
	}

	favicon := httptest.NewRecorder()
	service.Handler().ServeHTTP(favicon, httptest.NewRequest(http.MethodGet, "/assets/favicon.svg", nil))
	if favicon.Code != http.StatusOK || favicon.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(favicon.Body.String(), "<svg") {
		t.Fatalf("favicon response = status %d, type %q, body %q", favicon.Code, favicon.Header().Get("Content-Type"), favicon.Body.String())
	}
}

func TestServerSelectsPrecompressedHashedAssets(t *testing.T) {
	service := &Server{assets: fstest.MapFS{
		"assets/app-deadbeef.js":    &fstest.MapFile{Data: []byte("identity")},
		"assets/app-deadbeef.js.br": &fstest.MapFile{Data: []byte("brotli")},
		"assets/app-deadbeef.js.gz": &fstest.MapFile{Data: []byte("gzip")},
	}}
	service.handler = service.securityHeaders(service.routes())
	for _, test := range []struct {
		accept, encoding, body string
	}{
		{accept: "br, gzip", encoding: "br", body: "brotli"},
		{accept: "br;q=0.1, gzip;q=1", encoding: "gzip", body: "gzip"},
		{accept: "identity", encoding: "", body: "identity"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/assets/app-deadbeef.js", nil)
		request.Header.Set("Accept-Encoding", test.accept)
		response := httptest.NewRecorder()
		service.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Header().Get("Content-Encoding") != test.encoding || response.Header().Get("Vary") != "Accept-Encoding" || response.Body.String() != test.body {
			t.Fatalf("accept %q = status %d, encoding %q, vary %q, body %q", test.accept, response.Code, response.Header().Get("Content-Encoding"), response.Header().Get("Vary"), response.Body.String())
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
	if !strings.Contains(response.Body.String(), `"players":[]`) || !strings.Contains(response.Body.String(), `"objects":[]`) {
		t.Fatalf("empty arrays were serialized as null: %s", response.Body.String())
	}
}

func TestLoadMapLayersUsesManifestArtworkForShippedLayers(t *testing.T) {
	maps := validTestMapFS(t)
	layers, files, err := loadMapLayers(maps)
	if err != nil {
		t.Fatalf("loadMapLayers() error = %v", err)
	}
	if len(layers) != 2 || layers[0].ID != "palpagos" || !strings.Contains(layers[0].ImageURL, "/assets/map/palpagos-test.jpg?v=") || files["palpagos-test.jpg"].sha256 == "" {
		t.Fatalf("layers = %#v, files = %#v", layers, files)
	}

	service := &Server{
		settings: serverSettings{worldDataEnabled: true}, source: fixedSnapshot{},
		assets: fstest.MapFS{}, maps: maps, mapFiles: files, layers: layers,
	}
	service.handler = service.securityHeaders(service.routes())
	started := time.Now()
	response := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, layers[0].ImageURL, nil))
	if response.Code != http.StatusOK || response.Body.String() != "test map artwork" {
		t.Fatalf("map response = status %d, body %q", response.Code, response.Body.String())
	}
	if response.deadline.Before(started.Add(mapWriteTimeout - time.Second)) {
		t.Fatalf("map write deadline = %s, want approximately %s", response.deadline, started.Add(mapWriteTimeout))
	}
}

func TestLoadMapLayersRejectsInvalidMetadataAndArtwork(t *testing.T) {
	valid := validTestMapFS(t)
	manifestData, err := fs.ReadFile(valid, "manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest mapManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*mapManifest, fstest.MapFS)
		want   string
	}{
		{name: "hash mismatch", mutate: func(m *mapManifest, _ fstest.MapFS) { m.Layers[0].SHA256 = strings.Repeat("0", 64) }, want: "does not match"},
		{name: "invalid digest", mutate: func(m *mapManifest, _ fstest.MapFS) { m.Layers[0].SHA256 = "not-a-hash" }, want: "invalid SHA-256"},
		{name: "invalid bounds", mutate: func(m *mapManifest, _ fstest.MapFS) { m.Layers[0].Bounds = [4]float64{0, 0, 1, 1} }, want: "invalid embedded map layer"},
		{name: "unknown layer", mutate: func(m *mapManifest, _ fstest.MapFS) { m.Layers[0].ID = "custom" }, want: "invalid embedded map layer"},
		{name: "missing layer", mutate: func(m *mapManifest, _ fstest.MapFS) { m.Layers = m.Layers[:1] }, want: "must contain"},
		{name: "duplicate ID", mutate: func(m *mapManifest, _ fstest.MapFS) {
			m.Layers[1].ID = m.Layers[0].ID
			m.Layers[1].Bounds = m.Layers[0].Bounds
		}, want: "duplicate embedded map layer ID"},
		{name: "duplicate file", mutate: func(m *mapManifest, _ fstest.MapFS) {
			m.Layers[1].File = m.Layers[0].File
			m.Layers[1].SHA256 = m.Layers[0].SHA256
		}, want: "duplicate embedded map layer file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := cloneMapFS(valid)
			copyManifest := manifest
			copyManifest.Layers = append([]mapManifestLayer(nil), manifest.Layers...)
			test.mutate(&copyManifest, files)
			encoded, err := json.Marshal(copyManifest)
			if err != nil {
				t.Fatal(err)
			}
			files["manifest.json"] = &fstest.MapFile{Data: encoded}
			_, _, err = loadMapLayers(files)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("loadMapLayers() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadMapTilePyramidRejectsIntegrityAndRoutingDrift(t *testing.T) {
	tileData := []byte("tile")
	tileDigest := sha256.Sum256(tileData)
	tileHash := hex.EncodeToString(tileDigest[:])
	aggregate := sha256.Sum256([]byte("512/0/0 " + tileHash + "\n"))
	pyramid := &mapManifestTilePyramid{
		TileSize: 512,
		Format:   "webp",
		SHA256:   hex.EncodeToString(aggregate[:]),
		Levels: []mapManifestTileLevel{{Size: 512, Columns: 1, Rows: 1, Tiles: []mapManifestTile{{
			X: 0, Y: 0, File: "palpagos-z512-x0-y0.webp", Bytes: int64(len(tileData)), SHA256: tileHash,
		}}}},
	}
	maps := fstest.MapFS{"palpagos-z512-x0-y0.webp": &fstest.MapFile{Data: tileData}}
	if _, _, err := loadMapTilePyramid(maps, "palpagos", pyramid); err != nil {
		t.Fatalf("valid pyramid = %v", err)
	}

	wrongAggregate := *pyramid
	wrongAggregate.SHA256 = strings.Repeat("0", 64)
	if _, _, err := loadMapTilePyramid(maps, "palpagos", &wrongAggregate); err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("wrong aggregate error = %v", err)
	}

	wrongFilename := *pyramid
	wrongFilename.Levels = append([]mapManifestTileLevel(nil), pyramid.Levels...)
	wrongFilename.Levels[0].Tiles = append([]mapManifestTile(nil), pyramid.Levels[0].Tiles...)
	wrongFilename.Levels[0].Tiles[0].File = "other-z512-x0-y0.webp"
	if _, _, err := loadMapTilePyramid(maps, "palpagos", &wrongFilename); err == nil || !strings.Contains(err.Error(), "invalid tile") {
		t.Fatalf("wrong filename error = %v", err)
	}

	unsorted := *pyramid
	unsorted.Levels = append(append([]mapManifestTileLevel(nil), pyramid.Levels...), pyramid.Levels[0])
	if _, _, err := loadMapTilePyramid(maps, "palpagos", &unsorted); err == nil || !strings.Contains(err.Error(), "invalid tile level") {
		t.Fatalf("unsorted level error = %v", err)
	}
}

func validTestMapFS(t *testing.T) fstest.MapFS {
	t.Helper()
	palpagosArtwork := []byte("test map artwork")
	worldTreeArtwork := []byte("test world tree artwork")
	palpagosDigest := sha256.Sum256(palpagosArtwork)
	worldTreeDigest := sha256.Sum256(worldTreeArtwork)
	manifest := mapManifest{SchemaVersion: 2, Layers: []mapManifestLayer{
		{
			ID: "palpagos", Name: "Palpagos", File: "palpagos-test.jpg",
			Bounds: [4]float64{349400, 724400, -1099400, -724400}, SHA256: hex.EncodeToString(palpagosDigest[:]),
		},
		{
			ID: "world-tree", Name: "World Tree", File: "world-tree-test.jpg",
			Bounds: [4]float64{689148.5, -476400, 347351.5, -818197}, SHA256: hex.EncodeToString(worldTreeDigest[:]),
		},
	}}
	files := fstest.MapFS{
		"palpagos-test.jpg":   &fstest.MapFile{Data: palpagosArtwork},
		"world-tree-test.jpg": &fstest.MapFile{Data: worldTreeArtwork},
	}
	for index := range manifest.Layers {
		layer := &manifest.Layers[index]
		tileName := fmt.Sprintf("%s-z512-x0-y0.webp", layer.ID)
		tileData := []byte("tile for " + layer.ID)
		tileDigest := sha256.Sum256(tileData)
		tileHash := hex.EncodeToString(tileDigest[:])
		aggregate := sha256.Sum256([]byte("512/0/0 " + tileHash + "\n"))
		layer.TilePyramid = &mapManifestTilePyramid{
			TileSize: 512, Format: "webp", SHA256: hex.EncodeToString(aggregate[:]),
			Levels: []mapManifestTileLevel{{Size: 512, Columns: 1, Rows: 1, Tiles: []mapManifestTile{{X: 0, Y: 0, File: tileName, Bytes: int64(len(tileData)), SHA256: tileHash}}}},
		}
		files[tileName] = &fstest.MapFile{Data: tileData}
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["manifest.json"] = &fstest.MapFile{Data: encoded}
	return files
}

func cloneMapFS(source fstest.MapFS) fstest.MapFS {
	result := make(fstest.MapFS, len(source))
	for name, file := range source {
		copy := *file
		copy.Data = append([]byte(nil), file.Data...)
		result[name] = &copy
	}
	return result
}
