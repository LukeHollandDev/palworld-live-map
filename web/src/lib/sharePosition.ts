import type { MapCameraPosition, MapLayer } from '../types'

const SHARE_KIND_PARAM = 'share'
const SHARE_KIND_VALUE = 'position'
const REGION_PARAM = 'region'
const X_PARAM = 'x'
const Y_PARAM = 'y'
const ZOOM_PARAM = 'zoom'

export const SHARE_POSITION_MIN_ZOOM = 8

export interface SharePositionResult {
  copied: boolean
  url: string
}

function finiteNumber(value: string | null): number | null {
  if (value === null || value.trim() === '') return null
  const number = Number(value)
  return Number.isFinite(number) ? number : null
}

function containsPosition(layer: MapLayer, x: number, y: number): boolean {
  const [maxX, maxY, minX, minY] = layer.bounds
  return x >= minX && x <= maxX && y >= minY && y <= maxY
}

export function parseSharedPosition(url: string | URL, layers: readonly MapLayer[]): MapCameraPosition | null {
  try {
    const parsed = new URL(url, window.location.href)
    if (parsed.searchParams.get(SHARE_KIND_PARAM) !== SHARE_KIND_VALUE) return null

    const region = parsed.searchParams.get(REGION_PARAM)
    const x = finiteNumber(parsed.searchParams.get(X_PARAM))
    const y = finiteNumber(parsed.searchParams.get(Y_PARAM))
    const zoom = finiteNumber(parsed.searchParams.get(ZOOM_PARAM))
    const layer = region ? layers.find((candidate) => candidate.id === region) : undefined
    if (!layer || x === null || y === null || zoom === null) return null
    if (!containsPosition(layer, x, y) || zoom < 1) return null
    return { region: layer.id, x, y, zoom }
  } catch {
    return null
  }
}

function compactNumber(value: number, decimals: number): string {
  return String(Number(value.toFixed(decimals)))
}

export function buildSharedPositionUrl(position: MapCameraPosition, currentUrl = window.location.href): string {
  const url = new URL(currentUrl)
  url.searchParams.set(SHARE_KIND_PARAM, SHARE_KIND_VALUE)
  url.searchParams.set(REGION_PARAM, position.region)
  url.searchParams.set(X_PARAM, compactNumber(position.x, 3))
  url.searchParams.set(Y_PARAM, compactNumber(position.y, 3))
  url.searchParams.set(ZOOM_PARAM, compactNumber(position.zoom, 4))
  url.hash = ''
  return url.toString()
}

function legacyCopy(text: string): boolean {
  if (typeof document.execCommand !== 'function') return false
  const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const input = document.createElement('textarea')
  input.value = text
  input.readOnly = true
  input.setAttribute('aria-hidden', 'true')
  input.style.position = 'fixed'
  input.style.opacity = '0'
  input.style.pointerEvents = 'none'
  document.body.append(input)
  input.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    input.remove()
    previousFocus?.focus({ preventScroll: true })
  }
}

export async function copySharedPositionUrl(url: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(url)
      return true
    }
  } catch {
    // Restricted contexts can expose the API while denying clipboard writes.
  }
  return legacyCopy(url)
}
