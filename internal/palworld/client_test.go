package palworld

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testPlayerID         = "11111111111111111111111111111111"
	testActorID          = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testPlayerInstanceID = testPlayerID + " : " + testActorID
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
				{"playerId": "private-player-id", "name": " Zed ", "level": 18, "location_x": -400000, "location_y": -300000, "user_id": "private-id", "ip": "192.0.2.1"},
				{"name": "Alice\u0000", "level": 42, "location_x": 500000, "location_y": -600000, "account_name": "private-account"},
				{"name": "   ", "level": 1, "location_x": 1, "location_y": 2},
				{"name": "Missing coordinate", "level": 1, "location_x": 1},
				{"name": "Missing level", "location_x": 1, "location_y": 2},
				{"name": "Null level", "level": nil, "location_x": 1, "location_y": 2},
				{"name": "Outside map", "level": 1, "location_x": 900000, "location_y": 900000},
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
	if players[0].Name != "Alice" || players[0].Map != "world-tree" || !players[0].Online {
		t.Fatalf("players[0] = %#v", players[0])
	}
	if players[1].Name != "Zed" || players[1].Map != "palpagos" {
		t.Fatalf("players[1] = %#v", players[1])
	}
	if !strings.HasPrefix(players[0].ID, "player:") || !strings.HasPrefix(players[1].ID, "player:") || players[0].ID == players[1].ID {
		t.Fatalf("player IDs are missing or not unique: %#v", players)
	}
	encoded, err := json.Marshal(players)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "private-player-id") || strings.Contains(string(encoded), "private-account") {
		t.Fatalf("players expose an upstream identifier: %s", encoded)
	}
}

func TestClientProjectsSaveGUIDsIntoRESTIdentityNamespaces(t *testing.T) {
	client, err := NewClient("http://palworld:8212", "secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	playerFromSave, ok := client.PublicPlayerID("00112233-4455-6677-8899-AABBCCDDEEFF")
	if !ok {
		t.Fatal("valid save player GUID was rejected")
	}
	playerFromREST := client.publicID("player", "player-id:00112233445566778899aabbccddeeff")
	if playerFromSave != playerFromREST {
		t.Fatalf("save player ID %q != REST player ID %q", playerFromSave, playerFromREST)
	}
	guild, ok := client.PublicGuildKey("FFEEDDCCBBAA99887766554433221100")
	if !ok || guild != client.publicID("guild", "ffeeddccbbaa99887766554433221100") {
		t.Fatalf("guild key = %q, ok = %v", guild, ok)
	}
	if _, ok := client.PublicPlayerID("../../not-a-guid"); ok {
		t.Fatal("invalid save player GUID was accepted")
	}
}

func TestClientLinksPlayersGuildsAndCompanionsWithOpaqueIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/api/game-data":
			_ = json.NewEncoder(w).Encode(map[string]any{"ActorData": []map[string]any{
				{
					"InstanceID": strings.ToUpper(testPlayerInstanceID), "Type": "Character", "UnitType": "Player",
					"userid": "PRIVATE-USER-ID", "GuildID": "PRIVATE-GUILD-ID", "GuildName": "The Chaos",
					"LocationX": 1, "LocationY": 2,
				},
				{
					"Type": "PalBox", "GuildID": "PRIVATE-GUILD-ID", "GuildName": "The Chaos",
					"LocationX": 10, "LocationY": 20,
				},
				{
					"InstanceID": "PRIVATE-WORKER", "Type": "Character", "UnitType": "BaseCampPal",
					"GuildID": "PRIVATE-GUILD-ID", "NickName": "Anubis", "LocationX": 11, "LocationY": 21,
				},
				{
					"InstanceID": "PRIVATE-COMPANION", "TrainerInstanceID": testPlayerInstanceID,
					"Type": "Character", "UnitType": "OtomoPal", "GuildID": "PRIVATE-GUILD-ID",
					"NickName": "Dazemu", "LocationX": 3, "LocationY": 4,
				},
			}})
		case "/v1/api/players":
			_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{
				{
					"playerId": strings.ToUpper(testPlayerID), "name": "Luke",
					"level": 55, "location_x": 1, "location_y": 2,
				},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin-secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := client.WorldObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	players, err := client.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].GuildName != "The Chaos" || players[0].GuildKey == "" {
		t.Fatalf("linked player = %#v", players)
	}
	var base, worker, companion *WorldObject
	for index := range objects {
		switch objects[index].Kind {
		case "bases":
			base = &objects[index]
		case "workers":
			worker = &objects[index]
		case "companions":
			companion = &objects[index]
		}
	}
	if base == nil || worker == nil || companion == nil {
		t.Fatalf("linked objects = %#v", objects)
	}
	if worker.GuildKey != players[0].GuildKey || worker.BaseID != base.ID {
		t.Fatalf("worker relation = %#v, base = %#v, player = %#v", worker, base, players[0])
	}
	if companion.OwnerID != players[0].ID || companion.GuildKey != players[0].GuildKey {
		t.Fatalf("companion relation = %#v, player = %#v", companion, players[0])
	}
	encoded, err := json.Marshal(struct {
		Players []Player
		Objects []WorldObject
	}{Players: players, Objects: objects})
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"PRIVATE-USER-ID", "PRIVATE-GUILD-ID", testPlayerID, testActorID} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(privateValue)) {
			t.Fatalf("linked response exposes upstream identifier %q: %s", privateValue, encoded)
		}
	}
}

