# Palworld server with live map

This Compose project starts a new Palworld dedicated server and the read-only live map together. It uses [thijsvanloef/palworld-server-docker](https://github.com/thijsvanloef/palworld-server-docker) for the game server.

## Start

From this directory:

```bash
cp .env.example .env
# Replace both passwords and review the server settings in .env.
docker compose up -d
docker compose logs -f palworld
```

The first start downloads Palworld and can take several minutes. The map starts immediately and shows the server as unavailable until the game REST API is ready. Open `http://localhost:8080` when both services are healthy.

Players connect to the host on UDP port `8211`. The query port is UDP `27015`. The REST API uses TCP `8212` only inside the Compose network and must not be forwarded or added to `ports`.

## Operate

```bash
# Status and logs
docker compose ps
docker compose logs -f map
docker compose logs -f palworld

# Pull updates and recreate the containers
docker compose pull
docker compose up -d

# Stop without deleting the world
docker compose down
```

World data and the game image's automatic backups live under `./palworld`. Back up that directory before upgrades or configuration changes. `docker compose down -v` is unnecessary and should be avoided.

To rotate the REST admin password, change `ADMIN_PASSWORD` in `.env`, then run `docker compose up -d --force-recreate`. Both services receive the new value together.

## Internet access

Forward UDP `8211` to host the game publicly. Forward UDP `27015` only when using the community server browser. Do not forward TCP `8212`.

The map is passwordless by design. Before exposing HTTP `8080`, place it behind an HTTPS reverse proxy and access control if player positions should remain private.

