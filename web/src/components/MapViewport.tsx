import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import {
  clampView,
  coverScale,
  isScenePointVisible,
  MAX_ZOOM_RATIO,
  markerText,
  type Point,
  sceneSize,
  sceneViewportBounds,
  toScene,
  toWorld,
  type View
} from '../lib/map'
import type { ItemKind, MapItem, MapLayer } from '../types'

export interface MapViewportHandle {
  focusItem: (item: MapItem, returnFocus: HTMLElement) => void
  clearSelection: () => void
}

interface MapViewportProps {
  activeLayer: MapLayer
  items: MapItem[]
  enabledKinds: Set<ItemKind>
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

const MAX_INDIVIDUAL_MARKERS = 900
const CLUSTER_SIZE_PX = 52
const CONTROL_ZOOM_DURATION_MS = 220
const RESIZE_RENDER_SYNC_DELAY_MS = 120

function fitScale(width: number, height: number, size: number): number {
  return Math.max(0.01, Math.min(width / size, height / size))
}

function fitView(width: number, height: number, size: number): View {
  const scale = fitScale(width, height, size)
  return { scale, x: (width - size * scale) / 2, y: (height - size * scale) / 2 }
}

export const MapViewport = forwardRef<MapViewportHandle, MapViewportProps>(function MapViewport(
  { activeLayer, items, enabledKinds, hiddenIds, search, onShowItem, inspectorOpen, children },
  ref
) {
  const viewportRef = useRef<HTMLElement>(null)
  const sceneRef = useRef<HTMLDivElement>(null)
  const coordinatesRef = useRef<HTMLDivElement>(null)
  const size = useMemo(() => sceneSize(), [])
  const initialViewport = useMemo<RenderViewport>(() => {
    const compactHeader = window.innerWidth < 768
    const width = Math.max(1, window.innerWidth)
    const height = Math.max(1, window.innerHeight - (compactHeader ? 84 : 64))
    return { view: fitView(width, height, size), width, height }
  }, [size])
  const viewRef = useRef<View>(initialViewport.view)
  const viewportSizeRef = useRef<{ width: number; height: number } | null>(null)
  const dragRef = useRef<Drag | null>(null)
  const animationFrameRef = useRef<number | null>(null)
  const resizeSyncTimeoutRef = useRef<number | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [renderViewport, setRenderViewport] = useState(initialViewport)
  const [imageState, setImageState] = useState<'loading' | 'ready' | 'error'>(
    activeLayer.imageUrl ? 'loading' : 'error'
  )

  const current = useMemo(() => {
    const layerItems: MapItem[] = []
    const baseNames = new Map<string, string>()
    for (const item of items) {
      if (item.map !== activeLayer.id) continue
      layerItems.push(item)
      if (item.kind === 'bases') {
        baseNames.set(item.id, item.name)
        if (item.baseId) baseNames.set(item.baseId, item.name)
      }
    }
    return { items: layerItems, baseNames }
  }, [activeLayer.id, items])
  const visibleItems = useMemo(() => {
    const query = search.trim().toLowerCase()
    return current.items.filter((item) => {
      if (!enabledKinds.has(item.kind) || hiddenIds.has(item.id)) return false
      if (!query) return true
      const baseName = item.kind === 'workers' && item.baseId ? current.baseNames.get(item.baseId) || '' : ''
      return `${item.name} ${item.detail || ''} ${item.level || ''} ${item.guildName || ''} ${baseName}`
        .toLowerCase()
        .includes(query)
    })
  }, [current, enabledKinds, hiddenIds, search])

  const renderMarkers = useMemo<RenderMarker[]>(() => {
    const bounds = sceneViewportBounds(
      renderViewport.view,
      renderViewport.width,
      renderViewport.height,
      CLUSTER_SIZE_PX * 2
    )
    const projected = visibleItems.flatMap((item) => {
      const position = toScene(item, activeLayer, size)
      return position && (item.id === selectedId || isScenePointVisible(position, bounds)) ? [{ item, position }] : []
    })

    if (projected.length <= MAX_INDIVIDUAL_MARKERS) {
      return projected.map(({ item, position }) => ({ key: item.id, item, position }))
    }

    const cellSize = CLUSTER_SIZE_PX / renderViewport.view.scale
    const buckets = new Map<string, { items: MapItem[]; x: number; y: number }>()
    const selected: RenderMarker[] = []
    for (const { item, position } of projected) {
      if (item.id === selectedId) {
        selected.push({ key: item.id, item, position })
        continue
      }
      const key = `${Math.floor(position.x / cellSize)}:${Math.floor(position.y / cellSize)}`
      const bucket = buckets.get(key) || { items: [], x: 0, y: 0 }
      bucket.items.push(item)
      bucket.x += position.x
      bucket.y += position.y
      buckets.set(key, bucket)
    }

    const clustered = Array.from(buckets, ([key, bucket]): RenderMarker => {
      if (bucket.items.length === 1) {
        const item = bucket.items[0]
        return { key: item.id, item, position: { x: bucket.x, y: bucket.y } }
      }
      return {
        key: `cluster:${key}`,
        count: bucket.items.length,
        position: { x: bucket.x / bucket.items.length, y: bucket.y / bucket.items.length }
      }
    })
    return [...selected, ...clustered]
  }, [activeLayer, renderViewport, selectedId, size, visibleItems])

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
    (target: View) => {
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
        const progress = Math.min(1, Math.max(0, (now - startedAt) / CONTROL_ZOOM_DURATION_MS))
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
    applyView(fitView(viewport.clientWidth, viewport.clientHeight, size))
    syncRenderViewport()
  }, [applyView, cancelResizeRenderSync, cancelViewAnimation, size, syncRenderViewport])

  const animateFit = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    viewportSizeRef.current = { width: viewport.clientWidth, height: viewport.clientHeight }
    animateView(fitView(viewport.clientWidth, viewport.clientHeight, size))
  }, [animateView, size])

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
      if (animated) animateView(target)
      else {
        cancelViewAnimation()
        cancelResizeRenderSync()
        applyView(target)
        syncRenderViewport()
      }
    },
    [animateView, applyView, cancelResizeRenderSync, cancelViewAnimation, size, syncRenderViewport]
  )

  const focusItem = (item: MapItem, returnFocus: HTMLElement) => {
    const viewport = viewportRef.current
    const position = toScene(item, activeLayer, size)
    if (!viewport || !position) return
    cancelViewAnimation()
    cancelResizeRenderSync()
    const rect = viewport.getBoundingClientRect()
    const minimum = coverScale(rect.width, rect.height, size)
    const scale = Math.min(
      minimum * MAX_ZOOM_RATIO,
      Math.max(viewRef.current.scale, minimum * (item.kind === 'workers' ? 24 : 8))
    )
    applyView(
      clampView(
        { scale, x: rect.width / 2 - position.x * scale, y: rect.height / 2 - position.y * scale },
        rect.width,
        rect.height,
        size
      )
    )
    syncRenderViewport()
    setSelectedId(item.id)
    onShowItem(item, returnFocus)
  }

  useImperativeHandle(ref, () => ({ focusItem, clearSelection: () => setSelectedId(null) }))

  useEffect(() => {
    setSelectedId(null)
    setImageState(activeLayer.imageUrl ? 'loading' : 'error')
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
    },
    [cancelResizeRenderSync, cancelViewAnimation]
  )

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
      className="map-viewport relative size-full touch-pinch-zoom overflow-hidden bg-[#0d161e] active:cursor-grabbing"
      role="application"
      aria-label="Interactive world map. Use arrow keys to pan and plus or minus to zoom."
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the map is an interactive pan and zoom canvas
      tabIndex={0}
      style={{ cursor: dragRef.current ? 'grabbing' : 'grab' }}
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
        if (coordinatesRef.current)
          coordinatesRef.current.textContent = `X ${Math.round(world.x)} · Y ${Math.round(world.y)}`

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
        style={{ width: size, height: size, '--marker-scale': '1' } as React.CSSProperties}
      >
        {activeLayer.imageUrl && (
          <img
            className={`map-artwork absolute inset-0 size-full select-none object-fill ${
              activeLayer.id === 'palpagos' ? 'map-artwork-palpagos' : ''
            } ${imageState === 'ready' ? 'block' : 'hidden'}`}
            src={activeLayer.imageUrl}
            alt=""
            draggable={false}
            onLoad={() => setImageState('ready')}
            onError={() => setImageState('error')}
          />
        )}
        {imageState !== 'ready' && <div className="fallback-grid absolute inset-0 size-full" aria-hidden="true" />}
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
                    cancelViewAnimation()
                    cancelResizeRenderSync()
                    const rect = viewport.getBoundingClientRect()
                    const minimum = coverScale(rect.width, rect.height, size)
                    const scale = Math.min(minimum * MAX_ZOOM_RATIO, viewRef.current.scale * 2.5)
                    applyView(
                      clampView(
                        {
                          scale,
                          x: rect.width / 2 - position.x * scale,
                          y: rect.height / 2 - position.y * scale
                        },
                        rect.width,
                        rect.height,
                        size
                      )
                    )
                    syncRenderViewport()
                  }}
                >
                  <span>{count && count > 999 ? '999+' : count}</span>
                </button>
              )
            }
            return (
              <button
                key={key}
                type="button"
                className={`map-marker marker-${item.kind} ${selectedId === item.id ? 'selected' : ''}`}
                style={{ left: position.x, top: position.y }}
                aria-label={markerText(item)}
                tabIndex={-1}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={(event) => {
                  event.stopPropagation()
                  setSelectedId(item.id)
                  onShowItem(item, event.currentTarget)
                }}
              >
                <span className="marker-label">{markerText(item)}</span>
              </button>
            )
          })}
        </div>
      </div>

      {imageState === 'error' && (
        <div className="pointer-events-none absolute left-1/2 top-3 -translate-x-1/2 rounded-md border border-[#665a3e] bg-[#302a20]/95 px-3 py-2 text-xs text-[#d5bd82]">
          Map artwork is not installed.
        </div>
      )}

      {children}

      <fieldset
        className={`absolute right-[18px] bottom-[18px] z-[18] m-0 flex overflow-hidden border border-[#d3eff2]/50 bg-[#070f14]/80 p-0 shadow-[0_9px_22px_rgb(0_0_0/28%)] transition-[opacity,transform] ${
          inspectorOpen ? 'pointer-events-none translate-y-2 opacity-0' : ''
        }`}
        aria-label="Map controls"
        aria-hidden={inspectorOpen}
        inert={inspectorOpen}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          className="grid h-11 min-w-11 cursor-pointer place-items-center border-0 bg-transparent text-lg text-[#eefeff] hover:bg-[#087fab] focus-visible:bg-[#087fab] focus-visible:outline-none"
          aria-label="Zoom out"
          onClick={() => {
            const point = center()
            zoomAt(viewRef.current.scale / 1.35, point.x, point.y, true)
          }}
        >
          −
        </button>
        <button
          type="button"
          className="grid h-11 min-w-[58px] cursor-pointer place-items-center border-x border-y-0 border-[#cdeef3]/35 bg-transparent text-[10px] font-bold tracking-[.06em] text-[#eefeff] uppercase hover:bg-[#087fab] focus-visible:bg-[#087fab] focus-visible:outline-none"
          title="Fit the active region"
          onClick={animateFit}
        >
          Fit
        </button>
        <button
          type="button"
          className="grid h-11 min-w-11 cursor-pointer place-items-center border-0 bg-transparent text-lg text-[#eefeff] hover:bg-[#087fab] focus-visible:bg-[#087fab] focus-visible:outline-none"
          aria-label="Zoom in"
          onClick={() => {
            const point = center()
            zoomAt(viewRef.current.scale * 1.35, point.x, point.y, true)
          }}
        >
          +
        </button>
      </fieldset>
      <div
        ref={coordinatesRef}
        className="pointer-events-none absolute bottom-[18px] left-[18px] z-[18] border-l-[3px] border-[#c3faff] bg-[#070f14]/75 px-2.5 py-[7px] text-xs tracking-[.06em] text-[#e5fbfd] max-sm:bottom-3.5 max-sm:left-3"
      >
        X 0 · Y 0
      </div>
    </section>
  )
})
