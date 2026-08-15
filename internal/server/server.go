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
	"math"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	mapassets "github.com/LukeHollandDev/palworld-live-map/assets"
	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/landmarks"
	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
	"github.com/LukeHollandDev/palworld-live-map/internal/worldcatalogue"
	"github.com/LukeHollandDev/palworld-live-map/web"
)

type snapshotSource interface {
	Snapshot() palworld.Snapshot
	PlayerSnapshotSince(uint64) (palworld.PlayerSnapshot, uint64, bool)
	ObjectSnapshotSince(uint64) (palworld.ObjectSnapshot, uint64, bool)
}

type Server struct {
	settings           serverSettings
	source             snapshotSource
	assets             fs.FS
	maps               fs.FS
	mapFiles           map[string]mapFile
	layers             []mapLayer
	landmarks          []palworld.WorldObject
	landmarkCatalogue  landmarks.Metadata
	worldCatalogue     worldcatalogue.Catalogue
	claims             *playerclaim.Service
	claimStartLimiter  *claimRequestLimiter
	claimVerifyLimiter *claimRequestLimiter
	claimWork          chan struct{}
	catalogueURL       string
	handler            http.Handler
	playersResponse    cachedJSONResponse
	objectsResponse    cachedJSONResponse
}

type serverSettings struct {
	pollInterval               time.Duration
	worldPollInterval          time.Duration
	worldDataEnabled           bool
	playerClaimsEnabled        bool
	playerClaimsOrigin         string
	playerClaimsHTTP           bool
	playerClaimsTrustedProxies []netip.Prefix
}

type mapFile struct {
	sha256  string
	version string
}

type cachedJSONResponse struct {
	mu          sync.Mutex
	revision    uint64
	initialized bool
	identity    []byte
	gzip        []byte
	etag        string
}

const mapWriteTimeout = 2 * time.Minute

type mapLayer struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	ImageURL    string          `json:"imageUrl,omitempty"`
	Bounds      [4]float64      `json:"bounds"`
	TilePyramid *mapTilePyramid `json:"tilePyramid,omitempty"`
}

type mapTilePyramid struct {
	TileSize    int    `json:"tileSize"`
	Levels      []int  `json:"levels"`
	URLTemplate string `json:"urlTemplate"`
}

type mapManifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	Layers        []mapManifestLayer `json:"layers"`
}

type mapManifestLayer struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	File        string                  `json:"file"`
	Bounds      [4]float64              `json:"bounds"`
	SHA256      string                  `json:"sha256"`
	TilePyramid *mapManifestTilePyramid `json:"tilePyramid"`
}

type mapManifestTilePyramid struct {
	TileSize int                    `json:"tileSize"`
	Format   string                 `json:"format"`
	SHA256   string                 `json:"sha256"`
	Levels   []mapManifestTileLevel `json:"levels"`
}

type mapManifestTileLevel struct {
	Size    int               `json:"size"`
	Columns int               `json:"columns"`
	Rows    int               `json:"rows"`
	Tiles   []mapManifestTile `json:"tiles"`
}

type mapManifestTile struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type objectState struct {
	Enabled     bool                   `json:"enabled"`
	Available   bool                   `json:"available"`
	Stale       bool                   `json:"stale"`
	Unsupported bool                   `json:"unsupported"`
	Truncated   bool                   `json:"truncated"`
	Total       int                    `json:"total"`
	LastError   string                 `json:"lastError,omitempty"`
	UpdatedAt   time.Time              `json:"updatedAt,omitzero"`
	Objects     []palworld.WorldObject `json:"objects"`
}

func New(cfg config.Config, source snapshotSource) (*Server, error) {
	return NewWithClaims(cfg, source, nil)
}

