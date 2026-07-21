import type { ItemKind } from '../types'

const DIAMOND_PATH = 'M10 3 17 10 10 17 3 10Z'
const CIRCLE_PATH = 'M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z'

const GLYPH_PATHS: Record<ItemKind, string> = {
  players: DIAMOND_PATH,
  bases: 'M4 3h12q1 0 1 1v12q0 1-1 1H4q-1 0-1-1V4q0-1 1-1Z',
  workers: DIAMOND_PATH,
  companions: CIRCLE_PATH,
  'wild-pals': CIRCLE_PATH,
  npcs: 'M10 3 17 17H3Z'
}

export function MarkerGlyph({ kind }: { kind: ItemKind }) {
  const path = GLYPH_PATHS[kind]
  return (
    <svg
      className={`marker-glyph kind-${kind}`}
      viewBox="0 0 20 20"
      data-marker-kind={kind}
      aria-hidden="true"
      focusable="false"
    >
      <path className="marker-glyph-outline" d={path} strokeWidth="6" strokeLinejoin="round" />
      <path className="marker-glyph-fill" d={path} strokeWidth="2" strokeLinejoin="round" />
    </svg>
  )
}
