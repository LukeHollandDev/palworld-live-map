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
	objects   []WorldObject
	objectErr error
}

func (s *stubSource) Info(context.Context) (ServerInfo, error) {
	return s.info, s.infoErr
}

func (s *stubSource) Players(context.Context) ([]Player, error) {
	return append([]Player(nil), s.players...), s.playerErr
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
		objects: []WorldObject{{Kind: "bases", Name: "Home"}},
	}
	poller := testPoller(source)
	poller.refreshInfo(context.Background())
	poller.refreshPlayers(context.Background())
	poller.refreshWorld(context.Background())
	first := poller.Snapshot()
	if first.Server.Name != "The Chaos" || !first.Connected || first.Stale || !first.ObjectsAvailable || len(first.Objects) != 1 {
		t.Fatalf("first snapshot = %#v", first)
	}

	source.players = nil
	source.info = ServerInfo{}
	source.infoErr = errors.New("offline")
	source.playerErr = errors.New("offline")
	source.objects = nil
	source.objectErr = errors.New("offline")
	poller.refreshInfo(context.Background())
	poller.refreshPlayers(context.Background())
	poller.refreshWorld(context.Background())
	stale := poller.Snapshot()
	if stale.Server.Name != "The Chaos" || stale.Connected || !stale.Stale || !stale.ObjectsStale || len(stale.Players) != 1 || len(stale.Objects) != 1 {
		t.Fatalf("stale snapshot = %#v", stale)
	}

	stale.Players[0].Name = "mutated"
	stale.Objects[0].Name = "mutated"
	copy := poller.Snapshot()
	if copy.Players[0].Name != "Lamball" || copy.Objects[0].Name != "Home" {
		t.Fatal("Snapshot returned shared storage")
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
}
