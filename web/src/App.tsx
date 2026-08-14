import { useEffect, useMemo, useRef, useState } from 'react'
import { type Detail, DetailsDialog } from './components/DetailsDialog'
import { Explorer } from './components/Explorer'
import { MapViewport, type MapViewportHandle } from './components/MapViewport'
import { ProjectLinks } from './components/ProjectLinks'
import { StatusBar } from './components/StatusBar'
import { usePolling } from './hooks/usePolling'
import { buildGuildDetails, guildIdForBase } from './lib/guilds'
import type { LeaderboardId } from './lib/leaderboards'
import { itemSearchText } from './lib/map'
import {
  DEFAULT_ENABLED_KINDS,
  DEFAULT_ENABLED_PLAYER_STATUSES,
  FILTERABLE_KINDS,
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
  type PublicConfig,
  type WorldCatalogue
} from './types'

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
  const [catalogueError, setCatalogueError] = useState(false)
  const [catalogueAttempt, setCatalogueAttempt] = useState(0)

  // biome-ignore lint/correctness/useExhaustiveDependencies: incrementing configAttempt deliberately retries the request
  useEffect(() => {
    const controller = new AbortController()
    const load = async () => {
      try {
        const response = await fetch('/api/config', { cache: 'no-store', signal: controller.signal })
        if (!response.ok) throw new Error(`/api/config returned ${response.status}`)
        const nextConfig = (await response.json()) as PublicConfig
        setConfig(nextConfig)
        setConfigError(false)
      } catch {
        if (!controller.signal.aborted) setConfigError(true)
      }
    }
    void load()
    return () => controller.abort()
  }, [configAttempt])

  // The catalogue URL comes from the configuration, but the live map does not
  // need to wait for the larger catalogue response before it can start polling
  // and render the configured layers and legacy landmarks.
  // biome-ignore lint/correctness/useExhaustiveDependencies: incrementing catalogueAttempt deliberately retries the request
  useEffect(() => {
    if (!config) return
    const controller = new AbortController()
    const catalogueUrl = config.catalogueUrl
    const load = async () => {
      try {
        const response = await fetch(catalogueUrl, {
          cache: 'force-cache',
          signal: controller.signal
        })
        if (!response.ok) throw new Error(`${catalogueUrl} returned ${response.status}`)
        const catalogue = (await response.json()) as WorldCatalogue
        setConfig((current) => {
          if (!current || current.catalogueUrl !== catalogueUrl) return current
          const locations = Array.from(
            new Map(
              [...(current.landmarks || []), ...(catalogue.locations || [])].map((location) => [location.id, location])
            ).values()
          )
          return {
            ...current,
            landmarks: locations,
            landmarkCatalogue: {
              gameVersion: catalogue.gameVersion,
              generator: catalogue.generator,
              decoder: catalogue.decoder
            }
          }
        })
        setCatalogueError(false)
      } catch {
        if (!controller.signal.aborted) setCatalogueError(true)
      }
    }
    void load()
    return () => controller.abort()
  }, [catalogueAttempt, config?.catalogueUrl])

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

  return (
    <LiveMap
      config={config}
      catalogueError={catalogueError}
      onRetryCatalogue={() => {
        setCatalogueError(false)
        setCatalogueAttempt((attempt) => attempt + 1)
      }}
    />
  )
}