// NewWithClaims enables the private claim endpoints only when the opt-in
// configuration and an initialized claim service are both present.
func NewWithClaims(cfg config.Config, source snapshotSource, claims *playerclaim.Service) (*Server, error) {
	if cfg.PlayerClaimsEnabled != (claims != nil) {
		return nil, fmt.Errorf("player claims configuration and service must be enabled together")
	}
	webAssets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}
	maps, err := fs.Sub(mapassets.Maps, "palworld/maps")
	if err != nil {
		return nil, fmt.Errorf("open embedded map assets: %w", err)
	}
	layers, mapFiles, err := loadMapLayers(maps)
	if err != nil {
		return nil, err
	}
	landmarkCatalogue, err := landmarks.Load(mapassets.Landmarks)
	if err != nil {
		return nil, fmt.Errorf("load embedded landmarks: %w", err)
	}
	worldCatalogue, err := worldcatalogue.Load(mapassets.Catalogue)
	if err != nil {
		return nil, fmt.Errorf("load embedded world catalogue: %w", err)
	}

	s := &Server{
		settings: serverSettings{
			pollInterval: cfg.PollInterval, worldPollInterval: cfg.WorldPollInterval,
			worldDataEnabled: cfg.WorldDataEnabled, playerClaimsEnabled: cfg.PlayerClaimsEnabled,
			playerClaimsOrigin:         cfg.PlayerClaimsOrigin,
			playerClaimsHTTP:           cfg.PlayerClaimsHTTP,
			playerClaimsTrustedProxies: append([]netip.Prefix{}, cfg.PlayerClaimsTrustedProxies...),
		},
		source: source, assets: webAssets, maps: maps, mapFiles: mapFiles, layers: layers,
		landmarks: landmarkCatalogue.Locations, landmarkCatalogue: landmarkCatalogue.Metadata,
		worldCatalogue: worldCatalogue, claims: claims,
		catalogueURL: "/api/catalogue?v=" + worldCatalogue.ContentHash,
	}
	if claims != nil {
		s.claimStartLimiter = newClaimRequestLimiter(5, 10*time.Minute, 100, time.Minute)
		s.claimVerifyLimiter = newClaimRequestLimiter(60, 10*time.Minute, 600, time.Minute)
		s.claimWork = make(chan struct{}, 1)
	}
	s.handler = s.securityHeaders(s.routes())
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /-/health", s.health)
	mux.HandleFunc("GET /api/config", s.publicConfig)
	mux.HandleFunc("GET /api/catalogue", s.catalogue)
	mux.HandleFunc("GET /api/players", s.players)
	mux.HandleFunc("GET /api/objects", s.objects)
	mux.HandleFunc("GET /api/state", s.state)
	if s.claims != nil {
		mux.HandleFunc("POST /api/player-claims", s.startPlayerClaim)
		mux.HandleFunc("POST /api/player-claims/verify", s.verifyPlayerClaim)
		mux.HandleFunc("GET /api/me", s.claimSession)
		mux.HandleFunc("GET /api/me/progress", s.claimProgress)
		mux.HandleFunc("POST /api/logout", s.logoutClaimSession)
	}
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /assets/{path...}", s.webAsset)
	mux.HandleFunc("GET /assets/map/{file}", s.mapAsset)

	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]any{
		"pollIntervalMs":      s.settings.pollInterval.Milliseconds(),
		"worldPollIntervalMs": s.settings.worldPollInterval.Milliseconds(),
		"worldDataEnabled":    s.settings.worldDataEnabled,
		"playerClaimsEnabled": s.settings.playerClaimsEnabled,
		"layers":              s.layers,
		"catalogueUrl":        s.catalogueURL,
		"landmarks":           s.landmarks,
		"landmarkCatalogue":   s.landmarkCatalogue,
	})
}

func (s *Server) catalogue(w http.ResponseWriter, r *http.Request) {
	cacheControl := "no-cache"
	if r.URL.Query().Get("v") == s.worldCatalogue.ContentHash {
		cacheControl = "public, max-age=31536000, immutable"
	}
	etag := fmt.Sprintf(`W/"%s"`, s.worldCatalogue.ContentHash)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("Vary", "Accept-Encoding")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSONWithCache(w, r, http.StatusOK, s.worldCatalogue, cacheControl)
}

