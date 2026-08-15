# Palworld Live Map

[![CI](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml/badge.svg)](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/container-GHCR-2496ed?logo=docker&logoColor=white)](https://github.com/LukeHollandDev/palworld-live-map/pkgs/container/palworld-live-map)
[![Core license: MIT](https://img.shields.io/badge/core-MIT-green.svg)](LICENSE)

Give your Palworld community a live view of players, guilds, bases, Pals, in-game locations, and server health. It runs against your dedicated server's official APIs, stays read-only, and needs no client mods.

![Animated Palworld Live Map demo showing filters, search, a guild base and its Pals, leaderboards, map navigation, and both world regions](assets/images/demo.gif)

## Features

- Interactive maps of Palpagos and the World Tree
- Live player locations and save-backed leaderboards for level, captures,
  Paldeck discoveries, Arena RP, exploration, and boss/tower clears
- Guild bases, members, and assigned worker Pals
- Current companion Pals linked to their players
- Search and filters for players, guilds, Pals, bases, and in-game locations
- Live player count, server FPS, uptime, base count, in-game day, and connection status
- Optional save integration adds offline players, saved locations, levels,
  guilds, capture totals, Paldeck progress, Arena RP, fast-travel points,
  discovered areas, boss/tower clears, and last-seen times
- Optional saved-character connection proves control with a nonce-selected,
  reversible inventory sequence before granting self-only completion access;
  inventory contents never leave the backend
- Configurable refresh intervals and world-object layers
- Demo mode with fictional moving players and world objects

## Find anything on the map

Open the map filter to choose which markers are shown and search for players, guilds, Pals, bases, or in-game landmarks. Select a result to jump to its location and open its details.

## Run with Docker

Enable Palworld's REST API, then start the map with the REST API URL and your server's admin password:

```bash
docker run -d \
  --name palworld-live-map \
  --restart unless-stopped \
  -p 8080:8080 \
  -e PALWORLD_REST_URL="http://your-palworld-server:8212" \
  -e PALWORLD_ADMIN_PASSWORD="your-admin-password" \
  ghcr.io/lukehollanddev/palworld-live-map:latest
```

Replace the URL and password with your server's values, then open <http://localhost:8080>. Enable Palworld's game-data API to also display bases, Pals, and NPCs. A healthcheck endpoint is available at `/-/health`.

To enable save enrichment, add `-v /path/to/Pal/Saved/SaveGames/0:/data/palworld/saves:ro`
and `-e SAVE_DATA_ENABLED=true`. The save directory must be mounted read-only.

In-game locations are bundled with the map, so they remain available without the game-data API.

The bundled Compose file provides the same single-service setup:

```bash
cp .env.example .env
# Edit .env with the server URL and admin password, then:
docker compose up -d
```

For a local preview that does not need a Palworld server or credentials:

```bash
docker run --rm -p 127.0.0.1:8080:8080 -e DEMO_MODE=true \
  ghcr.io/lukehollanddev/palworld-live-map:latest
```

Docker is the supported deployment method. See [Development](DEVELOPMENT.md#run-locally) to run from source.

## Run with Palworld Server Docker

If you run your server with [`thijsvanloef/palworld-server-docker`](https://github.com/thijsvanloef/palworld-server-docker), add the map to the same Compose file, set `ADMIN_PASSWORD`, then start both services with `docker compose up -d`. The map connects through the `palworld` service name:

```yaml
services:
  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    environment:
      ADMIN_PASSWORD: "${ADMIN_PASSWORD}"
      REST_API_ENABLED: "true"
      REST_API_PORT: "8212"
      ENABLE_GAMEDATA_API: "true"
    volumes:
      - palworld-data:/palworld

  map:
    image: ghcr.io/lukehollanddev/palworld-live-map:latest
    restart: unless-stopped
    environment:
      PALWORLD_REST_URL: http://palworld:8212
      PALWORLD_ADMIN_PASSWORD: "${ADMIN_PASSWORD}"
      SAVE_DATA_ENABLED: "true"
      PALWORLD_SAVE_ROOT: /palworld-data/Pal/Saved/SaveGames/0
    ports:
      - "${HTTP_PORT:-8080}:8080"
    volumes:
      - palworld-data:/palworld-data:ro

volumes:
  palworld-data:
```

## Configuration

Every supported environment option and timeout is listed below and documented in [`.env.example`](.env.example).

| Variable                  | Purpose                                                              | Default                |
| ------------------------- | -------------------------------------------------------------------- | ---------------------- |
| `PALWORLD_REST_URL`       | Private URL of the official Palworld REST API                        | required               |
| `PALWORLD_ADMIN_PASSWORD` | REST admin password; never sent to browsers                          | required               |
| `DEMO_MODE`               | Use fictional data and do not contact Palworld                       | `false`                |
| `HTTP_PORT`               | Host port published by Compose                                       | `8080`                 |
| `ADDR`                    | Address the Go HTTP server listens on                                | `:8080`                |
| `POLL_INTERVAL`           | Player and metrics refresh interval; minimum `2s`                    | `5s`                   |
| `UPSTREAM_TIMEOUT`        | Player and server-information timeout; must be below `POLL_INTERVAL` | `4s`                   |
| `WORLD_DATA_ENABLED`      | Poll bases, Pals, and NPCs                                           | `true`                 |
| `WORLD_POLL_INTERVAL`     | World-object refresh interval; minimum `5s`                          | `15s`                  |
| `WORLD_TIMEOUT`           | World-object timeout; must be below `WORLD_POLL_INTERVAL`            | `10s`                  |
| `SAVE_DATA_ENABLED`       | Enrich REST-visible players from immutable save backups              | `false`                |
| `PALWORLD_SAVE_ROOT`      | Read-only `SaveGames/0` directory                                    | `/data/palworld/saves` |
| `PALWORLD_SAVE_WORLD_ID`  | Exact world ID when automatic discovery is ambiguous                 | empty                  |
| `SAVE_POLL_INTERVAL`      | Save enrichment interval; minimum `15s`                              | `30s`                  |
| `SAVE_TIMEOUT`            | Whole-generation timeout; must be below `SAVE_POLL_INTERVAL`         | `20s`                  |
| `PLAYER_CLAIMS_ENABLED`   | Enable save-backed “This is me” character connection                 | `false`                |
| `PLAYER_CLAIMS_ORIGIN`    | Exact public HTTPS origin used for same-origin request validation     | required when enabled  |
| `PLAYER_CLAIMS_SECRET_FILE` | Owner-only file containing a persistent 32-byte installation secret | required when enabled  |
| `PLAYER_CLAIMS_TRUSTED_PROXIES` | Comma-separated proxy CIDRs trusted to supply `X-Forwarded-For` | empty                  |

To enable save integration, mount the server's `SaveGames/0` directory read-only
and set `SAVE_DATA_ENABLED=true`. The image includes the pinned
[`palworld-save-reader`](https://github.com/LukeHollandDev/palworld-save-reader)
automatically. Save records add offline players and progression without exposing
raw account, player, or guild identifiers. Leaderboards omit an individual stat
when that value is unavailable rather than treating it as zero.

Save decoding can use substantial memory, so leave container headroom. A
decoding problem does not interrupt the live map: online players and available
progress remain visible, while the map reports that offline details are
temporarily unavailable.

### Connect a saved character privately

Character connection is opt-in and requires save integration plus HTTPS. It
does not ask a player to reveal an inventory item, party member, raw account ID,
or server password. After establishing a fresh immutable baseline, the map
selects an eight-slot inventory cycle from at least sixteen uniquely
distinguishable stacks. The player performs seven ordered swaps, waits for that
state to appear in a later immutable backup, then performs the reverse sequence
so their inventory is restored. The cycle has more than 25 bits of random
selection at the minimum candidate count and every phase must appear in a new
safe generation. The browser receives slot numbers only—never item names,
counts, instance IDs, or raw save identifiers.

Create a dedicated installation secret; do not reuse the Palworld admin
password:

```bash
umask 077
openssl rand -hex 32 > player-claims.secret
```

Mount that file read-only in the map container, then set
`PLAYER_CLAIMS_ENABLED=true`, `PLAYER_CLAIMS_ORIGIN` to the exact public origin
(for example `https://map.example.com`), and `PLAYER_CLAIMS_SECRET_FILE` to its
absolute container path. Keep it owner-only and readable by the container's
non-root runtime user. The secret must survive restarts and be backed up: it
keeps private character subjects and opaque public IDs stable even when the
REST URL or admin password changes.
Enabling claims on an existing deployment changes opaque player/guild IDs once
as they move from credential-derived IDs to the installation-secret key.

When an HTTPS reverse proxy connects to the map, configure its exact address
range in `PLAYER_CLAIMS_TRUSTED_PROXIES` and have it append `X-Forwarded-For`.
Forwarding headers from every other source are ignored. Apply request limits at
the proxy as well as the map's built-in per-client and process-wide limits.

Because roster safety deliberately reads the second-newest native backup, each
of baseline, proof, and restore normally needs roughly two Palworld backup
intervals plus save polling. Each phase has a 90-minute deadline. Sessions are
held only in memory in this first release, use Secure, HttpOnly, SameSite=Strict
cookies, and must be re-established after a map restart. Connected players can
see exact save-confirmed waypoint and journal completion through
`/api/me/progress` only;
raw save state keys are projected to public catalogue location IDs. Manual
checklist marks remain in that browser and are never copied into save evidence.

When character connection is enabled, unauthenticated player APIs publish live
players only and remove save-derived progress, offline roster entries, saved
locations, and last-seen values. The feature still does not make live names and
positions private; restrict access to the deployment separately if those are
sensitive.

## License

The Go application, web application, documentation, and other original project files are [MIT](LICENSE) unless marked otherwise. Palworld-derived map textures, screenshots, and extracted game data remain copyright Pocketpair; see the [Palworld asset provenance](assets/palworld/README.md) for extraction sources and the reproducible workflow. Inclusion in the same repository or container does not replace a component's own terms.

Palworld Live Map is an independent, fan-made project. It is not affiliated with, endorsed by, or sponsored by Pocketpair, Inc. Palworld and related names and marks belong to their respective owners.
