import { describe, expect, it } from 'vitest'
import type { MapLayer } from '../types'
import {
  buildSpatialGrid,
  clampView,
  coverScale,
  coverView,
  formatUptime,
  isScenePointVisible,
  itemSearchText,
  kindLabel,
  markerStackOrder,
  querySpatialGrid,
  sceneViewportBounds,
  selectMapTileLevel,
  selectVisibleMapTiles,
  toScene,
  toWorld
} from './map'

const layer: MapLayer = {
  id: 'palpagos',
  name: 'Palpagos Islands',
  bounds: [100, 200, -100, -200]
}

describe('map coordinates', () => {
  it('round trips between world and scene coordinates', () => {
    const scene = toScene({ x: 25, y: -50 }, layer, 1000)
    if (!scene) throw new Error('expected point within map bounds')
    expect(toWorld(scene, layer, 1000)).toEqual({ x: 25, y: -50 })
  })

  it('rejects points beyond the layer bounds', () => {
    expect(toScene({ x: 101, y: 0 }, layer, 1000)).toBeNull()
    expect(toScene({ x: 0, y: -201 }, layer, 1000)).toBeNull()
  })
})

describe('map view', () => {
  it('covers and centres a map without viewport gutters', () => {
    expect(coverScale(1200, 800, 1000)).toBe(1.2)
    expect(coverView(1200, 800, 1000)).toEqual({ scale: 1.2, x: 0, y: -200 })
  })

  it('clamps a zoomed scene to the viewport edges', () => {
    expect(clampView({ scale: 2, x: 50, y: -1500 }, 500, 500, 1000)).toEqual({ scale: 2, x: 0, y: -1500 })
  })

  it('computes scene bounds with screen-space overscan for marker culling', () => {
    const bounds = sceneViewportBounds({ scale: 2, x: -200, y: -100 }, 400, 300, 40)
    expect(bounds).toEqual({ left: 80, right: 320, top: 30, bottom: 220 })
    expect(isScenePointVisible({ x: 80, y: 220 }, bounds)).toBe(true)
    expect(isScenePointVisible({ x: 79, y: 100 }, bounds)).toBe(false)
  })
})

describe('map tile selection', () => {
  const pyramid = {
    tileSize: 512,
    levels: [1024, 2048, 4096, 8192],
    urlTemplate: '/assets/map/palpagos-z{size}-x{x}-y{y}.webp?v=version'
  }

  it('selects the smallest level that covers rendered CSS pixels at the device pixel ratio', () => {
    expect(selectMapTileLevel(pyramid.levels, 0.125, 1, 8192)).toBe(1024)
    expect(selectMapTileLevel(pyramid.levels, 0.126, 1, 8192)).toBe(2048)
    expect(selectMapTileLevel(pyramid.levels, 0.25, 2, 8192)).toBe(4096)
    expect(selectMapTileLevel(pyramid.levels, 4, 2, 8192)).toBe(8192)
  })

  it('requests only visible tiles plus a one-tile overscan with stable versioned URLs', () => {
    const selection = selectVisibleMapTiles(pyramid, { scale: 2, x: -7000, y: -7600 }, 1200, 600, 8192, 1)

    expect(selection.level).toBe(8192)
    expect(selection.tiles).toHaveLength(20)
    expect(selection.tiles.map(({ key }) => key)).toEqual([
      '8192:5:6',
      '8192:6:6',
      '8192:7:6',
      '8192:8:6',
      '8192:9:6',
      '8192:5:7',
      '8192:6:7',
      '8192:7:7',
      '8192:8:7',
      '8192:9:7',
      '8192:5:8',
      '8192:6:8',
      '8192:7:8',
      '8192:8:8',
      '8192:9:8',
      '8192:5:9',
      '8192:6:9',
      '8192:7:9',
      '8192:8:9',
      '8192:9:9'
    ])
    expect(selection.tiles[0].url).toBe('/assets/map/palpagos-z8192-x5-y6.webp?v=version')
    expect(selection.signature).toContain('|8192|5:6:9:9')
  })
})

describe('map spatial grid', () => {
  const entries = [
    { value: 'northwest', position: { x: 10, y: 10 } },
    { value: 'centre', position: { x: 300, y: 300 } },
    { value: 'southeast', position: { x: 790, y: 790 } }
  ]

  it('conserves every projected item when querying the full scene', () => {
    const grid = buildSpatialGrid(entries, 256)
    expect(grid.count).toBe(entries.length)
    expect(querySpatialGrid(grid, { left: 0, right: 800, top: 0, bottom: 800 })).toEqual(entries)
  })

  it('queries only intersecting cells and still applies exact viewport bounds', () => {
    const grid = buildSpatialGrid(entries, 256)
    expect(querySpatialGrid(grid, { left: 256, right: 511, top: 256, bottom: 511 })).toEqual([entries[1]])
    expect(querySpatialGrid(grid, { left: 301, right: 511, top: 301, bottom: 511 })).toEqual([])
  })
})

describe('map display helpers', () => {
  it('formats server uptime', () => {
    expect(formatUptime(90)).toBe('1m')
    expect(formatUptime(90061)).toBe('1d 1h 1m')
  })

  it('stacks important markers above related and ambient markers', () => {
    const ascendingKinds = [
      'wild-pals',
      'dungeon-entrances',
      'effigies',
      'journals',
      'ancient-shrine-pickups',
      'npc-locations',
      'npcs',
      'workers',
      'companions',
      'waypoints',
      'watchtowers',
      'alpha-pals',
      'bounties',
      'bosses',
      'oil-rigs',
      'bases',
      'players'
    ] as const
    const stack = ascendingKinds.map(markerStackOrder)

    expect(stack).toEqual([...stack].sort((left, right) => left - right))
    expect(new Set(stack).size).toBe(ascendingKinds.length)
  })

  it('labels and searches static NPC locations separately from live NPCs', () => {
    const location = {
      id: 'npc-location',
      kind: 'npc-locations',
      name: 'Merchant',
      x: 0,
      y: 0,
      map: 'palpagos'
    } as const
    const liveNpc = { ...location, id: 'live-npc', kind: 'npcs' as const }

    expect(kindLabel(location.kind)).toBe('NPC location')
    expect(kindLabel(liveNpc.kind)).toBe('NPC')
    expect(itemSearchText(location)).toContain('static npc')
    expect(itemSearchText(liveNpc)).toContain('live npc')
    expect(itemSearchText(liveNpc)).not.toContain('static npc')
  })
})
