import { useEffect, useRef } from 'react'
import { buildGuildDetails, type GuildDetails as GuildDetailsModel } from '../lib/guilds'
import { LEADERBOARDS, type LeaderboardId, leaderboardById } from '../lib/leaderboards'
import { kindLabel } from '../lib/map'
import type { MapItem, MapLayer } from '../types'
import { MarkerGlyph } from './MarkerGlyph'

export type Detail =
  | { kind: 'item'; itemId: string }
  | { kind: 'guild'; guildId: string }
  | { kind: 'leaderboard'; leaderboardId: LeaderboardId }

const DETAIL_WORKER_LIMIT = 250

interface DetailsDialogProps {
  detail: Detail | null
  items: MapItem[]
  layers: MapLayer[]
  rosterNotice: string | null
  returnFocus: HTMLElement | null
  fallbackFocus: HTMLElement | null
  onClose: () => void
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectGuild: (guildId: string, focus: HTMLElement) => void
  onSelectLeaderboard: (leaderboardId: LeaderboardId) => void
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
  rosterNotice,
  returnFocus,
  fallbackFocus,
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
        : `leaderboard:${detail.leaderboardId}`
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
      if (event.key !== 'Escape') return
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
    <aside
      className="pal-glass-panel surface-enter-motion absolute top-[78px] right-4 bottom-4 z-[24] flex w-[350px] flex-col overflow-hidden text-[#e5f0f2] max-sm:top-auto max-sm:right-0 max-sm:bottom-0 max-sm:left-0 max-sm:max-h-[49dvh] max-sm:w-auto max-sm:border-x-0 max-sm:border-b-0"
      role="dialog"
      aria-modal="false"
      aria-labelledby="details-title"
    >
      <header className="pal-panel-header relative z-[2] flex min-h-[78px] shrink-0 items-center justify-between gap-3.5 border-b pr-3.5 pl-5 [--pal-panel-accent:#72d7e5]">
        <div>
          <p className="m-0 mb-1 text-[10px] font-normal tracking-[.14em] text-[#b6f5fc]">{eyebrow}</p>
          <h2
            ref={titleRef}
            id="details-title"
            className="m-0 text-[22px] font-normal text-[#f3fbfc] outline-none"
            tabIndex={-1}
          >
            {title}
          </h2>
        </div>
        <button
          type="button"
          className="pal-interactive grid size-11 cursor-pointer place-items-center border-0 bg-transparent text-xl text-[#d7eef1]"
          aria-label="Close details"
          onClick={close}
        >
          ×
        </button>
      </header>
      <div ref={bodyRef} className="min-h-0 flex-1 overflow-y-auto overscroll-contain" data-details-body>
        <div className="grid gap-5 p-[18px] max-sm:p-3.5">
          {item ? (
            <ItemDetails
              item={item}
              items={items}
              layers={layers}
              onSelectItem={onSelectItem}
              onSelectGuild={onSelectGuild}
            />
          ) : guild ? (
            <GuildDetails guild={guild} layers={layers} onSelectItem={onSelectItem} />
          ) : leaderboard ? (
            <LeaderboardDetails
              leaderboardId={leaderboard.id}
              items={items}
              rosterNotice={rosterNotice}
              onSelectItem={onSelectItem}
              onSelectLeaderboard={onSelectLeaderboard}
            />
          ) : null}
        </div>
      </div>
    </aside>
  )
}

function LeaderboardDetails({
  leaderboardId,
  items,
  rosterNotice,
  onSelectItem,
  onSelectLeaderboard
}: {
  leaderboardId: LeaderboardId
  items: MapItem[]
  rosterNotice: string | null
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectLeaderboard: (leaderboardId: LeaderboardId) => void
}) {
  const leaderboard = leaderboardById(leaderboardId)
  const entries = leaderboard.entries(items)
  return (
    <>
      <nav className="grid gap-1.5" aria-label="Leaderboard types">
        {LEADERBOARDS.map((candidate) => (
          <button
            key={candidate.id}
            type="button"
            className={`min-h-11 cursor-pointer border px-3 text-left text-xs transition-colors focus-visible:border-[#8de9f5] focus-visible:outline-none ${
              candidate.id === leaderboard.id
                ? 'pal-selected shadow-[inset_3px_0_#72d7e5]'
                : 'pal-glass-inset pal-interactive text-[#9fb0b5]'
            }`}
            aria-current={candidate.id === leaderboard.id ? 'page' : undefined}
            onClick={() => onSelectLeaderboard(candidate.id)}
          >
            {candidate.title}
          </button>
        ))}
      </nav>
      <section>
        <SectionTitle>{leaderboard.title}</SectionTitle>
        <p className="mt-0 mb-3 text-xs leading-5 text-[#9fb0b5]">{leaderboard.description}</p>
        {rosterNotice ? (
          <p className="my-3 border border-[#66583d] bg-[#302b22] px-3 py-2.5 text-[11px] leading-4 text-[#d8c18a]">
            {rosterNotice}
          </p>
        ) : null}
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
          <p className="m-0 text-[13px] text-[#8f989d]">No players are currently known.</p>
        )}
      </section>
    </>
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

function formatTimestamp(value?: string) {
  if (!value) return undefined
  const timestamp = new Date(value)
  return Number.isNaN(timestamp.getTime())
    ? value
    : timestamp.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })
}

function formatCount(value?: number) {
  return value === undefined ? undefined : value.toLocaleString()
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
      <span className="text-base text-[#63cddd]" aria-hidden="true">
        ›
      </span>
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
      <span className="text-base text-[#63cddd]" aria-hidden="true">
        ›
      </span>
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
  onSelectItem,
  onSelectGuild
}: {
  item: MapItem
  items: MapItem[]
  layers: MapLayer[]
  onSelectItem: (item: MapItem, focus: HTMLElement) => void
  onSelectGuild: (guildId: string, focus: HTMLElement) => void
}) {
  const relationships = buildRelationships(item, items)
  const { base, owner, guildKey, guildName, guildMembers, guildBases, guildPals, relatedPals } = relationships

  const entries: Array<[string, string | number | undefined]> = []
  if (item.level) entries.push(['Level', item.level])
  if (item.kind === 'players') {
    entries.push(['Status', item.online === false ? 'Offline' : 'Online'])
    entries.push(['Pals caught', formatCount(item.captureTotal)])
    entries.push(['Unique Pals caught', formatCount(item.uniquePalsCaptured)])
    entries.push(['Paldeck unlocked', formatCount(item.paldeckUnlocked)])
    if (item.online === false) {
      entries.push(['Position', 'Last saved'])
      entries.push(['Last seen', formatTimestamp(item.lastSeenAt)])
    }
  }
  if (item.detail && item.kind !== 'players') {
    const detailLabel =
      item.kind === 'bases'
        ? 'Description'
        : item.kind === 'npcs'
          ? 'Type'
          : item.kind === 'bosses' || item.kind === 'alpha-pals'
            ? 'Encounter'
            : 'Species'
    entries.push([detailLabel, item.detail])
  }
  if (item.kind === 'bases') entries.push(['Assigned Pals', relatedPals.length])
  entries.push(['Region', layers.find((layer) => layer.id === item.map)?.name || item.map])
  entries.push(['Coordinates', coordinates(item)])

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
