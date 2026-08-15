import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MapLayer } from '../types'
import { buildSharedPositionUrl, copySharedPositionUrl, parseSharedPosition } from './sharePosition'

const layers: MapLayer[] = [
  { id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] },
  { id: 'world-tree', name: 'World Tree', bounds: [400, 500, 200, 300] }
]

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('shared positions', () => {
  it('builds a fixed, compact URL without item identity and reads it back', () => {
    const url = buildSharedPositionUrl(
      { region: 'palpagos', x: 10.12349, y: -20.56789, zoom: 8.123456 },
      'https://map.example.test/live?theme=dark#players'
    )

    expect(url).toBe(
      'https://map.example.test/live?theme=dark&share=position&region=palpagos&x=10.123&y=-20.568&zoom=8.1235'
    )
    expect(url).not.toMatch(/player|item|id=/)
    expect(parseSharedPosition(url, layers)).toEqual({
      region: 'palpagos',
      x: 10.123,
      y: -20.568,
      zoom: 8.1235
    })
  })

  it.each([
    'https://map.example.test/?region=palpagos&x=1&y=2&zoom=8',
    'https://map.example.test/?share=position&region=missing&x=1&y=2&zoom=8',
    'https://map.example.test/?share=position&region=palpagos&x=101&y=2&zoom=8',
    'https://map.example.test/?share=position&region=palpagos&x=1&y=2&zoom=0',
    'https://map.example.test/?share=position&region=palpagos&x=nope&y=2&zoom=8'
  ])('ignores invalid or unrelated URLs: %s', (url) => {
    expect(parseSharedPosition(url, layers)).toBeNull()
  })

  it('accepts large finite zoom ratios so the viewport can clamp them for the receiving screen', () => {
    expect(
      parseSharedPosition('https://map.example.test/?share=position&region=palpagos&x=1&y=2&zoom=320', layers)
    ).toEqual({ region: 'palpagos', x: 1, y: 2, zoom: 320 })
  })

  it('uses the Clipboard API when it is available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })

    await expect(copySharedPositionUrl('https://map.example.test/fixed')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('https://map.example.test/fixed')
  })

  it('falls back to a temporary selection when Clipboard API access is denied', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
    const execCommand = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand })

    await expect(copySharedPositionUrl('https://map.example.test/fixed')).resolves.toBe(true)
    expect(execCommand).toHaveBeenCalledWith('copy')
    expect(document.querySelector('textarea')).not.toBeInTheDocument()
  })

  it('reports failure so the UI can expose the URL for manual copying', async () => {
    vi.stubGlobal('navigator', { ...navigator, clipboard: undefined })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: undefined })

    await expect(copySharedPositionUrl('https://map.example.test/fixed')).resolves.toBe(false)
  })
})
