package palworld

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const saveResolveFailed = "resolve-failed"

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
// player is online. PartialError describes a usable but degraded snapshot and
// remains server-side; the poller publishes only a stable error category.
type RosterSnapshot struct {
	SnapshotAt   time.Time
	Players      []Player
	PartialError error
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

	mu             sync.RWMutex
	snapshot       Snapshot
	online         []Player
	saved          []Player
	playerRevision uint64
	objectRevision uint64
}

func NewPoller(source Source, playerEvery, worldEvery time.Duration, worldEnabled bool, logger *slog.Logger) *Poller {
	return NewPollerWithRoster(source, nil, playerEvery, worldEvery, 0, worldEnabled, logger)
}

func NewPollerWithRoster(source Source, roster RosterSource, playerEvery, worldEvery, rosterEvery time.Duration, worldEnabled bool, logger *slog.Logger) *Poller {
	return &Poller{
		source: source, roster: roster, playerEvery: playerEvery, worldEvery: worldEvery,
		rosterEvery: rosterEvery, worldEnabled: worldEnabled, logger: logger,
		snapshot:       Snapshot{Players: []Player{}, Objects: []WorldObject{}, SaveEnabled: roster != nil},
		online:         []Player{},
		saved:          []Player{},
		playerRevision: 1,
		objectRevision: 1,
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
	return p.playerSnapshotLocked()
}

// PlayerSnapshotSince avoids cloning player state when the caller already has
// the current immutable semantic revision.
func (p *Poller) PlayerSnapshotSince(revision uint64) (PlayerSnapshot, uint64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if revision == p.playerRevision {
		return PlayerSnapshot{}, revision, false
	}
	return p.playerSnapshotLocked(), p.playerRevision, true
}

func (p *Poller) playerSnapshotLocked() PlayerSnapshot {
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
	return p.objectSnapshotLocked()
}

// ObjectSnapshotSince avoids cloning the potentially large world-object slice
// when its semantic contents and status have not changed.
func (p *Poller) ObjectSnapshotSince(revision uint64) (ObjectSnapshot, uint64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if revision == p.objectRevision {
		return ObjectSnapshot{}, revision, false
	}
	return p.objectSnapshotLocked(), p.objectRevision, true
}

func (p *Poller) objectSnapshotLocked() ObjectSnapshot {
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
	if p.snapshot.Server != info {
		p.snapshot.Server = info
		p.playerRevision++
	}
	p.mu.Unlock()
}

func (p *Poller) refreshPlayers(ctx context.Context) {
	players, err := p.source.Players(ctx)
	if err != nil {
		p.mu.Lock()
		previousConnected, previousStale := p.snapshot.Connected, p.snapshot.Stale
		p.snapshot.Connected = false
		p.snapshot.Stale = !p.snapshot.LastSuccessAt.IsZero()
		if p.snapshot.Connected != previousConnected || p.snapshot.Stale != previousStale {
			p.playerRevision++
		}
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
	p.playerRevision++
	p.mu.Unlock()
}

func (p *Poller) refreshRoster(ctx context.Context) {
	roster, err := p.roster.Roster(ctx)
	if err != nil {
		p.mu.Lock()
		previousStale, previousError := p.snapshot.SaveStale, p.snapshot.SaveLastError
		p.snapshot.SaveStale = p.snapshot.SaveAvailable
		p.snapshot.SaveLastError = "refresh-failed"
		if p.snapshot.SaveStale != previousStale || p.snapshot.SaveLastError != previousError {
			p.playerRevision++
		}
		p.mu.Unlock()
		// Save reader errors can contain filesystem paths and raw save-authored
		// identifiers. The public state and logs expose only a stable category.
		p.logger.Warn("Palworld save-roster refresh failed", "category", "refresh-failed")
		return
	}
	now := time.Now().UTC()
	lastError := ""
	if roster.PartialError != nil {
		lastError = saveResolveFailed
	}
	p.mu.Lock()
	previousError := p.snapshot.SaveLastError
	p.saved = clonePlayers(roster.Players)
	for index := range p.saved {
		p.saved[index].Online = false
	}
	p.snapshot.SaveAvailable = true
	p.snapshot.SaveStale = false
	p.snapshot.SaveUpdatedAt = now
	p.snapshot.SaveSnapshotAt = roster.SnapshotAt.UTC()
	p.snapshot.SaveLastError = lastError
	p.snapshot.Players = mergePlayers(p.saved, p.online)
	p.playerRevision++
	p.mu.Unlock()
	switch {
	case lastError == saveResolveFailed && previousError != saveResolveFailed:
		p.logger.Warn("Palworld save-roster resolve failed; using partial enrichment", "category", saveResolveFailed)
	case lastError == "" && previousError == saveResolveFailed:
		p.logger.Info("Palworld save-roster resolve recovered")
	}
}

func (p *Poller) refreshMetrics(ctx context.Context) {
	metrics, err := p.source.Metrics(ctx)
	if err != nil {
		p.mu.Lock()
		previousStale := p.snapshot.MetricsStale
		p.snapshot.MetricsStale = p.snapshot.MetricsAvailable
		if p.snapshot.MetricsStale != previousStale {
			p.playerRevision++
		}
		p.mu.Unlock()
		p.logger.Warn("Palworld server-metrics refresh failed", "error", err)
		return
	}
	p.mu.Lock()
	p.snapshot.Metrics = metrics
	p.snapshot.MetricsAvailable = true
	p.snapshot.MetricsStale = false
	p.snapshot.MetricsUpdatedAt = time.Now().UTC()
	p.playerRevision++
	p.mu.Unlock()
}

func (p *Poller) refreshWorld(ctx context.Context) {
	objects, err := p.source.WorldObjects(ctx)
	objects = retainPublishableWorldObjects(objects)
	if err != nil {
		var limitError *WorldObjectLimitError
		if errors.As(err, &limitError) && len(objects) > 0 {
			p.unsupportedLog = false
			p.publishWorld(objects, true, limitError.Total, "object-limit")
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
		previousStale := p.snapshot.ObjectsStale
		previousUnsupported := p.snapshot.ObjectsUnsupported
		previousError := p.snapshot.ObjectsLastError
		p.snapshot.ObjectsStale = p.snapshot.ObjectsAvailable
		p.snapshot.ObjectsUnsupported = unsupported
		if unsupported {
			lastError = "unsupported"
		}
		p.snapshot.ObjectsLastError = lastError
		if p.snapshot.ObjectsStale != previousStale || p.snapshot.ObjectsUnsupported != previousUnsupported || p.snapshot.ObjectsLastError != previousError {
			p.objectRevision++
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
	p.publishWorld(objects, false, len(objects), "")
}

func (p *Poller) publishWorld(objects []WorldObject, truncated bool, total int, lastError string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.snapshot.ObjectsAvailable && !p.snapshot.ObjectsStale && !p.snapshot.ObjectsUnsupported &&
		p.snapshot.ObjectsTruncated == truncated && p.snapshot.ObjectsTotal == total &&
		p.snapshot.ObjectsLastError == lastError && slices.EqualFunc(p.snapshot.Objects, objects, worldObjectEqual) {
		return
	}
	p.snapshot.ObjectsAvailable = true
	p.snapshot.ObjectsStale = false
	p.snapshot.ObjectsUnsupported = false
	p.snapshot.ObjectsTruncated = truncated
	p.snapshot.ObjectsTotal = total
	p.snapshot.ObjectsLastError = lastError
	p.snapshot.ObjectsUpdatedAt = time.Now().UTC()
	p.snapshot.Objects = cloneWorldObjects(objects)
	p.objectRevision++
}

func retainPublishableWorldObjects(objects []WorldObject) []WorldObject {
	for index, object := range objects {
		if publishableWorldObject(object.Kind) {
			continue
		}
		result := make([]WorldObject, 0, len(objects)-1)
		result = append(result, objects[:index]...)
		for _, candidate := range objects[index+1:] {
			if publishableWorldObject(candidate.Kind) {
				result = append(result, candidate)
			}
		}
		return result
	}
	return objects
}

func clonePlayers(players []Player) []Player {
	result := make([]Player, len(players))
	copy(result, players)
	return result
}

func mergePlayers(saved, online []Player) []Player {
	result := make([]Player, 0, len(saved)+len(online))
	byID := make(map[string]int, len(saved))
	persistedByID := make(map[string]Player, len(saved))
	for _, player := range saved {
		player.Online = false
		if player.ID != "" {
			if _, duplicate := persistedByID[player.ID]; duplicate {
				continue
			}
			persistedByID[player.ID] = player
		}
		// Names come from the roster's resolve pass. When it fails or skips a
		// record the player stays available for ID-based enrichment of the REST
		// list, but is never published as an anonymous offline marker: an
		// unnamed dot cannot be identified, searched, or attributed.
		if player.Name == "" {
			continue
		}
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
		persisted, hasPersisted := persistedByID[player.ID]
		if !found || player.ID == "" {
			if hasPersisted {
				player = mergePersistedPlayer(player, persisted)
			}
			result = append(result, player)
			continue
		}
		result[index] = mergePersistedPlayer(player, persisted)
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

func mergePersistedPlayer(player, persisted Player) Player {
	// A current world relation is authoritative even when it says the player is
	// not in a guild. Otherwise save enrichment may fill the missing pair, while
	// retaining false provenance so privacy projections can remove it again.
	if !player.GuildFromLive {
		if player.GuildKey == "" {
			player.GuildKey = persisted.GuildKey
		}
		if player.GuildName == "" {
			player.GuildName = persisted.GuildName
		}
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
	if player.ArenaRankPoints == nil {
		player.ArenaRankPoints = persisted.ArenaRankPoints
	}
	if player.FastTravelUnlocked == nil {
		player.FastTravelUnlocked = persisted.FastTravelUnlocked
	}
	if player.AreasDiscovered == nil {
		player.AreasDiscovered = persisted.AreasDiscovered
	}
	if player.BossDefeats == nil {
		player.BossDefeats = persisted.BossDefeats
	}
	if player.TowerDefeats == nil {
		player.TowerDefeats = persisted.TowerDefeats
	}
	return player
}

func cloneWorldObjects(objects []WorldObject) []WorldObject {
	result := make([]WorldObject, len(objects))
	for index, object := range objects {
		result[index] = object
		if object.Z != nil {
			z := *object.Z
			result[index].Z = &z
		}
		result[index].Rewards = append([]LandmarkReward(nil), object.Rewards...)
	}
	return result
}

func worldObjectEqual(left, right WorldObject) bool {
	return left.ID == right.ID &&
		left.Kind == right.Kind &&
		left.Name == right.Name &&
		left.Detail == right.Detail &&
		left.BaseID == right.BaseID &&
		left.GuildKey == right.GuildKey &&
		left.OwnerID == right.OwnerID &&
		left.Level == right.Level &&
		left.X == right.X &&
		left.Y == right.Y &&
		optionalFloat64Equal(left.Z, right.Z) &&
		left.Map == right.Map &&
		slices.Equal(left.Rewards, right.Rewards)
}

func optionalFloat64Equal(left, right *float64) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
