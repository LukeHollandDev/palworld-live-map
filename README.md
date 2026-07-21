# Palworld Live Map

[![CI](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml/badge.svg)](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/container-GHCR-2496ed?logo=docker&logoColor=white)](https://github.com/LukeHollandDev/palworld-live-map/pkgs/container/palworld-live-map)
[![License: MIT](https://img.shields.io/badge/code-MIT-green.svg)](LICENSE)

A self-hosted, read-only live map for Palworld dedicated servers. Players can see who is online and where they are across Palpagos and World Tree, with live bases, Pals, NPCs, and server health—all in a browser with no client mods.

![A populated Palworld Live Map demo showing players, bases, Pals and NPCs](assets/images/demo.png)

## What Is Palworld Live Map?

Palworld Live Map is a self-hosted website for communities running a Palworld dedicated server. It connects to Palworld's official REST API and displays the current server state on interactive Palpagos and World Tree maps, including online players, bases, Pals, NPCs, and server health.

## Features

- Interactive Palpagos and World Tree maps
- Live player locations and online-player list
- Bases, companion Pals, wild Pals, and NPCs
- Hierarchical filters: players → companion Pals and guilds → bases → assigned Pals
- Live connection freshness, player capacity, server FPS, frame time, uptime, base count, and in-game day
- Configurable polling intervals and world-object layers
- Demo mode with fictional moving players and world objects
- Browser-based interface with no client mods required

## Explore guilds and companion Pals

Expand an online player to see their travelling companion Pals, or expand a guild to inspect its bases and assigned Pals. Selecting a guild opens a combined view of its currently visible members, bases, and Pals across the available maps.

![The Palworld Live Map demo showing a companion Pal beneath its player, bases and assigned Pals beneath their guild, and the guild details panel](assets/images/demo-details.webp)

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

### Run with Palworld Server Docker

If you use [`thijsvanloef/palworld-server-docker`](https://github.com/thijsvanloef/palworld-server-docker), add the map to the same Compose file. Both services use the same `ADMIN_PASSWORD`, and the map connects using the `palworld` service name:

```yaml
services:
  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    environment:
      ADMIN_PASSWORD: "${ADMIN_PASSWORD}"
      REST_API_ENABLED: "true"
      REST_API_PORT: "8212"
      ENABLE_GAMEDATA_API: "true"

  map:
    image: ghcr.io/lukehollanddev/palworld-live-map:latest
    restart: unless-stopped
    environment:
      PALWORLD_REST_URL: http://palworld:8212
      PALWORLD_ADMIN_PASSWORD: "${ADMIN_PASSWORD}"
    ports:
      - "${HTTP_PORT:-8080}:8080"
```

Set `ADMIN_PASSWORD` in the project's `.env`, then run `docker compose up -d`.

## Configuration

Every supported environment option and timeout is listed below and documented in [`.env.example`](.env.example).

| Variable                  | Purpose                                                           | Default  |
| ------------------------- | ----------------------------------------------------------------- | -------- |
| `PALWORLD_REST_URL`       | Private URL of the official Palworld REST API                     | required |
| `PALWORLD_ADMIN_PASSWORD` | REST admin password; never sent to browsers                       | required |
| `DEMO_MODE`               | Use fictional data and do not contact Palworld                    | `false`  |
| `HTTP_PORT`               | Host port published by Compose                                    | `8080`   |
| `ADDR`                    | Address the Go HTTP server listens on                             | `:8080`  |
| `POLL_INTERVAL`           | Player and metrics refresh interval; minimum `2s`                 | `5s`     |
| `UPSTREAM_TIMEOUT`        | Player and server-information timeout; must be below `POLL_INTERVAL` | `4s`  |
| `WORLD_DATA_ENABLED`      | Poll bases, Pals, and NPCs                                        | `true`   |
| `WORLD_POLL_INTERVAL`     | World-object refresh interval; minimum `5s`                       | `15s`    |
| `WORLD_TIMEOUT`           | World-object timeout; must be below `WORLD_POLL_INTERVAL`         | `10s`    |

## License

[MIT](LICENSE)

Palworld Live Map is an independent, fan-made project. It is not affiliated with, endorsed by, or sponsored by Pocketpair, Inc. Palworld and related names and marks belong to their respective owners.
