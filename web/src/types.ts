export type ItemKind = 'players' | 'bases' | 'workers' | 'companions' | 'wild-pals' | 'npcs'

export interface MapLayer {
  id: string
  name: string
  imageUrl?: string
  bounds: [number, number, number, number]
}

export interface PublicConfig {
  pollIntervalMs: number
  worldPollIntervalMs: number
  worldDataEnabled: boolean
  layers: MapLayer[]
}

export interface ServerInfo {
  name: string
  description?: string
  version?: string
}

export interface ServerMetrics {
  currentPlayers: number
  maxPlayers: number
  serverFps: number
  serverFrameTime: number
  uptimeSeconds: number
  baseCount: number
  days: number
}

export interface Player {
  id: string
  name: string
  level: number
  guildKey?: string
  guildName?: string
  x: number
  y: number
  map: string
}

export interface WorldObject {
  id: string
  kind: Exclude<ItemKind, 'players'>
  name: string
  detail?: string
  baseId?: string
  guildKey?: string
  ownerId?: string
  level?: number
  x: number
  y: number
  map: string
}

export interface MapItem {
  id: string
  kind: ItemKind
  name: string
  detail?: string
  baseId?: string
  guildKey?: string
  guildName?: string
  ownerId?: string
  level?: number
  x: number
  y: number
  map: string
}

export interface PlayerState {
  server: ServerInfo
  metrics: ServerMetrics
  metricsAvailable: boolean
  metricsStale: boolean
  metricsUpdatedAt?: string
  connected: boolean
  stale: boolean
  lastSuccessAt?: string
  players: Player[]
}

export interface ObjectState {
  enabled: boolean
  available: boolean
  stale: boolean
  unsupported: boolean
  truncated: boolean
  total: number
  updatedAt?: string
  lastError?: string
  objects: WorldObject[]
}

export const ALL_KINDS: ItemKind[] = ['players', 'bases', 'workers', 'companions', 'wild-pals', 'npcs']

export const EMPTY_OBJECT_STATE: ObjectState = {
  enabled: false,
  available: false,
  stale: false,
  unsupported: false,
  truncated: false,
  total: 0,
  objects: []
}
