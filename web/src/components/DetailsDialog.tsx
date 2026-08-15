import { IconCheck, IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useEffect, useId, useRef, useState } from 'react'
import { isChecklistItem } from '../lib/completion'
import { buildGuildDetails, type GuildDetails as GuildDetailsModel } from '../lib/guilds'
import { LEADERBOARDS, type LeaderboardId, leaderboardById } from '../lib/leaderboards'
import { kindLabel } from '../lib/map'
import type { ItemKind, LandmarkReward, MapItem, MapLayer } from '../types'
import { MapPanelHeader, MapPanelShell } from './MapPanel'
import { MarkerGlyph } from './MarkerGlyph'

export type Detail =
  | { kind: 'item'; itemId: string }
  | { kind: 'guild'; guildId: string }
  | { kind: 'leaderboard'; leaderboardId: LeaderboardId }

const DETAIL_WORKER_LIMIT = 250
const DETAIL_LABELS: Record<ItemKind, string> = {
  players: 'Details',
  bases: 'Description',
  workers: 'Species',
  companions: 'Species',
  'wild-pals': 'Species',
  'alpha-pals': 'Encounter',
  bosses: 'Encounter',
  bounties: 'Encounter',
  'oil-rigs': 'Facility',
  watchtowers: 'Location',
  waypoints: 'Location',
  'dungeon-entrances': 'Location',
  effigies: 'Type',
  journals: 'Type',
  'ancient-shrine-pickups': 'Pickup',
  'npc-locations': 'Type',
  npcs: 'Type'
}

interface DetailsDialogProps {
  detail: Detail | null
  items: MapItem[]
  layers: MapLayer[]
  returnFocus: HTMLElement | null
  fallbackFocus: HTMLElement | null
  manualChecklist: ManualChecklistDetails
  onClose: () => void
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectGuild: (guildId: string, focus: HTMLElement) => void
  onSelectLeaderboard: (leaderboardId: LeaderboardId) => void
}

interface ManualChecklistDetails {
  profileName: string
  completedIds: ReadonlySet<string>
  onSetCompletion: (landmarkId: string, completed: boolean) => void
}

function canRestoreFocus(target: HTMLElement | null) {
  return Boolean(
    target?.isConnected && !target.matches(':disabled') && !target.closest('[inert], [hidden], [aria-hidden="true"]')
  )
}

function restoreFocus(returnFocus: HTMLElement | null, fallbackFocus: HTMLElement | null) {
  window.requestAnimationFrame(() => {
    const target = canRestoreFocus(returnFocus) ? returnFocus : canRestoreFocus(fallbackFocus) ? fallbackFocus : null
    target?.focus({ preventScroll: true })
  })
}

