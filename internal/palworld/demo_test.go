package palworld

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
)

func TestDemoSourceIsDeterministicAndMovesPlayersAndCompanions(t *testing.T) {
	current := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := newDemoSource(func() time.Time { return current })

	first, err := source.Players(context.Background())
	if err != nil {
		t.Fatalf("Players() error = %v", err)
	}
	again, _ := source.Players(context.Background())
	if len(first) != 4 || first[0] != again[0] {
		t.Fatalf("demo players are not deterministic: first=%+v again=%+v", first, again)
	}
	initialObjects, err := source.WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	initialObjectsByID := make(map[string]WorldObject, len(initialObjects))
	for _, object := range initialObjects {
		initialObjectsByID[object.ID] = object
	}

	current = current.Add(30 * time.Second)
	moved, _ := source.Players(context.Background())
	if first[0].X == moved[0].X && first[0].Y == moved[0].Y {
		t.Fatal("demo player did not move as time advanced")
	}
	if first[0].ID == "" || first[0].ID != moved[0].ID {
		t.Fatalf("demo player ID changed while moving: %q != %q", first[0].ID, moved[0].ID)
	}
	if moved[3].Map != "world-tree" {
		t.Fatalf("world tree demo player map = %q", moved[3].Map)
	}
	objects, err := source.WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	movedByID := make(map[string]Player, len(moved))
	for _, player := range moved {
		movedByID[player.ID] = player
	}
	for _, object := range objects {
		if object.Kind != "companions" {
			continue
		}
		initial := initialObjectsByID[object.ID]
		if object.X == initial.X && object.Y == initial.Y {
			t.Fatalf("demo companion did not move with time: %#v", object)
		}
		owner, found := movedByID[object.OwnerID]
		if !found || object.Map != owner.Map || math.Hypot(object.X-owner.X, object.Y-owner.Y) > 2500 {
			t.Fatalf("demo companion does not travel with its owner: companion=%#v owner=%#v", object, owner)
		}
	}
}

