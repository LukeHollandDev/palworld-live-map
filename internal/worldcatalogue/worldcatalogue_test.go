package worldcatalogue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadProjectsCompleteModularCatalogue(t *testing.T) {
	files := validCatalogueFS(t)
	catalogue, err := Load(files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if catalogue.GameVersion != "1.0.1.100619" ||
		catalogue.Generator != "palworld-asset-exporter/test" ||
		catalogue.Decoder != "CUE4Parse/test" {
		t.Fatalf("catalogue provenance = %#v", catalogue)
	}
	if len(catalogue.Locations) != 9 {
		t.Fatalf("locations = %d, want 9", len(catalogue.Locations))
	}
	first := catalogue.Locations[0]
	if first.ID != "catalogue:bounty:test" ||
		first.Kind != "bounties" ||
		first.Name != "Test bounty" ||
		first.Detail != "Bounty" ||
		first.Level != 30 ||
		first.Map != "palpagos" {
		t.Fatalf("first location = %#v", first)
	}
	last := catalogue.Locations[len(catalogue.Locations)-1]
	if last.ID != "catalogue:npc-location:test" || last.Map != "world-tree" {
		t.Fatalf("last location = %#v", last)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(catalogue.ContentHash) {
		t.Fatalf("content hash = %q", catalogue.ContentHash)
	}

	again, err := Load(files)
	if err != nil {
		t.Fatal(err)
	}
	if again.ContentHash != catalogue.ContentHash {
		t.Fatalf("repeat content hash = %q, want %q", again.ContentHash, catalogue.ContentHash)
	}

	encoded, err := json.Marshal(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	if len(public) != 4 ||
		public["gameVersion"] == nil ||
		public["generator"] == nil ||
		public["decoder"] == nil ||
		public["locations"] == nil ||
		strings.Contains(string(encoded), "contentHash") ||
		strings.Contains(string(encoded), "sourcePackage") {
		t.Fatalf("public catalogue JSON = %s", encoded)
	}
}

func TestLoadContentHashCoversDatasets(t *testing.T) {
	original, err := Load(validCatalogueFS(t))
	if err != nil {
		t.Fatal(err)
	}
	changedFiles := validCatalogueFS(t)
	editDataset(t, changedFiles, "encounter-additions", func(value *dataset) {
		value.Locations[0].Name = "Changed bounty"
	})
	changed, err := Load(changedFiles)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ContentHash == original.ContentHash {
		t.Fatalf("content hash did not change: %s", original.ContentHash)
	}
}

func TestLoadRejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, fstest.MapFS)
		want   string
	}{
		{
			name: "unknown field",
			mutate: func(t *testing.T, files fstest.MapFS) {
				var value map[string]any
				decodeFile(t, files, manifestPath(), &value)
				value["unexpected"] = true
				replaceJSON(t, files, manifestPath(), value)
			},
			want: "unknown field",
		},
		{
			name: "trailing JSON",
			mutate: func(_ *testing.T, files fstest.MapFS) {
				files[manifestPath()].Data = append(files[manifestPath()].Data, []byte(`{}`)...)
			},
			want: "trailing data",
		},
		{
			name: "schema",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.SchemaVersion++ })
			},
			want: "schema",
		},
		{
			name: "game version",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.GameVersion = "latest" })
			},
			want: "numeric game version",
		},
		{
			name: "generator provenance",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Generator = "" })
			},
			want: "generator provenance",
		},
		{
			name: "mappings digest",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Mappings.SHA256 = strings.ToUpper(value.Mappings.SHA256)
				})
			},
			want: "mappings provenance",
		},
		{
			name: "duplicate pak",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Paks = append(value.Paks, value.Paks[0])
				})
			},
			want: "duplicate provenance file",
		},
		{
			name: "source provenance",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Sources[0].Purpose = "" })
			},
			want: "source provenance",
		},
		{
			name: "scan omitted",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Scan = nil })
			},
			want: "scan provenance",
		},
		{
			name: "scan package count",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Scan.WorldPartitionPackages = 0 })
			},
			want: "scan provenance",
		},
		{
			name: "scan exclusions omitted",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Scan.NPCExclusions = nil })
			},
			want: "scan provenance",
		},
		{
			name: "missing dataset",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Datasets = value.Datasets[:len(value.Datasets)-1]
				})
			},
			want: "4 supported datasets",
		},
		{
			name: "unsupported dataset",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Datasets[0].ID = "players" })
			},
			want: "unsupported dataset",
		},
		{
			name: "duplicate dataset",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Datasets[1] = value.Datasets[0]
				})
			},
			want: "duplicate dataset",
		},
		{
			name: "unexpected dataset file",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Datasets[0].File = "../encounter-additions.json"
				})
			},
			want: "must use file",
		},
		{
			name: "dataset count",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) { value.Datasets[0].Count++ })
			},
			want: "count is",
		},
		{
			name: "dataset digest",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editManifest(t, files, func(value *manifest) {
					value.Datasets[0].SHA256 = strings.Repeat("g", 64)
				})
			},
			want: "invalid SHA-256",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := validCatalogueFS(t)
			test.mutate(t, files)
			_, err := Load(files)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidDatasetEnvelope(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, fstest.MapFS)
		want   string
	}{
		{
			name: "missing file",
			mutate: func(_ *testing.T, files fstest.MapFS) {
				delete(files, datasetPath("navigation"))
			},
			want: "read world catalogue dataset",
		},
		{
			name: "hash mismatch",
			mutate: func(_ *testing.T, files fstest.MapFS) {
				files[datasetPath("navigation")].Data = append(
					files[datasetPath("navigation")].Data, '\n',
				)
			},
			want: "SHA-256 digest does not match",
		},
		{
			name: "unknown field",
			mutate: func(t *testing.T, files fstest.MapFS) {
				var value map[string]any
				decodeFile(t, files, datasetPath("navigation"), &value)
				value["unexpected"] = true
				replaceDatasetJSON(t, files, "navigation", value)
			},
			want: "unknown field",
		},
		{
			name: "schema",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) { value.SchemaVersion++ })
			},
			want: "unsupported schema",
		},
		{
			name: "ID",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) { value.ID = "collectibles" })
			},
			want: "contains ID",
		},
		{
			name: "count",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations = value.Locations[:len(value.Locations)-1]
				})
			},
			want: "count is",
		},
		{
			name: "trailing JSON",
			mutate: func(t *testing.T, files fstest.MapFS) {
				name := datasetPath("navigation")
				data := append([]byte(nil), files[name].Data...)
				data = append(data, []byte(`{}`)...)
				replaceDatasetData(t, files, "navigation", data)
			},
			want: "trailing data",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := validCatalogueFS(t)
			test.mutate(t, files)
			_, err := Load(files)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadRejectsInvalidLocations(t *testing.T) {
	validGUID := "11111111-22222222-33333333-44444444"
	tests := []struct {
		name   string
		mutate func(*testing.T, fstest.MapFS)
		want   string
	}{
		{
			name: "unsupported category",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "encounter-additions", func(value *dataset) {
					value.Locations[0].Category = "players"
				})
			},
			want: "unsupported category",
		},
		{
			name: "category in wrong dataset",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "encounter-additions", func(value *dataset) {
					value.Locations[0].Category = "waypoints"
				})
			},
			want: "unsupported category",
		},
		{
			name: "outside map",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations[0].X = 9_999_999
					value.Locations[0].Y = 9_999_999
				})
			},
			want: "outside shipped maps",
		},
		{
			name: "invalid level",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "encounter-additions", func(value *dataset) {
					value.Locations[0].Level = intPointer(-1)
				})
			},
			want: "invalid level",
		},
		{
			name: "invalid instance ID",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations[0].InstanceID = stringPointer("not-a-guid")
				})
			},
			want: "invalid instance ID",
		},
		{
			name: "missing record provenance",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations[0].SourceObject = nil
				})
			},
			want: "invalid source object",
		},
		{
			name: "duplicate ID",
			mutate: func(t *testing.T, files fstest.MapFS) {
				var encounters dataset
				decodeFile(t, files, datasetPath("encounter-additions"), &encounters)
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations[0].ID = encounters.Locations[0].ID
				})
			},
			want: "duplicate world catalogue location ID",
		},
		{
			name: "duplicate normalized instance ID",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "navigation", func(value *dataset) {
					value.Locations[0].InstanceID = &validGUID
					value.Locations[1].InstanceID = stringPointer(strings.ReplaceAll(strings.ToLower(validGUID), "-", ""))
				})
			},
			want: "duplicate world catalogue instance ID",
		},
		{
			name: "rewards on other category",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "encounter-additions", func(value *dataset) {
					value.Locations[0].Rewards = []reward{validReward()}
				})
			},
			want: "rewards are only supported",
		},
		{
			name: "missing shrine rewards",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "collectibles", func(value *dataset) {
					for index := range value.Locations {
						if value.Locations[index].Category == "ancient-shrine-pickups" {
							value.Locations[index].Rewards = nil
						}
					}
				})
			},
			want: "invalid rewards",
		},
		{
			name: "invalid shrine reward count",
			mutate: func(t *testing.T, files fstest.MapFS) {
				editDataset(t, files, "collectibles", func(value *dataset) {
					for index := range value.Locations {
						if value.Locations[index].Category == "ancient-shrine-pickups" {
							value.Locations[index].Rewards[0].Count = 0
						}
					}
				})
			},
			want: "invalid reward",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := validCatalogueFS(t)
			test.mutate(t, files)
			_, err := Load(files)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func validCatalogueFS(t *testing.T) fstest.MapFS {
	t.Helper()
	sourcePackage := "/Game/Test/Fixture.Fixture"
	sourceObject := "Fixture"
	detail := "Bounty"
	level := 30
	location := func(id, category, name string, x, y float64) locationRecord {
		return locationRecord{
			ID: id, Category: category, GameID: "game:" + id, Name: name,
			X: x, Y: y, Z: 100,
			SourcePackage: &sourcePackage, SourceObject: &sourceObject,
		}
	}

	encounterBounty := location("bounty:test", "bounties", "Test bounty", 0, 0)
	encounterBounty.Detail = &detail
	encounterBounty.Level = &level
	encounters := dataset{
		SchemaVersion: datasetSchema,
		ID:            "encounter-additions",
		Locations: []locationRecord{
			encounterBounty,
			location("oil-rig:test", "oil-rigs", "Test oil rig", 1, 1),
		},
	}
	navigation := dataset{
		SchemaVersion: datasetSchema,
		ID:            "navigation",
		Locations: []locationRecord{
			location("watchtower:test", "watchtowers", "Test watchtower", 2, 2),
			location("waypoint:test", "waypoints", "Test waypoint", 3, 3),
			location("dungeon:test", "dungeon-entrances", "Test dungeon", 4, 4),
		},
	}
	shrine := location("ancient-shrine:test", "ancient-shrine-pickups", "Test schematic", 7, 7)
	shrine.Rewards = []reward{validReward()}
	collectibles := dataset{
		SchemaVersion: datasetSchema,
		ID:            "collectibles",
		Locations: []locationRecord{
			location("effigy:test", "effigies", "Test effigy", 5, 5),
			location("journal:test", "journals", "Test journal", 6, 6),
			shrine,
		},
	}
	npcs := dataset{
		SchemaVersion: datasetSchema,
		ID:            "npc-locations",
		Locations: []locationRecord{
			location("npc-location:test", "npc-locations", "Test NPC", 500_000, -600_000),
		},
	}
	inputDatasets := []dataset{encounters, navigation, collectibles, npcs}
	files := make(fstest.MapFS, len(inputDatasets)+1)
	references := make([]datasetReference, 0, len(inputDatasets))
	for _, value := range inputDatasets {
		data := marshalJSON(t, value)
		digest := sha256.Sum256(data)
		spec := specByID(t, value.ID)
		files[pathFor(spec.file)] = &fstest.MapFile{Data: data}
		references = append(references, datasetReference{
			ID: value.ID, File: spec.file, Count: len(value.Locations),
			SHA256: hex.EncodeToString(digest[:]),
		})
	}
	inputManifest := manifest{
		SchemaVersion: manifestSchema,
		GameVersion:   "1.0.1.100619",
		Generator:     "palworld-asset-exporter/test",
		Decoder:       "CUE4Parse/test",
		Mappings: sourceFile{
			File: "Mappings.usmap", Bytes: 100, SHA256: strings.Repeat("a", 64),
		},
		Paks: []sourceFile{
			{File: "Pal-Windows.pak", Bytes: 1_000, SHA256: strings.Repeat("b", 64)},
		},
		Sources: []source{
			{Object: "/Game/Test/Fixture.Fixture", Purpose: "test fixture"},
		},
		Scan: &scan{
			PersistentPackage:      "/Game/Pal/Maps/MainWorld_5/PL_MainWorld5.PL_MainWorld5",
			WorldPartitionPrefix:   "Pal/Content/Pal/Maps/MainWorld_5/PL_MainWorld5/_Generated_/*.umap",
			WorldPartitionPackages: 9_977,
			NPCExclusions:          []exclusion{},
		},
		Datasets: references,
	}
	files[manifestPath()] = &fstest.MapFile{Data: marshalJSON(t, inputManifest)}
	return files
}

func validReward() reward {
	icon := "/Game/Pal/Texture/Item/T_Test.T_Test"
	return reward{
		ItemID: "Blueprint_Test", Name: "Test schematic", Count: 1,
		IconSource: &icon, NameSource: "localized",
	}
}

func editManifest(t *testing.T, files fstest.MapFS, mutate func(*manifest)) {
	t.Helper()
	var value manifest
	decodeFile(t, files, manifestPath(), &value)
	mutate(&value)
	replaceJSON(t, files, manifestPath(), value)
}

func editDataset(t *testing.T, files fstest.MapFS, id string, mutate func(*dataset)) {
	t.Helper()
	var value dataset
	decodeFile(t, files, datasetPath(id), &value)
	mutate(&value)
	replaceDatasetJSON(t, files, id, value)
}

func replaceDatasetJSON(t *testing.T, files fstest.MapFS, id string, value any) {
	t.Helper()
	replaceDatasetData(t, files, id, marshalJSON(t, value))
}

func replaceDatasetData(t *testing.T, files fstest.MapFS, id string, data []byte) {
	t.Helper()
	files[datasetPath(id)] = &fstest.MapFile{Data: data}
	digest := sha256.Sum256(data)
	editManifest(t, files, func(value *manifest) {
		for index := range value.Datasets {
			if value.Datasets[index].ID == id {
				value.Datasets[index].SHA256 = hex.EncodeToString(digest[:])
				return
			}
		}
		t.Fatalf("manifest has no dataset %q", id)
	})
}

func replaceJSON(t *testing.T, files fstest.MapFS, name string, value any) {
	t.Helper()
	files[name] = &fstest.MapFile{Data: marshalJSON(t, value)}
}

func decodeFile(t *testing.T, files fstest.MapFS, name string, destination any) {
	t.Helper()
	data, err := fs.ReadFile(files, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatal(err)
	}
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func specByID(t *testing.T, id string) datasetSpec {
	t.Helper()
	for _, spec := range datasetSpecs {
		if spec.id == id {
			return spec
		}
	}
	t.Fatalf("no test dataset spec for %q", id)
	return datasetSpec{}
}

func manifestPath() string {
	return pathFor(manifestName)
}

func datasetPath(id string) string {
	for _, spec := range datasetSpecs {
		if spec.id == id {
			return pathFor(spec.file)
		}
	}
	panic(fmt.Sprintf("unsupported test dataset %q", id))
}

func pathFor(name string) string {
	return catalogueRoot + "/" + name
}

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int) *int {
	return &value
}
