import { type ReactNode, type RefObject, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { guildIdForBase } from '../lib/guilds'
import { itemSearchText, markerText } from '../lib/map'
import type { ItemKind, MapItem, MapLayer, PlayerStatus } from '../types'
import { MarkerGlyph } from './MarkerGlyph'

// Funnel icon from Heroicons (MIT): https://heroicons.com/
function FilterIcon() {
  return (
    <svg
      className="size-6"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M12 3c2.755 0 5.455.232 8.083.678A1.112 1.112 0 0 1 21 4.774v1.044c0 .597-.237 1.169-.659 1.591l-5.432 5.432a2.25 2.25 0 0 0-.659 1.591v2.927c0 .853-.482 1.632-1.244 2.013L9.75 21v-6.568c0-.597-.237-1.169-.659-1.591L3.659 7.409A2.25 2.25 0 0 1 3 5.818V4.774c0-.54.384-1.006.917-1.096A48.5 48.5 0 0 1 12 3Z" />
    </svg>
  )
}

interface ExplorerProps {
  open: boolean
  activeLayer: MapLayer
  layers: MapLayer[]
  items: MapItem[]
  search: string
  searchInputRef: RefObject<HTMLInputElement | null>
  enabledKinds: Set<ItemKind>
  enabledPlayerStatuses: Set<PlayerStatus>
  hiddenIds: Set<string>
  expandedGuilds: Set<string>
  expandedBases: Set<string>
  objectNotice: string | null
  onSearchChange: (value: string) => void
  onToggleKinds: (kinds: ItemKind[], visible: boolean) => void
  onTogglePlayerStatus: (status: PlayerStatus, visible: boolean) => void
  onToggleItems: (ids: string[], visible: boolean) => void
  onToggleGuild: (id: string) => void
  onToggleBase: (id: string) => void
  onFocusItem: (item: MapItem, returnFocus: HTMLElement) => void
  onFocusGuild: (guildId: string, returnFocus: HTMLElement) => void
  onClose: () => void
  onOpen: () => void
  onLayerChange: (layer: MapLayer) => void
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

type PlayerCategoryGroup = 'online-players' | 'offline-players'
type CategoryGroup = PlayerCategoryGroup | Exclude<ItemKind, 'players' | 'workers'>
type NonPlayerCategoryGroup = Exclude<CategoryGroup, PlayerCategoryGroup>

const GROUP_KINDS: Record<NonPlayerCategoryGroup, ItemKind[]> = {
  bases: ['bases', 'workers'],
  companions: ['companions'],
  'wild-pals': ['wild-pals'],
  'alpha-pals': ['alpha-pals'],
  bosses: ['bosses'],
  npcs: ['npcs']
}

const DEFAULT_COLLAPSED_GROUPS: CategoryGroup[] = ['companions', 'wild-pals', 'alpha-pals', 'bosses', 'npcs']

const INITIAL_CATEGORY_ITEMS = 250

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
    disabled: items.length === 0 || !items.some(enabled)
  }
}

function ItemButton({
  item,
  meta,
  label,
  onFocus
}: {
  item: MapItem
  meta?: string
  label?: string
  onFocus: ExplorerProps['onFocusItem']
}) {
  return (
    <button
      type="button"
      className="pal-interactive grid min-h-7 w-full min-w-0 flex-1 cursor-pointer grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-1.5 border border-transparent bg-transparent px-1.5 py-1 text-left text-xs text-[#e3edef] focus-visible:outline-none"
      aria-label={`View ${markerText(item)}`}
      title={item.detail}
      onClick={(event) => onFocus(item, event.currentTarget)}
    >
      <MarkerGlyph kind={item.kind} online={item.online} />
      <span className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">{label || item.name}</span>
      {meta && <span className="ml-auto shrink-0 text-[11px] text-[#899398]">{meta}</span>}
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
  onToggleItems,
  onFocusItem,
  className = ''
}: Pick<ExplorerProps, 'enabledKinds' | 'enabledPlayerStatuses' | 'hiddenIds' | 'onToggleItems' | 'onFocusItem'> & {
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
      <ItemButton item={item} meta={meta} label={label} onFocus={onFocusItem} />
    </div>
  )
}

