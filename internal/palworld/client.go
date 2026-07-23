package palworld

import (
	"container/heap"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
)

const maxResponseBytes = 2 << 20
const maxWorldResponseBytes = 32 << 20
const maxWorldObjects = 20_000

const (
	// PalBaseCampModel.AreaRange uses Unreal centimetres. The standard Palworld
	// base radius is 3,500 units (35 m); tolerate small position/snapshot jitter
	// at the perimeter without claiming workers from elsewhere in the guild.
	standardBaseRadius      = 3500.0
	baseRadiusTolerance     = 0.025
	baseAssociationRadius   = standardBaseRadius * (1 + baseRadiusTolerance)
	baseAssociationRadiusSq = baseAssociationRadius * baseAssociationRadius
)

type Player struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	GuildKey  string  `json:"guildKey,omitempty"`
	GuildName string  `json:"guildName,omitempty"`
	Level     int     `json:"level"`
	Online    bool    `json:"online"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Map       string  `json:"map"`
}

type ServerInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
}

type ServerMetrics struct {
	CurrentPlayers  int     `json:"currentPlayers"`
	MaxPlayers      int     `json:"maxPlayers"`
	ServerFPS       int     `json:"serverFps"`
	ServerFrameTime float64 `json:"serverFrameTime"`
	UptimeSeconds   int64   `json:"uptimeSeconds"`
	BaseCount       int     `json:"baseCount"`
	Days            int     `json:"days"`
}

type WorldObject struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Detail   string  `json:"detail,omitempty"`
	BaseID   string  `json:"baseId,omitempty"`
	GuildKey string  `json:"guildKey,omitempty"`
	OwnerID  string  `json:"ownerId,omitempty"`
	Level    int     `json:"level,omitempty"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Map      string  `json:"map"`
}

type parsedWorldObject struct {
	identity string
	object   WorldObject
	guildID  string
}

type parsedPlayer struct {
	identity string
	player   Player
}

type retainedWorldObjects []parsedWorldObject

func (objects retainedWorldObjects) Len() int { return len(objects) }
func (objects retainedWorldObjects) Less(i, j int) bool {
	return worldObjectBetter(objects[j], objects[i])
}
func (objects retainedWorldObjects) Swap(i, j int) { objects[i], objects[j] = objects[j], objects[i] }
func (objects *retainedWorldObjects) Push(value any) {
	*objects = append(*objects, value.(parsedWorldObject))
}
func (objects *retainedWorldObjects) Pop() any {
	current := *objects
	last := current[len(current)-1]
	*objects = current[:len(current)-1]
	return last
}

type HTTPStatusError struct {
	Status int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("upstream returned %s", http.StatusText(e.Status))
}

// ResponseSizeError reports that an upstream response exceeded the configured
// safety limit. It intentionally contains no upstream response data.
type ResponseSizeError struct {
	Limit int64
}

func (e *ResponseSizeError) Error() string {
	return fmt.Sprintf("upstream response exceeds %d MiB limit", e.Limit/(1<<20))
}

// WorldObjectLimitError accompanies a usable, but explicitly truncated, world
// object result. Callers can publish the partial result while surfacing its
// incomplete status to clients.
type WorldObjectLimitError struct {
	Limit int
	Total int
}

func (e *WorldObjectLimitError) Error() string {
	return fmt.Sprintf("game-data contains %d supported objects; limited to %d", e.Total, e.Limit)
}

type Client struct {
	baseURL       *url.URL
	adminPassword string
	idKey         [sha256.Size]byte
	httpClient    *http.Client
	worldClient   *http.Client

	relationsMu     sync.RWMutex
	playerRelations map[string]playerRelation
}

type playerRelation struct {
	identity  string
	guildKey  string
	guildName string
}

type playerRecord struct {
	PlayerID  string   `json:"playerId"`
	UserID    string   `json:"userId"`
	Name      string   `json:"name"`
	LocationX *float64 `json:"location_x"`
	LocationY *float64 `json:"location_y"`
	Level     *int     `json:"level"`
}

type playersResponse struct {
	Players *[]playerRecord `json:"players"`
}

type infoResponse struct {
	Version     string `json:"version"`
	ServerName  string `json:"servername"`
	Description string `json:"description"`
}

