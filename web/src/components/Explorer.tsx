import { type ReactNode, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { guildIdForBase } from '../lib/guilds'
import { markerText } from '../lib/map'
import type { ItemKind, MapItem, MapLayer } from '../types'

interface ExplorerProps {
  open: boolean
  activeLayer: MapLayer
  layers: MapLayer[]
  items: MapItem[]
  search: string
  enabledKinds: Set<ItemKind>
  hiddenIds: Set<string>
  expandedPlayers: Set<string>
  expandedGuilds: Set<string>
  expandedBases: Set<string>
  objectNotice: string | null
  onToggleKinds: (kinds: ItemKind[], visible: boolean) => void
  onToggleItems: (ids: string[], visible: boolean) => void
  onTogglePlayer: (id: string) => void
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

type CategoryGroup = Exclude<ItemKind, 'workers' | 'companions'>

const GROUP_KINDS: Record<CategoryGroup, ItemKind[]> = {
  players: ['players', 'companions'],
  bases: ['bases', 'workers'],
  'wild-pals': ['wild-pals'],
  npcs: ['npcs']
}

const INITIAL_CATEGORY_ITEMS = 250

function visibilityState(items: MapItem[], enabledKinds: Set<ItemKind>, hiddenIds: Set<string>): CheckState {
  const visible = items.filter((item) => enabledKinds.has(item.kind) && !hiddenIds.has(item.id)).length
  return {
    checked: items.length > 0 && visible === items.length,
    indeterminate: visible > 0 && visible < items.length,
    disabled: items.length === 0 || !items.some((item) => enabledKinds.has(item.kind))
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
      className="grid min-h-7 w-full min-w-0 flex-1 cursor-pointer grid-cols-[13px_minmax(0,1fr)_auto] items-center gap-1.5 border border-transparent bg-transparent px-1.5 py-1 text-left text-xs text-[#e3edef] transition-colors hover:border-[#c5edf3]/35 hover:bg-[#216d82]/35 hover:text-white focus-visible:border-[#c5edf3]/35 focus-visible:bg-[#216d82]/35 focus-visible:text-white focus-visible:outline-none"
      aria-label={`View ${markerText(item)}`}
      title={item.detail}
      onClick={(event) => onFocus(item, event.currentTarget)}
    >
      <span className={`explorer-symbol kind-${item.kind}`} aria-hidden="true" />
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
      className="grid min-h-7 min-w-0 flex-1 cursor-pointer grid-cols-[13px_minmax(0,1fr)_auto] items-center gap-1.5 border border-transparent bg-transparent px-1.5 py-1 text-left text-xs text-[#e3edef] transition-colors hover:border-[#c5edf3]/35 hover:bg-[#216d82]/35 hover:text-white focus-visible:border-[#c5edf3]/35 focus-visible:bg-[#216d82]/35 focus-visible:text-white focus-visible:outline-none"
      aria-label={`View guild ${name}`}
      onClick={(event) => onFocus(guildId, event.currentTarget)}
    >
      <span className="explorer-symbol kind-bases" aria-hidden="true" />
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
  hiddenIds,
  onToggleItems,
  onFocusItem,
  className = ''
}: Pick<ExplorerProps, 'enabledKinds' | 'hiddenIds' | 'onToggleItems' | 'onFocusItem'> & {
  item: MapItem
  meta?: string
  label?: string
  className?: string
}) {
  return (
    <div className={`flex min-w-0 items-center gap-0.5 ${className}`}>
      <span className="grid size-8 shrink-0 place-items-center">
        <Checkbox
          state={visibilityState([item], enabledKinds, hiddenIds)}
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
  const [collapsedGroups, setCollapsedGroups] = useState<Set<CategoryGroup>>(() => new Set())

  useLayoutEffect(() => {
    if (wasOpen.current === props.open) return
    wasOpen.current = props.open
    ;(props.open ? closeRef.current : reopenRef.current)?.focus({ preventScroll: true })
  }, [props.open])

  useEffect(() => {
    if (!props.search.trim()) return
    setCollapsedGroups((current) => (current.size === 0 ? current : new Set()))
  }, [props.search])

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
  const matches = (item: MapItem) => {
    if (!query) return true
    const baseName = item.kind === 'workers' && item.baseId ? index.baseById.get(item.baseId)?.name || '' : ''
    return `${item.name} ${item.detail || ''} ${item.level || ''} ${item.guildName || ''} ${baseName}`
      .toLowerCase()
      .includes(query)
  }

  return (
    <>
      <button
        ref={reopenRef}
        type="button"
        className={`filter-trigger-motion absolute top-4 left-4 z-30 grid h-11 w-[124px] cursor-pointer grid-cols-[16px_auto] items-center justify-center gap-2 border border-[#cceaef]/40 bg-[#081115]/95 px-3 text-xs leading-none text-[#dceef0] shadow-[0_8px_22px_rgb(0_0_0/24%)] hover:border-[#64d7e7] hover:bg-[#17343d] max-sm:top-3 max-sm:left-3 ${
          props.open ? 'is-panel-open pointer-events-none' : 'is-panel-closed'
        }`}
        aria-label="Open map filters"
        aria-controls="map-filter-panel"
        aria-expanded="false"
        aria-hidden={props.open}
        inert={props.open}
        tabIndex={props.open ? -1 : 0}
        onClick={props.onOpen}
      >
        <svg
          className="block size-4"
          viewBox="0 0 20 20"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.6"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="m7 4 6 6-6 6" />
        </svg>
        <span>Map filter</span>
      </button>
      <aside
        id="map-filter-panel"
        className={`absolute inset-y-4 left-4 z-[24] flex w-[350px] min-h-0 shrink-0 flex-col max-sm:inset-y-3 max-sm:left-3 max-sm:z-[34] max-sm:w-[min(350px,calc(100vw-24px))] ${props.open ? '' : 'pointer-events-none'}`}
        aria-label="Map filters"
        aria-hidden={!props.open}
        inert={!props.open}
      >
        <div
          className={`filter-panel-header-motion relative z-[1] flex min-h-[70px] shrink-0 items-center justify-between border border-[#c4e4e9]/25 bg-[linear-gradient(90deg,#24b8dd_0_4px,rgb(25_40_47/95%)_4px_100%)] pr-3.5 pl-[18px] ${props.open ? 'is-panel-open' : 'is-panel-closed'}`}
        >
          <div>
            <span className="mb-[3px] block text-[10px] font-semibold tracking-[.14em] text-[#79bfca]">MAP FILTER</span>
            <strong className="text-[21px] font-normal tracking-[.035em]">Map</strong>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="grid size-11 cursor-pointer place-items-center border-0 bg-transparent text-xl text-[#d7eef1] hover:bg-black/25 hover:text-white"
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
            <fieldset
              className="mx-3.5 mt-3 mb-2 flex border border-[#c6e5ea]/20 bg-[#040b0e]/35"
              aria-label="World region"
            >
              {props.layers.map((layer) => {
                const active = layer.id === props.activeLayer.id
                return (
                  <button
                    key={layer.id}
                    type="button"
                    className={`min-h-10 min-w-0 flex-1 cursor-pointer overflow-hidden border-0 px-1 text-xs font-normal tracking-[.04em] text-ellipsis whitespace-nowrap uppercase transition-colors ${
                      active
                        ? 'bg-[#304952]/65 text-[#e8f7f8] shadow-[inset_0_-2px_#20c7ea]'
                        : 'bg-transparent text-[#a9b5b9] hover:bg-white/5 hover:text-white'
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
              <PlayerCategory
                {...props}
                players={index.byKind.players}
                companions={index.byKind.companions}
                matches={matches}
                expanded={!collapsedGroups.has('players')}
                onToggleExpanded={() => toggleCategory('players')}
              />
              <GuildCategory
                {...props}
                bases={index.byKind.bases}
                workers={index.byKind.workers}
                workersByBaseId={index.workersByBaseId}
                matches={matches}
                expanded={!collapsedGroups.has('bases')}
                onToggleExpanded={() => toggleCategory('bases')}
              />
              <SimpleCategory
                {...props}
                group="wild-pals"
                title="Wild Pals"
                items={index.byKind['wild-pals']}
                matches={matches}
                empty="No wild Pals are currently loaded."
                expanded={!collapsedGroups.has('wild-pals')}
                onToggleExpanded={() => toggleCategory('wild-pals')}
              />
              <SimpleCategory
                {...props}
                group="npcs"
                title="NPCs"
                items={index.byKind.npcs}
                matches={matches}
                empty="No NPCs are currently loaded."
                expanded={!collapsedGroups.has('npcs')}
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
  hiddenIds,
  onToggleKinds,
  onToggleExpanded,
  count
}: Pick<
  CategoryProps,
  'group' | 'title' | 'expanded' | 'enabledKinds' | 'hiddenIds' | 'onToggleKinds' | 'onToggleExpanded'
> & {
  items: MapItem[]
  controls: string
  count?: number
}) {
  const kinds = GROUP_KINDS[group]
  const state = visibilityState(items, enabledKinds, hiddenIds)
  const checked = items.length > 0 && kinds.every((kind) => enabledKinds.has(kind)) && state.checked
  const itemCount =
    count ??
    (group === 'bases' || group === 'players' ? items.filter((item) => item.kind === group).length : items.length)
  return (
    <div className="flex min-h-8 items-center gap-0.5">
      <span className="grid size-8 shrink-0 place-items-center">
        <Checkbox
          state={{ checked, indeterminate: !checked && state.indeterminate, disabled: items.length === 0 }}
          label={`Show ${title}`}
          onChange={(visible) => onToggleKinds(kinds, visible)}
        />
      </span>
      <AccordionButton expanded={expanded} label={`${title} section`} controls={controls} onClick={onToggleExpanded}>
        <span className={`explorer-symbol kind-${group}`} aria-hidden="true" />
        <strong className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-xs font-semibold">
          {title} ({itemCount})
        </strong>
      </AccordionButton>
    </div>
  )
}

function SimpleCategory({ group, title, items, matches, empty, ...props }: CategoryProps & { items: MapItem[] }) {
  const visible = items.filter(matches).sort((left, right) => left.name.localeCompare(right.name))
  const rendered = visible.slice(0, INITIAL_CATEGORY_ITEMS)
  const contentId = useId()
  return (
    <section className="border-b border-white/7 py-0.5 last:border-b-0">
      <CategoryHeader {...props} group={group} title={title} items={items} controls={contentId} />
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

interface PlayerCategoryProps extends ExplorerProps {
  players: MapItem[]
  companions: MapItem[]
  matches: (item: MapItem) => boolean
  expanded: boolean
  onToggleExpanded: () => void
}

function PlayerCategory({ players, companions, matches, ...props }: PlayerCategoryProps) {
  const playersById = new Map(players.map((player) => [player.id, player]))
  const onlinePlayersById = new Map(
    props.items.filter((item) => item.kind === 'players').map((player) => [player.id, player])
  )
  const companionsByOwnerId = new Map<string, MapItem[]>()
  const otherMapCompanions: MapItem[] = []
  const noOnlineOwnerCompanions: MapItem[] = []
  for (const companion of companions) {
    if (!companion.ownerId || !onlinePlayersById.has(companion.ownerId)) {
      noOnlineOwnerCompanions.push(companion)
      continue
    }
    if (!playersById.has(companion.ownerId)) {
      otherMapCompanions.push(companion)
      continue
    }
    const owned = companionsByOwnerId.get(companion.ownerId) || []
    owned.push(companion)
    companionsByOwnerId.set(companion.ownerId, owned)
  }
  for (const owned of companionsByOwnerId.values()) {
    owned.sort((left, right) => left.name.localeCompare(right.name))
  }
  otherMapCompanions.sort((left, right) => left.name.localeCompare(right.name))
  noOnlineOwnerCompanions.sort((left, right) => left.name.localeCompare(right.name))

  const views = players
    .slice()
    .sort((left, right) => left.name.localeCompare(right.name))
    .map((player) => {
      const owned = companionsByOwnerId.get(player.id) || []
      const playerMatches = matches(player)
      const matchingCompanions = playerMatches ? owned : owned.filter(matches)
      return { player, owned, playerMatches, matchingCompanions }
    })
    .filter(({ playerMatches, matchingCompanions }) => playerMatches || matchingCompanions.length > 0)
  const renderedViews = views.slice(0, INITIAL_CATEGORY_ITEMS)
  const searchQuery = props.search.trim().toLowerCase()
  const requestedOtherMap = props.search.trim()
    ? 'owner on another map companion pals'.includes(searchQuery)
      ? otherMapCompanions
      : otherMapCompanions.filter(matches)
    : otherMapCompanions
  const requestedNoOwner = props.search.trim()
    ? 'no online owner companion pals'.includes(searchQuery)
      ? noOnlineOwnerCompanions
      : noOnlineOwnerCompanions.filter(matches)
    : noOnlineOwnerCompanions
  const contentId = useId()
  let eligibleCompanions = 0
  let renderedCompanions = 0
  const renderFallbackGroup = (title: string, groupLabel: string, items: MapItem[], requested: MapItem[]) => {
    if (requested.length === 0) return null
    eligibleCompanions += requested.length
    const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedCompanions)
    const displayed = requested.slice(0, remaining)
    renderedCompanions += displayed.length
    return (
      <fieldset className="m-0 min-w-0 border-0 p-0">
        <legend className="sr-only">{groupLabel}</legend>
        <div className="flex min-h-8 items-center gap-0.5">
          <span className="grid size-8 shrink-0 place-items-center">
            <Checkbox
              state={visibilityState(items, props.enabledKinds, props.hiddenIds)}
              label={`Show ${groupLabel.toLowerCase()}`}
              onChange={(visible) =>
                props.onToggleItems(
                  items.map((companion) => companion.id),
                  visible
                )
              }
            />
          </span>
          <div className="grid min-h-7 min-w-0 flex-1 grid-cols-[13px_minmax(0,1fr)_auto] items-center gap-1.5 px-1.5 py-1 text-xs text-[#cbd7d9]">
            <span className="explorer-symbol kind-companions" aria-hidden="true" />
            <strong className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-medium">{title}</strong>
            <span className="ml-auto shrink-0 text-[10px] text-[#7f898e]">
              {items.length} Pal{items.length === 1 ? '' : 's'}
            </span>
          </div>
        </div>
        <div className="ml-3 grid gap-px border-l border-white/10 pl-2">
          {displayed.map((companion) => (
            <ObjectRow
              key={companion.id}
              item={companion}
              meta={companion.level ? `Lv ${companion.level}` : undefined}
              {...props}
            />
          ))}
        </div>
      </fieldset>
    )
  }

  return (
    <section className="border-b border-white/7 py-0.5">
      <CategoryHeader
        {...props}
        group="players"
        title="Players"
        items={[...players, ...companions]}
        count={players.length}
        controls={contentId}
      />
      <div id={contentId} className="grid gap-px pl-1" hidden={!props.expanded}>
        {renderedViews.map(({ player, owned, matchingCompanions }) => {
          const expanded = props.expandedPlayers.has(player.id) || Boolean(props.search.trim() && owned.length > 0)
          const playerContentId = `${contentId}-player-${player.id}`
          const requestedCompanions = props.search.trim() ? matchingCompanions : owned
          let displayedCompanions: MapItem[] = []
          if (expanded) {
            eligibleCompanions += requestedCompanions.length
            const remaining = Math.max(0, INITIAL_CATEGORY_ITEMS - renderedCompanions)
            displayedCompanions = requestedCompanions.slice(0, remaining)
            renderedCompanions += displayedCompanions.length
          }
          const playerItems = [player, ...owned]
          const meta = [player.level ? `Lv ${player.level}` : '', player.guildName || ''].filter(Boolean).join(' · ')
          return (
            <div key={player.id}>
              <div className="flex min-h-8 min-w-0 items-center gap-0.5">
                <span className="grid size-8 shrink-0 place-items-center">
                  <Checkbox
                    state={visibilityState(playerItems, props.enabledKinds, props.hiddenIds)}
                    label={`Show ${markerText(player)}`}
                    onChange={(visible) =>
                      props.onToggleItems(
                        playerItems.map((item) => item.id),
                        visible
                      )
                    }
                  />
                </span>
                <ItemButton item={player} meta={meta || undefined} onFocus={props.onFocusItem} />
                {owned.length > 0 ? (
                  <DisclosureToggle
                    expanded={expanded}
                    label={`companion Pals for ${player.name}`}
                    controls={playerContentId}
                    onClick={() => props.onTogglePlayer(player.id)}
                  />
                ) : (
                  <span className="size-8 shrink-0" aria-hidden="true" />
                )}
              </div>
              {owned.length > 0 ? (
                <div id={playerContentId} className="ml-3 border-l border-white/10 pl-2" hidden={!expanded}>
                  {expanded &&
                    displayedCompanions.map((companion) => (
                      <ObjectRow
                        key={companion.id}
                        item={companion}
                        meta={companion.level ? `Lv ${companion.level}` : undefined}
                        {...props}
                      />
                    ))}
                </div>
              ) : null}
            </div>
          )
        })}
        {renderFallbackGroup(
          'Owner on another map',
          'Companion Pals with an owner on another map',
          otherMapCompanions,
          requestedOtherMap
        )}
        {renderFallbackGroup(
          'No online owner',
          'Companion Pals with no online owner',
          noOnlineOwnerCompanions,
          requestedNoOwner
        )}
        {renderedViews.length < views.length ? (
          <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
            {props.search.trim()
              ? `${views.length - renderedViews.length} more player matches. Refine your search to inspect them.`
              : `Search to inspect ${views.length - renderedViews.length} more players.`}
          </p>
        ) : null}
        {renderedCompanions < eligibleCompanions ? (
          <p className="my-1 ml-5 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
            {eligibleCompanions - renderedCompanions} more companion Pal
            {eligibleCompanions - renderedCompanions === 1 ? '' : 's'} omitted. Refine your search or expand fewer
            players to inspect them.
          </p>
        ) : null}
        {views.length === 0 && requestedOtherMap.length === 0 && requestedNoOwner.length === 0 ? (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {props.search.trim() && (players.length > 0 || companions.length > 0)
              ? `No players or companion Pals match “${props.search.trim()}”.`
              : 'No players are currently online.'}
          </p>
        ) : null}
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
  for (const item of props.items) {
    if (item.guildKey && item.guildName) guildNames.set(item.guildKey, item.guildName)
  }
  const guildMap = new Map<string, { id: string; name: string; bases: MapItem[]; outsideWorkers: MapItem[] }>()
  for (const base of sortedBases) {
    const id = guildIdForBase(base)
    const inferredGuildName =
      guildNames.get(id) || (base.name.trim().toLowerCase() === 'palbox' ? 'Unnamed guild' : base.name)
    const guild = guildMap.get(id) || { id, name: inferredGuildName, bases: [], outsideWorkers: [] }
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
    const guild = guildMap.get(worker.guildKey) || {
      id: worker.guildKey,
      name: guildNames.get(worker.guildKey) || 'Unnamed guild',
      bases: [],
      outsideWorkers: []
    }
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
                    state={visibilityState(guildItems, props.enabledKinds, props.hiddenIds)}
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
                  meta={`${guild.bases.length} base${guild.bases.length === 1 ? '' : 's'}`}
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
                              state={visibilityState(baseItems, props.enabledKinds, props.hiddenIds)}
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
                    state={visibilityState(fallbackWorkers, props.enabledKinds, props.hiddenIds)}
                    label="Show Pals with no linked guild"
                    onChange={(visible) =>
                      props.onToggleItems(
                        fallbackWorkers.map((worker) => worker.id),
                        visible
                      )
                    }
                  />
                </span>
                <div className="grid min-h-7 min-w-0 flex-1 grid-cols-[13px_minmax(0,1fr)_auto] items-center gap-1.5 px-1.5 py-1 text-xs text-[#cbd7d9]">
                  <span className="explorer-symbol kind-workers" aria-hidden="true" />
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
      className="group flex min-h-8 min-w-0 flex-1 cursor-pointer items-center gap-1.5 rounded bg-transparent px-1 text-left text-[#e3edef] hover:bg-white/6 hover:text-white"
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
      className="grid size-8 shrink-0 cursor-pointer place-items-center rounded text-[#929da1] hover:bg-white/8 hover:text-white"
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
