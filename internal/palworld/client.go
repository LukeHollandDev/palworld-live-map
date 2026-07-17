package palworld

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20
const maxWorldResponseBytes = 32 << 20
const maxWorldObjects = 20_000

type Player struct {
	Name  string  `json:"name"`
	Level int     `json:"level"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Map   string  `json:"map"`
}

type WorldObject struct {
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Detail string  `json:"detail,omitempty"`
	Level  int     `json:"level,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Map    string  `json:"map"`
}

type HTTPStatusError struct {
	Status int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("upstream returned %s", http.StatusText(e.Status))
}

type Client struct {
	baseURL       *url.URL
	adminPassword string
	httpClient    *http.Client
	worldClient   *http.Client
}

type playersResponse struct {
	Players []struct {
		Name      string  `json:"name"`
		LocationX float64 `json:"location_x"`
		LocationY float64 `json:"location_y"`
		Level     int     `json:"level"`
	} `json:"players"`
}

type worldResponse struct {
	ActorData []struct {
		Type      string  `json:"Type"`
		UnitType  string  `json:"UnitType"`
		NickName  string  `json:"NickName"`
		GuildName string  `json:"GuildName"`
		Class     string  `json:"Class"`
		Level     int     `json:"level"`
		LocationX float64 `json:"LocationX"`
		LocationY float64 `json:"LocationY"`
		IsActive  string  `json:"IsActive"`
	} `json:"ActorData"`
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
		baseURL:       baseURL,
		adminPassword: adminPassword,
		httpClient:    &http.Client{Timeout: timeout, CheckRedirect: noRedirect},
		worldClient:   &http.Client{Timeout: worldTimeout, CheckRedirect: noRedirect},
	}, nil
}

func (c *Client) Players(ctx context.Context) ([]Player, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: "/v1/api/players"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create players request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("admin", c.adminPassword)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request players: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("request players: %w", &HTTPStatusError{Status: resp.StatusCode})
	}

	var payload playersResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode players response: %w", err)
	}

	players := make([]Player, 0, len(payload.Players))
	for _, raw := range payload.Players {
		name := cleanName(raw.Name)
		if name == "" || raw.Level < 0 || !finite(raw.LocationX) || !finite(raw.LocationY) {
			continue
		}
		players = append(players, Player{
			Name:  name,
			Level: raw.Level,
			X:     raw.LocationX,
			Y:     raw.LocationY,
			Map:   mapFor(raw.LocationX, raw.LocationY),
		})
	}
	sort.Slice(players, func(i, j int) bool {
		return strings.ToLower(players[i].Name) < strings.ToLower(players[j].Name)
	})

	return players, nil
}

func (c *Client) WorldObjects(ctx context.Context) ([]WorldObject, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: "/v1/api/game-data"})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create game-data request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("admin", c.adminPassword)

	resp, err := c.worldClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request game data: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("request game data: %w", &HTTPStatusError{Status: resp.StatusCode})
	}
	if resp.ContentLength > maxWorldResponseBytes {
		return nil, errors.New("game-data response exceeds 32 MiB limit")
	}
	limited := io.LimitReader(resp.Body, maxWorldResponseBytes+1)
	payloadBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read game-data response: %w", err)
	}
	if len(payloadBytes) > maxWorldResponseBytes {
		return nil, errors.New("game-data response exceeds 32 MiB limit")
	}
	var payload worldResponse
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("decode game-data response: %w", err)
	}

	objects := make([]WorldObject, 0, min(len(payload.ActorData), maxWorldObjects))
	for _, actor := range payload.ActorData {
		if len(objects) == maxWorldObjects {
			break
		}
		if strings.EqualFold(strings.TrimSpace(actor.IsActive), "false") || !finite(actor.LocationX) || !finite(actor.LocationY) {
			continue
		}
		kind := objectKind(actor.Type, actor.UnitType)
		if kind == "" || kind == "players" {
			continue
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
		objects = append(objects, WorldObject{
			Kind: kind, Name: name, Detail: detail, Level: max(actor.Level, 0),
			X: actor.LocationX, Y: actor.LocationY, Map: mapFor(actor.LocationX, actor.LocationY),
		})
	}
	return objects, nil
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
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value))
	if len(value) > 96 {
		value = value[:96]
	}
	return value
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func mapFor(x, y float64) string {
	if x >= 347351.5 && x <= 689148.5 && y >= -818197 && y <= -476400 {
		return "world-tree"
	}
	return "palpagos"
}