func TestDemoSourceProvidesAllPublicDataKinds(t *testing.T) {
	current := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	source := newDemoSource(func() time.Time { return current })
	info, err := source.Info(context.Background())
	if err != nil || info.Name == "" {
		t.Fatalf("Info() = %+v, %v", info, err)
	}
	metrics, err := source.Metrics(context.Background())
	if err != nil || metrics.CurrentPlayers != len(demoPlayers) || metrics.UptimeSeconds != demoInitialUptimeSeconds {
		t.Fatalf("Metrics() = %+v, %v", metrics, err)
	}
	players, err := source.Players(context.Background())
	if err != nil {
		t.Fatalf("Players() error = %v", err)
	}
	playersByID := make(map[string]Player, len(players))
	for _, player := range players {
		if player.ID == "" || playersByID[player.ID].ID != "" {
			t.Fatalf("demo player has missing or duplicate ID: %#v", player)
		}
		playersByID[player.ID] = player
		if player.GuildKey == "" || player.GuildName == "" || layerFor(player.X, player.Y) != player.Map {
			t.Fatalf("demo player has no guild relation: %#v", player)
		}
	}
	objects, err := source.WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	kinds := map[string]bool{}
	ids := map[string]bool{}
	bases := map[string]WorldObject{}
	for _, object := range objects {
		kinds[object.Kind] = true
		if object.ID == "" || ids[object.ID] {
			t.Fatalf("demo object has missing or duplicate ID: %#v", object)
		}
		ids[object.ID] = true
		if layerFor(object.X, object.Y) != object.Map {
			t.Fatalf("demo object is outside its declared map: %#v", object)
		}
		if object.Kind == "bases" && object.BaseID != object.ID {
			t.Fatalf("demo base relation does not reference public object ID: %#v", object)
		}
		if object.Kind == "bases" {
			bases[object.ID] = object
		}
		if (object.Kind == "workers" || object.Kind == "companions" || object.Kind == "wild-pals") && object.Detail != "" {
			t.Fatalf("demo Pal using its species as its name must not have a role or placeholder detail: %#v", object)
		}
	}
	for _, object := range objects {
		switch object.Kind {
		case "workers":
			base, found := bases[object.BaseID]
			if !found || base.GuildKey != object.GuildKey || base.Map != object.Map ||
				math.Hypot(object.X-base.X, object.Y-base.Y) > baseAssociationRadius {
				t.Fatalf("demo worker is not inside its assigned guild base: worker=%#v base=%#v", object, base)
			}
		case "companions":
			owner, found := playersByID[object.OwnerID]
			if !found || owner.GuildKey != object.GuildKey || owner.Map != object.Map ||
				math.Hypot(object.X-owner.X, object.Y-owner.Y) > 2500 {
				t.Fatalf("demo companion does not reference and follow its owner: companion=%#v owner=%#v", object, owner)
			}
		}
	}
	if metrics.BaseCount != len(bases) {
		t.Fatalf("demo metrics base count = %d, objects contain %d bases", metrics.BaseCount, len(bases))
	}
	for _, player := range players {
		found := false
		for _, base := range bases {
			if base.GuildKey == player.GuildKey && base.Map == player.Map {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("demo player guild has no base on %s: %#v", player.Map, player)
		}
	}
	for _, kind := range []string{"bases", "workers", "companions", "wild-pals", "npcs"} {
		if !kinds[kind] {
			t.Errorf("demo objects have no %q", kind)
		}
	}
}

func TestDemoPalpagosMarkersStayInsideVisibleArtwork(t *testing.T) {
	const minimumMarkerMargin = 4.0
	current := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	objects, err := newDemoSource(func() time.Time { return current }).WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	for _, object := range objects {
		if object.Map == "palpagos" &&
			(!insideDemoPalpagosArtwork(object.X, object.Y) || demoPalpagosArtworkMargin(object.X, object.Y) < minimumMarkerMargin) {
			t.Fatalf("demo object renders too close to the Palpagos artwork edge: %#v", object)
		}
	}
	for _, player := range demoPlayers {
		if player.mapID != "palpagos" {
			continue
		}
		for sample := range 360 {
			angle := float64(sample) / 360 * 2 * math.Pi
			x := player.centerX + math.Sin(angle)*player.radiusX
			y := player.centerY + math.Cos(angle)*player.radiusY
			if !insideDemoPalpagosArtwork(x, y) || demoPalpagosArtworkMargin(x, y) < minimumMarkerMargin {
				t.Fatalf("demo player %q approaches the Palpagos artwork edge at (%g, %g)", player.name, x, y)
			}
		}
	}
}

func layerFor(x, y float64) string {
	layer, _ := mapdata.LayerID(x, y)
	return layer
}

// This is the solid outline from web/src/assets/palpagos-mask.svg. Demo data is
// held inside it with enough margin for markers, not merely inside map bounds.
var demoPalpagosOutline = [...][2]float64{
	{48.5, 2.6}, {95.1, 9.2}, {95.1, 46.2}, {76.6, 73.6}, {70.2, 82.8},
	{48, 97.2}, {2.7, 97.2}, {2.7, 63.6}, {33, 26.3}, {35.4, 9.1},
}

func demoPalpagosPoint(x, y float64) [2]float64 {
	const (
		maxX = 349400.0
		maxY = 724400.0
		minX = -1099400.0
		minY = -724400.0
	)
	return [2]float64{(y - minY) / (maxY - minY) * 100, (maxX - x) / (maxX - minX) * 100}
}

func insideDemoPalpagosArtwork(x, y float64) bool {
	point := demoPalpagosPoint(x, y)
	inside := false
	previous := len(demoPalpagosOutline) - 1
	for current, vertex := range demoPalpagosOutline {
		prior := demoPalpagosOutline[previous]
		if (vertex[1] > point[1]) != (prior[1] > point[1]) &&
			point[0] < (prior[0]-vertex[0])*(point[1]-vertex[1])/(prior[1]-vertex[1])+vertex[0] {
			inside = !inside
		}
		previous = current
	}
	return inside
}

func demoPalpagosArtworkMargin(x, y float64) float64 {
	point := demoPalpagosPoint(x, y)
	margin := math.Inf(1)
	for index, start := range demoPalpagosOutline {
		end := demoPalpagosOutline[(index+1)%len(demoPalpagosOutline)]
		deltaX, deltaY := end[0]-start[0], end[1]-start[1]
		lengthSquared := deltaX*deltaX + deltaY*deltaY
		projection := ((point[0]-start[0])*deltaX + (point[1]-start[1])*deltaY) / lengthSquared
		projection = math.Max(0, math.Min(1, projection))
		closestX, closestY := start[0]+projection*deltaX, start[1]+projection*deltaY
		margin = math.Min(margin, math.Hypot(point[0]-closestX, point[1]-closestY))
	}
	return margin
}

func TestDemoSourceHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := NewDemoSource()
	if _, err := source.Players(ctx); err == nil {
		t.Fatal("Players() error = nil for cancelled context")
	}
}