func TestClientUsesCanonicalOwnerIdentityWhenPlayersOmitsPlayerID(t *testing.T) {
	worldRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/api/game-data":
			worldRequests++
			if worldRequests > 1 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ActorData": []map[string]any{
				{
					"InstanceID": testPlayerInstanceID, "Type": "Character", "UnitType": "Player",
					"userid": "user-only", "GuildID": "guild-one", "GuildName": "Guild One",
					"LocationX": 1, "LocationY": 2,
				},
				{
					"InstanceID": "companion-one", "TrainerInstanceID": testPlayerInstanceID,
					"Type": "Character", "UnitType": "OtomoPal", "NickName": "Pal",
					"LocationX": 3, "LocationY": 4,
				},
			}})
		case "/v1/api/players":
			_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{{
				"userId": "user-only", "name": "Player", "level": 10, "location_x": 1, "location_y": 2,
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := client.WorldObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	players, err := client.Players(context.Background())
	if err != nil || len(players) != 1 || len(objects) != 1 {
		t.Fatalf("Players() = %#v, WorldObjects() = %#v, err = %v", players, objects, err)
	}
	if objects[0].OwnerID == "" || objects[0].OwnerID != players[0].ID || players[0].GuildName != "Guild One" {
		t.Fatalf("owner alias did not resolve: player=%#v companion=%#v", players[0], objects[0])
	}
	linkedID := players[0].ID
	if _, err := client.WorldObjects(context.Background()); err == nil {
		t.Fatal("second WorldObjects() error = nil")
	}
	players, err = client.Players(context.Background())
	if err != nil || len(players) != 1 || players[0].ID != linkedID || players[0].GuildKey != "" || players[0].GuildName != "" {
		t.Fatalf("identity alias was not retained without stale guild data: players=%#v, err=%v", players, err)
	}
}

func TestClientClearsPlayerRelationshipsWhenGameDataFails(t *testing.T) {
	worldRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/api/game-data":
			worldRequests++
			if worldRequests > 1 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ActorData": []map[string]any{{
				"InstanceID": testPlayerInstanceID, "Type": "Character", "UnitType": "Player",
				"userid": "user-one", "GuildID": "guild-one", "GuildName": "Guild One",
				"LocationX": 1, "LocationY": 2,
			}}})
		case "/v1/api/players":
			_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{{
				"playerId": testPlayerID, "userId": "user-one", "name": "Player One",
				"level": 10, "location_x": 1, "location_y": 2,
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WorldObjects(context.Background()); err != nil {
		t.Fatal(err)
	}
	players, err := client.Players(context.Background())
	if err != nil || len(players) != 1 || players[0].GuildKey == "" {
		t.Fatalf("linked players = %#v, %v", players, err)
	}
	if _, err := client.WorldObjects(context.Background()); err == nil {
		t.Fatal("second WorldObjects() error = nil")
	}
	players, err = client.Players(context.Background())
	if err != nil || len(players) != 1 || players[0].GuildKey != "" || players[0].GuildName != "" {
		t.Fatalf("players retained stale relationships = %#v, %v", players, err)
	}
}

func TestPlayerRelationshipSnapshotIsIndependent(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8212", "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.setPlayerRelations(map[string]playerRelation{"player-id:one": {identity: "player-id:one", guildName: "First"}})
	snapshot := client.playerRelationsSnapshot()
	client.setPlayerRelations(map[string]playerRelation{"player-id:one": {identity: "player-id:one", guildName: "Second"}})
	if got := snapshot["player-id:one"].guildName; got != "First" {
		t.Fatalf("snapshot changed to %q", got)
	}
}

func TestWorldPlayerRelationsRejectAmbiguousActorInstances(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8212", "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	actors := []worldActor{
		{InstanceID: testPlayerInstanceID, UserID: "user-one", UnitType: "Player", IsActive: "true"},
		{InstanceID: testPlayerInstanceID, UnitType: "Player", IsActive: "true"},
	}
	owners, _ := client.worldPlayerRelations(actors)
	if len(owners) != 0 {
		t.Fatalf("ambiguous owners = %#v", owners)
	}
}

func TestPlayerIDFromInstanceValidatesObservedShape(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: strings.ToUpper(testPlayerInstanceID), want: testPlayerID},
		{value: testPlayerID, want: testPlayerID},
		{value: "not-a-guid:"},
		{value: testPlayerID + ":"},
		{value: testPlayerID + ":" + testActorID + ":extra"},
		{value: "g1111111111111111111111111111111 : " + testActorID},
		{value: testPlayerID + " : zaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, test := range tests {
		if got := playerIDFromInstance(test.value); got != test.want {
			t.Errorf("playerIDFromInstance(%q) = %q, want %q", test.value, got, test.want)
		}
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

func TestClientRejectsMissingRootCollections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Players(context.Background()); err == nil || !strings.Contains(err.Error(), "players") {
		t.Fatalf("Players() error = %v", err)
	}
	if _, err := client.WorldObjects(context.Background()); err == nil || !strings.Contains(err.Error(), "ActorData") {
		t.Fatalf("WorldObjects() error = %v", err)
	}
}

func TestClientPlayerIDsStayStableAsCoordinatesChange(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{
			{"playerId": "private-id", "name": "Moss", "level": 58, "location_x": requestCount, "location_y": -requestCount},
			{"name": "Legacy", "level": 12, "location_x": requestCount * 2, "location_y": requestCount * -2},
		}})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	first, err := client.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(second) != 2 || first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Fatalf("player IDs changed with coordinates: first=%#v second=%#v", first, second)
	}
}

