// Package landmarks loads and validates the versioned encounter catalogue
// embedded in the application image.
package landmarks

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"regexp"
	"strings"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
)

const maxLocations = 1_000

var gameVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){2,3}$`)

// Metadata identifies the exact game catalogue and extraction tooling behind
// the embedded landmarks. It is exposed to clients so stale exports are not
// silently presented as current after a server update.
type Metadata struct {
	GameVersion string `json:"gameVersion"`
	Generator   string `json:"generator"`
	Decoder     string `json:"decoder"`
}

// Catalogue is the validated landmark export and its provenance summary.
type Catalogue struct {
	Metadata  Metadata
	Locations []palworld.WorldObject
}

type manifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	GameVersion   string       `json:"gameVersion"`
	Generator     string       `json:"generator"`
	Decoder       string       `json:"decoder"`
	Mappings      sourceFile   `json:"mappings"`
	Paks          []sourceFile `json:"paks"`
	Sources       []source     `json:"sources"`
	Locations     []record     `json:"locations"`
}

type sourceFile struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type source struct {
	Object  string `json:"object"`
	Purpose string `json:"purpose"`
}

type record struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Name   string  `json:"name"`
	Detail string  `json:"detail,omitempty"`
	Level  int     `json:"level,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// Load validates catalogue metadata and projects each entry into the same
// public object model used by live world actors.
func Load(source fs.FS) (Catalogue, error) {
	data, err := fs.ReadFile(source, "landmarks/manifest.json")
	if err != nil {
		return Catalogue{}, fmt.Errorf("read landmark manifest: %w", err)
	}
	var input manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Catalogue{}, fmt.Errorf("decode landmark manifest: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Catalogue{}, errors.New("decode landmark manifest: trailing data")
	}
	if input.SchemaVersion != 2 {
		return Catalogue{}, errors.New("unsupported landmark manifest schema")
	}
	input.GameVersion = strings.TrimSpace(input.GameVersion)
	if !gameVersionPattern.MatchString(input.GameVersion) {
		return Catalogue{}, errors.New("landmark manifest has no complete numeric game version")
	}
	if err := validateProvenance(input); err != nil {
		return Catalogue{}, err
	}
	if len(input.Locations) == 0 || len(input.Locations) > maxLocations {
		return Catalogue{}, fmt.Errorf("landmark manifest must contain 1-%d locations", maxLocations)
	}

	seen := make(map[string]struct{}, len(input.Locations))
	locations := make([]palworld.WorldObject, 0, len(input.Locations))
	for _, item := range input.Locations {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Detail = strings.TrimSpace(item.Detail)
		if item.ID == "" || len(item.ID) > 240 || item.Name == "" || len(item.Name) > 120 || len(item.Detail) > 240 {
			return Catalogue{}, fmt.Errorf("invalid landmark %q", item.ID)
		}
		if item.Kind != "alpha-pals" && item.Kind != "bosses" {
			return Catalogue{}, fmt.Errorf("landmark %q has unsupported kind %q", item.ID, item.Kind)
		}
		if item.Level < 0 || item.Level > 999 || math.IsNaN(item.X) || math.IsNaN(item.Y) || math.IsInf(item.X, 0) || math.IsInf(item.Y, 0) {
			return Catalogue{}, fmt.Errorf("landmark %q has invalid values", item.ID)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return Catalogue{}, fmt.Errorf("duplicate landmark ID %q", item.ID)
		}
		mapID, ok := mapdata.LayerID(item.X, item.Y)
		if !ok {
			return Catalogue{}, fmt.Errorf("landmark %q lies outside shipped maps", item.ID)
		}
		seen[item.ID] = struct{}{}
		locations = append(locations, palworld.WorldObject{
			ID: "landmark:" + item.ID, Kind: item.Kind, Name: item.Name,
			Detail: item.Detail, Level: item.Level, X: item.X, Y: item.Y, Map: mapID,
		})
	}
	return Catalogue{
		Metadata: Metadata{
			GameVersion: input.GameVersion,
			Generator:   strings.TrimSpace(input.Generator),
			Decoder:     strings.TrimSpace(input.Decoder),
		},
		Locations: locations,
	}, nil
}

func validateProvenance(input manifest) error {
	if strings.TrimSpace(input.Generator) == "" || strings.TrimSpace(input.Decoder) == "" {
		return errors.New("landmark manifest has incomplete generator provenance")
	}
	if !validSourceFile(input.Mappings) {
		return errors.New("landmark manifest has invalid mappings provenance")
	}
	if len(input.Paks) == 0 || len(input.Paks) > 128 {
		return errors.New("landmark manifest has invalid PAK provenance")
	}
	for _, pak := range input.Paks {
		if !validSourceFile(pak) {
			return errors.New("landmark manifest has invalid PAK provenance")
		}
	}
	if len(input.Sources) == 0 || len(input.Sources) > 32 {
		return errors.New("landmark manifest has invalid source provenance")
	}
	for _, item := range input.Sources {
		object := strings.TrimSpace(item.Object)
		purpose := strings.TrimSpace(item.Purpose)
		if object == "" || len(object) > 300 || purpose == "" || len(purpose) > 300 {
			return errors.New("landmark manifest has invalid source provenance")
		}
	}
	return nil
}

func validSourceFile(file sourceFile) bool {
	name := strings.TrimSpace(file.File)
	digest := strings.TrimSpace(file.SHA256)
	if name == "" || len(name) > 240 || file.Bytes <= 0 || len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}
