package saveroster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
	"github.com/LukeHollandDev/palworld-live-map/internal/savesidecar"
)

const (
	testWorldOne = "11111111111111111111111111111111"
	testWorldTwo = "22222222222222222222222222222222"
)

type fakeSnapshotReader struct {
	snapshot *savesidecar.Snapshot
	err      error
	paths    []string
	read     func(context.Context, string) (*savesidecar.Snapshot, error)
}

func (f *fakeSnapshotReader) ReadSnapshot(ctx context.Context, path string) (*savesidecar.Snapshot, error) {
	f.paths = append(f.paths, path)
	if f.read != nil {
		return f.read(ctx, path)
	}
	return f.snapshot, f.err
}

type claimRead struct {
	path     string
	playerID string
}

type fakeClaimReader struct {
	documents map[string]savesidecar.ClaimPlayer
	err       error
	reads     []claimRead
}

func (*fakeClaimReader) KnowledgeQuizEnabled() bool { return true }

func (f *fakeClaimReader) ReadClaimPlayer(ctx context.Context, path, playerID string) (savesidecar.ClaimPlayer, error) {
	if err := ctx.Err(); err != nil {
		return savesidecar.ClaimPlayer{}, err
	}
	f.reads = append(f.reads, claimRead{path: path, playerID: playerID})
	if f.err != nil {
		return savesidecar.ClaimPlayer{}, f.err
	}
	document, ok := f.documents[path]
	if !ok {
		return savesidecar.ClaimPlayer{}, fmt.Errorf("no claim document for %q", path)
	}
	return document, nil
}

func TestNewValidatesConfiguration(t *testing.T) {
	valid := Options{
		Root: t.TempDir(), Reader: &fakeSnapshotReader{},
		ProjectPlayerID: testPlayerProjector, ProjectGuildKey: testGuildProjector,
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
		{name: "missing player projector", mutate: func(o *Options) { o.ProjectPlayerID = nil }, want: "projector"},
		{name: "missing guild projector", mutate: func(o *Options) { o.ProjectGuildKey = nil }, want: "guild key projector"},
		{name: "claim reader without secret", mutate: func(o *Options) { o.ClaimReader = &fakeClaimReader{} }, want: "configured together"},
		{name: "claim secret without reader", mutate: func(o *Options) { o.ClaimSecret = testClaimSecret() }, want: "configured together"},
		{name: "short claim secret", mutate: func(o *Options) {
			o.ClaimReader, o.ClaimSecret = &fakeClaimReader{}, []byte("too-short")
		}, want: "exactly 32 bytes"},
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
	disabled, err := New(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := disabled.Prepare(context.Background(), "player:any", 0); !errors.Is(err, playerclaim.ErrUnavailable) {
		t.Fatalf("Prepare() with claims disabled = %v, want ErrUnavailable", err)
	}
}

func TestClaimSubjectIsStableWorldScopedAndNeverPublished(t *testing.T) {
	const publicID = "player:public-alice"
	rawID := strings.Repeat("a", 32)
	prepare := func(worldID string) (playerclaim.Prepared, []byte) {
		t.Helper()
		root := t.TempDir()
		baseline := makeGeneration(t, root, worldID, "2026.07.21-10.00.00")
		makeGeneration(t, root, worldID, "2026.07.21-11.00.00")
		snapshotReader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{Players: []savesidecar.Player{{PlayerID: rawID, Name: "Alice"}}}}
		claimReader := &fakeClaimReader{documents: map[string]savesidecar.ClaimPlayer{
			baseline: inventoryClaimPlayer(rawID),
		}}
		source := newClaimTestSource(t, root, worldID, snapshotReader, claimReader, func(raw string) (string, bool) {
			return publicID, raw == rawID
		})
		roster, err := source.Roster(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := source.Prepare(context.Background(), publicID, 0)
		if err != nil {
			t.Fatal(err)
		}
		published, err := json.Marshal(roster)
		if err != nil {
			t.Fatal(err)
		}
		return prepared, published
	}

	first, firstPublic := prepare(testWorldOne)
	second, _ := prepare(testWorldOne)
	otherWorld, _ := prepare(testWorldTwo)
	if first.Subject == "" || first.Subject != second.Subject {
		t.Fatalf("same world/player subjects = %q and %q, want one stable non-empty subject", first.Subject, second.Subject)
	}
	if first.Subject == otherWorld.Subject {
		t.Fatalf("subjects for worlds %q and %q are equal: %q", testWorldOne, testWorldTwo, first.Subject)
	}
	if len(first.Subject) != 64 || first.Subject == rawID || first.Subject == publicID ||
		strings.Contains(first.Subject, rawID) || strings.Contains(first.Subject, publicID) {
		t.Fatalf("subject %q is not an opaque SHA-256-sized identifier", first.Subject)
	}
	for _, private := range []string{rawID, first.Subject} {
		if strings.Contains(string(firstPublic), private) {
			t.Fatalf("public roster leaked claim value %q: %s", private, firstPublic)
		}
	}
}

func TestClaimIsUnavailableForProjectorCollision(t *testing.T) {
	root := t.TempDir()
	baseline := makeGeneration(t, root, testWorldOne, "2026.07.21-10.00.00")
	firstRaw := strings.Repeat("a", 32)
	secondRaw := strings.Repeat("b", 32)
	const collision = "player:collision"
	claimReader := &fakeClaimReader{documents: map[string]savesidecar.ClaimPlayer{
		baseline: inventoryClaimPlayer(firstRaw),
	}}
	source := newClaimTestSource(t, root, testWorldOne, &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{
		Players: []savesidecar.Player{{PlayerID: firstRaw}, {PlayerID: secondRaw}},
	}}, claimReader, func(string) (string, bool) { return collision, true })
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Players) != 0 {
		t.Fatalf("collision roster = %#v, want both ambiguous players suppressed", roster.Players)
	}
	if _, err := source.Prepare(context.Background(), collision, 0); !errors.Is(err, playerclaim.ErrUnavailable) {
		t.Fatalf("Prepare() for colliding public ID = %v, want ErrUnavailable", err)
	}
	if len(claimReader.reads) != 0 {
		t.Fatalf("claim reader was consulted for ambiguous identity: %#v", claimReader.reads)
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
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{SnapshotAt: snapshotAt}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector)
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
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector)
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

