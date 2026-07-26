# Palworld Asset Exporter

The Palworld Asset Exporter regenerates the map artwork and static world catalogue that ship with Palworld Live Map. It runs inside Docker against an installed copy of the game, mounts the PAK files read-only, and never modifies the installation.

## Output

- `build/maps/palpagos.jpg` and `build/maps/world-tree.jpg`, the overview textures
- `build/maps/manifest.json`, source and output hashes plus export metadata
- `build/landmarks/manifest.json`, the original 90 Field Alphas and nine tower bosses
- `build/landmarks/catalogue/manifest.json`, provenance, scan metadata, dataset counts, and SHA-256 hashes
- `build/landmarks/catalogue/encounter-additions.json`, 33 human bounties and three oil rigs
- `build/landmarks/catalogue/navigation.json`, 22 watchtowers, 152 waypoints, and 170 dungeon entrances
- `build/landmarks/catalogue/collectibles.json`, 407 Pal Effigies, 64 Journals, and 106 Ancient Shrine pickups
- `build/landmarks/catalogue/npc-locations.json`, 90 fixed, game-placed NPC locations

The expanded catalogue contains 1,047 records in addition to the 99 legacy encounter landmarks. Each record retains its canonical game ID, coordinates, source package and object where applicable, and state/instance keys that can later be joined to save data. Texture references and structured Shrine rewards are retained in JSON; the referenced textures are not currently exported as binary image files.

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
3. Exports the two overview textures, then reads the boss-spawner, monster, localization, navigation, item, note, and NPC tables plus the persistent `PL_MainWorld5` actors.
4. Enumerates and loads all 9,977 generated World Partition packages in deterministic order, scanning them once for dungeon entrances, Effigies, Journals, Ancient Shrine pickups, and fixed NPC spawners.
5. Reconciles exact counts, class/biome/category histograms, table joins, IDs, non-zero instance GUIDs, and intentional NPC exclusions. A missing package or unexpected game-data change stops the export instead of producing a partial catalogue.
6. Writes deterministic output to hidden staging directories inside each configured output directory, keeping the eventual file moves safe for bind-mounted destinations.
7. Re-enumerates and re-hashes every PAK and the mappings file once staging is complete, failing the run if any filename, size, or hash changed during extraction.
8. Promotes the staged files only after all source checks pass. Existing output is backed up during promotion and restored if a move fails; if rollback itself cannot restore a file, the exporter preserves its recovery directory and reports the path.

Wild Pal spawn regions, dungeon encounter pools, and quests are intentionally outside this catalogue. It also excludes NPC coordinates sourced from third-party APIs, unconfigured/template spawners, and reward or emote helpers that are not fixed character placements.
