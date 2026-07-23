# Save-backed data

Palworld Live Map treats each data source according to what it can reliably
answer:

| Source | Authority | Current uses |
| --- | --- | --- |
| Official REST `/players` | Live, online-only | Online state, current level and current position |
| Official REST `/game-data` | Live, loaded actors | Bases, assigned/companion/wild Pals, NPCs and live guild joins |
| Completed native save backup | Persistent, slightly delayed | Complete player roster, guild membership/name, level, capture/Paldeck progress, last seen and last-saved position |
| Exporter-generated catalogue | Static game data | Field Alpha Pal and tower-boss encounter locations |

The bundled catalogue is generated directly from an installed Palworld PAK.
Field Alpha coordinates and levels come from the boss-spawner table. Tower
coordinates come from placed tower actors, while their boss identities and
levels come from the game's boss-battle manager. They are exact static game
assets for the recorded build, not save-derived or live actor positions.

The backend overlays REST players onto the saved roster using a normalized
player GUID. REST wins for an online player's level and position; saved guild
metadata fills gaps. Raw player, guild, account, and actor identifiers never
leave the backend. The public API receives keyed opaque IDs.

Per-player progress comes from typed fields under `SaveData.RecordData`:

- `PalCaptureCount` provides Pal-only lifetime captures after excluding the separate `Human` capture key.
- `TribeCaptureCount` provides the distinct Pal-species capture count.
- `PaldeckUnlockFlag` provides the number of unlocked/discovered Paldeck entries by counting true flags.

The values remain optional through the reader and public API. A missing or malformed block is reported as unavailable rather than silently presented as zero or as a partial aggregate.

### Historical playtime is not in the server save

Dedicated-server `Level.sav`, `LevelMeta.sav`, and `Players/*.sav` data do not contain an authoritative total-playtime field. Palworld has a client-local `Local_PlayTime` field in each player's `LocalData.sav`, but that file is not part of the server save tree and its unit is undocumented. The map therefore does not label last-seen timestamps or inferred sessions as total playtime. A future persisted online-duration tracker could expose a separately labelled "observed since" value, but it would not be historical Palworld playtime.

## Safe snapshot selection

Mount only the server's `Pal/Saved/SaveGames/0` directory and mount it read-only.
The reader does not write to the game tree, call `/save`, or parse the mutable
live files. It scans `WORLD/backup/world`, validates complete generations
containing `Level.sav`, `LevelMeta.sav`, and `Players`, then reads the
second-newest complete generation when two are available. The newest generation
is deliberately left alone in case Palworld is still publishing it.

If the root contains exactly one valid world, it is selected automatically. Set
`PALWORLD_SAVE_WORLD_ID` to its 32-hex directory name when multiple worlds are
present. Symlinked worlds, generations, and required files are rejected.

Example bind mount:

```yaml
services:
  map:
    environment:
      SAVE_DATA_ENABLED: "true"
      PALWORLD_SAVE_ROOT: /data/palworld/saves
      SAVE_POLL_INTERVAL: 30s
      SAVE_TIMEOUT: 20s
    volumes:
      - type: bind
        source: /srv/palworld/Pal/Saved/SaveGames/0
        target: /data/palworld/saves
        read_only: true
        bind:
          create_host_path: false
```

A failed or format-incompatible refresh never replaces a good roster. The API
marks the retained snapshot stale, and the browser explains that it is showing
the last successful save data.

## Save decompression

Current Palworld `Level.sav` files use Oodle Mermaid compression. Save reading
remains disabled by default.

Decompression runs in a one-shot subprocess. The parent sends only the
compressed stream and declared output size, removes the subprocess environment,
caps output and diagnostics, and kills it at `SAVE_TIMEOUT`. Invalid output or
a subprocess crash fails that refresh without replacing the last good roster.
Corresponding source and build instructions are included in the repository and
container.

## Extending the extractor

The save parser is a bounded, selective reader rather than a generic object
materializer. Add new fields to its typed snapshot and adapter, with fixtures
for each supported save layout. Candidate future layers to investigate include
guild bases and per-player normal/tower boss defeat flags.

Static encounters live in `assets/landmarks/manifest.json`, independently of
the live object API. The project exporter under `tools/map-exporter` recreates
the manifest from a locally installed game. Schema version 2 records the game,
generator and decoder versions, mappings and PAK digests, exact Unreal source
objects, and deterministic projected locations, making updates auditable when
Palworld changes spawn data.

The selective parser is derived from Palhelm under Apache-2.0.
`palworld-save-decode` and its ooz-derived decoder are GPL-3.0-or-later. See
the repository's licensing table. The static game-data extraction workflow
and source-object provenance are documented alongside the generated landmark
manifest.
