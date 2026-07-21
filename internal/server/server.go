package server

import (
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
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	mapassets "github.com/LukeHollandDev/palworld-live-map/assets"
	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/web"
)

type snapshotSource interface {
	Snapshot() palworld.Snapshot
	PlayerSnapshot() palworld.PlayerSnapshot
	ObjectSnapshot() palworld.ObjectSnapshot
}

type Server struct {
	settings serverSettings
	source   snapshotSource
	assets   fs.FS
	maps     fs.FS
	mapFiles map[string]mapFile
	layers   []mapLayer
	handler  http.Handler
}

type serverSettings struct {
	pollInterval      time.Duration
	worldPollInterval time.Duration
	worldDataEnabled  bool
}

type mapFile struct {
	sha256 string
}

const mapWriteTimeout = 2 * time.Minute

type mapLayer struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	ImageURL string     `json:"imageUrl,omitempty"`
	Bounds   [4]float64 `json:"bounds"`
}

type mapManifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	Layers        []mapManifestLayer `json:"layers"`
}

type mapManifestLayer struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	File   string     `json:"file"`
	Bounds [4]float64 `json:"bounds"`
	SHA256 string     `json:"sha256"`
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
	webAssets, err := fs.Sub(web.Assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}
	maps, err := fs.Sub(mapassets.Maps, "map")
	if err != nil {
		return nil, fmt.Errorf("open embedded map assets: %w", err)
	}
	layers, mapFiles, err := loadMapLayers(maps)
	if err != nil {
		return nil, err
	}

	s := &Server{
		settings: serverSettings{
			pollInterval: cfg.PollInterval, worldPollInterval: cfg.WorldPollInterval,
			worldDataEnabled: cfg.WorldDataEnabled,
		},
		source: source, assets: webAssets, maps: maps, mapFiles: mapFiles, layers: layers,
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
	mux.HandleFunc("GET /api/players", s.players)
	mux.HandleFunc("GET /api/objects", s.objects)
	mux.HandleFunc("GET /api/state", s.state)
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
		"layers":              s.layers,
	})
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
	if manifest.SchemaVersion != 1 {
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
		files[source.File] = mapFile{sha256: strings.ToLower(source.SHA256)}
		layers = append(layers, mapLayer{
			ID: source.ID, Name: source.Name, Bounds: source.Bounds,
			ImageURL: fmt.Sprintf("/assets/map/%s?v=%s", url.PathEscape(source.File), strings.ToLower(source.SHA256[:12])),
		})
	}
	return layers, files, nil
}

func validMapFilename(name string) bool {
	return fs.ValidPath(name) && path.Base(name) == name && strings.EqualFold(path.Ext(name), ".jpg")
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
	expected, err := hex.DecodeString(expectedHash)
	if err != nil || len(expected) != sha256.Size {
		return fmt.Errorf("invalid SHA-256 digest")
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

func (s *Server) players(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, s.source.PlayerSnapshot())
}

func (s *Server) objects(w http.ResponseWriter, r *http.Request) {
	snapshot := s.source.ObjectSnapshot()
	writeJSON(w, r, http.StatusOK, objectState{
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

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, s.source.Snapshot())
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
	data, err := fs.ReadFile(s.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func (s *Server) mapAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	asset, ok := s.mapFiles[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("v") == asset.sha256[:12] {
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
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

func acceptsGzip(value string) bool {
	gzipQuality := -1.0
	wildcardQuality := -1.0
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(entry, ";")
		encoding := strings.ToLower(strings.TrimSpace(parts[0]))
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, rawQuality, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(rawQuality), 64)
				if err != nil || parsed < 0 || parsed > 1 {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		switch encoding {
		case "gzip":
			gzipQuality = quality
		case "*":
			wildcardQuality = quality
		}
	}
	if gzipQuality >= 0 {
		return gzipQuality > 0
	}
	return wildcardQuality > 0
}
