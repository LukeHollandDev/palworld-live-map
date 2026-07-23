# Road to 1.0 — Direct Go Save Decoder

Working document. It records the licensing and engineering consequences of the
chosen 1.0 save-decoding architecture. It is an engineering risk map, not legal
advice.

## Chosen direction

Palworld Live Map will use the pure-Go internal `palsav` package for Palworld
container and Mermaid decompression inside the server process. The
application retains its existing selective, roster-focused GVAS parser; the
package's generic GVAS API is available but is not on the live-map data path.
This replaces the separately executed C++ decoder.

The decision optimises for a single portable binary and a smaller operational
surface:

- no C++, cgo, SIMDe, platform runtime, or helper discovery;
- no external or third-party Go modules;
- pure-Go container decompression alongside the existing bounded selective GVAS
  parser;
- byte-exact regression coverage against the available July 2026 saves.

It also changes two important boundaries:

- the ooz-derived GPL decoder is linked into the server, so the distributed
  combined executable is GPL-3.0-or-later;
- decompression is no longer process-isolated, so a decoder failure or
  pathological input can affect the long-lived service process.

Runtime configuration does not change either fact. Setting
`SAVE_DATA_ENABLED=false` does not remove linked code, and `SAVE_TIMEOUT` cannot
forcibly interrupt a synchronous decompression call already executing.

---

## Dependency and licence surface

The backend again has no external Go modules, but folding the decoder into the
root module does not make the resulting binary permissively licensed.

| Component | Role | Terms |
| --- | --- | --- |
| Original live-map Go, web, and documentation source | Service and UI | MIT unless marked otherwise |
| `internal/savegame` adapter | Bounded roster projection | Apache-2.0 with NOTICE |
| `internal/palsav` | Internal container/Mermaid decoder and generic GVAS API | GPL-3.0-or-later; includes Apache-2.0-derived portions |
| Combined Go server executable | Linked work containing all backend components | GPL-3.0-or-later |
| React runtime | Browser UI runtime | MIT |
| Palworld-derived assets | Maps and extracted game data | Pocketpair copyright; no project licence grant |

The permissive MIT and Apache-2.0 component licences remain intact for the
files they cover. GPLv3 governs conveyance of the linked executable as a whole.
The browser source, documentation, exporter, and unrelated repository
components do not become GPL merely by sharing the repository.

### Distribution requirements

Every binary or container release must:

1. identify the combined server as GPL-3.0-or-later;
2. preserve the MIT, Apache-2.0, GPL, copyright, and NOTICE material;
3. provide equivalent access to the exact Corresponding Source for that binary,
   including the internal decoder package and build scripts;
4. identify the exact source revision, rather than pointing only at a moving
   branch or a decoder-only archive; and
5. carry accurate OCI licence, source, and revision metadata.

Published images use `org.opencontainers.image.source` and
`org.opencontainers.image.revision` to identify the matching public source
revision. They also carry the complete exact Corresponding Source under
`/usr/src/palworld-live-map` and the licence set under `/licenses`. Release
automation must keep the labelled public revision available for as long as the
corresponding object code is offered.

---

## Save-data pipeline

The two logical stages remain, but both now execute in the same Go process:

| Stage | Job | Implementation |
| --- | --- | --- |
| Decompress | `PlM` Mermaid or legacy `PlZ` zlib container to raw GVAS | `internal/palsav` |
| Parse and project | GVAS bytes to the bounded player/guild roster | `internal/savegame` |

The available homelab snapshot is decisive: all examined `Level.sav`,
`LevelMeta.sav`, and `Players/*.sav` files use `PlM` Mermaid compression.
Zlib-only support would not provide the save-backed feature for this data.

The Mermaid implementation intentionally supports the subset exercised by
those saves and rejects unsupported modes. The internal package also offers a
generic parser with ordered properties and lazy collections; the live map keeps
its narrower existing GVAS parser so it decodes and exposes only the roster
fields it needs.

---

## Provenance risk

The decoder was ported from ooz-derived files in a PalworldSaveTools
distribution that declares GPL-3.0-or-later. The original Powzix ooz repository
examined during this work contains no identified licence file or other
affirmative copyright grant.

That distinction matters:

- the downstream GPL declaration states the terms applied by
  PalworldSaveTools and retained by this project;
- it does not prove that the original author authorised copying, modification,
  or redistribution;
- a Go port remains derived from the reference implementation; changing
  language does not create clean-room provenance;
- GPL compliance does not resolve separate copyright, reverse-engineering,
  trade-secret, or patent questions concerning an Oodle-compatible decoder.

The implementation includes no proprietary Epic or RAD Game Tools Oodle
runtime or source. That fact does not resolve the missing upstream grant.
Qualified IP counsel should review the provenance before a public 1.0 release.

If acceptable provenance cannot be established, the safe release choices are
to remove native-save support, obtain appropriately licensed decoding rights,
or replace the implementation with one whose independent provenance can be
demonstrated. Moving the same implementation back behind a subprocess would
restore binary and fault isolation, but would not cure its upstream provenance.

---

## Security consequences

The decoder applies input/output, string, property, collection, and depth
limits and rejects unsupported encodings. The snapshot selector continues to
reject symlinks, avoid live mutable saves, use a read-only mount, and retain the
last good roster after a failed refresh.

Those controls are defence in depth, not a sandbox. Direct integration means:

- a decoder panic can terminate the service;
- excessive CPU use cannot be killed independently;
- the surrounding context deadline is observed only when control returns to
  code that checks it;
- memory pressure is charged to the server process.

Before 1.0, fuzzing and adversarial fixture coverage should run continuously,
limits should be exercised in integration tests, and production guidance should
recommend container CPU/memory limits.

---

## 1.0 work

- [x] Decode all supplied save containers byte-for-byte in pure Go.
- [x] Parse and recursively walk the supplied GVAS data.
- [x] Record package provenance and retain GPL/Apache notices.
- [x] Integrate the internal package into the application reader.
- [x] Remove the C++ helper, SIMDe sources, and C++ runtime image layer.
- [x] Update binary/container licence metadata and ship the full licence set.
- [x] Ensure every published image points to its exact public Corresponding
      Source through OCI source and revision labels.
- [x] Re-run unit, race, fuzz, fixture, and multi-architecture build checks.
- [ ] Obtain legal review of the Powzix provenance and Oodle-related risks.
- [ ] Decide separately whether and how the Pocketpair-derived map assets may be
      redistributed.

## Post-1.0 options

- Split generic GVAS parsing from Mermaid decompression if a demonstrably
  permissive implementation becomes available.
- Add cooperative decoder cancellation or an optional worker-process mode if
  hostile-input isolation becomes more important than a single binary.
- Evaluate any migration from `internal/savegame` to the package's generic GVAS
  API as a separate change, with field-parity fixtures and peak-memory/CPU
  benchmarks against `Level.sav`, player saves, and DPS sidecars.
- Continue replacing Apache-derived projection code only when there is a clear
  maintenance benefit; doing so would not remove GPL from the linked decoder.
- Investigate additional typed projections such as guild bases and boss defeat
  flags without exposing arbitrary save contents through the public API.

---

The direct-Go architecture is a conscious trade: a simpler runtime and richer
decoder API in exchange for GPL coverage of the server executable, reduced
fault isolation, and unresolved upstream provenance that must not be described
as legally clean.