func TestClientPlayersOmitAmbiguousIdentities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{
			{"name": "Legacy", "level": 10, "location_x": 1, "location_y": 2},
			{"name": "Legacy", "level": 11, "location_x": 3, "location_y": 4},
			{"playerId": "duplicate", "name": "One", "level": 12, "location_x": 5, "location_y": 6},
			{"playerId": "duplicate", "name": "Two", "level": 13, "location_x": 7, "location_y": 8},
		}})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	players, err := client.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 0 {
		t.Fatalf("ambiguous players were published: %#v", players)
	}
}

func TestClientPublicIDsAreKeyed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{
			{"playerId": "private-id", "name": "Moss", "level": 58, "location_x": 1, "location_y": 2},
		}})
	}))
	defer server.Close()
	firstClient, err := NewClient(server.URL, "first-secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	secondClient, err := NewClient(server.URL, "second-secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstClient.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondClient.Players(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID == second[0].ID {
		t.Fatalf("public IDs are not deployment-keyed: first=%#v second=%#v", first, second)
	}
}

func TestClientMetricsSanitizesUpstreamResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/metrics" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "admin-secret" {
			t.Fatalf("unexpected basic auth: %q %q %v", username, password, ok)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"currentplayernum": 2,
			"maxplayernum":     20,
			"serverfps":        59,
			"serverframetime":  16.74,
			"uptime":           26397,
			"basecampnum":      11,
			"days":             343,
		})
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "admin-secret", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := client.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if metrics.CurrentPlayers != 2 || metrics.MaxPlayers != 20 || metrics.ServerFPS != 59 ||
		metrics.ServerFrameTime != 16.74 || metrics.UptimeSeconds != 26397 || metrics.BaseCount != 11 || metrics.Days != 343 {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestClientMetricsRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "empty object", payload: `{}`, want: "currentplayernum"},
		{
			name:    "one omitted field",
			payload: `{"currentplayernum":0,"maxplayernum":32,"serverfps":60,"serverframetime":16.7,"uptime":0,"basecampnum":0}`,
			want:    "days",
		},
		{
			name:    "null field",
			payload: `{"currentplayernum":null,"maxplayernum":32,"serverfps":60,"serverframetime":16.7,"uptime":0,"basecampnum":0,"days":0}`,
			want:    "currentplayernum",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.payload))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "admin", time.Second, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Metrics(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Metrics() error = %v, want field %q", err, test.want)
			}
		})
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
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/game-data" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		requestCount++
		guildName := "The Chaos"
		baseInstance := "PRIVATE-BASE-INSTANCE"
		if requestCount > 1 {
			guildName = "Renamed Guild"
			baseInstance = "private-base-instance"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Time": "2026-07-17 10:00:00",
			"ActorData": []map[string]any{
				{"InstanceID": baseInstance, "Type": "PalBox", "GuildID": "PRIVATE-GUILD-ID", "GuildName": guildName, "LocationX": -100, "LocationY": 200, "ip": "private"},
				{"Type": "PalBox", "GuildID": "PRIVATE-GUILD-ID", "GuildName": guildName, "LocationX": -1000, "LocationY": 2000},
				{"InstanceID": "private-worker-instance", "Type": "Character", "UnitType": "BaseCampPal", "GuildID": "private-guild-id", "NickName": "Anubis", "level": 44, "LocationX": -200, "LocationY": 300, "IsActive": "true", "userid": "private"},
				{"InstanceID": "private-npc-instance", "Type": "Character", "UnitType": "NPC", "Class": "BP_Desert_Trader_C", "GuildID": "ambient-world-group", "LocationX": -300, "LocationY": 400},
				{"Type": "Character", "UnitType": "WildPal", "NickName": "Hidden", "LocationX": 1, "LocationY": 2, "IsActive": "false"},
				{"InstanceID": "invalid-active", "Type": "Character", "UnitType": "NPC", "NickName": "Invalid", "LocationX": 1, "LocationY": 2, "IsActive": "sometimes"},
				{"Type": "Character", "UnitType": "Player", "NickName": "Duplicate", "LocationX": 1, "LocationY": 2},
				{"Type": "PalBox", "GuildName": "Missing coordinates"},
				{"Type": "Character", "UnitType": "NPC", "NickName": "Outside map", "LocationX": 900000, "LocationY": 900000},
				{"Type": "Character", "UnitType": "WildPal", "NickName": "Unstable legacy actor", "LocationX": 1, "LocationY": 2},
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
	if len(objects) != 4 {
		t.Fatalf("objects = %#v", objects)
	}
	var primaryBase, secondBase, worker, npc *WorldObject
	for i := range objects {
		switch {
		case objects[i].Kind == "bases" && objects[i].X == -100:
			primaryBase = &objects[i]
		case objects[i].Kind == "bases":
			secondBase = &objects[i]
		case objects[i].Kind == "workers":
			worker = &objects[i]
		case objects[i].Kind == "npcs":
			npc = &objects[i]
		}
	}
	if primaryBase == nil || primaryBase.Name != "The Chaos" || primaryBase.ID == "" || primaryBase.BaseID != primaryBase.ID || primaryBase.GuildKey == "" {
		t.Fatalf("primary base = %#v", primaryBase)
	}
	if secondBase == nil || secondBase.ID == "" || secondBase.GuildKey != primaryBase.GuildKey || secondBase.BaseID == primaryBase.BaseID {
		t.Fatalf("second base = %#v", secondBase)
	}
	if worker == nil || worker.Name != "Anubis" || worker.Level != 44 || worker.BaseID != primaryBase.BaseID ||
		worker.GuildKey != primaryBase.GuildKey {
		t.Fatalf("worker = %#v", worker)
	}
	if npc == nil || npc.Name != "Desert Trader" || npc.GuildKey != "" {
		t.Fatalf("npc = %#v", npc)
	}
	encoded, err := json.Marshal(objects)
	if err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"private-guild-id", "private-base-instance", "private-worker-instance", "private-npc-instance"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("objects expose private upstream value %q: %s", privateValue, encoded)
		}
	}
	again, err := client.WorldObjects(context.Background())
	if err != nil || len(again) != len(objects) {
		t.Fatalf("second WorldObjects() = %#v, %v", again, err)
	}
	for i := range objects {
		if objects[i].ID != again[i].ID {
			t.Fatalf("object %d ID changed: %q != %q", i, objects[i].ID, again[i].ID)
		}
	}
}

