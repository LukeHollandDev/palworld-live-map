# Palworld Assets and Provenance

The map artwork and static world catalogue in this directory are generated from a locally installed copy of Palworld. The repeatable, read-only workflow is documented in the [Palworld Asset Exporter guide](../../exporter/README.md).

## Maps

[`maps/palpagos.jpg`](maps/palpagos.jpg) and [`maps/world-tree.jpg`](maps/world-tree.jpg) are native 8192×8192 overview textures converted to JPEG. The browser samples each image's outer pixel so the surrounding viewport uses the same background colour.

[`maps/manifest.json`](maps/manifest.json) records the Unreal object paths, source PAK and mappings hashes, dimensions, conversion tool version, coordinate bounds, and output hashes.

## Static World Catalogue

[`landmarks/manifest.json`](landmarks/manifest.json) contains Field Alpha Pal spawners and tower bosses. These are static game locations rather than live actors, so the backend ships them separately from REST snapshots.

Field Alphas are generated from `DT_BossSpawnerLoactionData`—including the spelling used by the game—and joined with monster parameters and English Pal names. Tower coordinates come from placed `BP_PalBossTower` actors in `PL_MainWorld5`; identities, levels, elements, and names are joined from the relevant battle-manager, monster, and world-map data.

[`landmarks/catalogue/manifest.json`](landmarks/catalogue/manifest.json) indexes four hashed datasets containing bounties, oil rigs, watchtowers, waypoints, dungeon entrances, Pal Effigies, Journals, Ancient Shrine pickups, and fixed NPC locations. These are joined from local data tables and actors in the persistent level and generated World Partition packages. The catalogue keeps canonical IDs, source references, collection-state keys, and structured Shrine rewards; it does not incorporate third-party coordinates.

The manifests record the game version, exporter and decoder versions, mappings digest, source PAK digests, and extraction sources. The exporter reads `ProjectVersion` directly from `Pal/Config/DefaultGame.ini`; it is not entered manually.

## Updating

Generate fresh files from an installed game:

```sh
make game-assets
diff -u assets/palworld/maps/manifest.json build/maps/manifest.json
diff -u assets/palworld/landmarks/manifest.json build/landmarks/manifest.json
diff -ru assets/palworld/landmarks/catalogue build/landmarks/catalogue
```

Review the generated files before replacing the bundled assets.

## Ownership and Removal

Palworld, its artwork, and its game data are owned by Pocketpair. This unofficial project is not affiliated with or endorsed by Pocketpair or Palworld Entertainment. Technical extraction from an installed copy does not grant redistribution permission.

Rights holders can request a change or removal by opening an issue or using the private reporting channel in [`SECURITY.md`](../../SECURITY.md). A valid request will be handled promptly; the fallback is to distribute only the exporter and require operators to supply their own installed copy.
