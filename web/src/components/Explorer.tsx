import { IconCheck, IconChevronRight, IconSearch, IconX } from '@tabler/icons-react'
import { type ReactNode, type RefObject, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { SaveProgressState } from '../hooks/useSaveProgress'
import { guildIdForBase } from '../lib/guilds'
import { itemSearchText, markerText } from '../lib/map'
import { DEFAULT_ENABLED_PLAYER_STATUSES, FILTERABLE_KINDS } from '../lib/preferences'
import {
  type CompletionSource,
  completionSource,
  completionSourceLabel,
  formatSaveProgressAge
} from '../lib/saveProgress'
import type { ItemKind, MapItem, MapLayer, PlayerStatus } from '../types'
import { MapPanelHeader, MapPanelShell } from './MapPanel'
import { MarkerGlyph } from './MarkerGlyph'
import { PlayerClaimSessionControl } from './PlayerClaimPanel'

interface ExplorerProps {
  open: boolean
  activeLayer: MapLayer
  layers: MapLayer[]
  items: MapItem[]
  search: string
  filterButtonRef: RefObject<HTMLButtonElement | null>
  searchInputRef: RefObject<HTMLInputElement | null>
  enabledKinds: Set<ItemKind>
  enabledPlayerStatuses: Set<PlayerStatus>
  hiddenIds: Set<string>
  expandedGuilds: Set<string>
  expandedBases: Set<string>
  manualChecklist: CompletionChecklistView
  dataNotices: string[]
  catalogueRetry?: {
    message: string
    onRetry: () => void
  }
  onSearchChange: (value: string) => void
  onCheckAll: () => void
  onUncheckAll: () => void
  onToggleKinds: (kinds: ItemKind[], visible: boolean) => void
  onTogglePlayerStatus: (status: PlayerStatus, visible: boolean) => void
  onToggleItems: (ids: string[], visible: boolean) => void
  onToggleGuild: (id: string) => void
  onToggleBase: (id: string) => void
  onFocusItem: (item: MapItem, returnFocus: HTMLElement) => void
  onFocusGuild: (guildId: string, returnFocus: HTMLElement) => void
  onClose: () => void
  onLayerChange: (layer: MapLayer) => void
}

interface CompletionChecklistView {
  profileName: string
  completedIds: ReadonlySet<string>
  manualCompletedIds: ReadonlySet<string>
  saveCompletedIds: ReadonlySet<string>
  completed: number
  total: number
  remaining: number
  remainingOnly: boolean
  saveProgress: SaveProgressState
  onRetrySaveProgress: () => void
  onRemainingOnlyChange: (remainingOnly: boolean) => void
}
interface CheckState {
  checked: boolean
  indeterminate: boolean
  disabled?: boolean
}

function Checkbox({
  state,
  label,
  onChange,
  id
}: {
  state: CheckState
  label: string
  onChange: (checked: boolean) => void
  id?: string
}) {
  const ref = useRef<HTMLInputElement>(null)
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state.indeterminate
  }, [state.indeterminate])
  return (
    <input
      ref={ref}
      id={id}
      type="checkbox"
      className="size-3.5 shrink-0 accent-[#6cb4dd]"
      checked={state.checked}
      disabled={state.disabled}
      aria-label={label}
      onChange={(event) => onChange(event.currentTarget.checked)}
    />
  )
}

function saveProgressDescription(progress: SaveProgressState) {
  if (progress.phase === 'loading') return 'Loading private save progress…'
  if (progress.phase === 'unavailable')
    return progress.reason === 'catalogue-version'
      ? 'Private save progress does not match this map catalogue. Manual marks still count.'
      : 'Save progress is temporarily unavailable. Manual marks still count.'
  if (progress.phase === 'inactive') return 'Manual only · connect with “This is me” for save-confirmed progress.'
  const age = formatSaveProgressAge(progress.snapshot.snapshotAt)
  if (progress.refreshing) return `Refreshing private save progress; the valid snapshot from ${age} remains applied.`
  if (progress.refreshFailed) return `Refresh failed; keeping the valid save snapshot from ${age}.`
  return progress.stale
    ? `Save snapshot from ${age} may be stale. Manual marks still count.`
    : `Save-confirmed · ${age}`
}

function CompletionChecklistSummary({
  checklist,
  layerName
}: {
  checklist: CompletionChecklistView
  layerName: string
}) {
  const titleId = useId()
  const detailsId = useId()
  const remainingDescriptionId = useId()
  const saveProgressStatusId = useId()
  const [expanded, setExpanded] = useState(false)
  const progress = checklist.saveProgress
  const layerProgress = checklist.remainingOnly
    ? `${checklist.remaining} missing in ${layerName}`
    : `${checklist.completed} of ${checklist.total} landmarks complete in ${layerName}`
  return (
    <section className="pal-glass-inset mx-3.5 mb-2 text-xs" aria-labelledby={titleId}>
      <button
        type="button"
        className="pal-interactive group flex min-h-10 w-full cursor-pointer items-center gap-2 border-0 bg-transparent px-3 py-1.5 text-left text-[#edf9fb]"
        aria-label={`${expanded ? 'Collapse' : 'Expand'} exploration progress details`}
        aria-describedby={saveProgressStatusId}
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => setExpanded((current) => !current)}
      >
        <h3 id={titleId} className="m-0 min-w-0 flex-1 text-xs font-semibold">
          Exploration progress
        </h3>
        <strong
          aria-live="polite"
          aria-atomic="true"
          className="shrink-0 text-[11px] font-medium text-[#9de4c1] tabular-nums"
        >
          {checklist.completed}/{checklist.total}
          <span className="ml-1 font-normal text-[#83a0a5]"> · {checklist.remaining} missing</span>
        </strong>
        <span className="grid size-7 shrink-0 place-items-center text-[#929da1] group-hover:text-white">
          <Chevron expanded={expanded} />
        </span>
      </button>
      <label className="flex min-h-9 cursor-pointer items-center gap-2 border-t border-[#caeaef]/15 px-3 py-1.5 text-[11px] text-[#d6e7e9]">
        <input
          type="checkbox"
          className="size-3.5 shrink-0 accent-[#6cb4dd]"
          checked={checklist.remainingOnly}
          aria-describedby={remainingDescriptionId}
          onChange={(event) => checklist.onRemainingOnlyChange(event.currentTarget.checked)}
        />
        <span>Show only missing</span>
      </label>
      <p id={remainingDescriptionId} className="sr-only">
        Hide landmarks completed manually or confirmed by the connected save from the map and map filter.
      </p>
      <p id={saveProgressStatusId} aria-live="polite" aria-atomic="true" className="sr-only">
        {saveProgressDescription(progress)}
      </p>
      <div id={detailsId} className="grid gap-2 border-t border-[#caeaef]/15 px-3 py-2.5" hidden={!expanded}>
        <p className="m-0 truncate text-[10px] tracking-[.08em] text-[#77b9c2] uppercase">
          {progress.phase === 'available'
            ? `${checklist.profileName} + private save`
            : progress.phase === 'loading'
              ? `${checklist.profileName} · loading private save`
              : `${checklist.profileName} · manual only`}
        </p>
        <p className="m-0 text-[11px] text-[#a8b9bd]">{layerProgress}</p>
        <p
          aria-hidden="true"
          className={`m-0 text-[10px] leading-4 ${progress.phase === 'unavailable' || (progress.phase === 'available' && (progress.stale || progress.refreshFailed)) ? 'text-[#d8bc83]' : 'text-[#78949a]'}`}
        >
          {saveProgressDescription(progress)}
        </p>
        <p className="m-0 text-[10px] leading-4 text-[#78949a]">
          Private saves confirm waypoints and journals only; every other category uses manual marks.
        </p>
        {progress.phase === 'available' ? (
          <time className="sr-only" dateTime={progress.snapshot.snapshotAt}>
            Save snapshot {progress.snapshot.snapshotAt}
          </time>
        ) : null}
        {(progress.phase === 'unavailable' && progress.reason === 'request') ||
        (progress.phase === 'available' && (progress.stale || progress.refreshFailed)) ? (
          <button
            type="button"
            className="pal-interactive min-h-7 justify-self-start border border-[#8bb7bd]/25 bg-[#26363b]/55 px-2.5 text-[11px] text-[#d7e8ea]"
            disabled={progress.phase === 'available' && progress.refreshing}
            onClick={checklist.onRetrySaveProgress}
          >
            {progress.phase === 'unavailable'
              ? 'Retry save progress'
              : progress.refreshing
                ? 'Refreshing save progress…'
                : 'Refresh save progress'}
          </button>
        ) : null}
      </div>
    </section>
  )
}

