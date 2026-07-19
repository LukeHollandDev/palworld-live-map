import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from 'react'
import {
  clampView,
  fitScale,
  fitView,
  itemKey,
  MAX_ZOOM_RATIO,
  markerText,
  sceneSize,
  toScene,
  toWorld,
  type View
} from '../lib/map'
import type { ItemKind, MapItem, MapLayer } from '../types'

export interface MapViewportHandle {
  focusItem: (item: MapItem, returnFocus: HTMLElement) => void
  reset: () => void
  clearSelection: () => void
}

interface MapViewportProps {
  activeLayer: MapLayer
  items: MapItem[]
  enabledKinds: Set<ItemKind>
  hiddenKeys: Set<string>
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

export const MapViewport = forwardRef<MapViewportHandle, MapViewportProps>(function MapViewport(
  { activeLayer, items, enabledKinds, hiddenKeys, search, onShowItem, inspectorOpen, children },
  ref
) {
  const viewportRef = useRef<HTMLElement>(null)
  const sceneRef = useRef<HTMLDivElement>(null)
  const coordinatesRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<View>({ scale: 1, x: 0, y: 0 })
  const dragRef = useRef<Drag | null>(null)
  const size = useMemo(() => sceneSize(), [])
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const [imageState, setImageState] = useState<'loading' | 'ready' | 'error'>(
    activeLayer.imageUrl ? 'loading' : 'error'
  )

  const currentItems = useMemo(() => items.filter((item) => item.map === activeLayer.id), [activeLayer.id, items])
  const visibleItems = useMemo(() => {
    const query = search.trim().toLowerCase()
    return currentItems.filter((item) => {
      if (!enabledKinds.has(item.kind) || hiddenKeys.has(itemKey(item))) return false
      if (!query) return true
      const baseName =
        item.kind === 'workers' && item.baseId
          ? currentItems.find((candidate) => candidate.kind === 'bases' && candidate.baseId === item.baseId)?.name || ''
          : ''
      return `${item.name} ${item.detail || ''} ${item.level || ''} ${baseName}`.toLowerCase().includes(query)
    })
  }, [currentItems, enabledKinds, hiddenKeys, search])

  const applyView = useCallback(
    (view: View) => {
      viewRef.current = view
      const scene = sceneRef.current
      const viewport = viewportRef.current
      if (!scene || !viewport) return
      scene.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.scale})`
      const minimum = fitScale(viewport.clientWidth, viewport.clientHeight, size)
      const zoomRatio = Math.max(1, view.scale / minimum)
      scene.style.setProperty('--marker-scale', String(Math.min(2, Math.sqrt(zoomRatio)) / view.scale))
      scene.style.setProperty('--worker-scale', String(1 / view.scale))
    },
    [size]
  )

  const reset = useCallback(() => {
    const viewport = viewportRef.current
    if (!viewport?.clientWidth || !viewport.clientHeight) return
    const fitted = fitView(viewport.clientWidth, viewport.clientHeight, size)
    const boost = viewport.clientHeight > viewport.clientWidth ? 1.3 : 1.1
    const scale = fitted.scale * boost
    applyView(
      clampView(
        { scale, x: viewport.clientWidth / 2 - (size * scale) / 2, y: viewport.clientHeight / 2 - (size * scale) / 2 },
        viewport.clientWidth,
        viewport.clientHeight,
        size
      )
    )
  }, [applyView, size])

  const zoomAt = useCallback(
    (nextScale: number, clientX: number, clientY: number) => {
      const viewport = viewportRef.current
      if (!viewport) return
      const rect = viewport.getBoundingClientRect()
      const minimum = fitScale(rect.width, rect.height, size)
      const scale = Math.min(minimum * MAX_ZOOM_RATIO, Math.max(minimum, nextScale))
      const pointerX = clientX - rect.left
      const pointerY = clientY - rect.top
      const current = viewRef.current
      const sceneX = (pointerX - current.x) / current.scale
      const sceneY = (pointerY - current.y) / current.scale
      applyView(
        clampView(
          {
            scale,
            x: pointerX - sceneX * scale,
            y: pointerY - sceneY * scale
          },
          rect.width,
          rect.height,
          size
        )
      )
    },
    [applyView, size]
  )

  const focusItem = (item: MapItem, returnFocus: HTMLElement) => {
    const viewport = viewportRef.current
    const position = toScene(item, activeLayer, size)
    if (!viewport || !position) return
    const rect = viewport.getBoundingClientRect()
    const minimum = fitScale(rect.width, rect.height, size)
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
    setSelectedKey(itemKey(item))
    onShowItem(item, returnFocus)
  }

  useImperativeHandle(ref, () => ({ focusItem, reset, clearSelection: () => setSelectedKey(null) }))

  useEffect(() => {
    setSelectedKey(null)
    setImageState(activeLayer.imageUrl ? 'loading' : 'error')
    reset()
  }, [activeLayer, reset])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const observer = new ResizeObserver(reset)
    observer.observe(viewport)
    return () => observer.disconnect()
  }, [reset])

  useEffect(() => {
    const viewport = viewportRef.current
    if (!viewport) return
    const handleWheel = (event: WheelEvent) => {
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
      className="relative size-full touch-pinch-zoom overflow-hidden bg-[#111416] active:cursor-grabbing"
      role="application"
      aria-label="Interactive world map. Use arrow keys to pan and plus or minus to zoom."
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the map is an interactive pan and zoom canvas
      tabIndex={0}
      style={{ cursor: dragRef.current ? 'grabbing' : 'grab' }}
      onPointerDown={(event) => {
        if (event.button !== 0 || (event.target as Element).closest('button, input, aside')) return
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
      }}
      onPointerUp={(event) => {
        if (dragRef.current?.pointer !== event.pointerId) return
        dragRef.current = null
        event.currentTarget.style.cursor = 'grab'
      }}
      onPointerCancel={(event) => {
        if (dragRef.current?.pointer !== event.pointerId) return
        dragRef.current = null
        event.currentTarget.style.cursor = 'grab'
      }}
      onKeyDown={(event) => {
        const rect = event.currentTarget.getBoundingClientRect()
        const current = viewRef.current
        const pan = 56
        if (event.key === '+' || event.key === '=') {
          event.preventDefault()
          const point = center()
          zoomAt(current.scale * 1.25, point.x, point.y)
        } else if (event.key === '-') {
          event.preventDefault()
          const point = center()
          zoomAt(current.scale / 1.25, point.x, point.y)
        } else if (['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) {
          event.preventDefault()
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
        }
      }}
    >
      <div
        ref={sceneRef}
        className="map-scene"
        style={{ width: size, height: size, '--marker-scale': '1', '--worker-scale': '1' } as React.CSSProperties}
      >
        {activeLayer.imageUrl && (
          <img
            className={`absolute inset-0 size-full select-none object-fill ${imageState === 'ready' ? 'block' : 'hidden'}`}
            src={activeLayer.imageUrl}
            alt=""
            draggable={false}
            onLoad={() => setImageState('ready')}
            onError={() => setImageState('error')}
          />
        )}
        {imageState !== 'ready' && <div className="fallback-grid absolute inset-0 size-full" aria-hidden="true" />}
        <div className="absolute inset-0">
          {visibleItems.map((item) => {
            const position = toScene(item, activeLayer, size)
            if (!position) return null
            const key = itemKey(item)
            return (
              <button
                key={key}
                type="button"
                className={`map-marker marker-${item.kind} ${selectedKey === key ? 'selected' : ''}`}
                style={{ left: position.x, top: position.y }}
                aria-label={markerText(item)}
                tabIndex={-1}
                onPointerDown={(event) => event.stopPropagation()}
                onClick={(event) => {
                  event.stopPropagation()
                  setSelectedKey(key)
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
        className={`map-controls ${inspectorOpen ? 'inspector-open' : ''}`}
        aria-label="Map controls"
        onPointerDown={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          className="map-control"
          aria-label="Zoom out"
          onClick={() => {
            const point = center()
            zoomAt(viewRef.current.scale / 1.35, point.x, point.y)
          }}
        >
          −
        </button>
        <button type="button" className="map-control map-frame" title="Frame the active region" onClick={reset}>
          Frame
        </button>
        <button
          type="button"
          className="map-control"
          aria-label="Zoom in"
          onClick={() => {
            const point = center()
            zoomAt(viewRef.current.scale * 1.35, point.x, point.y)
          }}
        >
          +
        </button>
      </fieldset>
      <div ref={coordinatesRef} className="map-coordinates">
        X 0 · Y 0
      </div>
    </section>
  )
})