func TestAssociateWorkersWithBasesRequiresStandardPerimeterGuildAndMap(t *testing.T) {
	if baseAssociationRadius != 3587.5 {
		t.Fatalf("base association radius = %g", baseAssociationRadius)
	}
	client, err := NewClient("http://127.0.0.1:8212", "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects := []parsedWorldObject{
		{
			identity: "base",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:base", Kind: "bases", X: 0, Y: 0, Map: "palpagos",
			},
		},
		{
			identity: "boundary-worker",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:boundary", Kind: "workers", X: baseAssociationRadius, Y: 0, Map: "palpagos",
			},
		},
		{
			identity: "outside-worker",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:outside", Kind: "workers", X: baseAssociationRadius + 0.001, Y: 0, Map: "palpagos",
			},
		},
		{
			identity: "other-guild-worker",
			guildID:  "guild-two",
			object: WorldObject{
				ID: "object:other-guild", Kind: "workers", X: 0, Y: 0, Map: "palpagos",
			},
		},
		{
			identity: "other-map-worker",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:other-map", Kind: "workers", X: 0, Y: 0, Map: "world-tree",
			},
		},
		{
			identity: "nearby-companion",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:companion", Kind: "companions", X: 0, Y: 0, Map: "palpagos",
			},
		},
		{
			identity: "nearby-wild-pal",
			guildID:  "guild-one",
			object: WorldObject{
				ID: "object:wild", Kind: "wild-pals", X: 0, Y: 0, Map: "palpagos",
			},
		},
	}

	client.associateWorkersWithBases(objects)

	if got := objects[1].object.BaseID; got != "object:base" {
		t.Fatalf("boundary worker BaseID = %q", got)
	}
	for _, index := range []int{2, 3, 4, 5, 6} {
		if got := objects[index].object.BaseID; got != "" {
			t.Errorf("object %q received BaseID %q", objects[index].identity, got)
		}
	}
}

