# Map artwork and provenance

`palpagos.jpg` and `world-tree.jpg` are 8192×8192 overview textures exported directly from a locally installed Palworld 1.0 client. No image, tile, or API response from TH.GL is present in these files.

The repeatable, read-only export workflow is documented under [`tools/maps`](../../tools/maps/README.md). [`manifest.json`](manifest.json) records the exact Unreal object paths, source PAK and mappings hashes, native dimensions, conversion tool version, coordinate bounds, and output hashes.

The game already supplies both textures at 8192×8192. The exporter performs a deterministic JPEG conversion and does not use AI upscaling or add generated detail.

## Ownership and removal

Palworld and the underlying artwork are owned by Pocketpair. This unofficial project is not affiliated with or endorsed by Pocketpair, Palworld Entertainment, or TH.GL. Technical extraction from an installed copy does not itself grant redistribution permission.

Rights holders can request a change or removal by opening an issue or using the private reporting address in [`SECURITY.md`](../../SECURITY.md). A valid request will be handled promptly; the documented fallback is to distribute only the exporter and require operators to supply their own installed copy.