func loadMapLayers(maps fs.FS) ([]mapLayer, map[string]mapFile, error) {
	data, err := fs.ReadFile(maps, "manifest.json")
	if err != nil {
		return nil, nil, fmt.Errorf("read embedded map manifest: %w", err)
	}
	var manifest mapManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, nil, fmt.Errorf("decode embedded map manifest: %w", err)
	}
	if manifest.SchemaVersion != 2 {
		return nil, nil, fmt.Errorf("unsupported embedded map manifest schema")
	}
	if len(manifest.Layers) != mapdata.LayerCount {
		return nil, nil, fmt.Errorf("embedded map manifest must contain %d supported layers", mapdata.LayerCount)
	}
	layers := make([]mapLayer, 0, len(manifest.Layers))
	files := make(map[string]mapFile, len(manifest.Layers))
	ids := make(map[string]struct{}, len(manifest.Layers))
	for _, source := range manifest.Layers {
		if source.ID == "" || source.Name == "" || !validMapFilename(source.File) || !validBounds(source.Bounds) ||
			!mapdata.KnownLayer(source.ID, source.Bounds) {
			return nil, nil, fmt.Errorf("invalid embedded map layer %q", source.ID)
		}
		if _, exists := ids[source.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate embedded map layer ID %q", source.ID)
		}
		if _, exists := files[source.File]; exists {
			return nil, nil, fmt.Errorf("duplicate embedded map layer file %q", source.File)
		}
		if err := verifyMapFile(maps, source.File, source.SHA256); err != nil {
			return nil, nil, fmt.Errorf("verify artwork for map layer %q: %w", source.ID, err)
		}
		ids[source.ID] = struct{}{}
		files[source.File] = mapFile{sha256: strings.ToLower(source.SHA256), version: strings.ToLower(source.SHA256[:12])}
		layer := mapLayer{
			ID: source.ID, Name: source.Name, Bounds: source.Bounds,
			ImageURL: fmt.Sprintf("/assets/map/%s?v=%s", url.PathEscape(source.File), strings.ToLower(source.SHA256[:12])),
		}
		if source.TilePyramid == nil {
			return nil, nil, fmt.Errorf("embedded map layer %q has no tile pyramid", source.ID)
		}
		pyramid, tileFiles, err := loadMapTilePyramid(maps, source.ID, source.TilePyramid)
		if err != nil {
			return nil, nil, fmt.Errorf("verify tile pyramid for map layer %q: %w", source.ID, err)
		}
		for name, tile := range tileFiles {
			if _, exists := files[name]; exists {
				return nil, nil, fmt.Errorf("duplicate embedded map layer file %q", name)
			}
			files[name] = tile
		}
		layer.TilePyramid = pyramid
		layers = append(layers, layer)
	}
	return layers, files, nil
}

