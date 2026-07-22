import { ALL_KINDS, type ItemKind, type PlayerStatus } from '../types'

const FILTER_PREFERENCES_KEY = 'palworld-live-map.filters.v1'
const ZOOM_PREFERENCES_KEY = 'palworld-live-map.zoom.v1'
const LANDMARK_KINDS_VERSION = 2
const FILTER_KINDS_VERSION = 3
const KINDS_ADDED_IN_VERSION_2: ItemKind[] = ['alpha-pals', 'bosses']

export const DEFAULT_ENABLED_KINDS = ['players', 'bases', 'workers'] as const satisfies readonly ItemKind[]
export const DEFAULT_ENABLED_PLAYER_STATUSES = ['online', 'offline'] as const satisfies readonly PlayerStatus[]

export interface FilterPreferences {
  activeLayerId?: string
  enabledKinds?: Set<ItemKind>
  enabledPlayerStatuses?: Set<PlayerStatus>
  hiddenIds?: Set<string>
}

function isItemKind(value: unknown): value is ItemKind {
  return typeof value === 'string' && ALL_KINDS.includes(value as ItemKind)
}

function isPlayerStatus(value: unknown): value is PlayerStatus {
  return value === 'online' || value === 'offline'
}

export function loadFilterPreferences(): FilterPreferences {
  try {
    const raw = window.localStorage.getItem(FILTER_PREFERENCES_KEY)
    if (!raw)
      return {
        enabledKinds: new Set(DEFAULT_ENABLED_KINDS),
        enabledPlayerStatuses: new Set(DEFAULT_ENABLED_PLAYER_STATUSES)
      }
    const value = JSON.parse(raw) as Record<string, unknown>
    let enabledKinds = Array.isArray(value.enabledKinds) ? new Set(value.enabledKinds.filter(isItemKind)) : undefined
    const kindsVersion = typeof value.kindsVersion === 'number' ? value.kindsVersion : 1
    if (enabledKinds && kindsVersion < LANDMARK_KINDS_VERSION) {
      for (const kind of KINDS_ADDED_IN_VERSION_2) enabledKinds.add(kind)
    }
    if (
      enabledKinds &&
      kindsVersion < FILTER_KINDS_VERSION &&
      enabledKinds.size === ALL_KINDS.length &&
      ALL_KINDS.every((kind) => enabledKinds?.has(kind))
    ) {
      enabledKinds = new Set(DEFAULT_ENABLED_KINDS)
    }
    return {
      activeLayerId: typeof value.activeLayerId === 'string' ? value.activeLayerId : undefined,
      enabledKinds,
      enabledPlayerStatuses: Array.isArray(value.enabledPlayerStatuses)
        ? new Set(value.enabledPlayerStatuses.filter(isPlayerStatus))
        : new Set(DEFAULT_ENABLED_PLAYER_STATUSES),
      hiddenIds: Array.isArray(value.hiddenIds)
        ? new Set(value.hiddenIds.filter((id): id is string => typeof id === 'string').slice(0, 20_000))
        : undefined
    }
  } catch {
    return {
      enabledKinds: new Set(DEFAULT_ENABLED_KINDS),
      enabledPlayerStatuses: new Set(DEFAULT_ENABLED_PLAYER_STATUSES)
    }
  }
}

export function saveFilterPreferences(preferences: Required<FilterPreferences>) {
  try {
    window.localStorage.setItem(
      FILTER_PREFERENCES_KEY,
      JSON.stringify({
        activeLayerId: preferences.activeLayerId,
        kindsVersion: FILTER_KINDS_VERSION,
        enabledKinds: [...preferences.enabledKinds],
        enabledPlayerStatuses: [...preferences.enabledPlayerStatuses],
        hiddenIds: [...preferences.hiddenIds].slice(0, 20_000)
      })
    )
  } catch {
    // Browsers may deny storage in private or restricted contexts.
  }
}

function loadZoomPreferences(): Record<string, number> {
  try {
    const raw = window.localStorage.getItem(ZOOM_PREFERENCES_KEY)
    if (!raw) return {}
    const value = JSON.parse(raw) as Record<string, unknown>
    return Object.fromEntries(
      Object.entries(value).filter(
        (entry): entry is [string, number] => typeof entry[1] === 'number' && Number.isFinite(entry[1]) && entry[1] >= 1
      )
    )
  } catch {
    return {}
  }
}

export function loadZoomRatio(layerId: string): number {
  return loadZoomPreferences()[layerId] ?? 1
}

export function saveZoomRatio(layerId: string, ratio: number) {
  if (!Number.isFinite(ratio) || ratio < 1) return
  try {
    window.localStorage.setItem(
      ZOOM_PREFERENCES_KEY,
      JSON.stringify({ ...loadZoomPreferences(), [layerId]: Number(ratio.toFixed(6)) })
    )
  } catch {
    // Browsers may deny storage in private or restricted contexts.
  }
}
