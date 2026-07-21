import { ALL_KINDS, type ItemKind } from '../types'

const FILTER_PREFERENCES_KEY = 'palworld-live-map.filters.v1'
const ZOOM_PREFERENCES_KEY = 'palworld-live-map.zoom.v1'

export interface FilterPreferences {
  activeLayerId?: string
  enabledKinds?: Set<ItemKind>
  hiddenIds?: Set<string>
}

function isItemKind(value: unknown): value is ItemKind {
  return typeof value === 'string' && ALL_KINDS.includes(value as ItemKind)
}

export function loadFilterPreferences(): FilterPreferences {
  try {
    const raw = window.localStorage.getItem(FILTER_PREFERENCES_KEY)
    if (!raw) return {}
    const value = JSON.parse(raw) as Record<string, unknown>
    return {
      activeLayerId: typeof value.activeLayerId === 'string' ? value.activeLayerId : undefined,
      enabledKinds: Array.isArray(value.enabledKinds) ? new Set(value.enabledKinds.filter(isItemKind)) : undefined,
      hiddenIds: Array.isArray(value.hiddenIds)
        ? new Set(value.hiddenIds.filter((id): id is string => typeof id === 'string').slice(0, 20_000))
        : undefined
    }
  } catch {
    return {}
  }
}

export function saveFilterPreferences(preferences: Required<FilterPreferences>) {
  try {
    window.localStorage.setItem(
      FILTER_PREFERENCES_KEY,
      JSON.stringify({
        activeLayerId: preferences.activeLayerId,
        enabledKinds: [...preferences.enabledKinds],
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
