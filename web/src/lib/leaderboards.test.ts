import { describe, expect, it } from 'vitest'
import type { MapItem } from '../types'
import { leaderboardById } from './leaderboards'

describe('player level leaderboard', () => {
  it('includes offline and cross-map players with deterministic tie ordering', () => {
    const items: MapItem[] = [
      { id: 'z', kind: 'players', name: 'Zoe', level: 60, online: false, x: 0, y: 0, map: 'world-tree' },
      { id: 'b', kind: 'players', name: 'bob', level: 50, online: true, x: 0, y: 0, map: 'palpagos' },
      { id: 'a', kind: 'players', name: 'Alice', level: 50, online: false, x: 0, y: 0, map: 'palpagos' },
      { id: 'base', kind: 'bases', name: 'Home', x: 0, y: 0, map: 'palpagos' }
    ]

    const entries = leaderboardById('player-level').entries(items)

    expect(entries.map(({ item }) => item.id)).toEqual(['z', 'a', 'b'])
    expect(entries.map(({ rank }) => rank)).toEqual([1, 2, 3])
    expect(entries.map(({ value }) => value)).toEqual(['Lv 60', 'Lv 50', 'Lv 50'])
  })
})
