package landmarks

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	mapassets "github.com/LukeHollandDev/palworld-live-map/assets"
)

func TestLoadProjectsAndValidatesLocations(t *testing.T) {
	catalogue, err := Load(testFS(testManifest(2, `[
    {"id":"alpha:penking","kind":"alpha-pals","name":"Penking","detail":"Field Alpha · Water / Ice","level":15,"x":-285331.3,"y":210162.69},
    {"id":"tower:rayne","kind":"bosses","name":"Zoe & Grizzbolt","detail":"Rayne Syndicate Tower · Electric","level":10,"x":-321717,"y":209867}
  ]`)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	locations := catalogue.Locations
	if len(locations) != 2 || locations[0].ID != "landmark:alpha:penking" || locations[0].Map != "palpagos" || locations[1].Name != "Zoe & Grizzbolt" {
		t.Fatalf("locations = %#v", locations)
	}
	if catalogue.Metadata.GameVersion != "1.0.1.100619" || catalogue.Metadata.Generator != "palworld-game-exporter/test" {
		t.Fatalf("metadata = %#v", catalogue.Metadata)
	}
}

func TestEmbeddedCatalogueContainsVersionedRequestedEncounters(t *testing.T) {
	catalogue, err := Load(mapassets.Landmarks)
	if err != nil {
		t.Fatal(err)
	}
	locations := catalogue.Locations
	alphaCount, towerCount := 0, 0
	foundPenking, foundZoe := false, false
	for _, location := range locations {
		switch location.Kind {
		case "alpha-pals":
			alphaCount++
		case "bosses":
			towerCount++
		}
		if location.Name == "Penking" && location.Level == 15 && location.X == -285331.3 && location.Y == 210162.69 {
			foundPenking = true
		}
		if location.Name == "Zoe & Grizzbolt" && location.Level == 10 && location.X == -321596.25 && location.Y == 209085 {
			foundZoe = true
		}
	}
	if len(locations) != 99 || alphaCount != 90 || towerCount != 9 || !foundPenking || !foundZoe {
		t.Fatalf("catalogue: total=%d alphas=%d towers=%d Penking=%v Zoe=%v", len(locations), alphaCount, towerCount, foundPenking, foundZoe)
	}
}

func TestLoadRejectsInvalidManifest(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "schema", json: testManifest(3, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]`), want: "schema"},
		{name: "kind", json: testManifest(2, `[{"id":"x","kind":"players","name":"X","x":0,"y":0}]`), want: "unsupported kind"},
		{name: "bounds", json: testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":9999999,"y":9999999}]`), want: "outside"},
		{name: "duplicate", json: testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0},{"id":"x","kind":"bosses","name":"Y","x":1,"y":1}]`), want: "duplicate"},
		{name: "provenance", json: `{"schemaVersion":2,"gameVersion":"1.0.1.100619","locations":[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]}`, want: "provenance"},
		{name: "unknown version", json: strings.Replace(testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]`), `"gameVersion": "1.0.1.100619"`, `"gameVersion": "unknown"`, 1), want: "numeric game version"},
		{name: "partial version", json: strings.Replace(testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]`), `"gameVersion": "1.0.1.100619"`, `"gameVersion": "1.0"`, 1), want: "numeric game version"},
		{name: "moving label", json: strings.Replace(testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]`), `"gameVersion": "1.0.1.100619"`, `"gameVersion": "latest"`, 1), want: "numeric game version"},
		{name: "trailing JSON", json: testManifest(2, `[{"id":"x","kind":"bosses","name":"X","x":0,"y":0}]`) + `{}`, want: "trailing data"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(testFS(test.json))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func testFS(data string) fstest.MapFS {
	return fstest.MapFS{"landmarks/manifest.json": &fstest.MapFile{Data: []byte(data)}}
}

func testManifest(schema int, locations string) string {
	return fmt.Sprintf(`{
  "schemaVersion": %d,
  "gameVersion": "1.0.1.100619",
  "generator": "palworld-game-exporter/test",
  "decoder": "CUE4Parse/test",
  "mappings": {"file":"Mappings.usmap","bytes":1,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  "paks": [{"file":"Pal-Windows.pak","bytes":1,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],
  "sources": [{"object":"Pal/Content/Test","purpose":"test fixture"}],
  "locations": %s
}`, schema, locations)
}
