package saveroster

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/savegame"
)

const (
	testWorldOne = "11111111111111111111111111111111"
	testWorldTwo = "22222222222222222222222222222222"
)

type fakeSnapshotReader struct {
	snapshot *savegame.Snapshot
	err      error
	paths    []string
	read     func(context.Context, string) (*savegame.Snapshot, error)
}

func (f *fakeSnapshotReader) ReadSnapshot(ctx context.Context, path string) (*savegame.Snapshot, error) {
	f.paths = append(f.paths, path)
	if f.read != nil {
		return f.read(ctx, path)
	}
	return f.snapshot, f.err
}

func TestNewValidatesConfiguration(t *testing.T) {
	valid := Options{
		Root: t.TempDir(), Reader: &fakeSnapshotReader{},
		ProjectPlayerID: testPlayerProjector, ProjectGuildID: testGuildProjector,
	}
	tests := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{name: "relative root", mutate: func(o *Options) { o.Root = "SaveGames/0" }, want: "absolute"},
		{name: "negative timeout", mutate: func(o *Options) { o.Timeout = -time.Second }, want: "negative"},
		{name: "short world ID", mutate: func(o *Options) { o.WorldID = "1234" }, want: "32 hexadecimal"},
		{name: "hyphenated world ID", mutate: func(o *Options) { o.WorldID = "11111111-1111-1111-1111-111111111111" }, want: "32 hexadecimal"},
		{name: "spaced world ID", mutate: func(o *Options) { o.WorldID = " " + testWorldOne }, want: "32 hexadecimal"},
		{name: "non-hex world ID", mutate: func(o *Options) { o.WorldID = strings.Repeat("z", 32) }, want: "32 hexadecimal"},
		{name: "missing reader", mutate: func(o *Options) { o.Reader = nil }, want: "snapshot reader"},
		{name: "missing player projector", mutate: func(o *Options) { o.ProjectPlayerID = nil }, want: "projectors"},
		{name: "missing guild projector", mutate: func(o *Options) { o.ProjectGuildID = nil }, want: "projectors"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.mutate(&options)
			_, err := New(options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want substring %q", err, test.want)
			}
		})
	}

	valid.WorldID = strings.ToUpper(testWorldOne)
	if _, err := New(valid); err != nil {
		t.Fatalf("New(valid uppercase world ID) = %v", err)
	}
}

func TestRosterUsesSecondNewestCompleteGeneration(t *testing.T) {
	root := t.TempDir()
	oldest := makeGeneration(t, root, testWorldOne, "2026.07.21-10.00.00")
	want := makeGeneration(t, root, testWorldOne, "2026.07.21-11.00.00")
	newest := makeGeneration(t, root, testWorldOne, "2026.07.21-12.00.00")

	// Timestamp names, rather than mutable directory mtimes, define native
	// generation order. Deliberately make the metadata disagree with the names.
	now := time.Now()
	setMtime(t, oldest, now)
	setMtime(t, want, now.Add(-time.Hour))
	setMtime(t, newest, now.Add(-2*time.Hour))

	// A newer but incomplete directory and a symlink must not enter the set of
	// complete generations.
	incomplete := filepath.Join(root, testWorldOne, "backup", "world", "2026.07.21-13.00.00")
	if err := os.MkdirAll(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "Level.sav"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := makeGeneration(t, t.TempDir(), testWorldTwo, "2026.07.21-14.00.00")
	if err := os.Symlink(outside, filepath.Join(root, testWorldOne, "backup", "world", "2026.07.21-14.00.00")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	snapshotAt := time.Date(2026, 7, 21, 11, 0, 3, 0, time.FixedZone("local", 3600))
	reader := &fakeSnapshotReader{snapshot: &savegame.Snapshot{SnapshotAt: snapshotAt}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector, testGuildProjector)
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.paths) != 1 || reader.paths[0] != want {
		t.Fatalf("decoded paths = %q, want only %q", reader.paths, want)
	}
	if !roster.SnapshotAt.Equal(snapshotAt) || roster.SnapshotAt.Location() != time.UTC {
		t.Fatalf("SnapshotAt = %v (%v), want %v in UTC", roster.SnapshotAt, roster.SnapshotAt.Location(), snapshotAt)
	}
}

func TestRosterUsesOnlyCompleteGenerationAndGenerationTimeFallback(t *testing.T) {
	root := t.TempDir()
	want := makeGeneration(t, root, testWorldOne, "2026.07.21-09.08.07")
	reader := &fakeSnapshotReader{snapshot: &savegame.Snapshot{}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector, testGuildProjector)
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.paths) != 1 || reader.paths[0] != want {
		t.Fatalf("decoded paths = %q, want %q", reader.paths, want)
	}
	wantTime := time.Date(2026, 7, 21, 9, 8, 7, 0, time.UTC)
	if !roster.SnapshotAt.Equal(wantTime) {
		t.Fatalf("SnapshotAt = %v, want generation time %v", roster.SnapshotAt, wantTime)
	}
}

