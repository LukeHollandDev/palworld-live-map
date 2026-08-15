import type { MapItem } from '../types'

export const SAVE_PROGRESS_DOMAIN_IDS = ['waypoints', 'journals'] as const
export const SAVE_PROGRESS_STALE_AFTER_MS = 30 * 60_000

export type SaveProgressDomainID = (typeof SAVE_PROGRESS_DOMAIN_IDS)[number]
export type CompletionSource = 'save' | 'manual' | 'combined'

export interface SaveProgressDomain {
  id: SaveProgressDomainID
  coverage: 'complete'
  completedIds: string[]
  total: number
}

export interface SaveProgressSnapshot {
  snapshotAt: string
  catalogueVersion: string
  domains: SaveProgressDomain[]
}

const DOMAIN_IDS = new Set<string>(SAVE_PROGRESS_DOMAIN_IDS)
const MAX_COMPLETED_IDS = 20_000
const MAX_ID_LENGTH = 1_000
const MAX_CATALOGUE_VERSION_LENGTH = 256

export function catalogueVersionFromURL(catalogueURL: string, baseURL?: string): string | null {
  try {
    const parsed = new URL(
      catalogueURL,
      baseURL || (typeof window === 'undefined' ? 'http://localhost/' : window.location.href)
    )
    return boundedText(parsed.searchParams.get('v'), MAX_CATALOGUE_VERSION_LENGTH) || null
  } catch {
    return null
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

/**
 * Treat the private endpoint as a narrow protocol, not a bag of completion
 * IDs. The server currently has exact coverage only for waypoints and
 * journals, so an added, missing, partial, or duplicate domain invalidates the
 * entire transient overlay instead of silently claiming unsupported progress.
 */
export function parseSaveProgress(value: unknown): SaveProgressSnapshot | null {
  const input = record(value)
  const snapshotAt = boundedText(input?.snapshotAt, 80)
  const catalogueVersion = boundedText(input?.catalogueVersion, MAX_CATALOGUE_VERSION_LENGTH)
  if (!snapshotAt || !Number.isFinite(Date.parse(snapshotAt)) || !catalogueVersion || !Array.isArray(input?.domains))
    return null
  if (input.domains.length !== SAVE_PROGRESS_DOMAIN_IDS.length) return null

  const domains = new Map<SaveProgressDomainID, SaveProgressDomain>()
  for (const candidate of input.domains) {
    const domain = record(candidate)
    if (!domain || typeof domain.id !== 'string' || !DOMAIN_IDS.has(domain.id) || domain.coverage !== 'complete')
      return null
    const id = domain.id as SaveProgressDomainID
    if (domains.has(id) || !Number.isSafeInteger(domain.total) || Number(domain.total) < 0) return null
    if (!Array.isArray(domain.completedIds) || domain.completedIds.length > MAX_COMPLETED_IDS) return null

    const completedIds: string[] = []
    const seen = new Set<string>()
    for (const candidateID of domain.completedIds) {
      const completedID = boundedText(candidateID, MAX_ID_LENGTH)
      if (!completedID || seen.has(completedID)) return null
      seen.add(completedID)
      completedIds.push(completedID)
    }
    if (completedIds.length > Number(domain.total)) return null
    domains.set(id, { id, coverage: 'complete', completedIds, total: Number(domain.total) })
  }

  if (domains.size !== SAVE_PROGRESS_DOMAIN_IDS.length) return null
  return {
    snapshotAt: new Date(snapshotAt).toISOString(),
    catalogueVersion,
    domains: SAVE_PROGRESS_DOMAIN_IDS.map((id) => domains.get(id) as SaveProgressDomain)
  }
}

/** Only apply a domain ID to a catalogue item from that same exact domain. */
export function saveCompletionIDs(
  snapshot: SaveProgressSnapshot | null,
  items: readonly Pick<MapItem, 'id' | 'kind'>[]
): Set<string> {
  if (!snapshot) return new Set()
  const itemKinds = new Map(items.map((item) => [item.id, item.kind]))
  const completed = new Set<string>()
  for (const domain of snapshot.domains) {
    for (const id of domain.completedIds) {
      if (itemKinds.get(id) === domain.id) completed.add(id)
    }
  }
  return completed
}

export function combineCompletionIDs(
  manualCompletedIds: ReadonlySet<string>,
  saveCompletedIds: ReadonlySet<string>
): Set<string> {
  return new Set([...manualCompletedIds, ...saveCompletedIds])
}

export function completionSource(
  itemID: string,
  manualCompletedIds: ReadonlySet<string>,
  saveCompletedIds: ReadonlySet<string>
): CompletionSource | null {
  const manual = manualCompletedIds.has(itemID)
  const save = saveCompletedIds.has(itemID)
  if (manual && save) return 'combined'
  if (save) return 'save'
  if (manual) return 'manual'
  return null
}

export function completionSourceLabel(source: CompletionSource | null): string | null {
  if (source === 'save') return 'Save-confirmed'
  if (source === 'manual') return 'Manual'
  if (source === 'combined') return 'Combined'
  return null
}

export function isSaveProgressStale(snapshot: SaveProgressSnapshot, now = Date.now()): boolean {
  return now - Date.parse(snapshot.snapshotAt) > SAVE_PROGRESS_STALE_AFTER_MS
}

export function formatSaveProgressAge(snapshotAt: string, now = Date.now()): string {
  const ageMs = Math.max(0, now - Date.parse(snapshotAt))
  if (ageMs < 60_000) return 'just now'
  const minutes = Math.floor(ageMs / 60_000)
  if (minutes < 60) return `${minutes} min ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 48) return `${hours} hr ago`
  return `${Math.floor(hours / 24)} days ago`
}