type metricsResponse struct {
	CurrentPlayers  *int     `json:"currentplayernum"`
	MaxPlayers      *int     `json:"maxplayernum"`
	ServerFPS       *int     `json:"serverfps"`
	ServerFrameTime *float64 `json:"serverframetime"`
	UptimeSeconds   *int64   `json:"uptime"`
	BaseCount       *int     `json:"basecampnum"`
	Days            *int     `json:"days"`
}

type worldActor struct {
	InstanceID        string   `json:"InstanceID"`
	TrainerInstanceID string   `json:"TrainerInstanceID"`
	UserID            string   `json:"userid"`
	Type              string   `json:"Type"`
	UnitType          string   `json:"UnitType"`
	NickName          string   `json:"NickName"`
	GuildID           string   `json:"GuildID"`
	GuildName         string   `json:"GuildName"`
	Class             string   `json:"Class"`
	Level             int      `json:"level"`
	LocationX         *float64 `json:"LocationX"`
	LocationY         *float64 `json:"LocationY"`
	IsActive          string   `json:"IsActive"`
}

type worldResponse struct {
	ActorData *[]worldActor `json:"ActorData"`
}

func NewClient(rawURL, adminPassword string, timeout, worldTimeout time.Duration) (*Client, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Palworld REST URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("Palworld REST URL must use http or https")
	}
	if baseURL.Host == "" {
		return nil, errors.New("Palworld REST URL must include a host")
	}

	noRedirect := func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		baseURL:         baseURL,
		adminPassword:   adminPassword,
		idKey:           sha256.Sum256([]byte("palworld-live-map/public-id\x00" + baseURL.String() + "\x00" + adminPassword)),
		httpClient:      &http.Client{Timeout: timeout, CheckRedirect: noRedirect},
		worldClient:     &http.Client{Timeout: worldTimeout, CheckRedirect: noRedirect},
		playerRelations: make(map[string]playerRelation),
	}, nil
}

func (c *Client) Info(ctx context.Context) (ServerInfo, error) {
	var payload infoResponse
	if err := c.getJSON(ctx, c.httpClient, "/v1/api/info", "server info", maxResponseBytes, &payload); err != nil {
		return ServerInfo{}, err
	}
	name := cleanName(payload.ServerName)
	if name == "" {
		return ServerInfo{}, errors.New("server info response has no server name")
	}
	return ServerInfo{
		Name:        name,
		Description: cleanText(payload.Description, 256),
		Version:     cleanName(payload.Version),
	}, nil
}

func (c *Client) Players(ctx context.Context) ([]Player, error) {
	var payload playersResponse
	if err := c.getJSON(ctx, c.httpClient, "/v1/api/players", "players", maxResponseBytes, &payload); err != nil {
		return nil, err
	}
	if payload.Players == nil {
		return nil, errors.New("players response is missing players")
	}

	relations := c.playerRelationsSnapshot()
	candidates := make([]parsedPlayer, 0, len(*payload.Players))
	identityCounts := make(map[string]int, len(*payload.Players))
	for _, raw := range *payload.Players {
		name := cleanName(raw.Name)
		if name == "" || raw.Level == nil || *raw.Level < 0 || raw.LocationX == nil || raw.LocationY == nil ||
			!finite(*raw.LocationX) || !finite(*raw.LocationY) {
			continue
		}
		mapID := mapFor(*raw.LocationX, *raw.LocationY)
		if mapID == "" {
			continue
		}
		playerID := canonicalExternalID(raw.PlayerID)
		userID := canonicalExternalID(raw.UserID)
		relation, linked := relations["player-id:"+playerID]
		if !linked {
			relation, linked = relations["user-id:"+userID]
		}
		identity := ""
		if linked && relation.identity != "" {
			identity = relation.identity
		} else if playerID != "" {
			identity = "player-id:" + playerID
		} else if userID != "" {
			identity = "user-id:" + userID
		} else {
			// Older server builds can omit playerId. A name-based fallback stays
			// stable as the player moves and remains opaque on the public API.
			identity = "fallback-name:" + strings.ToLower(name)
		}
		identityCounts[identity]++
		player := Player{
			Name:   name,
			Level:  *raw.Level,
			Online: true,
			X:      *raw.LocationX,
			Y:      *raw.LocationY,
			Map:    mapID,
		}
		if linked {
			player.GuildKey = relation.guildKey
			player.GuildName = relation.guildName
		}
		candidates = append(candidates, parsedPlayer{identity: identity, player: player})
	}
	players := make([]Player, 0, len(candidates))
	for _, candidate := range candidates {
		// An occurrence number depends on upstream ordering and can transfer UI
		// state between two records. Omit identities that are not unambiguous.
		if identityCounts[candidate.identity] != 1 {
			continue
		}
		candidate.player.ID = c.publicID("player", candidate.identity)
		players = append(players, candidate.player)
	}
	sort.Slice(players, func(i, j int) bool {
		return strings.ToLower(players[i].Name) < strings.ToLower(players[j].Name)
	})

	return players, nil
}