func TestWorldSelectionIsStrictAndUnambiguous(t *testing.T) {
	root := t.TempDir()
	first := makeGeneration(t, root, testWorldOne, "2026.07.21-10.00.00")
	second := makeGeneration(t, root, strings.ToUpper(testWorldTwo), "2026.07.21-10.00.00")
	reader := &fakeSnapshotReader{snapshot: &savegame.Snapshot{}}

	automatic := newTestSource(t, root, "", 0, reader, testPlayerProjector, testGuildProjector)
	if _, err := automatic.Roster(context.Background()); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("automatic Roster() error = %v, want ambiguity", err)
	}
	if len(reader.paths) != 0 {
		t.Fatalf("decoder was called during ambiguous selection: %q", reader.paths)
	}

	explicit := newTestSource(t, root, testWorldTwo, 0, reader, testPlayerProjector, testGuildProjector)
	if _, err := explicit.Roster(context.Background()); err != nil {
		t.Fatalf("explicit Roster() = %v", err)
	}
	if len(reader.paths) != 1 || reader.paths[0] != second {
		t.Fatalf("decoded path = %q, want %q (other was %q)", reader.paths, second, first)
	}

	missing := newTestSource(t, root, strings.Repeat("3", 32), 0, reader, testPlayerProjector, testGuildProjector)
	if _, err := missing.Roster(context.Background()); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing explicit Roster() error = %v", err)
	}
}

