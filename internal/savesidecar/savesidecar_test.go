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

func TestReaderUsesVersionedPlayerPresetAndSkipsDPSFiles(t *testing.T) {
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
	for _, want := range []string{"--preset", playerPresetName, "--game-version", testGameVersion, filepath.Join(generation, "Players", "one.sav")} {
		if !strings.Contains(got, want) {
			t.Fatalf("arguments = %q, want %q", got, want)
		}
	}
}

func TestReaderToleratesOneFailureButRejectsAllFailures(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "player-details.json"))
	if err != nil {
		t.Fatal(err)
	}
	binary := writeFakeDecoder(t, `
case "$5" in
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

func TestNewReaderValidatesBinaryVersionAndPreset(t *testing.T) {
	if _, err := NewReader(Options{BinaryPath: "relative", GameVersion: testGameVersion}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path error = %v", err)
	}
	if _, err := NewReader(Options{BinaryPath: filepath.Join(t.TempDir(), "missing"), GameVersion: testGameVersion}); err == nil || !strings.Contains(err.Error(), "locate") {
		t.Fatalf("missing binary error = %v", err)
	}
	binary := writeFakeDecoderWithPresets(t, `[]`, "")
	if _, err := NewReader(Options{BinaryPath: binary, GameVersion: testGameVersion}); err == nil || !strings.Contains(err.Error(), "no player-details preset") {
		t.Fatalf("missing preset error = %v", err)
	}
	binary = writeFakeDecoder(t, "")
	if _, err := NewReader(Options{BinaryPath: binary}); err == nil || !strings.Contains(err.Error(), "game version") {
		t.Fatalf("missing version error = %v", err)
	}
}

func newTestReader(t *testing.T, binary string, maxOutput int64) *Reader {
	t.Helper()
	reader, err := NewReader(Options{BinaryPath: binary, GameVersion: testGameVersion, MaxOutputBytes: maxOutput})
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

func writeFakeDecoder(t *testing.T, body string) string {
	t.Helper()
	presets := `[{"name":"player-details","gameVersion":"` + testGameVersion + `","saveType":"player.sav"}]`
	return writeFakeDecoderWithPresets(t, presets, body)
}

func writeFakeDecoderWithPresets(t *testing.T, presets, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script decoder stand-in is not portable to Windows")
	}
	path := filepath.Join(t.TempDir(), "savedecode")
	script := `#!/bin/sh
if [ "$1" = "--list-presets" ]; then
  printf '%s' ` + shellQuote(presets) + `
  exit 0
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
