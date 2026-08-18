import { describe, expect, it } from 'vitest'
import type { MapItem } from '../types'
import {
  catalogueVersionFromURL,
  combineCompletionIDs,
  completionSource,
  completionSourceLabel,
  isSaveProgressStale,
  parseSaveProgress,
  saveCompletionIDs
} from './saveProgress'

const items: MapItem[] = [
  { id: 'waypoint-save', kind: 'waypoints', name: 'Waypoint', x: 1, y: 1, map: 'palpagos' },
  { id: 'journal-both', kind: 'journals', name: 'Journal', x: 2, y: 2, map: 'palpagos' },
  { id: 'effigy-manual', kind: 'effigies', name: 'Effigy', x: 3, y: 3, map: 'palpagos' }
]

function payload(snapshotAt = '2026-08-15T10:00:00Z') {
  return {
    snapshotAt,
    catalogueVersion: 'catalogue-content-hash',
    domains: [
      { id: 'alpha-pals', coverage: 'complete', completedIds: [], total: 0 },
      { id: 'bosses', coverage: 'complete', completedIds: [], total: 0 },
      { id: 'bounties', coverage: 'complete', completedIds: [], total: 0 },
      { id: 'watchtowers', coverage: 'complete', completedIds: [], total: 0 },
      { id: 'waypoints', coverage: 'complete', completedIds: ['waypoint-save'], total: 1 },
      { id: 'effigies', coverage: 'complete', completedIds: [], total: 0 },
      { id: 'journals', coverage: 'complete', completedIds: ['journal-both'], total: 1 },
      { id: 'ancient-shrine-pickups', coverage: 'complete', completedIds: [], total: 0 }
    ]
  }
}

describe('save-backed completion', () => {
  it('unions transient save completion with durable manual completion and labels each source', () => {
    const snapshot = parseSaveProgress(payload())
    const save = saveCompletionIDs(snapshot, items)
    const manual = new Set(['journal-both', 'effigy-manual'])
    const effective = combineCompletionIDs(manual, save)

    expect(effective).toEqual(new Set(['journal-both', 'effigy-manual', 'waypoint-save']))
    expect(completionSourceLabel(completionSource('waypoint-save', manual, save))).toBe('Save-confirmed')
    expect(completionSourceLabel(completionSource('effigy-manual', manual, save))).toBe('Manual')
    expect(completionSourceLabel(completionSource('journal-both', manual, save))).toBe('Combined')
  })

  it('accepts exactly the complete save-backed landmark domains and rejects everything else', () => {
    expect(parseSaveProgress(payload())?.domains.map((domain) => domain.id)).toEqual([
      'alpha-pals',
      'bosses',
      'bounties',
      'watchtowers',
      'waypoints',
      'effigies',
      'journals',
      'ancient-shrine-pickups'
    ])

    const unknown = payload()
    unknown.domains[0].id = 'npc-locations'
    expect(parseSaveProgress(unknown)).toBeNull()

    const missing = payload()
    missing.domains.pop()
    expect(parseSaveProgress(missing)).toBeNull()

    const partial = payload()
    partial.domains[0].coverage = 'partial'
    expect(parseSaveProgress(partial)).toBeNull()

    const duplicate = payload()
    duplicate.domains[1].id = 'alpha-pals'
    expect(parseSaveProgress(duplicate)).toBeNull()
  })

  it('does not let a response domain complete an item from another catalogue kind', () => {
    const crossDomain = payload()
    const waypointDomain = crossDomain.domains.find((domain) => domain.id === 'waypoints')
    if (!waypointDomain) throw new Error('Expected waypoint test domain')
    waypointDomain.completedIds = ['effigy-manual']
    expect(saveCompletionIDs(parseSaveProgress(crossDomain), items)).toEqual(new Set(['journal-both']))
  })

  it('reports freshness without discarding a stale but valid snapshot', () => {
    const snapshot = parseSaveProgress(payload())
    if (!snapshot) throw new Error('Expected a valid save-progress fixture')
    expect(isSaveProgressStale(snapshot, Date.parse('2026-08-15T10:29:59Z'))).toBe(false)
    expect(isSaveProgressStale(snapshot, Date.parse('2026-08-15T10:30:01Z'))).toBe(true)
  })

  it('extracts the exact catalogue content hash from the configured URL', () => {
    expect(catalogueVersionFromURL('/api/catalogue?v=abc123', 'https://map.test/')).toBe('abc123')
    expect(catalogueVersionFromURL('/api/catalogue', 'https://map.test/')).toBeNull()
    expect(catalogueVersionFromURL('/api/catalogue?v=%20abc%20', 'https://map.test/')).toBeNull()
    expect(catalogueVersionFromURL('not a valid URL', 'not a base')).toBeNull()
  })
})