func TestAutomaticSelectionIgnoresInvalidWorldsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	want := makeGeneration(t, root, testWorldOne, "generation")
	// A GUID-shaped directory without a complete native backup is not usable.
	if err := os.MkdirAll(filepath.Join(root, testWorldTwo, "backup", "world", "partial"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A complete non-GUID directory cannot become the active world.
	makeGeneration(t, root, "not-a-world-guid", "generation")
	// Nor can a GUID-shaped symlink escape the configured root.
	targetRoot := t.TempDir()
	makeGeneration(t, targetRoot, strings.Repeat("3", 32), "generation")
	if err := os.Symlink(filepath.Join(targetRoot, strings.Repeat("3", 32)), filepath.Join(root, strings.Repeat("3", 32))); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	reader := &fakeSnapshotReader{snapshot: &savegame.Snapshot{}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector, testGuildProjector)
	if _, err := source.Roster(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reader.paths) != 1 || reader.paths[0] != want {
		t.Fatalf("decoded paths = %q, want %q", reader.paths, want)
	}
}

func TestCompleteGenerationRejectsSymlinkAndWrongArtifactTypes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "symlinked level",
			mutate: func(t *testing.T, path string) {
				replaceWithSymlink(t, filepath.Join(path, "Level.sav"), filepath.Join(t.TempDir(), "level"), false)
			},
		},
		{
			name: "symlinked metadata",
			mutate: func(t *testing.T, path string) {
				replaceWithSymlink(t, filepath.Join(path, "LevelMeta.sav"), filepath.Join(t.TempDir(), "meta"), false)
			},
		},
		{
			name: "symlinked players",
			mutate: func(t *testing.T, path string) {
				replaceWithSymlink(t, filepath.Join(path, "Players"), filepath.Join(t.TempDir(), "players"), true)
			},
		},
		{
			name: "level is directory",
			mutate: func(t *testing.T, path string) {
				if err := os.Remove(filepath.Join(path, "Level.sav")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(path, "Level.sav"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "players is file",
			mutate: func(t *testing.T, path string) {
				if err := os.Remove(filepath.Join(path, "Players")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "Players"), []byte("not-directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := makeGeneration(t, t.TempDir(), testWorldOne, "generation")
			test.mutate(t, path)
			complete, err := completeGeneration(path)
			if err != nil {
				t.Fatal(err)
			}
			if complete {
				t.Fatal("generation with unsafe artifact was accepted")
			}
		})
	}
}

func TestDirectoryScanIsBoundedAndHonorsContext(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := readDirectoryBounded(context.Background(), root, 2); err == nil || !strings.Contains(err.Error(), "more than 2") {
		t.Fatalf("bounded scan error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readDirectoryBounded(cancelled, root, 4); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan error = %v, want context.Canceled", err)
	}

	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readDirectoryBounded(context.Background(), link, 4); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("symlinked root error = %v", err)
	}
}

func TestRosterProjectsAndSanitizesPlayers(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	x, y := -321000.0, 87000.0
	notFinite := math.NaN()
	validY := 1.0
	lastSeen := time.Date(2026, 7, 20, 23, 59, 0, 0, time.FixedZone("server", -7*3600))
	captureTotal, uniqueCaptured, paldeckUnlocked := int64(4321), 117, 119
	snapshotAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.FixedZone("server", 3600))
	rawPlayerOne := strings.Repeat("a", 32)
	rawPlayerTwo := strings.Repeat("b", 32)
	rawGuild := strings.Repeat("c", 32)
	rawCollisionOne := strings.Repeat("d", 32)
	rawCollisionTwo := strings.Repeat("e", 32)
	reader := &fakeSnapshotReader{snapshot: &savegame.Snapshot{
		SnapshotAt: snapshotAt,
		Players: []savegame.Player{
			{PlayerID: rawPlayerOne, DisplayName: " \x00 Alice\u202e\u2066\nAdmin\u2028\u2029\t ", Level: -4, GuildID: rawGuild, GuildName: " Builders\r\n ", X: &x, Y: &y, LastSeenAt: &lastSeen, CaptureTotal: &captureTotal, UniquePalsCaptured: &uniqueCaptured, PaldeckUnlocked: &paldeckUnlocked},
			{PlayerID: rawPlayerTwo, DisplayName: strings.Repeat("é", 60), Level: 5000, GuildID: strings.Repeat("f", 32), GuildName: "Must not survive", X: &notFinite, Y: &validY},
			{PlayerID: strings.Repeat("1", 32), DisplayName: "\x00\n\t", Level: 50},
			{PlayerID: strings.Repeat("2", 32), DisplayName: "Rejected ID", Level: 50},
			{PlayerID: rawCollisionOne, DisplayName: "Collision one", Level: 50},
			{PlayerID: rawCollisionTwo, DisplayName: "Collision two", Level: 50},
		},
	}}
	playerProjector := func(raw string) (string, bool) {
		switch raw {
		case rawPlayerOne:
			return "player:one", true
		case rawPlayerTwo:
			return "player:two", true
		case strings.Repeat("1", 32):
			return "player:blank-name", true
		case strings.Repeat("2", 32):
			return "", false
		case rawCollisionOne, rawCollisionTwo:
			return "player:collision", true
		default:
			return "", false
		}
	}
	guildProjector := func(raw string) (string, bool) {
		if raw == rawGuild {
			return "guild:one", true
		}
		return "", false
	}
	source := newTestSource(t, root, "", 0, reader, playerProjector, guildProjector)
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Players) != 2 {
		t.Fatalf("players = %#v, want two valid non-colliding records", roster.Players)
	}
	first, second := roster.Players[0], roster.Players[1]
	if first.ID != "player:one" || first.Name != "AliceAdmin" || first.Level != 0 || first.Online ||
		first.GuildKey != "guild:one" || first.GuildName != "Builders" ||
		first.X != x || first.Y != y || first.Map != "palpagos" || !first.LastSeenAt.Equal(lastSeen) || first.LastSeenAt.Location() != time.UTC ||
		first.CaptureTotal == nil || *first.CaptureTotal != captureTotal || first.UniquePalsCaptured == nil || *first.UniquePalsCaptured != uniqueCaptured ||
		first.PaldeckUnlocked == nil || *first.PaldeckUnlocked != paldeckUnlocked {
		t.Fatalf("first projected player = %#v", first)
	}
	if second.ID != "player:two" || second.Level != maxPlayerLevel || len(second.Name) > maxNameBytes || !strings.HasSuffix(second.Name, "é") ||
		second.GuildKey != "" || second.GuildName != "" || second.Map != "" || second.X != 0 || second.Y != 0 {
		t.Fatalf("second projected player = %#v", second)
	}
	if !roster.SnapshotAt.Equal(snapshotAt) || roster.SnapshotAt.Location() != time.UTC {
		t.Fatalf("SnapshotAt = %v (%v)", roster.SnapshotAt, roster.SnapshotAt.Location())
	}
	encoded, err := json.Marshal(roster.Players)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{rawPlayerOne, rawPlayerTwo, rawGuild, rawCollisionOne, rawCollisionTwo, "Must not survive"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("public players leaked private save value %q: %s", private, encoded)
		}
	}
}

