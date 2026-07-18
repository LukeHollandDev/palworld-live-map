package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LukeHollandDev/palworld-live-map/internal/config"
	"github.com/LukeHollandDev/palworld-live-map/internal/palworld"
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
	if cfg.DemoMode {
		source = palworld.NewDemoSource()
		logger.Info("demo mode enabled; no Palworld server will be contacted")
	} else {
		client, clientErr := palworld.NewClient(cfg.RESTURL, cfg.AdminPassword, cfg.UpstreamTimeout, cfg.WorldTimeout)
		if clientErr != nil {
			logger.Error("Palworld client configuration error", "error", clientErr)
			os.Exit(1)
		}
		source = client
	}
	poller := palworld.NewPoller(source, cfg.PollInterval, cfg.WorldPollInterval, cfg.WorldDataEnabled, logger)
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
	endpoint := os.Getenv("HEALTHCHECK_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:8080/-/health"
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
