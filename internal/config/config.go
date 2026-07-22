package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr              string
	RESTURL           string
	AdminPassword     string
	DemoMode          bool
	PollInterval      time.Duration
	UpstreamTimeout   time.Duration
	WorldPollInterval time.Duration
	WorldTimeout      time.Duration
	WorldDataEnabled  bool
	SaveDataEnabled   bool
	SaveRoot          string
	SaveWorldID       string
	SavePollInterval  time.Duration
	SaveTimeout       time.Duration
	SaveOodleLibrary  string
	SaveOodleURL      string
	SaveOodleSHA256   string
	SaveOodleCacheDir string
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

	cfg := Config{
		Addr:              envOr("ADDR", ":8080"),
		RESTURL:           strings.TrimRight(os.Getenv("PALWORLD_REST_URL"), "/"),
		AdminPassword:     os.Getenv("PALWORLD_ADMIN_PASSWORD"),
		DemoMode:          demoMode,
		PollInterval:      pollInterval,
		UpstreamTimeout:   upstreamTimeout,
		WorldPollInterval: worldPollInterval,
		WorldTimeout:      worldTimeout,
		WorldDataEnabled:  worldDataEnabled,
		SaveDataEnabled:   saveDataEnabled,
		SaveRoot:          envOr("PALWORLD_SAVE_ROOT", "/data/palworld/saves"),
		SaveWorldID:       strings.TrimSpace(os.Getenv("PALWORLD_SAVE_WORLD_ID")),
		SavePollInterval:  savePollInterval,
		SaveTimeout:       saveTimeout,
		SaveOodleLibrary:  strings.TrimSpace(os.Getenv("SAVE_OODLE_LIBRARY")),
		SaveOodleURL:      strings.TrimSpace(os.Getenv("SAVE_OODLE_DOWNLOAD_URL")),
		SaveOodleSHA256:   strings.ToLower(strings.TrimSpace(os.Getenv("SAVE_OODLE_SHA256"))),
		SaveOodleCacheDir: envOr("SAVE_OODLE_CACHE_DIR", "/tmp/palworld-live-map/oodle"),
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
		if cfg.SaveOodleLibrary != "" && !filepath.IsAbs(cfg.SaveOodleLibrary) {
			return Config{}, errors.New("SAVE_OODLE_LIBRARY must be an absolute path")
		}
		if !filepath.IsAbs(cfg.SaveOodleCacheDir) {
			return Config{}, errors.New("SAVE_OODLE_CACHE_DIR must be an absolute path")
		}
		hasLibrary := cfg.SaveOodleLibrary != ""
		hasDownload := cfg.SaveOodleURL != "" || cfg.SaveOodleSHA256 != ""
		if hasLibrary == hasDownload {
			return Config{}, errors.New("save data requires exactly one Oodle source: SAVE_OODLE_LIBRARY, or SAVE_OODLE_DOWNLOAD_URL with SAVE_OODLE_SHA256")
		}
		if hasDownload {
			parsed, err := url.Parse(cfg.SaveOodleURL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				return Config{}, errors.New("SAVE_OODLE_DOWNLOAD_URL must be an absolute HTTPS URL without credentials")
			}
			if digest, err := hex.DecodeString(cfg.SaveOodleSHA256); err != nil || len(digest) != 32 {
				return Config{}, errors.New("SAVE_OODLE_SHA256 must be a 64-character hexadecimal digest")
			}
		}
	}
	return cfg, nil
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
