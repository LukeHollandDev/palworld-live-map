import type { ItemKind, MapItem, MapLayer } from '../types'

export const MAP_PIXEL_SIZE = 8192
export const MAX_ZOOM_RATIO = 96

const MARKER_STACK_ORDER: Record<ItemKind, number> = {
  'wild-pals': 10,
  'dungeon-entrances': 15,
  effigies: 20,
  journals: 25,
  'ancient-shrine-pickups': 30,
  'npc-locations': 35,
  npcs: 40,
  workers: 45,
  companions: 50,
  waypoints: 55,
  watchtowers: 60,
  'alpha-pals': 65,
  bounties: 70,
  bosses: 75,
  'oil-rigs': 80,
  bases: 85,
  players: 90
}

const ITEM_KIND_SEARCH_TERMS: Record<ItemKind, string> = {
  players: 'player players guild member guild members',
  bases: 'base bases palbox',
  workers: 'base worker base workers assigned pal assigned pals',
  companions: 'companion pal companion pals',
  'wild-pals': 'wild pal wild pals',
  'alpha-pals': 'alpha pal alpha pals field alpha field alphas',
  bosses: 'tower boss tower bosses biome boss biome bosses',
  bounties: 'bounty bounties human boss human bosses wanted target wanted targets',
  'oil-rigs': 'oil rig oil rigs raid zone raid zones',
  watchtowers: 'watchtower watchtowers fast travel fast-travel',
  waypoints: 'waypoint waypoints fast travel fast-travel great eagle statue great eagle statues',
  'dungeon-entrances': 'dungeon entrance dungeon entrances cave caves',
  effigies: 'effigy effigies lifmunk effigy lifmunk effigies relic relics',
  journals: 'journal journals note notes diary diaries castaway',
  'ancient-shrine-pickups': 'ancient shrine pickup pickups schematic schematics item pickup',
  'npc-locations': 'npc location npc locations static npc static npcs non-player character trader merchant dealer',
  npcs: 'live npc live npcs spawned npc spawned npcs non-player character non-player characters'
}

export interface Point {
  x: number
  y: number
}

export interface View extends Point {
  scale: number
}

export interface SceneBounds {
  left: number
  right: number
  top: number
  bottom: number
}

export function sceneSize(pixelRatio = window.devicePixelRatio || 1): number {
  return MAP_PIXEL_SIZE / Math.max(1, pixelRatio)
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

export function coverScale(width: number, height: number, size: number): number {
  return Math.max(0.01, Math.max(width / size, height / size))
}

export function coverView(width: number, height: number, size: number): View {
  const scale = coverScale(width, height, size)
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

export function sceneViewportBounds(view: View, width: number, height: number, overscan = 80): SceneBounds {
  return {
    left: (-view.x - overscan) / view.scale,
    right: (width - view.x + overscan) / view.scale,
    top: (-view.y - overscan) / view.scale,
    bottom: (height - view.y + overscan) / view.scale
  }
}

export function isScenePointVisible(point: Point, bounds: SceneBounds): boolean {
  return point.x >= bounds.left && point.x <= bounds.right && point.y >= bounds.top && point.y <= bounds.bottom
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

export function itemSearchText(item: MapItem, relatedName = ''): string {
  const playerStatus = item.kind === 'players' ? (item.online === false ? 'offline' : 'online') : ''
  return `${item.name} ${item.detail || ''} ${item.level || ''} ${item.guildName || ''} ${ITEM_KIND_SEARCH_TERMS[item.kind]} ${playerStatus} ${relatedName}`.toLowerCase()
}

export function markerStackOrder(kind: ItemKind): number {
  return MARKER_STACK_ORDER[kind]
}

export function kindLabel(kind: MapItem['kind']): string {
  return (
    {
      players: 'Player',
      bases: 'Base',
      workers: 'Base worker',
      companions: 'Companion Pal',
      'wild-pals': 'Wild Pal',
      'alpha-pals': 'Alpha Pal',
      bosses: 'Tower boss',
      bounties: 'Bounty',
      'oil-rigs': 'Oil rig',
      watchtowers: 'Watchtower',
      waypoints: 'Waypoint',
      'dungeon-entrances': 'Dungeon entrance',
      effigies: 'Pal Effigy',
      journals: 'Journal',
      'ancient-shrine-pickups': 'Ancient Shrine pickup',
      'npc-locations': 'NPC location',
      npcs: 'NPC'
    } as const
  )[kind]
}
