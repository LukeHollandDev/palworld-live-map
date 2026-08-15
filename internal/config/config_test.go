package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("PALWORLD_REST_URL", "http://palworld:8212/")
	t.Setenv("PALWORLD_ADMIN_PASSWORD", "admin-secret")
	t.Setenv("PLAYER_CLAIMS_ENABLED", "false")
	t.Setenv("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK", "false")
	t.Setenv("PLAYER_CLAIMS_ORIGIN", "")
	t.Setenv("PLAYER_CLAIMS_SECRET_FILE", "")
	t.Setenv("PLAYER_CLAIMS_TRUSTED_PROXIES", "")
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
	if cfg.SaveDataEnabled || cfg.SaveRoot != "/data/palworld/saves" ||
		cfg.SavePollInterval != 30*time.Second || cfg.SaveTimeout != 20*time.Second {
		t.Fatalf("unexpected save defaults: enabled=%v root=%q interval=%s timeout=%s", cfg.SaveDataEnabled, cfg.SaveRoot, cfg.SavePollInterval, cfg.SaveTimeout)
	}
	if cfg.PlayerClaimsEnabled || cfg.PlayerClaimsInsecureLocal || cfg.PlayerClaimsOrigin != "" || cfg.PlayerClaimsSecret != [32]byte{} || len(cfg.PlayerClaimsTrustedProxies) != 0 {
		t.Fatalf("unexpected player claim defaults: enabled=%v origin=%q", cfg.PlayerClaimsEnabled, cfg.PlayerClaimsOrigin)
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
		{name: "claim boolean", key: "PLAYER_CLAIMS_ENABLED", value: "sometimes", want: "PLAYER_CLAIMS_ENABLED"},
		{name: "insecure claim boolean", key: "PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK", value: "sometimes", want: "PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK"},
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

func TestLoadValidatesEnabledSaveData(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "relative root", key: "PALWORLD_SAVE_ROOT", value: "saves", want: "absolute"},
		{name: "poll too short", key: "SAVE_POLL_INTERVAL", value: "10s", want: "at least 15s"},
		{name: "timeout", key: "SAVE_TIMEOUT", value: "30s", want: "shorter"},
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

func TestLoadAcceptsEnabledSaveReader(t *testing.T) {
	validEnvironment(t)
	t.Setenv("SAVE_DATA_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.SaveDataEnabled {
		t.Fatalf("save config = %+v", cfg)
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

func TestLoadAcceptsPlayerClaimsWithRawOrHexSecret(t *testing.T) {
	tests := []struct {
		name     string
		contents []byte
		want     [32]byte
	}{
		{
			name:     "raw",
			contents: bytes.Repeat([]byte{0xa5}, 32),
			want:     [32]byte{0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5, 0xa5},
		},
		{
			name:     "hex",
			contents: []byte(strings.Repeat("2f", 32)),
			want:     [32]byte{0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f, 0x2f},
		},
		{
			name:     "hex newline",
			contents: append([]byte(strings.Repeat("3a", 32)), '\n'),
			want:     [32]byte{0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a, 0x3a},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			path := writeClaimSecret(t, test.contents, 0o600)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", "https://map.example.test:8443")
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", path)
			t.Setenv("PLAYER_CLAIMS_TRUSTED_PROXIES", "10.0.0.0/8, 2001:db8::/32,10.0.0.0/8")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !cfg.PlayerClaimsEnabled || cfg.PlayerClaimsOrigin != "https://map.example.test:8443" || cfg.PlayerClaimsSecret != test.want {
				t.Fatalf("unexpected player claims config: enabled=%v origin=%q secretMatches=%v", cfg.PlayerClaimsEnabled, cfg.PlayerClaimsOrigin, cfg.PlayerClaimsSecret == test.want)
			}
			if len(cfg.PlayerClaimsTrustedProxies) != 2 || cfg.PlayerClaimsTrustedProxies[0].String() != "10.0.0.0/8" || cfg.PlayerClaimsTrustedProxies[1].String() != "2001:db8::/32" {
				t.Fatalf("trusted proxies = %v", cfg.PlayerClaimsTrustedProxies)
			}
		})
	}
}

func TestLoadValidatesPlayerClaimsTrustedProxies(t *testing.T) {
	for _, value := range []string{"not-a-network", "127.0.0.1", "10.0.0.0/99", "10.0.0.0/8,,192.0.2.0/24"} {
		t.Run(value, func(t *testing.T) {
			validEnvironment(t)
			path := writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o600)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", "https://map.example.test")
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", path)
			t.Setenv("PLAYER_CLAIMS_TRUSTED_PROXIES", value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "PLAYER_CLAIMS_TRUSTED_PROXIES") {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadPlayerClaimsRequireSaveDataAndNonDemoMode(t *testing.T) {
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

func TestClaimsOriginAcceptsOnlyCanonicalBrowserSerialization(t *testing.T) {
	for _, value := range []string{
		"https://map.example.test",
		"https://map.example.test.",
		"https://map.example.test:8443",
		"https://dead.beef",
		"https://face.cafe",
		"https://0x7f.example",
		"https://192.0.2.1",
		"https://[2001:db8::1]",
		"https://[2001:db8::1]:8443",
	} {
		t.Run(value, func(t *testing.T) {
			got, err := claimsOrigin(value)
			if err != nil || got != value {
				t.Fatalf("claimsOrigin(%q) = %q, %v", value, got, err)
			}
		})
	}
}

func TestLoadValidatesPlayerClaimsOrigin(t *testing.T) {
	tests := []string{
		"",
		"http://map.example.test",
		"http://localhost:8080",
		"HTTPS://map.example.test",
		"https://MAP.example.test",
		"https://map.example.test:443",
		"https://map.example.test:08443",
		"https://127.1",
		"https://2130706433",
		"https://127.1.",
		"https://2130706433.",
		"https://0x7f000001",
		"https://0x7f.1",
		"https://0x7f.1.",
		"https://127.0x0.0x0.0x1",
		"https://dead.1",
		"https://dead.1.",
		"https://7f.1",
		"https://1e3.1",
		"https://foo.0x",
		"https://[2001:0db8:0:0::1]",
		"https://[::ffff:192.0.2.1]",
		"https://[2001:db8::192.0.2.1]",
		"https://mäp.example.test",
		"https://user@map.example.test",
		"https://map.example.test/",
		"https://map.example.test/path",
		"https://map.example.test?query=yes",
		"https://map.example.test#fragment",
		" https://map.example.test",
	}
	for _, value := range tests {
		name := value
		if name == "" {
			name = "missing"
		}
		t.Run(name, func(t *testing.T) {
			validEnvironment(t)
			path := writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o600)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", value)
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", path)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "PLAYER_CLAIMS_ORIGIN") {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadAllowsInsecureClaimsOnlyOnExplicitLoopback(t *testing.T) {
	for _, value := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	} {
		t.Run("accept "+value, func(t *testing.T) {
			validEnvironment(t)
			path := writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o600)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", value)
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", path)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !cfg.PlayerClaimsInsecureLocal || cfg.PlayerClaimsOrigin != value {
				t.Fatalf("insecure claims config = enabled=%v origin=%q", cfg.PlayerClaimsInsecureLocal, cfg.PlayerClaimsOrigin)
			}
		})
	}

	for _, value := range []string{
		"http://map.example.test:8080",
		"http://192.0.2.1:8080",
		"http://127.1:8080",
		"http://localhost:80",
		"HTTP://localhost:8080",
	} {
		t.Run("reject "+value, func(t *testing.T) {
			validEnvironment(t)
			path := writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o600)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", value)
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", path)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PLAYER_CLAIMS") {
				t.Fatalf("Load() error = %v", err)
			}
		})
	}
}

func TestLoadValidatesPlayerClaimsSecretFile(t *testing.T) {
	validPath := func(t *testing.T) string {
		return writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o600)
	}
	tests := []struct {
		name string
		path func(*testing.T) string
		want string
	}{
		{name: "missing", path: func(*testing.T) string { return "" }, want: "required"},
		{name: "relative", path: func(*testing.T) string { return "secret" }, want: "absolute"},
		{name: "not found", path: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, want: "inspected"},
		{name: "directory", path: func(t *testing.T) string { return t.TempDir() }, want: "regular non-symlink"},
		{name: "group readable", path: func(t *testing.T) string { return writeClaimSecret(t, bytes.Repeat([]byte{1}, 32), 0o640) }, want: "group or other"},
		{name: "wrong length", path: func(t *testing.T) string { return writeClaimSecret(t, []byte("do-not-leak-this-secret"), 0o600) }, want: "exactly 32"},
		{name: "invalid hex", path: func(t *testing.T) string { return writeClaimSecret(t, bytes.Repeat([]byte("z"), 64), 0o600) }, want: "hexadecimal"},
		{name: "multiple trailing newlines", path: func(t *testing.T) string {
			return writeClaimSecret(t, append(bytes.Repeat([]byte("a"), 64), '\n', '\n'), 0o600)
		}, want: "exactly 32"},
		{name: "symlink", path: func(t *testing.T) string {
			target := validPath(t)
			link := filepath.Join(t.TempDir(), "secret-link")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("cannot create symlink: %v", err)
			}
			return link
		}, want: "regular non-symlink"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validEnvironment(t)
			t.Setenv("SAVE_DATA_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ENABLED", "true")
			t.Setenv("PLAYER_CLAIMS_ORIGIN", "https://map.example.test")
			t.Setenv("PLAYER_CLAIMS_SECRET_FILE", test.path(t))
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.want)
			}
			if err != nil && strings.Contains(err.Error(), "do-not-leak-this-secret") {
				t.Fatalf("Load() leaked secret in error: %v", err)
			}
		})
	}
}

func writeClaimSecret(t *testing.T, contents []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claim-secret")
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	return path
}
