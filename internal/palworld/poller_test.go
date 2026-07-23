package palworld

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

type stubSource struct {
	info      ServerInfo
	infoErr   error
	players   []Player
	playerErr error
	metrics   ServerMetrics
	metricErr error
	objects   []WorldObject
	objectErr error
}

func (s *stubSource) Info(context.Context) (ServerInfo, error) {
	return s.info, s.infoErr
}

func (s *stubSource) Players(context.Context) ([]Player, error) {
	return append([]Player(nil), s.players...), s.playerErr
}

func (s *stubSource) Metrics(context.Context) (ServerMetrics, error) {
	return s.metrics, s.metricErr
}

func (s *stubSource) WorldObjects(context.Context) ([]WorldObject, error) {
	return append([]WorldObject(nil), s.objects...), s.objectErr
}

func testPoller(source Source) *Poller {
	return NewPoller(source, time.Minute, time.Minute, true, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPollerRetainsLastGoodSnapshots(t *testing.T) {
	source := &stubSource{
		info:    ServerInfo{Name: "The Chaos", Description: "A Palworld server", Version: "v1.0.0"},
		players: []Player{{Name: "Lamball", Level: 3}},
		metrics: ServerMetrics{ServerFPS: 60, MaxPlayers: 20},
		objects: []WorldObject{{Kind: "bases", Name: "Home"}},
	}
	poller := testPoller(source)
	poller.refreshInfo(context.Background())
	poller.refreshPlayers(context.Background())
	poller.refreshMetrics(context.Background())
	poller.refreshWorld(context.Background())
	first := poller.Snapshot()
	if first.Server.Name != "The Chaos" || !first.Connected || first.Stale || !first.MetricsAvailable || first.MetricsStale || first.Metrics.ServerFPS != 60 || !first.ObjectsAvailable || len(first.Objects) != 1 {
		t.Fatalf("first snapshot = %#v", first)
	}

	source.players = nil
	source.info = ServerInfo{}
	source.infoErr = errors.New("offline")
	source.playerErr = errors.New("offline")
	source.metricErr = errors.New("offline")
	source.objects = nil
	source.objectErr = errors.New("offline")
	poller.refreshInfo(context.Background())
	poller.refreshPlayers(context.Background())
	poller.refreshMetrics(context.Background())
	poller.refreshWorld(context.Background())
	stale := poller.Snapshot()
	if stale.Server.Name != "The Chaos" || stale.Connected || !stale.Stale || !stale.MetricsAvailable || !stale.MetricsStale || stale.Metrics.ServerFPS != 60 || !stale.ObjectsStale || len(stale.Players) != 1 || len(stale.Objects) != 1 {
		t.Fatalf("stale snapshot = %#v", stale)
	}
	source.metricErr = nil
	source.metrics = ServerMetrics{ServerFPS: 58, MaxPlayers: 20}
	poller.refreshMetrics(context.Background())
	if recovered := poller.PlayerSnapshot(); recovered.MetricsStale || recovered.Metrics.ServerFPS != 58 {
		t.Fatalf("recovered metrics = %#v", recovered)
	}

	stale.Players[0].Name = "mutated"
	stale.Objects[0].Name = "mutated"
	copy := poller.Snapshot()
	if copy.Players[0].Name != "Lamball" || copy.Objects[0].Name != "Home" {
		t.Fatal("Snapshot returned shared storage")
	}
	playerOnly := poller.PlayerSnapshot()
	objectOnly := poller.ObjectSnapshot()
	playerOnly.Players[0].Name = "mutated again"
	objectOnly.Objects[0].Name = "mutated again"
	if poller.PlayerSnapshot().Players[0].Name != "Lamball" || poller.ObjectSnapshot().Objects[0].Name != "Home" {
		t.Fatal("narrow snapshot returned shared storage")
	}
}

func TestPollerReportsUnsupportedWorldData(t *testing.T) {
	source := &stubSource{objectErr: &HTTPStatusError{Status: http.StatusNotFound}}
	poller := testPoller(source)
	poller.refreshWorld(context.Background())
	snapshot := poller.Snapshot()
	if !snapshot.ObjectsUnsupported || snapshot.ObjectsAvailable || snapshot.ObjectsStale {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if snapshot.ObjectsLastError != "unsupported" {
		t.Fatalf("last error = %q", snapshot.ObjectsLastError)
	}
}

func TestPollerWorldStatusTransitionsAreExplicit(t *testing.T) {
	source := &stubSource{objects: []WorldObject{{ID: "object:one", Kind: "bases", Name: "Home"}}}
	poller := testPoller(source)
	poller.refreshWorld(context.Background())

	source.objects = nil
	source.objectErr = &HTTPStatusError{Status: http.StatusNotFound}
	poller.refreshWorld(context.Background())
	unsupported := poller.ObjectSnapshot()
	if !unsupported.Available || !unsupported.Stale || !unsupported.Unsupported || unsupported.LastError != "unsupported" || len(unsupported.Objects) != 1 {
		t.Fatalf("unsupported snapshot = %#v", unsupported)
	}

	source.objectErr = errors.New("temporary failure")
	poller.refreshWorld(context.Background())
	failed := poller.ObjectSnapshot()
	if failed.Unsupported || !failed.Stale || failed.LastError != "refresh-failed" || len(failed.Objects) != 1 {
		t.Fatalf("failed snapshot = %#v", failed)
	}

	source.objectErr = nil
	poller.refreshWorld(context.Background())
	recovered := poller.ObjectSnapshot()
	if !recovered.Available || recovered.Stale || recovered.Unsupported || recovered.Truncated || recovered.LastError != "" || recovered.Total != 0 || recovered.Objects == nil {
		t.Fatalf("recovered snapshot = %#v", recovered)
	}
}

func TestPollerPublishesExplicitTruncatedWorldResult(t *testing.T) {
	objects := []WorldObject{{ID: "object:one", Kind: "npcs", Name: "Merchant"}}
	source := &stubSource{
		objects:   objects,
		objectErr: &WorldObjectLimitError{Limit: maxWorldObjects, Total: maxWorldObjects + 25},
	}
	poller := testPoller(source)
	poller.refreshWorld(context.Background())
	snapshot := poller.ObjectSnapshot()
	if !snapshot.Available || snapshot.Stale || snapshot.Unsupported || !snapshot.Truncated || snapshot.Total != maxWorldObjects+25 || snapshot.LastError != "object-limit" || len(snapshot.Objects) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestPollerCategorizesOversizedWorldResponse(t *testing.T) {
	source := &stubSource{objectErr: &ResponseSizeError{Limit: maxWorldResponseBytes}}
	poller := testPoller(source)
	poller.refreshWorld(context.Background())
	if got := poller.ObjectSnapshot().LastError; got != "response-too-large" {
		t.Fatalf("last error = %q", got)
	}
}

func TestPollerEmptySnapshotsUseNonNullSlices(t *testing.T) {
	poller := testPoller(&stubSource{})
	if poller.Snapshot().Players == nil || poller.Snapshot().Objects == nil || poller.PlayerSnapshot().Players == nil || poller.ObjectSnapshot().Objects == nil {
		t.Fatal("empty snapshot contains a nil slice")
	}
}
