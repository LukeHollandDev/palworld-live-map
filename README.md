# Palworld Live Map

A self-hosted live map for Palworld dedicated servers. Players can see who is online and where they are across Palpagos and World Tree, with optional layers for bases, Pals, and NPCs. It works in a browser and requires no player mods.

## Features

- Detailed Palpagos and World Tree maps with pan, zoom, and coordinates
- Live positions for all online players, refreshed at a configurable interval
- A collapsible map explorer listing online players and guild-grouped bases with their current workers
- Companion Pals, wild Pals, and NPCs appear only when the server is actively reporting them
- Clickable details for players, bases, Pals, NPCs, and base worker rosters
- Live player capacity, FPS, frame time, uptime, base count, and in-game day metrics
- Search-driven map filtering, region controls, category visibility, and live counts
- Server name and description taken directly from your Palworld configuration
- Passwordless viewing with no separate account or sign-in
- Standalone Docker setup that does not depend on an existing proxy or network

## Quick start

### 1. Enable the Palworld APIs

The player map needs Palworld's REST API. Optional bases, Pals, and NPCs also need the game-data API. With `thijsvanloef/palworld-server-docker`:

```yaml
environment:
  REST_API_ENABLED: "true"
  REST_API_PORT: "8212"
  ENABLE_GAMEDATA_API: "true"
```

### 2. Configure and start the map

```bash
cp .env.example .env
# Set PALWORLD_REST_URL and PALWORLD_ADMIN_PASSWORD in .env, then:
docker compose up -d
```

The example REST URL is suitable when Palworld publishes its API on the same Docker host. If Palworld runs elsewhere, use a private address or hostname that the map container can reach.

### 3. Open the site

Visit `http://localhost:8080`, or use the host port selected by `HTTP_PORT` in `.env`.

## Configuration

Every option and its default is documented in [`.env.example`](.env.example). Most installations only need to set the Palworld address and admin password. You can also change the website port, refresh speed, and optional world layers there.

The site name and description come from `ServerName` and `ServerDescription` on the Palworld server.

## Learn more

- [Development and implementation details](DEVELOPMENT.md)
- [Contributing guidelines](CONTRIBUTING.md)
- [Map artwork notes](assets/map/README.md)

## License

MIT. Palworld and its included map artwork are owned by Pocketpair.
