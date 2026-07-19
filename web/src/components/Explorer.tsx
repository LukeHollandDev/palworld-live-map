import { useEffect, useId, useMemo, useRef } from 'react'
import { itemKey, markerText } from '../lib/map'
import type { ItemKind, MapItem, MapLayer } from '../types'

interface ExplorerProps {
  activeLayer: MapLayer
  layers: MapLayer[]
  items: MapItem[]
  search: string
  enabledKinds: Set<ItemKind>
  hiddenKeys: Set<string>
  expandedGuilds: Set<string>
  expandedBases: Set<string>
  objectNotice: string | null
  onSearch: (value: string) => void
  onToggleKinds: (kinds: ItemKind[], visible: boolean) => void
  onToggleItems: (keys: string[], visible: boolean) => void
  onToggleGuild: (id: string) => void
  onToggleBase: (id: string) => void
  onFocusItem: (item: MapItem, returnFocus: HTMLElement) => void
  onClose: () => void
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

const GROUP_KINDS: Record<string, ItemKind[]> = {
  players: ['players'],
  bases: ['bases', 'workers'],
  companions: ['companions'],
  'wild-pals': ['wild-pals'],
  npcs: ['npcs']
}

function visibilityState(items: MapItem[], enabledKinds: Set<ItemKind>, hiddenKeys: Set<string>): CheckState {
  const visible = items.filter((item) => enabledKinds.has(item.kind) && !hiddenKeys.has(itemKey(item))).length
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
  className = '',
  onFocus
}: {
  item: MapItem
  meta?: string
  label?: string
  className?: string
  onFocus: ExplorerProps['onFocusItem']
}) {
  return (
    <button
      type="button"
      className={`explorer-item ${className}`}
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

function ObjectRow({
  item,
  meta,
  label,
  enabledKinds,
  hiddenKeys,
  onToggleItems,
  onFocusItem,
  className = ''
}: Pick<ExplorerProps, 'enabledKinds' | 'hiddenKeys' | 'onToggleItems' | 'onFocusItem'> & {
  item: MapItem
  meta?: string
  label?: string
  className?: string
}) {
  const key = itemKey(item)
  return (
    <div className={`flex min-w-0 items-center gap-1.5 ${className}`}>
      <Checkbox
        state={visibilityState([item], enabledKinds, hiddenKeys)}
        label={`Show ${markerText(item)}`}
        onChange={(checked) => onToggleItems([key], checked)}
      />
      <ItemButton item={item} meta={meta} label={label} onFocus={onFocusItem} />
    </div>
  )
}

export function Explorer(props: ExplorerProps) {
  const currentItems = useMemo(
    () => props.items.filter((item) => item.map === props.activeLayer.id),
    [props.activeLayer.id, props.items]
  )
  const query = props.search.trim().toLowerCase()
  const assignedBaseName = (item: MapItem) =>
    item.kind === 'workers' && item.baseId
      ? currentItems.find((candidate) => candidate.kind === 'bases' && candidate.baseId === item.baseId)?.name || ''
      : ''
  const matches = (item: MapItem) =>
    !query ||
    `${item.name} ${item.detail || ''} ${item.level || ''} ${assignedBaseName(item)}`.toLowerCase().includes(query)

  return (
    <aside
      className="absolute inset-y-3 left-3 z-10 flex w-[330px] flex-col overflow-hidden rounded-lg border border-white/15 bg-[rgb(24_28_31/94%)] shadow-[0_10px_30px_rgb(0_0_0/28%)] backdrop-blur-sm max-sm:inset-y-0 max-sm:left-0 max-sm:w-[min(310px,88vw)] max-sm:rounded-none max-sm:border-y-0 max-sm:border-l-0"
      aria-label="Map filters"
    >
      <div className="flex items-center justify-between px-3 py-2.5">
        <strong className="text-sm">Map</strong>
        <button
          type="button"
          className="grid size-7 cursor-pointer place-items-center rounded text-lg text-[#aeb6ba] hover:bg-white/10 hover:text-white"
          aria-label="Collapse map filter"
          title="Collapse map filter"
          onClick={props.onClose}
        >
          ‹
        </button>
      </div>

      <fieldset
        className="mx-3 mb-3 flex rounded-md border border-[#3a4145] bg-[#15191b] p-0.5"
        aria-label="World region"
      >
        {props.layers.map((layer) => (
          <button
            key={layer.id}
            type="button"
            className={`min-w-0 flex-1 cursor-pointer overflow-hidden text-ellipsis whitespace-nowrap rounded px-2 py-1.5 text-xs ${layer.id === props.activeLayer.id ? 'bg-[#343b3f] text-white shadow-sm' : 'text-[#90999e] hover:text-[#d9dddf]'}`}
            aria-pressed={layer.id === props.activeLayer.id}
            onClick={() => props.onLayerChange(layer)}
          >
            {layer.name}
          </button>
        ))}
      </fieldset>

      <label className="mx-3 mb-3 grid gap-1 text-[11px] font-semibold uppercase tracking-[.05em] text-[#929ba0]">
        <span>Filter map</span>
        <input
          type="search"
          className="h-9 rounded-md border border-[#3d4448] bg-[#15191b] px-2.5 text-[13px] font-normal normal-case tracking-normal text-[#eef0f1] outline-none placeholder:text-[#687176] focus:border-[#6ba9cd] focus:ring-2 focus:ring-[#6ba9cd]/20"
          placeholder="Player, Pal or base…"
          autoComplete="off"
          value={props.search}
          onChange={(event) => props.onSearch(event.currentTarget.value)}
        />
      </label>

      <div className="min-h-0 flex-1 overflow-y-auto border-t border-white/8 px-3 py-2" aria-live="polite">
        <SimpleCategory
          {...props}
          group="players"
          title="Players"
          items={currentItems.filter((item) => item.kind === 'players')}
          matches={matches}
          empty="No players online."
        />
        <BaseCategory
          {...props}
          bases={currentItems.filter((item) => item.kind === 'bases')}
          workers={currentItems.filter((item) => item.kind === 'workers')}
          matches={matches}
        />
        <SimpleCategory
          {...props}
          group="companions"
          title="Companion Pals"
          items={currentItems.filter((item) => item.kind === 'companions')}
          matches={matches}
          empty="No companion Pals are currently reported."
        />
        <SimpleCategory
          {...props}
          group="wild-pals"
          title="Wild Pals"
          items={currentItems.filter((item) => item.kind === 'wild-pals')}
          matches={matches}
          empty="No wild Pals are currently loaded."
        />
        <SimpleCategory
          {...props}
          group="npcs"
          title="NPCs"
          items={currentItems.filter((item) => item.kind === 'npcs')}
          matches={matches}
          empty="No NPCs are currently loaded."
        />
      </div>

      {props.objectNotice && (
        <p className="m-3 mt-1 rounded-md border border-[#554b37] bg-[#302b22] px-2.5 py-2 text-[11px] leading-4 text-[#d2b980]">
          {props.objectNotice}
        </p>
      )}
    </aside>
  )
}

interface CategoryProps extends ExplorerProps {
  group: Exclude<ItemKind, 'workers'>
  title: string
  matches: (item: MapItem) => boolean
  empty: string
}

function CategoryHeader({
  group,
  title,
  items,
  enabledKinds,
  hiddenKeys,
  onToggleKinds
}: Pick<CategoryProps, 'group' | 'title' | 'enabledKinds' | 'hiddenKeys' | 'onToggleKinds'> & { items: MapItem[] }) {
  const checkboxId = useId()
  const kinds = GROUP_KINDS[group]
  const state = visibilityState(items, enabledKinds, hiddenKeys)
  const checked = items.length > 0 && kinds.every((kind) => enabledKinds.has(kind)) && state.checked
  return (
    <label
      htmlFor={checkboxId}
      className="flex min-h-8 cursor-pointer items-center gap-2 rounded px-1 hover:bg-white/5"
    >
      <Checkbox
        id={checkboxId}
        state={{ checked, indeterminate: !checked && state.indeterminate, disabled: items.length === 0 }}
        label={`Show ${title}`}
        onChange={(visible) => onToggleKinds(kinds, visible)}
      />
      <span className={`explorer-symbol kind-${group}`} aria-hidden="true" />
      <strong className="text-xs font-semibold">
        {title} ({group === 'bases' ? items.filter((item) => item.kind === 'bases').length : items.length})
      </strong>
    </label>
  )
}

function SimpleCategory({ group, title, items, matches, empty, ...props }: CategoryProps & { items: MapItem[] }) {
  const visible = items.filter(matches).sort((left, right) => left.name.localeCompare(right.name))
  return (
    <section className="border-b border-white/7 py-1.5 last:border-b-0">
      <CategoryHeader {...props} group={group} title={title} items={items} />
      <div className="grid gap-0.5 pl-1.5">
        {visible.length === 0 ? (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {items.length > 0 && props.search.trim()
              ? `No ${title.toLowerCase()} match “${props.search.trim()}”.`
              : empty}
          </p>
        ) : (
          visible.map((item) => (
            <ObjectRow
              key={itemKey(item)}
              item={item}
              meta={item.level ? `Lv ${item.level}` : item.kind === 'npcs' ? item.detail : undefined}
              {...props}
            />
          ))
        )}
      </div>
    </section>
  )
}

interface BaseCategoryProps extends ExplorerProps {
  bases: MapItem[]
  workers: MapItem[]
  matches: (item: MapItem) => boolean
}

function BaseCategory({ bases, workers, matches, ...props }: BaseCategoryProps) {
  const sortedBases = bases
    .slice()
    .sort((left, right) => left.name.localeCompare(right.name) || left.x - right.x || left.y - right.y)
  const guildMap = new Map<string, { id: string; name: string; bases: MapItem[] }>()
  for (const base of sortedBases) {
    const id = base.guildKey || `base:${base.baseId}`
    const guild = guildMap.get(id) || { id, name: base.name, bases: [] }
    guild.bases.push(base)
    guildMap.set(id, guild)
  }
  const guilds = Array.from(guildMap.values()).sort((left, right) => left.name.localeCompare(right.name))
  const names = new Map<string, number>()
  for (const guild of guilds) names.set(guild.name, (names.get(guild.name) || 0) + 1)
  const occurrences = new Map<string, number>()
  let rendered = 0

  return (
    <section className="border-b border-white/7 py-1.5">
      <CategoryHeader {...props} group="bases" title="Bases" items={[...bases, ...workers]} />
      <div className="grid gap-0.5 pl-1">
        {guilds.map((guild) => {
          const occurrence = (occurrences.get(guild.name) || 0) + 1
          occurrences.set(guild.name, occurrence)
          const displayName = (names.get(guild.name) || 0) > 1 ? `${guild.name} #${occurrence}` : guild.name
          const entries = guild.bases
            .map((base, index) => {
              const baseWorkers = workers
                .filter((worker) => worker.baseId === base.baseId)
                .sort((left, right) => left.name.localeCompare(right.name))
              return { base, baseWorkers, index, matchingWorkers: baseWorkers.filter(matches) }
            })
            .filter(({ base, matchingWorkers }) => matches(base) || matchingWorkers.length > 0)
          if (entries.length === 0) return null
          rendered++
          const guildItems = guild.bases.flatMap((base) => [
            base,
            ...workers.filter((worker) => worker.baseId === base.baseId)
          ])
          const expanded = props.expandedGuilds.has(guild.id) || Boolean(props.search.trim())
          return (
            <div key={guild.id}>
              <div className="flex min-h-7 items-center gap-1.5">
                <BranchToggle expanded={expanded} label={displayName} onClick={() => props.onToggleGuild(guild.id)} />
                <Checkbox
                  state={visibilityState(guildItems, props.enabledKinds, props.hiddenKeys)}
                  label={`Show guild ${displayName}`}
                  onChange={(visible) => props.onToggleItems(guildItems.map(itemKey), visible)}
                />
                <span className="min-w-0 flex-1 overflow-hidden text-ellipsis whitespace-nowrap text-xs font-medium">
                  {displayName}
                </span>
                <span className="shrink-0 text-[10px] text-[#7f898e]">
                  {guild.bases.length} base{guild.bases.length === 1 ? '' : 's'}
                </span>
              </div>
              {expanded && (
                <div className="ml-2.5 border-l border-white/10 pl-2">
                  {entries.map(({ base, baseWorkers, index, matchingWorkers }) => {
                    const baseExpanded = props.expandedBases.has(base.baseId || '') || Boolean(props.search.trim())
                    const baseItems = [base, ...baseWorkers]
                    const baseLabel = guild.bases.length === 1 ? 'Base' : `Base ${index + 1}`
                    return (
                      <div key={itemKey(base)}>
                        <div className="flex min-w-0 items-center gap-1.5">
                          <BranchToggle
                            expanded={baseExpanded}
                            label={`${displayName} ${baseLabel}`}
                            onClick={() => base.baseId && props.onToggleBase(base.baseId)}
                          />
                          <Checkbox
                            state={visibilityState(baseItems, props.enabledKinds, props.hiddenKeys)}
                            label={`Show ${baseLabel} for ${displayName}`}
                            onChange={(visible) => props.onToggleItems(baseItems.map(itemKey), visible)}
                          />
                          <ItemButton
                            item={base}
                            label={baseLabel}
                            meta={`${baseWorkers.length} Pal${baseWorkers.length === 1 ? '' : 's'}`}
                            className="kind-base-item"
                            onFocus={props.onFocusItem}
                          />
                        </div>
                        {baseExpanded && (
                          <div className="ml-3 border-l border-white/8 pl-2">
                            {(props.search.trim() ? matchingWorkers : baseWorkers).map((worker) => (
                              <ObjectRow
                                key={itemKey(worker)}
                                item={worker}
                                meta={worker.level ? `Lv ${worker.level}` : undefined}
                                className="worker-row"
                                {...props}
                              />
                            ))}
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          )
        })}
        {workers
          .filter((worker) => !worker.baseId && matches(worker))
          .map((worker) => (
            <ObjectRow
              key={itemKey(worker)}
              item={worker}
              meta={worker.level ? `Lv ${worker.level}` : undefined}
              className="pl-5"
              {...props}
            />
          ))}
        {rendered === 0 && workers.filter((worker) => !worker.baseId && matches(worker)).length === 0 && (
          <p className="my-1.5 pl-5 text-[11px] text-[#778187]">
            {props.search.trim() && bases.length
              ? `No bases match “${props.search.trim()}”.`
              : 'No bases are currently reported.'}
          </p>
        )}
      </div>
    </section>
  )
}

function BranchToggle({ expanded, label, onClick }: { expanded: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={`grid size-5 shrink-0 cursor-pointer place-items-center rounded text-sm text-[#90999e] transition-transform hover:bg-white/8 hover:text-white ${expanded ? 'rotate-90' : ''}`}
      aria-label={`${expanded ? 'Collapse' : 'Expand'} ${label}`}
      aria-expanded={expanded}
      onClick={onClick}
    >
      ›
    </button>
  )
}