func TestRosterSanitizesPartialDecoderFailure(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	privateDetail := "decoder stderr /private/save/" + strings.Repeat("a", 32) + " ClaimSecretItem state-key"
	resolveError := errors.New(privateDetail)
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{
		Stats: savesidecar.Stats{ResolveFailed: true, ResolveError: resolveError},
	}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector)
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if roster.PartialError == nil || roster.PartialError.Error() != "save decoder resolve failed" {
		t.Fatalf("PartialError = %v, want stable resolve category", roster.PartialError)
	}
	if strings.Contains(roster.PartialError.Error(), privateDetail) {
		t.Fatalf("PartialError leaked private decoder detail: %v", roster.PartialError)
	}
}

func TestWorldSelectionIsStrictAndUnambiguous(t *testing.T) {
	root := t.TempDir()
	first := makeGeneration(t, root, testWorldOne, "2026.07.21-10.00.00")
	second := makeGeneration(t, root, strings.ToUpper(testWorldTwo), "2026.07.21-10.00.00")
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{}}

	automatic := newTestSource(t, root, "", 0, reader, testPlayerProjector)
	if _, err := automatic.Roster(context.Background()); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("automatic Roster() error = %v, want ambiguity", err)
	}
	if len(reader.paths) != 0 {
		t.Fatalf("decoder was called during ambiguous selection: %q", reader.paths)
	}

	explicit := newTestSource(t, root, testWorldTwo, 0, reader, testPlayerProjector)
	if _, err := explicit.Roster(context.Background()); err != nil {
		t.Fatalf("explicit Roster() = %v", err)
	}
	if len(reader.paths) != 1 || reader.paths[0] != second {
		t.Fatalf("decoded path = %q, want %q (other was %q)", reader.paths, second, first)
	}

	missing := newTestSource(t, root, strings.Repeat("3", 32), 0, reader, testPlayerProjector)
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

	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{}}
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector)
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

