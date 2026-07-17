package palworld

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientInfoSanitizesUpstreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/info" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "admin-secret" {
			t.Fatalf("unexpected basic auth: %q %q %v", username, password, ok)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"servername":  " The Chaos\u0000 ",
			"description": " Official Palworld server. ",
			"version":     "v1.0.0",
			"worldguid":   "private-world-id",
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin-secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.Name != "The Chaos" || info.Description != "Official Palworld server." || info.Version != "v1.0.0" {
		t.Fatalf("info = %#v", info)
	}
}

func TestClientPlayersSanitizesUpstreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/players" {
			t.Errorf("path = %q", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "admin-secret" {
			t.Errorf("unexpected basic auth: %q %q %v", username, password, ok)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"players": []map[string]any{
				{"name": " Zed ", "level": 18, "location_x": 400000, "location_y": -900000, "user_id": "private-id", "ip": "192.0.2.1"},
				{"name": "Alice\u0000", "level": 42, "location_x": 500000, "location_y": -600000, "account_name": "private-account"},
				{"name": "   ", "level": 1, "location_x": 1, "location_y": 2},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/ignored-prefix", "admin-secret", time.Second, time.Second)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	players, err := client.Players(context.Background())
	if err != nil {
		t.Fatalf("Players() error = %v", err)
	}
	if len(players) != 2 {
		t.Fatalf("len(players) = %d", len(players))
	}
	if players[0].Name != "Alice" || players[0].Map != "world-tree" {
		t.Fatalf("players[0] = %#v", players[0])
	}
	if players[1].Name != "Zed" || players[1].Map != "palpagos" {
		t.Fatalf("players[1] = %#v", players[1])
	}
}

func TestClientPlayersReportsUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "wrong", time.Second, time.Second)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.Players(context.Background()); err == nil {
		t.Fatal("Players() error = nil")
	}
}

func TestNewClientRejectsUnsafeURL(t *testing.T) {
	if _, err := NewClient("file:///etc/passwd", "secret", time.Second, time.Second); err == nil {
		t.Fatal("NewClient() error = nil")
	}
}

func TestCleanTextTruncatesAtUTF8Boundary(t *testing.T) {
	tests := []struct {
		name  string
		value string
		limit int
		want  string
	}{
		{name: "two byte rune", value: "12345é", limit: 6, want: "12345"},
		{name: "three byte rune", value: "12345€", limit: 7, want: "12345"},
		{name: "four byte rune", value: "12345🌍", limit: 8, want: "12345"},
		{name: "complete rune", value: "12345é", limit: 7, want: "12345é"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cleanText(test.value, test.limit); got != test.want {
				t.Fatalf("cleanText(%q, %d) = %q, want %q", test.value, test.limit, got, test.want)
			}
		})
	}
}

func TestClientWorldObjectsSanitizesAndClassifiesActors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/game-data" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Time": "2026-07-17 10:00:00",
			"ActorData": []map[string]any{
				{"Type": "PalBox", "GuildName": "The Chaos", "LocationX": -100, "LocationY": 200, "ip": "private"},
				{"Type": "Character", "UnitType": "BaseCampPal", "NickName": "Anubis", "level": 44, "LocationX": -200, "LocationY": 300, "IsActive": "true", "userid": "private"},
				{"Type": "Character", "UnitType": "NPC", "Class": "BP_Desert_Trader_C", "LocationX": -300, "LocationY": 400},
				{"Type": "Character", "UnitType": "WildPal", "NickName": "Hidden", "LocationX": 1, "LocationY": 2, "IsActive": "false"},
				{"Type": "Character", "UnitType": "Player", "NickName": "Duplicate", "LocationX": 1, "LocationY": 2},
			},
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := client.WorldObjects(context.Background())
	if err != nil {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("objects = %#v", objects)
	}
	if objects[0].Kind != "bases" || objects[0].Name != "The Chaos" {
		t.Fatalf("base = %#v", objects[0])
	}
	if objects[1].Kind != "workers" || objects[1].Name != "Anubis" || objects[1].Level != 44 {
		t.Fatalf("worker = %#v", objects[1])
	}
	if objects[2].Kind != "npcs" || objects[2].Name != "Desert Trader" {
		t.Fatalf("npc = %#v", objects[2])
	}
}

func TestClientWorldObjectsReportsUnsupportedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WorldObjects(context.Background())
	var statusError *HTTPStatusError
	if !errors.As(err, &statusError) || statusError.Status != http.StatusNotFound {
		t.Fatalf("WorldObjects() error = %v", err)
	}
}
