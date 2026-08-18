import { IconCheck, IconCrosshair, IconMinus, IconPlus } from '@tabler/icons-react'
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState
} from 'react'
import {
  buildSpatialGrid,
  clampView,
  coverScale,
  formatGameCoordinates,
  itemSearchText,
  MAX_ZOOM_RATIO,
  type MapTilePyramid,
  type MapTileSelection,
  markerStackOrder,
  markerText,
  type Point,
  querySpatialGrid,
  sceneSize,
  sceneViewportBounds,
  selectVisibleMapTiles,
  toScene,
  toWorld,
  type View
} from '../lib/map'
import { loadZoomRatio, saveZoomRatio } from '../lib/preferences'
import { completionSource, completionSourceLabel } from '../lib/saveProgress'
import type { ItemKind, MapCameraPosition, MapItem, MapLayer, PlayerStatus } from '../types'
import { MarkerGlyph } from './MarkerGlyph'

export interface MapViewportHandle {
  focusItem: (item: MapItem, returnFocus: HTMLElement) => void
  focusPosition: (position: MapCameraPosition) => void
  getZoomRatio: () => number
  clearSelection: () => void
}

interface MapViewportProps {
  activeLayer: MapLayer
  items: MapItem[]
  manualCompletedIds?: ReadonlySet<string>
  saveCompletedIds?: ReadonlySet<string>
  enabledKinds: Set<ItemKind>
  enabledPlayerStatuses: Set<PlayerStatus>
  hiddenIds: Set<string>
  search: string
  onShowItem: (item: MapItem, returnFocus: HTMLElement) => void
  inspectorOpen: boolean
  children: React.ReactNode
}

interface Drag {
  pointer: number
  x: number
  y: number
  viewX: number
  viewY: number
}

interface RenderViewport {
  view: View
  width: number
  height: number
}

interface RenderMarker {
  key: string
  position: Point
  item?: MapItem
  count?: number
}

interface ProjectedMarker {
  item: MapItem
  position: Point
}

interface MarkerBucket {
  first: MapItem
  count: number
  x: number
  y: number
}

interface ImageResult {
  url: string
  state: 'ready' | 'error'
  background?: string
}

interface TileTransition {
  layerId: string
  current: MapTileSelection | null
  previous: MapTileSelection | null
}

const MAX_RENDERED_MARKERS = 300
const TARGET_CLUSTER_MARKERS = 250
const CLUSTER_SIZE_PX = 52
const MIN_CLUSTER_GROWTH = 1.15
const CONTROL_ZOOM_DURATION_MS = 220
const ITEM_FOCUS_DURATION_MS = 420
const RESIZE_RENDER_SYNC_DELAY_MS = 120
const MAP_FIT_PADDING_PX = 64
const ZOOM_SAVE_DELAY_MS = 120
const SELECTED_MARKER_STACK = 120
const EMPTY_COMPLETION_IDS: ReadonlySet<string> = new Set()

function mapTilePyramid(layer: MapLayer): MapTilePyramid | null {
  const candidate = (layer as MapLayer & { tilePyramid?: Partial<MapTilePyramid> }).tilePyramid
  if (
    !candidate ||
    !Number.isFinite(candidate.tileSize) ||
    !candidate.tileSize ||
    !Array.isArray(candidate.levels) ||
    !candidate.levels.length ||
    typeof candidate.urlTemplate !== 'string' ||
    !candidate.urlTemplate.includes('{size}') ||
    !candidate.urlTemplate.includes('{x}') ||
    !candidate.urlTemplate.includes('{y}')
  )
    return null
  return candidate as MapTilePyramid
}

function TileArtwork({
  selection,
  onReady,
  onError,
  onSample
}: {
  selection: MapTileSelection
  onReady: (signature: string) => void
  onError: (signature: string) => void
  onSample: (image: HTMLImageElement) => void
}) {
  const imagesRef = useRef(new Map<string, HTMLImageElement>())
  const [loaded, setLoaded] = useState<Set<string>>(() => new Set())
  const ready = selection.tiles.length > 0 && loaded.size === selection.tiles.length

  const markLoaded = useCallback((key: string) => {
    setLoaded((current) => {
      if (current.has(key)) return current
      const next = new Set(current)
      next.add(key)
      return next
    })
  }, [])

  useLayoutEffect(() => {
    const cached = new Set<string>()
    for (const tile of selection.tiles) {
      const image = imagesRef.current.get(tile.key)
      if (image?.complete && image.naturalWidth > 0) cached.add(tile.key)
    }
    if (cached.size) setLoaded(cached)
  }, [selection.tiles])

  useEffect(() => {
    if (ready) onReady(selection.signature)
  }, [onReady, ready, selection.signature])

  return (
    <div
      className={`map-tile-layer absolute inset-0 ${ready ? 'is-ready' : ''}`}
      data-map-tile-level={selection.level}
      aria-hidden="true"
    >
      {selection.tiles.map((tile, index) => (
        <img
          key={tile.key}
          ref={(image) => {
            if (image) imagesRef.current.set(tile.key, image)
            else imagesRef.current.delete(tile.key)
          }}
          className="map-tile absolute select-none"
          src={tile.url}
          alt=""
          width={tile.pixelSize}
          height={tile.pixelSize}
          decoding="async"
          draggable={false}
          style={{ left: tile.x, top: tile.y, width: tile.size, height: tile.size }}
          onLoad={(event) => {
            markLoaded(tile.key)
            if (index === 0) onSample(event.currentTarget)
          }}
          onError={() => onError(selection.signature)}
        />
      ))}
    </div>
  )
}

