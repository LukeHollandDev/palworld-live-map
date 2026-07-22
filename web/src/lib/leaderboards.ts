import type { MapItem } from '../types'

export type LeaderboardId = 'player-level'

export interface LeaderboardEntry {
  item: MapItem
  rank: number
  value: string
}

export interface LeaderboardDefinition {
  id: LeaderboardId
  title: string
  description: string
  entries: (items: readonly MapItem[]) => LeaderboardEntry[]
}

function comparePlayerLevels(left: MapItem, right: MapItem) {
  return (
    (right.level || 0) - (left.level || 0) ||
    left.name.localeCompare(right.name, 'en', { sensitivity: 'base' }) ||
    left.name.localeCompare(right.name, 'en') ||
    left.id.localeCompare(right.id, 'en')
  )
}

export const LEADERBOARDS: readonly LeaderboardDefinition[] = [
  {
    id: 'player-level',
    title: 'Player levels',
    description: 'All known players, ordered by their latest saved or live level.',
    entries: (items) =>
      items
        .filter((item) => item.kind === 'players')
        .slice()
        .sort(comparePlayerLevels)
        .map((item, index) => ({ item, rank: index + 1, value: `Lv ${item.level || 0}` }))
  }
]

export function leaderboardById(id: LeaderboardId) {
  return LEADERBOARDS.find((leaderboard) => leaderboard.id === id) || LEADERBOARDS[0]
}
