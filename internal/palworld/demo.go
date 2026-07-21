package palworld

import (
	"context"
	"math"
	"time"
)

// DemoSource provides fictional, deterministic data for public demonstrations.
// It implements Source so demo deployments exercise the production poller and API.
type DemoSource struct {
	now     func() time.Time
	started time.Time
}

type demoPlayer struct {
	id            string
	name          string
	guildKey      string
	guildName     string
	level         int
	mapID         string
	centerX       float64
	centerY       float64
	radiusX       float64
	radiusY       float64
	periodSeconds float64
	phase         float64
}

var (
	demoCoastBaseID  = opaqueID("object", "demo-base-coast")
	demoFrostBaseID  = opaqueID("object", "demo-base-frost")
	demoTreeBaseID   = opaqueID("object", "demo-base-world-tree")
	demoGuildKey     = opaqueID("guild", "demo-guild-aurora")
	demoTreeGuildKey = opaqueID("guild", "demo-guild-world-tree")
	demoMossID       = opaqueID("player", "demo-player-moss")
	demoEmberID      = opaqueID("player", "demo-player-ember")
	demoJuniperID    = opaqueID("player", "demo-player-juniper")
	demoOrbitID      = opaqueID("player", "demo-player-orbit")
)

var demoPlayers = []demoPlayer{
	{id: demoMossID, name: "Moss", guildKey: demoGuildKey, guildName: "Aurora Expedition", level: 58, mapID: "palpagos", centerX: -270000, centerY: 150000, radiusX: 18000, radiusY: 14000, periodSeconds: 180, phase: 0},
	{id: demoEmberID, name: "Ember", guildKey: demoGuildKey, guildName: "Aurora Expedition", level: 55, mapID: "palpagos", centerX: -500000, centerY: -165000, radiusX: 20000, radiusY: 14000, periodSeconds: 220, phase: 1.7},
	{id: demoJuniperID, name: "Juniper", guildKey: demoGuildKey, guildName: "Aurora Expedition", level: 51, mapID: "palpagos", centerX: 90000, centerY: 100000, radiusX: 16000, radiusY: 12000, periodSeconds: 200, phase: 3.2},
	{id: demoOrbitID, name: "Orbit", guildKey: demoTreeGuildKey, guildName: "World Tree Survey", level: 60, mapID: "world-tree", centerX: 518000, centerY: -645000, radiusX: 16000, radiusY: 12000, periodSeconds: 160, phase: .8},
}

var demoObjects = []WorldObject{
	{ID: demoCoastBaseID, Kind: "bases", Name: "Aurora Expedition", Detail: "Coastal crafting base", BaseID: demoCoastBaseID, GuildKey: demoGuildKey, X: -246000, Y: 128000, Map: "palpagos"},
	{ID: opaqueID("object", "demo-worker-anubis"), Kind: "workers", Name: "Anubis", BaseID: demoCoastBaseID, GuildKey: demoGuildKey, Level: 49, X: -244800, Y: 129600, Map: "palpagos"},
	{ID: opaqueID("object", "demo-worker-azurobe"), Kind: "workers", Name: "Azurobe", BaseID: demoCoastBaseID, GuildKey: demoGuildKey, Level: 44, X: -247700, Y: 126600, Map: "palpagos"},
	{ID: demoFrostBaseID, Kind: "bases", Name: "Aurora Expedition", Detail: "Mountain supply camp", BaseID: demoFrostBaseID, GuildKey: demoGuildKey, X: 45000, Y: 70000, Map: "palpagos"},
	{ID: opaqueID("object", "demo-worker-foxcicle"), Kind: "workers", Name: "Foxcicle", BaseID: demoFrostBaseID, GuildKey: demoGuildKey, Level: 38, X: 46500, Y: 71500, Map: "palpagos"},
	{ID: opaqueID("object", "demo-companion-dazemu"), Kind: "companions", Name: "Dazemu", GuildKey: demoGuildKey, OwnerID: demoEmberID, Level: 54, Map: "palpagos"},
	{ID: demoTreeBaseID, Kind: "bases", Name: "World Tree Survey", Detail: "Research outpost", BaseID: demoTreeBaseID, GuildKey: demoTreeGuildKey, X: 550000, Y: -680000, Map: "world-tree"},
	{ID: opaqueID("object", "demo-worker-knocklem"), Kind: "workers", Name: "Knocklem", BaseID: demoTreeBaseID, GuildKey: demoTreeGuildKey, Level: 57, X: 551500, Y: -678500, Map: "world-tree"},
	{ID: opaqueID("object", "demo-companion-xenolord"), Kind: "companions", Name: "Xenolord", GuildKey: demoTreeGuildKey, OwnerID: demoOrbitID, Level: 60, Map: "world-tree"},
	{ID: opaqueID("object", "demo-wild-lamball"), Kind: "wild-pals", Name: "Lamball", Level: 7, X: -321000, Y: 87000, Map: "palpagos"},
	{ID: opaqueID("object", "demo-wild-frostallion"), Kind: "wild-pals", Name: "Frostallion", Level: 50, X: 120000, Y: 60000, Map: "palpagos"},
	{ID: opaqueID("object", "demo-wild-celesdir"), Kind: "wild-pals", Name: "Celesdir", Level: 56, X: 566000, Y: -604000, Map: "world-tree"},
	{ID: opaqueID("object", "demo-npc-merchant"), Kind: "npcs", Name: "Wandering Merchant", Detail: "Merchant", X: -390000, Y: 245000, Map: "palpagos"},
	{ID: opaqueID("object", "demo-npc-researcher"), Kind: "npcs", Name: "Expedition Researcher", Detail: "Researcher", X: 482000, Y: -704000, Map: "world-tree"},
}