type PlayerCategoryGroup = 'online-players' | 'offline-players'
type CategoryGroup =
  | PlayerCategoryGroup
  | Exclude<ItemKind, 'players' | 'workers' | 'companions' | 'wild-pals' | 'npcs'>
type NonPlayerCategoryGroup = Exclude<CategoryGroup, PlayerCategoryGroup>

const GROUP_KINDS: Record<NonPlayerCategoryGroup, ItemKind[]> = {
  bases: ['bases', 'workers'],
  'alpha-pals': ['alpha-pals'],
  bosses: ['bosses'],
  bounties: ['bounties'],
  'oil-rigs': ['oil-rigs'],
  watchtowers: ['watchtowers'],
  waypoints: ['waypoints'],
  'dungeon-entrances': ['dungeon-entrances'],
  effigies: ['effigies'],
  journals: ['journals'],
  'ancient-shrine-pickups': ['ancient-shrine-pickups'],
  'npc-locations': ['npc-locations']
}

const DEFAULT_COLLAPSED_GROUPS: CategoryGroup[] = [
  'offline-players',
  'bases',
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
]

const INITIAL_CATEGORY_ITEMS = 250
const GLOBAL_SEARCH_RESULT_BUDGET = 200
const SEARCH_RESULTS_STATUS_ID = 'map-search-results-status'

const SEARCH_SIMPLE_KINDS: ItemKind[] = [
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
]

interface ExplorerIndex {
  byKind: Record<ItemKind, MapItem[]>
  baseById: Map<string, MapItem>
  workersByBaseId: Map<string, MapItem[]>
  companionsByOwnerId: Map<string, MapItem[]>
  onlinePlayers: MapItem[]
  offlinePlayers: MapItem[]
  guildControlItems: MapItem[]
  guildCount: number
}

interface GuildBucket {
  id: string
  name: string
  displayName: string
  bases: MapItem[]
  outsideWorkers: MapItem[]
}

interface GuildData {
  guilds: GuildBucket[]
  fallbackWorkers: MapItem[]
}

interface SearchCandidate {
  item: MapItem
  rank: number
}

interface SearchPlan {
  allowedKeys: ReadonlySet<string>
  totalMatches: number
}

function searchResultKey(item: MapItem): string {
  return `${item.kind}:${item.id}`
}

function itemMatchesSearch(item: MapItem, query: string, baseById: Map<string, MapItem>): boolean {
  if (!query) return true
  const baseName = item.kind === 'workers' && item.baseId ? baseById.get(item.baseId)?.name || '' : ''
  return itemSearchText(item, baseName).includes(query)
}

function searchRank(item: MapItem, query: string, directMatch: boolean): number {
  const name = item.name.trim().toLowerCase()
  if (name === query) return 0
  if (name.startsWith(query)) return 1
  if (name.includes(query)) return 2
  return directMatch ? 3 : 4
}

function buildGuildData(bases: MapItem[], workers: MapItem[], workersByBaseId: Map<string, MapItem[]>): GuildData {
  const sortedBases = bases
    .slice()
    .sort((left, right) => left.name.localeCompare(right.name) || left.x - right.x || left.y - right.y)
  const guildNames = new Map<string, string>()
  for (const item of [...bases, ...workers]) {
    if (item.guildKey && item.guildName) guildNames.set(item.guildKey, item.guildName)
  }
  const guildMap = new Map<string, Omit<GuildBucket, 'displayName'>>()
  const newGuild = (id: string, name = guildNames.get(id) || 'Unnamed guild'): Omit<GuildBucket, 'displayName'> => ({
    id,
    name,
    bases: [],
    outsideWorkers: []
  })
  for (const base of sortedBases) {
    const id = guildIdForBase(base)
    const inferredGuildName =
      guildNames.get(id) || (base.name.trim().toLowerCase() === 'palbox' ? 'Unnamed guild' : base.name)
    const guild = guildMap.get(id) || newGuild(id, inferredGuildName)
    if (guild.name === 'Unnamed guild' && inferredGuildName !== 'Unnamed guild') guild.name = inferredGuildName
    guild.bases.push(base)
    guildMap.set(id, guild)
  }
  const baseLinkedIds = new Set(
    Array.from(workersByBaseId.values())
      .flat()
      .map((worker) => worker.id)
  )
  const fallbackWorkers: MapItem[] = []
  for (const worker of workers) {
    if (baseLinkedIds.has(worker.id)) continue
    if (!worker.guildKey) {
      fallbackWorkers.push(worker)
      continue
    }
    const guild = guildMap.get(worker.guildKey) || newGuild(worker.guildKey)
    guild.outsideWorkers.push(worker)
    guildMap.set(guild.id, guild)
  }
  for (const guild of guildMap.values()) {
    guild.outsideWorkers.sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id))
  }
  fallbackWorkers.sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id))
  const sortedGuilds = Array.from(guildMap.values()).sort(
    (left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id)
  )
  const nameCounts = new Map<string, number>()
  for (const guild of sortedGuilds) nameCounts.set(guild.name, (nameCounts.get(guild.name) || 0) + 1)
  const occurrences = new Map<string, number>()
  const guilds = sortedGuilds.map((guild) => {
    const occurrence = (occurrences.get(guild.name) || 0) + 1
    occurrences.set(guild.name, occurrence)
    return {
      ...guild,
      displayName: (nameCounts.get(guild.name) || 0) > 1 ? `${guild.name} #${occurrence}` : guild.name
    }
  })
  return { guilds, fallbackWorkers }
}