func TestAssociateWorkersWithBasesChoosesNearestAndBreaksOverlapTiesDeterministically(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:8212", "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Put base-b first so the exact tie proves selection does not depend on
	// upstream or heap order.
	objects := []parsedWorldObject{
		{
			identity: "base-b", guildID: "guild",
			object: WorldObject{ID: "object:base-b", Kind: "bases", X: 2000, Y: 0, Map: "palpagos"},
		},
		{
			identity: "base-a", guildID: "guild",
			object: WorldObject{ID: "object:base-a", Kind: "bases", X: 0, Y: 0, Map: "palpagos"},
		},
		{
			identity: "nearest-worker", guildID: "guild",
			object: WorldObject{ID: "object:nearest", Kind: "workers", X: 1600, Y: 0, Map: "palpagos"},
		},
		{
			identity: "tie-worker", guildID: "guild",
			object: WorldObject{ID: "object:tie", Kind: "workers", X: 1000, Y: 0, Map: "palpagos"},
		},
	}

	client.associateWorkersWithBases(objects)

	if got := objects[2].object.BaseID; got != "object:base-b" {
		t.Errorf("nearest overlap BaseID = %q", got)
	}
	if got := objects[3].object.BaseID; got != "object:base-a" {
		t.Errorf("tied overlap BaseID = %q", got)
	}
}

