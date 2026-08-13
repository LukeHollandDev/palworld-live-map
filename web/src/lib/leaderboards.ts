import type { MapItem } from '../types'

export type LeaderboardId =
  | 'player-level'
  | 'total-captures'
  | 'paldeck-discoveries'
  | 'arena-rp'
  | 'fast-travel-points'
  | 'areas-discovered'
  | 'boss-clears'
  | 'tower-clears'

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

interface ScoredPlayer {
  item: MapItem
  score: number
  tieBreaker?: number
}

function comparePlayers(left: MapItem, right: MapItem) {
  return (
    left.name.localeCompare(right.name, 'en', { sensitivity: 'base' }) ||
    left.name.localeCompare(right.name, 'en') ||
    left.id.localeCompare(right.id, 'en')
  )
}

function compareOptionalNumbersDescending(left: number | undefined, right: number | undefined) {
  if (left === undefined) return right === undefined ? 0 : 1
  if (right === undefined) return -1
  return right - left
}

function playerMetricEntries(
  items: readonly MapItem[],
  scoreFor: (item: MapItem) => number | undefined,
  formatValue: (score: number, item: MapItem) => string,
  tieBreakerFor?: (item: MapItem) => number | undefined
): LeaderboardEntry[] {
  const scoredPlayers: ScoredPlayer[] = []

  for (const item of items) {
    if (item.kind !== 'players') continue
    const score = scoreFor(item)
    if (score === undefined) continue
    scoredPlayers.push({ item, score, tieBreaker: tieBreakerFor?.(item) })
  }

  return scoredPlayers
    .sort(
      (left, right) =>
        right.score - left.score ||
        compareOptionalNumbersDescending(left.tieBreaker, right.tieBreaker) ||
        comparePlayers(left.item, right.item)
    )
    .map(({ item, score }, index) => ({ item, rank: index + 1, value: formatValue(score, item) }))
}

function count(value: number) {
  return value.toLocaleString('en')
}

function countWithUnit(value: number, singular: string, plural = `${singular}s`) {
  return `${count(value)} ${value === 1 ? singular : plural}`
}

export const LEADERBOARDS: readonly LeaderboardDefinition[] = [
  {
    id: 'player-level',
    title: 'Player levels',
    description: 'All known players, ordered by their latest saved or live level.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.level,
        (level) => `Lv ${count(level)}`
      )
  },
  {
    id: 'total-captures',
    title: 'Total captures',
    description: 'Players ordered by the total number of Pals they have captured.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.captureTotal,
        (captures) => countWithUnit(captures, 'capture')
      )
  },
  {
    id: 'paldeck-discoveries',
    title: 'Paldeck discoveries',
    description: 'Players ordered by the number of Paldeck species they have discovered.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.paldeckUnlocked,
        (discovered) => `${count(discovered)} discovered`
      )
  },
  {
    id: 'arena-rp',
    title: 'Arena RP',
    description: 'Players ordered by their saved Arena ranking points.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.arenaRankPoints,
        (points) => `${count(points)} RP`
      )
  },
  {
    id: 'fast-travel-points',
    title: 'Fast-travel points',
    description: 'Players ordered by the number of fast-travel points they have unlocked.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.fastTravelUnlocked,
        (points) => `${count(points)} unlocked`
      )
  },
  {
    id: 'areas-discovered',
    title: 'Areas discovered',
    description: 'Players ordered by the number of map areas they have discovered.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.areasDiscovered,
        (areas) => countWithUnit(areas, 'area')
      )
  },
  {
    id: 'boss-clears',
    title: 'Boss clears',
    description: 'Players ordered by the number of different field bosses they have defeated.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.bossDefeats,
        (bosses) => `${count(bosses)} cleared`
      )
  },
  {
    id: 'tower-clears',
    title: 'Tower clears',
    description: 'Players ordered by the number of different tower fights they have cleared.',
    entries: (items) =>
      playerMetricEntries(
        items,
        (item) => item.towerDefeats,
        (towers) => `${count(towers)} cleared`
      )
  }
]

export function leaderboardById(id: LeaderboardId) {
  return LEADERBOARDS.find((leaderboard) => leaderboard.id === id) || LEADERBOARDS[0]
}