function buildSearchPlan(query: string, index: ExplorerIndex, guildData: GuildData): SearchPlan {
  const bucketMaps = Array.from({ length: 3 + SEARCH_SIMPLE_KINDS.length }, () => new Map<string, SearchCandidate>())
  const addCandidate = (bucketIndex: number, item: MapItem, directMatch: boolean) => {
    const key = searchResultKey(item)
    const candidate = { item, rank: searchRank(item, query, directMatch) }
    const existing = bucketMaps[bucketIndex].get(key)
    if (!existing || candidate.rank < existing.rank) bucketMaps[bucketIndex].set(key, candidate)
  }
  const matches = (item: MapItem) => itemMatchesSearch(item, query, index.baseById)

  for (const player of index.byKind.players) {
    if (player.online === false) continue
    const playerMatches = matches(player)
    if (playerMatches) addCandidate(0, player, true)
    for (const companion of index.companionsByOwnerId.get(player.id) || []) {
      const companionMatches = matches(companion)
      if (playerMatches || companionMatches) addCandidate(0, companion, companionMatches)
    }
  }
  for (const player of index.byKind.players) {
    if (player.online === false && matches(player)) addCandidate(1, player, true)
  }

  for (const guild of guildData.guilds) {
    const guildMatches = guild.displayName.toLowerCase().includes(query)
    for (const base of guild.bases) {
      const baseMatches = matches(base)
      if (guildMatches || baseMatches) addCandidate(2, base, baseMatches)
      const baseWorkers = index.workersByBaseId.get(base.id) || index.workersByBaseId.get(base.baseId || '') || []
      for (const worker of baseWorkers) {
        const workerMatches = matches(worker)
        if (guildMatches || workerMatches) addCandidate(2, worker, workerMatches)
      }
    }
    for (const worker of guild.outsideWorkers) {
      const workerMatches = matches(worker)
      if (guildMatches || workerMatches) addCandidate(2, worker, workerMatches)
    }
  }
  const fallbackMatches = 'no linked guild outside base perimeters'.includes(query)
  for (const worker of guildData.fallbackWorkers) {
    const workerMatches = matches(worker)
    if (fallbackMatches || workerMatches) addCandidate(2, worker, workerMatches)
  }

  SEARCH_SIMPLE_KINDS.forEach((kind, offset) => {
    for (const item of index.byKind[kind]) {
      if (matches(item)) addCandidate(3 + offset, item, true)
    }
  })

  const buckets = bucketMaps.map((bucket) =>
    Array.from(bucket.values()).sort(
      (left, right) =>
        left.rank - right.rank ||
        left.item.name.localeCompare(right.item.name) ||
        left.item.kind.localeCompare(right.item.kind) ||
        left.item.id.localeCompare(right.item.id)
    )
  )
  const totalKeys = new Set(buckets.flatMap((bucket) => bucket.map(({ item }) => searchResultKey(item))))
  const allowedKeys = new Set<string>()
  for (let rank = 0; rank <= 4 && allowedKeys.size < GLOBAL_SEARCH_RESULT_BUDGET; rank++) {
    const rankedBuckets = buckets.map((bucket) => bucket.filter((candidate) => candidate.rank === rank))
    const cursors = rankedBuckets.map(() => 0)
    let added = true
    while (added && allowedKeys.size < GLOBAL_SEARCH_RESULT_BUDGET) {
      added = false
      rankedBuckets.forEach((bucket, bucketIndex) => {
        if (allowedKeys.size >= GLOBAL_SEARCH_RESULT_BUDGET) return
        const candidate = bucket[cursors[bucketIndex]]
        if (!candidate) return
        cursors[bucketIndex]++
        allowedKeys.add(searchResultKey(candidate.item))
        added = true
      })
    }
  }
  return { allowedKeys, totalMatches: totalKeys.size }
}

function playerStatusForGroup(group: CategoryGroup): PlayerStatus | undefined {
  if (group === 'online-players') return 'online'
  if (group === 'offline-players') return 'offline'
  return undefined
}

function visibilityState(
  items: MapItem[],
  enabledKinds: Set<ItemKind>,
  enabledPlayerStatuses: Set<PlayerStatus>,
  hiddenIds: Set<string>
): CheckState {
  const enabled = (item: MapItem) =>
    enabledKinds.has(item.kind) &&
    (item.kind !== 'players' || enabledPlayerStatuses.has(item.online === false ? 'offline' : 'online'))
  const visible = items.filter((item) => enabled(item) && !hiddenIds.has(item.id)).length
  return {
    checked: items.length > 0 && visible === items.length,
    indeterminate: visible > 0 && visible < items.length,
    disabled: items.length === 0
  }
}

function ItemButton({
  item,
  meta,
  label,
  completion = null,
  onFocus
}: {
  item: MapItem
  meta?: string
  label?: string
  completion?: CompletionSource | null
  onFocus: ExplorerProps['onFocusItem']
}) {
  const sourceLabel = completionSourceLabel(completion)
  const completionDescription = sourceLabel ? `, ${sourceLabel.toLowerCase()} completion` : ''
  return (
    <button
      type="button"
      className="pal-interactive grid min-h-7 w-full min-w-0 flex-1 cursor-pointer grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-1.5 border border-transparent bg-transparent px-1.5 py-1 text-left text-xs text-[#e3edef] focus-visible:outline-none"
      aria-label={`View ${markerText(item)}${completionDescription}`}
      title={item.detail}
      onClick={(event) => onFocus(item, event.currentTarget)}
    >
      <MarkerGlyph kind={item.kind} online={item.online} />
      <span className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">{label || item.name}</span>
      {meta || sourceLabel ? (
        <span className="ml-auto flex shrink-0 items-center gap-1.5 text-[11px] text-[#899398]">
          {meta ? <span>{meta}</span> : null}
          {sourceLabel ? (
            <span
              className={`flex items-center gap-0.5 text-[10px] ${completion === 'save' ? 'text-[#8ed7f2]' : completion === 'combined' ? 'text-[#b7e8a2]' : 'text-[#8fe0c2]'}`}
              title={`${sourceLabel} completion`}
            >
              <IconCheck className="size-3" aria-hidden="true" />
              {sourceLabel}
            </span>
          ) : null}
        </span>
      ) : null}
    </button>
  )
}

function GuildButton({
  guildId,
  name,
  meta,
  onFocus
}: {
  guildId: string
  name: string
  meta: string
  onFocus: ExplorerProps['onFocusGuild']
}) {
  return (
    <button
      type="button"
      className="pal-interactive grid min-h-7 min-w-0 flex-1 cursor-pointer grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-1.5 border border-transparent bg-transparent px-1.5 py-1 text-left text-xs text-[#e3edef] focus-visible:outline-none"
      aria-label={`View guild ${name}`}
      onClick={(event) => onFocus(guildId, event.currentTarget)}
    >
      <MarkerGlyph kind="bases" />
      <span className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-medium">{name}</span>
      <span className="ml-auto shrink-0 text-[10px] text-[#7f898e]">{meta}</span>
    </button>
  )
}

function ObjectRow({
  item,
  meta,
  label,
  enabledKinds,
  enabledPlayerStatuses,
  hiddenIds,
  manualChecklist,
  onToggleItems,
  onFocusItem,
  className = ''
}: Pick<
  ExplorerProps,
  'enabledKinds' | 'enabledPlayerStatuses' | 'hiddenIds' | 'manualChecklist' | 'onToggleItems' | 'onFocusItem'
> & {
  item: MapItem
  meta?: string
  label?: string
  className?: string
}) {
  return (
    <div className={`flex min-w-0 items-center gap-0.5 ${className}`}>
      <span className="grid size-8 shrink-0 place-items-center">
        <Checkbox
          state={visibilityState([item], enabledKinds, enabledPlayerStatuses, hiddenIds)}
          label={`Show ${markerText(item)}`}
          onChange={(checked) => onToggleItems([item.id], checked)}
        />
      </span>
      <ItemButton
        item={item}
        meta={meta}
        label={label}
        completion={completionSource(item.id, manualChecklist.manualCompletedIds, manualChecklist.saveCompletedIds)}
        onFocus={onFocusItem}
      />
    </div>
  )
}

