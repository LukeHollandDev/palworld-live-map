import type { ItemKind, MapItem } from '../types'

export const LOCAL_COMPLETION_STORAGE_KEY = 'palworld-live-map.completion-profiles.v1'
export const LOCAL_COMPLETION_VERSION = 1 as const
export const DEFAULT_COMPLETION_PROFILE_ID = 'manual:default'
export const DEFAULT_COMPLETION_PROFILE_NAME = 'My checklist'

const MAX_PROFILES = 20
const MAX_MANUAL_MARKS = 20_000
const MAX_ID_LENGTH = 1_000
const MAX_PROFILE_NAME_LENGTH = 80

export const CHECKLIST_KINDS = [
  'alpha-pals',
  'bosses',
  'bounties',
  'oil-rigs',
  'watchtowers',
  'waypoints',
  'dungeon-entrances',
  'effigies',
  'journals',
  'ancient-shrine-pickups',
  'npc-locations'
] as const satisfies readonly ItemKind[]

const CHECKLIST_KIND_SET = new Set<ItemKind>(CHECKLIST_KINDS)

export interface ManualCompletionMark {
  landmarkId: string
  completedAt: string
}

export interface LocalManualCompletionProfile {
  id: string
  name: string
  source: 'manual'
  createdAt: string
  manualMarks: ManualCompletionMark[]
}

// This payload deliberately owns only user-authored browser state. Save-backed
// observations, identity claims, and claim credentials must remain outside it
// so a later save overlay can be linked or revoked without rewriting manual
// progress or persisting private evidence in localStorage.
export interface LocalCompletionState {
  version: typeof LOCAL_COMPLETION_VERSION
  activeProfileId: string
  remainingOnly: boolean
  profiles: LocalManualCompletionProfile[]
}

export interface CompletionSummary {
  total: number
  completed: number
  remaining: number
}

function timestamp(now: Date) {
  return Number.isFinite(now.getTime()) ? now.toISOString() : new Date(0).toISOString()
}

export function createDefaultLocalCompletionState(now = new Date()): LocalCompletionState {
  return {
    version: LOCAL_COMPLETION_VERSION,
    activeProfileId: DEFAULT_COMPLETION_PROFILE_ID,
    remainingOnly: false,
    profiles: [
      {
        id: DEFAULT_COMPLETION_PROFILE_ID,
        name: DEFAULT_COMPLETION_PROFILE_NAME,
        source: 'manual',
        createdAt: timestamp(now),
        manualMarks: []
      }
    ]
  }
}

function record(value: unknown): Record<string, unknown> | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function boundedText(value: unknown, maximum: number): string | undefined {
  if (typeof value !== 'string' || value.length === 0 || value.length > maximum || value !== value.trim())
    return undefined
  return value
}

function validTimestamp(value: unknown): value is string {
  return typeof value === 'string' && Number.isFinite(Date.parse(value))
}

function sanitizeManualMarks(value: unknown): ManualCompletionMark[] {
  if (!Array.isArray(value)) return []
  const marks = new Map<string, ManualCompletionMark>()
  for (const candidate of value.slice(0, MAX_MANUAL_MARKS)) {
    const input = record(candidate)
    const landmarkId = boundedText(input?.landmarkId, MAX_ID_LENGTH)
    const completedAt = input?.completedAt
    if (!landmarkId || !validTimestamp(completedAt)) continue
    marks.set(landmarkId, { landmarkId, completedAt })
  }
  return [...marks.values()]
}

function sanitizeProfile(value: unknown): LocalManualCompletionProfile | undefined {
  const input = record(value)
  const id = boundedText(input?.id, MAX_ID_LENGTH)
  const name = boundedText(input?.name, MAX_PROFILE_NAME_LENGTH)
  if (!id || !name || input?.source !== 'manual' || !validTimestamp(input.createdAt)) return undefined
  return {
    id,
    name,
    source: 'manual',
    createdAt: input.createdAt,
    manualMarks: sanitizeManualMarks(input.manualMarks)
  }
}

