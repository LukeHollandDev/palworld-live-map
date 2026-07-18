package palworld

import (
	"context"
	"testing"
	"time"
)

func TestDemoSourceIsDeterministicAndMovesPlayers(t *testing.T) {
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

	current = current.Add(30 * time.Second)
	moved, _ := source.Players(context.Background())
	if first[0].X == moved[0].X && first[0].Y == moved[0].Y {
		t.Fatal("demo player did not move as time advanced")
	}
	if moved[3].Map != "world-tree" {
		t.Fatalf("world tree demo player map = %q", moved[3].Map)
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
	if err != nil || metrics.CurrentPlayers != len(demoPlayers) || metrics.UptimeSeconds != 0 {
		t.Fatalf("Metrics() = %+v, %v", metrics, err)
	}
	objects, err := source.WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	kinds := map[string]bool{}
	for _, object := range objects {
		kinds[object.Kind] = true
	}
	for _, kind := range []string{"bases", "workers", "companions", "wild-pals", "npcs"} {
		if !kinds[kind] {
			t.Errorf("demo objects have no %q", kind)
		}
	}
}

func TestDemoSourceHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source := NewDemoSource()
	if _, err := source.Players(ctx); err == nil {
		t.Fatal("Players() error = nil for cancelled context")
	}
}