const demoInitialUptimeSeconds = int64(3*24*60*60 + 8*60*60 + 27*60)

var demoCompanionOffsets = map[string][2]float64{
	demoEmberID: {1400, -900},
	demoOrbitID: {-1200, 1100},
}

func NewDemoSource() *DemoSource {
	now := time.Now
	return newDemoSource(now)
}

func newDemoSource(now func() time.Time) *DemoSource {
	return &DemoSource{now: now, started: now().UTC()}
}

func (d *DemoSource) Info(ctx context.Context) (ServerInfo, error) {
	if err := ctx.Err(); err != nil {
		return ServerInfo{}, err
	}
	return ServerInfo{
		Name:        "Palpagos Community Demo",
		Description: "Fictional live data — try the maps, filters and server details",
		Version:     "1.0 demo",
	}, nil
}

func (d *DemoSource) Players(ctx context.Context) ([]Player, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	elapsed := d.now().UTC().Sub(d.started).Seconds()
	return demoPlayersAt(elapsed), nil
}

func demoPlayersAt(elapsed float64) []Player {
	players := make([]Player, 0, len(demoPlayers))
	for _, spec := range demoPlayers {
		angle := spec.phase + elapsed/spec.periodSeconds*2*math.Pi
		players = append(players, Player{
			ID: spec.id, Name: spec.name,
			GuildKey: spec.guildKey, GuildName: spec.guildName, Level: spec.level, Map: spec.mapID,
			X: spec.centerX + math.Sin(angle)*spec.radiusX,
			Y: spec.centerY + math.Cos(angle)*spec.radiusY,
		})
	}
	return players
}

func (d *DemoSource) Metrics(ctx context.Context) (ServerMetrics, error) {
	if err := ctx.Err(); err != nil {
		return ServerMetrics{}, err
	}
	uptime := demoInitialUptimeSeconds + max(0, int64(d.now().UTC().Sub(d.started).Seconds()))
	return ServerMetrics{
		CurrentPlayers: len(demoPlayers), MaxPlayers: 32, ServerFPS: 60,
		ServerFrameTime: 16.72, UptimeSeconds: uptime,
		BaseCount: demoObjectCount("bases"), Days: 184,
	}, nil
}

func demoObjectCount(kind string) int {
	count := 0
	for _, object := range demoObjects {
		if object.Kind == kind {
			count++
		}
	}
	return count
}

func (d *DemoSource) WorldObjects(ctx context.Context) ([]WorldObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	objects := append([]WorldObject(nil), demoObjects...)
	owners := make(map[string]Player, len(demoPlayers))
	for _, player := range demoPlayersAt(d.now().UTC().Sub(d.started).Seconds()) {
		owners[player.ID] = player
	}
	for index := range objects {
		object := &objects[index]
		owner, found := owners[object.OwnerID]
		if object.Kind != "companions" || !found {
			continue
		}
		offset := demoCompanionOffsets[owner.ID]
		object.X, object.Y, object.Map = owner.X+offset[0], owner.Y+offset[1], owner.Map
	}
	return objects, nil
}
