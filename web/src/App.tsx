import { useEffect, useMemo, useRef, useState } from 'react'
import { type Detail, DetailsDialog } from './components/DetailsDialog'
import { Explorer } from './components/Explorer'
import { MapViewport, type MapViewportHandle } from './components/MapViewport'
import { StatusBar } from './components/StatusBar'
import { usePolling } from './hooks/usePolling'
import { itemKey } from './lib/map'
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

export function App() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [configError, setConfigError] = useState(false)

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
  }, [])

  if (!config) {
    return (
      <div className="grid h-dvh grid-rows-[64px_1fr] bg-[#171a1d] text-[#f4f5f5] max-sm:grid-rows-[52px_1fr]">
        <StatusBar demoMode={false} playerState={null} offline={configError} onShowDetails={() => {}} />
        <main className="grid place-items-center bg-[#111416] text-sm text-[#8f989d]">
          {configError ? 'Map unavailable' : 'Loading map…'}
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
  const [activeLayer, setActiveLayer] = useState<MapLayer>(config.layers[0])
  const [enabledKinds, setEnabledKinds] = useState(() => new Set<ItemKind>(ALL_KINDS))
  const [hiddenKeys, setHiddenKeys] = useState(() => new Set<string>())
  const [expandedGuilds, setExpandedGuilds] = useState(() => new Set<string>())
  const [expandedBases, setExpandedBases] = useState(() => new Set<string>())
  const [search, setSearch] = useState('')
  const [filtersOpen, setFiltersOpen] = useState(() => typeof window === 'undefined' || window.innerWidth > 640)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [returnFocus, setReturnFocus] = useState<HTMLElement | null>(null)
  const mapRef = useRef<MapViewportHandle>(null)
  const searchRef = useRef<HTMLInputElement>(null)

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

  useEffect(() => {
    document.title = playerState?.server.name || 'Palworld Live Map'
  }, [playerState?.server.name])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      if ((event.target as HTMLElement).matches('input, textarea, select')) return
      event.preventDefault()
      searchRef.current?.focus()
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  const showItem = (item: MapItem, focus: HTMLElement) => {
    setReturnFocus(focus)
    setDetail({ kind: 'item', item })
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
    setHiddenKeys((current) => {
      const next = new Set(current)
      next.delete(itemKey(item))
      return next
    })
    mapRef.current?.focusItem(item, focus)
  }

  const toggleKinds = (kinds: ItemKind[], visible: boolean) => {
    setEnabledKinds((current) => {
      const next = new Set(current)
      for (const kind of kinds) visible ? next.add(kind) : next.delete(kind)
      return next
    })
    if (visible) {
      setHiddenKeys((current) => {
        const next = new Set(current)
        for (const item of items) {
          if (item.map === activeLayer.id && kinds.includes(item.kind)) next.delete(itemKey(item))
        }
        return next
      })
    }
  }

  const toggleItems = (keys: string[], visible: boolean) => {
    setHiddenKeys((current) => {
      const next = new Set(current)
      for (const key of keys) visible ? next.delete(key) : next.add(key)
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
  if (!config.worldDataEnabled || !objectState.enabled)
    objectNotice = 'Extra live layers are disabled by this map’s configuration.'
  else if (objectState.unsupported)
    objectNotice = 'Extra live layers need ENABLE_GAMEDATA_API=true and a Palworld server restart.'
  else if (objectState.stale) objectNotice = 'World objects are using the last successful snapshot.'
  else if (objects.error)
    objectNotice = objectState.available
      ? 'World object refresh failed; showing the last received snapshot.'
      : 'World objects are temporarily unavailable.'
  else if (!objectState.available) objectNotice = 'Loading bases, Pals and NPCs…'

  const explorerProps = {
    activeLayer,
    layers: config.layers,
    items,
    search,
    enabledKinds,
    hiddenKeys,
    expandedGuilds,
    expandedBases,
    objectNotice,
    onSearch: setSearch,
    onToggleKinds: toggleKinds,
    onToggleItems: toggleItems,
    onToggleGuild: (id: string) => toggleSetValue(setExpandedGuilds, id),
    onToggleBase: (id: string) => toggleSetValue(setExpandedBases, id),
    onFocusItem: focusItem,
    onClose: () => setFiltersOpen(false),
    onLayerChange: (layer: MapLayer) => {
      setActiveLayer(layer)
    }
  }

  return (
    <div className="grid h-dvh grid-rows-[64px_1fr] overflow-hidden bg-[#171a1d] text-[#f4f5f5] max-sm:grid-rows-[52px_1fr]">
      <StatusBar
        demoMode={config.demoMode}
        playerState={playerState}
        offline={Boolean(players.error && !playerState)}
        onShowDetails={(focus) => {
          setReturnFocus(focus)
          setDetail({ kind: 'server' })
        }}
      />
      <main className="relative min-h-0 size-full">
        <MapViewport
          ref={mapRef}
          activeLayer={activeLayer}
          items={items}
          enabledKinds={enabledKinds}
          hiddenKeys={hiddenKeys}
          search={search}
          onShowItem={showItem}
          inspectorOpen={Boolean(detail)}
        >
          <Explorer {...explorerProps} open={filtersOpen} onOpen={() => setFiltersOpen(true)} />
          <label className={`command-search ${filtersOpen ? 'drawer-open' : ''} ${detail ? 'inspector-open' : ''}`}>
            <span className="command-search-icon" aria-hidden="true" />
            <input
              ref={searchRef}
              type="search"
              placeholder="Search players, bases and Pals…"
              autoComplete="off"
              value={search}
              onChange={(event) => setSearch(event.currentTarget.value)}
            />
            {search ? (
              <button type="button" aria-label="Clear search" onClick={() => setSearch('')}>
                ×
              </button>
            ) : (
              <kbd>/</kbd>
            )}
          </label>
          <DetailsDialog
            detail={detail}
            items={items}
            layers={config.layers}
            playerState={playerState}
            returnFocus={returnFocus}
            onClose={() => {
              setDetail(null)
              mapRef.current?.clearSelection()
            }}
          />
        </MapViewport>
      </main>
    </div>
  )
}