func loadMapTilePyramid(maps fs.FS, layerID string, source *mapManifestTilePyramid) (*mapTilePyramid, map[string]mapFile, error) {
	if source.TileSize <= 0 || !strings.EqualFold(source.Format, "webp") || len(source.Levels) == 0 {
		return nil, nil, fmt.Errorf("invalid tile pyramid metadata")
	}
	if _, err := decodeSHA256(source.SHA256); err != nil {
		return nil, nil, fmt.Errorf("invalid aggregate SHA-256 digest")
	}
	version := strings.ToLower(source.SHA256[:12])
	levels := make([]int, 0, len(source.Levels))
	files := make(map[string]mapFile)
	aggregate := sha256.New()
	previousSize := 0
	for _, level := range source.Levels {
		if level.Size <= previousSize || level.Size%source.TileSize != 0 || level.Columns != level.Size/source.TileSize || level.Rows != level.Size/source.TileSize || len(level.Tiles) != level.Columns*level.Rows {
			return nil, nil, fmt.Errorf("invalid tile level %d", level.Size)
		}
		previousSize = level.Size
		levels = append(levels, level.Size)
		seen := make(map[[2]int]struct{}, len(level.Tiles))
		for index, tile := range level.Tiles {
			coordinate := [2]int{tile.X, tile.Y}
			expectedName := fmt.Sprintf("%s-z%d-x%d-y%d.webp", layerID, level.Size, tile.X, tile.Y)
			if tile.X != index%level.Columns || tile.Y != index/level.Columns || tile.Bytes <= 0 || !validMapTileFilename(tile.File) || tile.File != expectedName {
				return nil, nil, fmt.Errorf("invalid tile at level %d (%d,%d)", level.Size, tile.X, tile.Y)
			}
			if _, duplicate := seen[coordinate]; duplicate {
				return nil, nil, fmt.Errorf("duplicate tile at level %d (%d,%d)", level.Size, tile.X, tile.Y)
			}
			seen[coordinate] = struct{}{}
			if err := verifyMapFile(maps, tile.File, tile.SHA256); err != nil {
				return nil, nil, fmt.Errorf("verify tile %q: %w", tile.File, err)
			}
			info, err := fs.Stat(maps, tile.File)
			if err != nil || info.Size() != tile.Bytes {
				return nil, nil, fmt.Errorf("tile %q byte size does not match manifest", tile.File)
			}
			files[tile.File] = mapFile{sha256: strings.ToLower(tile.SHA256), version: version}
			_, _ = fmt.Fprintf(aggregate, "%d/%d/%d %s\n", level.Size, tile.X, tile.Y, strings.ToLower(tile.SHA256))
		}
	}
	if !strings.EqualFold(hex.EncodeToString(aggregate.Sum(nil)), source.SHA256) {
		return nil, nil, fmt.Errorf("aggregate SHA-256 digest does not match tile manifest")
	}
	return &mapTilePyramid{
		TileSize:    source.TileSize,
		Levels:      levels,
		URLTemplate: fmt.Sprintf("/assets/map/%s-z{size}-x{x}-y{y}.webp?v=%s", url.PathEscape(layerID), version),
	}, files, nil
}

func validMapFilename(name string) bool {
	return fs.ValidPath(name) && path.Base(name) == name && strings.EqualFold(path.Ext(name), ".jpg")
}

func validMapTileFilename(name string) bool {
	return fs.ValidPath(name) && path.Base(name) == name && strings.EqualFold(path.Ext(name), ".webp")
}

func validBounds(bounds [4]float64) bool {
	for _, value := range bounds {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	maxX, maxY, minX, minY := bounds[0], bounds[1], bounds[2], bounds[3]
	return maxX > minX && maxY > minY
}

func verifyMapFile(maps fs.FS, name, expectedHash string) error {
	_, err := decodeSHA256(expectedHash)
	if err != nil {
		return err
	}
	file, err := maps.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("map artwork is not a non-empty regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		return fmt.Errorf("SHA-256 digest does not match manifest")
	}
	return nil
}

func decodeSHA256(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("invalid SHA-256 digest")
	}
	return decoded, nil
}

func (s *Server) players(w http.ResponseWriter, r *http.Request) {
	s.playersResponse.mu.Lock()
	snapshot, revision, changed := s.source.PlayerSnapshotSince(s.playersResponse.revision)
	if changed || !s.playersResponse.initialized {
		s.playersResponse.update(revision, snapshot)
	}
	response := s.playersResponse.snapshot()
	s.playersResponse.mu.Unlock()
	serveCachedJSON(w, r, response)
}

func (s *Server) objects(w http.ResponseWriter, r *http.Request) {
	s.objectsResponse.mu.Lock()
	snapshot, revision, changed := s.source.ObjectSnapshotSince(s.objectsResponse.revision)
	if changed || !s.objectsResponse.initialized {
		s.objectsResponse.update(revision, objectState{
			Enabled:     s.settings.worldDataEnabled,
			Available:   snapshot.Available,
			Stale:       snapshot.Stale,
			Unsupported: snapshot.Unsupported,
			Truncated:   snapshot.Truncated,
			Total:       snapshot.Total,
			LastError:   snapshot.LastError,
			UpdatedAt:   snapshot.UpdatedAt,
			Objects:     snapshot.Objects,
		})
	}
	response := s.objectsResponse.snapshot()
	s.objectsResponse.mu.Unlock()
	serveCachedJSON(w, r, response)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	snapshot := s.source.Snapshot()
	writeJSON(w, r, http.StatusOK, snapshot)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, "index.html")
}