export function Explorer(props: ExplorerProps) {
  const reopenRef = useRef<HTMLButtonElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const wasOpen = useRef(props.open)
  const [collapsedGroups, setCollapsedGroups] = useState<Set<CategoryGroup>>(() => new Set(DEFAULT_COLLAPSED_GROUPS))

  useLayoutEffect(() => {
    if (wasOpen.current === props.open) return
    wasOpen.current = props.open
    ;(props.open ? closeRef.current : reopenRef.current)?.focus({ preventScroll: true })
  }, [props.open])

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
      npcs: []
    }
    const baseById = new Map<string, MapItem>()
    const workersByBaseId = new Map<string, MapItem[]>()
    for (const item of props.items) {
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
    }
    return { byKind, baseById, workersByBaseId }
  }, [props.activeLayer.id, props.items])

  const query = props.search.trim().toLowerCase()
  const searching = Boolean(query)
  const matches = (item: MapItem) => {
    if (!query) return true
    const baseName = item.kind === 'workers' && item.baseId ? index.baseById.get(item.baseId)?.name || '' : ''
    return itemSearchText(item, baseName).includes(query)
  }
  const onlinePlayers = index.byKind.players.filter((player) => player.online !== false)
  const offlinePlayers = index.byKind.players.filter((player) => player.online === false)

  return (
    <>
      <button
        ref={reopenRef}
        type="button"
        className={`pal-glass-control filter-trigger-motion absolute top-[78px] left-4 z-30 grid size-12 cursor-pointer place-items-center text-[#dceef0] max-sm:top-[88px] max-sm:left-3 max-sm:size-11 ${
          props.open ? 'is-panel-open pointer-events-none' : 'is-panel-closed'
        }`}
        aria-label={
          props.search.trim() ? `Open map filters, current search: ${props.search.trim()}` : 'Open map filters'
        }
        aria-controls="map-filter-panel"
        aria-expanded="false"
        aria-hidden={props.open}
        inert={props.open}
        tabIndex={props.open ? -1 : 0}
        onClick={props.onOpen}
      >
        <FilterIcon />
        {props.search.trim() ? (
          <span
            className="absolute top-1.5 right-1.5 size-1.5 rounded-full bg-[#55d4e7] shadow-[0_0_5px_rgb(85_212_231/65%)]"
            aria-hidden="true"
          />
        ) : null}
      </button>
      <aside
        id="map-filter-panel"
        className={`filter-panel-motion absolute top-[78px] bottom-4 left-4 z-[24] flex w-[350px] min-h-0 shrink-0 flex-col max-sm:top-auto max-sm:right-3 max-sm:bottom-3 max-sm:left-3 max-sm:z-[34] max-sm:h-[min(52dvh,480px)] max-sm:w-auto ${props.open ? 'is-panel-open' : 'is-panel-closed pointer-events-none'}`}
        aria-label="Map filters"
        aria-hidden={!props.open}
        inert={!props.open}
      >
        <div
          className={`pal-panel-header filter-panel-header-motion relative z-[1] flex min-h-[78px] shrink-0 items-center justify-between gap-3.5 border pr-3.5 pl-5 [--pal-panel-accent:#72d7e5] ${props.open ? 'is-panel-open' : 'is-panel-closed'}`}
        >
          <div>
            <p className="m-0 mb-1 text-[10px] font-normal tracking-[.14em] text-[#b6f5fc]">MAP FILTER</p>
            <h2 className="m-0 text-[22px] font-normal text-[#f3fbfc]">Map</h2>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="pal-interactive grid size-11 cursor-pointer place-items-center border-0 bg-transparent text-xl text-[#d7eef1]"
            aria-label="Collapse map filter"
            aria-controls="map-filter-panel"
            aria-expanded="true"
            title="Collapse map filter"
            onClick={props.onClose}
          >
            ×
          </button>
        </div>

        <div
          className={`filter-panel-body-motion relative flex min-h-0 flex-1 flex-col overflow-hidden ${props.open ? 'is-panel-open' : 'is-panel-closed'}`}
        >
          <div className="filter-panel-body-content relative z-[1] flex min-h-0 flex-1 flex-col overflow-hidden">
            <search
              id="map-search-control"
              aria-label="Map search"
              className="pal-glass-inset relative mx-3.5 mt-3 flex h-11 shrink-0 items-center text-[#dceef0] transition-[border-color,box-shadow] focus-within:border-[#62d6e7] focus-within:shadow-[inset_0_-2px_#22c7e8]"
            >
              <span className="grid size-10 shrink-0 place-items-center text-[#65bbc7]" aria-hidden="true">
                <svg
                  className="size-[18px]"
                  viewBox="0 0 20 20"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.7"
                  strokeLinecap="round"
                  aria-hidden="true"
                >
                  <circle cx="8.5" cy="8.5" r="4.75" />
                  <path d="m12 12 4 4" />
                </svg>
              </span>
              <label className="sr-only" htmlFor="map-search">
                Search players, bases, Pals and bosses
              </label>
              <input
                id="map-search"
                ref={props.searchInputRef}
                type="search"
                aria-label="Search players, bases, Pals and bosses"
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
                  ×
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

            <div
              className="min-h-0 flex-1 overflow-y-auto border-t border-[#caeaef]/20 px-3.5 pt-1.5 pb-3.5"
              aria-live="polite"
            >
              {props.search && (
                <p className="mt-px mb-2 text-[11px] text-[#688088]">
                  Results for <strong className="font-medium text-[#9ec1c7]">“{props.search}”</strong>
                </p>
              )}
              <SimpleCategory
                {...props}
                group="online-players"
                title="Online Players"
                items={onlinePlayers}
                matches={matches}
                empty="No players are currently online in this region."
                expanded={searching || !collapsedGroups.has('online-players')}
                onToggleExpanded={() => toggleCategory('online-players')}
              />
              <SimpleCategory
                {...props}
                group="offline-players"
                title="Offline Players"
                items={offlinePlayers}
                matches={matches}
                empty="No saved offline players are loaded for this region."
                expanded={searching || !collapsedGroups.has('offline-players')}
                onToggleExpanded={() => toggleCategory('offline-players')}
              />
              <GuildCategory
                {...props}
                bases={index.byKind.bases}
                workers={index.byKind.workers}
                workersByBaseId={index.workersByBaseId}
                matches={matches}
                expanded={searching || !collapsedGroups.has('bases')}
                onToggleExpanded={() => toggleCategory('bases')}
              />
              <SimpleCategory
                {...props}
                group="companions"
                title="Companion Pals"
                items={index.byKind.companions}
                matches={matches}
                empty="No companion Pals are currently loaded."
                expanded={searching || !collapsedGroups.has('companions')}
                onToggleExpanded={() => toggleCategory('companions')}
              />
              <SimpleCategory
                {...props}
                group="wild-pals"
                title="Wild Pals"
                items={index.byKind['wild-pals']}
                matches={matches}
                empty="No wild Pals are currently loaded."
                expanded={searching || !collapsedGroups.has('wild-pals')}
                onToggleExpanded={() => toggleCategory('wild-pals')}
              />
              <SimpleCategory
                {...props}
                group="alpha-pals"
                title="Alpha Pals"
                items={index.byKind['alpha-pals']}
                matches={matches}
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
                empty="No Tower Boss landmarks are loaded for this region."
                expanded={searching || !collapsedGroups.has('bosses')}
                onToggleExpanded={() => toggleCategory('bosses')}
              />
              <SimpleCategory
                {...props}
                group="npcs"
                title="NPCs"
                items={index.byKind.npcs}
                matches={matches}
                empty="No NPCs are currently loaded."
                expanded={searching || !collapsedGroups.has('npcs')}
                onToggleExpanded={() => toggleCategory('npcs')}
              />
            </div>

            {props.objectNotice && (
              <p className="m-3 mt-1 rounded-md border border-[#554b37] bg-[#302b22] px-2.5 py-2 text-[11px] leading-4 text-[#d2b980]">
                {props.objectNotice}
              </p>
            )}
          </div>
        </div>
      </aside>
    </>
  )
}

