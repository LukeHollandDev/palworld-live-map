package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"

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
	handler http.Handler
}

type mapLayer struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	ImageURL string     `json:"imageUrl,omitempty"`
	Bounds   [4]float64 `json:"bounds"`
}

func New(cfg config.Config, source snapshotSource) (*Server, error) {
	assets, err := fs.Sub(web.Assets, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded web assets: %w", err)
	}

	s := &Server{
		cfg: cfg, source: source, assets: assets,
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
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /app.js", s.static("app.js"))
	mux.HandleFunc("GET /styles.css", s.static("styles.css"))

	if s.cfg.MapAssetDir != "" {
		mux.HandleFunc("GET /map-assets/palpagos.webp", s.mapAsset("palpagos.webp"))
		mux.HandleFunc("GET /map-assets/world-tree.webp", s.mapAsset("world-tree.webp"))
	}

	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) publicConfig(w http.ResponseWriter, _ *http.Request) {
	layers := []mapLayer{
		{
			ID:       "palpagos",
			Name:     "Palpagos",
			ImageURL: s.mapImage("palpagos.webp"),
			Bounds:   [4]float64{349400, 724400, -1099400, -724400},
		},
		{
			ID:       "world-tree",
			Name:     "World Tree",
			ImageURL: s.mapImage("world-tree.webp"),
			Bounds:   [4]float64{689148.5, -476400, 347351.5, -818197},
		},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":          s.cfg.SiteTitle,
		"pollIntervalMs": s.cfg.PollInterval.Milliseconds(),
		"layers":         layers,
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

func (s *Server) mapImage(name string) string {
	if s.cfg.MapAssetDir == "" {
		return ""
	}
	info, err := os.Stat(filepath.Join(s.cfg.MapAssetDir, name))
	if err != nil || info.IsDir() {
		return ""
	}
	return "/map-assets/" + name
}

func (s *Server) mapAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(s.cfg.MapAssetDir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, path)
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
