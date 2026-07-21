import { useEffect, useMemo, useRef, useState } from 'react'
import { type Detail, DetailsDialog } from './components/DetailsDialog'
import { Explorer } from './components/Explorer'
import { MapViewport, type MapViewportHandle } from './components/MapViewport'
import { ProjectLinks } from './components/ProjectLinks'
import { StatusBar } from './components/StatusBar'
import { usePolling } from './hooks/usePolling'
import { guildIdForBase } from './lib/guilds'
import { loadFilterPreferences, saveFilterPreferences } from './lib/preferences'
import {
  ALL_KINDS,
  EMPTY_OBJECT_STATE,
  type ItemKind,
  type MapItem,
  type MapLayer,
  type ObjectState,
  type PlayerState,
  type PublicConfig
} from './types'

function SearchToggleIcon({ className = '' }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m9 6 6 6-6 6" />
    </svg>
  )
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
      <div className="grid h-dvh grid-rows-[64px_1fr] bg-[#171a1d] text-[#f4f5f5] max-md:grid-rows-[76px_1fr]">
        <StatusBar playerState={null} offline={configError} />
        <main className="grid place-items-center bg-[#111416] text-sm text-[#8f989d]">
          {configError ? (
            <div className="grid justify-items-center gap-3">
              <p className="m-0">Map unavailable</p>
              <button
                type="button"
                className="min-h-11 cursor-pointer border border-[#64d7e7]/60 bg-[#112b33] px-4 text-xs text-[#e5f7f8] hover:border-[#9cebf4] hover:bg-[#173b46]"
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
    () => initialPreferences.enabledKinds || new Set<ItemKind>(ALL_KINDS)
  )
  const [hiddenIds, setHiddenIds] = useState(() => initialPreferences.hiddenIds || new Set<string>())
  const [expandedPlayers, setExpandedPlayers] = useState(() => new Set<string>())
  const [expandedGuilds, setExpandedGuilds] = useState(() => new Set<string>())
  const [expandedBases, setExpandedBases] = useState(() => new Set<string>())
  const [search, setSearch] = useState('')
  const [searchOpen, setSearchOpen] = useState(() => typeof window === 'undefined' || window.innerWidth >= 640)
  const [filtersOpen, setFiltersOpen] = useState(() => typeof window === 'undefined' || window.innerWidth >= 640)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(null)
  const mapRef = useRef<MapViewportHandle>(null)
  const pendingFocusRef = useRef<{ itemId: string; returnFocus: HTMLElement } | null>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const searchToggleRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    saveFilterPreferences({ activeLayerId: activeLayer.id, enabledKinds, hiddenIds })
  }, [activeLayer.id, enabledKinds, hiddenIds])

  const items = useMemo<MapItem[]>(
    () => [
      ...(playerState?.players || []).map((player) => ({
        ...player,
        kind: 'players' as const,
        detail: `Level ${player.level}`
      })),
      ...(objectState.objects || [])
    ],
    [objectState.objects, playerState?.players]
  )
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
      setSearchOpen(true)
      window.requestAnimationFrame(() => searchRef.current?.focus())
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  useEffect(() => {
    if (!detail) return
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
        : searchToggleRef.current || pending.returnFocus
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

  const explorerProps = {
    activeLayer,
    layers: config.layers,
    items,
    search,
    enabledKinds,
    hiddenIds,
    expandedPlayers,
    expandedGuilds,
    expandedBases,
    objectNotice,
    onToggleKinds: toggleKinds,
    onToggleItems: toggleItems,
    onTogglePlayer: (id: string) => toggleSetValue(setExpandedPlayers, id),
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
    <div className="grid h-dvh grid-rows-[64px_1fr] overflow-hidden bg-[#171a1d] text-[#f4f5f5] max-md:grid-rows-[76px_1fr]">
      <StatusBar playerState={playerState} offline={Boolean(players.error)} />
      <main className="relative min-h-0 w-full overflow-hidden bg-[#0d161e]">
        <Explorer {...explorerProps} open={filtersOpen} onOpen={() => setFiltersOpen(true)} />
        <div className="relative size-full min-h-0 min-w-0 overflow-hidden">
          <MapViewport
            ref={mapRef}
            activeLayer={activeLayer}
            items={items}
            enabledKinds={enabledKinds}
            hiddenIds={hiddenIds}
            search={search}
            onShowItem={showItem}
            inspectorOpen={Boolean(detail)}
          >
            <search
              id="map-search-control"
              aria-label="Map search"
              className={`absolute top-4 z-20 h-12 overflow-hidden border border-[#cceaef]/35 bg-[#081115]/95 shadow-[0_8px_22px_rgb(0_0_0/24%)] transition-[width,right,border-color,box-shadow] duration-200 ease-[cubic-bezier(0.22,1,0.36,1)] focus-within:border-[#62d6e7] focus-within:shadow-[inset_0_-2px_#22c7e8,0_8px_22px_rgb(0_0_0/24%)] motion-reduce:transition-none max-sm:top-3 ${
                searchOpen ? 'w-[min(420px,calc(100%_-_32px))] max-sm:w-[calc(100%_-_128px)]' : 'w-12'
              } ${detail ? 'right-[402px] max-[1180px]:hidden max-sm:right-3 max-sm:block' : 'right-4 max-sm:right-3'}`}
            >
              <div
                className={`absolute inset-y-0 right-0 left-12 flex min-w-0 items-center transition-[opacity,transform] duration-150 motion-reduce:transition-none ${
                  searchOpen ? 'translate-x-0 opacity-100 delay-50' : 'pointer-events-none translate-x-2 opacity-0'
                }`}
                aria-hidden={!searchOpen}
                inert={!searchOpen}
              >
                <label className="sr-only" htmlFor="map-search">
                  Search players, bases, Pals and NPCs
                </label>
                <input
                  id="map-search"
                  ref={searchRef}
                  type="search"
                  aria-label="Search players, bases, Pals and NPCs"
                  placeholder="Search map…"
                  autoComplete="off"
                  enterKeyHint="search"
                  spellCheck="false"
                  value={search}
                  className="h-full min-w-0 flex-1 appearance-none border-0 bg-transparent pl-1 text-sm tracking-[.02em] text-[#e7f6f8] outline-0 placeholder:text-[#60767d] [&::-webkit-search-cancel-button]:hidden [&::-webkit-search-decoration]:hidden"
                  onChange={(event) => setSearch(event.currentTarget.value)}
                  onKeyDown={(event) => {
                    if (event.key !== 'Escape') return
                    event.preventDefault()
                    event.stopPropagation()
                    if (search) {
                      setSearch('')
                    } else {
                      searchToggleRef.current?.focus({ preventScroll: true })
                      setSearchOpen(false)
                    }
                  }}
                />
                {search ? (
                  <button
                    type="button"
                    className="size-11 shrink-0 cursor-pointer border-0 bg-transparent text-lg text-[#739097] hover:text-white"
                    aria-label="Clear search"
                    onClick={() => {
                      setSearch('')
                      searchRef.current?.focus()
                    }}
                  >
                    ×
                  </button>
                ) : null}
              </div>
              <button
                ref={searchToggleRef}
                type="button"
                className="absolute inset-y-0 left-0 grid size-12 cursor-pointer place-items-center border-0 border-r border-white/10 bg-transparent text-[#65bbc7] transition-colors hover:bg-[#11282f] hover:text-[#8de9f5] focus-visible:outline-none"
                aria-label={
                  searchOpen
                    ? 'Collapse map search'
                    : search
                      ? `Open map search, current query: ${search}`
                      : 'Open map search'
                }
                aria-controls="map-search-control"
                aria-expanded={searchOpen}
                title={searchOpen ? 'Collapse search' : 'Open search'}
                onClick={() => {
                  if (searchOpen) {
                    setSearchOpen(false)
                  } else {
                    setSearchOpen(true)
                    window.requestAnimationFrame(() => searchRef.current?.focus())
                  }
                }}
              >
                <SearchToggleIcon
                  className={`size-5 transition-transform duration-200 motion-reduce:transition-none ${
                    searchOpen ? 'rotate-0' : 'rotate-180'
                  }`}
                />
                {!searchOpen && search ? (
                  <span
                    className="absolute top-2 right-2 size-1.5 rounded-full bg-[#55d4e7] shadow-[0_0_5px_rgb(85_212_231/65%)]"
                    aria-hidden="true"
                  />
                ) : null}
              </button>
            </search>
            <ProjectLinks hidden={Boolean(detail)} />
            <DetailsDialog
              detail={detail}
              items={items}
              layers={config.layers}
              returnFocus={returnFocus}
              fallbackFocus={searchToggleRef.current}
              onSelectItem={(item, focus) => {
                setSearch('')
                focusItem(item, returnFocus?.isConnected ? returnFocus : searchToggleRef.current || focus)
              }}
              onSelectGuild={(guildId, focus) => {
                showGuild(guildId, returnFocus?.isConnected ? returnFocus : searchToggleRef.current || focus)
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
