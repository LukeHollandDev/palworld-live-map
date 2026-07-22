# Development and Implementation

This document covers the technical details intentionally kept out of the main README. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request expectations.

## How It Works

The Go service connects to one Palworld dedicated server and polls its official REST API. It can also read completed native save backups through a separate read-only adapter. Browsers only talk to the live-map service; they never connect directly to Palworld, receive the REST admin password, or receive raw save identifiers.

The service uses four upstream endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) provides the configured server name, description, and game version.
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) provides online player positions.
- [`/v1/api/metrics`](https://docs.palworldgame.com/api/rest-api/metrics/) provides player capacity, performance, uptime, base-count, and in-game day metrics.
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) provides the optional world-object layers.

Only fields needed by the UI are exposed publicly. Account names, raw player IDs, user IDs, IP addresses, raw guild IDs, the world GUID, and REST credentials are not represented in the browser-facing models. REST and save records use the same credential-keyed opaque player/guild projections, allowing an online record to overlay its persistent save record without publishing upstream identifiers. World objects receive equivalent stable opaque IDs.

Player and server-metric data are refreshed using `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; the save roster uses `SAVE_POLL_INTERVAL`; server information is refreshed once per minute. A single set of background pollers serves every connected browser. Browsers fetch `/api/players` and `/api/objects` independently at their respective intervals, so unchanged world-object data is not retransmitted with every player update. The combined `/api/state` route remains available for compatibility. The last successful player, metric, world-object, and save-roster results are retained independently, with explicit availability/staleness timestamps in the public state.

The save adapter selects the second-newest complete `backup/world` generation when at least two exist, or the only complete generation otherwise, and then runs a bounded selective decoder. It currently extracts roster identity, level, guild membership/name, `LastTransform.Translation`, `LastOnlineDateTime`, and typed `RecordData` capture/Paldeck aggregates. The data-source contract and extension path are detailed in [`docs/save-data.md`](docs/save-data.md).

Field Alpha and tower-boss locations are versioned static game data under `assets/landmarks`, not live server actors. The checked-in manifest is generated directly from an installed Palworld PAK, including exact tower actor coordinates and source provenance. The locations are returned with public configuration and share the normal `WorldObject` projection/filter pipeline.

When current metrics are available, the status strip shows data freshness, current/max players, current server FPS, uptime, base count, and in-game day. Stale metrics are withheld rather than presented as current, and the UI deliberately does not display the more easily misread frame-time metric. Some servers return an average-FPS extension, but the application omits that near-duplicate value from its public model.

World records without coordinates inside a shipped map are omitted, as are legacy moving actors that provide no stable instance or owner identity. A `BaseCampPal` is assigned to a base only when it lies within the standard 3,500-unit Palbox radius plus 2.5% tolerance on the same map and in the same guild; overlapping perimeters choose the nearest base with a stable tie-break. Companion and wild Pals are never assigned by proximity. Public world snapshots are capped at 20,000 objects; truncation is explicit, and retention prioritizes bases, workers, companions, NPCs, then wild Pals. Dense browser views cull offscreen markers and render clusters rather than tens of thousands of individual DOM controls.

### Membership and ownership projection

The live endpoints and persistent roster are joined internally before their identifiers are discarded. An online `/players` record is matched to the saved roster by normalized player GUID; its current level and coordinates win, while saved guild metadata fills REST gaps. The game-data player actor still supplies the full instance identity needed to connect an `OtomoPal.TrainerInstanceID` to its owner. The public snapshot contains only deployment-scoped opaque IDs and display fields.

The Explorer is intentionally limited to controls for the selected map. Online Players, Offline Players, and Guilds are the only expanded and enabled categories for a fresh or migrated default. Search is hosted at the top of the Explorer and feeds the shared list/map query; selecting a result follows the normal focus path, enabling its kind or player status and smoothly moving to its coordinates before opening details.

- **Online Players and Offline Players:** separate categories use the same green/gray status treatment as their markers. Offline entries use the last position available from the completed save snapshot.
- **Companion Pals:** a separate flat map-object category. Exact opaque owner-ID relationships remain available from the companion and player detail panels.
- **Guilds → bases → assigned Pals:** Palboxes group by opaque guild key. A `BaseCampPal` is nested below a base only after the same-guild, same-map perimeter test above. Same-guild workers outside every qualifying perimeter remain under that guild's "Outside base perimeters" group; records without a linked guild use a separate fallback group.
- **Wild Pals, Field Alphas, Tower Bosses, and NPCs:** independent map categories that remain collapsed and disabled by default.

The hierarchy controls affect the complete group even when search and rendering caps show only part of it. Guild-member rosters are deliberately available from guild details rather than the map filter. Companion and wild Pals are never captured by base proximity, and ambient guild-like values on wild Pals or NPCs are not treated as player-guild membership.

When `DEMO_MODE=true`, the application does not construct the REST client or contact a Palworld server. A deterministic fictional source implements both live and roster interfaces, including one offline member, so online/offline merging, the leaderboard, public API, and browser UI are exercised. Its workers stay inside their assigned base perimeters, companion Pals follow their owners, and both maps contain a complete player-guild-base example. Demo mode is suitable for screenshots, smoke tests, and public evaluation—not load or upstream-compatibility testing.

## Frontend

The browser application is a self-contained project in [`web/`](web). It uses React and TypeScript, Vite for development and production builds, Tailwind CSS for styling, Biome for formatting and linting, and Vitest for unit and component tests.

## Run from Source

Go 1.26.5 or newer and Node.js 24 or newer are required.

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

### Develop against a remote server through SSH

Do not expose the Palworld REST port publicly. When the API is reachable only from a remote machine, keep a loopback-only tunnel open in one terminal:

```bash
ssh -N -L 127.0.0.1:8212:127.0.0.1:8212 user@palworld-host
```

The tunnel destination is evaluated on `palworld-host`; change it to the hostname or container address that machine uses for the REST API when necessary. For a source run, set `PALWORLD_REST_URL=http://127.0.0.1:8212` in `.env` and run `make run`.

