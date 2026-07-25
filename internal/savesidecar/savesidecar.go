package savesidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultBinaryName           = "savedecode"
	playerPresetName            = "player-details"
	maxPlayerFiles              = 10_000
	maxOutputBytes              = 16 << 20
	maxStderrBytes              = 8 << 10
	presetProbeTimeout          = 5 * time.Second
	unrealUnixEpochTicks uint64 = 621355968000000000
)

// Options configures a Reader.
type Options struct {
	// BinaryPath is the absolute path to savedecode. When empty, the Reader
	// looks beside the running server executable.
	BinaryPath string
	// GameVersion selects one exact versioned player-details preset.
	GameVersion string
	// MaxOutputBytes overrides the per-process stdout cap. Zero uses the
	// default.
	MaxOutputBytes int64
}

// Reader invokes the refactored savedecode CLI once per immutable player save
// and aggregates its versioned player-details projections.
type Reader struct {
	binary      string
	gameVersion string
	maxOutput   int64
}

// NewReader resolves the executable and verifies that it advertises the exact
// preset/version contract needed by the application.
func NewReader(options Options) (*Reader, error) {
	binary := strings.TrimSpace(options.BinaryPath)
	if binary == "" {
		resolved, err := binaryNextToExecutable()
		if err != nil {
			return nil, err
		}
		binary = resolved
	}
	if !filepath.IsAbs(binary) {
		return nil, errors.New("save decoder path must be absolute")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil, fmt.Errorf("locate save decoder: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("save decoder is not a regular file: %s", binary)
	}
	gameVersion := strings.TrimSpace(options.GameVersion)
	if gameVersion == "" {
		return nil, errors.New("save decoder game version is required")
	}
	maxOutput := options.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = maxOutputBytes
	}
	reader := &Reader{binary: binary, gameVersion: gameVersion, maxOutput: maxOutput}
	ctx, cancel := context.WithTimeout(context.Background(), presetProbeTimeout)
	defer cancel()
	data, err := reader.run(ctx, "--list-presets")
	if err != nil {
		return nil, fmt.Errorf("inspect save decoder presets: %w", err)
	}
	var presets []preset
	if err := decodeSingleJSON(data, &presets); err != nil {
		return nil, fmt.Errorf("decode save decoder presets: %w", err)
	}
	for _, candidate := range presets {
		if candidate.Name == playerPresetName && candidate.GameVersion == gameVersion && candidate.SaveType == "player.sav" {
			return reader, nil
		}
	}
	return nil, fmt.Errorf("save decoder has no %s preset for Palworld %s", playerPresetName, gameVersion)
}

func binaryNextToExecutable() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate server executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return "", fmt.Errorf("resolve server executable: %w", err)
	}
	return filepath.Join(filepath.Dir(self), defaultBinaryName), nil
}

// ReadSnapshot projects every regular, non-DPS player save in dir. Individual
// corrupt files are tolerated; a generation where every player file fails is
// rejected so schema drift cannot silently publish an empty result.
func (r *Reader) ReadSnapshot(ctx context.Context, dir string) (*Snapshot, error) {
	if ctx == nil {
		return nil, errors.New("save decoder requires a context")
	}
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "." || !filepath.IsAbs(dir) {
		return nil, errors.New("save decoder requires an absolute snapshot directory")
	}
	levelPath := filepath.Join(dir, "Level.sav")
	levelInfo, err := regularFileWithoutSymlink(levelPath)
	if err != nil {
		return nil, fmt.Errorf("inspect snapshot Level.sav: %w", err)
	}
	playersPath := filepath.Join(dir, "Players")
	if ok, err := directoryWithoutSymlink(playersPath); err != nil {
		return nil, fmt.Errorf("inspect snapshot Players directory: %w", err)
	} else if !ok {
		return nil, errors.New("snapshot Players path is not a non-symlink directory")
	}
	entries, err := os.ReadDir(playersPath)
	if err != nil {
		return nil, fmt.Errorf("read snapshot Players directory: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if entry.IsDir() || !strings.HasSuffix(lower, ".sav") || strings.HasSuffix(lower, "_dps.sav") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("player save is a symlink: %s", name)
		}
		files = append(files, name)
	}
	if len(files) > maxPlayerFiles {
		return nil, fmt.Errorf("snapshot contains more than %d player saves", maxPlayerFiles)
	}
	sort.Strings(files)

	snapshot := &Snapshot{
		SnapshotAt: levelInfo.ModTime().UTC(),
		Players:    []Player{},
	}
	byID := make(map[string]int, len(files))
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(playersPath, name)
		if _, err := regularFileWithoutSymlink(path); err != nil {
			return nil, fmt.Errorf("inspect player save %s: %w", name, err)
		}
		snapshot.Stats.PlayerFiles++
		data, err := r.run(
			ctx,
			"--preset", playerPresetName,
			"--game-version", r.gameVersion,
			path,
		)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			snapshot.Stats.DecodeFailures++
			continue
		}
		player, err := DecodePlayer(data)
		if err != nil {
			snapshot.Stats.DecodeFailures++
			continue
		}
		key := strings.ToLower(player.PlayerID)
		if index, duplicate := byID[key]; duplicate {
			snapshot.Stats.DuplicatePlayers++
			if newerPlayer(player, snapshot.Players[index]) {
				snapshot.Players[index] = player
			}
			continue
		}
		byID[key] = len(snapshot.Players)
		snapshot.Players = append(snapshot.Players, player)
	}
	if len(files) > 0 && len(snapshot.Players) == 0 {
		return nil, fmt.Errorf("save decoder failed for all %d player saves", len(files))
	}
	after, err := regularFileWithoutSymlink(levelPath)
	if err != nil || after.Size() != levelInfo.Size() || !after.ModTime().Equal(levelInfo.ModTime()) {
		return nil, errors.New("snapshot Level.sav changed during player decoding")
	}
	sort.Slice(snapshot.Players, func(i, j int) bool {
		return strings.ToLower(snapshot.Players[i].PlayerID) < strings.ToLower(snapshot.Players[j].PlayerID)
	})
	return snapshot, nil
}