func TestClientWorldObjectsOmitAmbiguousIdentities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ActorData": []map[string]any{
			{"InstanceID": "duplicate", "Type": "Character", "UnitType": "NPC", "NickName": "One", "LocationX": 1, "LocationY": 2},
			{"InstanceID": "duplicate", "Type": "Character", "UnitType": "NPC", "NickName": "Two", "LocationX": 3, "LocationY": 4},
			{"TrainerInstanceID": "owner", "Type": "Character", "UnitType": "OtomoPal", "Class": "BP_Pal_C", "NickName": "Pal", "LocationX": 5, "LocationY": 6},
			{"TrainerInstanceID": "owner", "Type": "Character", "UnitType": "OtomoPal", "Class": "BP_Pal_C", "NickName": "Pal", "LocationX": 7, "LocationY": 8},
			{"InstanceID": "unique", "Type": "Character", "UnitType": "NPC", "NickName": "Merchant", "LocationX": 9, "LocationY": 10},
		}})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := client.WorldObjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Name != "Merchant" {
		t.Fatalf("ambiguous world identities were published: %#v", objects)
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

func TestClientWorldObjectsReturnsExplicitPartialResultAtObjectLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ActorData":[`)
		for i := 0; i < maxWorldObjects; i++ {
			if i > 0 {
				_, _ = io.WriteString(w, ",")
			}
			_, _ = fmt.Fprintf(w, `{"InstanceID":"actor-%d","Type":"Character","UnitType":"NPC","NickName":"Merchant","LocationX":%d,"LocationY":0}`, i, i)
		}
		_, _ = io.WriteString(w, `,{"Type":"PalBox","GuildID":"guild","GuildName":"Priority base","LocationX":0,"LocationY":0}]}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", 5*time.Second, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := client.WorldObjects(context.Background())
	var limitError *WorldObjectLimitError
	if !errors.As(err, &limitError) || limitError.Total != maxWorldObjects+1 || limitError.Limit != maxWorldObjects {
		t.Fatalf("WorldObjects() error = %v", err)
	}
	if len(objects) != maxWorldObjects {
		t.Fatalf("len(objects) = %d, want %d", len(objects), maxWorldObjects)
	}
	if objects[0].Kind != "bases" || objects[0].Name != "Priority base" {
		t.Fatalf("priority object was not retained: first = %#v", objects[0])
	}
}

func TestClientWorldObjectsRejectsOversizedResponseBeforeReading(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxWorldResponseBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "admin", time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.WorldObjects(context.Background())
	var sizeError *ResponseSizeError
	if !errors.As(err, &sizeError) || sizeError.Limit != maxWorldResponseBytes {
		t.Fatalf("WorldObjects() error = %v", err)
	}
}

func TestMapForShippedBounds(t *testing.T) {
	tests := []struct {
		x, y  float64
		mapID string
	}{
		{x: 347351.5, y: -818197, mapID: "world-tree"},
		{x: 689148.5, y: -476400, mapID: "world-tree"},
		{x: -1099400, y: -724400, mapID: "palpagos"},
		{x: 349400, y: 724400, mapID: "palpagos"},
		{x: 900000, y: 900000, mapID: ""},
	}
	for _, test := range tests {
		if got := mapFor(test.x, test.y); got != test.mapID {
			t.Errorf("mapFor(%g, %g) = %q, want %q", test.x, test.y, got, test.mapID)
		}
	}
}
