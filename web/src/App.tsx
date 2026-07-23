import { useEffect, useMemo, useRef, useState } from 'react'
import { type Detail, DetailsDialog } from './components/DetailsDialog'
import { Explorer } from './components/Explorer'
import { MapViewport, type MapViewportHandle } from './components/MapViewport'
import { ProjectLinks } from './components/ProjectLinks'
import { StatusBar } from './components/StatusBar'
import { usePolling } from './hooks/usePolling'
import { guildIdForBase } from './lib/guilds'
import type { LeaderboardId } from './lib/leaderboards'
import {
  DEFAULT_ENABLED_KINDS,
  DEFAULT_ENABLED_PLAYER_STATUSES,
  loadFilterPreferences,
  saveFilterPreferences
} from './lib/preferences'
import {
  EMPTY_OBJECT_STATE,
  type ItemKind,
  type MapItem,
  type MapLayer,
  type ObjectState,
  type PlayerState,
  type PlayerStatus,
  type PublicConfig
} from './types'

// Trophy icon from Primer Octicons (MIT): https://primer.style/octicons/icon/trophy-24/
function LeaderboardIcon() {
  return (
    <svg className="size-6" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M5.09 10.121A5.251 5.251 0 0 1 1 5V3.75C1 2.784 1.784 2 2.75 2h2.364c.236-.586.81-1 1.48-1h10.812c.67 0 1.244.414 1.48 1h2.489c.966 0 1.75.784 1.75 1.75V5a5.252 5.252 0 0 1-4.219 5.149 7.01 7.01 0 0 1-4.644 5.478l.231 3.003.034.031c.079.065.303.203.836.282.838.124 1.637.81 1.637 1.807v.75h2.25a.75.75 0 0 1 0 1.5H4.75a.75.75 0 0 1 0-1.5H7v-.75c0-.996.8-1.683 1.637-1.807.533-.08.757-.217.836-.282l.034-.031.231-3.003A7.012 7.012 0 0 1 5.09 10.12ZM6.5 2.594V9a5.5 5.5 0 1 0 11 0V2.594a.094.094 0 0 0-.094-.094H6.594a.094.094 0 0 0-.094.094Zm4.717 13.363-.215 2.793-.001.021-.003.043a1.212 1.212 0 0 1-.022.147c-.05.237-.194.567-.553.86-.348.286-.853.5-1.566.605a.478.478 0 0 0-.274.136.264.264 0 0 0-.083.188v.75h7v-.75a.264.264 0 0 0-.083-.188.478.478 0 0 0-.274-.136c-.713-.105-1.218-.32-1.567-.604-.358-.294-.502-.624-.552-.86a1.22 1.22 0 0 1-.025-.19l-.001-.022-.215-2.793a7.069 7.069 0 0 1-1.566 0ZM19 8.578A3.751 3.751 0 0 0 21.625 5V3.75a.25.25 0 0 0-.25-.25H19ZM5 3.5H2.75a.25.25 0 0 0-.25.25V5A3.752 3.752 0 0 0 5 8.537Z" />
    </svg>
  )
}

function releaseVersionParts(version: string | undefined) {
  const match = version?.trim().match(/^v?(\d+(?:\.\d+){2,3})$/i)
  return match?.[1].split('.') || []
}

function landmarkCatalogueCompatibility(catalogueVersion: string, serverVersion: string | undefined) {
  if (serverVersion?.trim().toLowerCase() === '1.0 demo') return 'compatible'
  const catalogueParts = releaseVersionParts(catalogueVersion)
  const serverParts = releaseVersionParts(serverVersion)
  if (catalogueParts.length === 0 || serverParts.length === 0) return 'unverifiable'
  return catalogueParts.length === serverParts.length &&
    catalogueParts.every((part, index) => part === serverParts[index])
    ? 'compatible'
    : 'mismatch'
}

export function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [configError, setConfigError] = useState(false)
  const [configAttempt, setConfigAttempt] = useState(0)

  // biome-ignore lint/correctness/useExhaustiveDependencies: incrementing configAttempt deliberately retries the request
  useEffect(() => {
    const controller = new AbortController()
    const load = async () => {
      try {
        const response = await fetch('/api/config', { cache: 'no-store', signal: controller.signal })
        if (!response.ok) throw new Error(`/api/config returned ${response.status}`)
        setConfig((await response.json()) as PublicConfig)
      } catch {
        if (!controller.signal.aborted) setConfigError(true)
      }
    }
    void load()
    return () => controller.abort()
  }, [configAttempt])

  if (!config) {
    return (
      <div className="relative h-dvh overflow-hidden bg-[#171a1d] text-[#f4f5f5]">
        <StatusBar playerState={null} offline={configError} />
        <main className="absolute inset-0 grid place-items-center bg-[#111416] text-sm text-[#8f989d]">
          {configError ? (
            <div className="grid justify-items-center gap-3">
              <p className="m-0">Map unavailable</p>
              <button
                type="button"
                className="pal-glass-control min-h-11 cursor-pointer px-4 text-xs text-[#e5f7f8]"
                onClick={() => {
                  setConfigError(false)
                  setConfigAttempt((attempt) => attempt + 1)
                }}
              >
                Retry
              </button>
            </div>
          ) : (
            'Loading map…'
          )}
        </main>
      </div>
    )
  }

  return <LiveMap config={config} />
}