func (c *Client) Metrics(ctx context.Context) (ServerMetrics, error) {
	var payload metricsResponse
	if err := c.getJSON(ctx, c.httpClient, "/v1/api/metrics", "server metrics", maxResponseBytes, &payload); err != nil {
		return ServerMetrics{}, err
	}
	missing := missingMetricFields(payload)
	if len(missing) > 0 {
		return ServerMetrics{}, fmt.Errorf("server metrics response is missing required fields: %s", strings.Join(missing, ", "))
	}
	if *payload.CurrentPlayers < 0 || *payload.MaxPlayers < 0 || *payload.ServerFPS < 0 ||
		*payload.ServerFrameTime < 0 || !finite(*payload.ServerFrameTime) ||
		*payload.UptimeSeconds < 0 || *payload.BaseCount < 0 || *payload.Days < 0 {
		return ServerMetrics{}, errors.New("server metrics response contains invalid values")
	}
	return ServerMetrics{
		CurrentPlayers: *payload.CurrentPlayers, MaxPlayers: *payload.MaxPlayers,
		ServerFPS: *payload.ServerFPS, ServerFrameTime: *payload.ServerFrameTime,
		UptimeSeconds: *payload.UptimeSeconds, BaseCount: *payload.BaseCount, Days: *payload.Days,
	}, nil
}

func (c *Client) WorldObjects(ctx context.Context) ([]WorldObject, error) {
	var payload worldResponse
	if err := c.getJSON(ctx, c.worldClient, "/v1/api/game-data", "game data", maxWorldResponseBytes, &payload); err != nil {
		c.clearPlayerGuildRelations()
		return nil, err
	}
	if payload.ActorData == nil {
		c.clearPlayerGuildRelations()
		return nil, errors.New("game data response is missing ActorData")
	}

	ownerIDs, relations := c.worldPlayerRelations(*payload.ActorData)
	identityCounts := make(map[string]int, min(len(*payload.ActorData), maxWorldObjects))
	for _, actor := range *payload.ActorData {
		if candidate, ok := parseWorldActor(actor); ok {
			identityCounts[candidate.identity]++
		}
	}
	parsed := make(retainedWorldObjects, 0, min(len(*payload.ActorData), maxWorldObjects))
	supportedCount := 0
	for _, actor := range *payload.ActorData {
		candidate, ok := parseWorldActor(actor)
		if !ok {
			continue
		}
		if identityCounts[candidate.identity] != 1 {
			continue
		}
		supportedCount++
		candidate.object.ID = c.publicID("object", candidate.identity)
		if candidate.guildID != "" && guildOwnedKind(candidate.object.Kind) {
			candidate.object.GuildKey = c.publicID("guild", canonicalExternalID(candidate.guildID))
		}
		if candidate.object.Kind == "companions" {
			candidate.object.OwnerID = ownerIDs[canonicalExternalID(actor.TrainerInstanceID)]
		}
		if parsed.Len() < maxWorldObjects {
			heap.Push(&parsed, candidate)
		} else if worldObjectBetter(candidate, parsed[0]) {
			parsed[0] = candidate
			heap.Fix(&parsed, 0)
		}
	}
	c.associateWorkersWithBases(parsed)
	sort.Slice(parsed, func(i, j int) bool {
		return worldObjectBetter(parsed[i], parsed[j])
	})
	objects := make([]WorldObject, len(parsed))
	for i := range parsed {
		objects[i] = parsed[i].object
	}
	c.setPlayerRelations(relations)
	if supportedCount > maxWorldObjects {
		return objects, &WorldObjectLimitError{Limit: maxWorldObjects, Total: supportedCount}
	}
	return objects, nil
}

