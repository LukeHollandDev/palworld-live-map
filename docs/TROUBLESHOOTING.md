# Troubleshooting

## The map says the server is unavailable

Confirm the Palworld REST API is enabled, the server has finished starting, and the URL is reachable from the map container:

```bash
docker compose logs map
docker compose exec map /palworld-live-map -healthcheck
```

For a Palworld container in another Compose project, `localhost` points back to the map container. Use a shared Docker network and service name, the Docker host address, or another private hostname.

`401 Unauthorized` means `PALWORLD_ADMIN_PASSWORD` does not match the game server's `AdminPassword`. Rotate both together and recreate the map container.

## Players work but bases, Pals, or NPCs do not

The optional layers require the game-data API:

```text
ENABLE_GAMEDATA_API=true
```

Restart the Palworld server after changing it. A `404` from `/v1/api/game-data` is shown in the UI as an unsupported/disabled game-data API.

These layers contain currently loaded actors and Palboxes, not every saved building part. Empty regions can therefore be normal.

## The full-stack server is still unavailable

The first start downloads the dedicated server and can take several minutes. Follow progress with:

```bash
docker compose logs -f palworld
docker compose ps
```

The map deliberately starts independently and recovers automatically when Palworld becomes ready.

## Players appear on the wrong map or in the wrong place

Confirm both the game and live-map images are current. Include the Palworld version, map container tag, affected coordinates, selected region, and a screenshot in a compatibility report.

Map coordinates and artwork provenance are recorded in `assets/map/manifest.json`. If a game update changes either texture or bounds, regenerate and recalibrate the maps using `tools/maps`.

## The website should not be public

The map is passwordless by design. Bind `HTTP_PORT` to a private interface or place it behind an HTTPS reverse proxy with authentication. Do not publish or forward Palworld REST port `8212`; it accepts the server's administration credential.

## Collecting a useful report

Include sanitized output from:

```bash
docker compose ps
docker compose logs --tail=200 map
docker compose config
```

Remove passwords, public IP addresses, player names, and private hostnames before posting logs.