export function Explorer(props: ExplorerProps) {
  const closeRef = useRef<HTMLButtonElement>(null)
  const wasOpen = useRef(props.open)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<CategoryGroup>>(() => new Set(DEFAULT_COLLAPSED_GROUPS))

  useLayoutEffect(() => {
    if (wasOpen.current === props.open) return
    wasOpen.current = props.open
    if (props.open) {
      closeRef.current?.focus({ preventScroll: true })
      return
    }
    if (document.activeElement instanceof Element && document.activeElement.closest('#map-filter-panel')) {
      props.filterButtonRef.current?.focus({ preventScroll: true })
    }
  }, [props.filterButtonRef, props.open])

  const toggleCategory = (group: CategoryGroup) => {
    setCollapsedGroups((current) => {
      const next = new Set(current)
      if (next.has(group)) next.delete(group)
      else next.add(group)
      return next
    })
  }

  const index = useMemo(() => {
    const byKind: Record<ItemKind, MapItem[]> = {
      players: [],
      bases: [],
      workers: [],
      companions: [],
      'wild-pals': [],
      'alpha-pals': [],
      bosses: [],
      bounties: [],
      'oil-rigs': [],
      watchtowers: [],
      waypoints: [],
      'dungeon-entrances': [],
      effigies: [],
      journals: [],
      'ancient-shrine-pickups': [],
      'npc-locations': [],
      npcs: []
    }
    const baseById = new Map<string, MapItem>()
    const workersByBaseId = new Map<string, MapItem[]>()
    const companionsByOwnerId = new Map<string, MapItem[]>()
    const linkedWorkerIds = new Set<string>()
    for (const item of props.items) {
      if (item.kind === 'companions' && item.ownerId) {
        const ownerCompanions = companionsByOwnerId.get(item.ownerId) || []
        ownerCompanions.push(item)
        companionsByOwnerId.set(item.ownerId, ownerCompanions)
      }
      if (item.map !== props.activeLayer.id) continue
      byKind[item.kind].push(item)
      if (item.kind === 'bases') {
        baseById.set(item.id, item)
        if (item.baseId) baseById.set(item.baseId, item)
      }
    }
    for (const worker of byKind.workers) {
      if (!worker.baseId || !baseById.has(worker.baseId)) continue
      const baseWorkers = workersByBaseId.get(worker.baseId) || []
      baseWorkers.push(worker)
      workersByBaseId.set(worker.baseId, baseWorkers)
      linkedWorkerIds.add(worker.id)
    }
    for (const companions of companionsByOwnerId.values()) {
      companions.sort(
        (left, right) =>
          left.name.localeCompare(right.name) ||
          (left.level || 0) - (right.level || 0) ||
          left.id.localeCompare(right.id)
      )
    }
    const guildIds = new Set(byKind.bases.map(guildIdForBase))
    for (const worker of byKind.workers) {
      if (!linkedWorkerIds.has(worker.id) && worker.guildKey) guildIds.add(worker.guildKey)
    }
    return {
      byKind,
      baseById,
      workersByBaseId,
      companionsByOwnerId,
      onlinePlayers: byKind.players.filter((player) => player.online !== false),
      offlinePlayers: byKind.players.filter((player) => player.online === false),
      guildControlItems: [...byKind.bases, ...byKind.workers],
      guildCount: guildIds.size
    }
  }, [props.activeLayer.id, props.items])

  const searchTerm = props.search.trim()
  const query = searchTerm.toLowerCase()
  const searching = Boolean(query)
  const matches = (item: MapItem) => itemMatchesSearch(item, query, index.baseById)
  const guildExpanded = searching || !collapsedGroups.has('bases')
  const guildData = useMemo(
    () => (guildExpanded ? buildGuildData(index.byKind.bases, index.byKind.workers, index.workersByBaseId) : undefined),
    [guildExpanded, index]
  )
  const searchPlan = useMemo(
    () => (query && guildData ? buildSearchPlan(query, index, guildData) : undefined),
    [guildData, index, query]
  )
  const allFiltersChecked =
    FILTERABLE_KINDS.every((kind) => props.enabledKinds.has(kind)) &&
    DEFAULT_ENABLED_PLAYER_STATUSES.every((status) => props.enabledPlayerStatuses.has(status)) &&
    props.hiddenIds.size === 0

  return (
    // biome-ignore lint/complexity/noUselessFragments: the stable wrapper keeps this large panel's markup isolated from its external header trigger
    <>
      <MapPanelShell
        id="map-filter-panel"
        side="left"
        mobileSize="fixed"
        mobileSheetActive={props.open}
        mobileSheetLabel="map filters"
        className={`filter-panel-motion max-sm:z-[34] ${props.open ? 'is-panel-open' : 'is-panel-closed pointer-events-none'}`}
        aria-label="Map filters"
        aria-hidden={!props.open}
        inert={!props.open}
      >
        <MapPanelHeader
          as="div"
          eyebrow="MAP FILTER"
          title="Map"
          closeButtonRef={closeRef}
          closeLabel="Collapse map filter"
          closeControls="map-filter-panel"
          closeExpanded
          closeTitle="Collapse map filter"
          onClose={props.onClose}
        />

        <div className="filter-panel-body-motion relative flex min-h-0 flex-1 flex-col overflow-hidden">
          <div className="relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden">
            <search
              id="map-search-control"
              aria-label="Map search"
              className="pal-glass-inset relative mx-3.5 mt-3 flex h-11 shrink-0 items-center text-[#dceef0] transition-[border-color,box-shadow] focus-within:border-[#62d6e7] focus-within:shadow-[inset_0_-2px_#22c7e8]"
            >
              <span className="grid size-10 shrink-0 place-items-center text-[#65bbc7]" aria-hidden="true">
                <IconSearch className="size-[18px]" aria-hidden="true" />
              </span>
              <label className="sr-only" htmlFor="map-search">
                Search map locations and live objects
              </label>
              <input
                id="map-search"
                ref={props.searchInputRef}
                type="search"
                aria-label="Search map locations and live objects"
                aria-describedby={searching ? SEARCH_RESULTS_STATUS_ID : undefined}
                aria-keyshortcuts="/"
                placeholder="Filter map results…"
                autoComplete="off"
                enterKeyHint="search"
                spellCheck="false"
                value={props.search}
                className="h-full min-w-0 flex-1 appearance-none border-0 bg-transparent pr-2 text-sm tracking-[.02em] text-[#e7f6f8] outline-0 placeholder:text-[#60767d] [&::-webkit-search-cancel-button]:hidden [&::-webkit-search-decoration]:hidden"
                onChange={(event) => props.onSearchChange(event.currentTarget.value)}
                onKeyDown={(event) => {
                  if (event.key !== 'Escape') return
                  event.preventDefault()
                  event.stopPropagation()
                  if (props.search) props.onSearchChange('')
                  else props.onClose()
                }}
              />
              {props.search ? (
                <button
                  type="button"
                  className="pal-interactive grid size-10 shrink-0 cursor-pointer place-items-center border-0 bg-transparent text-lg text-[#739097]"
                  aria-label="Clear search"
                  onClick={() => {
                    props.onSearchChange('')
                    props.searchInputRef.current?.focus()
                  }}
                >
                  <IconX className="size-5" aria-hidden="true" />
                </button>
              ) : null}
            </search>
            <fieldset className="pal-glass-inset mx-3.5 mt-2 mb-2 flex" aria-label="World region">
              {props.layers.map((layer) => {
                const active = layer.id === props.activeLayer.id
                return (
                  <button
                    key={layer.id}
                    type="button"
                    className={`min-h-10 min-w-0 flex-1 cursor-pointer overflow-hidden border-0 px-1 text-xs font-normal tracking-[.04em] text-ellipsis whitespace-nowrap uppercase transition-colors ${
                      active
                        ? 'bg-[#34444a]/80 text-[#e8f7f8] shadow-[inset_0_-2px_#20c7ea]'
                        : 'bg-transparent text-[#a9b5b9] hover:bg-[#34444a]/55 hover:text-white'
                    }`}
                    aria-pressed={active}
                    onClick={() => props.onLayerChange(layer)}
                  >
                    {layer.name}
                  </button>
                )
              })}
            </fieldset>
            <PlayerClaimSessionControl />
            <CompletionChecklistSummary checklist={props.manualChecklist} layerName={props.activeLayer.name} />
            <div className="mx-3.5 mb-2 flex shrink-0 justify-end gap-1.5">
              <button
                type="button"
                className="pal-interactive min-h-7 cursor-pointer border border-[#8bb7bd]/25 bg-[#26363b]/55 px-2.5 text-[11px] text-[#b7cdd1] transition-colors enabled:hover:border-[#7fd7e3]/50 enabled:hover:text-[#e5f8fa] disabled:cursor-default disabled:opacity-40"
                disabled={allFiltersChecked}
                onClick={props.onCheckAll}
              >
                Show all
              </button>
              <button
                type="button"
                className="pal-interactive min-h-7 cursor-pointer border border-[#8bb7bd]/25 bg-[#26363b]/55 px-2.5 text-[11px] text-[#b7cdd1] transition-colors enabled:hover:border-[#7fd7e3]/50 enabled:hover:text-[#e5f8fa] disabled:cursor-default disabled:opacity-40"
                disabled={props.enabledKinds.size === 0 && props.enabledPlayerStatuses.size === 0}
                onClick={props.onUncheckAll}
              >
                Hide all
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain border-t border-[#caeaef]/20 px-3.5 pt-1.5 pb-3.5">
              {searchPlan ? (
                <p
                  id={SEARCH_RESULTS_STATUS_ID}
                  role="status"
                  aria-label="Search results"
                  aria-live="polite"
                  aria-atomic="true"
                  className="mt-px mb-2 text-[11px] text-[#688088]"
                >
                  {searchPlan.totalMatches > GLOBAL_SEARCH_RESULT_BUDGET
                    ? `Showing ${GLOBAL_SEARCH_RESULT_BUDGET} of ${searchPlan.totalMatches} matches for “${searchTerm}”. Refine your search to inspect ${searchPlan.totalMatches - GLOBAL_SEARCH_RESULT_BUDGET} more.`
                    : `${searchPlan.totalMatches} match${searchPlan.totalMatches === 1 ? '' : 'es'} for “${searchTerm}”.`}
                </p>
              ) : null}
              <PlayerCategory
                {...props}
                players={index.onlinePlayers}
                companionsByOwnerId={index.companionsByOwnerId}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                expanded={searching || !collapsedGroups.has('online-players')}
                onToggleExpanded={() => toggleCategory('online-players')}
              />
              <SimpleCategory
                {...props}
                group="offline-players"
                title="Offline Players"
                items={index.offlinePlayers}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No saved offline players are loaded for this region."
                expanded={searching || !collapsedGroups.has('offline-players')}
                onToggleExpanded={() => toggleCategory('offline-players')}
              />
              <GuildCategory
                {...props}
                guildData={guildData}
                controlItems={index.guildControlItems}
                guildCount={index.guildCount}
                bases={index.byKind.bases}
                workers={index.byKind.workers}
                workersByBaseId={index.workersByBaseId}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                expanded={guildExpanded}
                onToggleExpanded={() => toggleCategory('bases')}
              />
              <SimpleCategory
                {...props}
                group="alpha-pals"
                title="Alpha Pals"
                items={index.byKind['alpha-pals']}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Alpha Pal landmarks are loaded for this region."
                expanded={searching || !collapsedGroups.has('alpha-pals')}
                onToggleExpanded={() => toggleCategory('alpha-pals')}
              />
              <SimpleCategory
                {...props}
                group="bosses"
                title="Tower Bosses"
                items={index.byKind.bosses}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Tower Boss landmarks are loaded for this region."
                expanded={searching || !collapsedGroups.has('bosses')}
                onToggleExpanded={() => toggleCategory('bosses')}
              />
              <SimpleCategory
                {...props}
                group="bounties"
                title="Bounties"
                items={index.byKind.bounties}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Bounty locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('bounties')}
                onToggleExpanded={() => toggleCategory('bounties')}
              />
              <SimpleCategory
                {...props}
                group="oil-rigs"
                title="Oil Rigs"
                items={index.byKind['oil-rigs']}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Oil Rig locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('oil-rigs')}
                onToggleExpanded={() => toggleCategory('oil-rigs')}
              />
              <SimpleCategory
                {...props}
                group="watchtowers"
                title="Watchtowers"
                items={index.byKind.watchtowers}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Watchtower locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('watchtowers')}
                onToggleExpanded={() => toggleCategory('watchtowers')}
              />
              <SimpleCategory
                {...props}
                group="waypoints"
                title="Waypoints"
                items={index.byKind.waypoints}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Waypoint locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('waypoints')}
                onToggleExpanded={() => toggleCategory('waypoints')}
              />
              <SimpleCategory
                {...props}
                group="dungeon-entrances"
                title="Dungeon Entrances"
                items={index.byKind['dungeon-entrances']}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Dungeon Entrance locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('dungeon-entrances')}
                onToggleExpanded={() => toggleCategory('dungeon-entrances')}
              />
              <SimpleCategory
                {...props}
                group="effigies"
                title="Pal Effigies"
                items={index.byKind.effigies}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Pal Effigy locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('effigies')}
                onToggleExpanded={() => toggleCategory('effigies')}
              />
              <SimpleCategory
                {...props}
                group="journals"
                title="Journals"
                items={index.byKind.journals}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Journal locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('journals')}
                onToggleExpanded={() => toggleCategory('journals')}
              />
              <SimpleCategory
                {...props}
                group="ancient-shrine-pickups"
                title="Ancient Shrine Pickups"
                items={index.byKind['ancient-shrine-pickups']}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No Ancient Shrine pickups are loaded for this region."
                expanded={searching || !collapsedGroups.has('ancient-shrine-pickups')}
                onToggleExpanded={() => toggleCategory('ancient-shrine-pickups')}
              />
              <SimpleCategory
                {...props}
                group="npc-locations"
                title="NPC Locations"
                items={index.byKind['npc-locations']}
                matches={matches}
                searchResultKeys={searchPlan?.allowedKeys}
                empty="No static NPC locations are loaded for this region."
                expanded={searching || !collapsedGroups.has('npc-locations')}
                onToggleExpanded={() => toggleCategory('npc-locations')}
              />
            </div>

            {props.dataNotices.map((notice) => (
              <p
                key={notice}
                aria-live="polite"
                className="m-3 mt-1 rounded-md border border-[#554b37] bg-[#302b22] px-2.5 py-2 text-[11px] leading-4 text-[#d2b980]"
              >
                {notice}
              </p>
            ))}
            {props.catalogueRetry ? (
              <div
                role="status"
                aria-live="polite"
                className="m-3 mt-1 flex items-center gap-2 rounded-md border border-[#554b37] bg-[#302b22] px-2.5 py-2 text-[11px] leading-4 text-[#d2b980]"
              >
                <span className="min-w-0 flex-1">{props.catalogueRetry.message}</span>
                <button
                  type="button"
                  className="pal-interactive min-h-8 shrink-0 cursor-pointer border border-[#8f7b50]/50 bg-[#3b3428] px-2 text-[11px] text-[#ead5a2]"
                  onClick={props.catalogueRetry.onRetry}
                >
                  Retry catalogue
                </button>
              </div>
            ) : null}
          </div>
        </div>
      </MapPanelShell>
    </>
  )
}

