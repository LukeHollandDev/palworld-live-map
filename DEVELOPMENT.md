# Development and Implementation

This document covers the technical details intentionally kept out of the main README. See [CONTRIBUTING.md](CONTRIBUTING.md) for commit and pull-request expectations.

## How It Works

The Go service connects to one Palworld dedicated server and polls its official REST API. Browsers only talk to the live-map service; they never connect directly to Palworld or receive the REST admin password.

The service uses four upstream endpoints:

- [`/v1/api/info`](https://docs.palworldgame.com/api/rest-api/info/) provides the configured server name, description, and game version.
- [`/v1/api/players`](https://docs.palworldgame.com/api/rest-api/players/) provides online player positions.
- [`/v1/api/metrics`](https://docs.palworldgame.com/api/rest-api/metrics/) provides player capacity, performance, uptime, base-count, and in-game day metrics.
- [`/v1/api/game-data`](https://docs.palworldgame.com/api/rest-api/game-data/) provides the optional world-object layers.

Only fields needed by the UI are exposed publicly. Account names, raw player IDs, user IDs, IP addresses, raw guild IDs, the world GUID, and REST credentials are not represented in the browser-facing models. Players and world objects receive credential-keyed opaque IDs for stable UI state; bases receive an opaque guild grouping key so the UI can group Palboxes owned by the same guild without publishing upstream identifiers.

Player and server-metric data are refreshed using `POLL_INTERVAL`; world objects use `WORLD_POLL_INTERVAL`; server information is refreshed once per minute. A single set of background pollers serves every connected browser. Browsers fetch `/api/players` and `/api/objects` independently at those same respective intervals, so unchanged world-object data is not retransmitted with every player update. The combined `/api/state` route remains available for compatibility, but mirrors the current models rather than preserving undocumented fields such as the former `averageFps`. The last successful player, metric, and world-object results are retained when Palworld is temporarily unavailable, and the UI exposes their refresh state and timestamps. A failed server-information refresh is logged while the previous name and description remain visible.

When current metrics are available, the status strip shows data freshness, current/max players, current server FPS, frame time, uptime, base count, and in-game day. Stale metrics are withheld rather than presented as current. Some servers return an average-FPS extension, but the application deliberately omits that near-duplicate value from its public model.

World records without coordinates inside a shipped map are omitted, as are legacy moving actors that provide no stable instance or owner identity. A `BaseCampPal` is assigned to a base only when it lies within the standard 3,500-unit Palbox radius plus 2.5% tolerance on the same map and in the same guild; overlapping perimeters choose the nearest base with a stable tie-break. Companion and wild Pals are never assigned by proximity. Public world snapshots are capped at 20,000 objects; truncation is explicit, and retention prioritizes bases, workers, companions, NPCs, then wild Pals. Dense browser views cull offscreen markers and render clusters rather than tens of thousands of individual DOM controls.

### Membership and ownership projection

The live endpoints are joined internally before their identifiers are discarded. An online `/players` record is matched to its game-data player actor through unique user IDs and the player-ID/actor-instance prefix. That actor supplies the guild and full instance identity needed to connect an `OtomoPal.TrainerInstanceID` to its owner. The public snapshot contains only deployment-scoped opaque IDs and the display fields needed by the UI.

The Explorer uses two independent hierarchies on the selected map:

- **Players → companion Pals:** an exact opaque owner-ID match nests a travelling companion below its online player. Companions whose online owner is on another map, and companions with no currently online owner, remain in explicit fallback groups.
- **Guilds → bases → assigned Pals:** Palboxes group by opaque guild key. A `BaseCampPal` is nested below a base only after the same-guild, same-map perimeter test above. Same-guild workers outside every qualifying perimeter remain under that guild's "Outside base perimeters" group; records without a linked guild use a separate fallback group.

The hierarchy controls affect the complete group even when search and rendering caps show only part of it. Companion and wild Pals are never captured by base proximity, and ambient guild-like values on wild Pals or NPCs are not treated as player-guild membership.

When `DEMO_MODE=true`, the application does not construct the REST client or contact a Palworld server. A deterministic fictional source implements the same internal interface, so the regular poller, snapshots, public API, and browser UI are all exercised. Its workers stay inside their assigned base perimeters, companion Pals follow their owners, and both maps contain a complete player-guild-base example. Demo mode is suitable for screenshots, smoke tests, and public evaluation—not load or upstream-compatibility testing.

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

To regenerate the map artwork from a local Palworld installation, follow [`tools/map-exporter/README.md`](tools/map-exporter/README.md). The Dockerised exporter is intentionally outside the production image and Go dependency graph.

## Optional save-data direction

Save-data support is not implemented. The live REST snapshot cannot provide offline guild rosters, stored or party Pals, exact Pal-container membership, or a modded base radius. Those gaps should not be filled by guesswork.

If save inspection is added, it should be optional and disabled by default. A conservative design is a separate reader or sidecar that:

- receives a read-only save mount and parses a stable copy on a slow interval;
- retains its last valid result and leaves the REST-only application usable when absent or stale;
- emits a versioned, allowlisted JSON snapshot rather than raw save data; and
- replaces platform, actor, guild, container, and player identifiers with deployment-scoped opaque IDs before publication.

Modern Palworld 1.0 `PlM` saves require an Oodle decoder supplied by the operator, which is another reason to keep parsing outside the static Go runtime. `GroupSaveDataMap` can supply guild rosters, `CharacterSaveParameterMap` can supply player and Pal ownership, and `BaseCampSaveData` can supply the exact worker-container link. Raw save JSON, filesystem paths, inventories, precise last-online timestamps, and parser errors containing private values must remain private.

Local API captures, save experiments, and operator-specific tunnel notes belong under the gitignored `.local/` directory. Do not commit them or link public documentation to files that other checkouts will not contain.

## Verification

```bash
make test            # Vitest and Go unit tests
make check           # frontend checks/build, Go formatting, vet, and race-enabled tests
make build           # frontend assets and local Go binary
make image           # local production container image
make exporter-check  # compile the maintainer-only map exporter in Docker
```

Use `make clean` to remove the application/exporter build outputs and their standard coverage paths, or `make distclean` to also remove downloaded frontend dependencies and the ignored `build/` workspace. Neither target removes `.env` or `.local/`.

CI runs all frontend and Go checks and compiles the map exporter before it builds the production container. Pull requests build a native validation image; pushes to `main` and version tags publish the multi-platform image only after the checks pass. Manually dispatched runs perform checks without publishing.
