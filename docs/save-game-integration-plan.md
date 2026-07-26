# Save-game integration — phased re-land plan

Status: **approved plan — strategy C**
Scope: bring save-game parsing back into `main`, phase by phase, keeping the
distributed **server MIT** by isolating the GPL decoder in a separate pure-Go
sidecar binary.

---

## Approach

Palworld saves are GVAS archives wrapped in a compression container. Our real
saves are **`PlM1` (Oodle-Mermaid)** — verified on the homelab server — so
decoding them requires the Oodle-Mermaid decompressor, whose provenance is
Powzix `ooz` → PalworldSaveTools (**GPL-3.0**). It is functionally required; the
permissive zlib-only path does not read our saves.

We keep the server MIT by shipping that decoder as a **separate static Go
sidecar binary** (`savedecode`) that the server execs. A separate process the
server invokes over a defined interface is **aggregation, not linking — it does
not relicense our Go.** The GPL lives only in the sidecar; the server binary and
its source stay MIT.

**Component split:**

- **Sidecar (GPL-3.0), own module — `~/projects/palworld-save-reader`:**
  `palsav` (Oodle + GVAS decoder, zero third-party deps) + `savegame` (bounded,
  selective snapshot reader) + `cmd/savedecode` (emits compact roster JSON on
  stdout). Already extracted and green (see Phase 1).
- **Server (MIT), this repo:** a small first-party `savesidecar` exec/decode
  package + `saveroster` projection + poller/config/server wiring. All
  first-party Go over stdlib JSON. No GPL, no Palhelm code in the server binary.

The stable seam is `savegame.Snapshot` (JSON-tagged) → sidecar stdout JSON →
`saveroster`. The measured cost of this path on the real saves is **0.43 s,
79 MB RSS, 3.8 KB JSON** per poll (selective decode — see
`.local/SPIKE-FINDINGS.md`), versus the ~1 GB / 300 MB of a full-tree decode.

---

## Source branches

| Branch | What it is |
| --- | --- |
| `wip/save-game-refactor` (`d600571`) | Pure-Go decoder refactor. Tests pass. **Base to port from.** |
| `main` | Save-game removed (`a3fa796`), then moved on: exporter promotion, asset relocation, filter-category UI. |
| `archive/save-game-original` | Original integration with the vendored C++ Oodle decoder. Heavy. Reference only — do **not** base on it. |

`d600571` is one commit on top of `8ad4a99` (pre-removal), so it does **not**
contain main's later work. This is a **rebase + phased re-land**, not a
cherry-pick.

---

## Integration surface (what `main` lost in `a3fa796`)

Grouped by concern. Under strategy C the decoder libraries move to the sidecar
module; only the wiring below lands in this repo.

**Sidecar module (already extracted — Phase 1):**

- `palsav/*` — container decode (zlib + Oodle) + GVAS parser.
- `savegame/*` — bounded, read-only snapshot reader projecting player fields
  (name, level, guild, X/Y, last-online, capture totals, unique Pals, Paldeck
  count).
- `cmd/savedecode` — roster-JSON CLI (and `--full` for inspection).

**Server wiring (this repo):**

- `internal/savesidecar/*` — **new, first-party:** snapshot the save tree
  read-only, exec the sidecar with a timeout, decode its roster JSON.
- `internal/saveroster/*` — first-party roster projection, ported to consume the
  sidecar JSON.
- `internal/palworld/poller.go` — `NewPollerWithRoster` (adds `SavePollInterval`).
- `internal/palworld/client.go` — `PublicPlayerID` / `PublicGuildKey` projection.
- `cmd/palworld-live-map/main.go` — flag-gated wiring; locate the sidecar next to
  the server binary.
- `internal/config/config.go` — env vars + validation: `SAVE_DATA_ENABLED`
  (default `false`), `PALWORLD_SAVE_ROOT`, `PALWORLD_SAVE_WORLD_ID`,
  `SAVE_POLL_INTERVAL` (min 15s), `SAVE_TIMEOUT`. Rejects `DEMO_MODE` +
  `SAVE_DATA_ENABLED` together.

**API/web:**

- `internal/server` Player JSON gains optional fields (`guildName`, `lastSeenAt`,
  `captureTotal`, `uniquePalsCaptured`, `paldeckUnlocked`) — all `omitempty`, so
  older clients ignore them.
- `web/src/types.ts` — extend `Player` (untouched by main → clean re-add).
- `web/src/components/DetailsDialog.tsx` — surface the new fields.
- `web/src/App.tsx` + `App.test.tsx` — **merge point**: main added the
  filter-reveal feature here.

**Packaging/docs:**

