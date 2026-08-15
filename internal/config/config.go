package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Config struct {
	Addr                       string
	RESTURL                    string
	AdminPassword              string
	DemoMode                   bool
	PollInterval               time.Duration
	UpstreamTimeout            time.Duration
	WorldPollInterval          time.Duration
	WorldTimeout               time.Duration
	WorldDataEnabled           bool
	SaveDataEnabled            bool
	SaveRoot                   string
	SaveWorldID                string
	SavePollInterval           time.Duration
	SaveTimeout                time.Duration
	PlayerClaimsEnabled        bool
	PlayerClaimsOrigin         string
	PlayerClaimsInsecureLocal  bool
	PlayerClaimsSecret         [32]byte
	PlayerClaimsTrustedProxies []netip.Prefix
}

func Load() (Config, error) {
	demoMode, err := boolean("DEMO_MODE", false)
	if err != nil {
		return Config{}, err
	}
	pollInterval, err := duration("POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	upstreamTimeout, err := duration("UPSTREAM_TIMEOUT", 4*time.Second)
	if err != nil {
		return Config{}, err
	}
	worldPollInterval, err := duration("WORLD_POLL_INTERVAL", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	worldTimeout, err := duration("WORLD_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	worldDataEnabled, err := boolean("WORLD_DATA_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	saveDataEnabled, err := boolean("SAVE_DATA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	savePollInterval, err := duration("SAVE_POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	saveTimeout, err := duration("SAVE_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	playerClaimsEnabled, err := boolean("PLAYER_CLAIMS_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	playerClaimsInsecureLocal, err := boolean("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK", false)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:                      envOr("ADDR", ":8080"),
		RESTURL:                   strings.TrimRight(os.Getenv("PALWORLD_REST_URL"), "/"),
		AdminPassword:             os.Getenv("PALWORLD_ADMIN_PASSWORD"),
		DemoMode:                  demoMode,
		PollInterval:              pollInterval,
		UpstreamTimeout:           upstreamTimeout,
		WorldPollInterval:         worldPollInterval,
		WorldTimeout:              worldTimeout,
		WorldDataEnabled:          worldDataEnabled,
		SaveDataEnabled:           saveDataEnabled,
		SaveRoot:                  envOr("PALWORLD_SAVE_ROOT", "/data/palworld/saves"),
		SaveWorldID:               strings.TrimSpace(os.Getenv("PALWORLD_SAVE_WORLD_ID")),
		SavePollInterval:          savePollInterval,
		SaveTimeout:               saveTimeout,
		PlayerClaimsEnabled:       playerClaimsEnabled,
		PlayerClaimsInsecureLocal: playerClaimsInsecureLocal,
	}

	var missing []string
	if !cfg.DemoMode {
		if cfg.RESTURL == "" {
			missing = append(missing, "PALWORLD_REST_URL")
		}
		if cfg.AdminPassword == "" {
			missing = append(missing, "PALWORLD_ADMIN_PASSWORD")
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing configuration: %s", strings.Join(missing, ", "))
	}
	if cfg.PollInterval < 2*time.Second {
		return Config{}, errors.New("POLL_INTERVAL must be at least 2s")
	}
	if cfg.UpstreamTimeout <= 0 || cfg.UpstreamTimeout >= cfg.PollInterval {
		return Config{}, errors.New("UPSTREAM_TIMEOUT must be positive and shorter than POLL_INTERVAL")
	}
	if cfg.WorldDataEnabled {
		if cfg.WorldPollInterval < 5*time.Second {
			return Config{}, errors.New("WORLD_POLL_INTERVAL must be at least 5s")
		}
		if cfg.WorldTimeout <= 0 || cfg.WorldTimeout >= cfg.WorldPollInterval {
			return Config{}, errors.New("WORLD_TIMEOUT must be positive and shorter than WORLD_POLL_INTERVAL")
		}
	}
	if cfg.DemoMode && cfg.SaveDataEnabled {
		return Config{}, errors.New("SAVE_DATA_ENABLED cannot be used with DEMO_MODE")
	}
	if cfg.SaveDataEnabled {
		if !filepath.IsAbs(cfg.SaveRoot) {
			return Config{}, errors.New("PALWORLD_SAVE_ROOT must be an absolute path")
		}
		if cfg.SavePollInterval < 15*time.Second {
			return Config{}, errors.New("SAVE_POLL_INTERVAL must be at least 15s")
		}
		if cfg.SaveTimeout <= 0 || cfg.SaveTimeout >= cfg.SavePollInterval {
			return Config{}, errors.New("SAVE_TIMEOUT must be positive and shorter than SAVE_POLL_INTERVAL")
		}
	}
	if cfg.PlayerClaimsEnabled {
		if cfg.DemoMode {
			return Config{}, errors.New("PLAYER_CLAIMS_ENABLED cannot be used with DEMO_MODE")
		}
		if !cfg.SaveDataEnabled {
			return Config{}, errors.New("PLAYER_CLAIMS_ENABLED requires SAVE_DATA_ENABLED")
		}

		origin, err := claimsOriginForMode(os.Getenv("PLAYER_CLAIMS_ORIGIN"), cfg.PlayerClaimsInsecureLocal)
		if err != nil {
			return Config{}, err
		}
		secret, err := claimsSecret(os.Getenv("PLAYER_CLAIMS_SECRET_FILE"))
		if err != nil {
			return Config{}, err
		}
		cfg.PlayerClaimsOrigin = origin
		cfg.PlayerClaimsSecret = secret
		trustedProxies, err := claimsTrustedProxies(os.Getenv("PLAYER_CLAIMS_TRUSTED_PROXIES"))
		if err != nil {
			return Config{}, err
		}
		cfg.PlayerClaimsTrustedProxies = trustedProxies
	}
	return cfg, nil
}

func claimsTrustedProxies(value string) ([]netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	if len(parts) > 32 {
		return nil, errors.New("PLAYER_CLAIMS_TRUSTED_PROXIES contains too many networks")
	}
	result := make([]netip.Prefix, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil || !prefix.IsValid() {
			return nil, errors.New("PLAYER_CLAIMS_TRUSTED_PROXIES must be a comma-separated list of IP CIDR networks")
		}
		prefix = prefix.Masked()
		if _, duplicate := seen[prefix]; duplicate {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix)
	}
	return result, nil
}

func claimsOrigin(value string) (string, error) {
	return claimsOriginForMode(value, false)
}

func claimsOriginForMode(value string, allowInsecureLoopback bool) (string, error) {
	if value == "" {
		return "", errors.New("PLAYER_CLAIMS_ORIGIN is required when player claims are enabled")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
	}
	if parsed.Scheme == "http" && !allowInsecureLoopback {
		return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
	}

	hostname := parsed.Hostname()
	canonicalHost := ""
	if address, addressErr := netip.ParseAddr(hostname); addressErr == nil {
		if address.Zone() != "" {
			return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
		}
		if address.Is6() && strings.Contains(hostname, ".") {
			return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
		}
		canonicalHost = address.String()
		if address.Is6() {
			canonicalHost = "[" + canonicalHost + "]"
		}
		if parsed.Scheme == "http" && !address.IsLoopback() {
			return "", errors.New("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK permits only an exact http loopback origin")
		}
	} else {
		for _, character := range hostname {
			if character > unicode.MaxASCII {
				return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
			}
		}
		if legacyIPv4Host(hostname) {
			return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
		}
		canonicalHost = strings.ToLower(hostname)
		if parsed.Scheme == "http" && canonicalHost != "localhost" {
			return "", errors.New("PLAYER_CLAIMS_ALLOW_INSECURE_LOOPBACK permits only an exact http loopback origin")
		}
	}

	defaultPort := uint64(443)
	if parsed.Scheme == "http" {
		defaultPort = 80
	}
	if port := parsed.Port(); port != "" {
		portNumber, portErr := strconv.ParseUint(port, 10, 16)
		if portErr != nil {
			return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
		}
		if portNumber != defaultPort {
			canonicalHost += ":" + strconv.FormatUint(portNumber, 10)
		}
	}
	canonical := (&url.URL{Scheme: parsed.Scheme, Host: canonicalHost}).String()
	if value != canonical {
		return "", errors.New("PLAYER_CLAIMS_ORIGIN must be an exact https origin with no path, query, user information, or fragment")
	}
	return value, nil
}

func legacyIPv4Host(hostname string) bool {
	numericHostname := strings.TrimSuffix(hostname, ".")
	if numericHostname == "" {
		return false
	}
	labels := strings.Split(numericHostname, ".")
	last := labels[len(labels)-1]
	if last == "" {
		return false
	}
	if strings.HasPrefix(last, "0x") || strings.HasPrefix(last, "0X") {
		for index := 2; index < len(last); index++ {
			character := last[index]
			if (character < '0' || character > '9') &&
				(character < 'a' || character > 'f') &&
				(character < 'A' || character > 'F') {
				return false
			}
		}
		return true
	}
	for index := range len(last) {
		if last[index] < '0' || last[index] > '9' {
			return false
		}
	}
	return true
}

func claimsSecret(value string) ([32]byte, error) {
	var secret [32]byte
	path := strings.TrimSpace(value)
	if path == "" {
		return secret, errors.New("PLAYER_CLAIMS_SECRET_FILE is required when player claims are enabled")
	}
	if !filepath.IsAbs(path) {
		return secret, errors.New("PLAYER_CLAIMS_SECRET_FILE must be an absolute path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return secret, fmt.Errorf("PLAYER_CLAIMS_SECRET_FILE cannot be inspected: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return secret, errors.New("PLAYER_CLAIMS_SECRET_FILE must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return secret, errors.New("PLAYER_CLAIMS_SECRET_FILE must not grant permissions to group or other users")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return secret, fmt.Errorf("PLAYER_CLAIMS_SECRET_FILE cannot be read: %w", err)
	}
	switch len(contents) {
	case len(secret):
		copy(secret[:], contents)
		return secret, nil
	}
	hexContents := contents
	if len(hexContents) > 0 && hexContents[len(hexContents)-1] == '\n' {
		hexContents = hexContents[:len(hexContents)-1]
		if len(hexContents) > 0 && hexContents[len(hexContents)-1] == '\r' {
			hexContents = hexContents[:len(hexContents)-1]
		}
	}
	if len(hexContents) == hex.EncodedLen(len(secret)) {
		decoded, err := hex.DecodeString(string(hexContents))
		if err == nil {
			copy(secret[:], decoded)
			return secret, nil
		}
	}
	return [32]byte{}, errors.New("PLAYER_CLAIMS_SECRET_FILE must contain exactly 32 raw bytes or 64 hexadecimal characters")
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return parsed, nil
}

func boolean(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false: %w", key, err)
	}
	return parsed, nil
}
