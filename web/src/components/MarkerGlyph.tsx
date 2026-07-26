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
  bounties: 'M10 2.5 16 5.5v4.8c0 3.5-2.4 5.8-6 7.2-3.6-1.4-6-3.7-6-7.2V5.5Z',
  'oil-rigs': 'M4 17V8h3V4h6v4h3v9H4Zm5-9h2V6H9Z',
  watchtowers: 'M7 3h6l-1 4 3 10H5L8 7Z',
  waypoints: 'M10 2.5A5.5 5.5 0 0 1 15.5 8c0 4-5.5 9.5-5.5 9.5S4.5 12 4.5 8A5.5 5.5 0 0 1 10 2.5Z',
  'dungeon-entrances': 'M4 17V9a6 6 0 0 1 12 0v8h-4V9a2 2 0 0 0-4 0v8Z',
  effigies: 'M10 2.5 13 6l-1.5 3 3.5 8H5l3.5-8L7 6Z',
  journals: 'M3 4.5q3.5-1.5 7 1.5v11q-3.5-3-7-1.5Zm14 0q-3.5-1.5-7 1.5v11q3.5-3 7-1.5Z',
  'ancient-shrine-pickups': 'M10 2 12 7.5 18 10 12 12.5 10 18 8 12.5 2 10 8 7.5Z',
  'npc-locations': 'M10 2.5 17 6.5v7L10 17.5 3 13.5v-7Z',
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
