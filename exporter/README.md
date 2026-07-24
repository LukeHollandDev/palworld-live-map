# Palworld Asset Exporter

The Palworld Asset Exporter regenerates the map artwork and encounter catalogue that ship with Palworld Live Map. It runs inside Docker against an installed copy of the game, mounts the PAK files read-only, and never modifies the installation.

## Output

- `build/maps/palpagos.jpg` and `build/maps/world-tree.jpg`, the overview textures
- `build/maps/manifest.json`, source and output hashes plus export metadata
- `build/landmarks/manifest.json`, 90 Field Alphas and nine tower bosses with names, elements, levels, and coordinates, plus the sources they came from

Nothing is copied into `assets/palworld/maps` or `assets/palworld/landmarks` automatically. Review the generated files before replacing checked-in data.

## Requirements

- A current Palworld installation
- Docker, with the engine running

Nothing else. No .NET or C# tooling, and no separate mappings download; the image bundles everything and Docker builds and runs it for you.

## Running

Everything depends on `PALWORLD_ROOT`, the directory containing the game's `Pal` and `Engine` folders. It defaults to the standard CrossOver Steam bottle path on macOS, so from the repository root:

```bash
./exporter/export.sh
```

For any other installation, point it at the game:

```bash
PALWORLD_ROOT="/path/to/Palworld" ./exporter/export.sh
```

Output directories are redirected the same way:

```bash
PALWORLD_ROOT="/path/to/Palworld" \
MAP_OUTPUT_DIR="$PWD/my-exported-maps" \
LANDMARK_OUTPUT_DIR="$PWD/my-exported-landmarks" \
./exporter/export.sh
```

The game version is always read from `Pal/Config/DefaultGame.ini` inside the mounted PAK. For automation, `PALWORLD_GAME_VERSION` may be supplied as an optional assertion. It never overrides the PAK value, and the export stops unless both match exactly:

```bash
PALWORLD_GAME_VERSION="1.0.1.100619" ./exporter/export.sh
```

## Configuration

| Variable | Purpose | Default |
| --- | --- | --- |
| `PALWORLD_ROOT` | Palworld installation directory | Default CrossOver Steam location on macOS |
| `PALWORLD_GAME_VERSION` | Optional exact-version assertion checked against the PAK-derived `ProjectVersion` | Unset |
| `MAP_OUTPUT_DIR` | Directory for the exported images and manifest | `build/maps` |
| `LANDMARK_OUTPUT_DIR` | Directory for the landmark manifest | `build/landmarks` |

## How It Works

The Docker build downloads the pinned Palworld community mappings file and fails unless it matches the pinned checksum, so the image ships with a verified copy. The wrapper script builds that image, mounts the game's PAK directory read-only, and starts the exporter, which then:

1. Hashes the ordered source PAK set before CUE4Parse mounts and reads it. These initial hashes are the manifest provenance.
2. Reads and strictly validates `ProjectVersion` from the mounted PAK.
3. Exports the two overview textures, then reads the boss-spawner, monster, English name, region-name, and boss-battle-manager game data, plus tower `BossType` and `RootComponent.RelativeLocation` from placed actors in `PL_MainWorld5`.
4. Requires exactly 90 joined Field Alphas and nine uniquely mapped tower actors, so an unexpected game-data change stops the export instead of producing a partial catalogue.
5. Writes deterministic output to hidden staging directories inside each configured output directory, keeping the eventual file moves safe for bind-mounted destinations.
6. Re-enumerates and re-hashes every PAK and the mappings file once staging is complete, failing the run if any filename, size, or hash changed during extraction.
7. Promotes the staged files only after all source checks pass. Existing output is backed up during promotion and restored if a move fails; if rollback itself cannot restore a file, the exporter preserves its recovery directory and reports the path.