export function DetailsDialog({
  detail,
  items,
  layers,
  returnFocus,
  fallbackFocus,
  manualChecklist,
  onClose,
  onSelectItem,
  onSelectGuild,
  onSelectLeaderboard
}: DetailsDialogProps) {
  const titleRef = useRef<HTMLHeadingElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const detailKey = detail
    ? detail.kind === 'item'
      ? `item:${detail.itemId}`
      : detail.kind === 'guild'
        ? `guild:${detail.guildId}`
        : 'leaderboard'
    : undefined

  useEffect(() => {
    if (!detailKey) return
    const frame = window.requestAnimationFrame(() => {
      if (bodyRef.current) bodyRef.current.scrollTop = 0
      titleRef.current?.focus({ preventScroll: true })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [detailKey])

  useEffect(() => {
    if (!detail) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape' || event.defaultPrevented) return
      onClose()
      restoreFocus(returnFocus, fallbackFocus)
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [detail, fallbackFocus, onClose, returnFocus])

  if (!detail) return null

  const item = detail.kind === 'item' ? items.find((candidate) => candidate.id === detail.itemId) : undefined
  const guild = detail.kind === 'guild' ? buildGuildDetails(detail.guildId, items) : undefined
  const leaderboard = detail.kind === 'leaderboard' ? leaderboardById(detail.leaderboardId) : undefined
  if (detail.kind === 'item' && !item) return null
  const title = leaderboard ? 'Leaderboards' : item?.name || guild?.name || 'Unnamed guild'
  const eyebrow = (
    leaderboard ? 'SERVER RANKINGS' : item ? `${kindLabel(item.kind)} DETAILS` : 'GUILD DETAILS'
  ).toUpperCase()

  const close = () => {
    onClose()
    restoreFocus(returnFocus, fallbackFocus)
  }

  return (
    <MapPanelShell
      id={leaderboard ? 'leaderboard-panel' : undefined}
      side="right"
      mobileSize={leaderboard ? 'fixed' : 'content'}
      mobileSheetActive={Boolean(leaderboard)}
      mobileSheetLabel={leaderboard ? 'leaderboards' : undefined}
      className="surface-enter-motion"
      role="dialog"
      aria-modal="false"
      aria-labelledby="details-title"
    >
      <MapPanelHeader
        eyebrow={eyebrow}
        title={title}
        titleId="details-title"
        titleRef={titleRef}
        titleTabIndex={-1}
        closeLabel="Close details"
        onClose={close}
      />
      <div ref={bodyRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain" data-details-body>
        <div className="grid gap-5 p-[18px] max-sm:p-3.5">
          {item ? (
            <ItemDetails
              item={item}
              items={items}
              layers={layers}
              manualChecklist={manualChecklist}
              onSelectItem={onSelectItem}
              onSelectGuild={onSelectGuild}
            />
          ) : guild ? (
            <GuildDetails guild={guild} layers={layers} onSelectItem={onSelectItem} />
          ) : leaderboard ? (
            <LeaderboardDetails
              leaderboardId={leaderboard.id}
              items={items}
              onSelectItem={onSelectItem}
              onSelectLeaderboard={onSelectLeaderboard}
            />
          ) : null}
        </div>
      </div>
    </MapPanelShell>
  )
}

function LeaderboardDetails({
  leaderboardId,
  items,
  onSelectItem,
  onSelectLeaderboard
}: {
  leaderboardId: LeaderboardId
  items: MapItem[]
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectLeaderboard: (leaderboardId: LeaderboardId) => void
}) {
  const leaderboard = leaderboardById(leaderboardId)
  const entries = leaderboard.entries(items)
  return (
    <>
      <LeaderboardPicker leaderboardId={leaderboard.id} onSelectLeaderboard={onSelectLeaderboard} />
      <section>
        <SectionTitle>{leaderboard.title}</SectionTitle>
        <p className="mt-0 mb-3 text-xs leading-5 text-[#9fb0b5]">{leaderboard.description}</p>
        {entries.length > 0 ? (
          <ol className="m-0 grid list-none gap-1.5 p-0">
            {entries.map(({ item, rank, value }) => {
              const status = item.online === false ? 'Offline' : 'Online'
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    className="pal-glass-inset pal-interactive grid min-h-12 w-full cursor-pointer grid-cols-[28px_22px_minmax(0,1fr)_auto] items-center gap-2 px-2.5 py-2 text-left text-xs focus-visible:outline-none"
                    aria-label={`View leaderboard rank ${rank}: ${item.name} · ${value}, ${status}`}
                    onClick={(event) => onSelectItem(item, event.currentTarget)}
                  >
                    <strong className="text-right text-[11px] font-semibold text-[#789097] tabular-nums">{rank}</strong>
                    <MarkerGlyph kind="players" online={item.online} />
                    <span className="min-w-0">
                      <span className="block truncate text-[#f0f9fa]">{item.name}</span>
                      <span
                        className={`mt-0.5 block text-[10px] ${item.online === false ? 'text-[#9aa3a7]' : 'text-[#76d39a]'}`}
                      >
                        {status}
                      </span>
                    </span>
                    <strong className="font-medium text-[#d8eef1] tabular-nums">{value}</strong>
                  </button>
                </li>
              )
            })}
          </ol>
        ) : (
          <p className="m-0 text-[13px] text-[#8f989d]">No leaderboard data is currently available.</p>
        )}
      </section>
    </>
  )
}

function LeaderboardPicker({
  leaderboardId,
  onSelectLeaderboard
}: {
  leaderboardId: LeaderboardId
  onSelectLeaderboard: (leaderboardId: LeaderboardId) => void
}) {
  const selected = leaderboardById(leaderboardId)
  const selectedIndex = LEADERBOARDS.findIndex((candidate) => candidate.id === selected.id)
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(selectedIndex)
  const wrapperRef = useRef<HTMLFieldSetElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const labelId = useId()
  const selectedValueId = useId()
  const listboxId = useId()

  useEffect(() => {
    if (!open) return
    const frame = window.requestAnimationFrame(() => {
      const option = optionRefs.current[activeIndex]
      option?.focus({ preventScroll: true })
      option?.scrollIntoView?.({ block: 'nearest' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeIndex, open])

  useEffect(() => {
    if (!open) return
    const closeOnPointerDown = (event: PointerEvent) => {
      if (wrapperRef.current?.contains(event.target as Node)) return
      setOpen(false)
    }
    document.addEventListener('pointerdown', closeOnPointerDown)
    return () => document.removeEventListener('pointerdown', closeOnPointerDown)
  }, [open])

  useEffect(() => {
    if (!open) setActiveIndex(selectedIndex)
  }, [open, selectedIndex])

  const closeAndFocusTrigger = () => {
    setOpen(false)
    triggerRef.current?.focus({ preventScroll: true })
  }

  const selectLeaderboard = (id: LeaderboardId, index: number) => {
    setOpen(false)
    setActiveIndex(index)
    onSelectLeaderboard(id)
    triggerRef.current?.focus({ preventScroll: true })
  }

  const focusOption = (index: number) => {
    const normalized = (index + LEADERBOARDS.length) % LEADERBOARDS.length
    setActiveIndex(normalized)
    const option = optionRefs.current[normalized]
    option?.focus({ preventScroll: true })
    option?.scrollIntoView?.({ block: 'nearest' })
  }

  const openAt = (index: number) => {
    setActiveIndex(index)
    setOpen(true)
  }

  return (
    <fieldset
      ref={wrapperRef}
      className="relative m-0 min-w-0 border-0 p-0"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setOpen(false)
      }}
      onKeyDownCapture={(event) => {
        if (!open || event.key !== 'Escape') return
        event.preventDefault()
        event.stopPropagation()
        closeAndFocusTrigger()
      }}
    >
      <legend id={labelId} className="sr-only">
        Leaderboard type
      </legend>
      <button
        ref={triggerRef}
        type="button"
        aria-labelledby={`${labelId} ${selectedValueId}`}
        aria-controls={listboxId}
        aria-expanded={open}
        aria-haspopup="listbox"
        className={`pal-glass-inset pal-interactive flex min-h-11 w-full min-w-0 cursor-pointer items-center justify-between gap-3 px-3 text-left text-sm text-[#e7f6f8] focus-visible:border-[#8de9f5] focus-visible:outline-none ${
          open ? 'pal-selected' : ''
        }`}
        onClick={() => (open ? setOpen(false) : openAt(selectedIndex))}
        onKeyDown={(event) => {
          if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
          event.preventDefault()
          if (event.key === 'Home') openAt(0)
          else if (event.key === 'End') openAt(LEADERBOARDS.length - 1)
          else openAt(selectedIndex)
        }}
      >
        <span id={selectedValueId} className="truncate">
          {selected.title}
        </span>
        <IconChevronDown
          aria-hidden="true"
          className={`shrink-0 text-[#8fcbd3] transition-transform ${open ? 'rotate-180' : ''}`}
          size={18}
          stroke={1.8}
        />
      </button>
      {open ? (
        <div
          id={listboxId}
          role="listbox"
          aria-label="Leaderboard types"
          className="absolute inset-x-0 top-[calc(100%+4px)] z-40 grid max-h-[min(22rem,calc(100dvh-14rem))] gap-1 overflow-y-auto border border-[#72d7e5]/45 bg-[#0d1519] p-1 shadow-[0_14px_30px_rgb(0_0_0/45%),inset_0_1px_rgb(217_250_255/7%)] max-sm:static max-sm:mt-1 max-sm:max-h-[42dvh]"
        >
          {LEADERBOARDS.map((candidate, index) => {
            const isSelected = candidate.id === selected.id
            return (
              <button
                key={candidate.id}
                ref={(node) => {
                  optionRefs.current[index] = node
                }}
                type="button"
                role="option"
                aria-selected={isSelected}
                tabIndex={index === activeIndex ? 0 : -1}
                className={`pal-interactive flex min-h-10 w-full cursor-pointer items-center justify-between gap-3 border px-3 text-left text-sm focus-visible:border-[#8de9f5] focus-visible:outline-none ${
                  isSelected
                    ? 'pal-selected shadow-[inset_3px_0_#72d7e5]'
                    : 'border-transparent bg-transparent text-[#b6c6ca] hover:border-[#84cfd9]/35 hover:bg-[#273439]/75 hover:text-white hover:shadow-[inset_3px_0_#4c9fab] focus-visible:bg-[#273439]/75 focus-visible:text-white focus-visible:shadow-[inset_3px_0_#4c9fab]'
                }`}
                onClick={() => selectLeaderboard(candidate.id, index)}
                onKeyDown={(event) => {
                  if (event.key === 'ArrowDown') {
                    event.preventDefault()
                    focusOption(index + 1)
                  } else if (event.key === 'ArrowUp') {
                    event.preventDefault()
                    focusOption(index - 1)
                  } else if (event.key === 'Home') {
                    event.preventDefault()
                    focusOption(0)
                  } else if (event.key === 'End') {
                    event.preventDefault()
                    focusOption(LEADERBOARDS.length - 1)
                  }
                }}
              >
                <span>{candidate.title}</span>
                {isSelected ? <IconCheck aria-hidden="true" className="shrink-0 text-[#8de9f5]" size={17} /> : null}
              </button>
            )
          })}
        </div>
      ) : null}
    </fieldset>
  )
}

function FactList({ entries }: { entries: Array<[string, string | number | undefined]> }) {
  const visible = entries.filter(([, value]) => value !== undefined && value !== '')
  return (
    <dl className="pal-glass-inset m-0 grid grid-cols-[minmax(105px,.7fr)_minmax(0,1fr)] text-xs">
      {visible.map(([label, value], index) => {
        const border = index === visible.length - 1 ? '' : 'border-b border-[#ceeaee]/15'
        return (
          <div className="contents" key={label}>
            <dt className={`m-0 px-3 py-[11px] text-[#a9b7bc] ${border}`}>{label}</dt>
            <dd className={`m-0 px-3 py-[11px] text-right text-[#eff9fa] ${border}`}>{value}</dd>
          </div>
        )
      })}
    </dl>
  )
}

function JournalPreview({ preview }: { preview: string }) {
  return (
    <section>
      <SectionTitle>Journal preview</SectionTitle>
      <p className="pal-glass-inset m-0 px-3 py-3 text-[13px] leading-5 text-[#dbeaec]">{preview}</p>
    </section>
  )
}

function LandmarkRewards({ rewards }: { rewards: LandmarkReward[] }) {
  if (rewards.length === 0) return null
  return (
    <section>
      <SectionTitle>Rewards</SectionTitle>
      <ul className="pal-glass-inset m-0 grid list-none divide-y divide-[#ceeaee]/15 p-0">
        {rewards.map((reward) => {
          const count = reward.count.toLocaleString()
          return (
            <li
              key={`${reward.name}:${reward.count}`}
              className="flex min-h-11 items-center justify-between gap-3 px-3 py-2.5 text-xs"
            >
              <span className="min-w-0 text-[#eff9fa]">{reward.name}</span>
              <span className="shrink-0 rounded-sm border border-[#72d7e5]/25 bg-[#20353a] px-2 py-1 font-medium text-[#9eeaf3] tabular-nums">
                <span aria-hidden="true">×{count}</span>
                <span className="sr-only">Quantity {count}</span>
              </span>
            </li>
          )
        })}
      </ul>
    </section>
  )
}

interface ItemRelationships {
  base?: MapItem
  owner?: MapItem
  guildKey?: string
  guildName?: string
  guildMembers: MapItem[]
  guildBases: MapItem[]
  guildPals: MapItem[]
  relatedPals: MapItem[]
}

function itemBaseKey(base: MapItem) {
  return base.baseId || base.id
}

function buildRelationships(item: MapItem, items: MapItem[]): ItemRelationships {
  const playersById = new Map<string, MapItem>()
  const basesById = new Map<string, MapItem>()
  const guildByOwnerId = new Map<string, string>()
  for (const candidate of items) {
    if (candidate.kind === 'players') playersById.set(candidate.id, candidate)
    if (candidate.ownerId && candidate.guildKey) guildByOwnerId.set(candidate.ownerId, candidate.guildKey)
    if (candidate.kind !== 'bases') continue
    basesById.set(candidate.id, candidate)
    if (candidate.baseId) basesById.set(candidate.baseId, candidate)
  }

  const base =
    item.kind === 'bases' ? item : item.kind === 'workers' && item.baseId ? basesById.get(item.baseId) : undefined
  const owner = item.ownerId ? playersById.get(item.ownerId) : undefined
  const playerGuild = (player: MapItem) => player.guildKey || guildByOwnerId.get(player.id)
  const guildKey =
    item.guildKey || base?.guildKey || (item.kind === 'players' ? playerGuild(item) : owner && playerGuild(owner))
  const guild = guildKey ? buildGuildDetails(guildKey, items) : undefined
  const guildMembers = guild?.members || []
  const guildBases = guild?.bases || []
  const guildPals = guild?.pals || []
  const baseKey = base ? itemBaseKey(base) : undefined
  const basePals = baseKey
    ? items.filter(
        (candidate) =>
          candidate.kind === 'workers' &&
          candidate.baseId !== undefined &&
          (candidate.baseId === baseKey || candidate.baseId === base?.id)
      )
    : []
  const ownerId = item.kind === 'players' ? item.id : owner?.id
  const ownerPals = ownerId
    ? items.filter((candidate) => candidate.kind === 'companions' && candidate.ownerId === ownerId)
    : []
  const relatedPals = (item.kind === 'players' || item.kind === 'companions' ? ownerPals : basePals)
    .filter((candidate) => candidate.id !== item.id)
    .sort(compareItems)
  return {
    base,
    owner,
    guildKey,
    guildName: guild?.name,
    guildMembers,
    guildBases,
    guildPals,
    relatedPals
  }
}

function compareItems(left: MapItem, right: MapItem) {
  return (
    left.name.localeCompare(right.name) || (left.level || 0) - (right.level || 0) || left.id.localeCompare(right.id)
  )
}

function plural(count: number, singular: string) {
  return `${count} ${singular}${count === 1 ? '' : 's'}`
}

function levelLabel(item: MapItem) {
  return item.level ? `${item.name} · Lv ${item.level}` : item.name
}

function coordinates(item: MapItem) {
  return `X ${Math.round(item.x)}\u00a0\u00a0Y ${Math.round(item.y)}`
}

function altitude(z?: number) {
  if (z === undefined || !Number.isFinite(z)) return undefined
  return `${Math.round(z / 100).toLocaleString()} m`
}

function lastSeen(lastSeenAt?: string) {
  if (!lastSeenAt) return undefined
  const timestamp = new Date(lastSeenAt)
  if (Number.isNaN(timestamp.getTime())) return undefined
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(timestamp)
}

function baseLabel(base: MapItem, guildBases: MapItem[]) {
  if (guildBases.length <= 1) return base.name
  const index = guildBases.findIndex((candidate) => candidate.id === base.id)
  return index < 0 ? base.name : `Base ${index + 1}`
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h3 className="m-0 mb-2 border-l-[3px] border-[#a8f6ff] bg-[#38494f]/80 px-2 py-1 text-xs font-normal tracking-[.08em] text-[#edf9fb] uppercase">
      {children}
    </h3>
  )
}

interface ItemLinkProps {
  item: MapItem
  relation: string
  title: string
  detail?: string
  showRelation?: boolean
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
}

function ItemLink({ item, relation, title, detail, showRelation = false, onSelectItem }: ItemLinkProps) {
  return (
    <button
      type="button"
      className="pal-glass-inset pal-interactive grid min-h-11 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-2.5 py-2 text-left text-xs focus-visible:outline-none"
      aria-label={`View ${relation} ${title}`}
      onClick={(event) => onSelectItem(item, event.currentTarget)}
    >
      <span className="min-w-0">
        {showRelation ? (
          <span className="mb-0.5 block text-[10px] tracking-[.1em] text-[#75cbd6] uppercase">{relation}</span>
        ) : null}
        <span className="block truncate text-[#f0f9fa]">{title}</span>
        {detail ? <span className="mt-0.5 block truncate text-[10px] text-[#8fa4aa]">{detail}</span> : null}
      </span>
      <IconChevronRight className="size-4 text-[#63cddd]" aria-hidden="true" />
    </button>
  )
}

function GuildLink({
  guildId,
  name,
  memberCount,
  onlineMemberCount,
  baseCount,
  palCount,
  onSelectGuild
}: {
  guildId: string
  name: string
  memberCount: number
  onlineMemberCount: number
  baseCount: number
  palCount: number
  onSelectGuild: (guildId: string, focus: HTMLElement) => void
}) {
  return (
    <button
      type="button"
      className="pal-glass-inset pal-interactive grid min-h-14 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-2.5 text-left focus-visible:outline-none"
      aria-label={`View guild ${name}`}
      onClick={(event) => onSelectGuild(guildId, event.currentTarget)}
    >
      <span className="min-w-0">
        <span className="block text-[10px] tracking-[.12em] text-[#75cbd6] uppercase">Guild</span>
        <strong className="mt-0.5 block truncate text-sm font-medium text-[#f0fafb]">{name}</strong>
        <span className="mt-1 block text-[10px] text-[#91a6ac]">
          {plural(onlineMemberCount, 'online player')}
          {memberCount === onlineMemberCount ? '' : ` · ${plural(memberCount, 'member')}`} · {plural(baseCount, 'base')}{' '}
          · {plural(palCount, 'Pal')}
        </span>
      </span>
      <IconChevronRight className="size-4 text-[#63cddd]" aria-hidden="true" />
    </button>
  )
}

interface RelatedItemListProps {
  items: MapItem[]
  relation: string
  guildBases?: MapItem[]
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
}

function RelatedItemList({ items, relation, guildBases, onSelectItem }: RelatedItemListProps) {
  const rendered = items.slice(0, DETAIL_WORKER_LIMIT)
  return (
    <>
      <ul className="m-0 grid list-none gap-1.5 p-0">
        {rendered.map((related) => (
          <li key={related.id}>
            <ItemLink
              item={related}
              relation={relation}
              title={related.kind === 'bases' ? baseLabel(related, guildBases || items) : levelLabel(related)}
              detail={related.kind === 'bases' ? coordinates(related) : related.detail || kindLabel(related.kind)}
              onSelectItem={onSelectItem}
            />
          </li>
        ))}
      </ul>
      {rendered.length < items.length ? (
        <p className="mt-2 border-l-2 border-[#64d7e7]/40 px-2 py-1.5 text-[11px] text-[#9ec1c7]">
          {items.length - rendered.length} more items are hidden from this panel. Use map search to find them.
        </p>
      ) : null}
    </>
  )
}

function RelatedItems({ title, items, relation, guildBases, onSelectItem }: RelatedItemListProps & { title: string }) {
  if (items.length === 0) return null
  return (
    <section>
      <SectionTitle>{title}</SectionTitle>
      <RelatedItemList items={items} relation={relation} guildBases={guildBases} onSelectItem={onSelectItem} />
    </section>
  )
}

function GuildRoster({
  title,
  items,
  relation,
  empty,
  guildBases,
  onSelectItem
}: RelatedItemListProps & { title: string; empty: string }) {
  return (
    <section>
      <SectionTitle>{title}</SectionTitle>
      {items.length > 0 ? (
        <RelatedItemList items={items} relation={relation} guildBases={guildBases} onSelectItem={onSelectItem} />
      ) : (
        <p className="m-0 text-[13px] text-[#8f989d]">{empty}</p>
      )}
    </section>
  )
}

function GuildDetails({
  guild,
  layers,
  onSelectItem
}: {
  guild: GuildDetailsModel
  layers: MapLayer[]
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
}) {
  const regions = Array.from(
    new Set(
      [...guild.members, ...guild.bases, ...guild.pals].map(
        (item) => layers.find((layer) => layer.id === item.map)?.name || item.map
      )
    )
  )

  return (
    <>
      <FactList
        entries={[
          ['Members', guild.members.length],
          ['Online members', guild.onlineMembers.length],
          ['Bases', guild.bases.length],
          ['Pals', guild.pals.length],
          ['Regions', regions.join(' · ')]
        ]}
      />
      <GuildRoster
        title="Online members"
        items={guild.onlineMembers}
        relation="guild member"
        empty="No guild members are currently online."
        onSelectItem={onSelectItem}
      />
      <GuildRoster
        title="Offline members"
        items={guild.members.filter((member) => member.online === false)}
        relation="guild member"
        empty="No guild members are currently offline."
        onSelectItem={onSelectItem}
      />
      <GuildRoster
        title="Bases"
        items={guild.bases}
        relation="guild base"
        empty="No bases are linked to this guild."
        guildBases={guild.bases}
        onSelectItem={onSelectItem}
      />
      <GuildRoster
        title="Pals"
        items={guild.pals}
        relation="guild Pal"
        empty="No Pals are currently linked to this guild."
        onSelectItem={onSelectItem}
      />
    </>
  )
}

function ItemDetails({
  item,
  items,
  layers,
  manualChecklist,
  onSelectItem,
  onSelectGuild
}: {
  item: MapItem
  items: MapItem[]
  layers: MapLayer[]
  manualChecklist: ManualChecklistDetails
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectGuild: (guildId: string, focus: HTMLElement) => void
}) {
  const relationships = buildRelationships(item, items)
  const { base, owner, guildKey, guildName, guildMembers, guildBases, guildPals, relatedPals } = relationships

  const entries: Array<[string, string | number | undefined]> = []
  const journalPreview = item.kind === 'journals' ? item.detail : undefined
  const rewards = item.rewards || []
  if (item.level) entries.push(['Level', item.level])
  if (item.kind === 'players') {
    entries.push(['Status', item.online === false ? 'Offline' : 'Online'])
    entries.push(['Last seen', lastSeen(item.lastSeenAt)])
    entries.push(['Captures', item.captureTotal?.toLocaleString()])
    entries.push(['Species captured', item.uniquePalsCaptured?.toLocaleString()])
    entries.push(['Paldeck unlocked', item.paldeckUnlocked?.toLocaleString()])
    entries.push(['Arena RP', item.arenaRankPoints?.toLocaleString()])
    entries.push(['Fast-travel points', item.fastTravelUnlocked?.toLocaleString()])
    entries.push(['Areas discovered', item.areasDiscovered?.toLocaleString()])
    entries.push(['Boss clears', item.bossDefeats?.toLocaleString()])
    entries.push(['Tower clears', item.towerDefeats?.toLocaleString()])
  }
  if (item.detail && item.kind !== 'players' && item.kind !== 'journals' && rewards.length === 0) {
    entries.push([DETAIL_LABELS[item.kind], item.detail])
  }
  if (item.kind === 'bases') entries.push(['Assigned Pals', relatedPals.length])
  entries.push(['Region', layers.find((layer) => layer.id === item.map)?.name || item.map])
  entries.push(['Coordinates', coordinates(item)])
  entries.push(['Altitude', altitude(item.z)])

  const guildMembershipNotice =
    item.kind === 'bases'
      ? 'No guild is linked to this base.'
      : `No guild membership is known for this ${item.kind === 'players' ? 'player' : item.kind === 'companions' ? 'companion Pal' : item.kind === 'workers' ? 'worker Pal' : 'map item'}.`
  const relatedPalTitle =
    item.kind === 'players'
      ? 'Current companion Pals'
      : item.kind === 'companions' && owner
        ? `Other companion Pals with ${owner.name}`
        : item.kind === 'bases'
          ? 'Assigned Pals'
          : 'Other Pals assigned to this base'
  const relatedPalRelation = item.kind === 'players' || item.kind === 'companions' ? 'companion Pal' : 'assigned Pal'
  const hasGuildRelationships =
    item.kind === 'players' || item.kind === 'bases' || item.kind === 'workers' || item.kind === 'companions'

  return (
    <>
      <FactList entries={entries} />
      {journalPreview ? <JournalPreview preview={journalPreview} /> : null}
      <LandmarkRewards rewards={rewards} />
      {isChecklistItem(item) ? <ManualChecklistControl item={item} checklist={manualChecklist} /> : null}
      {hasGuildRelationships ? (
        <section>
          <SectionTitle>Guild</SectionTitle>
          <div className="grid gap-1.5">
            {guildKey ? (
              <GuildLink
                guildId={guildKey}
                name={guildName || 'Unnamed guild'}
                memberCount={guildMembers.length}
                onlineMemberCount={guildMembers.filter((member) => member.online !== false).length}
                baseCount={guildBases.length}
                palCount={guildPals.length}
                onSelectGuild={onSelectGuild}
              />
            ) : null}
            {owner ? (
              <ItemLink
                item={owner}
                relation="owner"
                title={levelLabel(owner)}
                detail={`${owner.online === false ? 'Offline' : 'Online'}${owner.guildName ? ` · ${owner.guildName}` : ''}`}
                showRelation
                onSelectItem={onSelectItem}
              />
            ) : item.ownerId ? (
              <p className="pal-glass-inset m-0 px-3 py-2.5 text-xs text-[#a9b7bc]">
                This companion Pal’s owner is not available in the current roster.
              </p>
            ) : null}
            {base && item.kind !== 'bases' ? (
              <ItemLink
                item={base}
                relation="assigned base"
                title={baseLabel(base, guildBases)}
                detail={coordinates(base)}
                showRelation
                onSelectItem={onSelectItem}
              />
            ) : null}
            {!guildKey ? <p className="m-0 text-[13px] text-[#8f989d]">{guildMembershipNotice}</p> : null}
          </div>
        </section>
      ) : null}
      <RelatedItems
        title={relatedPalTitle}
        items={relatedPals}
        relation={relatedPalRelation}
        onSelectItem={onSelectItem}
      />
      {item.kind === 'bases' && relatedPals.length === 0 ? (
        <p className="m-0 text-[13px] text-[#8f989d]">This base currently has no assigned Pals.</p>
      ) : null}
    </>
  )
}

function ManualChecklistControl({ item, checklist }: { item: MapItem; checklist: ManualChecklistDetails }) {
  const descriptionId = useId()
  const completed = checklist.completedIds.has(item.id)
  return (
    <section>
      <SectionTitle>{checklist.profileName}</SectionTitle>
      <div className="pal-glass-inset grid gap-2.5 p-3">
        <div>
          <p className="m-0 text-[10px] tracking-[.1em] text-[#78c6d0] uppercase">Manual · this browser</p>
          <p id={descriptionId} className="mt-1 mb-0 text-[11px] leading-4 text-[#9fb0b5]">
            This manual mark is stored only in this browser. It is separate from save-backed progress.
          </p>
        </div>
        <label
          className={`pal-interactive flex min-h-11 cursor-pointer items-center gap-2.5 border px-3 py-2 text-xs focus-within:border-[#8de9f5] ${
            completed
              ? 'pal-selected border-[#72d7e5]/55 text-[#effafb]'
              : 'border-[#8bb7bd]/25 bg-[#26363b]/55 text-[#d6e7e9]'
          }`}
        >
          <input
            type="checkbox"
            className="size-4 shrink-0 accent-[#63c9d8]"
            checked={completed}
            aria-label={`Mark ${item.name} complete in ${checklist.profileName}`}
            aria-describedby={descriptionId}
            onChange={(event) => checklist.onSetCompletion(item.id, event.currentTarget.checked)}
          />
          <span>{completed ? 'Completed' : 'Mark complete'}</span>
        </label>
      </div>
    </section>
  )
}