interface CategoryProps extends ExplorerProps {
  group: CategoryGroup
  title: string
  matches: (item: MapItem) => boolean
  searchResultKeys?: ReadonlySet<string>
  empty: string
  expanded: boolean
  onToggleExpanded: () => void
}

function CategoryHeader({
  group,
  title,
  items,
  expanded,
  controls,
  enabledKinds,
  enabledPlayerStatuses,
  hiddenIds,
  onToggleKinds,
  onTogglePlayerStatus,
  onToggleExpanded,
  count
}: Pick<
  CategoryProps,
  | 'group'
  | 'title'
  | 'expanded'
  | 'enabledKinds'
  | 'enabledPlayerStatuses'
  | 'hiddenIds'
  | 'onToggleKinds'
  | 'onTogglePlayerStatus'
  | 'onToggleExpanded'
> & {
  items: MapItem[]
  controls: string
  count?: number
}) {
  const playerStatus = playerStatusForGroup(group)
  const kinds = playerStatus ? (['players'] as ItemKind[]) : GROUP_KINDS[group as NonPlayerCategoryGroup]
  const state = visibilityState(items, enabledKinds, enabledPlayerStatuses, hiddenIds)
  const categoryEnabled = playerStatus
    ? enabledPlayerStatuses.has(playerStatus) && enabledKinds.has('players')
    : kinds.every((kind) => enabledKinds.has(kind))
  const checked = items.length > 0 && categoryEnabled && state.checked
  const itemCount = count ?? (group === 'bases' ? items.filter((item) => item.kind === group).length : items.length)
  return (
    <div className="flex min-h-8 items-center gap-0.5">
      <span className="grid size-8 shrink-0 place-items-center">
        <Checkbox
          state={{ checked, indeterminate: !checked && state.indeterminate, disabled: items.length === 0 }}
          label={`Show ${title}`}
          onChange={(visible) => {
            if (playerStatus) onTogglePlayerStatus(playerStatus, visible)
            else onToggleKinds(kinds, visible)
          }}
        />
      </span>
      <AccordionButton expanded={expanded} label={`${title} section`} controls={controls} onClick={onToggleExpanded}>
        <MarkerGlyph
          kind={playerStatus ? 'players' : (group as ItemKind)}
          online={playerStatus ? playerStatus === 'online' : undefined}
        />
        <strong className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-xs font-semibold">
          {title} ({itemCount})
        </strong>
      </AccordionButton>
    </div>
  )
}

