import { describe, expect, it } from 'vitest'
import type { MapLayer } from '../types'
import { clampView, coverScale, coverView, fitScale, fitView, formatUptime, itemKey, toScene, toWorld } from './map'

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
  it('fits and centres the scene in the viewport', () => {
    expect(fitScale(1200, 800, 1000)).toBe(0.8)
    expect(fitView(1200, 800, 1000)).toEqual({ scale: 0.8, x: 200, y: 0 })
  })

  it('covers and centres a map without viewport gutters', () => {
    expect(coverScale(1200, 800, 1000)).toBe(1.2)
    expect(coverView(1200, 800, 1000)).toEqual({ scale: 1.2, x: 0, y: -200 })
  })

  it('clamps a zoomed scene to the viewport edges', () => {
    expect(clampView({ scale: 2, x: 50, y: -1500 }, 500, 500, 1000)).toEqual({ scale: 2, x: 0, y: -1500 })
  })
})

describe('map display helpers', () => {
  it('builds stable item keys from public fields', () => {
    const item = { kind: 'players' as const, map: 'palpagos', name: 'Luke', detail: 'Level 55', x: 10, y: 20 }
    expect(itemKey(item)).toBe(itemKey({ ...item }))
  })

  it('formats server uptime', () => {
    expect(formatUptime(90)).toBe('1m')
    expect(formatUptime(90061)).toBe('1d 1h 1m')
  })
})
