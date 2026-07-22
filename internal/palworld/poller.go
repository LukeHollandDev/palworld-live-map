package palworld

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	Server             ServerInfo    `json:"server"`
	Connected          bool          `json:"connected"`
	Stale              bool          `json:"stale"`
	LastSuccessAt      time.Time     `json:"lastSuccessAt,omitzero"`
	Players            []Player      `json:"players"`
	SaveEnabled        bool          `json:"saveEnabled"`
	SaveAvailable      bool          `json:"saveAvailable"`
	SaveStale          bool          `json:"saveStale"`
	SaveUpdatedAt      time.Time     `json:"saveUpdatedAt,omitzero"`
	SaveSnapshotAt     time.Time     `json:"saveSnapshotAt,omitzero"`
	SaveLastError      string        `json:"saveLastError,omitempty"`
	Metrics            ServerMetrics `json:"metrics"`
	MetricsAvailable   bool          `json:"metricsAvailable"`
	MetricsStale       bool          `json:"metricsStale"`
	MetricsUpdatedAt   time.Time     `json:"metricsUpdatedAt,omitzero"`
	ObjectsAvailable   bool          `json:"objectsAvailable"`
	ObjectsStale       bool          `json:"objectsStale"`
	ObjectsUnsupported bool          `json:"objectsUnsupported"`
	ObjectsTruncated   bool          `json:"objectsTruncated"`
	ObjectsTotal       int           `json:"objectsTotal"`
	ObjectsLastError   string        `json:"objectsLastError,omitempty"`
	ObjectsUpdatedAt   time.Time     `json:"objectsUpdatedAt,omitzero"`
	Objects            []WorldObject `json:"objects"`
}

type PlayerSnapshot struct {
	Server           ServerInfo    `json:"server"`
	Connected        bool          `json:"connected"`
	Stale            bool          `json:"stale"`
	LastSuccessAt    time.Time     `json:"lastSuccessAt,omitzero"`
	Players          []Player      `json:"players"`
	SaveEnabled      bool          `json:"saveEnabled"`
	SaveAvailable    bool          `json:"saveAvailable"`
	SaveStale        bool          `json:"saveStale"`
	SaveUpdatedAt    time.Time     `json:"saveUpdatedAt,omitzero"`
	SaveSnapshotAt   time.Time     `json:"saveSnapshotAt,omitzero"`
	SaveLastError    string        `json:"saveLastError,omitempty"`
	Metrics          ServerMetrics `json:"metrics"`
	MetricsAvailable bool          `json:"metricsAvailable"`
	MetricsStale     bool          `json:"metricsStale"`
	MetricsUpdatedAt time.Time     `json:"metricsUpdatedAt,omitzero"`
}

type ObjectSnapshot struct {
	Available   bool          `json:"available"`
	Stale       bool          `json:"stale"`
	Unsupported bool          `json:"unsupported"`
	Truncated   bool          `json:"truncated"`
	Total       int           `json:"total"`
	LastError   string        `json:"lastError,omitempty"`
	UpdatedAt   time.Time     `json:"updatedAt,omitzero"`
	Objects     []WorldObject `json:"objects"`
}

type Source interface {
	Info(context.Context) (ServerInfo, error)
	Players(context.Context) ([]Player, error)
	Metrics(context.Context) (ServerMetrics, error)
	WorldObjects(context.Context) ([]WorldObject, error)
}

// RosterSnapshot is the persistent, save-derived view of players. Positions
// are the last saved positions and are superseded by REST coordinates while a
// player is online.
type RosterSnapshot struct {
	SnapshotAt time.Time
	Players    []Player
}

type RosterSource interface {
	Roster(context.Context) (RosterSnapshot, error)
}

type Poller struct {
	source         Source
	roster         RosterSource
	playerEvery    time.Duration
	worldEvery     time.Duration
	rosterEvery    time.Duration
	worldEnabled   bool
	logger         *slog.Logger
	unsupportedLog bool

	mu       sync.RWMutex
	snapshot Snapshot
	online   []Player
	saved    []Player
}

func NewPoller(source Source, playerEvery, worldEvery time.Duration, worldEnabled bool, logger *slog.Logger) *Poller {
	return NewPollerWithRoster(source, nil, playerEvery, worldEvery, 0, worldEnabled, logger)
}

