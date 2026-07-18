# Map Exporter

This maintainer tool exports the overview map textures from an installed copy of Palworld. It runs inside Docker and mounts the game's PAK directory read-only, so it does not modify the game installation.

## What It Creates

The exporter writes these files to `build/maps` by default:

- `palpagos.jpg`
- `world-tree.jpg`
- `manifest.json`, containing source and output hashes plus export metadata

Nothing is copied into `assets/map` automatically. Review the generated files locally before replacing the checked-in artwork.

## Requirements

Before starting, install or provide:

- A legitimate, current Palworld installation
- Docker with the Docker engine running
- `curl` and `shasum`

You do not need to install .NET or C# tooling. Docker builds and runs the exporter for you.

## Quick Start on macOS with CrossOver

The script knows the default Palworld location in a CrossOver Steam bottle. From the repository root, run:

```bash
PALWORLD_GAME_VERSION="1.0" ./tools/map-exporter/export.sh
```

Replace `1.0` with the version of the installed game. Generated files will appear in `build/maps`.

## Use a Different Palworld Installation

Set `PALWORLD_ROOT` to the directory containing the game's `Pal` and `Engine` folders:

```bash
PALWORLD_ROOT="/path/to/Palworld" \
PALWORLD_GAME_VERSION="1.0" \
./tools/map-exporter/export.sh
```

To choose a different output directory as well:

```bash
PALWORLD_ROOT="/path/to/Palworld" \
PALWORLD_GAME_VERSION="1.0" \
MAP_OUTPUT_DIR="$PWD/my-exported-maps" \
./tools/map-exporter/export.sh
```

## Configuration

The script accepts configuration through environment variables:

| Variable | Purpose | Default |
| --- | --- | --- |
| `PALWORLD_ROOT` | Palworld installation directory | Default CrossOver Steam location on macOS |
| `PALWORLD_GAME_VERSION` | Version recorded in `manifest.json` | `unknown` |
| `MAP_OUTPUT_DIR` | Directory for the exported images and manifest | `build/maps` |
| `MAP_EXPORT_CACHE_DIR` | Directory for the verified mappings cache | `build/map-exporter-cache` |

## What the Exporter Does

The wrapper downloads a pinned Palworld community mappings file, verifies its checksum, builds the Docker image, and starts the exporter. The exporter then:

1. Reads the installed game PAKs.
2. Locates the two known overview textures.
3. Exports both textures as 8192×8192 JPEG files.
4. Writes the source details and file hashes to `manifest.json`.

## Troubleshooting

- If the PAK directory is not found, check that `PALWORLD_ROOT` points to the directory containing `Pal/Content/Paks`.
- If Docker cannot connect, start Docker Desktop and run the command again.
- If the installed game version is no longer compatible, the texture paths or mappings may need to be updated.

Keep source PAK files, the mappings cache, and temporary output out of the repository. Only copy reviewed map exports into `assets/map` when intentionally updating the bundled artwork.
