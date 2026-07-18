# Development and implementation

This document covers the technical details intentionally kept out of the main README. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request expectations.

## How it works

The Go service connects to one Palworld dedicated server and polls its official REST API. Browsers only talk to the live-map service; they never connect directly to Palworld or receive the REST admin password.

The service uses four upstream endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) provides the configured server name, description, and game version.
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) provides online player positions.
- [`/v1/api/metrics`](https://docs.palworldgame.com/api/rest-api/metrics/) provides player capacity, performance, uptime, base-count, and in-game day metrics.
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) provides the optional world-object layers.

Only fields needed by the UI are exposed publicly. Account names, player IDs, user IDs, IP addresses, raw guild IDs, the world GUID, and REST credentials are not represented in the browser-facing models. Bases receive an opaque one-way guild grouping key so the UI can group Palboxes owned by the same guild without publishing its upstream identifier.

Player and server-metric data are refreshed using `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; server information is refreshed once per minute. A single set of background pollers serves every connected browser. Browsers fetch `/api/players` and `/api/objects` independently at those same respective intervals, so unchanged world-object data is not retransmitted with every player update. The combined `/api/state` route remains available for compatibility. The last successful player, metric, world-object, and server-information results are retained when Palworld is temporarily unavailable; the UI exposes refresh state and timestamps.

When `DEMO_MODE=true`, the application does not construct the REST client or contact a Palworld server. A deterministic fictional source implements the same internal interface, so the regular poller, snapshots, public API, and browser UI are all exercised. Demo mode is suitable for screenshots, smoke tests, and public evaluation—not load or upstream-compatibility testing.

## Run from source

Go 1.26 or newer is required.

```bash
cp .env.example .env
# Edit .env with your Palworld REST API URL and admin password.
make run
```

Open `http://localhost:8080`.

For a local demo without Palworld:

```bash
make demo
```

To regenerate the map artwork from a local Palworld installation, follow [`tools/maps/README.md`](tools/maps/README.md). The Dockerised exporter is intentionally outside the production image and Go dependency graph.

## Verification

```bash
make check  # formatting, vet, and race-enabled tests
make image  # local container image
```

The CI workflow repeats formatting, vet, tests, the Go build, and the container build for pull requests and pushes to `main`.