// worldPlayerRelations builds joins from the game-data snapshot without
// publishing any upstream account, actor, or guild identifiers. Companion
// records refer to a player's game actor instance, so that player actor bridges
// the /players identities, guild membership, and companion ownership.
func (c *Client) worldPlayerRelations(actors []worldActor) (map[string]string, map[string]playerRelation) {
	userCounts := make(map[string]int)
	playerCounts := make(map[string]int)
	instanceCounts := make(map[string]int)
	for _, actor := range actors {
		if objectKind(actor.Type, actor.UnitType) != "players" || !worldActorActive(actor) {
			continue
		}
		userID := canonicalExternalID(actor.UserID)
		if userID != "" {
			userCounts[userID]++
		}
		if playerID := playerIDFromInstance(actor.InstanceID); playerID != "" {
			playerCounts[playerID]++
		}
		if instanceID := canonicalExternalID(actor.InstanceID); instanceID != "" {
			instanceCounts[instanceID]++
		}
	}

	ownerIDs := make(map[string]string)
	relations := make(map[string]playerRelation)
	for _, actor := range actors {
		if objectKind(actor.Type, actor.UnitType) != "players" || !worldActorActive(actor) {
			continue
		}
		userID := canonicalExternalID(actor.UserID)
		playerID := playerIDFromInstance(actor.InstanceID)
		uniqueUser := userID != "" && userCounts[userID] == 1
		uniquePlayer := playerID != "" && playerCounts[playerID] == 1
		guildID := canonicalExternalID(actor.GuildID)
		identity := ""
		if uniquePlayer {
			identity = "player-id:" + playerID
		} else if uniqueUser {
			identity = "user-id:" + userID
		}
		if identity == "" {
			continue
		}
		relation := playerRelation{identity: identity, guildName: cleanName(actor.GuildName)}
		if guildID != "" {
			relation.guildKey = c.publicID("guild", guildID)
		}
		if uniquePlayer {
			relations["player-id:"+playerID] = relation
		}
		if uniqueUser {
			relations["user-id:"+userID] = relation
		}
		instanceID := canonicalExternalID(actor.InstanceID)
		if instanceID != "" && instanceCounts[instanceID] == 1 {
			ownerIDs[instanceID] = c.publicID("player", identity)
		}
	}
	return ownerIDs, relations
}

func (c *Client) playerRelationsSnapshot() map[string]playerRelation {
	c.relationsMu.RLock()
	defer c.relationsMu.RUnlock()
	relations := make(map[string]playerRelation, len(c.playerRelations))
	for identity, relation := range c.playerRelations {
		relations[identity] = relation
	}
	return relations
}

func (c *Client) setPlayerRelations(relations map[string]playerRelation) {
	c.relationsMu.Lock()
	c.playerRelations = relations
	c.relationsMu.Unlock()
}

// Keep the canonical identity aliases used by retained object snapshots, while
// removing guild metadata that can no longer be claimed as current.
func (c *Client) clearPlayerGuildRelations() {
	c.relationsMu.Lock()
	defer c.relationsMu.Unlock()
	for alias, relation := range c.playerRelations {
		relation.guildKey = ""
		relation.guildName = ""
		c.playerRelations[alias] = relation
	}
}

func worldActorActive(actor worldActor) bool {
	active := strings.TrimSpace(actor.IsActive)
	return active == "" || strings.EqualFold(active, "true")
}

