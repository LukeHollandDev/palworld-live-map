# Development

This guide covers the project architecture and local workflow. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request guidelines.

## Architecture

The Go service polls one Palworld dedicated server and exposes only the data needed by the UI. Browsers never connect directly to Palworld or receive the REST admin password.

It uses four official REST endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) for server metadata
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) for online players
- [`/v1/api/metrics`](https://docs.palworldgame.com/api/rest-api/metrics/) for server health and capacity
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) for optional world objects

Credentials, network details, and upstream identifiers are excluded from public models. Players, guilds, and world objects use stable opaque IDs.

Player and metric data use `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; server metadata refreshes once per minute. Results are cached independently so an upstream failure does not discard the last successful snapshot.

Optional save enrichment invokes the separately licensed `savedecode` binary
with its exact versioned `player-details` preset for each player file in the
selected immutable backup generation. The app performs bounded aggregation and
joins those details to REST-visible players without importing decoder packages.

Field Alpha and tower-boss locations are versioned data under [`assets/palworld`](assets/palworld). The frontend lives in [`web`](web) and uses React, TypeScript, Vite, Tailwind CSS, Biome, and Vitest.

`DEMO_MODE=true` uses deterministic fictional data without contacting a Palworld server. It is useful for development, screenshots, and smoke tests.

## Run Locally

Install Go 1.26.5 or newer, Node.js 24 or newer, and GNU Make. Production deployments should use the container described in the main README.

```bash
cp .env.example .env
# Add your Palworld REST API URL and admin password to .env.
make run
```

Open <http://localhost:8080>.

To exercise save enrichment from a sibling `palworld-save-reader` checkout,
build its unchanged source into this repository's ignored local workspace:

```bash
(cd ../palworld-save-reader && go build -o ../palworld-live-map/.local/bin/savedecode ./cmd/savedecode)
```

Then set `SAVE_DATA_ENABLED=true`, `PALWORLD_SAVE_ROOT`,
`PALWORLD_SAVE_DECODER` (to the absolute `.local/bin/savedecode` path), and the
matching `PALWORLD_SAVE_GAME_VERSION` in `.env`.

To run without a Palworld server:

```bash
make demo
```

### Frontend hot reload

Run the demo server and Vite in separate terminals:

```bash
make demo
```

```bash
npm --prefix web run dev
```

Open <http://localhost:5173>. Vite proxies API and map requests to the Go service on port 8080.

### Remote server over SSH

Do not expose the Palworld REST port publicly. Instead, open a loopback-only tunnel:

```bash
ssh -N -L 127.0.0.1:8212:127.0.0.1:8212 user@palworld-host
```

Set `PALWORLD_REST_URL=http://127.0.0.1:8212` in `.env`, then run `make run`. Change the tunnel destination if the API uses a different hostname or container address on the remote machine.

Docker Desktop can usually reach the tunnel at `http://host.docker.internal:8212`. Native Linux containers cannot reach a loopback-only host listener through `host-gateway`; use a source run or a private container network instead.

### Regenerate game assets

Run `make game-assets` to regenerate map artwork and encounter data from a local Palworld installation. See the [asset exporter guide](exporter/README.md) for requirements and details.

## Verify Changes

| Command | Purpose |
| --- | --- |
| `make test` | Run frontend and Go tests |
| `make check` | Run frontend checks/build, Go formatting, vet, and race-enabled tests |
| `make build` | Build frontend assets and the local Go binary |
| `make image` | Build the production container image locally |
| `make exporter-check` | Test and compile the asset exporter |

Use `make clean` to remove build and coverage outputs. Use `make distclean` to also remove frontend dependencies and the ignored `build/` workspace. Neither removes `.env` or `.local/`.

CI runs the frontend, Go, exporter, and container checks before publishing images.