func NewPollerWithRoster(source Source, roster RosterSource, playerEvery, worldEvery, rosterEvery time.Duration, worldEnabled bool, logger *slog.Logger) *Poller {
	return &Poller{
		source: source, roster: roster, playerEvery: playerEvery, worldEvery: worldEvery,
		rosterEvery: rosterEvery, worldEnabled: worldEnabled, logger: logger,
		snapshot: Snapshot{Players: []Player{}, Objects: []WorldObject{}, SaveEnabled: roster != nil},
		online:   []Player{}, saved: []Player{},
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
	if p.roster != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			p.runRoster(ctx)
		}()
	}
	workers.Wait()
}

func (p *Poller) runInfo(ctx context.Context) {
	runEvery(ctx, time.Minute, p.refreshInfo)
}

func (p *Poller) runPlayers(ctx context.Context) {
	runEvery(ctx, p.playerEvery, p.refreshPlayers)
}

func (p *Poller) runMetrics(ctx context.Context) {
	runEvery(ctx, p.playerEvery, p.refreshMetrics)
}

func (p *Poller) runWorld(ctx context.Context) {
	runEvery(ctx, p.worldEvery, p.refreshWorld)
}

func (p *Poller) runRoster(ctx context.Context) {
	runEvery(ctx, p.rosterEvery, p.refreshRoster)
}

func runEvery(ctx context.Context, interval time.Duration, refresh func(context.Context)) {
	refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh(ctx)
		}
	}
}

func (p *Poller) Snapshot() Snapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := p.snapshot
	result.Players = clonePlayers(p.snapshot.Players)
	result.Objects = cloneWorldObjects(p.snapshot.Objects)
	return result
}

// PlayerSnapshot avoids copying the potentially large world-object slice for
// the frequently-polled player endpoint.
func (p *Poller) PlayerSnapshot() PlayerSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return PlayerSnapshot{
		Server: p.snapshot.Server, Connected: p.snapshot.Connected, Stale: p.snapshot.Stale,
		LastSuccessAt: p.snapshot.LastSuccessAt, Players: clonePlayers(p.snapshot.Players),
		SaveEnabled: p.snapshot.SaveEnabled, SaveAvailable: p.snapshot.SaveAvailable,
		SaveStale: p.snapshot.SaveStale, SaveUpdatedAt: p.snapshot.SaveUpdatedAt,
		SaveSnapshotAt: p.snapshot.SaveSnapshotAt, SaveLastError: p.snapshot.SaveLastError,
		Metrics: p.snapshot.Metrics, MetricsAvailable: p.snapshot.MetricsAvailable,
		MetricsStale: p.snapshot.MetricsStale, MetricsUpdatedAt: p.snapshot.MetricsUpdatedAt,
	}
}

// ObjectSnapshot avoids copying player state for the slower world-data endpoint.
func (p *Poller) ObjectSnapshot() ObjectSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return ObjectSnapshot{
		Available: p.snapshot.ObjectsAvailable, Stale: p.snapshot.ObjectsStale,
		Unsupported: p.snapshot.ObjectsUnsupported, Truncated: p.snapshot.ObjectsTruncated,
		Total: p.snapshot.ObjectsTotal, LastError: p.snapshot.ObjectsLastError,
		UpdatedAt: p.snapshot.ObjectsUpdatedAt, Objects: cloneWorldObjects(p.snapshot.Objects),
	}
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
	p.online = clonePlayers(players)
	for index := range p.online {
		p.online[index].Online = true
	}
	p.snapshot.Players = mergePlayers(p.saved, p.online)
	p.mu.Unlock()
}

func (p *Poller) refreshRoster(ctx context.Context) {
	roster, err := p.roster.Roster(ctx)
	if err != nil {
		p.mu.Lock()
		p.snapshot.SaveStale = p.snapshot.SaveAvailable
		p.snapshot.SaveLastError = "refresh-failed"
		p.mu.Unlock()
		p.logger.Warn("Palworld save-roster refresh failed", "error", err)
		return
	}
	now := time.Now().UTC()
	p.mu.Lock()
	p.saved = clonePlayers(roster.Players)
	for index := range p.saved {
		p.saved[index].Online = false
	}
	p.snapshot.SaveAvailable = true
	p.snapshot.SaveStale = false
	p.snapshot.SaveUpdatedAt = now
	p.snapshot.SaveSnapshotAt = roster.SnapshotAt.UTC()
	p.snapshot.SaveLastError = ""
	p.snapshot.Players = mergePlayers(p.saved, p.online)
	p.mu.Unlock()
}