function fitScale(width: number, height: number, size: number): number {
  const availableWidth = Math.max(1, width - MAP_FIT_PADDING_PX * 2)
  const availableHeight = Math.max(1, height - MAP_FIT_PADDING_PX * 2)
  return Math.max(0.001, Math.min(availableWidth / size, availableHeight / size))
}

function fitView(width: number, height: number, size: number): View {
  const scale = fitScale(width, height, size)
  return { scale, x: (width - size * scale) / 2, y: (height - size * scale) / 2 }
}

function bucketMarkers(markers: ProjectedMarker[], cellSize: number): Map<string, MarkerBucket> {
  const buckets = new Map<string, MarkerBucket>()
  for (const { item, position } of markers) {
    const key = `${Math.floor(position.x / cellSize)}:${Math.floor(position.y / cellSize)}`
    const bucket = buckets.get(key)
    if (bucket) {
      bucket.count++
      bucket.x += position.x
      bucket.y += position.y
    } else {
      buckets.set(key, { first: item, count: 1, x: position.x, y: position.y })
    }
  }
  return buckets
}

function sampleImageBackground(
  image: HTMLImageElement,
  adjustment: readonly [red: number, green: number, blue: number] = [0, 0, 0]
): string | undefined {
  try {
    const canvas = document.createElement('canvas')
    canvas.width = 1
    canvas.height = 1
    const context = canvas.getContext('2d', { colorSpace: 'srgb', willReadFrequently: true })
    if (!context) return undefined
    context.drawImage(image, 0, 0, 1, 1, 0, 0, 1, 1)
    const sampled = context.getImageData(0, 0, 1, 1).data
    const [red, green, blue] = adjustment.map((offset, channel) =>
      Math.min(255, Math.max(0, sampled[channel] + offset))
    )
    return `rgb(${red} ${green} ${blue})`
  } catch {
    return undefined
  }
}

