# Development and implementation

This document covers the technical details intentionally kept out of the main README. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request expectations.

## How it works

The Go service connects to one Palworld dedicated server and polls its official REST API. Browsers only talk to the live-map service; they never connect directly to Palworld or receive the REST admin password.

The service uses three upstream endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) provides the configured server name, description, and game version.
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) provides online player positions.
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) provides the optional world-object layers.

Only fields needed by the UI are exposed publicly. Account names, player IDs, user IDs, IP addresses, the world GUID, and REST credentials are not represented in the browser-facing models.

Player data is refreshed using `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; server information is refreshed once per minute. A single set of background pollers serves every connected browser. The last successful player, world-object, and server-information results are retained when Palworld is temporarily unavailable, with stale data identified in the UI.

The game-data endpoint represents currently loaded characters and Palboxes. It does not contain every placed wall, chest, crafting station, or other building part. Supporting those would require a separate save-file parser.

## Project layout

- `cmd/palworld-live-map` starts the pollers and HTTP server.
- `internal/config` validates environment configuration.
- `internal/palworld` contains the REST client, data sanitisation, classification, and polling state.
- `internal/server` exposes the read-only JSON and static-asset routes.
- `web` contains the dependency-free browser application embedded into the binary.
- `assets/map` contains the embedded Palpagos and World Tree artwork.

The browser application uses a fixed 1000×1000 scene and converts Palworld world coordinates into that space using the bounds in `internal/server/server.go`. See [the map artwork notes](assets/map/README.md) for source and ownership details.

## Run from source

Go 1.26 or newer is required.

```bash
cp .env.example .env
# For a Palworld API on the local machine, use:
# PALWORLD_REST_URL=http://127.0.0.1:8212
set -a; source .env; set +a
go run ./cmd/palworld-live-map
```

Open `http://localhost:8080`.

## Verification

```bash
make check  # formatting, vet, and race-enabled tests
make image  # local container image
```

The CI workflow repeats formatting, vet, tests, the Go build, and the container build for pull requests and pushes to `main`.

## Container publishing

Pushing `main` publishes `ghcr.io/lukehollanddev/palworld-live-map:latest`, `:main`, and a commit tag. Images target amd64 and arm64 and include provenance and an SBOM.

Version tags follow semantic versioning. Before 1.0, releases increment the minor component: `v0.1.0` publishes `:0.1.0` and `:0.1`, followed by `v0.2.0`. From `v1.0.0`, releases publish full and major/minor tags such as `:1.0.0` and `:1.0`. The `latest` tag tracks the newest build from `main`.

Publishing uses the repository's built-in `GITHUB_TOKEN`. Set the GHCR package to public if anonymous pulls should be supported.