interface PlayerCategoryProps extends ExplorerProps {
  players: MapItem[]
  companionsByOwnerId: Map<string, MapItem[]>
  matches: (item: MapItem) => boolean
  searchResultKeys?: ReadonlySet<string>
  expanded: boolean
  onToggleExpanded: () => void
}

function PlayerCategory({ players, expanded, onToggleExpanded, ...props }: PlayerCategoryProps) {
  const contentId = useId()
  return (
    <section className="border-b border-white/7 py-0.5 last:border-b-0">
      <CategoryHeader
        {...props}
        group="online-players"
        title="Online Players"
        items={players}
        expanded={expanded}
        onToggleExpanded={onToggleExpanded}
        controls={contentId}
      />
      <div id={contentId} className="grid gap-px pl-1.5" hidden={!expanded}>
        {expanded ? <PlayerCategoryBody {...props} players={players} /> : null}
      </div>
    </section>
  )
}

function PlayerCategoryBody({
  players,
  companionsByOwnerId,
  matches,
  searchResultKeys,
  ...props
}: Omit<PlayerCategoryProps, 'expanded' | 'onToggleExpanded'>) {
  const hasRawSearchMatches =
    Boolean(searchResultKeys) &&
    players.some((player) => matches(player) || (companionsByOwnerId.get(player.id) || []).some(matches))
  const rows = players
    .map((player) => {
      const companions = companionsByOwnerId.get(player.id) || []
      const playerMatches = matches(player)
      const matchingCompanions = playerMatches ? companions : companions.filter(matches)
      const renderedCompanions = searchResultKeys
        ? matchingCompanions.filter((companion) => searchResultKeys.has(searchResultKey(companion)))
        : matchingCompanions
      const playerIsResult = !searchResultKeys || searchResultKeys.has(searchResultKey(player))
      return {
        player,
        companions: renderedCompanions,
        visible: searchResultKeys
          ? playerIsResult || renderedCompanions.length > 0
          : playerMatches || companions.some(matches)
      }
    })
    .filter((row) => row.visible)
    .sort(
      (left, right) =>
        left.player.name.localeCompare(right.player.name) || left.player.id.localeCompare(right.player.id)
    )

  let companionBudget = INITIAL_CATEGORY_ITEMS
  const renderedRows = rows.map((row) => {
    const companions = row.companions.slice(0, companionBudget)
    companionBudget -= companions.length
    return { ...row, companions }
  })
  const visibleCompanionCount = rows.reduce((total, row) => total + row.companions.length, 0)
  const renderedCompanionCount = renderedRows.reduce((total, row) => total + row.companions.length, 0)
  const omittedCompanions = visibleCompanionCount - renderedCompanionCount

  return (
    <>
      {rows.length === 0 ? (
        <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
          {hasRawSearchMatches
            ? `Matching online players or companion Pals fall outside the first ${GLOBAL_SEARCH_RESULT_BUDGET} results.`
            : players.length > 0 && props.search.trim()
              ? `No online players or companion Pals match “${props.search.trim()}”.`
              : 'No players are currently online in this region.'}
        </p>
      ) : (
        renderedRows.map(({ player, companions }) => (
          <div key={player.id}>
            <ObjectRow
              item={player}
              meta={[player.level ? `Lv ${player.level}` : '', player.guildName || ''].filter(Boolean).join(' · ')}
              {...props}
            />
            {companions.length > 0 ? (
              <fieldset className="m-0 ml-8 grid min-w-0 gap-px border-0 border-l border-[#64d7e7]/25 p-0 pl-1.5">
                <legend className="sr-only">Companion Pals for {player.name}</legend>
                {companions.map((companion) => (
                  <ItemButton
                    key={companion.id}
                    item={companion}
                    meta={[companion.detail || '', companion.level ? `Lv ${companion.level}` : '']
                      .filter(Boolean)
                      .join(' · ')}
                    onFocus={props.onFocusItem}
                  />
                ))}
              </fieldset>
            ) : null}
          </div>
        ))
      )}
      {!searchResultKeys && omittedCompanions > 0 ? (
        <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
          {props.search.trim()
            ? `${omittedCompanions} more companion matches. Refine your search to inspect them.`
            : `${omittedCompanions} more companion Pals are omitted. Use search to inspect them.`}
        </p>
      ) : null}
    </>
  )
}

