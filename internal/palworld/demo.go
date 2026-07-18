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
	name          string
	level         int
	mapID         string
	centerX       float64
	centerY       float64
	radiusX       float64
	radiusY       float64
	periodSeconds float64
	phase         float64
}

var demoPlayers = []demoPlayer{
	{name: "Moss", level: 58, mapID: "palpagos", centerX: -211000, centerY: 185000, radiusX: 92000, radiusY: 126000, periodSeconds: 180, phase: 0},
	{name: "Ember", level: 55, mapID: "palpagos", centerX: -515000, centerY: -168000, radiusX: 116000, radiusY: 78000, periodSeconds: 220, phase: 1.7},
	{name: "Juniper", level: 51, mapID: "palpagos", centerX: -760000, centerY: 360000, radiusX: 68000, radiusY: 104000, periodSeconds: 200, phase: 3.2},
	{name: "Orbit", level: 60, mapID: "world-tree", centerX: 518000, centerY: -645000, radiusX: 52000, radiusY: 59000, periodSeconds: 160, phase: .8},
}

var demoObjects = []WorldObject{
	{Kind: "bases", Name: "Seabreeze Workshop", Detail: "Demo Guild · Coastal crafting base", BaseID: "demo-base-coast", GuildKey: "demo-guild-aurora", X: -246000, Y: 128000, Map: "palpagos"},
	{Kind: "workers", Name: "Anubis", Detail: "Handiwork", BaseID: "demo-base-coast", GuildKey: "demo-guild-aurora", Level: 49, X: -243000, Y: 132000, Map: "palpagos"},
	{Kind: "workers", Name: "Azurobe", Detail: "Watering", BaseID: "demo-base-coast", GuildKey: "demo-guild-aurora", Level: 44, X: -249000, Y: 124000, Map: "palpagos"},
	{Kind: "bases", Name: "Frostline Outpost", Detail: "Demo Guild · Mountain supply camp", BaseID: "demo-base-frost", GuildKey: "demo-guild-aurora", X: -652000, Y: 327000, Map: "palpagos"},
	{Kind: "workers", Name: "Foxcicle", Detail: "Cooling", BaseID: "demo-base-frost", GuildKey: "demo-guild-aurora", Level: 38, X: -648000, Y: 331000, Map: "palpagos"},
	{Kind: "companions", Name: "Dazemu", Detail: "Travelling with Ember", Level: 54, X: -498000, Y: -173000, Map: "palpagos"},
	{Kind: "companions", Name: "Xenolord", Detail: "Travelling with Orbit", Level: 60, X: 524000, Y: -638000, Map: "world-tree"},
	{Kind: "wild-pals", Name: "Lamball", Detail: "Fictional demo spawn", Level: 7, X: -321000, Y: 87000, Map: "palpagos"},
	{Kind: "wild-pals", Name: "Frostallion", Detail: "Fictional demo spawn", Level: 50, X: -812000, Y: 418000, Map: "palpagos"},
	{Kind: "wild-pals", Name: "Celesdir", Detail: "Fictional demo spawn", Level: 56, X: 566000, Y: -604000, Map: "world-tree"},
	{Kind: "npcs", Name: "Wandering Merchant", Detail: "Fictional demo NPC", X: -390000, Y: 245000, Map: "palpagos"},
	{Kind: "npcs", Name: "Expedition Researcher", Detail: "Fictional demo NPC", X: 482000, Y: -704000, Map: "world-tree"},
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
	players := make([]Player, 0, len(demoPlayers))
	for _, spec := range demoPlayers {
		angle := spec.phase + elapsed/spec.periodSeconds*2*math.Pi
		players = append(players, Player{
			Name: spec.name, Level: spec.level, Map: spec.mapID,
			X: spec.centerX + math.Sin(angle)*spec.radiusX,
			Y: spec.centerY + math.Cos(angle)*spec.radiusY,
		})
	}
	return players, nil
}

func (d *DemoSource) Metrics(ctx context.Context) (ServerMetrics, error) {
	if err := ctx.Err(); err != nil {
		return ServerMetrics{}, err
	}
	uptime := max(0, int64(d.now().UTC().Sub(d.started).Seconds()))
	return ServerMetrics{
		CurrentPlayers: len(demoPlayers), MaxPlayers: 32, ServerFPS: 60,
		AverageFPS: 59.8, ServerFrameTime: 16.72, UptimeSeconds: uptime,
		BaseCount: 2, Days: 184,
	}, nil
}

func (d *DemoSource) WorldObjects(ctx context.Context) ([]WorldObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]WorldObject(nil), demoObjects...), nil
}
