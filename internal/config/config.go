package config

import (
	"errors"
	"fmt"
	"os"
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