// Names, levels, and guilds only exist once the decoder's resolve pass supplies
// them, and the guild GUID must leave through the projector like every other
// private identifier.
func TestRosterProjectsNamesLevelsAndGuilds(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	rawGuild := strings.Repeat("c", 32)
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{
		SnapshotAt: time.Now().UTC(),
		Players: []savesidecar.Player{
			{PlayerID: strings.Repeat("a", 32), Name: "Sable", Level: 42, GuildID: rawGuild, GuildName: "Aurora"},
			// A control character and an over-long name are player-controlled
			// input on their way to a browser.
			{PlayerID: strings.Repeat("b", 32), Name: "Bad\x00Name" + strings.Repeat("x", 200), Level: -3},
			// An unprojectable guild must cost the guild pair, not the player.
			{PlayerID: strings.Repeat("d", 32), Name: "Loner", Level: 7, GuildID: "not-a-guid", GuildName: "Phantom"},
		},
	}}
	source := newTestSource(t, root, "", 0, reader, func(raw string) (string, bool) {
		return "player:" + raw[:1], true
	})
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Players) != 3 {
		t.Fatalf("players = %#v", roster.Players)
	}
	named, sanitized, loner := roster.Players[0], roster.Players[1], roster.Players[2]

	wantGuild, _ := testGuildProjector(rawGuild)
	if named.Name != "Sable" || named.Level != 42 || named.GuildKey != wantGuild || named.GuildName != "Aurora" {
		t.Fatalf("named player = %#v, want the projected guild %q", named, wantGuild)
	}
	if strings.Contains(named.GuildKey, rawGuild) {
		t.Fatalf("guild key %q leaks the private GUID", named.GuildKey)
	}

	if strings.ContainsRune(sanitized.Name, 0) || len(sanitized.Name) > maxNameBytes {
		t.Fatalf("sanitized name = %q, want control characters stripped and length bounded", sanitized.Name)
	}
	if sanitized.Level != 0 {
		t.Fatalf("sanitized player level = %d, want a negative level dropped", sanitized.Level)
	}

	if loner.Name != "Loner" || loner.GuildKey != "" || loner.GuildName != "" {
		t.Fatalf("loner = %#v, want the player kept and the unprojectable guild dropped", loner)
	}
}