function LiveMap({
  config,
  catalogueError,
  onRetryCatalogue
}: {
  config: PublicConfig
  catalogueError: boolean
  onRetryCatalogue: () => void
}) {
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
  const [seenKinds, setSeenKinds] = useState(() => new Set<ItemKind>(initialPreferences.seenKinds || []))
  const [expandedGuilds, setExpandedGuilds] = useState(() => new Set<string>())
  const [expandedBases, setExpandedBases] = useState(() => new Set<string>())
  const [search, setSearch] = useState('')
  const [filtersOpen, setFiltersOpen] = useState(() => typeof window === 'undefined' || window.innerWidth >= 640)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(null)
  const mapRef = useRef<MapViewportHandle>(null)
  const pendingFocusRef = useRef<{ itemId: string; returnFocus: HTMLElement } | null>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const filterButtonRef = useRef<HTMLButtonElement>(null)
  const leaderboardButtonRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    saveFilterPreferences({ activeLayerId: activeLayer.id, enabledKinds, enabledPlayerStatuses, hiddenIds, seenKinds })
  }, [activeLayer.id, enabledKinds, enabledPlayerStatuses, hiddenIds, seenKinds])

  const items = useMemo<MapItem[]>(() => {
    const combined: MapItem[] = [
      ...(config.landmarks || []),
      ...(objectState.objects || []).filter((item) => item.kind !== 'wild-pals' && item.kind !== 'npcs'),
      ...(playerState?.players || []).map((player) => ({
        ...player,
        kind: 'players' as const,
        online: player.online !== false,
        detail: `${player.online === false ? 'Offline' : 'Online'} · Level ${player.level}`
      }))
    ]
    return Array.from(new Map(combined.map((item) => [item.id, item])).values())
  }, [config.landmarks, objectState.objects, playerState?.players])
  const presentedItems = useMemo(() => items.filter((item) => item.kind !== 'companions'), [items])
  const itemById = useMemo(() => new Map(items.map((item) => [item.id, item])), [items])

  // Reveal a category on the map the first time it has content, then remember it
  // so a kind the user later hides is never auto-enabled again.
  useEffect(() => {
    const unseen: ItemKind[] = []
    for (const item of presentedItems) {
      if (seenKinds.has(item.kind) || unseen.includes(item.kind)) continue
      unseen.push(item.kind)
    }
    if (unseen.length === 0) return
    setEnabledKinds((current) => {
      const next = new Set(current)
      for (const kind of unseen) next.add(kind)
      return next
    })
    setSeenKinds((current) => {
      const next = new Set(current)
      for (const kind of unseen) next.add(kind)
      return next
    })
  }, [presentedItems, seenKinds])
  const detailedItem = detail?.kind === 'item' ? itemById.get(detail.itemId) : undefined
  const detailedItemLayerId =
    detailedItem?.kind === 'companions' && detailedItem.ownerId
      ? itemById.get(detailedItem.ownerId)?.map || detailedItem.map
      : detailedItem?.map
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
    const reconcileMobilePanels = () => {
      if (window.innerWidth < 640 && detail?.kind === 'leaderboard') setFiltersOpen(false)
    }
    reconcileMobilePanels()
    window.addEventListener('resize', reconcileMobilePanels)
    return () => window.removeEventListener('resize', reconcileMobilePanels)
  }, [detail?.kind])

  useEffect(() => {
    if (!detail) return
    if (detail.kind === 'leaderboard') return
    if (detail.kind === 'guild' ? detailedGuildExists : detailedItemLayerId === activeLayer.id) return
    setDetail(null)
    mapRef.current?.clearSelection()
  }, [activeLayer.id, detail, detailedGuildExists, detailedItemLayerId])

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
    const query = search.trim().toLowerCase()
    if (query && !buildGuildDetails(guildId, items).name.toLowerCase().includes(query)) setSearch('')
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

  const mobilePanelLayout = () => window.innerWidth < 640

  const toggleFilters = () => {
    if (filtersOpen) {
      setFiltersOpen(false)
      return
    }
    if (mobilePanelLayout() && detail?.kind === 'leaderboard') {
      pendingFocusRef.current = null
      setDetail(null)
      mapRef.current?.clearSelection()
    }
    setFiltersOpen(true)
  }

  const toggleLeaderboards = (focus: HTMLButtonElement) => {
    if (detail?.kind !== 'leaderboard') {
      if (mobilePanelLayout()) setFiltersOpen(false)
      showLeaderboard('player-level', focus)
      return
    }
    pendingFocusRef.current = null
    setDetail(null)
    mapRef.current?.clearSelection()
  }

  const focusItem = (item: MapItem, focus: HTMLElement) => {
    if (item.kind === 'companions') {
      pendingFocusRef.current = null
      setReturnFocus(focus)
      setDetail({ kind: 'item', itemId: item.id })
      const owner = item.ownerId ? itemById.get(item.ownerId) : undefined
      const targetLayerId = owner?.map || item.map
      if (targetLayerId !== activeLayer.id) {
        const layer = config.layers.find((candidate) => candidate.id === targetLayerId)
        if (layer) setActiveLayer(layer)
      }
      return
    }
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
    const selectedItems = visible
      ? ids.map((id) => itemById.get(id)).filter((item): item is MapItem => Boolean(item))
      : []
    const newlyEnabledKinds = new Set(
      selectedItems.filter((item) => !enabledKinds.has(item.kind)).map((item) => item.kind)
    )
    const newlyEnabledPlayerStatuses = new Set(
      selectedItems
        .filter((item) => item.kind === 'players')
        .map((item) => (item.online === false ? 'offline' : 'online') as PlayerStatus)
        .filter((status) => !enabledPlayerStatuses.has(status))
    )
    if (visible) {
      setEnabledKinds((current) => {
        const next = new Set(current)
        for (const item of selectedItems) next.add(item.kind)
        return next
      })
      setEnabledPlayerStatuses((current) => {
        const next = new Set(current)
        for (const item of selectedItems) {
          if (item.kind === 'players') next.add(item.online === false ? 'offline' : 'online')
        }
        return next
      })
    }
    setHiddenIds((current) => {
      const next = new Set(current)
      if (visible && (newlyEnabledKinds.size > 0 || newlyEnabledPlayerStatuses.size > 0)) {
        for (const item of items) {
          const playerStatus = item.kind === 'players' ? (item.online === false ? 'offline' : 'online') : undefined
          if (
            newlyEnabledKinds.has(item.kind) ||
            (playerStatus !== undefined && newlyEnabledPlayerStatuses.has(playerStatus))
          )
            next.add(item.id)
        }
      }
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

  const uncheckAll = () => {
    setEnabledKinds(new Set())
    setEnabledPlayerStatuses(new Set())
    setSeenKinds((current) => {
      const next = new Set(current)
      for (const kind of FILTERABLE_KINDS) next.add(kind)
      return next
    })
  }

  const checkAll = () => {
    setEnabledKinds(new Set(FILTERABLE_KINDS))
    setEnabledPlayerStatuses(new Set(DEFAULT_ENABLED_PLAYER_STATUSES))
    setHiddenIds(new Set())
    setSeenKinds((current) => {
      const next = new Set(current)
      for (const kind of FILTERABLE_KINDS) next.add(kind)
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
  else if (!objectState.available) objectNotice = 'Loading guild bases and Pal relationships…'

  const catalogueCompatibility = playerState
    ? landmarkCatalogueCompatibility(config.landmarkCatalogue.gameVersion, playerState.server.version)
    : 'compatible'
  if (catalogueCompatibility === 'mismatch') {
    const catalogueNotice = `World catalogue version mismatch: locations were exported for Palworld ${config.landmarkCatalogue.gameVersion}, but this server reports ${playerState?.server.version}. Static locations may be outdated; regenerate them with make game-assets.`
    objectNotice = objectNotice ? `${objectNotice} ${catalogueNotice}` : catalogueNotice
  } else if (catalogueCompatibility === 'unverifiable') {
    const reportedVersion = playerState?.server.version?.trim()
    const reason = reportedVersion
      ? `the server reports an unrecognised version (${reportedVersion})`
      : 'the server did not report a version'
    const catalogueNotice = `World catalogue compatibility could not be verified because ${reason}. Static locations may be outdated; regenerate them with make game-assets after confirming the installed game version.`
    objectNotice = objectNotice ? `${objectNotice} ${catalogueNotice}` : catalogueNotice
  }

  const dataNotices = [
    playerState?.saveEnabled && playerState.saveLastError === 'resolve-failed'
      ? 'Offline player details are temporarily unavailable. Live players and saved progress remain available.'
      : null,
    objectNotice
  ].filter((notice): notice is string => notice !== null)

  const explorerProps = {
    activeLayer,
    layers: config.layers,
    items,
    search,
    filterButtonRef,
    searchInputRef: searchRef,
    enabledKinds,
    enabledPlayerStatuses,
    hiddenIds,
    expandedGuilds,
    expandedBases,
    dataNotices,
    catalogueRetry: catalogueError
      ? {
          message: 'Additional static map locations are temporarily unavailable.',
          onRetry: onRetryCatalogue
        }
      : undefined,
    onSearchChange: setSearch,
    onCheckAll: checkAll,
    onUncheckAll: uncheckAll,
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
      <StatusBar
        playerState={playerState}
        offline={Boolean(players.error)}
        controls={{
          filterButtonRef,
          filterSearch: search,
          filtersOpen,
          leaderboardButtonRef,
          leaderboardOpen: detail?.kind === 'leaderboard',
          onToggleFilters: toggleFilters,
          onToggleLeaderboards: toggleLeaderboards
        }}
      />
      <main className="absolute inset-0 overflow-hidden bg-[#0d161e]">
        <Explorer {...explorerProps} open={filtersOpen} />
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
            <ProjectLinks hidden={Boolean(detail && detail.kind !== 'leaderboard')} />
            <DetailsDialog
              detail={detail}
              items={items}
              layers={config.layers}
              returnFocus={returnFocus}
              fallbackFocus={leaderboardButtonRef.current}
              onSelectItem={(item, focus) => {
                const query = search.trim().toLowerCase()
                if (query && !itemSearchText(item).includes(query)) setSearch('')
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
