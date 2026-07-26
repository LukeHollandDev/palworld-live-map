package savesidecar

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testGameVersion = "1.0.1.100619"

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
	resolved := `{"resolveVersion":2,"kind":"roster","roster":[{
		"playerUId":"AAAAAAAA-0000-0000-0000-000000000000",
		"character":{"nickname":"Sable","level":42},
		"guild":{"id":"5aa6910c-e317-4a73-be66-6d55190b9dbf","name":"Aurora"}
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
		"wrong kind":     `printf '%s' '{"resolveVersion":2,"kind":"guilds","roster":[]}'`,
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

// A GUID appearing twice makes every record under it ambiguous, and labelling a
// player with someone else's name is worse than leaving them unnamed.
func TestResolveDropsNamesForDuplicateGUIDs(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	resolved := `{"resolveVersion":2,"kind":"roster","roster":[
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
const emptyResolveBody = `printf '%s' '{"resolveVersion":2,"kind":"roster","roster":[]}'`

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