// DecodePlayer parses and derives the application fields from one
// player-details projection.
func DecodePlayer(data []byte) (Player, error) {
	var raw playerProjection
	if err := decodeSingleJSON(data, &raw); err != nil {
		return Player{}, fmt.Errorf("decode player-details JSON: %w", err)
	}
	if strings.TrimSpace(raw.PlayerUID) == "" {
		return Player{}, errors.New("decode player-details JSON: missing PlayerUId")
	}
	if raw.LastTransform == nil || raw.LastTransform.Translation == nil {
		return Player{}, errors.New("decode player-details JSON: missing LastTransform.Translation")
	}
	if raw.RecordData == nil || raw.RecordData.TribeCaptureCount == nil ||
		raw.RecordData.PalCaptureCount == nil || raw.RecordData.PaldeckUnlockFlag == nil {
		return Player{}, errors.New("decode player-details JSON: incomplete RecordData")
	}
	if raw.LastOnline == nil {
		return Player{}, errors.New("decode player-details JSON: missing LastOnlineDateTime")
	}
	x, y := raw.LastTransform.Translation.X, raw.LastTransform.Translation.Y
	player := Player{PlayerID: raw.PlayerUID, X: &x, Y: &y}
	if lastSeen, ok := unrealDateTime(*raw.LastOnline); ok {
		player.LastSeenAt = &lastSeen
	}
	if *raw.RecordData.TribeCaptureCount >= 0 && *raw.RecordData.TribeCaptureCount <= math.MaxInt {
		value := int(*raw.RecordData.TribeCaptureCount)
		player.UniquePalsCaptured = &value
	}
	player.CaptureTotal = captureTotal(raw.RecordData.PalCaptureCount)
	player.PaldeckUnlocked = paldeckUnlocked(raw.RecordData.PaldeckUnlockFlag)
	return player, nil
}

func decodeSingleJSON(data []byte, destination any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("save decoder produced no output")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("save decoder produced multiple JSON values")
		}
		return err
	}
	return nil
}

func (r *Reader) run(ctx context.Context, arguments ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.binary, arguments...)
	stderr := &cappedBuffer{limit: maxStderrBytes}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare save decoder: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start save decoder: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, r.maxOutput+1))
	waitErr := cmd.Wait()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if waitErr != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("save decoder failed: %w: %s", waitErr, message)
		}
		return nil, fmt.Errorf("save decoder failed: %w", waitErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("read save decoder output: %w", readErr)
	}
	if int64(len(data)) > r.maxOutput {
		return nil, fmt.Errorf("save decoder output exceeds %d bytes", r.maxOutput)
	}
	return data, nil
}

func captureTotal(entries []countEntry) *int64 {
	seen := make(map[string]struct{}, len(entries))
	var total int64
	for _, entry := range entries {
		key := strings.ToLower(strings.TrimSpace(entry.Key))
		if key == "" || entry.Value < 0 {
			return nil
		}
		if _, duplicate := seen[key]; duplicate {
			return nil
		}
		seen[key] = struct{}{}
		if key == "human" {
			continue
		}
		if entry.Value > math.MaxInt64-total {
			return nil
		}
		total += entry.Value
	}
	return &total
}

func paldeckUnlocked(entries []flagEntry) *int {
	seen := make(map[string]struct{}, len(entries))
	total := 0
	for _, entry := range entries {
		key := strings.ToLower(strings.TrimSpace(entry.Key))
		if key == "" {
			return nil
		}
		if _, duplicate := seen[key]; duplicate {
			return nil
		}
		seen[key] = struct{}{}
		if entry.Value && key != "human" {
			total++
		}
	}
	return &total
}

func unrealDateTime(ticks uint64) (time.Time, bool) {
	if ticks < unrealUnixEpochTicks {
		return time.Time{}, false
	}
	delta := ticks - unrealUnixEpochTicks
	seconds := delta / 10_000_000
	if seconds > 253402300799 {
		return time.Time{}, false
	}
	nanos := (delta % 10_000_000) * 100
	return time.Unix(int64(seconds), int64(nanos)).UTC(), true
}

func newerPlayer(candidate, current Player) bool {
	if candidate.LastSeenAt == nil {
		return false
	}
	return current.LastSeenAt == nil || candidate.LastSeenAt.After(*current.LastSeenAt)
}

func regularFileWithoutSymlink(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("path is not a non-symlink regular file")
	}
	return info, nil
}

func directoryWithoutSymlink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir(), nil
}

type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(data []byte) (int, error) {
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if len(data) > remaining {
			c.buf.Write(data[:remaining])
		} else {
			c.buf.Write(data)
		}
	}
	return len(data), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
