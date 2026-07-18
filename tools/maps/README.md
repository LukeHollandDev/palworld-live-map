# Generate map artwork from an installed game

This maintainer tool reads map textures from a locally installed copy of Palworld. It does not download assets from TH.GL or any other map service, and it mounts the game PAKs read-only.

Requirements:

- A legitimately installed, current Palworld client.
- Docker.
- `curl` and `shasum` on the host.

On macOS with Palworld installed in the default CrossOver Steam bottle:

```bash
PALWORLD_GAME_VERSION=1.0 ./tools/maps/export
```

For another installation or output directory:

```bash
PALWORLD_ROOT="/path/to/Palworld" \
MAP_OUTPUT_DIR="$PWD/build/maps" \
PALWORLD_GAME_VERSION="1.0" \
./tools/maps/export
```

The tool pins CUE4Parse and the Palworld community mappings file, verifies the mappings checksum, exports the two known in-game textures, applies a deterministic 8192×8192 JPEG conversion, and records the inputs and output hashes in `manifest.json`.

Review alignment and image quality in a local demo before copying generated files into `assets/map`. Never commit PAK files, mappings caches, temporary exports, or other game content.

These steps establish technical provenance but do not grant permission to redistribute Pocketpair artwork. See the asset notice before publishing generated output.
