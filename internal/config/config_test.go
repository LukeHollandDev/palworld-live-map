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