function SimpleCategory({
  group,
  title,
  items,
  controlItems = items,
  count,
  matches,
  searchResultKeys,
  empty,
  ...props
}: CategoryProps & { items: MapItem[]; controlItems?: MapItem[]; count?: number }) {
  const hasRawSearchMatches = Boolean(searchResultKeys) && items.some(matches)
  const visible = props.expanded
    ? items
        .filter((item) => (searchResultKeys ? searchResultKeys.has(searchResultKey(item)) : matches(item)))
        .sort((left, right) => left.name.localeCompare(right.name))
    : []
  const rendered = visible.slice(0, INITIAL_CATEGORY_ITEMS)
  const contentId = useId()
  return (
    <section className="border-b border-white/7 py-0.5 last:border-b-0">
      <CategoryHeader {...props} group={group} title={title} items={controlItems} count={count} controls={contentId} />
      <div id={contentId} className="grid gap-px pl-1.5" hidden={!props.expanded}>
        {!props.expanded ? null : visible.length === 0 ? (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {hasRawSearchMatches
              ? `Matches in ${title} fall outside the first ${GLOBAL_SEARCH_RESULT_BUDGET} results.`
              : items.length > 0 && props.search.trim()
                ? `No ${title.toLowerCase()} match “${props.search.trim()}”.`
                : empty}
          </p>
        ) : (
          rendered.map((item) => (
            <ObjectRow
              key={item.id}
              item={item}
              meta={
                item.kind === 'players'
                  ? [item.level ? `Lv ${item.level}` : '', item.guildName || ''].filter(Boolean).join(' · ')
                  : item.level
                    ? `Lv ${item.level}`
                    : item.kind === 'npcs' || item.kind === 'npc-locations'
                      ? item.detail
                      : undefined
              }
              {...props}
            />
          ))
        )}
        {props.expanded && !searchResultKeys && rendered.length < visible.length && (
          <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
            {props.search.trim()
              ? `${visible.length - rendered.length} more matches. Refine your search to inspect them.`
              : `Search to inspect ${visible.length - rendered.length} more ${title.toLowerCase()}.`}
          </p>
        )}
      </div>
    </section>
  )
}

interface GuildCategoryProps extends ExplorerProps {
  guildData?: GuildData
  controlItems: MapItem[]
  guildCount: number
  bases: MapItem[]
  workers: MapItem[]
  workersByBaseId: Map<string, MapItem[]>
  matches: (item: MapItem) => boolean
  searchResultKeys?: ReadonlySet<string>
  expanded: boolean
  onToggleExpanded: () => void
}

function GuildCategory({
  guildData,
  controlItems,
  guildCount,
  expanded,
  onToggleExpanded,
  ...props
}: GuildCategoryProps) {
  const contentId = useId()
  return (
    <section className="border-b border-white/7 py-0.5">
      <CategoryHeader
        {...props}
        group="bases"
        title="Guilds"
        items={controlItems}
        count={guildCount}
        expanded={expanded}
        onToggleExpanded={onToggleExpanded}
        controls={contentId}
      />
      <div id={contentId} className="grid gap-px pl-1" hidden={!expanded}>
        {expanded && guildData ? (
          <GuildCategoryBody
            {...props}
            guildData={guildData}
            controlItems={controlItems}
            guildCount={guildCount}
            expanded={expanded}
            onToggleExpanded={onToggleExpanded}
            contentId={contentId}
          />
        ) : null}
      </div>
    </section>
  )
}