func parseWorldActor(actor worldActor) (parsedWorldObject, bool) {
	if !worldActorActive(actor) || actor.LocationX == nil || actor.LocationY == nil ||
		!finite(*actor.LocationX) || !finite(*actor.LocationY) {
		return parsedWorldObject{}, false
	}
	kind := objectKind(actor.Type, actor.UnitType)
	if kind == "" || kind == "players" {
		return parsedWorldObject{}, false
	}
	mapID := mapFor(*actor.LocationX, *actor.LocationY)
	if mapID == "" {
		return parsedWorldObject{}, false
	}
	name := cleanName(actor.NickName)
	detail := ""
	if kind == "bases" {
		name = cleanName(actor.GuildName)
		if name == "" {
			name = "Palbox"
		}
	} else {
		detail = humanizeClass(actor.Class)
		if name == "" {
			name = detail
			detail = ""
		}
	}
	if name == "" {
		name = objectKindLabel(kind)
	}
	guildID := canonicalExternalID(actor.GuildID)
	identity, ok := worldIdentity(actor, kind, guildID)
	if !ok {
		return parsedWorldObject{}, false
	}
	return parsedWorldObject{
		identity: identity,
		object: WorldObject{
			Kind: kind, Name: name, Detail: detail, Level: max(actor.Level, 0),
			X: *actor.LocationX, Y: *actor.LocationY, Map: mapID,
		},
		guildID: guildID,
	}, true
}

func worldIdentity(actor worldActor, kind, guildID string) (string, bool) {
	if instanceID := canonicalExternalID(actor.InstanceID); instanceID != "" {
		return instanceID, true
	}
	if kind == "bases" {
		// PalBox records do not promise InstanceID. Their world position is static,
		// so coordinates are a stable fallback rather than movement state.
		return strings.Join([]string{
			"palbox", guildID, strings.TrimSpace(actor.Class),
			strconv.FormatFloat(*actor.LocationX, 'g', -1, 64),
			strconv.FormatFloat(*actor.LocationY, 'g', -1, 64),
		}, "\x00"), true
	}
	owner := canonicalExternalID(actor.TrainerInstanceID)
	if owner == "" {
		owner = canonicalExternalID(actor.UserID)
	}
	if owner == "" {
		// Coordinates would make IDs churn as an actor moves. Omit legacy records
		// that provide neither an instance ID nor a stable owner identity.
		return "", false
	}
	return strings.Join([]string{
		"owned", kind, owner, guildID, strings.TrimSpace(actor.Class), cleanName(actor.NickName),
	}, "\x00"), true
}

func objectPriority(kind string) int {
	switch kind {
	case "bases":
		return 0
	case "workers":
		return 1
	case "companions":
		return 2
	case "npcs":
		return 3
	case "wild-pals":
		return 4
	default:
		return 5
	}
}

func guildOwnedKind(kind string) bool {
	return kind == "bases" || kind == "workers" || kind == "companions"
}

func worldObjectBetter(left, right parsedWorldObject) bool {
	leftPriority := objectPriority(left.object.Kind)
	rightPriority := objectPriority(right.object.Kind)
	if leftPriority != rightPriority {
		return leftPriority < rightPriority
	}
	return left.identity < right.identity
}

func (c *Client) getJSON(ctx context.Context, httpClient *http.Client, path, subject string, limit int64, target any) error {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: path})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("create %s request: %w", subject, err)
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("admin", c.adminPassword)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", subject, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("request %s: %w", subject, &HTTPStatusError{Status: resp.StatusCode})
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("read %s response: %w", subject, &ResponseSizeError{Limit: limit})
	}
	limited := &io.LimitedReader{R: resp.Body, N: limit + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(target); err != nil {
		if limited.N == 0 {
			return fmt.Errorf("read %s response: %w", subject, &ResponseSizeError{Limit: limit})
		}
		return fmt.Errorf("decode %s response: %w", subject, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if limited.N == 0 {
			return fmt.Errorf("read %s response: %w", subject, &ResponseSizeError{Limit: limit})
		}
		if err == nil {
			return fmt.Errorf("decode %s response: multiple JSON values", subject)
		}
		return fmt.Errorf("decode %s response: %w", subject, err)
	}
	if limited.N == 0 {
		return fmt.Errorf("read %s response: %w", subject, &ResponseSizeError{Limit: limit})
	}
	return nil
}

func missingMetricFields(payload metricsResponse) []string {
	fields := []struct {
		name    string
		missing bool
	}{
		{name: "currentplayernum", missing: payload.CurrentPlayers == nil},
		{name: "maxplayernum", missing: payload.MaxPlayers == nil},
		{name: "serverfps", missing: payload.ServerFPS == nil},
		{name: "serverframetime", missing: payload.ServerFrameTime == nil},
		{name: "uptime", missing: payload.UptimeSeconds == nil},
		{name: "basecampnum", missing: payload.BaseCount == nil},
		{name: "days", missing: payload.Days == nil},
	}
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.missing {
			missing = append(missing, field.name)
		}
	}
	return missing
}

