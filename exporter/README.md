# Palworld Asset Exporter

The Palworld Asset Exporter is a top-level maintainer component that exports overview map textures and encounter landmarks from an installed copy of Palworld. It runs inside Docker and mounts the game's PAK directory read-only, so it does not modify the game installation.

Production code lives in [`src`](src), while the self-contained fixture harness
and its synthetic inputs live in [`tests`](tests).

## What It Creates

The exporter writes these files to `build/maps` by default:

- `palpagos.jpg`
- `world-tree.jpg`
- `manifest.json`, containing source and output hashes plus export metadata

It separately writes `build/landmarks/manifest.json`. This schema-version 2 manifest contains:

- Field Alpha names, elements, levels, and exact spawner coordinates
- Tower names, elements, normal-difficulty levels, and exact placed-actor coordinates
- The mappings and PAK hashes plus every game-data source used by the extraction

Nothing is copied into `assets/map` or `assets/landmarks` automatically. Review the generated files locally before replacing checked-in data.

## Requirements

Before starting, install or provide:

- A legitimate, current Palworld installation
- Docker with the Docker engine running
- a POSIX shell, `awk`, `curl`, and `shasum`

You do not need to install .NET or C# tooling. Docker builds and runs the exporter for you.

When changing the exporter itself, run `make exporter-check` from the repository root. CI performs the same locked restore, fixture suite, and release compile before the production container can be published. The fixtures exercise the production landmark shaper without mounting or reading the large game PAK.

The Docker build pins .NET SDK 10.0.302 and runtime 10.0.10 by release tag and multi-architecture image digest. NuGet restore runs in locked mode against the checked-in exporter and fixture-test `packages.lock.json` files, so an upstream image, package, or transitive-dependency change cannot silently alter the exporter. When intentionally updating a package, regenerate both lock files through the test project with the pinned SDK, review the dependency and content-hash changes, and then rerun `make exporter-check`:

```bash
docker run --rm \
  --mount "type=bind,src=$PWD/exporter,dst=/src" \
  --workdir /src \
  mcr.microsoft.com/dotnet/sdk:10.0.302-noble@sha256:ed034a8bf0b24ded0cbbac07e17825d8e9ebfe21e308191d0f7421eaf5ad4664 \
  dotnet restore tests/PalworldAssetExporter.Tests.csproj --use-lock-file --force-evaluate
```

## Quick Start on macOS with CrossOver

The script knows the default Palworld location in a CrossOver Steam bottle. From the repository root, run:

```bash
./exporter/export.sh
```

The exporter reads the exact `ProjectVersion` from `Pal/Config/DefaultGame.ini` inside the mounted PAK. Generated files will appear in `build/maps` and `build/landmarks`.

For automation, `PALWORLD_GAME_VERSION` may be supplied as an optional assertion. It never overrides the PAK value; the export stops unless both values match exactly:

```bash
PALWORLD_GAME_VERSION="1.0.1.100619" ./exporter/export.sh
```

## Use a Different Palworld Installation

Set `PALWORLD_ROOT` to the directory containing the game's `Pal` and `Engine` folders:

```bash
PALWORLD_ROOT="/path/to/Palworld" ./exporter/export.sh
```

To choose a different output directory as well:

```bash
PALWORLD_ROOT="/path/to/Palworld" \
MAP_OUTPUT_DIR="$PWD/my-exported-maps" \
LANDMARK_OUTPUT_DIR="$PWD/my-exported-landmarks" \
./exporter/export.sh
```

## Configuration

The script accepts configuration through environment variables:

| Variable | Purpose | Default |
| --- | --- | --- |
| `PALWORLD_ROOT` | Palworld installation directory | Default CrossOver Steam location on macOS |
| `PALWORLD_GAME_VERSION` | Optional exact-version assertion checked against the PAK-derived `ProjectVersion` | Unset |
| `MAP_OUTPUT_DIR` | Directory for the exported images and manifest | `build/maps` |
| `LANDMARK_OUTPUT_DIR` | Directory for the landmark manifest | `build/landmarks` |
| `ASSET_EXPORT_CACHE_DIR` | Directory for the verified mappings cache | `build/asset-exporter-cache` |

## What the Exporter Does

The wrapper downloads a pinned Palworld community mappings file, verifies its checksum, builds the Docker image, and starts the exporter. The exporter then:

1. Hashes the ordered source PAK set before CUE4Parse mounts and reads it; these initial hashes are the manifest provenance.
2. Reads and strictly validates `ProjectVersion` from `Pal/Config/DefaultGame.ini` in the mounted PAK.
3. Locates and exports the two known overview textures as 8192×8192 JPEG files.
4. Reads the boss-spawner, monster, English name, region-name, and boss-battle-manager game data.
5. Reads tower `BossType` and `RootComponent.RelativeLocation` from placed actors in `PL_MainWorld5`.
6. Requires exactly 90 joined Field Alphas and nine uniquely mapped tower actors; an unexpected game-data change stops the export instead of producing a partial catalogue.
7. Writes deterministic map and landmark files into hidden staging directories inside each configured output directory; staging on the destination filesystem keeps the eventual file moves safe for bind-mounted and independently configured output directories.
8. Re-enumerates and re-hashes every PAK and re-hashes the mappings file after all staged output is complete, failing the run if any filename, size, or hash changed during extraction.
9. Promotes the staged files only after all source checks pass. Existing output files are backed up during promotion and restored if a move fails; staging and backups are removed on success or ordinary failure. If rollback itself cannot restore a file, the exporter preserves its recovery directory and reports the path.

## Troubleshooting

- If the PAK directory is not found, check that `PALWORLD_ROOT` points to the directory containing `Pal/Content/Paks`.
- If version extraction fails, verify that the installation is complete and current. If an optional `PALWORLD_GAME_VERSION` assertion fails, update or remove it only after confirming the PAK-derived version is the one you intend to export.
- If Docker cannot connect, start Docker Desktop and run the command again.
- If the installed game version is no longer compatible, the texture paths, table joins, actor mappings, or mappings file may need to be updated. Landmark extraction deliberately fails when expected counts or joins change.
- If the final PAK or mappings verification fails, the prior output remains in place. Rerun after the game installation and mappings file have stopped updating.

Keep source PAK files, the mappings cache, and temporary output out of the repository. Only copy reviewed output into `assets/map` or `assets/landmarks` when intentionally updating bundled data.

## Licensing and provenance

The exporter implementation is project-owned and MIT-licensed, but it uses
third-party packages and a pinned community mappings file to read
Pocketpair-owned game data. See [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md)
and the repository's [`LICENSING.md`](../LICENSING.md) before distributing the
exporter or generated output.