func (p *Poller) refreshMetrics(ctx context.Context) {
	metrics, err := p.source.Metrics(ctx)
	if err != nil {
		p.mu.Lock()
		p.snapshot.MetricsStale = p.snapshot.MetricsAvailable
		p.mu.Unlock()
		p.logger.Warn("Palworld server-metrics refresh failed", "error", err)
		return
	}
	p.mu.Lock()
	p.snapshot.Metrics = metrics
	p.snapshot.MetricsAvailable = true
	p.snapshot.MetricsStale = false
	p.snapshot.MetricsUpdatedAt = time.Now().UTC()
	p.mu.Unlock()
}

func (p *Poller) refreshWorld(ctx context.Context) {
	objects, err := p.source.WorldObjects(ctx)
	if err != nil {
		var limitError *WorldObjectLimitError
		if errors.As(err, &limitError) && len(objects) > 0 {
			p.unsupportedLog = false
			p.mu.Lock()
			p.snapshot.ObjectsAvailable = true
			p.snapshot.ObjectsStale = false
			p.snapshot.ObjectsUnsupported = false
			p.snapshot.ObjectsTruncated = true
			p.snapshot.ObjectsTotal = limitError.Total
			p.snapshot.ObjectsLastError = "object-limit"
			p.snapshot.ObjectsUpdatedAt = time.Now().UTC()
			p.snapshot.Objects = cloneWorldObjects(objects)
			p.mu.Unlock()
			p.logger.Warn("Palworld world-object result was truncated", "objects", limitError.Total, "limit", limitError.Limit)
			return
		}
		var statusError *HTTPStatusError
		unsupported := errors.As(err, &statusError) && statusError.Status == http.StatusNotFound
		lastError := "refresh-failed"
		var sizeError *ResponseSizeError
		if errors.As(err, &sizeError) {
			lastError = "response-too-large"
		}
		p.mu.Lock()
		p.snapshot.ObjectsStale = p.snapshot.ObjectsAvailable
		p.snapshot.ObjectsUnsupported = unsupported
		if unsupported {
			lastError = "unsupported"
		}
		p.snapshot.ObjectsLastError = lastError
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
	p.snapshot.ObjectsTruncated = false
	p.snapshot.ObjectsTotal = len(objects)
	p.snapshot.ObjectsLastError = ""
	p.snapshot.ObjectsUpdatedAt = time.Now().UTC()
	p.snapshot.Objects = cloneWorldObjects(objects)
	p.mu.Unlock()
}

func clonePlayers(players []Player) []Player {
	result := make([]Player, len(players))
	copy(result, players)
	return result
}

func mergePlayers(saved, online []Player) []Player {
	result := make([]Player, 0, len(saved)+len(online))
	byID := make(map[string]int, len(saved))
	for _, player := range saved {
		player.Online = false
		if player.ID != "" {
			if _, duplicate := byID[player.ID]; duplicate {
				continue
			}
			byID[player.ID] = len(result)
		}
		result = append(result, player)
	}
	for _, player := range online {
		player.Online = true
		index, found := byID[player.ID]
		if !found || player.ID == "" {
			result = append(result, player)
			continue
		}
		persisted := result[index]
		if player.GuildKey == "" {
			player.GuildKey = persisted.GuildKey
		}
		if player.GuildName == "" {
			player.GuildName = persisted.GuildName
		}
		if player.LastSeenAt.IsZero() {
			player.LastSeenAt = persisted.LastSeenAt
		}
		if player.CaptureTotal == nil {
			player.CaptureTotal = persisted.CaptureTotal
		}
		if player.UniquePalsCaptured == nil {
			player.UniquePalsCaptured = persisted.UniquePalsCaptured
		}
		if player.PaldeckUnlocked == nil {
			player.PaldeckUnlocked = persisted.PaldeckUnlocked
		}
		result[index] = player
	}
	sort.SliceStable(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if left != right {
			return left < right
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func cloneWorldObjects(objects []WorldObject) []WorldObject {
	result := make([]WorldObject, len(objects))
	copy(result, objects)
	return result
}
