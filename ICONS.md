# Tabler Icons migration plan

## Decision

Standardise Palworld Live Map on [Tabler Icons](https://tabler.io/icons).

Tabler is the preferred library because its MIT-licensed set provides the strongest semantic coverage for the map’s specialist categories while matching the interface’s crisp, technical visual style.

The comparison and selected replacements are documented in [`docs/icon-library-comparison.html`](docs/icon-library-comparison.html).

## Goals

- Replace the current mixture of custom paths, Heroicons, Octicons, hand-drawn SVGs, and text glyphs with one consistent icon system.
- Give every existing icon role a direct Tabler replacement.
- Preserve category colours, online/offline states, accessible names, interaction behaviour, and focus handling.
- Keep map markers legible over detailed map artwork at their current 20px size.
- Import only the icons used by the application.

## Non-goals

- Do not redesign the surrounding controls, panels, marker labels, or map interactions.
- Do not change category names, filter behaviour, marker stacking, or saved preferences.
- Do not replace the project favicon or Palworld-specific branding.
- Do not use Iconify in production; it is only used by the comparison document.

## Proposed dependency

Add `@tabler/icons-react` to `web/package.json`.

Use named imports from the package so Vite can tree-shake unused icons:

```tsx
import { IconSearch, IconX } from '@tabler/icons-react'
```

Use `aria-hidden="true"` for decorative icons. The containing button, link, or labelled control remains responsible for its accessible name.

## Map marker mapping

| Current category | `ItemKind` | Tabler replacement |
| --- | --- | --- |
| Player | `players` | `IconUser` |
| Base | `bases` | `IconBuildingWarehouse` |
| Base worker | `workers` | `IconHammer` |
| Companion Pal | `companions` | `IconHeartHandshake` |
| Wild Pal | `wild-pals` | `IconPaw` |
| Alpha Pal | `alpha-pals` | `IconCrown` |
| Tower boss | `bosses` | `IconSkull` |
| Bounty | `bounties` | `IconTarget` |
| Oil rig | `oil-rigs` | `IconBuildingFactory2` |
| Watchtower | `watchtowers` | `IconTower` |
| Waypoint | `waypoints` | `IconMapPin` |
| Dungeon entrance | `dungeon-entrances` | `IconBuildingArch` |
| Pal Effigy | `effigies` | `IconBuildingMonument` |
| Journal | `journals` | `IconNotebook` |
| Ancient Shrine pickup | `ancient-shrine-pickups` | `IconSparkle` |
| NPC location | `npc-locations` | `IconUserPin` |
| NPC | `npcs` | `IconMessage2` |

## Interface icon mapping

The map footer leaderboard control intentionally uses the uppercase
`LEADERBOARDS` text label rather than an icon.

| Current role | Tabler replacement |
| --- | --- |
| Open filters | `IconFilter` |
| Search | `IconSearch` |
| Close details | `IconX` |
| Collapse filters | `IconX` |
| Clear search | `IconX` |
| Expand section | `IconChevronRight` |
| Map coordinates | `IconCrosshair` |
| Zoom out | `IconMinus` |
| Zoom in | `IconPlus` |
| GitHub | `IconBrandGithub` |

## Marker implementation

Retain `MarkerGlyph` as the shared marker abstraction so callers and accessibility contracts do not change.

Replace `GLYPH_PATHS` with an `ItemKind` to Tabler component mapping. `MarkerGlyph` should continue to provide:

- The `marker-glyph` and `kind-*` classes.
- `data-marker-kind`.
- `data-player-status` for players.
- `aria-hidden="true"` and `focusable="false"`.
- The existing category and online/offline colour selection.

Tabler icons are outlined rather than filled shapes. Keep them visually open
without a circular backing shape. Preserve map contrast with a subtle CSS
drop-shadow around the icon strokes.

Recommended initial marker settings:

- Icon size: 18–20px inside the existing 20px footprint.
- Stroke width: start at Tabler’s default `2`; test `2.25` if the 20px view is too light.
- Stroke colour: `var(--marker-glyph-color)`.
- Contrast: subtle CSS drop-shadow only; no background circle.
- Selected/hover state: preserve the existing filter/drop-shadow behaviour.

Do not pass category colours as inline component properties unless necessary. Keeping colour ownership in `index.css` preserves the existing theme contract.

## Implementation phases

### 1. Add the library

- Add `@tabler/icons-react` to `web/package.json`.
- Regenerate `web/package-lock.json`.
- Record the MIT licence and required notice in the project’s third-party licence documentation, creating one if none exists.
- Confirm the production bundle contains only imported icons.

### 2. Migrate map markers

- Replace the custom path table in `web/src/components/MarkerGlyph.tsx`.
- Update the marker styles in `web/src/index.css` for outlined icons and a
  subtle non-circular contrast shadow.
- Keep the public `MarkerGlyph` props and DOM data attributes stable.
- Check every category at 20px on both map layers and in the Explorer list.
- Check online and offline player colours independently.

### 3. Migrate interface controls

- Keep the map footer leaderboard control as the uppercase `LEADERBOARDS` text
  label; do not assign it an icon.
- Replace filter, search, close, clear, and chevron artwork in `web/src/components/Explorer.tsx`.
- Replace coordinate, zoom-in, and zoom-out artwork in `web/src/components/MapViewport.tsx`.
- Replace the details close glyph in `web/src/components/DetailsDialog.tsx`.
- Replace the GitHub SVG in `web/src/components/ProjectLinks.tsx`.
- Remove obsolete icon-source comments after their artwork is removed.

### 4. Update tests

- Update `web/src/components/MarkerGlyph.test.tsx` to test the component contract rather than custom SVG path data.
- Continue testing that all `ItemKind` values render.
- Test distinct Tabler components for categories that must not be visually conflated.
- Preserve player online/offline state assertions.
- Update component tests that assert literal text glyphs or SVG structure.
- Avoid snapshots of Tabler’s internal SVG paths; those would couple tests to the dependency’s implementation.

### 5. Visual and accessibility verification

- Compare all markers at 20px, 24px, and selected-map scale.
- Test over light, dark, and visually busy areas of both maps.
- Confirm similar categories remain distinguishable:
  - Player, NPC, and NPC location.
  - Base, worker, and companion.
  - Watchtower and waypoint.
  - Alpha Pal, tower boss, and bounty.
  - Pal Effigy and Ancient Shrine pickup.
- Confirm controls remain understandable without tooltips.
- Confirm every icon-only button retains an accessible name.
- Confirm decorative SVGs are ignored by assistive technology.
- Confirm focus rings, hover states, reduced motion, and high zoom remain intact.

### 6. Final cleanup

- Run formatting, linting, type checking, unit tests, and the production build.
- Inspect the production bundle for accidental whole-library inclusion.
- Remove dead custom SVG CSS and path constants.
- Keep the comparison HTML as the design record unless it becomes misleading after later icon changes.

## Expected files

- `web/package.json`
- `web/package-lock.json`
- `web/src/components/DetailsDialog.tsx`
- `web/src/components/Explorer.tsx`
- `web/src/components/MapViewport.tsx`
- `web/src/components/MarkerGlyph.tsx`
- `web/src/components/MarkerGlyph.test.tsx`
- `web/src/components/ProjectLinks.tsx`
- `web/src/index.css`
- Any tests that currently assert the replaced SVG or text-glyph structure
- Third-party licence/notice file, if required

## Acceptance criteria

- Every icon listed above uses its selected Tabler replacement.
- No Heroicons, Octicons, hand-drawn control SVGs, or `×`, `−`, and `+` icon glyphs remain in application components.
- No production Iconify dependency or network icon loading is introduced.
- Every `ItemKind` renders a distinct, coloured marker.
- Markers remain readable over both supported map layers at the normal zoom level.
- Existing interaction, accessibility, filter, selection, and player-status behaviour is unchanged.
- The full web check, test, and build commands pass.
- The dependency is tree-shaken and its MIT licence obligations are documented.

## Product checkpoint

Before merging, review the implemented 20px markers in the real map rather than approving them only from the comparison page. Pay particular attention to `companions`, `ancient-shrine-pickups`, and `npcs`, where the selected icons are semantic representations rather than literal Palworld landmarks.