Docker Desktop can normally reach the host tunnel through `http://host.docker.internal:8212`. On native Linux, the `host-gateway` entry in [`compose.yml`](compose.yml) does not make a loopback-only listener reachable from a container. Prefer the source run or place the SSH client in a private container network; do not bind the REST tunnel to `0.0.0.0` merely to make it reachable.

For frontend hot module replacement, run the Go demo server and Vite in separate terminals:

```bash
make web-assets # required once after a clean checkout or make clean
DEMO_MODE=true go run ./cmd/palworld-live-map
```

```bash
cd web
npm ci
npm run dev
```

Open `http://localhost:5173`. Vite proxies the API and map artwork requests to the Go service on port 8080.

To regenerate the map artwork and encounter catalogue from a local Palworld installation, follow [`tools/map-exporter/README.md`](tools/map-exporter/README.md) or run `make game-assets`. The Dockerised exporter is intentionally outside the production image and Go dependency graph.

## Verification

```bash
make test            # Vitest and Go unit tests
make check           # frontend checks/build, Go formatting, vet, and race-enabled tests
make build           # frontend assets and local Go binary
make image           # local production container image
make exporter-check  # run exporter fixtures and compile it in pinned Docker tooling
make game-assets     # export map artwork and encounter data from an installed game
```

Use `make clean` to remove the application/exporter build outputs and their standard coverage paths, or `make distclean` to also remove downloaded frontend dependencies and the ignored `build/` workspace. Neither target removes `.env` or `.local/`.

CI runs all frontend and Go checks, the exporter's fixture suite, and its release compile before it builds the production container. Pull requests build a native validation image; pushes to `main` and version tags publish the multi-platform image only after the checks pass. Manually dispatched runs perform checks without publishing.
