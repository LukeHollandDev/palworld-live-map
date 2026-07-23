# Encounter landmark data

`manifest.json` is a compact, versioned projection generated from a locally
installed copy of Palworld. It contains field Alpha Pal spawners and tower
bosses. These locations are static game data rather than live actors, so the
backend ships them separately from live REST snapshots.

The project-owned [Palworld Asset Exporter](../../exporter)
produces the manifest directly from the installed PAK. It records the game
version, exporter and decoder versions, mappings digest, source PAK digests,
and every game-data source used by the extraction.

The version is not entered by hand: the exporter reads `ProjectVersion` from
the PAK's `Pal/Config/DefaultGame.ini` and records that exact value in both
generated manifests.

Field Alphas come from the game's `DT_BossSpawnerLoactionData` table (including
the spelling used by the asset), joined to the monster-parameter and English
Pal-name tables. Tower coordinates come from the nine placed
`BP_PalBossTower` actors in `PL_MainWorld5`; boss identities and normal-mode
levels come from `BP_PalBossBattleManager`, names and elements come from the
joined monster rows, and tower names come from the English world-map text
table.

To regenerate and review the catalogue from an installed game:

```sh
make game-assets
diff -u assets/landmarks/manifest.json build/landmarks/manifest.json
```

Only copy the reviewed generated manifest into this directory when updating
the bundled game-data snapshot.

Palworld and the underlying game data are owned by Pocketpair. This unofficial
project is not affiliated with or endorsed by Pocketpair or Palworld
Entertainment. Technical extraction from an installed copy does not itself
grant redistribution permission; rights holders can request a change or
removal through the process documented in [`SECURITY.md`](../../SECURITY.md).