interface CategoryProps extends ExplorerProps {
  group: CategoryGroup
  title: string
  matches: (item: MapItem) => boolean
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

function SimpleCategory({
  group,
  title,
  items,
  controlItems = items,
  count,
  matches,
  empty,
  ...props
}: CategoryProps & { items: MapItem[]; controlItems?: MapItem[]; count?: number }) {
  const visible = items.filter(matches).sort((left, right) => left.name.localeCompare(right.name))
  const rendered = visible.slice(0, INITIAL_CATEGORY_ITEMS)
  const contentId = useId()
  return (
    <section className="border-b border-white/7 py-0.5 last:border-b-0">
      <CategoryHeader {...props} group={group} title={title} items={controlItems} count={count} controls={contentId} />
      <div id={contentId} className="grid gap-px pl-1.5" hidden={!props.expanded}>
        {visible.length === 0 ? (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {items.length > 0 && props.search.trim()
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
                    : item.kind === 'npcs'
                      ? item.detail
                      : undefined
              }
              {...props}
            />
          ))
        )}
        {rendered.length < visible.length && (
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
  bases: MapItem[]
  workers: MapItem[]
  workersByBaseId: Map<string, MapItem[]>
  matches: (item: MapItem) => boolean
  expanded: boolean
  onToggleExpanded: () => void
}

function GuildCategory({ bases, workers, workersByBaseId, matches, ...props }: GuildCategoryProps) {
  const sortedBases = bases
    .slice()
    .sort((left, right) => left.name.localeCompare(right.name) || left.x - right.x || left.y - right.y)
  const guildNames = new Map<string, string>()
  for (const item of [...bases, ...workers]) {
    if (item.guildKey && item.guildName) guildNames.set(item.guildKey, item.guildName)
  }
  interface GuildBucket {
    id: string
    name: string
    bases: MapItem[]
    outsideWorkers: MapItem[]
  }
  const guildMap = new Map<string, GuildBucket>()
  const newGuild = (id: string, name = guildNames.get(id) || 'Unnamed guild'): GuildBucket => ({
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
    guild.outsideWorkers.sort((left, right) => left.name.localeCompare(right.name))
  }
  fallbackWorkers.sort((left, right) => left.name.localeCompare(right.name))
  const guilds = Array.from(guildMap.values()).sort((left, right) => left.name.localeCompare(right.name))
  const names = new Map<string, number>()
  for (const guild of guilds) names.set(guild.name, (names.get(guild.name) || 0) + 1)
  const occurrences = new Map<string, number>()
  let rendered = 0
  let eligibleBaseWorkers = 0
  let renderedBaseWorkers = 0
  let eligibleOutsideWorkers = 0
  let renderedOutsideWorkers = 0
  const contentId = useId()

  return (
    <section className="border-b border-white/7 py-0.5">
      <CategoryHeader
        {...props}
        group="bases"
        title="Guilds"
        items={[...bases, ...workers]}
        count={guilds.length}
        controls={contentId}
      />
      <div id={contentId} className="grid gap-px pl-1" hidden={!props.expanded}>
        {guilds.map((guild) => {
          const occurrence = (occurrences.get(guild.name) || 0) + 1
          occurrences.set(guild.name, occurrence)
          const displayName = (names.get(guild.name) || 0) > 1 ? `${guild.name} #${occurrence}` : guild.name
          const guildMatches = displayName.toLowerCase().includes(props.search.trim().toLowerCase())
          const matchingOutsideWorkers = guildMatches ? guild.outsideWorkers : guild.outsideWorkers.filter(matches)
          const entries = guild.bases
            .map((base, index) => {
              const baseWorkers = (workersByBaseId.get(base.id) || workersByBaseId.get(base.baseId || '') || [])
                .slice()
                .sort((left, right) => left.name.localeCompare(right.name))
              return {
                base,
                baseWorkers,
                index,
                matchingWorkers: guildMatches ? baseWorkers : baseWorkers.filter(matches)
              }
            })
            .filter(({ base, matchingWorkers }) => guildMatches || matches(base) || matchingWorkers.length > 0)
          if (entries.length === 0 && matchingOutsideWorkers.length === 0) return null
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
          const requestedOutsideWorkers = props.search.trim() ? matchingOutsideWorkers : guild.outsideWorkers
          let displayedOutsideWorkers: MapItem[] = []
          if (expanded) {
            eligibleOutsideWorkers += requestedOutsideWorkers.length
            const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedOutsideWorkers)
            displayedOutsideWorkers = requestedOutsideWorkers.slice(0, remaining)
            renderedOutsideWorkers += displayedOutsideWorkers.length
          }
          return (
            <div key={guild.id}>
              <div className="flex min-h-8 items-center gap-0.5">
                <span className="grid size-8 shrink-0 place-items-center">
                  <Checkbox
                    state={visibilityState(
                      guildItems,
                      props.enabledKinds,
                      props.enabledPlayerStatuses,
                      props.hiddenIds
                    )}
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
                {expanded && requestedOutsideWorkers.length > 0 ? (
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
        {renderedBaseWorkers < eligibleBaseWorkers && (
          <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
            {eligibleBaseWorkers - renderedBaseWorkers} more assigned Pal
            {eligibleBaseWorkers - renderedBaseWorkers === 1 ? '' : 's'} omitted. Refine your search or expand fewer
            bases to inspect them.
          </p>
        )}
        {(() => {
          const fallbackMatches = 'no linked guild outside base perimeters'.includes(props.search.trim().toLowerCase())
          const matchingWorkers = fallbackMatches ? fallbackWorkers : fallbackWorkers.filter(matches)
          const requestedWorkers = props.search.trim() ? matchingWorkers : fallbackWorkers
          if (requestedWorkers.length === 0) return null
          rendered++
          eligibleOutsideWorkers += requestedWorkers.length
          const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedOutsideWorkers)
          const displayedWorkers = requestedWorkers.slice(0, remaining)
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
        {renderedOutsideWorkers < eligibleOutsideWorkers && (
          <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
            {eligibleOutsideWorkers - renderedOutsideWorkers} more Pal
            {eligibleOutsideWorkers - renderedOutsideWorkers === 1 ? '' : 's'} outside base perimeters omitted. Refine
            your search or expand fewer guilds to inspect them.
          </p>
        )}
        {rendered === 0 && (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {props.search.trim() && (bases.length > 0 || workers.length > 0)
              ? `No guilds match “${props.search.trim()}”.`
              : 'No guilds are currently available.'}
          </p>
        )}
      </div>
    </section>
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
    <svg
      className={`size-4 transition-transform ${expanded ? 'rotate-90' : ''}`}
      viewBox="0 0 20 20"
      fill="none"
      aria-hidden="true"
    >
      <path d="m7 4 6 6-6 6" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