func TestRosterProjectsAndSanitizesPlayers(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	x, y := -321000.0, 87000.0
	notFinite := math.NaN()
	validY := 1.0
	lastSeen := time.Date(2026, 7, 20, 23, 59, 0, 0, time.FixedZone("server", -7*3600))
	captureTotal := int64(4321)
	uniqueCaptured, paldeckUnlocked := 117, 119
	arenaRankPoints, fastTravelUnlocked, areasDiscovered := 875, 64, 23
	bossDefeats, towerDefeats := 31, 7
	negativeMetric := -1
	snapshotAt := time.Date(2026, 7, 21, 12, 0, 0, 0, time.FixedZone("server", 3600))
	rawPlayerOne := strings.Repeat("a", 32)
	rawPlayerTwo := strings.Repeat("b", 32)
	rawCollisionOne := strings.Repeat("d", 32)
	rawCollisionTwo := strings.Repeat("e", 32)
	reader := &fakeSnapshotReader{snapshot: &savesidecar.Snapshot{
		SnapshotAt: snapshotAt,
		Players: []savesidecar.Player{
			{PlayerID: rawPlayerOne, X: &x, Y: &y, LastSeenAt: &lastSeen, CaptureTotal: &captureTotal, UniquePalsCaptured: &uniqueCaptured, PaldeckUnlocked: &paldeckUnlocked, ArenaRankPoints: &arenaRankPoints, FastTravelUnlocked: &fastTravelUnlocked, AreasDiscovered: &areasDiscovered, BossDefeats: &bossDefeats, TowerDefeats: &towerDefeats},
			{PlayerID: rawPlayerTwo, X: &notFinite, Y: &validY, ArenaRankPoints: &negativeMetric, FastTravelUnlocked: &negativeMetric, AreasDiscovered: &negativeMetric, BossDefeats: &negativeMetric, TowerDefeats: &negativeMetric},
			{PlayerID: strings.Repeat("2", 32)},
			{PlayerID: rawCollisionOne},
			{PlayerID: rawCollisionTwo},
		},
	}}
	playerProjector := func(raw string) (string, bool) {
		switch raw {
		case rawPlayerOne:
			return "player:one", true
		case rawPlayerTwo:
			return "player:two", true
		case strings.Repeat("2", 32):
			return "", false
		case rawCollisionOne, rawCollisionTwo:
			return "player:collision", true
		default:
			return "", false
		}
	}
	source := newTestSource(t, root, "", 0, reader, playerProjector)
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Players) != 2 {
		t.Fatalf("players = %#v, want two valid non-colliding records", roster.Players)
	}
	first, second := roster.Players[0], roster.Players[1]
	if first.ID != "player:one" || first.Name != "" || first.Level != 0 || first.Online ||
		first.GuildKey != "" || first.GuildName != "" ||
		first.X != x || first.Y != y || first.Map != "palpagos" || !first.LastSeenAt.Equal(lastSeen) || first.LastSeenAt.Location() != time.UTC ||
		first.CaptureTotal == nil || *first.CaptureTotal != captureTotal || first.UniquePalsCaptured == nil || *first.UniquePalsCaptured != uniqueCaptured ||
		first.PaldeckUnlocked == nil || *first.PaldeckUnlocked != paldeckUnlocked ||
		first.ArenaRankPoints == nil || *first.ArenaRankPoints != arenaRankPoints ||
		first.FastTravelUnlocked == nil || *first.FastTravelUnlocked != fastTravelUnlocked ||
		first.AreasDiscovered == nil || *first.AreasDiscovered != areasDiscovered ||
		first.BossDefeats == nil || *first.BossDefeats != bossDefeats ||
		first.TowerDefeats == nil || *first.TowerDefeats != towerDefeats {
		t.Fatalf("first projected player = %#v", first)
	}
	if second.ID != "player:two" || second.Name != "" || second.Level != 0 ||
		second.GuildKey != "" || second.GuildName != "" || second.Map != "" || second.X != 0 || second.Y != 0 ||
		second.ArenaRankPoints != nil || second.FastTravelUnlocked != nil || second.AreasDiscovered != nil ||
		second.BossDefeats != nil || second.TowerDefeats != nil {
		t.Fatalf("second projected player = %#v", second)
	}
	if !roster.SnapshotAt.Equal(snapshotAt) || roster.SnapshotAt.Location() != time.UTC {
		t.Fatalf("SnapshotAt = %v (%v)", roster.SnapshotAt, roster.SnapshotAt.Location())
	}
	encoded, err := json.Marshal(roster.Players)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{rawPlayerOne, rawPlayerTwo, rawCollisionOne, rawCollisionTwo} {
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

func TestCompleteGenerationsAcceptsLargeNativeBackupHistories(t *testing.T) {
	root := t.TempDir()
	generationsPath := filepath.Join(root, testWorldOne, "backup", "world")
	if err := os.MkdirAll(generationsPath, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 513; index++ {
		if err := os.Mkdir(filepath.Join(generationsPath, fmt.Sprintf("old-%04d", index)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	makeGeneration(t, root, testWorldOne, "2026.08.16-12.00.00")
	makeGeneration(t, root, testWorldOne, "2026.08.16-12.05.00")
	generations, err := completeGenerations(context.Background(), filepath.Join(root, testWorldOne))
	if err != nil || len(generations) != 2 {
		t.Fatalf("completeGenerations() = %d generations, %v; want two complete generations", len(generations), err)
	}
}

func TestRosterTimeoutCoversDecoder(t *testing.T) {
	root := t.TempDir()
	makeGeneration(t, root, testWorldOne, "generation")
	reader := &fakeSnapshotReader{read: func(ctx context.Context, _ string) (*savesidecar.Snapshot, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	source := newTestSource(t, root, "", 10*time.Millisecond, reader, testPlayerProjector)
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
	source := newTestSource(t, root, "", 0, reader, testPlayerProjector)
	if _, err := source.Roster(context.Background()); err == nil || !strings.Contains(err.Error(), "no snapshot") {
		t.Fatalf("nil snapshot error = %v", err)
	}
	if _, err := source.Roster(nil); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil context error = %v", err)
	}
}

func newTestSource(t *testing.T, root, worldID string, timeout time.Duration, reader SnapshotReader, player IDProjector) *Source {
	t.Helper()
	source, err := New(Options{
		Root: root, WorldID: worldID, Timeout: timeout, Reader: reader,
		ProjectPlayerID: player, ProjectGuildKey: testGuildProjector,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func newClaimTestSource(t *testing.T, root, worldID string, reader SnapshotReader, claimReader ClaimReader, player IDProjector) *Source {
	t.Helper()
	source, err := New(Options{
		Root: root, WorldID: worldID, Reader: reader,
		ProjectPlayerID: player, ProjectGuildKey: testGuildProjector,
		ClaimReader: claimReader, ClaimSecret: testClaimSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func testClaimSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func inventoryClaimPlayer(playerID string) savesidecar.ClaimPlayer {
	player := savesidecar.ClaimPlayer{
		PlayerID: playerID,
		Common:   make([]savesidecar.ClaimStack, 16),
		Progress: savesidecar.ClaimProgress{
			Available: true, FastTravel: []string{}, Areas: []string{}, Notes: []string{},
			Relics: []string{}, ItemPickups: []string{},
			NormalBosses: []string{}, TowerBosses: []string{},
		},
	}
	for index := range player.Common {
		player.Common[index] = savesidecar.ClaimStack{
			Slot: uint32(index), ItemID: fmt.Sprintf("ClaimSecretItem%02d", index), Count: uint32(index + 11),
		}
		if index%3 == 0 {
			player.Common[index].DynamicItemID = fmt.Sprintf("ClaimSecretInstance%02d", index)
		}
	}
	return player
}

func prepareInventoryClaim(t *testing.T, root, actualWorldID, configuredWorldID, rawID string) (*Source, *fakeClaimReader, playerclaim.Prepared) {
	t.Helper()
	const publicID = "player:alice"
	claimReader := &fakeClaimReader{documents: make(map[string]savesidecar.ClaimPlayer)}
	source := newClaimTestSource(t, root, configuredWorldID,
		&fakeSnapshotReader{snapshot: &savesidecar.Snapshot{Players: []savesidecar.Player{{PlayerID: rawID}}}},
		claimReader, func(candidate string) (string, bool) { return publicID, candidate == rawID })
	roster, err := source.Roster(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roster.Players) != 1 || roster.Players[0].ID != publicID {
		t.Fatalf("Roster() players = %#v, want claim target %q from world %q", roster.Players, publicID, actualWorldID)
	}
	prepared, err := source.Prepare(context.Background(), publicID, 0x0123456789abcdef)
	if err != nil {
		t.Fatal(err)
	}
	return source, claimReader, prepared
}

func makeIncompleteGeneration(t *testing.T, root, worldID, name string) string {
	t.Helper()
	path := filepath.Join(root, worldID, "backup", "world", name)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "Level.sav"), []byte("partial-save"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
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

func TestSelectKnowledgeQuizBuildsOneCyclableQuestionWithCharacterOptions(t *testing.T) {
	stacks := make([]savesidecar.ClaimStack, 8)
	for index := range stacks {
		stacks[index] = savesidecar.ClaimStack{Slot: uint32(index), ItemID: fmt.Sprintf("TestItem%d", index+1), Count: 1}
	}
	player := savesidecar.ClaimPlayer{
		Common:  stacks,
		Weapons: []savesidecar.ClaimStack{{Slot: 0, ItemID: "PrivateWeapon", Count: 1}},
		Party:   []savesidecar.ClaimPal{{Slot: 0, InstanceID: "private-instance", Species: "PrivatePal"}},
	}
	snapshotAt := time.Date(2026, time.August, 16, 1, 0, 0, 0, time.UTC)
	instructions, correct, remaining, ok := selectKnowledgeQuiz(player, 42, snapshotAt)
	if !ok || instructions.Kind != playerclaim.InventoryQuiz || len(instructions.Questions) != 1 || len(correct) != 1 || len(remaining) == 0 {
		t.Fatalf("selectKnowledgeQuiz() = %+v, %v, %v, %v", instructions, correct, remaining, ok)
	}
	for index, question := range instructions.Questions {
		answer, exists := correct[question.ID]
		if len(question.Options) != 8 || !question.CanCycle || !exists || answer < 0 || answer >= len(question.Options) {
			t.Fatalf("question %d = %+v, correct %d", index, question, answer)
		}
		seen := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			seen[option] = struct{}{}
		}
		if len(seen) != 8 {
			t.Fatalf("question %d options are not unique: %v", index, question.Options)
		}
	}
	for _, question := range instructions.Questions {
		for _, option := range question.Options {
			if !strings.HasPrefix(option, "Test Item") {
				t.Fatalf("question options were not grounded in the character's common inventory: %v", question.Options)
			}
		}
	}
	encoded, err := json.Marshal(instructions)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "correct") || strings.Contains(string(encoded), "TestItem") {
		t.Fatalf("quiz JSON exposed answers or raw item IDs: %s", encoded)
	}
	if got := humanizeItemID("MegaPalSphere"); got != "Mega Pal Sphere" {
		t.Fatalf("humanizeItemID() = %q", got)
	}
}

func TestSelectKnowledgeQuizAllowsPartyPalQuestions(t *testing.T) {
	player := savesidecar.ClaimPlayer{Party: []savesidecar.ClaimPal{
		{Slot: 0, InstanceID: "private-one", Species: "Lamball"},
		{Slot: 1, InstanceID: "private-two", Species: "Cattiva"},
		{Slot: 2, InstanceID: "private-three", Species: "Foxparks"},
	}}
	instructions, correct, remaining, ok := selectKnowledgeQuiz(
		player, 7, time.Date(2026, time.August, 16, 1, 0, 0, 0, time.UTC),
	)
	if !ok || len(instructions.Questions) != 1 || len(correct) != 1 || len(remaining) != 2 {
		t.Fatalf("selectKnowledgeQuiz() = %+v, %v, %v, %v", instructions, correct, remaining, ok)
	}
	for index, question := range instructions.Questions {
		if !strings.HasPrefix(question.Prompt, "Which Pal species was in party slot ") {
			t.Fatalf("question %d prompt = %q", index, question.Prompt)
		}
		if !reflect.DeepEqual(sortedStrings(question.Options), []string{"Cattiva", "Foxparks", "Lamball"}) {
			t.Fatalf("question %d options = %v; want only the character's party", index, question.Options)
		}
	}
}

func TestSelectKnowledgeQuizSkipsContainersWithoutThreeDistinctRealOptions(t *testing.T) {
	player := savesidecar.ClaimPlayer{
		Weapons: []savesidecar.ClaimStack{
			{Slot: 0, ItemID: "LaserRifle", Count: 1},
			{Slot: 1, ItemID: "RocketLauncher", Count: 1},
		},
		Armor: []savesidecar.ClaimStack{
			{Slot: 0, ItemID: "PalMetalArmor", Count: 1},
			{Slot: 1, ItemID: "LifePendant", Count: 1},
			{Slot: 2, ItemID: "AttackPendant", Count: 1},
		},
	}
	instructions, _, remaining, ok := selectKnowledgeQuiz(
		player, 17, time.Date(2026, time.August, 16, 1, 0, 0, 0, time.UTC),
	)
	if !ok || len(instructions.Questions) != 1 || len(remaining) != 2 {
		t.Fatalf("selectKnowledgeQuiz() = %+v, remaining %d, ok %v", instructions, len(remaining), ok)
	}
	for _, question := range instructions.Questions {
		if !strings.HasPrefix(question.Prompt, "What was equipped in equipment slot ") || len(question.Options) != 3 {
			t.Fatalf("question used an undersized or unrelated container: %+v", question)
		}
	}
}

func TestSelectKnowledgeQuizUsesOnlyTheFirstTwoCommonInventoryRows(t *testing.T) {
	privateStacks := func(prefix string) []savesidecar.ClaimStack {
		result := make([]savesidecar.ClaimStack, 8)
		for index := range result {
			result[index] = savesidecar.ClaimStack{Slot: uint32(index), ItemID: fmt.Sprintf("%s%d", prefix, index), Count: 1}
		}
		return result
	}
	player := savesidecar.ClaimPlayer{
		Common: []savesidecar.ClaimStack{
			{Slot: 0, ItemID: "Wood", Count: 1},
			{Slot: 5, ItemID: "Stone", Count: 1},
			{Slot: 11, ItemID: "Fiber", Count: 1},
			{Slot: 12, ItemID: "LateRowOre", Count: 1},
			{Slot: 20, ItemID: "LateRowCoal", Count: 1},
		},
		DropSlot:  privateStacks("DroppedSecret"),
		Essential: privateStacks("KeyItemSecret"),
	}
	instructions, _, remaining, ok := selectKnowledgeQuiz(
		player, 23, time.Date(2026, time.August, 16, 1, 0, 0, 0, time.UTC),
	)
	if !ok || len(instructions.Questions) != 1 || len(remaining) != 2 {
		t.Fatalf("selectKnowledgeQuiz() = %+v, remaining %d, ok %v", instructions, len(remaining), ok)
	}
	wantOptions := []string{"Fiber", "Stone", "Wood"}
	for _, question := range instructions.Questions {
		if !strings.HasPrefix(question.Prompt, "What was in inventory slot ") ||
			!reflect.DeepEqual(sortedStrings(question.Options), wantOptions) {
			t.Fatalf("question included a later row, dropped item, or key item: %+v", question)
		}
	}
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	slices.Sort(result)
	return result
}

func TestCycleQuestionReplacesTheCurrentQuestion(t *testing.T) {
	stacks := make([]savesidecar.ClaimStack, 8)
	for index := range stacks {
		stacks[index] = savesidecar.ClaimStack{Slot: uint32(index), ItemID: fmt.Sprintf("PrivateItem%d", index+1), Count: 1}
	}
	player := savesidecar.ClaimPlayer{
		Common:  stacks,
		Weapons: []savesidecar.ClaimStack{{Slot: 0, ItemID: "PrivateWeapon", Count: 1}},
		Armor:   []savesidecar.ClaimStack{{Slot: 0, ItemID: "PrivateArmor", Count: 1}},
	}
	instructions, correct, remaining, ok := selectKnowledgeQuiz(player, 99, time.Now().UTC())
	if !ok || len(remaining) < 2 {
		t.Fatalf("selectKnowledgeQuiz() remaining = %d, ok = %v", len(remaining), ok)
	}
	prepared := playerclaim.Prepared{
		Subject: "subject", PublicPlayerID: "player", Instructions: instructions,
		Evidence: &claimQuizEvidence{target: claimTarget{}, correct: correct, remaining: remaining},
	}
	q1ID := prepared.Instructions.Questions[0].ID
	if err := (&Source{}).CycleQuestion(context.Background(), &prepared, q1ID); err != nil {
		t.Fatalf("CycleQuestion(q1) error = %v", err)
	}
	if prepared.Instructions.Questions[0].ID == q1ID {
		t.Fatalf("Q1 cycle did not replace the question: %+v", prepared.Instructions.Questions)
	}
	q1After := prepared.Instructions.Questions[0]
	if err := (&Source{}).CycleQuestion(context.Background(), &prepared, q1After.ID); err != nil {
		t.Fatalf("CycleQuestion(replacement) error = %v", err)
	}
	if prepared.Instructions.Questions[0].ID == q1After.ID {
		t.Fatalf("second cycle did not replace the question: %+v", prepared.Instructions.Questions)
	}
}

// Distinct guilds must project to distinct keys without the result carrying any
// part of the private GUID, which is what the real keyed HMAC gives.
func testGuildProjector(raw string) (string, bool) {
	canonical, ok := canonicalPrivateGUID(raw)
	if !ok {
		return "", false
	}
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(canonical))
	return fmt.Sprintf("guild:test-%x", digest.Sum64()), true
}
