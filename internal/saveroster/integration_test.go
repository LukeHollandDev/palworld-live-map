package saveroster_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/saveroster"
	"github.com/LukeHollandDev/palworld-live-map/internal/savesidecar"
)

// This is the only test that runs the real palworld-save-reader against a real
// save. Everything else in this package stubs the decoder, which proves the
// selection and projection logic but not the contract between the two
// programs: the executable name, the --list-presets probe, the player-details
// preset's field names, and the JSON shape savesidecar parses.
//
// Real saves cannot live in this repository -- they carry player names, account
// identifiers, coordinates and progression -- so the test reads them from a
// directory named by the environment and skips when it is unset. The decoder is
// the same ignored bin/palworld-save-reader executable used by a source run.
//
//	PALWORLD_SAVE_ROOT_FIXTURE  a SaveGames/0 directory
func TestReaderAndRosterAgainstRealSave(t *testing.T) {
	root := strings.TrimSpace(os.Getenv("PALWORLD_SAVE_ROOT_FIXTURE"))
	if root == "" {
		t.Skip("set PALWORLD_SAVE_ROOT_FIXTURE to run against a real save")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("PALWORLD_SAVE_ROOT_FIXTURE must be absolute")
	}
	decoder, err := filepath.Abs(filepath.Join("..", "..", "bin", "palworld-save-reader"))
	if err != nil {
		t.Fatal(err)
	}

	// NewReader runs --list-presets, so this alone proves the decoder is
	// present, executable, and still advertising the preset this app needs.
	reader, err := savesidecar.NewReader(savesidecar.Options{BinaryPath: decoder})
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	source, err := saveroster.New(saveroster.Options{
		Root:    root,
		Timeout: 2 * time.Minute,
		Reader:  reader,
		// A real projector is a keyed HMAC. This one only has to be stable and
		// unlike its input, which is what projectID enforces.
		ProjectPlayerID: func(private string) (string, bool) {
			return "test-" + strings.ReplaceAll(private, "0", "x"), true
		},
		ProjectGuildKey: func(private string) (string, bool) {
			return "test-guild-" + strings.ReplaceAll(private, "0", "x"), true
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	snapshot, err := source.Roster(context.Background())
	if err != nil {
		t.Fatalf("Roster() error = %v", err)
	}
	if len(snapshot.Players) == 0 {
		t.Fatal("roster is empty; the decoder contract or the fixture layout has drifted")
	}
	if snapshot.SnapshotAt.IsZero() {
		t.Fatal("roster has no snapshot time")
	}

	// Positions are what the map draws. A player who decoded but landed on no
	// map layer means the preset's LastTransform is being read wrongly, which
	// an "it returned some players" assertion would not catch.
	positioned, named, guilded := 0, 0, 0
	for _, player := range snapshot.Players {
		if player.ID == "" {
			t.Fatal("roster contains a player with no public ID")
		}
		if player.Map != "" {
			positioned++
		}
		if player.Name != "" {
			named++
		}
		if player.GuildKey != "" {
			guilded++
		}
	}
	if positioned == 0 {
		t.Fatal("no player resolved to a map layer; LastTransform decoding has drifted")
	}
	// Names come from the resolve pass, not the preset. Without them the poller
	// publishes no offline players at all, so this is the assertion that would
	// catch the resolve document's shape drifting.
	if named == 0 {
		t.Fatal("no player was named; the resolve pass or its JSON shape has drifted")
	}
	if guilded == 0 {
		t.Fatal("no player resolved to a guild; the resolve guild shape has drifted")
	}
	t.Logf("decoded %d players, %d positioned, %d named, %d guilded, snapshot at %s",
		len(snapshot.Players), positioned, named, guilded, snapshot.SnapshotAt.Format(time.RFC3339))
}
