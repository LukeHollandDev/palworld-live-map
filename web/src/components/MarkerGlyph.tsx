import type { ItemKind } from '../types'

const DIAMOND_PATH = 'M10 3 17 10 10 17 3 10Z'
const CIRCLE_PATH = 'M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Z'

const GLYPH_PATHS: Record<ItemKind, string> = {
  players: DIAMOND_PATH,
  bases: 'M4 3h12q1 0 1 1v12q0 1-1 1H4q-1 0-1-1V4q0-1 1-1Z',
  workers: DIAMOND_PATH,
  companions: CIRCLE_PATH,
  'wild-pals': CIRCLE_PATH,
  'alpha-pals': 'M10 2.5 12.1 6.5 16.7 5.5 14.5 9.6 17.5 13 12.9 12.2 10 17 7.1 12.2 2.5 13 5.5 9.6 3.3 5.5 7.9 6.5Z',
  bosses: 'M10 2.5 16.5 5.2 15.5 13.4 10 17.5 4.5 13.4 3.5 5.2Z',
  npcs: 'M10 3 17 17H3Z'
}

export function MarkerGlyph({ kind, online }: { kind: ItemKind; online?: boolean }) {
  const path = GLYPH_PATHS[kind]
  const playerStatus =
    kind === 'players' ? (online === false ? 'offline' : online === true ? 'online' : undefined) : undefined
  return (
    <svg
      className={`marker-glyph kind-${kind}${playerStatus ? ` player-${playerStatus}` : ''}`}
      viewBox="0 0 20 20"
      data-marker-kind={kind}
      data-player-status={playerStatus}
      aria-hidden="true"
      focusable="false"
    >
      <path className="marker-glyph-outline" d={path} strokeWidth="6" strokeLinejoin="round" />
      <path className="marker-glyph-fill" d={path} strokeWidth="2" strokeLinejoin="round" />
    </svg>
  )
}
