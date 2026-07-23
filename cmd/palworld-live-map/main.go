package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
	"github.com/LukeHollandDev/palworld-live-map/internal/savegame"
	"github.com/LukeHollandDev/palworld-live-map/internal/saveroster"
	webserver "github.com/LukeHollandDev/palworld-live-map/internal/server"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the running service and exit")
	flag.Parse()
	if *healthcheck {
		if err := checkHealth(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}

	var source palworld.Source
	var roster palworld.RosterSource
	if cfg.DemoMode {
		demo := palworld.NewDemoSource()
		source = demo
		roster = demo
		logger.Info("demo mode enabled; no Palworld server will be contacted")
	} else {
		client, clientErr := palworld.NewClient(cfg.RESTURL, cfg.AdminPassword, cfg.UpstreamTimeout, cfg.WorldTimeout)
		if clientErr != nil {
			logger.Error("Palworld client configuration error", "error", clientErr)
			os.Exit(1)
		}
		source = client
		if cfg.SaveDataEnabled {
			reader, readerErr := savegame.NewReader(savegame.Options{})
			if readerErr != nil {
				logger.Error("save reader setup failed", "error", readerErr)
				os.Exit(1)
			}
			rosterSource, rosterErr := saveroster.New(saveroster.Options{
				Root: cfg.SaveRoot, WorldID: cfg.SaveWorldID, Timeout: cfg.SaveTimeout, Reader: reader,
				ProjectPlayerID: client.PublicPlayerID, ProjectGuildID: client.PublicGuildKey,
			})
			if rosterErr != nil {
				logger.Error("save roster setup failed", "error", rosterErr)
				os.Exit(1)
			}
			roster = rosterSource
			logger.Info("save-backed player roster enabled", "pollInterval", cfg.SavePollInterval)
		}
	}
	poller := palworld.NewPollerWithRoster(
		source, roster, cfg.PollInterval, cfg.WorldPollInterval, cfg.SavePollInterval, cfg.WorldDataEnabled, logger,
	)
	app, err := webserver.New(cfg, poller)
	if err != nil {
		logger.Error("web server setup failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go poller.Run(ctx)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Palworld live map listening", "addr", cfg.Addr, "pollInterval", cfg.PollInterval)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", "error", err)
		os.Exit(1)
	}
}

func checkHealth() error {
	endpoint, err := healthcheckEndpoint(os.Getenv("ADDR"))
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("healthcheck request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck returned %s", resp.Status)
	}
	return nil
}

func healthcheckEndpoint(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("derive healthcheck URL from ADDR %q: expected host:port", addr)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/-/health"}).String(), nil
}
