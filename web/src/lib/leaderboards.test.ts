import { describe, expect, it } from 'vitest'
import type { MapItem } from '../types'
import { LEADERBOARDS, type LeaderboardId, leaderboardById } from './leaderboards'

function player(id: string, name: string, metrics: Partial<MapItem> = {}): MapItem {
  return { id, kind: 'players', name, online: false, x: 0, y: 0, map: 'palpagos', ...metrics }
}

describe('leaderboards', () => {
  it('offers all supported leaderboard types in display order', () => {
    expect(LEADERBOARDS.map(({ id }) => id)).toEqual([
      'player-level',
      'total-captures',
      'paldeck-discoveries',
      'arena-rp',
      'fast-travel-points',
      'areas-discovered',
      'boss-clears',
      'tower-clears'
    ])
  })

  it('includes offline and cross-map players in the level board with deterministic tie ordering', () => {
    const items: MapItem[] = [
      player('z', 'Zoe', { level: 60, map: 'world-tree' }),
      player('b', 'bob', { level: 50, online: true }),
      player('a', 'Alice', { level: 50 }),
      player('zero', 'New player', { level: 0 }),
      player('missing', 'Unknown'),
      { id: 'base', kind: 'bases', name: 'Home', x: 0, y: 0, map: 'palpagos' }
    ]

    const entries = leaderboardById('player-level').entries(items)

    expect(entries.map(({ item }) => item.id)).toEqual(['z', 'a', 'b', 'zero'])
    expect(entries.map(({ rank }) => rank)).toEqual([1, 2, 3, 4])
    expect(entries.map(({ value }) => value)).toEqual(['Lv 60', 'Lv 50', 'Lv 50', 'Lv 0'])
  })

  it('ranks total captures while retaining a real zero and omitting unavailable values', () => {
    const entries = leaderboardById('total-captures').entries([
      player('missing', 'Missing'),
      player('zero', 'Zero', { captureTotal: 0 }),
      player('one', 'One', { captureTotal: 1 }),
      player('winner', 'Winner', { captureTotal: 1234 })
    ])

    expect(entries.map(({ item, value }) => [item.id, value])).toEqual([
      ['winner', '1,234 captures'],
      ['one', '1 capture'],
      ['zero', '0 captures']
    ])
  })

  it('shows only Paldeck discoveries and does not use captured species to break ties', () => {
    const entries = leaderboardById('paldeck-discoveries').entries([
      player('lower-captures', 'Amy', { paldeckUnlocked: 150, uniquePalsCaptured: 80 }),
      player('higher-captures', 'Zed', { paldeckUnlocked: 150, uniquePalsCaptured: 90 }),
      player('discoveries-only', 'Bob', { paldeckUnlocked: 149 }),
      player('missing', 'Missing', { uniquePalsCaptured: 120 })
    ])

    expect(entries.map(({ item, value }) => [item.id, value])).toEqual([
      ['lower-captures', '150 discovered'],
      ['higher-captures', '150 discovered'],
      ['discoveries-only', '149 discovered']
    ])
  })

  it.each<{
    id: LeaderboardId
    metric: 'arenaRankPoints' | 'fastTravelUnlocked' | 'areasDiscovered' | 'bossDefeats' | 'towerDefeats'
    value: string
  }>([
    { id: 'arena-rp', metric: 'arenaRankPoints', value: '1,234 RP' },
    { id: 'fast-travel-points', metric: 'fastTravelUnlocked', value: '1,234 unlocked' },
    { id: 'areas-discovered', metric: 'areasDiscovered', value: '1,234 areas' },
    { id: 'boss-clears', metric: 'bossDefeats', value: '1,234 cleared' },
    { id: 'tower-clears', metric: 'towerDefeats', value: '1,234 cleared' }
  ])('ranks $id values and excludes players without that save metric', ({ id, metric, value }) => {
    const entries = leaderboardById(id).entries([
      player('beta', 'Beta', { [metric]: 1234 }),
      player('missing', 'Missing'),
      player('alpha', 'Alpha', { [metric]: 1234 }),
      player('zero', 'Zero', { [metric]: 0 })
    ])

    expect(entries.map(({ item, value: formatted }) => [item.id, formatted])).toEqual([
      ['alpha', value],
      ['beta', value],
      ['zero', value.replace('1,234', '0')]
    ])
  })

  it('uses singular units for a single discovered area', () => {
    const [entry] = leaderboardById('areas-discovered').entries([player('one', 'One', { areasDiscovered: 1 })])
    expect(entry.value).toBe('1 area')
  })
})