function GuildCategoryBody({
  guildData,
  bases,
  workers,
  workersByBaseId,
  matches,
  searchResultKeys,
  contentId,
  ...props
}: Omit<GuildCategoryProps, 'guildData'> & { guildData: GuildData; contentId: string }) {
  const { guilds, fallbackWorkers } = guildData
  const searchQuery = props.search.trim().toLowerCase()
  const hasRawSearchMatches =
    Boolean(searchResultKeys) &&
    (guilds.some((guild) => {
      if (guild.displayName.toLowerCase().includes(searchQuery)) return true
      return (
        guild.outsideWorkers.some(matches) ||
        guild.bases.some((base) => {
          const baseWorkers = workersByBaseId.get(base.id) || workersByBaseId.get(base.baseId || '') || []
          return matches(base) || baseWorkers.some(matches)
        })
      )
    }) ||
      (fallbackWorkers.length > 0 &&
        ('no linked guild outside base perimeters'.includes(searchQuery) || fallbackWorkers.some(matches))))
  let rendered = 0
  let eligibleBaseWorkers = 0
  let renderedBaseWorkers = 0
  let eligibleOutsideWorkers = 0
  let renderedOutsideWorkers = 0

  return (
    <>
      {guilds.map((guild) => {
        const displayName = guild.displayName
        const guildMatches = displayName.toLowerCase().includes(searchQuery)
        const matchingOutsideWorkers = guildMatches ? guild.outsideWorkers : guild.outsideWorkers.filter(matches)
        const requestedOutsideWorkers = props.search.trim() ? matchingOutsideWorkers : guild.outsideWorkers
        const selectedOutsideWorkers = searchResultKeys
          ? requestedOutsideWorkers.filter((worker) => searchResultKeys.has(searchResultKey(worker)))
          : requestedOutsideWorkers
        const entries = guild.bases
          .map((base, index) => {
            const baseWorkers = (workersByBaseId.get(base.id) || workersByBaseId.get(base.baseId || '') || [])
              .slice()
              .sort((left, right) => left.name.localeCompare(right.name))
            const matchingWorkers = guildMatches ? baseWorkers : baseWorkers.filter(matches)
            return {
              base,
              baseWorkers,
              index,
              matchingWorkers: searchResultKeys
                ? matchingWorkers.filter((worker) => searchResultKeys.has(searchResultKey(worker)))
                : matchingWorkers,
              baseIsResult: !searchResultKeys || searchResultKeys.has(searchResultKey(base))
            }
          })
          .filter(({ base, baseIsResult, matchingWorkers }) =>
            searchResultKeys
              ? baseIsResult || matchingWorkers.length > 0
              : guildMatches || matches(base) || matchingWorkers.length > 0
          )
        if (entries.length === 0 && selectedOutsideWorkers.length === 0) return null
        rendered++
        const guildItems = [
          ...guild.bases.flatMap((base) => [
            base,
            ...(workersByBaseId.get(base.id) || workersByBaseId.get(base.baseId || '') || [])
          ]),
          ...guild.outsideWorkers
        ]
        const workerCount = guildItems.filter((item) => item.kind === 'workers').length
        const expanded = props.expandedGuilds.has(guild.id) || Boolean(props.search.trim())
        const guildContentId = `${contentId}-guild-${rendered}`
        let displayedOutsideWorkers: MapItem[] = []
        if (expanded) {
          eligibleOutsideWorkers += selectedOutsideWorkers.length
          const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedOutsideWorkers)
          displayedOutsideWorkers = selectedOutsideWorkers.slice(0, remaining)
          renderedOutsideWorkers += displayedOutsideWorkers.length
        }
        return (
          <div key={guild.id}>
            <div className="flex min-h-8 items-center gap-0.5">
              <span className="grid size-8 shrink-0 place-items-center">
                <Checkbox
                  state={visibilityState(guildItems, props.enabledKinds, props.enabledPlayerStatuses, props.hiddenIds)}
                  label={`Show guild ${displayName}`}
                  onChange={(visible) =>
                    props.onToggleItems(
                      guildItems.map((item) => item.id),
                      visible
                    )
                  }
                />
              </span>
              <GuildButton
                guildId={guild.id}
                name={displayName}
                meta={`${guild.bases.length} base${guild.bases.length === 1 ? '' : 's'} · ${workerCount} Pal${workerCount === 1 ? '' : 's'}`}
                onFocus={props.onFocusGuild}
              />
              <DisclosureToggle
                expanded={expanded}
                label={displayName}
                controls={guildContentId}
                onClick={() => props.onToggleGuild(guild.id)}
              />
            </div>
            <div id={guildContentId} className="ml-3 border-l border-white/10 pl-2" hidden={!expanded}>
              {expanded &&
                entries.map(({ base, baseWorkers, index, matchingWorkers }) => {
                  const baseExpanded = props.expandedBases.has(base.id) || Boolean(props.search.trim())
                  const baseItems = [base, ...baseWorkers]
                  const baseLabel = guild.bases.length === 1 ? 'Base' : `Base ${index + 1}`
                  const baseContentId = `${guildContentId}-base-${index}`
                  const requestedWorkers = props.search.trim() ? matchingWorkers : baseWorkers
                  let displayedWorkers: MapItem[] = []
                  if (baseExpanded) {
                    eligibleBaseWorkers += requestedWorkers.length
                    const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedBaseWorkers)
                    displayedWorkers = requestedWorkers.slice(0, remaining)
                    renderedBaseWorkers += displayedWorkers.length
                  }
                  return (
                    <div key={base.id}>
                      <div className="flex min-h-8 min-w-0 items-center gap-0.5">
                        <span className="grid size-8 shrink-0 place-items-center">
                          <Checkbox
                            state={visibilityState(
                              baseItems,
                              props.enabledKinds,
                              props.enabledPlayerStatuses,
                              props.hiddenIds
                            )}
                            label={`Show ${baseLabel} for ${displayName}`}
                            onChange={(visible) =>
                              props.onToggleItems(
                                baseItems.map((item) => item.id),
                                visible
                              )
                            }
                          />
                        </span>
                        <ItemButton
                          item={base}
                          label={baseLabel}
                          meta={`${baseWorkers.length} assigned Pal${baseWorkers.length === 1 ? '' : 's'}`}
                          onFocus={props.onFocusItem}
                        />
                        <DisclosureToggle
                          expanded={baseExpanded}
                          label={`${displayName} ${baseLabel}`}
                          controls={baseContentId}
                          onClick={() => props.onToggleBase(base.id)}
                        />
                      </div>
                      <div id={baseContentId} className="ml-3 border-l border-white/8 pl-2" hidden={!baseExpanded}>
                        {baseExpanded &&
                          displayedWorkers.map((worker) => (
                            <ObjectRow
                              key={worker.id}
                              item={worker}
                              meta={worker.level ? `Lv ${worker.level}` : undefined}
                              {...props}
                            />
                          ))}
                      </div>
                    </div>
                  )
                })}
              {expanded && selectedOutsideWorkers.length > 0 ? (
                <fieldset className="m-0 mt-1 min-w-0 border-0 border-t border-white/10 p-0 pt-1">
                  <legend className="sr-only">Outside base perimeters for {displayName}</legend>
                  <h4 className="m-0 px-2 py-1 text-[10px] font-normal tracking-[.1em] text-[#8eb8bf] uppercase">
                    Outside base perimeters
                  </h4>
                  <div className="grid gap-px">
                    {displayedOutsideWorkers.map((worker) => (
                      <ObjectRow
                        key={worker.id}
                        item={worker}
                        meta={worker.level ? `Lv ${worker.level}` : undefined}
                        className="pl-1"
                        {...props}
                      />
                    ))}
                  </div>
                </fieldset>
              ) : null}
            </div>
          </div>
        )
      })}
      {!searchResultKeys && renderedBaseWorkers < eligibleBaseWorkers && (
        <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
          {eligibleBaseWorkers - renderedBaseWorkers} more assigned Pal
          {eligibleBaseWorkers - renderedBaseWorkers === 1 ? '' : 's'} omitted. Refine your search or expand fewer bases
          to inspect them.
        </p>
      )}
      {(() => {
        const fallbackMatches = 'no linked guild outside base perimeters'.includes(searchQuery)
        const matchingWorkers = fallbackMatches ? fallbackWorkers : fallbackWorkers.filter(matches)
        const requestedWorkers = props.search.trim() ? matchingWorkers : fallbackWorkers
        const selectedWorkers = searchResultKeys
          ? requestedWorkers.filter((worker) => searchResultKeys.has(searchResultKey(worker)))
          : requestedWorkers
        if (selectedWorkers.length === 0) return null
        rendered++
        eligibleOutsideWorkers += selectedWorkers.length
        const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedOutsideWorkers)
        const displayedWorkers = selectedWorkers.slice(0, remaining)
        renderedOutsideWorkers += displayedWorkers.length
        return (
          <fieldset className="m-0 min-w-0 border-0 p-0">
            <legend className="sr-only">Pals with no linked guild</legend>
            <div className="flex min-h-8 items-center gap-0.5">
              <span className="grid size-8 shrink-0 place-items-center">
                <Checkbox
                  state={visibilityState(
                    fallbackWorkers,
                    props.enabledKinds,
                    props.enabledPlayerStatuses,
                    props.hiddenIds
                  )}
                  label="Show Pals with no linked guild"
                  onChange={(visible) =>
                    props.onToggleItems(
                      fallbackWorkers.map((worker) => worker.id),
                      visible
                    )
                  }
                />
              </span>
              <div className="grid min-h-7 min-w-0 flex-1 grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-1.5 px-1.5 py-1 text-xs text-[#cbd7d9]">
                <MarkerGlyph kind="workers" />
                <strong className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-medium">
                  No linked guild
                </strong>
                <span className="ml-auto shrink-0 text-[10px] text-[#7f898e]">
                  {fallbackWorkers.length} Pal{fallbackWorkers.length === 1 ? '' : 's'}
                </span>
              </div>
            </div>
            <div className="ml-3 border-l border-white/10 pl-2">
              <h4 className="m-0 px-2 py-1 text-[10px] font-normal tracking-[.1em] text-[#8eb8bf] uppercase">
                Outside base perimeters
              </h4>
              <div className="grid gap-px">
                {displayedWorkers.map((worker) => (
                  <ObjectRow
                    key={worker.id}
                    item={worker}
                    meta={worker.level ? `Lv ${worker.level}` : undefined}
                    className="pl-1"
                    {...props}
                  />
                ))}
              </div>
            </div>
          </fieldset>
        )
      })()}
      {!searchResultKeys && renderedOutsideWorkers < eligibleOutsideWorkers && (
        <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
          {eligibleOutsideWorkers - renderedOutsideWorkers} more Pal
          {eligibleOutsideWorkers - renderedOutsideWorkers === 1 ? '' : 's'} outside base perimeters omitted. Refine
          your search or expand fewer guilds to inspect them.
        </p>
      )}
      {rendered === 0 && (
        <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
          {hasRawSearchMatches
            ? `Guild matches fall outside the first ${GLOBAL_SEARCH_RESULT_BUDGET} results.`
            : props.search.trim() && (bases.length > 0 || workers.length > 0)
              ? `No guilds match “${props.search.trim()}”.`
              : 'No guilds are currently available.'}
        </p>
      )}
    </>
  )
}

function AccordionButton({
  expanded,
  label,
  controls,
  onClick,
  children
}: {
  expanded: boolean
  label: string
  controls: string
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      className="pal-interactive group flex min-h-8 min-w-0 flex-1 cursor-pointer items-center gap-1.5 rounded border border-transparent bg-transparent px-1 text-left text-[#e3edef]"
      aria-label={`${expanded ? 'Collapse' : 'Expand'} ${label}`}
      aria-expanded={expanded}
      aria-controls={controls}
      onClick={onClick}
    >
      {children}
      <span className="ml-auto grid size-8 shrink-0 place-items-center text-[#929da1] group-hover:text-white">
        <Chevron expanded={expanded} />
      </span>
    </button>
  )
}

function DisclosureToggle({
  expanded,
  label,
  controls,
  onClick
}: {
  expanded: boolean
  label: string
  controls: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className="pal-interactive grid size-8 shrink-0 cursor-pointer place-items-center rounded border border-transparent bg-transparent text-[#929da1]"
      aria-label={`${expanded ? 'Collapse' : 'Expand'} ${label}`}
      aria-expanded={expanded}
      aria-controls={controls}
      onClick={onClick}
    >
      <Chevron expanded={expanded} />
    </button>
  )
}

function Chevron({ expanded }: { expanded: boolean }) {
  return (
    <IconChevronRight className={`size-4 transition-transform ${expanded ? 'rotate-90' : ''}`} aria-hidden="true" />
  )
}