function sanitizeLocalCompletionState(value: unknown, now = new Date()): LocalCompletionState {
  const input = record(value)
  if (input?.version !== LOCAL_COMPLETION_VERSION || !Array.isArray(input.profiles))
    return createDefaultLocalCompletionState(now)

  const profiles: LocalManualCompletionProfile[] = []
  const seenIDs = new Set<string>()
  for (const candidate of input.profiles) {
    if (profiles.length >= MAX_PROFILES) break
    const profile = sanitizeProfile(candidate)
    if (!profile || seenIDs.has(profile.id)) continue
    seenIDs.add(profile.id)
    profiles.push(profile)
  }
  if (profiles.length === 0) return createDefaultLocalCompletionState(now)

  const requestedActiveID = boundedText(input.activeProfileId, MAX_ID_LENGTH)
  const activeProfileId = profiles.some((profile) => profile.id === requestedActiveID)
    ? (requestedActiveID as string)
    : profiles[0].id
  return {
    version: LOCAL_COMPLETION_VERSION,
    activeProfileId,
    remainingOnly: input.remainingOnly === true,
    profiles
  }
}

export function loadLocalCompletionState(): LocalCompletionState {
  try {
    const raw = window.localStorage.getItem(LOCAL_COMPLETION_STORAGE_KEY)
    return raw ? sanitizeLocalCompletionState(JSON.parse(raw)) : createDefaultLocalCompletionState()
  } catch {
    return createDefaultLocalCompletionState()
  }
}

export function saveLocalCompletionState(state: LocalCompletionState) {
  try {
    // Sanitizing before serialization is an intentional allowlist. Even if a
    // future in-memory composite grows save evidence, this writer persists only
    // the versioned manual profile layer and its local view preference.
    const manualState = sanitizeLocalCompletionState(state)
    window.localStorage.setItem(LOCAL_COMPLETION_STORAGE_KEY, JSON.stringify(manualState))
  } catch {
    // Browsers may deny storage in private or restricted contexts.
  }
}

export function activeLocalCompletionProfile(state: LocalCompletionState): LocalManualCompletionProfile {
  return state.profiles.find((profile) => profile.id === state.activeProfileId) || state.profiles[0]
}

export function manualCompletionIDs(profile: LocalManualCompletionProfile): Set<string> {
  return new Set(profile.manualMarks.map((mark) => mark.landmarkId))
}

export function setManualLandmarkCompletion(
  state: LocalCompletionState,
  landmarkId: string,
  completed: boolean,
  now = new Date()
): LocalCompletionState {
  if (!boundedText(landmarkId, MAX_ID_LENGTH)) return state
  const profileIndex = state.profiles.findIndex((profile) => profile.id === state.activeProfileId)
  if (profileIndex < 0) return state
  const profile = state.profiles[profileIndex]
  const existingIndex = profile.manualMarks.findIndex((mark) => mark.landmarkId === landmarkId)
  if ((completed && existingIndex >= 0) || (!completed && existingIndex < 0)) return state

  const manualMarks = profile.manualMarks.slice()
  if (completed) {
    if (manualMarks.length >= MAX_MANUAL_MARKS) return state
    manualMarks.push({ landmarkId, completedAt: timestamp(now) })
  } else {
    manualMarks.splice(existingIndex, 1)
  }

  const profiles = state.profiles.slice()
  profiles[profileIndex] = { ...profile, manualMarks }
  return { ...state, profiles }
}

export function setRemainingOnly(state: LocalCompletionState, remainingOnly: boolean): LocalCompletionState {
  return state.remainingOnly === remainingOnly ? state : { ...state, remainingOnly }
}

export function isChecklistItem(item: Pick<MapItem, 'kind'>): boolean {
  return CHECKLIST_KIND_SET.has(item.kind)
}

export function summarizeManualCompletion(
  items: readonly MapItem[],
  completedIDs: ReadonlySet<string>,
  layerId?: string
): CompletionSummary {
  const seen = new Set<string>()
  let total = 0
  let completed = 0
  for (const item of items) {
    if ((layerId && item.map !== layerId) || !isChecklistItem(item) || seen.has(item.id)) continue
    seen.add(item.id)
    total++
    if (completedIDs.has(item.id)) completed++
  }
  return { total, completed, remaining: total - completed }
}
