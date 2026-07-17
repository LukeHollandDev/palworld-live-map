# Palworld Live Map

A focused, public read-only live map for Palworld dedicated servers. It polls the official REST API centrally, removes account and network identifiers, and plots online players regardless of guild membership. Optional 1.0 actor data adds bases, workers, companion Pals, wild Pals, and NPCs.

No player mod is required. Browsers never connect to Palworld and never receive the REST admin password.

```text
Palworld :8212 (private) -> central pollers -> sanitized snapshot -> web map
                                  |
                         private terrain artwork
```

## Features

- Full-screen Palpagos and World Tree map with pan, zoom, cursor coordinates, and terrain imagery
- All online players refreshed every five seconds
- Layer legend and counts for players, bases, workers, companions, wild Pals, and NPCs
- Name search, minimum-level filter, and independent layer toggles
- One central player poll and one actor poll regardless of browser count
- Last-known data retained and labelled stale during temporary REST outages
- Small dependency-free Go binary and multi-platform GHCR image
- No viewer login; the HTTP surface is read-only

Pocketpair documents the [player list](https://docs.palworldgame.com/api/rest-api/players) and the optional [world actor snapshot](https://docs.palworldgame.com/api/rest-api/game-data). The actor API represents loaded characters and Palboxes; it does not expose every placed wall, chest, or crafting station. Showing every building piece would require a separate save-file parser.

## Palworld server configuration

The regular player layer needs the REST API. Extra layers also require the game-data launch flag. With `thijsvanloef/palworld-server-docker`:

```yaml
environment:
  REST_API_ENABLED: "true"
  REST_API_PORT: "8212"
  ENABLE_GAMEDATA_API: "true"
```

Changing `ENABLE_GAMEDATA_API` requires a Palworld server restart. Keep port 8212 private; only the live-map container needs access.

## Run locally

```bash
cp .env.example .env
# Edit .env, then:
set -a; source .env; set +a
export PALWORLD_REST_URL=http://your-palworld-host:8212
./scripts/fetch-private-map-art.sh ./maps
go run ./cmd/palworld-live-map
```

Open `http://localhost:8080`.

## Docker and homelab

```bash
docker pull ghcr.io/lukehollanddev/palworld-live-map:latest
docker compose -f compose.example.yml up -d
```

For stable deployments, pin a release tag. The example joins `palworld_default` and `homelab_proxy`, publishes no host port, and reaches Palworld internally at `http://server:8212`.

`https://palworld.lukeholland.dev` can coexist with the game endpoint: HTTPS uses TCP 443 while Palworld uses UDP 8211. The included [nginx vhost](deploy/palworld.lukeholland.dev.conf) proxies only the web app.

The map has no viewer authentication. If the hostname is internet-accessible, live names, positions, and enabled object layers are public. Use nginx access control or a VPN if that is not desired.

## Terrain artwork

Game-derived artwork is not committed or published in the image. Install the two private 1.0 overview files on the host:

```bash
./scripts/fetch-private-map-art.sh ./maps
```

See [maps/README.md](maps/README.md) for source and orientation details. The files are ignored by both Git and Docker.

## Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `PALWORLD_REST_URL` | yes | — | Private REST base URL, usually `http://server:8212` |
| `PALWORLD_ADMIN_PASSWORD` | yes | — | REST Basic Auth password; backend only |
| `ADDR` | no | `:8080` | HTTP listen address |
| `POLL_INTERVAL` | no | `5s` | Player poll interval, minimum 2 seconds |
| `UPSTREAM_TIMEOUT` | no | `4s` | Player request timeout |
| `WORLD_DATA_ENABLED` | no | `true` | Poll the optional actor snapshot |
| `WORLD_POLL_INTERVAL` | no | `15s` | Actor poll interval, minimum 5 seconds |
| `WORLD_TIMEOUT` | no | `10s` | Actor snapshot timeout |
| `MAP_ASSET_DIR` | no | — | Directory containing private WebP maps |
| `SITE_TITLE` | no | `Palworld Live Map` | Header and browser title |

## Publishing to GHCR

Pushing `main` publishes `ghcr.io/lukehollanddev/palworld-live-map:latest`, `:main`, and a commit tag. Version tags follow semantic versioning: while the project is experimental, releases increment the minor component; for example, `v0.1.0` publishes `:0.1.0` and `:0.1`, followed later by `v0.2.0`. Once the interface is stable, `v1.0.0` publishes `:1.0.0` and `:1.0`. The `:latest` tag always tracks the newest build from `main`.

Images target amd64 and arm64 and include provenance and an SBOM.

The workflow uses the built-in `GITHUB_TOKEN`; no Docker Hub credentials are required. Set the resulting GHCR package to public if anonymous pulls should work. See GitHub's [container publishing guidance](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images).

## Development

```bash
make check
make image
```

## License

MIT. Palworld and its artwork are owned by Pocketpair; no game assets are distributed by this project.