- `Dockerfile` stays `distroless/static-debian12`; `go build`s the sidecar as a
  second static binary (no cgo/C++/Python) and ships its GPL license under
  `/licenses/...`.
- `compose.yml` read-only save-tree mount, `.env.example`, `README.md`,
  `DEVELOPMENT.md`, `docs/save-data.md`, `SECURITY.md`, `LICENSING`.

### Rebase drift (post-removal `main` work)

Almost entirely orthogonal (exporter promotion, asset relocation, filter UI):

- **Zero-collision** (fully deleted, referenced by nothing): `saveroster`,
  `config.go`, `poller.go`, `client.go`, `main.go`, `Dockerfile` — re-apply
  cleanly.
- **Real merge**: `web/src/App.tsx`, `web/src/App.test.tsx` (filter-reveal).
- **Trivial**: `internal/server/server_test.go` path drift.

---

## Phased plan

### Phase 0 — Sidecar spike (DONE, 2026-07-24)

- Built `cmd/savedecode` over the wip pure-Go decoder; measured on real saves:
  0.43 s, 79 MB RSS, 3.8 KB roster; all fields populated, clean parse.
- Confirmed module boundary: `palsav` + `savegame` are self-contained (zero
  third-party deps); `saveroster` stays in the app.
- Artifacts in `.local/` (`out/`, `SPIKE-FINDINGS.md`).

### Phase 1 — Land the sidecar as its own module (DONE as a local module)

- **Done 2026-07-24:** extracted to `~/projects/palworld-save-reader` — a
  standalone Go 1.26 module (`palsav` + `savegame` + `cmd/savedecode`),
  git-initialised, tests green, GPL-3.0 with NOTICE/LICENSE. `cmd/savedecode`
  emits the roster JSON contract (and `--full` for inspection).
- **Hosting: local `replace` for now.** The main repo will `require` the module
  and point a `replace` at `../palworld-save-reader` (sibling checkout). No
  GitHub publish yet — revisit when CI/other machines need to build without the
  sibling checkout, then publish + tag and drop the `replace`. Consequence to
  document: the main repo does not build standalone without that sibling module.
- The Docker build needs the sidecar source in build context (COPY the sibling
  in, or a build arg) since it is not yet fetched from a remote.
- Risk: very low (separate binary, self-tested, not wired into the server).

### Phase 2 — Server-side wiring behind a default-off flag

- Add `internal/savesidecar` (first-party): snapshot the save tree read-only,
  exec the sidecar with a timeout, decode its roster JSON.
- Port `internal/saveroster` (first-party) to consume that JSON; add
  `NewPollerWithRoster`, client projection helpers, config env vars
  (`SAVE_DATA_ENABLED=false` default, `PALWORLD_SAVE_ROOT`, `SAVE_POLL_INTERVAL`,
  `SAVE_TIMEOUT`), `main.go` wiring (locate the sidecar next to the server binary).
- Extend Player JSON with `omitempty` fields (no client change required yet).
- Tests: table-driven against small committed roster-JSON fixtures (redacted).
- Risk: low (flag-gated; default behavior unchanged; subprocess is our own binary).

### Phase 3 — Frontend surfacing

- **Full field scope:** surface the core fields (name / guild / level / position)
  **and** `captureTotal`, `uniquePalsCaptured`, `paldeckUnlocked`.
- Extend `web/src/types.ts` + `DetailsDialog`; feature-detect optional fields so
  the UI degrades gracefully when a save omits them.
- Merge with the new filter-reveal `App.tsx` / `App.test.tsx`.
- Risk: low–medium (UI only).

### Phase 4 — Packaging & docs

- App image stays `static-debian12`; the sidecar is a second static Go binary in
  the same image (both no-cgo). Add read-only save mount to `compose.yml`,
  `.env.example`; refresh `README`, `DEVELOPMENT.md`, `docs/save-data.md`,
  `SECURITY.md`, `LICENSING` (server MIT; sidecar GPL-3.0, isolated).
- Risk: low (non-runtime).

---

## Verified facts (2026-07-24)

- `wip/save-game-refactor` @ `d600571`: `palsav` / `savegame` / `saveroster`
  tests pass; decodes real homelab `PlM1` saves to valid GVAS in-process, pure Go.
- Homelab saves (`Level.sav`, `LevelMeta.sav`, players) are all `PlM1` (Oodle).
  The permissive zlib-only path does not read them; the Oodle decoder is required.
- No standalone Oodle `.so` on the server host — statically linked in the UE
  server binary; nothing to mount for a proprietary-runtime approach.
- Rebase drift on `main` is orthogonal except `web/src/App.tsx` +
  `web/src/App.test.tsx` (filter-reveal feature) and a trivial server-test path.
