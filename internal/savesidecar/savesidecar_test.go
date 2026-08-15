package savesidecar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testGameVersion            = "1.0.1.100619"
	claimCandidateFloorForTest = 16
)

func TestPrivateClaimSaveTypesMarshalWithoutEvidence(t *testing.T) {
	stack := ClaimStack{Slot: 7, ItemID: "private-item-id", Count: 31, DynamicItemID: "private-instance-id"}
	pal := ClaimPal{Slot: 3, InstanceID: "private-pal-id"}
	progress := ClaimProgress{
		Available:    true,
		FastTravel:   []string{"private-fast-travel-key"},
		Areas:        []string{"private-area-key"},
		Notes:        []string{"private-note-key"},
		NormalBosses: []string{"private-normal-boss-key"},
		TowerBosses:  []string{"private-tower-boss-key"},
	}
	player := ClaimPlayer{
		PlayerID: "aaaaaaaa000000000000000000000000",
		Common:   []ClaimStack{stack},
		Party:    []ClaimPal{pal},
		Progress: progress,
	}

	for name, value := range map[string]any{
		"player":   player,
		"stack":    stack,
		"pal":      pal,
		"progress": progress,
	} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if got, want := string(encoded), `{}`; got != want {
				t.Fatalf("private save type JSON = %s, want %s", got, want)
			}
		})
	}
}

func TestDecodePlayerDerivesProgress(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	player, err := DecodePlayer(data)
	if err != nil {
		t.Fatalf("DecodePlayer() error = %v", err)
	}
	if player.PlayerID != "aaaaaaaa-0000-0000-0000-000000000000" ||
		player.X == nil || *player.X != -184343.5 || player.Y == nil || *player.Y != 256561.11 ||
		player.LastSeenAt == nil || !player.LastSeenAt.Equal(time.Unix(10, 0).UTC()) ||
		player.CaptureTotal == nil || *player.CaptureTotal != 7 ||
		player.UniquePalsCaptured == nil || *player.UniquePalsCaptured != 2 ||
		player.PaldeckUnlocked == nil || *player.PaldeckUnlocked != 1 {
		t.Fatalf("player = %#v", player)
	}
}

