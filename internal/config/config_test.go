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
	t.Setenv("PLAYER_CLAIMS_ENABLED", "false")
}

func TestLoadDefaults(t *testing.T) {
	validEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.RESTURL != "http://palworld:8212" || cfg.DemoMode {
		t.Fatalf("unexpected base config: %+v", cfg)
	}
	if cfg.PollInterval != 5*time.Second || cfg.UpstreamTimeout != 4*time.Second {
		t.Fatalf("unexpected player timing: %+v", cfg)
	}
	if cfg.WorldPollInterval != 15*time.Second || cfg.WorldTimeout != 10*time.Second || !cfg.WorldDataEnabled {
		t.Fatalf("unexpected world config: %+v", cfg)
	}
	if cfg.SaveDataEnabled || cfg.SaveRoot != "/data/palworld/saves" || cfg.SavePollInterval != 30*time.Second || cfg.SaveTimeout != 20*time.Second {
		t.Fatalf("unexpected save config: %+v", cfg)
	}
	if cfg.PlayerClaimsEnabled {
		t.Fatal("PlayerClaimsEnabled = true")
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
	tests := []struct{ name, key, value, want string }{
		{"duration", "POLL_INTERVAL", "quickly", "POLL_INTERVAL"},
		{"boolean", "WORLD_DATA_ENABLED", "sometimes", "WORLD_DATA_ENABLED"},
		{"demo boolean", "DEMO_MODE", "sometimes", "DEMO_MODE"},
		{"poll too short", "POLL_INTERVAL", "1s", "at least 2s"},
		{"world timeout", "WORLD_TIMEOUT", "20s", "shorter"},
		{"save boolean", "SAVE_DATA_ENABLED", "sometimes", "SAVE_DATA_ENABLED"},
		{"claim boolean", "PLAYER_CLAIMS_ENABLED", "sometimes", "PLAYER_CLAIMS_ENABLED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv(test.key, test.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadValidatesEnabledSaveData(t *testing.T) {
	tests := []struct{ name, key, value, want string }{
		{"relative root", "PALWORLD_SAVE_ROOT", "saves", "absolute"},
		{"poll too short", "SAVE_POLL_INTERVAL", "10s", "at least 15s"},
		{"timeout", "SAVE_TIMEOUT", "30s", "shorter"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv(test.key, test.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadDoesNotValidateUnusedSaveTimingWhenDisabled(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "false")
	t.Setenv("PALWORLD_SAVE_ROOT", "relative")
	t.Setenv("SAVE_POLL_INTERVAL", "1s")
	t.Setenv("SAVE_TIMEOUT", "2m")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadAcceptsEnabledSaveReader(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "true")
	cfg, err := Load()
	if err != nil || !cfg.SaveDataEnabled {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestLoadDoesNotValidateUnusedWorldTimingWhenDisabled(t *testing.T) {
	validEnvironment(t)
	t.Setenv("WORLD_DATA_ENABLED", "false")
	t.Setenv("WORLD_POLL_INTERVAL", "1s")
	t.Setenv("WORLD_TIMEOUT", "20s")
	cfg, err := Load()
	if err != nil || cfg.WorldDataEnabled {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestLoadPlayerClaimsOnlyRequireSaveData(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		validEnvironment(t)
		t.Setenv("SAVE_DATA_ENABLED", "true")
		t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
		cfg, err := Load()
		if err != nil || !cfg.PlayerClaimsEnabled {
			t.Fatalf("Load() = %+v, %v", cfg, err)
		}
	})
	t.Run("save data", func(t *testing.T) {
		validEnvironment(t)
		t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "SAVE_DATA_ENABLED") {
			t.Fatalf("Load() error = %v", err)
		}
	})
	t.Run("demo mode", func(t *testing.T) {
		validEnvironment(t)
		t.Setenv("DEMO_MODE", "true")
		t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "DEMO_MODE") {
			t.Fatalf("Load() error = %v", err)
		}
	})
}