function LiveMap({ config }: { config: PublicConfig }) {
  const players = usePolling<PlayerState>('/api/players', config.pollIntervalMs)
  const objects = usePolling<ObjectState>('/api/objects', config.worldPollIntervalMs, config.worldDataEnabled)
  const playerState = players.data
  const objectState = objects.data || { ...EMPTY_OBJECT_STATE, enabled: config.worldDataEnabled }
  const initialPreferences = useMemo(loadFilterPreferences, [])
  const [activeLayer, setActiveLayer] = useState<MapLayer>(
    () => config.layers.find((layer) => layer.id === initialPreferences.activeLayerId) || config.layers[0]
  )
  const [enabledKinds, setEnabledKinds] = useState(
    () => new Set<ItemKind>(initialPreferences.enabledKinds || DEFAULT_ENABLED_KINDS)
  )
  const [enabledPlayerStatuses, setEnabledPlayerStatuses] = useState(
    () => new Set<PlayerStatus>(initialPreferences.enabledPlayerStatuses || DEFAULT_ENABLED_PLAYER_STATUSES)
  )
  const [hiddenIds, setHiddenIds] = useState(() => initialPreferences.hiddenIds || new Set<string>())
  const [expandedGuilds, setExpandedGuilds] = useState(() => new Set<string>())
  const [expandedBases, setExpandedBases] = useState(() => new Set<string>())
  const [search, setSearch] = useState('')
  const [filtersOpen, setFiltersOpen] = useState(() => typeof window === 'undefined' || window.innerWidth >= 640)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(null)
  const mapRef = useRef<MapViewportHandle>(null)
  const pendingFocusRef = useRef<{ itemId: string; returnFocus: HTMLElement } | null>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const leaderboardButtonRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    saveFilterPreferences({ activeLayerId: activeLayer.id, enabledKinds, enabledPlayerStatuses, hiddenIds })
  }, [activeLayer.id, enabledKinds, enabledPlayerStatuses, hiddenIds])

  const items = useMemo<MapItem[]>(() => {
    const combined: MapItem[] = [
      ...(config.landmarks || []),
      ...(objectState.objects || []),
      ...(playerState?.players || []).map((player) => ({
        ...player,
        kind: 'players' as const,
        online: player.online !== false,
        detail: `${player.online === false ? 'Offline' : 'Online'} · Level ${player.level}`
      }))
    ]
    return Array.from(new Map(combined.map((item) => [item.id, item])).values())
  }, [config.landmarks, objectState.objects, playerState?.players])
  const itemById = useMemo(() => new Map(items.map((item) => [item.id, item])), [items])
  const detailedItem = detail?.kind === 'item' ? itemById.get(detail.itemId) : undefined
  const detailedGuildExists =
    detail?.kind === 'guild' &&
    items.some(
      (item) => item.guildKey === detail.guildId || (item.kind === 'bases' && guildIdForBase(item) === detail.guildId)
    )

  useEffect(() => {
    document.title = playerState?.server.name || 'Palworld Live Map'
  }, [playerState?.server.name])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      if ((event.target as HTMLElement).matches('input, textarea, select')) return
      event.preventDefault()
      setFiltersOpen(true)
      window.requestAnimationFrame(() => searchRef.current?.focus())
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  useEffect(() => {
    if (!detail) return
    if (detail.kind === 'leaderboard') return
    if (detail.kind === 'guild' ? detailedGuildExists : detailedItem?.map === activeLayer.id) return
    setDetail(null)
    mapRef.current?.clearSelection()
  }, [activeLayer.id, detail, detailedGuildExists, detailedItem])

  useEffect(() => {
    const pending = pendingFocusRef.current
    if (!pending) return
    const item = itemById.get(pending.itemId)
    if (!item || item.map !== activeLayer.id) return
    const frame = window.requestAnimationFrame(() => {
      if (pendingFocusRef.current !== pending) return
      pendingFocusRef.current = null
      const safeReturnFocus = pending.returnFocus.isConnected
        ? pending.returnFocus
        : leaderboardButtonRef.current || pending.returnFocus
      mapRef.current?.focusItem(item, safeReturnFocus)
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeLayer.id, itemById])

  const showItem = (item: MapItem, focus: HTMLElement) => {
    setReturnFocus(focus)
    setDetail({ kind: 'item', itemId: item.id })
  }

  const showGuild = (guildId: string, focus: HTMLElement) => {
    pendingFocusRef.current = null
    setSearch('')
    setReturnFocus(focus)
    mapRef.current?.clearSelection()
    setDetail({ kind: 'guild', guildId })
  }

  const showLeaderboard = (leaderboardId: LeaderboardId, focus: HTMLElement) => {
    pendingFocusRef.current = null
    setReturnFocus(focus)
    mapRef.current?.clearSelection()
    setDetail({ kind: 'leaderboard', leaderboardId })
  }

  const focusItem = (item: MapItem, focus: HTMLElement) => {
    setEnabledKinds((current) => {
      const next = new Set(current)
      if (item.kind === 'workers') {
        next.add('bases')
        next.add('workers')
      } else {
        next.add(item.kind)
      }
      return next
    })
    if (item.kind === 'players') {
      setEnabledPlayerStatuses((current) => {
        const next = new Set(current)
        next.add(item.online === false ? 'offline' : 'online')
        return next
      })
    }
    setHiddenIds((current) => {
      const next = new Set(current)
      next.delete(item.id)
      return next
    })
    if (item.map !== activeLayer.id) {
      const layer = config.layers.find((candidate) => candidate.id === item.map)
      if (!layer) return
      pendingFocusRef.current = { itemId: item.id, returnFocus: focus }
      setReturnFocus(focus)
      setDetail(null)
      mapRef.current?.clearSelection()
      setActiveLayer(layer)
      return
    }
    mapRef.current?.focusItem(item, focus)
  }

  const toggleKinds = (kinds: ItemKind[], visible: boolean) => {
    setEnabledKinds((current) => {
      const next = new Set(current)
      for (const kind of kinds) visible ? next.add(kind) : next.delete(kind)
      return next
    })
    if (visible) {
      setHiddenIds((current) => {
        const next = new Set(current)
        for (const item of items) {
          if (item.map === activeLayer.id && kinds.includes(item.kind)) next.delete(item.id)
        }
        return next
      })
    }
  }

  const toggleItems = (ids: string[], visible: boolean) => {
    setHiddenIds((current) => {
      const next = new Set(current)
      for (const id of ids) visible ? next.delete(id) : next.add(id)
      return next
    })
  }

  const togglePlayerStatus = (status: PlayerStatus, visible: boolean) => {
    setEnabledPlayerStatuses((current) => {
      const next = new Set(current)
      if (visible) next.add(status)
      else next.delete(status)
      return next
    })
    if (!visible) return
    setEnabledKinds((current) => new Set(current).add('players'))
    setHiddenIds((current) => {
      const next = new Set(current)
      for (const item of items) {
        if (
          item.kind === 'players' &&
          item.map === activeLayer.id &&
          (item.online === false ? 'offline' : 'online') === status
        )
          next.delete(item.id)
      }
      return next
    })
  }

  const toggleSetValue = (setter: React.Dispatch<React.SetStateAction<Set<string>>>, id: string) => {
    setter((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  let objectNotice: string | null = null
  const retainedLimitNotice = objectState.truncated
    ? ` It contains ${objectState.objects.length.toLocaleString()} of ${objectState.total.toLocaleString()} projected objects.`
    : ''
  if (!config.worldDataEnabled || !objectState.enabled)
    objectNotice = 'Extra live layers are disabled by this map’s configuration.'
  else if (objectState.unsupported)
    objectNotice = 'Extra live layers need ENABLE_GAMEDATA_API=true and a Palworld server restart.'
  else if (objectState.lastError === 'response-too-large')
    objectNotice = objectState.available
      ? `The latest world object response exceeded the safety limit; showing the last successful snapshot.${retainedLimitNotice}`
      : 'The world object response exceeded the configured safety limit.'
  else if (objectState.lastError === 'refresh-failed')
    objectNotice = objectState.available
      ? `World object refresh failed; showing the last successful snapshot.${retainedLimitNotice}`
      : 'World objects are temporarily unavailable.'
  else if (objects.error)
    objectNotice = objectState.available
      ? `The map lost contact with its object API; showing the last successful snapshot.${retainedLimitNotice}`
      : 'World objects are temporarily unavailable.'
  else if (objectState.stale)
    objectNotice = `World objects are using the last successful snapshot.${retainedLimitNotice}`
  else if (objectState.truncated || objectState.lastError === 'object-limit')
    objectNotice = `Showing ${objectState.objects.length.toLocaleString()} of ${objectState.total.toLocaleString()} world objects; this snapshot reached the configured limit.`
  else if (!objectState.available) objectNotice = 'Loading bases, Pals and NPCs…'

  const catalogueCompatibility = playerState
    ? landmarkCatalogueCompatibility(config.landmarkCatalogue.gameVersion, playerState.server.version)
    : 'compatible'
  if (catalogueCompatibility === 'mismatch') {
    const catalogueNotice = `Landmark catalogue version mismatch: locations were exported for Palworld ${config.landmarkCatalogue.gameVersion}, but this server reports ${playerState?.server.version}. Alpha Pal and Tower Boss locations may be outdated; regenerate them with make game-assets.`
    objectNotice = objectNotice ? `${objectNotice} ${catalogueNotice}` : catalogueNotice
  } else if (catalogueCompatibility === 'unverifiable') {
    const reportedVersion = playerState?.server.version?.trim()
    const reason = reportedVersion
      ? `the server reports an unrecognised version (${reportedVersion})`
      : 'the server did not report a version'
    const catalogueNotice = `Landmark catalogue compatibility could not be verified because ${reason}. Alpha Pal and Tower Boss locations may be outdated; regenerate them with make game-assets after confirming the installed game version.`
    objectNotice = objectNotice ? `${objectNotice} ${catalogueNotice}` : catalogueNotice
  }

  const explorerProps = {
    activeLayer,
    layers: config.layers,
    items,
    search,
    searchInputRef: searchRef,
    enabledKinds,
    enabledPlayerStatuses,
    hiddenIds,
    expandedGuilds,
    expandedBases,
    objectNotice,
    onSearchChange: setSearch,
    onToggleKinds: toggleKinds,
    onTogglePlayerStatus: togglePlayerStatus,
    onToggleItems: toggleItems,
    onToggleGuild: (id: string) => toggleSetValue(setExpandedGuilds, id),
    onToggleBase: (id: string) => toggleSetValue(setExpandedBases, id),
    onFocusItem: focusItem,
    onFocusGuild: showGuild,
    onClose: () => setFiltersOpen(false),
    onLayerChange: (layer: MapLayer) => {
      pendingFocusRef.current = null
      setDetail(null)
      mapRef.current?.clearSelection()
      setActiveLayer(layer)
    }
  }

  return (
    <div className="relative h-dvh overflow-hidden bg-[#171a1d] text-[#f4f5f5]">
      <StatusBar playerState={playerState} offline={Boolean(players.error)} />
      <main className="absolute inset-0 overflow-hidden bg-[#0d161e]">
        <Explorer {...explorerProps} open={filtersOpen} onOpen={() => setFiltersOpen(true)} />
        <div className="relative size-full min-h-0 min-w-0 overflow-hidden">
          <MapViewport
            ref={mapRef}
            activeLayer={activeLayer}
            items={items}
            enabledKinds={enabledKinds}
            enabledPlayerStatuses={enabledPlayerStatuses}
            hiddenIds={hiddenIds}
            search={search}
            onShowItem={showItem}
            inspectorOpen={Boolean(detail)}
          >
            <button
              ref={leaderboardButtonRef}
              type="button"
              className={`pal-glass-control absolute top-[78px] z-20 grid size-12 cursor-pointer place-items-center text-[#dceef0] transition-[right,border-color,background-color,opacity,transform] focus-visible:outline-none max-sm:top-[88px] max-sm:size-11 ${
                detail?.kind === 'leaderboard'
                  ? 'pointer-events-none right-4 translate-y-1 opacity-0 max-sm:right-3'
                  : detail
                    ? 'right-[382px] max-[1180px]:hidden'
                    : 'right-4 max-sm:right-3'
              }`}
              aria-label="Open leaderboards"
              aria-haspopup="dialog"
              aria-expanded={detail?.kind === 'leaderboard'}
              aria-hidden={detail?.kind === 'leaderboard'}
              inert={detail?.kind === 'leaderboard'}
              title="Leaderboards"
              onClick={(event) => showLeaderboard('player-level', event.currentTarget)}
            >
              <LeaderboardIcon />
            </button>
            <ProjectLinks hidden={Boolean(detail)} />
            <DetailsDialog
              detail={detail}
              items={items}
              layers={config.layers}
              returnFocus={returnFocus}
              fallbackFocus={leaderboardButtonRef.current}
              onSelectItem={(item, focus) => {
                setSearch('')
                focusItem(item, returnFocus?.isConnected ? returnFocus : leaderboardButtonRef.current || focus)
              }}
              onSelectGuild={(guildId, focus) => {
                showGuild(guildId, returnFocus?.isConnected ? returnFocus : leaderboardButtonRef.current || focus)
              }}
              onSelectLeaderboard={(leaderboardId) => {
                setDetail({ kind: 'leaderboard', leaderboardId })
              }}
              onClose={() => {
                pendingFocusRef.current = null
                setDetail(null)
                mapRef.current?.clearSelection()
              }}
            />
          </MapViewport>
        </div>
      </main>
    </div>
  )
}
