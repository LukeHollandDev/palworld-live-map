// Package worldcatalogue loads and validates the modular static-location
// catalogue generated from Palworld's local game assets.
package worldcatalogue

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"math"
	"path"
	"regexp"
	"strings"

	"github.com/LukeHollandDev/palworld-live-map/internal/mapdata"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
)

const (
	catalogueRoot   = "palworld/landmarks/catalogue"
	manifestName    = "manifest.json"
	manifestSchema  = 1
	datasetSchema   = 1
	maxManifestSize = 1 << 20
	maxDatasetSize  = 32 << 20
	maxLocations    = 10_000
)

var (
	gameVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){2,3}$`)
	digestPattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	datasetSpecs       = []datasetSpec{
		{
			id: "encounter-additions", file: "encounter-additions.json",
			categories: map[string]struct{}{"bounties": {}, "oil-rigs": {}},
		},
		{
			id: "navigation", file: "navigation.json",
			categories: map[string]struct{}{"watchtowers": {}, "waypoints": {}, "dungeon-entrances": {}},
		},
		{
			id: "collectibles", file: "collectibles.json",
			categories: map[string]struct{}{"effigies": {}, "journals": {}, "ancient-shrine-pickups": {}},
		},
		{
			id: "npc-locations", file: "npc-locations.json",
			categories: map[string]struct{}{"npc-locations": {}},
		},
	}
)

// Catalogue is the public, validated projection of the richer generated
// datasets. ContentHash identifies the complete manifest-and-dataset payload
// and is intentionally omitted from the API representation.
type Catalogue struct {
	GameVersion string                 `json:"gameVersion"`
	Generator   string                 `json:"generator"`
	Decoder     string                 `json:"decoder"`
	Locations   []palworld.WorldObject `json:"locations"`
	ContentHash string                 `json:"-"`
}

type datasetSpec struct {
	id         string
	file       string
	categories map[string]struct{}
}

type manifest struct {
	SchemaVersion int                `json:"schemaVersion"`
	GameVersion   string             `json:"gameVersion"`
	Generator     string             `json:"generator"`
	Decoder       string             `json:"decoder"`
	Mappings      sourceFile         `json:"mappings"`
	Paks          []sourceFile       `json:"paks"`
	Sources       []source           `json:"sources"`
	Scan          *scan              `json:"scan"`
	Datasets      []datasetReference `json:"datasets"`
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

type scan struct {
	PersistentPackage      string      `json:"persistentPackage"`
	WorldPartitionPrefix   string      `json:"worldPartitionPrefix"`
	WorldPartitionPackages int         `json:"worldPartitionPackages"`
	NPCExclusions          []exclusion `json:"npcExclusions"`
}

type exclusion struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type datasetReference struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Count  int    `json:"count"`
	SHA256 string `json:"sha256"`
}

type dataset struct {
	SchemaVersion int              `json:"schemaVersion"`
	ID            string           `json:"id"`
	Locations     []locationRecord `json:"locations"`
}

type locationRecord struct {
	ID            string   `json:"id"`
	Category      string   `json:"category"`
	GameID        string   `json:"gameId"`
	Name          string   `json:"name"`
	X             float64  `json:"x"`
	Y             float64  `json:"y"`
	Z             float64  `json:"z"`
	Detail        *string  `json:"detail"`
	Level         *int     `json:"level"`
	InstanceID    *string  `json:"instanceId"`
	StateKey      *string  `json:"stateKey"`
	ClassName     *string  `json:"className"`
	IconSource    *string  `json:"iconSource"`
	Rewards       []reward `json:"rewards"`
	SourcePackage *string  `json:"sourcePackage"`
	SourceObject  *string  `json:"sourceObject"`
}

type reward struct {
	ItemID     string  `json:"itemId"`
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	IconSource *string `json:"iconSource"`
	NameSource string  `json:"nameSource"`
}

// Load verifies the manifest, every referenced dataset, and all public
// location fields before returning anything to the server.
func Load(sourceFS fs.FS) (Catalogue, error) {
	manifestPath := path.Join(catalogueRoot, manifestName)
	manifestData, err := readRegularFile(sourceFS, manifestPath, maxManifestSize)
	if err != nil {
		return Catalogue{}, fmt.Errorf("read world catalogue manifest: %w", err)
	}

	var input manifest
	if err := decodeStrict(manifestData, &input); err != nil {
		return Catalogue{}, fmt.Errorf("decode world catalogue manifest: %w", err)
	}
	specs, total, err := validateManifest(input)
	if err != nil {
		return Catalogue{}, err
	}

	contentHasher := sha256.New()
	writeHashPart(contentHasher, manifestName, manifestData)
	seenIDs := make(map[string]struct{}, total)
	seenInstances := make(map[string]struct{}, total)
	locations := make([]palworld.WorldObject, 0, total)

	for _, reference := range input.Datasets {
		spec := specs[reference.ID]
		datasetPath := path.Join(catalogueRoot, reference.File)
		data, err := readRegularFile(sourceFS, datasetPath, maxDatasetSize)
		if err != nil {
			return Catalogue{}, fmt.Errorf("read world catalogue dataset %q: %w", reference.ID, err)
		}
		actualDigest := sha256.Sum256(data)
		if hex.EncodeToString(actualDigest[:]) != reference.SHA256 {
			return Catalogue{}, fmt.Errorf("world catalogue dataset %q SHA-256 digest does not match manifest", reference.ID)
		}
		writeHashPart(contentHasher, reference.File, data)

		var inputDataset dataset
		if err := decodeStrict(data, &inputDataset); err != nil {
			return Catalogue{}, fmt.Errorf("decode world catalogue dataset %q: %w", reference.ID, err)
		}
		if inputDataset.SchemaVersion != datasetSchema {
			return Catalogue{}, fmt.Errorf("world catalogue dataset %q has unsupported schema", reference.ID)
		}
		if inputDataset.ID != reference.ID {
			return Catalogue{}, fmt.Errorf("world catalogue dataset %q contains ID %q", reference.ID, inputDataset.ID)
		}
		if inputDataset.Locations == nil || len(inputDataset.Locations) != reference.Count {
			return Catalogue{}, fmt.Errorf(
				"world catalogue dataset %q count is %d, manifest declares %d",
				reference.ID, len(inputDataset.Locations), reference.Count,
			)
		}

		for index := range inputDataset.Locations {
			location, instanceID, err := projectLocation(inputDataset.Locations[index], spec)
			if err != nil {
				return Catalogue{}, fmt.Errorf(
					"world catalogue dataset %q location %d: %w", reference.ID, index, err,
				)
			}
			if _, duplicate := seenIDs[location.ID]; duplicate {
				return Catalogue{}, fmt.Errorf("duplicate world catalogue location ID %q", location.ID)
			}
			if instanceID != "" {
				if _, duplicate := seenInstances[instanceID]; duplicate {
					return Catalogue{}, fmt.Errorf(
						"duplicate world catalogue instance ID %q", *inputDataset.Locations[index].InstanceID,
					)
				}
				seenInstances[instanceID] = struct{}{}
			}
			seenIDs[location.ID] = struct{}{}
			locations = append(locations, location)
		}
	}

	return Catalogue{
		GameVersion: input.GameVersion,
		Generator:   input.Generator,
		Decoder:     input.Decoder,
		Locations:   locations,
		ContentHash: hex.EncodeToString(contentHasher.Sum(nil)),
	}, nil
}

func validateManifest(input manifest) (map[string]datasetSpec, int, error) {
	if input.SchemaVersion != manifestSchema {
		return nil, 0, errors.New("unsupported world catalogue manifest schema")
	}
	if !gameVersionPattern.MatchString(input.GameVersion) {
		return nil, 0, errors.New("world catalogue manifest has no complete numeric game version")
	}
	if !validText(input.Generator, 200) || !validText(input.Decoder, 200) {
		return nil, 0, errors.New("world catalogue manifest has incomplete generator provenance")
	}
	if !validSourceFile(input.Mappings) {
		return nil, 0, errors.New("world catalogue manifest has invalid mappings provenance")
	}
	if len(input.Paks) == 0 || len(input.Paks) > 128 {
		return nil, 0, errors.New("world catalogue manifest has invalid PAK provenance")
	}
	seenFiles := map[string]struct{}{input.Mappings.File: {}}
	for _, pak := range input.Paks {
		if !validSourceFile(pak) {
			return nil, 0, errors.New("world catalogue manifest has invalid PAK provenance")
		}
		if _, duplicate := seenFiles[pak.File]; duplicate {
			return nil, 0, fmt.Errorf("world catalogue manifest has duplicate provenance file %q", pak.File)
		}
		seenFiles[pak.File] = struct{}{}
	}
	if len(input.Sources) == 0 || len(input.Sources) > 64 {
		return nil, 0, errors.New("world catalogue manifest has invalid source provenance")
	}
	seenSources := make(map[string]struct{}, len(input.Sources))
	for _, item := range input.Sources {
		if !validText(item.Object, 1_024) || !validText(item.Purpose, 500) {
			return nil, 0, errors.New("world catalogue manifest has invalid source provenance")
		}
		key := item.Object + "\x00" + item.Purpose
		if _, duplicate := seenSources[key]; duplicate {
			return nil, 0, errors.New("world catalogue manifest has duplicate source provenance")
		}
		seenSources[key] = struct{}{}
	}
	if err := validateScan(input.Scan); err != nil {
		return nil, 0, err
	}
	if len(input.Datasets) != len(datasetSpecs) {
		return nil, 0, fmt.Errorf(
			"world catalogue manifest must reference %d supported datasets", len(datasetSpecs),
		)
	}

	known := make(map[string]datasetSpec, len(datasetSpecs))
	for _, spec := range datasetSpecs {
		known[spec.id] = spec
	}
	referenced := make(map[string]datasetSpec, len(input.Datasets))
	seenDatasetFiles := make(map[string]struct{}, len(input.Datasets))
	total := 0
	for _, reference := range input.Datasets {
		spec, supported := known[reference.ID]
		if !supported {
			return nil, 0, fmt.Errorf("world catalogue manifest references unsupported dataset %q", reference.ID)
		}
		if _, duplicate := referenced[reference.ID]; duplicate {
			return nil, 0, fmt.Errorf("world catalogue manifest references duplicate dataset %q", reference.ID)
		}
		if reference.File != spec.file {
			return nil, 0, fmt.Errorf(
				"world catalogue dataset %q must use file %q", reference.ID, spec.file,
			)
		}
		if _, duplicate := seenDatasetFiles[reference.File]; duplicate {
			return nil, 0, fmt.Errorf("world catalogue manifest references duplicate dataset file %q", reference.File)
		}
		if reference.Count <= 0 || reference.Count > maxLocations || total > maxLocations-reference.Count {
			return nil, 0, fmt.Errorf("world catalogue dataset %q has invalid count", reference.ID)
		}
		if !digestPattern.MatchString(reference.SHA256) {
			return nil, 0, fmt.Errorf("world catalogue dataset %q has invalid SHA-256 digest", reference.ID)
		}
		referenced[reference.ID] = spec
		seenDatasetFiles[reference.File] = struct{}{}
		total += reference.Count
	}
	for _, spec := range datasetSpecs {
		if _, ok := referenced[spec.id]; !ok {
			return nil, 0, fmt.Errorf("world catalogue manifest does not reference dataset %q", spec.id)
		}
	}
	return referenced, total, nil
}

func validateScan(input *scan) error {
	if input == nil ||
		!validText(input.PersistentPackage, 1_024) ||
		!validText(input.WorldPartitionPrefix, 1_024) ||
		input.WorldPartitionPackages <= 0 ||
		input.WorldPartitionPackages > 1_000_000 ||
		input.NPCExclusions == nil ||
		len(input.NPCExclusions) > 64 {
		return errors.New("world catalogue manifest has invalid scan provenance")
	}
	seen := make(map[string]struct{}, len(input.NPCExclusions))
	for _, item := range input.NPCExclusions {
		if !validText(item.Reason, 500) || item.Count <= 0 || item.Count > 1_000_000 {
			return errors.New("world catalogue manifest has invalid scan provenance")
		}
		if _, duplicate := seen[item.Reason]; duplicate {
			return errors.New("world catalogue manifest has duplicate scan exclusion reason")
		}
		seen[item.Reason] = struct{}{}
	}
	return nil
}

func projectLocation(input locationRecord, spec datasetSpec) (palworld.WorldObject, string, error) {
	if !validText(input.ID, 500) {
		return palworld.WorldObject{}, "", errors.New("location has an invalid ID")
	}
	if _, supported := spec.categories[input.Category]; !supported {
		return palworld.WorldObject{}, "", fmt.Errorf(
			"location %q has unsupported category %q for dataset %q", input.ID, input.Category, spec.id,
		)
	}
	if !validText(input.GameID, 500) || !validText(input.Name, 500) {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has incomplete game identity", input.ID)
	}
	if !finite(input.X) || !finite(input.Y) || !finite(input.Z) {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has non-finite coordinates", input.ID)
	}
	mapID, ok := mapdata.LayerID(input.X, input.Y)
	if !ok {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q lies outside shipped maps", input.ID)
	}
	if input.Level != nil && (*input.Level < 0 || *input.Level > 999) {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid level", input.ID)
	}

	detail, err := optionalText(input.Detail, 1_000)
	if err != nil {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid detail", input.ID)
	}
	instanceID, err := optionalInstanceID(input.InstanceID)
	if err != nil {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid instance ID", input.ID)
	}
	for name, value := range map[string]*string{
		"state key": input.StateKey, "class name": input.ClassName, "icon source": input.IconSource,
	} {
		limit := 500
		if name == "icon source" {
			limit = 1_024
		}
		if _, err := optionalText(value, limit); err != nil {
			return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid %s", input.ID, name)
		}
	}
	if _, err := requiredOptionalText(input.SourcePackage, 1_024); err != nil {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid source package", input.ID)
	}
	if _, err := requiredOptionalText(input.SourceObject, 1_024); err != nil {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q has invalid source object", input.ID)
	}
	if err := validateRewards(input); err != nil {
		return palworld.WorldObject{}, "", fmt.Errorf("location %q: %w", input.ID, err)
	}

	level := 0
	if input.Level != nil {
		level = *input.Level
	}
	return palworld.WorldObject{
		ID: "catalogue:" + input.ID, Kind: input.Category, Name: input.Name,
		Detail: detail, Level: level, X: input.X, Y: input.Y, Map: mapID,
	}, instanceID, nil
}

func validateRewards(input locationRecord) error {
	if input.Category != "ancient-shrine-pickups" {
		if input.Rewards != nil {
			return errors.New("rewards are only supported for ancient-shrine pickups")
		}
		return nil
	}
	if len(input.Rewards) == 0 || len(input.Rewards) > 2 {
		return errors.New("ancient-shrine pickup has invalid rewards")
	}
	seen := make(map[string]struct{}, len(input.Rewards))
	for _, item := range input.Rewards {
		if !validText(item.ItemID, 500) ||
			!validText(item.Name, 500) ||
			!validText(item.NameSource, 200) ||
			item.Count <= 0 ||
			item.Count > 1_000_000 {
			return errors.New("ancient-shrine pickup has invalid reward")
		}
		if _, err := optionalText(item.IconSource, 1_024); err != nil {
			return errors.New("ancient-shrine pickup has invalid reward icon")
		}
		if _, duplicate := seen[item.ItemID]; duplicate {
			return fmt.Errorf("ancient-shrine pickup has duplicate reward %q", item.ItemID)
		}
		seen[item.ItemID] = struct{}{}
	}
	return nil
}

func validSourceFile(input sourceFile) bool {
	return validFilename(input.File) &&
		input.Bytes > 0 &&
		digestPattern.MatchString(input.SHA256)
}

func validFilename(value string) bool {
	return validText(value, 500) &&
		value != "." &&
		value != ".." &&
		!strings.ContainsAny(value, `/\`)
}

func validText(value string, maximum int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maximum
}

func optionalText(value *string, maximum int) (string, error) {
	if value == nil {
		return "", nil
	}
	if !validText(*value, maximum) {
		return "", errors.New("invalid text")
	}
	return *value, nil
}

func requiredOptionalText(value *string, maximum int) (string, error) {
	if value == nil {
		return "", errors.New("missing text")
	}
	return optionalText(value, maximum)
}

func optionalInstanceID(value *string) (string, error) {
	if value == nil {
		return "", nil
	}
	if !validText(*value, 64) {
		return "", errors.New("invalid instance ID")
	}
	normalized := strings.ReplaceAll(*value, "-", "")
	if len(normalized) != 32 {
		return "", errors.New("invalid instance ID")
	}
	decoded, err := hex.DecodeString(normalized)
	if err != nil {
		return "", errors.New("invalid instance ID")
	}
	allZero := true
	for _, item := range decoded {
		if item != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return "", errors.New("invalid instance ID")
	}
	return strings.ToLower(normalized), nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func readRegularFile(sourceFS fs.FS, name string, maximum int64) ([]byte, error) {
	file, err := sourceFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, errors.New("file is not a supported non-empty regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, errors.New("file exceeds supported size")
	}
	return data, nil
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func writeHashPart(destination hash.Hash, name string, data []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(name)))
	_, _ = destination.Write(size[:])
	_, _ = destination.Write([]byte(name))
	binary.BigEndian.PutUint64(size[:], uint64(len(data)))
	_, _ = destination.Write(size[:])
	_, _ = destination.Write(data)
}