func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func (s *Server) webAsset(w http.ResponseWriter, r *http.Request) {
	name := "assets/" + strings.TrimPrefix(r.PathValue("path"), "/")
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	encoding := preferredStaticEncoding(r.Header.Get("Accept-Encoding"))
	servedName := name
	if encoding != "" {
		candidate := name + "." + encoding
		if _, err := fs.Stat(s.assets, candidate); err == nil {
			servedName = candidate
		} else {
			encoding = ""
		}
	}
	data, err := fs.ReadFile(s.assets, servedName)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Vary", "Accept-Encoding")
	if encoding == "br" {
		w.Header().Set("Content-Encoding", "br")
	} else if encoding == "gz" {
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func preferredStaticEncoding(value string) string {
	brQuality := encodingQuality(value, "br")
	gzipQuality := encodingQuality(value, "gzip")
	if brQuality > 0 && brQuality >= gzipQuality {
		return "br"
	}
	if gzipQuality > 0 {
		return "gz"
	}
	return ""
}

func acceptsEncoding(value, requested string) bool {
	return encodingQuality(value, requested) > 0
}

func encodingQuality(value, requested string) float64 {
	quality := -1.0
	wildcardQuality := -1.0
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(entry, ";")
		encoding := strings.ToLower(strings.TrimSpace(parts[0]))
		entryQuality := 1.0
		for _, parameter := range parts[1:] {
			key, rawQuality, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(rawQuality), 64)
				if err != nil || parsed < 0 || parsed > 1 {
					entryQuality = 0
				} else {
					entryQuality = parsed
				}
			}
		}
		if encoding == requested {
			quality = entryQuality
		} else if encoding == "*" {
			wildcardQuality = entryQuality
		}
	}
	if quality >= 0 {
		return quality
	}
	return wildcardQuality
}

func (s *Server) mapAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	asset, ok := s.mapFiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("v") == asset.version {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", asset.sha256))
	// Keep the short server-wide write deadline for API responses, but allow
	// slower clients enough time to stream the largest embedded map image.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(mapWriteTimeout))
	http.ServeFileFS(w, r, s.maps, name)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	writeJSONWithCache(w, r, status, value, "no-store")
}

func writeJSONWithCache(w http.ResponseWriter, r *http.Request, status int, value any, cacheControl string) {
	w.Header().Set("Content-Type", "application/json")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.Header().Set("Vary", "Accept-Encoding")
	var writer io.Writer = w
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		compressed, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err == nil {
			w.Header().Set("Content-Encoding", "gzip")
			defer compressed.Close()
			writer = compressed
		}
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

type immutableJSONResponse struct {
	identity []byte
	gzip     []byte
	etag     string
}

func (c *cachedJSONResponse) update(revision uint64, value any) {
	identity, err := json.Marshal(value)
	if err != nil {
		identity = []byte("null")
	}
	identity = append(identity, '\n')
	var compressed bytes.Buffer
	if writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed); err == nil {
		_, _ = writer.Write(identity)
		_ = writer.Close()
	}
	digest := sha256.Sum256(identity)
	c.revision = revision
	c.initialized = true
	c.identity = identity
	c.gzip = compressed.Bytes()
	c.etag = fmt.Sprintf(`W/"%s"`, hex.EncodeToString(digest[:]))
}

func (c *cachedJSONResponse) snapshot() immutableJSONResponse {
	return immutableJSONResponse{identity: c.identity, gzip: c.gzip, etag: c.etag}
}

func serveCachedJSON(w http.ResponseWriter, r *http.Request, response immutableJSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("ETag", response.etag)
	if matchesETag(r.Header.Get("If-None-Match"), response.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := response.identity
	if acceptsGzip(r.Header.Get("Accept-Encoding")) && len(response.gzip) > 0 {
		w.Header().Set("Content-Encoding", "gzip")
		body = response.gzip
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func matchesETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}

func acceptsGzip(value string) bool {
	return acceptsEncoding(value, "gzip")
}
