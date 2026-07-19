import type { MapItem, MapLayer } from '../types'

export const MAP_PIXEL_SIZE = 8192
export const MAX_ZOOM_RATIO = 96

export interface Point {
  x: number
  y: number
}

export interface View extends Point {
  scale: number
}

export function sceneSize(pixelRatio = window.devicePixelRatio || 1): number {
  return MAP_PIXEL_SIZE / Math.max(1, pixelRatio)
}

export function itemKey(item: MapItem): string {
  return JSON.stringify([item.kind, item.map, item.name, item.detail ?? '', item.x, item.y])
}

export function toScene(item: Pick<MapItem, 'x' | 'y'>, layer: MapLayer, size: number): Point | null {
  const [maxX, maxY, minX, minY] = layer.bounds
  if (item.x < minX || item.x > maxX || item.y < minY || item.y > maxY) return null
  return {
    x: ((item.y - minY) / (maxY - minY)) * size,
    y: ((maxX - item.x) / (maxX - minX)) * size
  }
}

export function toWorld(point: Point, layer: MapLayer, size: number): Point {
  const [maxX, maxY, minX, minY] = layer.bounds
  return {
    x: maxX - (point.y / size) * (maxX - minX),
    y: minY + (point.x / size) * (maxY - minY)
  }
}

export function fitScale(width: number, height: number, size: number): number {
  return Math.max(0.01, Math.min(width / size, height / size))
}

export function fitView(width: number, height: number, size: number): View {
  const scale = fitScale(width, height, size)
  return { scale, x: (width - size * scale) / 2, y: (height - size * scale) / 2 }
}

export function clampView(view: View, width: number, height: number, size: number): View {
  const scaledSize = size * view.scale
  return {
    scale: view.scale,
    x: scaledSize <= width ? (width - scaledSize) / 2 : Math.min(0, Math.max(width - scaledSize, view.x)),
    y: scaledSize <= height ? (height - scaledSize) / 2 : Math.min(0, Math.max(height - scaledSize, view.y))
  }
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const parts: string[] = []
  if (days) parts.push(`${days}d`)
  if (days || hours) parts.push(`${hours}h`)
  parts.push(`${minutes}m`)
  return parts.join(' ')
}

export function markerText(item: MapItem): string {
  if (item.kind === 'bases' || item.kind === 'npcs' || !item.level) return item.name
  return `${item.name} · Lv ${item.level}`
}

export function kindLabel(kind: MapItem['kind']): string {
  return (
    {
      players: 'Player',
      bases: 'Base',
      workers: 'Base worker',
      companions: 'Companion Pal',
      'wild-pals': 'Wild Pal',
      npcs: 'NPC'
    } as const
  )[kind]
}
