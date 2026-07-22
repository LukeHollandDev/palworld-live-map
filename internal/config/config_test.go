package config

import (
	"strings"
	"testing"
	"time"
)

func validEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("PALWORLD_REST_URL", "http://palworld:8212/")
	t.Setenv("PALWORLD_ADMIN_PASSWORD", "admin-secret")
}

func TestLoadDefaults(t *testing.T) {
	validEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RESTURL != "http://palworld:8212" {
		t.Fatalf("RESTURL = %q", cfg.RESTURL)
	}
	if cfg.DemoMode {
		t.Fatal("DemoMode = true, want false")
	}
	if cfg.PollInterval != 5*time.Second || cfg.UpstreamTimeout != 4*time.Second {
		t.Fatalf("unexpected player timing: poll=%s timeout=%s", cfg.PollInterval, cfg.UpstreamTimeout)
	}
	if cfg.WorldPollInterval != 15*time.Second || cfg.WorldTimeout != 10*time.Second || !cfg.WorldDataEnabled {
		t.Fatalf("unexpected world defaults: interval=%s timeout=%s enabled=%v", cfg.WorldPollInterval, cfg.WorldTimeout, cfg.WorldDataEnabled)
	}
	if cfg.SaveDataEnabled || cfg.SaveRoot != "/data/palworld/saves" || cfg.SavePollInterval != 30*time.Second || cfg.SaveTimeout != 20*time.Second || cfg.SaveOodleCacheDir != "/tmp/palworld-live-map/oodle" {
		t.Fatalf("unexpected save defaults: enabled=%v root=%q interval=%s timeout=%s", cfg.SaveDataEnabled, cfg.SaveRoot, cfg.SavePollInterval, cfg.SaveTimeout)
	}
}

func TestLoadDemoModeWithoutPalworldCredentials(t *testing.T) {
	t.Setenv("PALWORLD_REST_URL", "")
	t.Setenv("PALWORLD_ADMIN_PASSWORD", "")
	t.Setenv("DEMO_MODE", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.DemoMode || cfg.RESTURL != "" || cfg.AdminPassword != "" {
		t.Fatalf("unexpected demo config: %+v", cfg)
	}
}

func TestLoadRejectsSaveDataInDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("SAVE_DATA_ENABLED", "true")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DEMO_MODE") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRealModeRequiresPalworldCredentials(t *testing.T) {
	t.Setenv("PALWORLD_REST_URL", "")
	t.Setenv("PALWORLD_ADMIN_PASSWORD", "")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PALWORLD_REST_URL") || !strings.Contains(err.Error(), "PALWORLD_ADMIN_PASSWORD") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "duration", key: "POLL_INTERVAL", value: "quickly", want: "POLL_INTERVAL"},
		{name: "boolean", key: "WORLD_DATA_ENABLED", value: "sometimes", want: "WORLD_DATA_ENABLED"},
		{name: "demo boolean", key: "DEMO_MODE", value: "sometimes", want: "DEMO_MODE"},
		{name: "poll too short", key: "POLL_INTERVAL", value: "1s", want: "at least 2s"},
		{name: "world timeout", key: "WORLD_TIMEOUT", value: "20s", want: "shorter"},
		{name: "save boolean", key: "SAVE_DATA_ENABLED", value: "sometimes", want: "SAVE_DATA_ENABLED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv(tt.key, tt.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestLoadValidatesEnabledSaveReader(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "relative root", key: "PALWORLD_SAVE_ROOT", value: "saves", want: "absolute"},
		{name: "poll too short", key: "SAVE_POLL_INTERVAL", value: "10s", want: "at least 15s"},
		{name: "timeout", key: "SAVE_TIMEOUT", value: "30s", want: "shorter"},
		{name: "relative library", key: "SAVE_OODLE_LIBRARY", value: "liboodle.so", want: "absolute"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv(test.key, test.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestLoadDoesNotValidateUnusedSaveTimingWhenSaveDataIsDisabled(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "false")
	t.Setenv("PALWORLD_SAVE_ROOT", "relative")
	t.Setenv("SAVE_POLL_INTERVAL", "1s")
	t.Setenv("SAVE_TIMEOUT", "2m")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadAcceptsExplicitSaveLibrary(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "true")
	t.Setenv("SAVE_OODLE_LIBRARY", "/opt/palworld/liboo2corelinux64.so.9")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SaveDataEnabled || cfg.SaveOodleLibrary == "" || cfg.SaveOodleURL != "" {
		t.Fatalf("save config = %+v", cfg)
	}
}

func TestLoadAcceptsPinnedSaveLibraryDownload(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "true")
	t.Setenv("SAVE_OODLE_DOWNLOAD_URL", "https://downloads.example.invalid/oodle.so")
	t.Setenv("SAVE_OODLE_SHA256", strings.Repeat("a1", 32))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SaveOodleURL == "" || cfg.SaveOodleSHA256 != strings.Repeat("a1", 32) {
		t.Fatalf("save config = %+v", cfg)
	}
}

func TestLoadRequiresOneValidSaveLibrarySource(t *testing.T) {
	tests := []struct {
		name    string
		library string
		url     string
		digest  string
		cache   string
		want    string
	}{
		{name: "missing", want: "exactly one"},
		{name: "both", library: "/opt/oodle.so", url: "https://example.invalid/oodle.so", digest: strings.Repeat("01", 32), want: "exactly one"},
		{name: "partial download", url: "https://example.invalid/oodle.so", want: "SHA256"},
		{name: "insecure download", url: "http://example.invalid/oodle.so", digest: strings.Repeat("01", 32), want: "HTTPS URL"},
		{name: "invalid digest", url: "https://example.invalid/oodle.so", digest: "not-a-digest", want: "SHA256"},
		{name: "relative cache", library: "/opt/oodle.so", cache: "cache", want: "CACHE_DIR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("SAVE_OODLE_LIBRARY", test.library)
			t.Setenv("SAVE_OODLE_DOWNLOAD_URL", test.url)
			t.Setenv("SAVE_OODLE_SHA256", test.digest)
			if test.cache != "" {
				t.Setenv("SAVE_OODLE_CACHE_DIR", test.cache)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestLoadDoesNotValidateUnusedWorldTimingWhenWorldDataIsDisabled(t *testing.T) {
	validEnvironment(t)
	t.Setenv("WORLD_DATA_ENABLED", "false")
	t.Setenv("WORLD_POLL_INTERVAL", "1s")
	t.Setenv("WORLD_TIMEOUT", "20s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WorldDataEnabled {
		t.Fatal("WorldDataEnabled = true")
	}
}