export const MapViewport = forwardRef<MapViewportHandle, MapViewportProps>(function MapViewport(
  {
    activeLayer,
    items,
    manualCompletedIds = EMPTY_COMPLETION_IDS,
    saveCompletedIds = EMPTY_COMPLETION_IDS,
    enabledKinds,
    enabledPlayerStatuses,
    hiddenIds,
    search,
    onShowItem,
    inspectorOpen,
    children
  },
  ref
) {
  const viewportRef = useRef<HTMLElement>(null)
  const sceneRef = useRef<HTMLDivElement>(null)
  const coordinatesRef = useRef<HTMLSpanElement>(null)
  const size = useMemo(() => sceneSize(), [])
  const initialViewport = useMemo<RenderViewport>(() => {
    const width = Math.max(1, window.innerWidth)
    const height = Math.max(1, window.innerHeight)
    return { view: fitView(width, height, size), width, height }
  }, [size])
  const viewRef = useRef<View>(initialViewport.view)
  const viewportSizeRef = useRef<{ width: number; height: number } | null>(null)
  const dragRef = useRef<Drag | null>(null)
  const animationFrameRef = useRef<number | null>(null)
  const resizeSyncTimeoutRef = useRef<number | null>(null)
  const zoomSaveTimeoutRef = useRef<number | null>(null)
  const pendingZoomRef = useRef<{ layerId: string; ratio: number } | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [renderViewport, setRenderViewport] = useState(initialViewport)
  const [imageResult, setImageResult] = useState<ImageResult | null>(null)
  const [readyTileSignature, setReadyTileSignature] = useState<string | null>(null)
  const readyTileSignatureRef = useRef<string | null>(null)
  const [errorTileSignature, setErrorTileSignature] = useState<string | null>(null)
  const [tileBackground, setTileBackground] = useState<{ layerId: string; value: string } | null>(null)
  const imageUrl = activeLayer.imageUrl
  const tilePyramid = useMemo(() => mapTilePyramid(activeLayer), [activeLayer])
  const requestedTiles = useMemo(
    () =>
      tilePyramid
        ? selectVisibleMapTiles(tilePyramid, renderViewport.view, renderViewport.width, renderViewport.height, size)
        : null,
    [renderViewport, size, tilePyramid]
  )
  const [tileTransition, setTileTransition] = useState<TileTransition>(() => ({
    layerId: activeLayer.id,
    current: requestedTiles,
    previous: null
  }))

  useLayoutEffect(() => {
    setTileTransition((current) => {
      if (current.layerId === activeLayer.id && current.current?.signature === requestedTiles?.signature) return current
      return {
        layerId: activeLayer.id,
        current: requestedTiles,
        previous:
          current.layerId === activeLayer.id
            ? current.current?.signature === readyTileSignatureRef.current
              ? current.current
              : current.previous
            : null
      }
    })
  }, [activeLayer.id, requestedTiles])

  const currentTileSignature = tileTransition.current?.signature
  const currentTilesFailed = Boolean(tilePyramid && errorTileSignature === currentTileSignature)
  const fallbackImageState = !imageUrl ? 'error' : imageResult?.url === imageUrl ? imageResult.state : 'loading'
  const imageState = tilePyramid
    ? currentTilesFailed
      ? fallbackImageState
      : readyTileSignature === currentTileSignature
        ? 'ready'
        : 'loading'
    : !imageUrl
      ? 'error'
      : imageResult?.url === imageUrl
        ? imageResult.state
        : 'loading'
  const imageBackground = tilePyramid
    ? tileBackground?.layerId === activeLayer.id
      ? tileBackground.value
      : undefined
    : imageUrl && imageResult?.url === imageUrl
      ? imageResult.background
      : undefined

  const current = useMemo(() => {
    const layerItems: MapItem[] = []
    const baseNames = new Map<string, string>()
    const companionSearchTextByOwnerId = new Map<string, string[]>()
    for (const item of items) {
      if (item.kind === 'companions' && item.ownerId) {
        const ownerTerms = companionSearchTextByOwnerId.get(item.ownerId) || []
        ownerTerms.push(itemSearchText(item))
        companionSearchTextByOwnerId.set(item.ownerId, ownerTerms)
      }
      if (item.map !== activeLayer.id) continue
      layerItems.push(item)
      if (item.kind === 'bases') {
        baseNames.set(item.id, item.name)
        if (item.baseId) baseNames.set(item.baseId, item.name)
      }
    }
    return { items: layerItems, baseNames, companionSearchTextByOwnerId }
  }, [activeLayer.id, items])
  const visibleItems = useMemo(() => {
    const query = search.trim().toLowerCase()
    return current.items.filter((item) => {
      if (item.kind === 'companions') return false
      if (!enabledKinds.has(item.kind) || hiddenIds.has(item.id)) return false
      if (item.kind === 'players' && !enabledPlayerStatuses.has(item.online === false ? 'offline' : 'online'))
        return false
      if (!query) return true
      const relatedText =
        item.kind === 'workers' && item.baseId
          ? current.baseNames.get(item.baseId) || ''
          : item.kind === 'players'
            ? (current.companionSearchTextByOwnerId.get(item.id) || []).join(' ')
            : ''
      return itemSearchText(item, relatedText).includes(query)
    })
  }, [current, enabledKinds, enabledPlayerStatuses, hiddenIds, search])

  const projectedItems = useMemo(() => {
    const entries = visibleItems.flatMap((item) => {
      const position = toScene(item, activeLayer, size)
      return position ? [{ value: item, position }] : []
    })
    return {
      grid: buildSpatialGrid(entries),
      byId: new Map(entries.map((entry) => [entry.value.id, entry]))
    }
  }, [activeLayer, size, visibleItems])
  const renderMarkers = useMemo<RenderMarker[]>(() => {
    const bounds = sceneViewportBounds(
      renderViewport.view,
      renderViewport.width,
      renderViewport.height,
      CLUSTER_SIZE_PX * 2
    )
    const projected = querySpatialGrid(projectedItems.grid, bounds).map(({ value: item, position }) => ({
      item,
      position
    }))
    const selectedEntry = selectedId ? projectedItems.byId.get(selectedId) : undefined
    if (selectedEntry && !projected.some(({ item }) => item.id === selectedId))
      projected.push({ item: selectedEntry.value, position: selectedEntry.position })

    if (projected.length <= MAX_RENDERED_MARKERS) {
      return projected.map(({ item, position }) => ({ key: item.id, item, position }))
    }

    const selected: RenderMarker[] = []
    const clusterCandidates: ProjectedMarker[] = []
    for (const marker of projected) {
      const { item, position } = marker
      if (item.id === selectedId) {
        selected.push({ key: item.id, item, position })
        continue
      }
      clusterCandidates.push(marker)
    }

    const target = Math.max(1, TARGET_CLUSTER_MARKERS - selected.length)
    let cellSizePx = CLUSTER_SIZE_PX
    let buckets = bucketMarkers(clusterCandidates, cellSizePx / renderViewport.view.scale)
    while (buckets.size > target) {
      const growth = Math.max(MIN_CLUSTER_GROWTH, Math.sqrt(buckets.size / target))
      cellSizePx = Math.ceil(cellSizePx * growth)
      buckets = bucketMarkers(clusterCandidates, cellSizePx / renderViewport.view.scale)
    }

    const clustered = Array.from(buckets, ([key, bucket]): RenderMarker => {
      if (bucket.count === 1) {
        return { key: bucket.first.id, item: bucket.first, position: { x: bucket.x, y: bucket.y } }
      }
      return {
        key: `cluster:${key}`,
        count: bucket.count,
        position: { x: bucket.x / bucket.count, y: bucket.y / bucket.count }
      }
    })
    return [...selected, ...clustered]
  }, [projectedItems, renderViewport, selectedId])

  const syncRenderViewport = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    setRenderViewport({
      view: { ...viewRef.current },
      width: viewport.clientWidth,
      height: viewport.clientHeight
    })
  }, [])

  const cancelResizeRenderSync = useCallback(() => {
    if (resizeSyncTimeoutRef.current !== null) window.clearTimeout(resizeSyncTimeoutRef.current)
    resizeSyncTimeoutRef.current = null
  }, [])

  const scheduleResizeRenderSync = useCallback(() => {
    cancelResizeRenderSync()
    resizeSyncTimeoutRef.current = window.setTimeout(() => {
      resizeSyncTimeoutRef.current = null
      syncRenderViewport()
    }, RESIZE_RENDER_SYNC_DELAY_MS)
  }, [cancelResizeRenderSync, syncRenderViewport])

  const syncRenderViewportDuringPan = useCallback(() => {
    const rendered = renderViewport.view
    const current = viewRef.current
    if (Math.abs(current.x - rendered.x) >= CLUSTER_SIZE_PX || Math.abs(current.y - rendered.y) >= CLUSTER_SIZE_PX)
      syncRenderViewport()
  }, [renderViewport.view, syncRenderViewport])

  const flushZoomPreference = useCallback(() => {
    if (zoomSaveTimeoutRef.current !== null) window.clearTimeout(zoomSaveTimeoutRef.current)
    zoomSaveTimeoutRef.current = null
    const pending = pendingZoomRef.current
    pendingZoomRef.current = null
    if (pending) saveZoomRatio(pending.layerId, pending.ratio)
  }, [])

  const queueZoomPreference = useCallback(
    (ratio: number) => {
      pendingZoomRef.current = { layerId: activeLayer.id, ratio }
      if (zoomSaveTimeoutRef.current !== null) window.clearTimeout(zoomSaveTimeoutRef.current)
      zoomSaveTimeoutRef.current = window.setTimeout(flushZoomPreference, ZOOM_SAVE_DELAY_MS)
    },
    [activeLayer.id, flushZoomPreference]
  )

  const applyView = useCallback(
    (view: View) => {
      viewRef.current = view
      const scene = sceneRef.current
      const viewport = viewportRef.current
      if (!scene || !viewport) return
      scene.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.scale})`
      const minimum = coverScale(viewport.clientWidth, viewport.clientHeight, size)
      const zoomRatio = Math.max(1, view.scale / minimum)
      scene.style.setProperty('--marker-scale', String(Math.min(2, Math.sqrt(zoomRatio)) / view.scale))
    },
    [size]
  )

  const cancelViewAnimation = useCallback(() => {
    if (animationFrameRef.current !== null) window.cancelAnimationFrame(animationFrameRef.current)
    animationFrameRef.current = null
  }, [])

  const animateView = useCallback(
    (target: View, durationMs = CONTROL_ZOOM_DURATION_MS) => {
      cancelViewAnimation()
      cancelResizeRenderSync()
      if (typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        applyView(target)
        syncRenderViewport()
        return
      }

      const start = { ...viewRef.current }
      const startedAt = window.performance.now()
      const step = (now: number) => {
        const progress = Math.min(1, Math.max(0, (now - startedAt) / durationMs))
        const eased = 1 - (1 - progress) ** 3
        applyView({
          scale: start.scale + (target.scale - start.scale) * eased,
          x: start.x + (target.x - start.x) * eased,
          y: start.y + (target.y - start.y) * eased
        })
        if (progress < 1) {
          animationFrameRef.current = window.requestAnimationFrame(step)
        } else {
          animationFrameRef.current = null
          syncRenderViewport()
        }
      }
      animationFrameRef.current = window.requestAnimationFrame(step)
    },
    [applyView, cancelResizeRenderSync, cancelViewAnimation, syncRenderViewport]
  )

  const reset = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    cancelViewAnimation()
    cancelResizeRenderSync()
    viewportSizeRef.current = { width: viewport.clientWidth, height: viewport.clientHeight }
    const fitted = fitView(viewport.clientWidth, viewport.clientHeight, size)
    const maximum = coverScale(viewport.clientWidth, viewport.clientHeight, size) * MAX_ZOOM_RATIO
    const scale = Math.min(maximum, fitted.scale * loadZoomRatio(activeLayer.id))
    applyView({ scale, x: (viewport.clientWidth - size * scale) / 2, y: (viewport.clientHeight - size * scale) / 2 })
    syncRenderViewport()
  }, [activeLayer.id, applyView, cancelResizeRenderSync, cancelViewAnimation, size, syncRenderViewport])

  const animateFit = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    viewportSizeRef.current = { width: viewport.clientWidth, height: viewport.clientHeight }
    animateView(fitView(viewport.clientWidth, viewport.clientHeight, size))
    queueZoomPreference(1)
  }, [animateView, queueZoomPreference, size])

  const resizeView = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    const width = viewport.clientWidth
    const height = viewport.clientHeight
    const previous = viewportSizeRef.current
    viewportSizeRef.current = { width, height }
    cancelViewAnimation()
    if (!previous) {
      cancelResizeRenderSync()
      applyView(fitView(width, height, size))
      syncRenderViewport()
      return
    }
    if (previous.width === width && previous.height === height) return

    const currentView = viewRef.current
    const sceneX = (previous.width / 2 - currentView.x) / currentView.scale
    const sceneY = (previous.height / 2 - currentView.y) / currentView.scale
    const previousMinimum = fitScale(previous.width, previous.height, size)
    const minimum = fitScale(width, height, size)
    const maximum = coverScale(width, height, size) * MAX_ZOOM_RATIO
    const scale = Math.min(maximum, Math.max(minimum, minimum * (currentView.scale / previousMinimum)))
    applyView(clampView({ scale, x: width / 2 - sceneX * scale, y: height / 2 - sceneY * scale }, width, height, size))
    scheduleResizeRenderSync()
  }, [applyView, cancelResizeRenderSync, cancelViewAnimation, scheduleResizeRenderSync, size, syncRenderViewport])

  const zoomAt = useCallback(
    (nextScale: number, clientX: number, clientY: number, animated = false) => {
      const viewport = viewportRef.current
      if (!viewport) return
      const rect = viewport.getBoundingClientRect()
      const minimum = fitScale(rect.width, rect.height, size)
      const maximum = coverScale(rect.width, rect.height, size) * MAX_ZOOM_RATIO
      const scale = Math.min(maximum, Math.max(minimum, nextScale))
      const pointerX = clientX - rect.left
      const pointerY = clientY - rect.top
      const current = viewRef.current
      const sceneX = (pointerX - current.x) / current.scale
      const sceneY = (pointerY - current.y) / current.scale
      const target = clampView(
        {
          scale,
          x: pointerX - sceneX * scale,
          y: pointerY - sceneY * scale
        },
        rect.width,
        rect.height,
        size
      )
      queueZoomPreference(Math.max(1, scale / fitScale(rect.width, rect.height, size)))
      if (animated) animateView(target)
      else {
        cancelViewAnimation()
        cancelResizeRenderSync()
        applyView(target)
        syncRenderViewport()
      }
    },
    [animateView, applyView, cancelResizeRenderSync, cancelViewAnimation, queueZoomPreference, size, syncRenderViewport]
  )

  const focusItem = (item: MapItem, returnFocus: HTMLElement) => {
    const viewport = viewportRef.current
    const position = toScene(item, activeLayer, size)
    if (!viewport || !position) return
    const rect = viewport.getBoundingClientRect()
    const minimum = coverScale(rect.width, rect.height, size)
    const scale = Math.min(
      minimum * MAX_ZOOM_RATIO,
      Math.max(viewRef.current.scale, minimum * (item.kind === 'workers' ? 24 : 8))
    )
    const target = clampView(
      { scale, x: rect.width / 2 - position.x * scale, y: rect.height / 2 - position.y * scale },
      rect.width,
      rect.height,
      size
    )
    queueZoomPreference(Math.max(1, scale / fitScale(rect.width, rect.height, size)))
    setSelectedId(item.id)
    animateView(target, ITEM_FOCUS_DURATION_MS)
    onShowItem(item, returnFocus)
  }

  const getZoomRatio = () => {
    const rect = viewportRef.current?.getBoundingClientRect()
    if (!rect?.width || !rect.height) return 1
    return Math.max(1, viewRef.current.scale / fitScale(rect.width, rect.height, size))
  }

  const focusPosition = (position: MapCameraPosition) => {
    const viewport = viewportRef.current
    if (!viewport || position.region !== activeLayer.id) return
    const scenePosition = toScene(position, activeLayer, size)
    if (!scenePosition) return
    const rect = viewport.getBoundingClientRect()
    const minimum = fitScale(rect.width, rect.height, size)
    const maximum = coverScale(rect.width, rect.height, size) * MAX_ZOOM_RATIO
    const scale = Math.min(maximum, Math.max(minimum, minimum * position.zoom))
    cancelViewAnimation()
    cancelResizeRenderSync()
    setSelectedId(null)
    applyView(
      clampView(
        {
          scale,
          x: rect.width / 2 - scenePosition.x * scale,
          y: rect.height / 2 - scenePosition.y * scale
        },
        rect.width,
        rect.height,
        size
      )
    )
    syncRenderViewport()
  }

  useImperativeHandle(ref, () => ({
    focusItem,
    focusPosition,
    getZoomRatio,
    clearSelection: () => setSelectedId(null)
  }))

  // biome-ignore lint/correctness/useExhaustiveDependencies: changing maps must reset the selection and fitted view
  useEffect(() => {
    setSelectedId(null)
    reset()
  }, [activeLayer, reset])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const observer = new ResizeObserver(resizeView)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [resizeView])

  useEffect(
    () => () => {
      cancelViewAnimation()
      cancelResizeRenderSync()
      flushZoomPreference()
    },
    [cancelResizeRenderSync, cancelViewAnimation, flushZoomPreference]
  )

  // biome-ignore lint/correctness/useExhaustiveDependencies: changing maps must flush that map's pending zoom value
  useEffect(() => () => flushZoomPreference(), [activeLayer.id, flushZoomPreference])

  useEffect(() => {
    if (selectedId && !current.items.some((item) => item.id === selectedId)) setSelectedId(null)
  }, [current.items, selectedId])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const handleWheel = (event: WheelEvent) => {
      if (
        event.target instanceof Element &&
        event.target.closest('button, input, textarea, select, aside, search, [role="search"], [role="dialog"]')
      )
        return
      event.preventDefault()
      zoomAt(viewRef.current.scale * (event.deltaY < 0 ? 1.16 : 0.86), event.clientX, event.clientY)
    }
    viewport.addEventListener('wheel', handleWheel, { passive: false })
    return () => viewport.removeEventListener('wheel', handleWheel)
  }, [zoomAt])

  const center = () => {
    const rect = viewportRef.current?.getBoundingClientRect()
    return rect ? { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 } : { x: 0, y: 0 }
  }

  return (
    <section
      ref={viewportRef}
      className={`map-viewport map-layer-${activeLayer.id} relative size-full touch-pinch-zoom overflow-hidden active:cursor-grabbing`}
      role="application"
      aria-label={`${activeLayer.name} interactive world map. Use arrow keys to pan and plus or minus to zoom.`}
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the map is an interactive pan and zoom canvas
      tabIndex={0}
      style={
        {
          cursor: dragRef.current ? 'grabbing' : 'grab',
          ...(imageBackground ? { '--map-background': imageBackground } : {})
        } as React.CSSProperties
      }
      onPointerDown={(event) => {
        if (event.button !== 0 || (event.target as Element).closest('button, input, aside, search, [role="search"]'))
          return
        cancelViewAnimation()
        cancelResizeRenderSync()
        const current = viewRef.current
        dragRef.current = {
          pointer: event.pointerId,
          x: event.clientX,
          y: event.clientY,
          viewX: current.x,
          viewY: current.y
        }
        event.currentTarget.setPointerCapture(event.pointerId)
        event.currentTarget.style.cursor = 'grabbing'
      }}
      onPointerMove={(event) => {
        if (
          event.target instanceof Element &&
          event.target.closest(
            'button:not(.map-marker), input, textarea, select, aside, search, [role="search"], [role="dialog"]'
          )
        )
          return
        const rect = event.currentTarget.getBoundingClientRect()
        const current = viewRef.current
        const world = toWorld(
          {
            x: (event.clientX - rect.left - current.x) / current.scale,
            y: (event.clientY - rect.top - current.y) / current.scale
          },
          activeLayer,
          size
        )
        if (coordinatesRef.current) coordinatesRef.current.textContent = formatGameCoordinates(world)

        const drag = dragRef.current
        if (!drag || drag.pointer !== event.pointerId) return
        applyView(
          clampView(
            { scale: current.scale, x: drag.viewX + event.clientX - drag.x, y: drag.viewY + event.clientY - drag.y },
            rect.width,
            rect.height,
            size
          )
        )
        syncRenderViewportDuringPan()
      }}
      onPointerUp={(event) => {
        if (dragRef.current?.pointer !== event.pointerId) return
        dragRef.current = null
        event.currentTarget.style.cursor = 'grab'
        syncRenderViewport()
      }}
      onPointerCancel={(event) => {
        if (dragRef.current?.pointer !== event.pointerId) return
        dragRef.current = null
        event.currentTarget.style.cursor = 'grab'
        syncRenderViewport()
      }}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return
        const rect = event.currentTarget.getBoundingClientRect()
        const current = viewRef.current
        const pan = 56
        if (event.key === '+' || event.key === '=') {
          event.preventDefault()
          const point = center()
          zoomAt(current.scale * 1.25, point.x, point.y, true)
        } else if (event.key === '-') {
          event.preventDefault()
          const point = center()
          zoomAt(current.scale / 1.25, point.x, point.y, true)
        } else if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) {
          event.preventDefault()
          cancelViewAnimation()
          cancelResizeRenderSync()
          applyView(
            clampView(
              {
                ...current,
                x: current.x + (event.key === 'ArrowLeft' ? pan : event.key === 'ArrowRight' ? -pan : 0),
                y: current.y + (event.key === 'ArrowUp' ? pan : event.key === 'ArrowDown' ? -pan : 0)
              },
              rect.width,
              rect.height,
              size
            )
          )
          syncRenderViewport()
        }
      }}
    >
      <div
        ref={sceneRef}
        className="map-scene"
        style={
          {
            width: size,
            height: size,
            '--marker-scale': '1'
          } as React.CSSProperties
        }
      >
        {imageState !== 'ready' && <div className="fallback-grid absolute inset-0 size-full" aria-hidden="true" />}
        {tilePyramid
          ? [tileTransition.previous, tileTransition.current]
              .filter((selection): selection is MapTileSelection => Boolean(selection))
              .map((selection) => (
                <TileArtwork
                  key={selection.signature}
                  selection={selection}
                  onReady={(signature) => {
                    readyTileSignatureRef.current = signature
                    setReadyTileSignature(signature)
                    setTileTransition((current) =>
                      current.current?.signature === signature && current.previous
                        ? { ...current, previous: null }
                        : current
                    )
                  }}
                  onError={(signature) => {
                    setErrorTileSignature(signature)
                    setTileTransition((current) =>
                      current.current?.signature === signature ? { ...current, previous: null } : current
                    )
                  }}
                  onSample={(image) => {
                    const background = sampleImageBackground(
                      image,
                      activeLayer.id === 'palpagos' ? [-1, -1, 0] : undefined
                    )
                    if (background) setTileBackground({ layerId: activeLayer.id, value: background })
                  }}
                />
              ))
          : imageUrl && (
              <img
                className={`map-artwork absolute inset-0 size-full select-none object-fill ${
                  imageState === 'ready' ? 'block' : 'hidden'
                }`}
                src={imageUrl}
                alt=""
                draggable={false}
                onLoad={(event) =>
                  setImageResult({
                    url: imageUrl,
                    state: 'ready',
                    background: sampleImageBackground(
                      event.currentTarget,
                      activeLayer.id === 'palpagos' ? [-1, -1, 0] : undefined
                    )
                  })
                }
                onError={() => setImageResult({ url: imageUrl, state: 'error' })}
              />
            )}
        {tilePyramid && currentTilesFailed && imageUrl && (
          <img
            className={`map-artwork absolute inset-0 size-full select-none object-fill ${
              fallbackImageState === 'ready' ? 'block' : 'hidden'
            }`}
            src={imageUrl}
            alt=""
            draggable={false}
            onLoad={(event) =>
              setImageResult({
                url: imageUrl,
                state: 'ready',
                background: sampleImageBackground(
                  event.currentTarget,
                  activeLayer.id === 'palpagos' ? [-1, -1, 0] : undefined
                )
              })
            }
            onError={() => setImageResult({ url: imageUrl, state: 'error' })}
          />
        )}
        <div className="absolute inset-0">
          {renderMarkers.map(({ key, item, position, count }) => {
            if (!item) {
              return (
                <button
                  key={key}
                  type="button"
                  className="map-marker map-cluster"
                  style={{ left: position.x, top: position.y }}
                  aria-label={`Zoom to ${count} nearby map items`}
                  tabIndex={-1}
                  onPointerDown={(event) => event.stopPropagation()}
                  onClick={(event) => {
                    event.stopPropagation()
                    const viewport = viewportRef.current
                    if (!viewport) return
                    const rect = viewport.getBoundingClientRect()
                    const minimum = coverScale(rect.width, rect.height, size)
                    const scale = Math.min(minimum * MAX_ZOOM_RATIO, viewRef.current.scale * 2.5)
                    const target = clampView(
                      {
                        scale,
                        x: rect.width / 2 - position.x * scale,
                        y: rect.height / 2 - position.y * scale
                      },
                      rect.width,
                      rect.height,
                      size
                    )
                    queueZoomPreference(Math.max(1, scale / fitScale(rect.width, rect.height, size)))
                    animateView(target, ITEM_FOCUS_DURATION_MS)
                  }}
                >
                  <span>{count && count > 999 ? '999+' : count}</span>
                </button>
              )
            }
            const selected = selectedId === item.id
            const source = completionSource(item.id, manualCompletedIds, saveCompletedIds)
            const sourceLabel = completionSourceLabel(source)
            return (
              <button
                key={key}
                type="button"
                className={`map-marker ${selected ? 'selected' : ''}`}
                style={
                  {
                    left: position.x,
                    top: position.y,
                    '--marker-stack': selected ? SELECTED_MARKER_STACK : markerStackOrder(item.kind)
                  } as React.CSSProperties
                }
                aria-label={`${markerText(item)}${sourceLabel ? ` · ${sourceLabel} completion` : ''}`}
                data-completion-source={source || undefined}
                tabIndex={-1}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={(event) => {
                  event.stopPropagation()
                  setSelectedId(item.id)
                  onShowItem(item, event.currentTarget)
                }}
              >
                <MarkerGlyph kind={item.kind} online={item.online} />
                {sourceLabel ? (
                  <span
                    className={`pointer-events-none absolute right-1 bottom-1 grid size-3.5 place-items-center rounded-full border border-[#d9fff0] text-white shadow-[0_0_0_2px_rgb(8_18_24/70%)] ${source === 'save' ? 'bg-[#176083]' : source === 'combined' ? 'bg-[#4e7a2a]' : 'bg-[#176a4a]'}`}
                    aria-hidden="true"
                  >
                    <IconCheck className="size-2.5" stroke={2.5} />
                  </span>
                ) : null}
                <span className="marker-label">
                  {markerText(item)}
                  {sourceLabel ? ` · ${sourceLabel}` : ''}
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {imageState === 'error' && (
        <div className="pointer-events-none absolute top-[78px] left-1/2 -translate-x-1/2 rounded-md border border-[#665a3e] bg-[#302a20]/95 px-3 py-2 text-xs text-[#d5bd82] max-sm:top-[140px]">
          Map artwork is not installed.
        </div>
      )}

      {children}

      <div
        className={`pal-glass-surface absolute right-[18px] bottom-[18px] z-[18] flex h-11 overflow-hidden transition-[opacity,transform] max-sm:right-3 max-sm:bottom-3 ${
          inspectorOpen ? 'pointer-events-none translate-y-2 opacity-0' : ''
        }`}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div className="pointer-events-none flex w-[184px] shrink-0 items-center gap-2 border-r border-[#cdeef3]/35 px-3 text-[11px] tracking-[.055em] whitespace-nowrap text-[#cce8eb] tabular-nums max-sm:w-[150px] max-sm:px-2">
          <IconCrosshair
            className="size-3.5 shrink-0 text-[#67cad8] max-sm:hidden"
            stroke={1.5}
            aria-hidden="true"
            focusable="false"
          />
          <span ref={coordinatesRef}>X —&nbsp;&nbsp;Y —</span>
        </div>
        <fieldset
          className="m-0 flex h-full border-0 p-0"
          aria-label="Map controls"
          aria-hidden={inspectorOpen}
          inert={inspectorOpen}
        >
          <button
            type="button"
            className="pal-interactive grid h-full min-w-11 cursor-pointer place-items-center border-0 bg-transparent text-lg text-[#eefeff] focus-visible:outline-none"
            aria-label="Zoom out"
            onClick={() => {
              const point = center()
              zoomAt(viewRef.current.scale / 1.35, point.x, point.y, true)
            }}
          >
            <IconMinus className="size-5" aria-hidden="true" focusable="false" />
          </button>
          <button
            type="button"
            className="pal-interactive grid h-full min-w-[58px] cursor-pointer place-items-center border-x border-y-0 border-[#cdeef3]/35 bg-transparent text-[11px] font-bold tracking-[.06em] text-[#eefeff] uppercase focus-visible:outline-none"
            title="Fit the active region"
            onClick={animateFit}
          >
            Fit
          </button>
          <button
            type="button"
            className="pal-interactive grid h-full min-w-11 cursor-pointer place-items-center border-0 bg-transparent text-lg text-[#eefeff] focus-visible:outline-none"
            aria-label="Zoom in"
            onClick={() => {
              const point = center()
              zoomAt(viewRef.current.scale * 1.35, point.x, point.y, true)
            }}
          >
            <IconPlus className="size-5" aria-hidden="true" focusable="false" />
          </button>
        </fieldset>
      </div>
    </section>
  )
})
