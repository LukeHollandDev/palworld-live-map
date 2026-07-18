package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	mapassets "github.com/LukeHollandDev/palworld-live-map/assets"
	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/web"
)

type snapshotSource interface {
	Snapshot() palworld.Snapshot
}

type Server struct {
	cfg     config.Config
	source  snapshotSource
	assets  fs.FS
	maps    fs.FS
	layers  []mapLayer
	handler http.Handler
}

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

type playerState struct {
	Server           palworld.ServerInfo    `json:"server"`
	Metrics          palworld.ServerMetrics `json:"metrics"`
	MetricsAvailable bool                   `json:"metricsAvailable"`
	MetricsUpdatedAt time.Time              `json:"metricsUpdatedAt,omitzero"`
	Connected        bool                   `json:"connected"`
	Stale            bool                   `json:"stale"`
	LastSuccessAt    time.Time              `json:"lastSuccessAt,omitzero"`
	Players          []palworld.Player      `json:"players"`
}

type objectState struct {
	Enabled     bool                   `json:"enabled"`
	Available   bool                   `json:"available"`
	Stale       bool                   `json:"stale"`
	Unsupported bool                   `json:"unsupported"`
	UpdatedAt   time.Time              `json:"updatedAt,omitzero"`
	Objects     []palworld.WorldObject `json:"objects"`
}

func New(cfg config.Config, source snapshotSource) (*Server, error) {
	webAssets, err := fs.Sub(web.Assets, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}
	maps, err := fs.Sub(mapassets.Maps, "map")
	if err != nil {
		return nil, fmt.Errorf("open embedded map assets: %w", err)
	}
	layers, err := loadMapLayers(maps)
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg: cfg, source: source, assets: webAssets, maps: maps, layers: layers,
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
	mux.HandleFunc("GET /app.js", s.static("app.js"))
	mux.HandleFunc("GET /styles.css", s.static("styles.css"))
	mux.HandleFunc("GET /assets/map/palpagos.jpg", s.mapAsset("palpagos.jpg"))
	mux.HandleFunc("GET /assets/map/world-tree.jpg", s.mapAsset("world-tree.jpg"))

	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) publicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"demoMode":            s.cfg.DemoMode,
		"pollIntervalMs":      s.cfg.PollInterval.Milliseconds(),
		"worldPollIntervalMs": s.cfg.WorldPollInterval.Milliseconds(),
		"worldDataEnabled":    s.cfg.WorldDataEnabled,
		"layers":              s.layers,
	})
}

func loadMapLayers(maps fs.FS) ([]mapLayer, error) {
	data, err := fs.ReadFile(maps, "manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded map manifest: %w", err)
	}
	var manifest mapManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode embedded map manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.Layers) == 0 {
		return nil, fmt.Errorf("unsupported or empty embedded map manifest")
	}
	layers := make([]mapLayer, 0, len(manifest.Layers))
	for _, source := range manifest.Layers {
		if source.ID == "" || source.Name == "" || source.File == "" || len(source.SHA256) < 12 {
			return nil, fmt.Errorf("invalid embedded map layer %q", source.ID)
		}
		if _, err := fs.Stat(maps, source.File); err != nil {
			return nil, fmt.Errorf("open artwork for map layer %q: %w", source.ID, err)
		}
		layers = append(layers, mapLayer{
			ID: source.ID, Name: source.Name, Bounds: source.Bounds,
			ImageURL: fmt.Sprintf("/assets/map/%s?v=%s", source.File, source.SHA256[:12]),
		})
	}
	return layers, nil
}

func (s *Server) players(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.source.Snapshot()
	writeJSON(w, http.StatusOK, playerState{
		Server:           snapshot.Server,
		Metrics:          snapshot.Metrics,
		MetricsAvailable: snapshot.MetricsAvailable,
		MetricsUpdatedAt: snapshot.MetricsUpdatedAt,
		Connected:        snapshot.Connected,
		Stale:            snapshot.Stale,
		LastSuccessAt:    snapshot.LastSuccessAt,
		Players:          snapshot.Players,
	})
}

func (s *Server) objects(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.source.Snapshot()
	writeJSON(w, http.StatusOK, objectState{
		Enabled:     s.cfg.WorldDataEnabled,
		Available:   snapshot.ObjectsAvailable,
		Stale:       snapshot.ObjectsStale,
		Unsupported: snapshot.ObjectsUnsupported,
		UpdatedAt:   snapshot.ObjectsUpdatedAt,
		Objects:     snapshot.Objects,
	})
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.source.Snapshot())
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.serveAsset(w, r, "index.html")
}

func (s *Server) static(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.serveAsset(w, r, name)
	}
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

func (s *Server) mapAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(s.maps, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	}
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
