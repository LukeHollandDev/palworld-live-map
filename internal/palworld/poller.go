package palworld

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type Snapshot struct {
	Server             ServerInfo    `json:"server"`
	Connected          bool          `json:"connected"`
	Stale              bool          `json:"stale"`
	LastSuccessAt      time.Time     `json:"lastSuccessAt,omitzero"`
	Players            []Player      `json:"players"`
	Metrics            ServerMetrics `json:"metrics"`
	MetricsAvailable   bool          `json:"metricsAvailable"`
	MetricsUpdatedAt   time.Time     `json:"metricsUpdatedAt,omitzero"`
	ObjectsAvailable   bool          `json:"objectsAvailable"`
	ObjectsStale       bool          `json:"objectsStale"`
	ObjectsUnsupported bool          `json:"objectsUnsupported"`
	ObjectsUpdatedAt   time.Time     `json:"objectsUpdatedAt,omitzero"`
	Objects            []WorldObject `json:"objects"`
}

type Source interface {
	Info(context.Context) (ServerInfo, error)
	Players(context.Context) ([]Player, error)
	Metrics(context.Context) (ServerMetrics, error)
	WorldObjects(context.Context) ([]WorldObject, error)
}

type Poller struct {
	source         Source
	playerEvery    time.Duration
	worldEvery     time.Duration
	worldEnabled   bool
	logger         *slog.Logger
	unsupportedLog bool

	mu       sync.RWMutex
	snapshot Snapshot
}

func NewPoller(source Source, playerEvery, worldEvery time.Duration, worldEnabled bool, logger *slog.Logger) *Poller {
	return &Poller{
		source: source, playerEvery: playerEvery, worldEvery: worldEvery,
		worldEnabled: worldEnabled, logger: logger,
		snapshot: Snapshot{Players: []Player{}, Objects: []WorldObject{}},
	}
}

func (p *Poller) Run(ctx context.Context) {
	var workers sync.WaitGroup
	workers.Add(3)
	go func() {
		defer workers.Done()
		p.runInfo(ctx)
	}()
	go func() {
		defer workers.Done()
		p.runPlayers(ctx)
	}()
	go func() {
		defer workers.Done()
		p.runMetrics(ctx)
	}()
	if p.worldEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			p.runWorld(ctx)
		}()
	}
	workers.Wait()
}

func (p *Poller) runInfo(ctx context.Context) {
	p.refreshInfo(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshInfo(ctx)
		}
	}
}

func (p *Poller) runPlayers(ctx context.Context) {
	p.refreshPlayers(ctx)
	ticker := time.NewTicker(p.playerEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshPlayers(ctx)
		}
	}
}

func (p *Poller) runMetrics(ctx context.Context) {
	p.refreshMetrics(ctx)
	ticker := time.NewTicker(p.playerEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshMetrics(ctx)
		}
	}
}

func (p *Poller) runWorld(ctx context.Context) {
	p.refreshWorld(ctx)
	ticker := time.NewTicker(p.worldEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.refreshWorld(ctx)
		}
	}
}

func (p *Poller) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := p.snapshot
	result.Players = append([]Player(nil), p.snapshot.Players...)
	result.Objects = append([]WorldObject(nil), p.snapshot.Objects...)
	return result
}

func (p *Poller) refreshInfo(ctx context.Context) {
	info, err := p.source.Info(ctx)
	if err != nil {
		p.logger.Warn("Palworld server-info refresh failed", "error", err)
		return
	}
	p.mu.Lock()
	p.snapshot.Server = info
	p.mu.Unlock()
}

func (p *Poller) refreshPlayers(ctx context.Context) {
	players, err := p.source.Players(ctx)
	if err != nil {
		p.mu.Lock()
		p.snapshot.Connected = false
		p.snapshot.Stale = !p.snapshot.LastSuccessAt.IsZero()
		p.mu.Unlock()
		p.logger.Warn("Palworld player refresh failed", "error", err)
		return
	}
	p.mu.Lock()
	p.snapshot.Connected = true
	p.snapshot.Stale = false
	p.snapshot.LastSuccessAt = time.Now().UTC()
	p.snapshot.Players = append([]Player(nil), players...)
	p.mu.Unlock()
}

func (p *Poller) refreshMetrics(ctx context.Context) {
	metrics, err := p.source.Metrics(ctx)
	if err != nil {
		p.logger.Warn("Palworld server-metrics refresh failed", "error", err)
		return
	}
	p.mu.Lock()
	p.snapshot.Metrics = metrics
	p.snapshot.MetricsAvailable = true
	p.snapshot.MetricsUpdatedAt = time.Now().UTC()
	p.mu.Unlock()
}

func (p *Poller) refreshWorld(ctx context.Context) {
	objects, err := p.source.WorldObjects(ctx)
	if err != nil {
		var statusError *HTTPStatusError
		unsupported := errors.As(err, &statusError) && statusError.Status == http.StatusNotFound
		p.mu.Lock()
		p.snapshot.ObjectsStale = p.snapshot.ObjectsAvailable
		if unsupported && !p.snapshot.ObjectsAvailable {
			p.snapshot.ObjectsUnsupported = true
		}
		p.mu.Unlock()
		if unsupported {
			if !p.unsupportedLog {
				p.logger.Info("Palworld game-data API is disabled; enable ENABLE_GAMEDATA_API on the game server")
				p.unsupportedLog = true
			}
			return
		}
		p.logger.Warn("Palworld world-object refresh failed", "error", err)
		return
	}
	p.unsupportedLog = false
	p.mu.Lock()
	p.snapshot.ObjectsAvailable = true
	p.snapshot.ObjectsStale = false
	p.snapshot.ObjectsUnsupported = false
	p.snapshot.ObjectsUpdatedAt = time.Now().UTC()
	p.snapshot.Objects = append([]WorldObject(nil), objects...)
	p.mu.Unlock()
}
