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

func TestDecodeParsesRosterContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "roster.json"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	want := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	if !snapshot.SnapshotAt.Equal(want) {
		t.Fatalf("SnapshotAt = %v, want %v", snapshot.SnapshotAt, want)
	}
	if len(snapshot.Players) != 2 {
		t.Fatalf("players = %#v", snapshot.Players)
	}
	alice := snapshot.Players[0]
	if alice.PlayerID != "aaaaaaaa-0000-0000-0000-000000000000" || alice.DisplayName != "Alice" || alice.Level != 56 ||
		alice.GuildName != "Builders" || alice.X == nil || *alice.X != -184343.5 || alice.Y == nil ||
		alice.LastSeenAt == nil || alice.CaptureTotal == nil || *alice.CaptureTotal != 513 ||
		alice.UniquePalsCaptured == nil || *alice.UniquePalsCaptured != 140 || alice.PaldeckUnlocked == nil {
		t.Fatalf("alice = %#v", alice)
	}
	bob := snapshot.Players[1]
	if bob.X != nil || bob.Y != nil || bob.LastSeenAt != nil || bob.CaptureTotal != nil || bob.GuildID != "" {
		t.Fatalf("bob should carry nil optionals: %#v", bob)
	}
	if snapshot.Stats.PlayerFiles != 2 {
		t.Fatalf("stats = %#v", snapshot.Stats)
	}
}

func TestDecodeRejectsEmptyAndMalformed(t *testing.T) {
	if _, err := Decode(nil); err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatalf("Decode(nil) error = %v", err)
	}
	if _, err := Decode([]byte("   \n")); err == nil || !strings.Contains(err.Error(), "no output") {
		t.Fatalf("Decode(blank) error = %v", err)
	}
	if _, err := Decode([]byte("{not json")); err == nil || !strings.Contains(err.Error(), "decode save roster JSON") {
		t.Fatalf("Decode(malformed) error = %v", err)
	}
}

func TestDecodeNormalisesNilPlayers(t *testing.T) {
	snapshot, err := Decode([]byte(`{"snapshotAt":"2026-07-21T12:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Players == nil {
		t.Fatal("Players should be a non-nil empty slice")
	}
}

func TestReaderExecsDecoderAndDecodes(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "roster.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The fake decoder records the directory argument it was handed, then
	// prints the fixture roster to stdout.
	argFile := filepath.Join(t.TempDir(), "arg")
	binary := writeFakeDecoder(t, "#!/bin/sh\nprintf '%s' \"$1\" > "+shellQuote(argFile)+"\ncat "+shellQuote(writeFixture(t, fixture))+"\n")

	reader, err := NewReader(Options{BinaryPath: binary})
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	snapshot, err := reader.ReadSnapshot(context.Background(), "/some/generation/path")
	if err != nil {
		t.Fatalf("ReadSnapshot() error = %v", err)
	}
	if len(snapshot.Players) != 2 || snapshot.Players[0].DisplayName != "Alice" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	gotArg, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotArg) != "/some/generation/path" {
		t.Fatalf("decoder arg = %q", gotArg)
	}
}

func TestReaderReportsDecoderFailureWithStderr(t *testing.T) {
	binary := writeFakeDecoder(t, "#!/bin/sh\necho 'boom: bad save' 1>&2\nexit 3\n")
	reader, err := NewReader(Options{BinaryPath: binary})
	if err != nil {
		t.Fatal(err)
	}
	_, err = reader.ReadSnapshot(context.Background(), "/dir")
	if err == nil || !strings.Contains(err.Error(), "boom: bad save") {
		t.Fatalf("ReadSnapshot() error = %v, want stderr detail", err)
	}
}

func TestReaderHonorsContextTimeout(t *testing.T) {
	binary := writeFakeDecoder(t, "#!/bin/sh\nsleep 5\n")
	reader, err := NewReader(Options{BinaryPath: binary})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := reader.ReadSnapshot(ctx, "/dir"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ReadSnapshot() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("ReadSnapshot ignored timeout; elapsed %s", elapsed)
	}
}

func TestReaderBoundsDecoderOutput(t *testing.T) {
	// Emit far more than the configured cap.
	binary := writeFakeDecoder(t, "#!/bin/sh\nhead -c 4096 /dev/zero | tr '\\0' 'a'\n")
	reader, err := NewReader(Options{BinaryPath: binary, MaxOutputBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadSnapshot(context.Background(), "/dir"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ReadSnapshot() error = %v, want output-cap error", err)
	}
}

func TestNewReaderValidatesBinary(t *testing.T) {
	if _, err := NewReader(Options{BinaryPath: "relative/path"}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path error = %v", err)
	}
	if _, err := NewReader(Options{BinaryPath: filepath.Join(t.TempDir(), "missing")}); err == nil || !strings.Contains(err.Error(), "locate save decoder") {
		t.Fatalf("missing binary error = %v", err)
	}
	dir := t.TempDir()
	if _, err := NewReader(Options{BinaryPath: dir}); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory-as-binary error = %v", err)
	}
}

func writeFakeDecoder(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script decoder stand-in is not portable to windows")
	}
	path := filepath.Join(t.TempDir(), "savedecode")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFixture(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
