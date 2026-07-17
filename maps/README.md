# Private map artwork

The service works in coordinate-grid mode without artwork. To install private Palworld 1.0 overview imagery for your own server, run:

```bash
./scripts/fetch-private-map-art.sh ./maps
```

This downloads one overview tile per world layer from THGL's public tile CDN. The imagery is derived from Pocketpair game assets, stays on your machine, and is never included in Git or the published container image.

Alternatively, place either or both of these square WebP files here:

- `palpagos.webp`
- `world-tree.webp`

They are mounted at runtime and are excluded from Git and the Docker build context, so privately sourced game artwork is never published in the image.

The current coordinate transform expects north/top at maximum world X, south/bottom at minimum world X, west/left at minimum world Y, and east/right at maximum world Y. Bounds live in `internal/server/server.go` and can be calibrated without changing the browser protocol.
