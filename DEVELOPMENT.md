# Development and Implementation

This document covers the technical details intentionally kept out of the main README. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request expectations.

## How It Works

The Go service connects to one Palworld dedicated server and polls its official REST API. Browsers only talk to the live-map service; they never connect directly to Palworld or receive the REST admin password.

The service uses four upstream endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) provides the configured server name, description, and game version.
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) provides online player positions.
- [`/v1/api/metrics`](https://docs.palworldgame.com/api/rest-api/metrics/) provides player capacity, performance, uptime, base-count, and in-game day metrics.
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) provides the optional world-object layers.

Only fields needed by the UI are exposed publicly. Account names, player IDs, user IDs, IP addresses, raw guild IDs, the world GUID, and REST credentials are not represented in the browser-facing models. Bases receive an opaque one-way guild grouping key so the UI can group Palboxes owned by the same guild without publishing its upstream identifier.

Player and server-metric data are refreshed using `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; server information is refreshed once per minute. A single set of background pollers serves every connected browser. Browsers fetch `/api/players` and `/api/objects` independently at those same respective intervals, so unchanged world-object data is not retransmitted with every player update. The combined `/api/state` route remains available for compatibility. The last successful player, metric, world-object, and server-information results are retained when Palworld is temporarily unavailable; the UI exposes refresh state and timestamps.

When `DEMO_MODE=true`, the application does not construct the REST client or contact a Palworld server. A deterministic fictional source implements the same internal interface, so the regular poller, snapshots, public API, and browser UI are all exercised. Demo mode is suitable for screenshots, smoke tests, and public evaluation—not load or upstream-compatibility testing.

## Frontend

The browser application is a self-contained project in [`web/`](web). It uses React and TypeScript, Vite for development and production builds, Tailwind CSS for styling, Biome for formatting and linting, and Vitest for unit and component tests.

## Run from Source

Go 1.26 or newer and Node.js 24 or newer are required.

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

For frontend hot module replacement, run the Go demo server and Vite in separate terminals:

```bash
DEMO_MODE=true go run ./cmd/palworld-live-map
```

```bash
cd web
npm ci
npm run dev
```

Open `http://localhost:5173`. Vite proxies the API and map artwork requests to the Go service on port 8080.

To regenerate the map artwork from a local Palworld installation, follow [`tools/map-exporter/README.md`](tools/map-exporter/README.md). The Dockerised exporter is intentionally outside the production image and Go dependency graph.

## Verification

```bash
make check  # Biome, TypeScript, Vitest, generated assets, Go formatting, vet, and race-enabled tests
make image  # local production container image
```

The CI workflow repeats all frontend and Go checks, the Go build, and the container build for pull requests and pushes to `main`.
