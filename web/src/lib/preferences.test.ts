import { afterEach, describe, expect, it } from 'vitest'
import type { ItemKind } from '../types'
import { DEFAULT_ENABLED_PLAYER_STATUSES, loadFilterPreferences, saveFilterPreferences } from './preferences'

const WORLD_CATALOGUE_KINDS: ItemKind[] = [
  'bounties',
  'oil-rigs',
  'watchtowers',
  'waypoints',
  'dungeon-entrances',
  'effigies',
  'journals',
  'ancient-shrine-pickups',
  'npc-locations'
]

afterEach(() => window.localStorage.clear())

describe('filter preferences', () => {
  it('enables only players and guild content by default', () => {
    expect(loadFilterPreferences().enabledKinds).toEqual(new Set(['players', 'bases', 'workers']))
    expect(loadFilterPreferences().enabledPlayerStatuses).toEqual(new Set(DEFAULT_ENABLED_PLAYER_STATUSES))
  })

  it('enables newly introduced landmark and world-catalogue kinds in legacy preferences', () => {
    window.localStorage.setItem(
      'palworld-live-map.filters.v1',
      JSON.stringify({ activeLayerId: 'palpagos', enabledKinds: ['players'], hiddenIds: [] })
    )

    expect(loadFilterPreferences().enabledKinds).toEqual(
      new Set(['players', 'alpha-pals', 'bosses', ...WORLD_CATALOGUE_KINDS])
    )
  })

  it('adds world-catalogue kinds without restoring categories hidden in current preferences', () => {
    window.localStorage.setItem(
      'palworld-live-map.filters.v1',
      JSON.stringify({ kindsVersion: 3, enabledKinds: ['players'], hiddenIds: [] })
    )

    expect(loadFilterPreferences().enabledKinds).toEqual(new Set(['players', ...WORLD_CATALOGUE_KINDS]))
  })

  it('preserves an explicit choice to hide landmark kinds after migration', () => {
    saveFilterPreferences({
      activeLayerId: 'palpagos',
      enabledKinds: new Set(['players']),
      enabledPlayerStatuses: new Set(['online']),
      hiddenIds: new Set()
    })

    expect(loadFilterPreferences().enabledKinds).toEqual(new Set(['players']))
    expect(loadFilterPreferences().enabledPlayerStatuses).toEqual(new Set(['online']))
  })

  it('migrates the previous all-enabled default and retires standalone ambient categories', () => {
    window.localStorage.setItem(
      'palworld-live-map.filters.v1',
      JSON.stringify({
        kindsVersion: 2,
        enabledKinds: ['players', 'bases', 'workers', 'companions', 'wild-pals', 'alpha-pals', 'bosses', 'npcs'],
        hiddenIds: []
      })
    )
    expect(loadFilterPreferences().enabledKinds).toEqual(new Set(['players', 'bases', 'workers']))

    saveFilterPreferences({
      activeLayerId: 'palpagos',
      enabledKinds: new Set(['players', 'bases', 'workers', 'companions', 'wild-pals', 'alpha-pals', 'bosses', 'npcs']),
      enabledPlayerStatuses: new Set(['online', 'offline']),
      hiddenIds: new Set()
    })
    expect(loadFilterPreferences().enabledKinds).toEqual(
      new Set(['players', 'bases', 'workers', 'alpha-pals', 'bosses'])
    )
  })

  it('round-trips an explicit uncheck-all choice', () => {
    saveFilterPreferences({
      activeLayerId: 'palpagos',
      enabledKinds: new Set(),
      enabledPlayerStatuses: new Set(),
      hiddenIds: new Set(),
      seenKinds: new Set<ItemKind>(['players', ...WORLD_CATALOGUE_KINDS])
    })

    expect(loadFilterPreferences().enabledKinds).toEqual(new Set())
    expect(loadFilterPreferences().enabledPlayerStatuses).toEqual(new Set())
  })
})