func (c *Client) associateWorkersWithBases(objects []parsedWorldObject) {
	type guildMap struct {
		guildID string
		mapID   string
	}
	basesByGuildMap := make(map[guildMap][]int)
	for i := range objects {
		if objects[i].object.Kind == "bases" {
			if objects[i].guildID != "" {
				objects[i].object.GuildKey = c.publicID("guild", canonicalExternalID(objects[i].guildID))
			}
			objects[i].object.BaseID = objects[i].object.ID
			key := guildMap{guildID: objects[i].guildID, mapID: objects[i].object.Map}
			basesByGuildMap[key] = append(basesByGuildMap[key], i)
		}
	}
	for i := range objects {
		if objects[i].object.Kind != "workers" || objects[i].guildID == "" {
			continue
		}
		nearest := -1
		nearestDistanceSq := math.Inf(1)
		key := guildMap{guildID: objects[i].guildID, mapID: objects[i].object.Map}
		for _, candidate := range basesByGuildMap[key] {
			base := objects[candidate]
			deltaX := objects[i].object.X - base.object.X
			deltaY := objects[i].object.Y - base.object.Y
			distanceSq := deltaX*deltaX + deltaY*deltaY
			if distanceSq > baseAssociationRadiusSq {
				continue
			}
			if nearest < 0 || distanceSq < nearestDistanceSq ||
				(distanceSq == nearestDistanceSq && base.identity < objects[nearest].identity) {
				nearest = candidate
				nearestDistanceSq = distanceSq
			}
		}
		if nearest >= 0 {
			objects[i].object.BaseID = objects[nearest].object.BaseID
		}
	}
}

func (c *Client) publicID(namespace string, parts ...string) string {
	hash := hmac.New(sha256.New, c.idKey[:])
	writeIDParts(hash, namespace, parts...)
	return fmt.Sprintf("%s:%x", namespace, hash.Sum(nil)[:16])
}

func opaqueID(namespace string, parts ...string) string {
	hash := sha256.New()
	writeIDParts(hash, namespace, parts...)
	return fmt.Sprintf("%s:%x", namespace, hash.Sum(nil)[:16])
}

func writeIDParts(hash io.Writer, namespace string, parts ...string) {
	var size [8]byte
	for _, part := range append([]string{namespace}, parts...) {
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(part))
	}
}

func objectKind(actorType, unitType string) string {
	if strings.EqualFold(strings.TrimSpace(actorType), "PalBox") {
		return "bases"
	}
	switch strings.ToLower(strings.TrimSpace(unitType)) {
	case "player":
		return "players"
	case "basecamppal":
		return "workers"
	case "wildpal":
		return "wild-pals"
	case "otomopal":
		return "companions"
	case "npc":
		return "npcs"
	default:
		return ""
	}
}

func objectKindLabel(kind string) string {
	switch kind {
	case "workers":
		return "Base worker"
	case "wild-pals":
		return "Wild Pal"
	case "companions":
		return "Companion Pal"
	case "npcs":
		return "NPC"
	default:
		return "Map object"
	}
}

func humanizeClass(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, "_C"))
	value = strings.TrimPrefix(value, "BP_")
	value = strings.ReplaceAll(value, "_", " ")
	return cleanName(value)
}

func cleanName(value string) string {
	return cleanText(value, 96)
}

func canonicalExternalID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isHexID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// Current Player actor instance IDs start with the playerId returned by
// /players, separated from another 32-character hexadecimal ID by a colon.
func playerIDFromInstance(value string) string {
	value = canonicalExternalID(value)
	parts := strings.Split(value, ":")
	if len(parts) == 1 && isHexID(parts[0]) {
		return parts[0]
	}
	if len(parts) != 2 {
		return ""
	}
	playerID := strings.TrimSpace(parts[0])
	actorID := strings.TrimSpace(parts[1])
	if !isHexID(playerID) || !isHexID(actorID) {
		return ""
	}
	return playerID
}

func cleanText(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value))
	if len(value) > limit {
		value = value[:limit]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func mapFor(x, y float64) string {
	id, _ := mapdata.LayerID(x, y)
	return id
}
