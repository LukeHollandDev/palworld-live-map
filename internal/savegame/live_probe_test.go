package savegame

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestLiveSnapshotProbe is opt-in so CI never needs a proprietary Oodle
// runtime or real save data. It reports aggregate coverage only—never player,
// guild, or account identifiers and names.
func TestLiveSnapshotProbe(t *testing.T) {
	snapshotDir := os.Getenv("PALWORLD_LIVE_SNAPSHOT")
	oodlePath := os.Getenv("PALWORLD_LIVE_OODLE")
	if snapshotDir == "" || oodlePath == "" {
		t.Skip("set PALWORLD_LIVE_SNAPSHOT and PALWORLD_LIVE_OODLE to run")
	}
	reader, err := NewReader(Options{OodleLibraryPath: oodlePath})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := reader.ReadSnapshot(ctx, snapshotDir)
	if err != nil {
		raw, _, readErr := readSave(filepath.Join(snapshotDir, "Level.sav"), reader.maxSaveBytes, reader.oodle)
		stats := newStats()
		gvas, parseErr := parseGVAS(raw, &stats)
		if readErr == nil && parseErr == nil {
			properties := make([]string, 0, len(gvas.Properties))
			for name, property := range gvas.Properties {
				properties = append(properties, name+":"+property.Type)
			}
			sort.Strings(properties)
			t.Logf("root schema=%v skips=%v failures=%v", properties, stats.SkippedDetails, stats.DecodeFailures)
		}
		t.Fatal(err)
	}
	positioned, guildMembers, lastSeen, progressPlayers := 0, 0, 0, 0
	var captureTotal int64
	uniqueCaptured, paldeckUnlocked := 0, 0
	for _, player := range snapshot.Players {
		if player.PlayerID == "" || player.DisplayName == "" || player.Level <= 0 {
			t.Fatal("live snapshot contains an incomplete player record")
		}
		if player.X != nil && player.Y != nil {
			positioned++
		}
		if player.GuildID != "" && player.GuildName != "" {
			guildMembers++
		}
		if player.LastSeenAt != nil {
			lastSeen++
		}
		if player.CaptureTotal != nil && player.UniquePalsCaptured != nil && player.PaldeckUnlocked != nil {
			progressPlayers++
			captureTotal += *player.CaptureTotal
			uniqueCaptured += *player.UniquePalsCaptured
			paldeckUnlocked += *player.PaldeckUnlocked
		}
	}
	if len(snapshot.Players) == 0 || positioned == 0 || guildMembers == 0 || progressPlayers == 0 {
		t.Fatalf("insufficient aggregate extraction: players=%d positioned=%d guildMembers=%d lastSeen=%d progress=%d", len(snapshot.Players), positioned, guildMembers, lastSeen, progressPlayers)
	}
	t.Logf("aggregate extraction: players=%d positioned=%d guildMembers=%d lastSeen=%d progress=%d captures=%d uniqueCaptured=%d paldeckUnlocked=%d playerFiles=%d skips=%d failures=%v",
		len(snapshot.Players), positioned, guildMembers, lastSeen, progressPlayers, captureTotal, uniqueCaptured,
		paldeckUnlocked, snapshot.Stats.PlayerFiles, snapshot.Stats.SkippedProperties, snapshot.Stats.DecodeFailures)
}