func TestProjectIDRejectsPrivateGUIDAndUnsafeProjectorResults(t *testing.T) {
	raw := "00112233-4455-6677-8899-AABBCCDDEEFF"
	tests := []struct {
		name  string
		value string
		ok    bool
	}{
		{name: "safe opaque ID", value: "player:opaque-value", ok: true},
		{name: "raw identity", value: raw},
		{name: "canonical identity", value: "00112233445566778899aabbccddeeff"},
		{name: "prefixed private identity", value: "player:00112233445566778899aabbccddeeff"},
		{name: "control", value: "player:bad\nvalue"},
		{name: "invalid UTF-8", value: "player:\xff"},
		{name: "overlong", value: strings.Repeat("x", maxPublicIDBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := projectID(func(string) (string, bool) { return test.value, true }, raw)
			if ok != test.ok {
				t.Fatalf("projectID() = %q, %v; want ok=%v", got, ok, test.ok)
			}
		})
	}
}

func TestCompleteGenerationsRejectsSymlinkedBackupTree(t *testing.T) {
	t.Run("backup", func(t *testing.T) {
		root := t.TempDir()
		world := filepath.Join(root, testWorldOne)
		if err := os.MkdirAll(world, 0o700); err != nil {
			t.Fatal(err)
		}
		targetRoot := t.TempDir()
		makeGeneration(t, targetRoot, testWorldTwo, "generation")
		target := filepath.Join(targetRoot, testWorldTwo, "backup")
		if err := os.Symlink(target, filepath.Join(world, "backup")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		generations, err := completeGenerations(context.Background(), world)
		if err != nil || len(generations) != 0 {
			t.Fatalf("completeGenerations() = %#v, %v; want no generations", generations, err)
		}
	})

	t.Run("backup world", func(t *testing.T) {
		root := t.TempDir()
		world := filepath.Join(root, testWorldOne)
		if err := os.MkdirAll(filepath.Join(world, "backup"), 0o700); err != nil {
			t.Fatal(err)
		}
		targetRoot := t.TempDir()
		makeGeneration(t, targetRoot, testWorldTwo, "generation")
		target := filepath.Join(targetRoot, testWorldTwo, "backup", "world")
		if err := os.Symlink(target, filepath.Join(world, "backup", "world")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		generations, err := completeGenerations(context.Background(), world)
		if err != nil || len(generations) != 0 {
			t.Fatalf("completeGenerations() = %#v, %v; want no generations", generations, err)
		}
	})
}

func TestNonTimestampGenerationsFallBackToModificationTime(t *testing.T) {
	root := t.TempDir()
	older := makeGeneration(t, root, testWorldOne, "arbitrary-a")
	newer := makeGeneration(t, root, testWorldOne, "arbitrary-b")
	base := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	setMtime(t, older, base)
	setMtime(t, newer, base.Add(time.Minute))
	generations, err := completeGenerations(context.Background(), filepath.Join(root, testWorldOne))
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 2 || generations[0].path != newer || generations[1].path != older {
		t.Fatalf("generation order = %#v, want newer then older", generations)
	}
}

func TestRosterTimeoutCoversDecoder(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	reader := &fakeSnapshotReader{read: func(ctx context.Context, _ string) (*savegame.Snapshot, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	source := newTestSource(t, root, "", 10*time.Millisecond, reader, testPlayerProjector, testGuildProjector)
	started := time.Now()
	_, err := source.Roster(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Roster() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Roster() ignored timeout; elapsed %s", elapsed)
	}
}

func TestRosterRejectsNilSnapshotAndNilContext(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	reader := &fakeSnapshotReader{}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector, testGuildProjector)
	if _, err := source.Roster(context.Background()); err == nil || !strings.Contains(err.Error(), "no snapshot") {
		t.Fatalf("nil snapshot error = %v", err)
	}
	if _, err := source.Roster(nil); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil context error = %v", err)
	}
}

func newTestSource(t *testing.T, root, worldID string, timeout time.Duration, reader SnapshotReader, player, guild IDProjector) *Source {
	t.Helper()
	source, err := New(Options{
		Root: root, WorldID: worldID, Timeout: timeout, Reader: reader,
		ProjectPlayerID: player, ProjectGuildID: guild,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func makeGeneration(t *testing.T, root, worldID, name string) string {
	t.Helper()
	path := filepath.Join(root, worldID, "backup", "world", name)
	if err := os.MkdirAll(filepath.Join(path, "Players"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"Level.sav", "LevelMeta.sav"} {
		if err := os.WriteFile(filepath.Join(path, artifact), []byte("test-save"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func setMtime(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func replaceWithSymlink(t *testing.T, destination, target string, directory bool) {
	t.Helper()
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if directory {
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
	} else if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func testPlayerProjector(raw string) (string, bool) {
	if _, ok := canonicalWorldID(raw); !ok {
		return "", false
	}
	return "player:test", true
}

func testGuildProjector(raw string) (string, bool) {
	if _, ok := canonicalWorldID(raw); !ok {
		return "", false
	}
	return "guild:test", true
}
