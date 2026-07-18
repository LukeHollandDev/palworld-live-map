# Palworld Live Map

[![CI](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml/badge.svg)](https://github.com/LukeHollandDev/palworld-live-map/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/container-GHCR-2496ed?logo=docker&logoColor=white)](https://github.com/LukeHollandDev/palworld-live-map/pkgs/container/palworld-live-map)
[![License: MIT](https://img.shields.io/badge/code-MIT-green.svg)](LICENSE)

A self-hosted, read-only live map for Palworld dedicated servers. Players can see who is online and where they are across Palpagos and World Tree, with live bases, Pals, NPCs, and server health—all in a browser with no client mods.

![A populated Palworld Live Map demo showing players, bases, Pals and NPCs](assets/images/demo.png)

## What is Palworld Live Map?

Palworld Live Map is a self-hosted website for communities running a Palworld dedicated server. It connects to Palworld's official REST API and displays the current server state on interactive Palpagos and World Tree maps, including online players, bases, Pals, NPCs, and server health.

## Features

- Interactive Palpagos and World Tree maps
- Live player locations and online-player list
- Bases, companion Pals, wild Pals, and NPCs
- Server status, performance, uptime, and in-game day information
- Configurable polling intervals and world-object layers
- Demo mode with fictional moving players and world objects
- Browser-based interface with no client mods required

## Try it without a Palworld server

Demo mode generates fictional moving players and world objects while exercising the production poller, API, and frontend:

```bash
docker run --rm -p 8080:8080 -e DEMO_MODE=true \
  ghcr.io/lukehollanddev/palworld-live-map:latest
```

Open <http://localhost:8080>.

## Run the map with your Palworld server

First, enable Palworld's official REST API. Enable the game-data API as well if you want to display bases, Pals, and NPCs. With [`thijsvanloef/palworld-server-docker`](https://github.com/thijsvanloef/palworld-server-docker), use:

```yaml
environment:
  REST_API_ENABLED: "true"
  REST_API_PORT: "8212"
  ENABLE_GAMEDATA_API: "true"
```

Clone this repository and create your local configuration:

```bash
git clone https://github.com/LukeHollandDev/palworld-live-map.git
cd palworld-live-map
cp .env.example .env
```

Edit `.env` and set the URL that the map container can use to reach your Palworld REST API, along with the same admin password configured on your server:

```dotenv
PALWORLD_REST_URL=http://your-palworld-server:8212
PALWORLD_ADMIN_PASSWORD=replace-with-your-admin-password
```

Start the map:

```bash
docker compose up -d
```

Open <http://localhost:8080>, or change `HTTP_PORT` in `.env` to publish the map on another host port.

### Run alongside Palworld Server Docker

When both containers are in the same Compose project, the map can reach the REST API using the Palworld service name. The important parts of the configuration look like this:

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

Set `ADMIN_PASSWORD` in the project's `.env`; Compose passes the same value to Palworld and the map.

A complete ready-to-run example is included in [`deploy/full-stack`](deploy/full-stack). After cloning the repository:

```bash
cd deploy/full-stack
cp .env.example .env
# Edit .env and replace the passwords before starting the services.
docker compose up -d
docker compose logs -f palworld
```

## Configuration

Most installations only need these settings:

| Variable                  | Purpose                                        | Default  |
| ------------------------- | ---------------------------------------------- | -------- |
| `PALWORLD_REST_URL`       | Private URL of the official Palworld REST API  | required |
| `PALWORLD_ADMIN_PASSWORD` | REST admin password; never sent to browsers    | required |
| `DEMO_MODE`               | Use fictional data and do not contact Palworld | `false`  |
| `HTTP_PORT`               | Host port used by Compose                      | `8080`   |
| `POLL_INTERVAL`           | Player and metrics refresh interval            | `5s`     |
| `WORLD_DATA_ENABLED`      | Poll bases, Pals, and NPCs                     | `true`   |
| `WORLD_POLL_INTERVAL`     | World-object refresh interval                  | `15s`    |

Every option and timeout is documented in [`.env.example`](.env.example).

## License

[MIT](LICENSE)