func TestDecodePlayerRejectsMalformedAndInvalidatesAmbiguousProgress(t *testing.T) {
	for _, data := range [][]byte{nil, []byte(`{}`), []byte(`{not-json`), []byte(`{} {}`)} {
		if _, err := DecodePlayer(data); err == nil {
			t.Fatalf("DecodePlayer(%q) succeeded", data)
		}
	}
	player, err := DecodePlayer([]byte(`{
		"PlayerUId":"aaaaaaaa-0000-0000-0000-000000000000",
		"LastTransform":{"Translation":{"X":0,"Y":0,"Z":0}},
		"RecordData":{
			"TribeCaptureCount":-1,
			"PalCaptureCount":[{"Key":"Lamball","Value":1},{"Key":"lamball","Value":2}],
			"PaldeckUnlockFlag":[{"Key":"","Value":true}]
		},
		"LastOnlineDateTime":621355968000000000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if player.CaptureTotal != nil || player.UniquePalsCaptured != nil || player.PaldeckUnlocked != nil {
		t.Fatalf("invalid progress survived: %#v", player)
	}
}

func TestReaderUsesPlayerPresetAndSkipsDPSFiles(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	argFile := filepath.Join(t.TempDir(), "arguments")
	binary := writeFakeDecoder(t, `
printf '%s\n' "$@" > `+shellQuote(argFile)+`
printf '%s' `+shellQuote(string(fixture))+`
`)
	reader := newTestReader(t, binary, 0)
	generation := makeSnapshot(t, "one.sav", "one_dps.sav")
	snapshot, err := reader.ReadSnapshot(context.Background(), generation)
	if err != nil {
		t.Fatalf("ReadSnapshot() error = %v", err)
	}
	if snapshot.Stats.PlayerFiles != 1 || snapshot.Stats.DecodeFailures != 0 || len(snapshot.Players) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	arguments, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatal(err)
	}
	got := string(arguments)
	for _, want := range []string{"--preset", playerPresetName, filepath.Join(generation, "Players", "one.sav")} {
		if !strings.Contains(got, want) {
			t.Fatalf("arguments = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "--game-version") {
		t.Fatalf("arguments = %q, obsolete --game-version must not be passed", got)
	}
}

func TestReaderToleratesOneFailureButRejectsAllFailures(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	binary := writeFakeDecoder(t, `
case "$3" in
  *bad.sav) echo "bad player save" >&2; exit 1 ;;
esac
printf '%s' `+shellQuote(string(fixture))+`
`)
	reader := newTestReader(t, binary, 0)
	snapshot, err := reader.ReadSnapshot(context.Background(), makeSnapshot(t, "bad.sav", "good.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Stats.PlayerFiles != 2 || snapshot.Stats.DecodeFailures != 1 || len(snapshot.Players) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if _, err := reader.ReadSnapshot(context.Background(), makeSnapshot(t, "bad.sav")); err == nil || !strings.Contains(err.Error(), "failed for all") {
		t.Fatalf("all-failed error = %v", err)
	}
}

func TestReaderErrorsNeverIncludeRawPlayerSaveGUID(t *testing.T) {
	const rawGUID = "aaaaaaaa-0000-0000-0000-000000000000"
	generation := makeSnapshot(t)
	target := filepath.Join(t.TempDir(), "outside.sav")
	if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(generation, "Players", rawGUID+".sav")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := (&Reader{}).ReadSnapshot(context.Background(), generation)
	if err == nil {
		t.Fatal("ReadSnapshot() accepted a player-save symlink")
	}
	if strings.Contains(strings.ToLower(err.Error()), rawGUID) {
		t.Fatalf("loggable snapshot error leaked raw player GUID: %v", err)
	}
}

func TestReaderHonorsContextAndOutputBounds(t *testing.T) {
	slow := writeFakeDecoder(t, "exec sleep 5\n")
	reader := newTestReader(t, slow, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := reader.ReadSnapshot(ctx, makeSnapshot(t, "one.sav")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}

	large := writeFakeDecoder(t, "head -c 4096 /dev/zero | tr '\\0' 'a'\n")
	reader = newTestReader(t, large, 128)
	if _, err := reader.ReadSnapshot(context.Background(), makeSnapshot(t, "one.sav")); err == nil || !strings.Contains(err.Error(), "failed for all") {
		t.Fatalf("bounded output error = %v", err)
	}
}

func TestReaderSerializesDecoderProcessesAndCancelsQueuedWork(t *testing.T) {
	dir := t.TempDir()
	startedPath := filepath.Join(dir, "started")
	releasePath := filepath.Join(dir, "release")
	launchesPath := filepath.Join(dir, "launches")
	binary := writeFakeDecoder(t, `
printf '%s\n' launched >> `+shellQuote(launchesPath)+`
: > `+shellQuote(startedPath)+`
while [ ! -f `+shellQuote(releasePath)+` ]; do sleep 0.01; done
printf '%s' '{}'
`)
	reader := newTestReader(t, binary, 0)

	firstContext, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	defer func() { _ = os.WriteFile(releasePath, []byte("release"), 0o600) }()
	first := make(chan error, 1)
	go func() {
		_, err := reader.run(firstContext, "--work", "first")
		first <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first decoder process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	queuedContext, cancelQueued := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelQueued()
	if _, err := reader.run(queuedContext, "--work", "queued"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued run error = %v, want context deadline exceeded", err)
	}
	launches, err := os.ReadFile(launchesPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(launches))); got != 1 {
		t.Fatalf("decoder launches while first process held gate = %d, want 1", got)
	}

	if err := os.WriteFile(releasePath, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("first run error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first decoder process did not release the gate")
	}
	if _, err := reader.run(context.Background(), "--work", "after-release"); err != nil {
		t.Fatalf("run after release error = %v", err)
	}
	launches, err = os.ReadFile(launchesPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Fields(string(launches))); got != 2 {
		t.Fatalf("decoder launches after gate release = %d, want 2", got)
	}
}

func TestNewReaderValidatesBinaryAndPreset(t *testing.T) {
	if _, err := NewReader(Options{BinaryPath: "relative"}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path error = %v", err)
	}
	if _, err := NewReader(Options{BinaryPath: filepath.Join(t.TempDir(), "missing")}); err == nil || !strings.Contains(err.Error(), "locate") {
		t.Fatalf("missing binary error = %v", err)
	}
	binary := writeFakeDecoderWithPresets(t, `[]`, "")
	if _, err := NewReader(Options{BinaryPath: binary}); err == nil || !strings.Contains(err.Error(), "no valid player-details preset") {
		t.Fatalf("missing preset error = %v", err)
	}
	binary = writeFakeDecoderWithPresets(t, `[{"name":"player-details","gameVersion":"","saveType":"player.sav"}]`, "")
	if _, err := NewReader(Options{BinaryPath: binary}); err == nil || !strings.Contains(err.Error(), "no valid player-details preset") {
		t.Fatalf("missing preset provenance error = %v", err)
	}
}

// The container image installs the decoder beside the server binary under the
// name the production lookup expects, so the two must not drift apart.
func TestNewReaderDefaultsToTheReaderNameBesideTheExecutable(t *testing.T) {
	if defaultBinaryName != "palworld-save-reader" {
		t.Fatalf("defaultBinaryName = %q, want the palworld-save-reader executable name", defaultBinaryName)
	}
	_, err := NewReader(Options{})
	if err == nil {
		t.Skip("a palworld-save-reader binary happens to sit beside the test executable")
	}
	if !strings.Contains(err.Error(), defaultBinaryName) {
		t.Fatalf("error = %v, want it to name %q", err, defaultBinaryName)
	}
}

// The preset carries no name, so a record only becomes publishable as an
// offline player once the resolve pass names it.
func TestReaderResolvesNamesLevelsAndGuilds(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	argFile := filepath.Join(t.TempDir(), "resolve-arguments")
	resolved := `{"resolveVersion":4,"kind":"roster","roster":[{
		"playerUId":"AAAAAAAA-0000-0000-0000-000000000000",
		"character":{"nickname":"Sable","level":42,"arenaRankPoints":1875},
		"guild":{"id":"5aa6910c-e317-4a73-be66-6d55190b9dbf","name":"Aurora"},
		"fastTravelUnlocked":73,
		"areasDiscovered":48,
		"bossDefeats":27,
		"towerDefeats":6
	}]}`
	binary := writeFakeDecoderWithResolve(t, testPresets, `
printf '%s\n' "$@" > `+shellQuote(argFile)+`
printf '%s' `+shellQuote(resolved)+`
`, `printf '%s' `+shellQuote(string(fixture)))
	reader := newTestReader(t, binary, 0)
	generation := makeSnapshot(t, "one.sav")
	snapshot, err := reader.ReadSnapshot(context.Background(), generation)
	if err != nil {
		t.Fatalf("ReadSnapshot() error = %v", err)
	}
	if len(snapshot.Players) != 1 {
		t.Fatalf("players = %#v", snapshot.Players)
	}
	player := snapshot.Players[0]
	if player.Name != "Sable" || player.Level != 42 {
		t.Fatalf("player = %#v, want Sable at level 42", player)
	}
	if player.GuildID != "5aa6910c-e317-4a73-be66-6d55190b9dbf" || player.GuildName != "Aurora" {
		t.Fatalf("player = %#v, want the resolved guild", player)
	}
	if player.ArenaRankPoints == nil || *player.ArenaRankPoints != 1875 ||
		player.FastTravelUnlocked == nil || *player.FastTravelUnlocked != 73 ||
		player.AreasDiscovered == nil || *player.AreasDiscovered != 48 ||
		player.BossDefeats == nil || *player.BossDefeats != 27 ||
		player.TowerDefeats == nil || *player.TowerDefeats != 6 {
		t.Fatalf("player = %#v, want resolved leaderboard progress", player)
	}
	if snapshot.Stats.NamesResolved != 1 || snapshot.Stats.ResolveFailed {
		t.Fatalf("stats = %#v", snapshot.Stats)
	}
	arguments, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatal(err)
	}
	// The pass must be pointed at the generation, never the live world.
	for _, want := range []string{"--resolve", resolveRosterKind, "--saves", generation} {
		if !strings.Contains(string(arguments), want) {
			t.Fatalf("resolve arguments = %q, want %q", arguments, want)
		}
	}
}

// Losing the names must not cost the progress counters: enrichment of the REST
// player list has to survive a resolve failure.
func TestReaderKeepsPresetDataWhenResolveFails(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, resolveBody := range map[string]string{
		"exits non-zero": `echo boom >&2; exit 1`,
		"emits garbage":  `printf '%s' '{not-json'`,
		"wrong kind":     `printf '%s' '{"resolveVersion":4,"kind":"guilds","roster":[]}'`,
		"wrong version":  `printf '%s' '{"resolveVersion":999,"kind":"roster","roster":[]}'`,
	} {
		binary := writeFakeDecoderWithResolve(t, testPresets, resolveBody,
			`printf '%s' `+shellQuote(string(fixture)))
		reader := newTestReader(t, binary, 0)
		snapshot, err := reader.ReadSnapshot(context.Background(), makeSnapshot(t, "one.sav"))
		if err != nil {
			t.Fatalf("ReadSnapshot() error = %v, want the generation to survive", err)
		}
		if len(snapshot.Players) != 1 || snapshot.Players[0].Name != "" {
			t.Fatalf("players = %#v", snapshot.Players)
		}
		if snapshot.Players[0].CaptureTotal == nil {
			t.Fatalf("progress counters lost with the names: %#v", snapshot.Players[0])
		}
		if !snapshot.Stats.ResolveFailed || snapshot.Stats.NamesResolved != 0 {
			t.Fatalf("stats = %#v", snapshot.Stats)
		}
		if snapshot.Stats.ResolveError == nil {
			t.Fatalf("stats = %#v, want the resolve diagnostic retained for logging", snapshot.Stats)
		}
	}
}

func TestReaderResolvesPrivateClaimSlotsForOnePlayer(t *testing.T) {
	resolved := `{"resolveVersion":4,"kind":"player","player":{
		"playerUId":"AAAAAAAA-0000-0000-0000-000000000000",
		"progress":{"fastTravel":["FT-Two","ft-one"],"areas":[],"notes":["Day0"],"normalBosses":[],"towerBosses":[]},
		"inventory":{"common":[
			{"slot":7,"itemId":"Stone","count":31},
			{"slot":2,"itemId":"Wood","count":19}
		]},
		"pals":[
			{"instanceId":"pal-two","location":"party","slot":3},
			{"instanceId":"stored","location":"storage","slot":0},
			{"instanceId":"pal-one","location":"party","slot":1}
		]
	}}`
	argFile := filepath.Join(t.TempDir(), "claim-arguments")
	binary := writeFakeDecoderWithResolve(t, testPresets, `
printf '%s\n' "$@" > `+shellQuote(argFile)+`
printf '%s' `+shellQuote(resolved), emptyResolveBody)
	reader := newTestReader(t, binary, 0)
	player, err := reader.ReadClaimPlayer(
		context.Background(), makeSnapshot(t, "one.sav"), "aaaaaaaa-0000-0000-0000-000000000000",
	)
	if err != nil {
		t.Fatalf("ReadClaimPlayer() error = %v", err)
	}
	if player.PlayerID != "aaaaaaaa000000000000000000000000" || len(player.Common) != 2 ||
		player.Common[0].Slot != 2 || player.Common[0].ItemID != "Wood" ||
		player.Common[1].Slot != 7 || player.Common[1].Count != 31 {
		t.Fatalf("claim player = %#v", player)
	}
	if len(player.Party) != 2 || player.Party[0].Slot != 1 || player.Party[1].Slot != 3 {
		t.Fatalf("claim party = %#v", player.Party)
	}
	if !player.Progress.Available || strings.Join(player.Progress.FastTravel, ",") != "ft-one,ft-two" ||
		strings.Join(player.Progress.Notes, ",") != "day0" || player.Progress.Areas == nil {
		t.Fatalf("claim progress = %#v", player.Progress)
	}
	arguments, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--resolve", resolvePlayerKind, "--id", player.PlayerID, "--saves"} {
		if !strings.Contains(string(arguments), want) {
			t.Fatalf("claim arguments = %q, want %q", arguments, want)
		}
	}
}

func TestReaderKeepsProofInventoryWhenProgressOrCollectionsAreIncomplete(t *testing.T) {
	common := make([]map[string]any, claimCandidateFloorForTest)
	for index := range common {
		common[index] = map[string]any{
			"slot": index, "itemId": fmt.Sprintf("PrivateItem%02d", index), "count": index + 1,
		}
	}
	completeProgress := map[string]any{
		"fastTravel": []string{}, "areas": []string{}, "notes": []string{},
		"normalBosses": []string{}, "towerBosses": []string{},
	}
	incompleteProgress := map[string]any{
		"fastTravel": []string{"private-state-key"}, "notes": []string{},
		"normalBosses": []string{}, "towerBosses": []string{},
	}
	for _, test := range []struct {
		name          string
		progress      any
		wantAvailable bool
	}{
		{name: "unrelated collection warning", progress: completeProgress, wantAvailable: true},
		{name: "missing progress domain", progress: incompleteProgress},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := map[string]any{
				"resolveVersion": 4,
				"kind":           "player",
				"player": map[string]any{
					"playerUId": "aaaaaaaa-0000-0000-0000-000000000000",
					"progress":  test.progress,
					"inventory": map[string]any{"common": common},
					"pals":      []any{},
					"warnings":  []string{"private collection warning /save/path raw-guid item-id"},
				},
			}
			resolved, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			binary := writeFakeDecoderWithResolve(t, testPresets, `printf '%s' `+shellQuote(string(resolved)), emptyResolveBody)
			reader := newTestReader(t, binary, 0)
			player, err := reader.ReadClaimPlayer(context.Background(), makeSnapshot(t, "one.sav"), "aaaaaaaa-0000-0000-0000-000000000000")
			if err != nil {
				t.Fatalf("ReadClaimPlayer() error = %v", err)
			}
			if len(player.Common) != claimCandidateFloorForTest {
				t.Fatalf("proof inventory stacks = %d, want %d", len(player.Common), claimCandidateFloorForTest)
			}
			if player.Progress.Available != test.wantAvailable {
				t.Fatalf("progress availability = %v, want %v: %#v", player.Progress.Available, test.wantAvailable, player.Progress)
			}
			if !test.wantAvailable && (player.Progress.FastTravel != nil || player.Progress.Areas != nil ||
				player.Progress.Notes != nil || player.Progress.NormalBosses != nil || player.Progress.TowerBosses != nil) {
				t.Fatalf("incomplete progress retained private keys: %#v", player.Progress)
			}
		})
	}
}

func TestReaderBoundsPartyAfterFilteringStoragePals(t *testing.T) {
	read := func(t *testing.T, pals []map[string]any) (ClaimPlayer, error) {
		t.Helper()
		document := map[string]any{
			"resolveVersion": 4,
			"kind":           "player",
			"player": map[string]any{
				"playerUId": "aaaaaaaa-0000-0000-0000-000000000000",
				"inventory": map[string]any{"common": []any{}},
				"pals":      pals,
			},
		}
		resolved, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		binary := writeFakeDecoderWithResolve(t, testPresets, `printf '%s' `+shellQuote(string(resolved)), emptyResolveBody)
		reader := newTestReader(t, binary, 0)
		return reader.ReadClaimPlayer(context.Background(), makeSnapshot(t, "one.sav"), "aaaaaaaa-0000-0000-0000-000000000000")
	}

	pals := make([]map[string]any, 0, maxClaimPartyPals*2+2)
	for index := 0; index < maxClaimPartyPals*2; index++ {
		pals = append(pals, map[string]any{
			"instanceId": fmt.Sprintf("private-storage-pal-%03d", index), "location": "storage", "slot": index,
		})
	}
	pals = append(pals,
		map[string]any{"instanceId": "private-party-one", "location": "party", "slot": 1},
		map[string]any{"instanceId": "private-party-two", "location": "party", "slot": 2},
	)
	player, err := read(t, pals)
	if err != nil {
		t.Fatalf("ReadClaimPlayer() with storage Pals = %v", err)
	}
	if len(player.Party) != 2 {
		t.Fatalf("party Pals = %#v, want only two filtered party entries", player.Party)
	}

	tooManyParty := make([]map[string]any, maxClaimPartyPals+1)
	for index := range tooManyParty {
		tooManyParty[index] = map[string]any{
			"instanceId": fmt.Sprintf("private-party-%03d", index), "location": "party", "slot": index,
		}
	}
	if _, err := read(t, tooManyParty); err == nil {
		t.Fatal("ReadClaimPlayer() accepted more than the bounded party size")
	}
}

func TestReaderRejectsUnsafeClaimDocuments(t *testing.T) {
	tests := map[string]string{
		"wrong player":   `{"resolveVersion":4,"kind":"player","player":{"playerUId":"bbbbbbbb-0000-0000-0000-000000000000","inventory":{"common":[]},"pals":[]}}`,
		"duplicate slot": `{"resolveVersion":4,"kind":"player","player":{"playerUId":"aaaaaaaa-0000-0000-0000-000000000000","inventory":{"common":[{"slot":1,"itemId":"Wood","count":1},{"slot":1,"itemId":"Stone","count":1}]},"pals":[]}}`,
	}
	for name, resolved := range tests {
		t.Run(name, func(t *testing.T) {
			binary := writeFakeDecoderWithResolve(t, testPresets, `printf '%s' `+shellQuote(resolved), emptyResolveBody)
			reader := newTestReader(t, binary, 0)
			if _, err := reader.ReadClaimPlayer(context.Background(), makeSnapshot(t, "one.sav"), "aaaaaaaa-0000-0000-0000-000000000000"); err == nil {
				t.Fatal("ReadClaimPlayer() succeeded")
			}
		})
	}
	reader := &Reader{}
	for _, playerID := range []string{"", "not-a-guid", "aaaaaaaa-0000-0000-0000-00000000000z"} {
		if _, err := reader.ReadClaimPlayer(context.Background(), "/tmp/snapshot", playerID); err == nil {
			t.Fatalf("ReadClaimPlayer(%q) succeeded", playerID)
		}
	}
}

// A GUID appearing twice makes every record under it ambiguous, and labelling a
// player with someone else's name is worse than leaving them unnamed.
func TestResolveDropsNamesForDuplicateGUIDs(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	resolved := `{"resolveVersion":4,"kind":"roster","roster":[
		{"playerUId":"aaaaaaaa-0000-0000-0000-000000000000","character":{"nickname":"First","level":1}},
		{"playerUId":"AAAAAAAA00000000000000000000AAAA","character":{"nickname":"Other","level":2}},
		{"playerUId":"aaaaaaaa-0000-0000-0000-000000000000","character":{"nickname":"Second","level":2}}
	]}`
	binary := writeFakeDecoderWithResolve(t, testPresets,
		`printf '%s' `+shellQuote(resolved),
		`printf '%s' `+shellQuote(string(fixture)))
	reader := newTestReader(t, binary, 0)
	snapshot, err := reader.ReadSnapshot(context.Background(), makeSnapshot(t, "one.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 1 || snapshot.Players[0].Name != "" {
		t.Fatalf("players = %#v, want the ambiguous record left unnamed", snapshot.Players)
	}
	if snapshot.Stats.NamesResolved != 0 {
		t.Fatalf("stats = %#v", snapshot.Stats)
	}
}

func newTestReader(t *testing.T, binary string, maxOutput int64) *Reader {
	t.Helper()
	reader, err := NewReader(Options{BinaryPath: binary, MaxOutputBytes: maxOutput})
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func makeSnapshot(t *testing.T, players ...string) string {
	t.Helper()
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "Level.sav"), []byte("level"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(path, "Players"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range players {
		if err := os.WriteFile(filepath.Join(path, "Players", name), []byte("player"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

const testPresets = `[{"name":"player-details","gameVersion":"` + testGameVersion + `","saveType":"player.sav"}]`

func writeFakeDecoder(t *testing.T, body string) string {
	t.Helper()
	return writeFakeDecoderWithPresets(t, testPresets, body)
}

func writeFakeDecoderWithPresets(t *testing.T, presets, body string) string {
	t.Helper()
	return writeFakeDecoderWithResolve(t, presets, emptyResolveBody, body)
}

// emptyResolveBody names nobody, which is what every test that predates the
// resolve pass expects: preset behaviour unchanged, no offline players.
const emptyResolveBody = `printf '%s' '{"resolveVersion":4,"kind":"roster","roster":[]}'`

func writeFakeDecoderWithResolve(t *testing.T, presets, resolveBody, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script decoder stand-in is not portable to Windows")
	}
	path := filepath.Join(t.TempDir(), defaultBinaryName)
	script := `#!/bin/sh
if [ "$1" = "--list-presets" ]; then
  printf '%s' ` + shellQuote(presets) + `
  exit 0
fi
if [ "$1" = "--list-resolvers" ]; then
  printf '%s' '["guild","guilds","player","players","roster","world"]'
  exit 0
fi
if [ "$1" = "--resolve" ]; then
` + resolveBody + `
  exit $?
fi
` + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
