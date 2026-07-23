# Palworld save reader

`savegame` is a bounded, read-only decoder for immutable Palworld 1.0 save
snapshots. It extracts the internal player GUID, display name, level, guild
membership/name, persisted X/Y position, last-online time, lifetime Pal
captures, distinct Pals caught, and Paldeck unlock count. It does not write to
the snapshot or expose arbitrary save content.

## API

```go
reader, err := savegame.NewReader(savegame.Options{})
snapshot, err := reader.ReadSnapshot(ctx, "/path/to/immutable-snapshot")
```

The snapshot directory must contain `Level.sav`; `Players/` is optional. Make
the snapshot outside this package before calling `ReadSnapshot` so the game
cannot mutate files during decoding. The reader detects a changing
`Level.sav` and rejects non-regular save files. `_dps.sav` sidecars are ignored.

Container decompression runs in-process through the sibling `internal/palsav`
package; this package retains its selective GVAS parser and roster projection.
It performs no decoder discovery, download, cache, copy, or save-tree write
operation.

## Limits and missing data

Defaults are 512 MiB per compressed/decompressed save and 10,000 players,
with hard caps of 2 GiB and 100,000. Collection depth/count/decoded-byte limits
inside the GVAS parser provide additional protection against malformed input.
The decoder is synchronous and in-process, so a context deadline cannot
forcibly preempt a decompression call already executing.

Name, level, and guild fields come only from `Level.sav`. X/Y, last-online, and
the three progress fields come only from the matching individual player save.
Missing or unreadable player saves leave those fields `nil` and increment
bounded aggregate stats; diagnostics never include names or GUIDs.

## Licensing

This directory is derived from Apache-2.0 Palhelm code; see [NOTICE](NOTICE)
and [LICENSE](LICENSE). It directly imports the GPL-3.0-or-later
[`palsav`](../palsav) package. The files in this directory retain their
Apache-2.0 terms, while the distributed server executable containing both
components is licensed as a whole under GPL-3.0-or-later. See the repository's
[licensing statement](../../LICENSING.md).
