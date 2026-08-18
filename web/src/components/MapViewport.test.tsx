import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { createRef, useLayoutEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ItemKind, MapItem, MapLayer } from '../types'
import { MapViewport, type MapViewportHandle } from './MapViewport'

const VIEWPORT_WIDTH = 1200
const VIEWPORT_HEIGHT = 600
const MAP_SIZE = 8192
const MARKER_BUDGET = 300

const layer: MapLayer = {
  id: 'palpagos',
  name: 'Palpagos Islands',
  bounds: [100, 100, -100, -100]
}

const tiledLayer: MapLayer = {
  ...layer,
  imageUrl: '/assets/map/palpagos.jpg?v=source',
  tilePyramid: {
    tileSize: 512,
    levels: [1024, 2048, 4096, 8192],
    urlTemplate: '/assets/map/palpagos-z{size}-x{x}-y{y}.webp?v=tiles'
  }
}

function readTransform(scene: HTMLElement) {
  const match = scene.style.transform.match(/^translate\(([-\d.]+)px, ([-\d.]+)px\) scale\(([-\d.]+)\)$/)
  if (!match) throw new Error(`Unexpected map transform: ${scene.style.transform}`)
  return { x: Number(match[1]), y: Number(match[2]), scale: Number(match[3]) }
}

interface ViewportDimensions {
  width: number
  height: number
}

function installViewportMocks(dimensions: ViewportDimensions = { width: VIEWPORT_WIDTH, height: VIEWPORT_HEIGHT }) {
  let now = 0
  let nextFrameId = 0
  const frames = new Map<number, FrameRequestCallback>()

  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => dimensions.width)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockImplementation(() => dimensions.height)
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
    () => new DOMRect(0, 0, dimensions.width, dimensions.height)
  )
  vi.spyOn(window.performance, 'now').mockImplementation(() => now)
  vi.stubGlobal('devicePixelRatio', 1)
  vi.stubGlobal(
    'requestAnimationFrame',
    vi.fn((callback: FrameRequestCallback) => {
      const id = ++nextFrameId
      frames.set(id, callback)
      return id
    })
  )
  vi.stubGlobal(
    'cancelAnimationFrame',
    vi.fn((id: number) => {
      frames.delete(id)
    })
  )

  return (elapsedMs: number) => {
    now += elapsedMs
    const pending = [...frames.values()]
    frames.clear()
    act(() => {
      for (const callback of pending) callback(now)
    })
  }
}

function installResizeObserverMock() {
  let callback: ResizeObserverCallback | null = null
  let observer: ResizeObserver | null = null

  class ResizeObserverMock implements ResizeObserver {
    constructor(nextCallback: ResizeObserverCallback) {
      callback = nextCallback
      observer = this
    }

    disconnect() {}
    observe() {}
    unobserve() {}
  }

  vi.stubGlobal('ResizeObserver', ResizeObserverMock)
  return () => {
    if (!callback || !observer) throw new Error('Expected ResizeObserver to be installed')
    act(() => callback?.([], observer as ResizeObserver))
  }
}

function renderViewport(items: MapItem[] = [], enabledKinds: Set<ItemKind> = new Set<ItemKind>()) {
  const result = render(
    <MapViewport
      activeLayer={layer}
      items={items}
      enabledKinds={enabledKinds}
      enabledPlayerStatuses={new Set(['online', 'offline'])}
      hiddenIds={new Set<string>()}
      search=""
      onShowItem={() => undefined}
      inspectorOpen={false}
    >
      {null}
    </MapViewport>
  )
  const scene = result.container.querySelector<HTMLElement>('.map-scene')
  if (!scene) throw new Error('Expected map scene')
  return scene
}

function markerGrid(count: number, columns: number): MapItem[] {
  const rows = Math.ceil(count / columns)
  return Array.from({ length: count }, (_, index) => {
    const row = Math.floor(index / columns)
    const column = index % columns
    return {
      id: `dense-${index}`,
      kind: 'effigies',
      name: `Dense marker ${index}`,
      x: -95 + (row / Math.max(1, rows - 1)) * 190,
      y: -95 + (column / Math.max(1, columns - 1)) * 190,
      map: layer.id
    }
  })
}

function markerNodes(scene: HTMLElement): HTMLButtonElement[] {
  return [...scene.querySelectorAll<HTMLButtonElement>('.map-marker')]
}

function representedItems(scene: HTMLElement): number {
  return markerNodes(scene).reduce((total, marker) => {
    if (!marker.classList.contains('map-cluster')) return total + 1
    const match = marker.getAttribute('aria-label')?.match(/^Zoom to (\d+) nearby map items$/)
    if (!match) throw new Error('Expected the cluster accessible name to contain its item count')
    return total + Number(match[1])
  }, 0)
}

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('MapViewport zoom controls', () => {
  it('uses decorative Tabler icons without changing control names', () => {
    installViewportMocks()
    const { container } = render(
      <MapViewport
        activeLayer={layer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        {null}
      </MapViewport>
    )

    const coordinatesIcon = container.querySelector('.tabler-icon-crosshair')
    const map = screen.getByRole('application')
    const zoomOut = screen.getByRole('button', { name: 'Zoom out' })
    const zoomIn = screen.getByRole('button', { name: 'Zoom in' })
    expect(coordinatesIcon).toHaveAttribute('aria-hidden', 'true')
    expect(screen.getByText('X — Y —')).toBeInTheDocument()
    fireEvent.pointerMove(map, { clientX: 720, clientY: 300 })
    expect(screen.getByText('X -344 Y 270')).toBeInTheDocument()
    fireEvent.pointerMove(zoomIn, { clientX: 1190, clientY: 590 })
    expect(screen.getByText('X -344 Y 270')).toBeInTheDocument()
    expect(zoomOut.querySelector('.tabler-icon-minus')).toHaveAttribute('aria-hidden', 'true')
    expect(zoomIn.querySelector('.tabler-icon-plus')).toHaveAttribute('aria-hidden', 'true')
    expect(zoomOut).not.toHaveTextContent('−')
    expect(zoomIn).not.toHaveTextContent('+')
  })

  it('keeps a cached image ready when it loads before mount effects run', () => {
    installViewportMocks()
    const imageLayer = { ...layer, imageUrl: '/assets/map/palpagos.jpg?v=test' }
    const canvasContext = {
      drawImage: vi.fn(),
      getImageData: vi.fn(() => ({ data: new Uint8ClampedArray([12, 21, 31, 255]) }))
    }
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(
      canvasContext as unknown as CanvasRenderingContext2D
    )

    function CompleteCachedImage() {
      useLayoutEffect(() => {
        document.querySelector<HTMLImageElement>('.map-artwork')?.dispatchEvent(new Event('load', { bubbles: true }))
      }, [])
      return null
    }

    const { container, rerender } = render(
      <MapViewport
        activeLayer={imageLayer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        <CompleteCachedImage />
      </MapViewport>
    )

    expect(screen.getByRole('application')).toHaveClass('map-layer-palpagos')
    expect(container.querySelector('.map-artwork')).toHaveClass('block')
    expect(container.querySelector('.map-artwork')).not.toHaveClass('map-artwork-palpagos')
    expect(container.querySelector('.fallback-grid')).not.toBeInTheDocument()
    expect(container.querySelector('.map-cartography-frame')).not.toBeInTheDocument()
    expect(screen.getByRole('application')).toHaveStyle({ '--map-background': 'rgb(11 20 31)' })
    expect(canvasContext.drawImage).toHaveBeenCalledOnce()

    rerender(
      <MapViewport
        activeLayer={{
          ...imageLayer,
          id: 'world-tree',
          imageUrl: '/assets/map/world-tree.jpg?v=test'
        }}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        {null}
      </MapViewport>
    )
    expect(screen.getByRole('application')).toHaveClass('map-layer-world-tree')
  })

  it('loads only the fitted LOD tiles and keeps the ready layer visible across an LOD transition', () => {
    installViewportMocks()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    const { container } = render(
      <MapViewport
        activeLayer={tiledLayer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        {null}
      </MapViewport>
    )

    expect(screen.getByRole('application')).toHaveAccessibleName(/Palpagos Islands interactive world map/)
    let layers = [...container.querySelectorAll<HTMLElement>('.map-tile-layer')]
    expect(layers).toHaveLength(1)
    expect(layers[0]).toHaveAttribute('data-map-tile-level', '1024')
    let tiles = [...layers[0].querySelectorAll<HTMLImageElement>('.map-tile')]
    expect(tiles).toHaveLength(4)
    expect(tiles.map(({ src }) => new URL(src).pathname)).toEqual([
      '/assets/map/palpagos-z1024-x0-y0.webp',
      '/assets/map/palpagos-z1024-x1-y0.webp',
      '/assets/map/palpagos-z1024-x0-y1.webp',
      '/assets/map/palpagos-z1024-x1-y1.webp'
    ])
    for (const tile of tiles) fireEvent.load(tile)
    expect(layers[0]).toHaveClass('is-ready')

    const viewport = screen.getByRole('application')
    for (let step = 0; step < 6; step++)
      fireEvent.wheel(viewport, { clientX: VIEWPORT_WIDTH / 2, clientY: VIEWPORT_HEIGHT / 2, deltaY: -100 })

    layers = [...container.querySelectorAll<HTMLElement>('.map-tile-layer')]
    expect(layers).toHaveLength(2)
    expect(layers[0]).toHaveAttribute('data-map-tile-level', '1024')
    expect(layers[0]).toHaveClass('is-ready')
    expect(layers[1]).toHaveAttribute('data-map-tile-level', '2048')
    expect(layers[1]).not.toHaveClass('is-ready')

    for (let step = 0; step < 6; step++)
      fireEvent.wheel(viewport, { clientX: VIEWPORT_WIDTH / 2, clientY: VIEWPORT_HEIGHT / 2, deltaY: -100 })

    layers = [...container.querySelectorAll<HTMLElement>('.map-tile-layer')]
    expect(layers).toHaveLength(2)
    expect(layers[0]).toHaveAttribute('data-map-tile-level', '1024')
    expect(layers[0]).toHaveClass('is-ready')
    expect(layers[1]).toHaveAttribute('data-map-tile-level', '4096')
    expect(layers[1]).not.toHaveClass('is-ready')
    tiles = [...layers[1].querySelectorAll<HTMLImageElement>('.map-tile')]
    expect(tiles.length).toBeGreaterThan(0)
    expect(tiles.length).toBeLessThanOrEqual(36)
    for (const tile of tiles) fireEvent.load(tile)

    layers = [...container.querySelectorAll<HTMLElement>('.map-tile-layer')]
    expect(layers).toHaveLength(1)
    expect(layers[0]).toHaveAttribute('data-map-tile-level', '4096')
    expect(layers[0]).toHaveClass('is-ready')
  })

  it('falls back to the source artwork when a requested tile fails', () => {
    installViewportMocks()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
    const { container } = render(
      <MapViewport
        activeLayer={tiledLayer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        {null}
      </MapViewport>
    )

    const failedTile = container.querySelector<HTMLImageElement>('.map-tile')
    if (!failedTile) throw new Error('Expected a requested map tile')
    fireEvent.error(failedTile)

    const fallback = container.querySelector<HTMLImageElement>('.map-artwork')
    expect(fallback).toHaveAttribute('src', tiledLayer.imageUrl)
    expect(fallback).toHaveClass('hidden')
    fireEvent.load(fallback as HTMLImageElement)
    expect(fallback).toHaveClass('block')
    expect(container.querySelector('.fallback-grid')).not.toBeInTheDocument()
    expect(screen.queryByText('Map artwork is not installed.')).not.toBeInTheDocument()
  })

  it('removes visually hidden controls from keyboard and assistive technology navigation', () => {
    installViewportMocks()
    const props = {
      activeLayer: layer,
      items: [] as MapItem[],
      enabledKinds: new Set<ItemKind>(),
      enabledPlayerStatuses: new Set(['online', 'offline'] as const),
      hiddenIds: new Set<string>(),
      search: '',
      onShowItem: () => undefined
    }
    const { container, rerender } = render(
      <MapViewport {...props} inspectorOpen={false}>
        {null}
      </MapViewport>
    )

    expect(screen.getByRole('group', { name: 'Map controls' })).toBeInTheDocument()
    rerender(
      <MapViewport {...props} inspectorOpen>
        {null}
      </MapViewport>
    )

    const controls = container.querySelector('fieldset[aria-label="Map controls"]')
    expect(controls).toHaveAttribute('aria-hidden', 'true')
    expect(controls).toHaveAttribute('inert')
    expect(screen.queryByRole('group', { name: 'Map controls' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zoom out' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Fit' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zoom in' })).not.toBeInTheDocument()

    rerender(
      <MapViewport {...props} inspectorOpen={false}>
        {null}
      </MapViewport>
    )
    expect(screen.getByRole('group', { name: 'Map controls' })).toBeInTheDocument()
  })

  it('does not pan or zoom for keyboard and wheel input from inspector children', () => {
    installViewportMocks()
    const { container } = render(
      <MapViewport
        activeLayer={layer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        <aside>
          <button type="button">Inspector action</button>
        </aside>
      </MapViewport>
    )
    const scene = container.querySelector<HTMLElement>('.map-scene')
    if (!scene) throw new Error('Expected map scene')
    const initial = scene.style.transform
    const inspectorAction = screen.getByRole('button', { name: 'Inspector action' })

    fireEvent.keyDown(inspectorAction, { key: 'ArrowRight' })
    expect(scene.style.transform).toBe(initial)
    expect(fireEvent.wheel(inspectorAction, { deltaY: -100, clientX: 100, clientY: 100 })).toBe(true)
    expect(scene.style.transform).toBe(initial)

    expect(fireEvent.wheel(screen.getByRole('application'), { deltaY: -100, clientX: 600, clientY: 300 })).toBe(false)
    expect(scene.style.transform).not.toBe(initial)
  })

  it('fits the whole map on a short viewport and animates zoom in, zoom out, and fit', () => {
    const advanceFrame = installViewportMocks()
    const scene = renderViewport()
    const fitted = readTransform(scene)

    expect(fitted).toEqual({ x: 364, y: 64, scale: (VIEWPORT_HEIGHT - 128) / MAP_SIZE })

    fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
    expect(readTransform(scene)).toEqual(fitted)

    advanceFrame(110)
    const midway = readTransform(scene)
    expect(midway.scale).toBeGreaterThan(fitted.scale)

    advanceFrame(110)
    const zoomed = readTransform(scene)
    expect(zoomed.scale).toBeGreaterThan(midway.scale)

    fireEvent.click(screen.getByRole('button', { name: 'Zoom out' }))
    expect(readTransform(scene)).toEqual(zoomed)
    advanceFrame(220)
    expect(readTransform(scene).scale).toBeCloseTo(fitted.scale)

    fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
    advanceFrame(220)
    const zoomedAgain = readTransform(scene)
    fireEvent.click(screen.getByRole('button', { name: 'Fit' }))
    expect(readTransform(scene)).toEqual(zoomedAgain)
    advanceFrame(220)
    expect(readTransform(scene)).toEqual(fitted)
  })

  it('animates an item focus instead of jumping to its map position', () => {
    const advanceFrame = installViewportMocks()
    const item: MapItem = {
      id: 'focus-target',
      kind: 'players',
      name: 'Focus target',
      x: 80,
      y: -70,
      map: layer.id
    }
    const ref = createRef<MapViewportHandle>()
    const onShowItem = vi.fn()
    const { container } = render(
      <MapViewport
        ref={ref}
        activeLayer={layer}
        items={[item]}
        enabledKinds={new Set<ItemKind>(['players'])}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={onShowItem}
        inspectorOpen={false}
      >
        <button type="button" onClick={(event) => ref.current?.focusItem(item, event.currentTarget)}>
          Focus selected item
        </button>
      </MapViewport>
    )
    const scene = container.querySelector<HTMLElement>('.map-scene')
    if (!scene) throw new Error('Expected map scene')
    const fitted = readTransform(scene)

    fireEvent.click(screen.getByRole('button', { name: 'Focus selected item' }))
    expect(readTransform(scene)).toEqual(fitted)
    expect(onShowItem).toHaveBeenCalledOnce()

    advanceFrame(210)
    const midway = readTransform(scene)
    expect(midway.scale).toBeGreaterThan(fitted.scale)
    expect(midway.x).not.toBe(fitted.x)

    advanceFrame(210)
    const focused = readTransform(scene)
    expect(focused.scale).toBeGreaterThan(midway.scale)
    expect(focused.x).not.toBe(midway.x)
  })

  it('keeps the artwork padded on initial load and screen rotation', () => {
    const dimensions = { width: 360, height: 640 }
    installViewportMocks(dimensions)
    const triggerResize = installResizeObserverMock()
    const scene = renderViewport()

    const expectPaddedMap = () => {
      const view = readTransform(scene)
      const renderedSize = MAP_SIZE * view.scale

      expect(view.x).toBeGreaterThanOrEqual(63.9)
      expect(view.y).toBeGreaterThanOrEqual(63.9)
      expect(dimensions.width - (view.x + renderedSize)).toBeGreaterThanOrEqual(63.9)
      expect(dimensions.height - (view.y + renderedSize)).toBeGreaterThanOrEqual(63.9)
    }

    expect(readTransform(scene)).toEqual({ x: 64, y: 204, scale: (dimensions.width - 128) / MAP_SIZE })
    expectPaddedMap()

    dimensions.width = 640
    dimensions.height = 360
    triggerResize()

    expect(readTransform(scene)).toEqual({ x: 204, y: 64, scale: (dimensions.height - 128) / MAP_SIZE })
    expectPaddedMap()
  })

  it('keeps wheel zoom immediate', () => {
    installViewportMocks()
    const scene = renderViewport()
    const fitted = readTransform(scene)

    fireEvent.wheel(screen.getByRole('application'), { clientX: 600, clientY: 300, deltaY: -100 })

    expect(readTransform(scene).scale).toBeGreaterThan(fitted.scale)
  })

  it('restores the saved zoom level for the active map', () => {
    installViewportMocks()
    const scene = renderViewport()
    const fitted = readTransform(scene)

    fireEvent.wheel(screen.getByRole('application'), { clientX: 600, clientY: 300, deltaY: -100 })
    const zoomedScale = readTransform(scene).scale
    expect(zoomedScale).toBeGreaterThan(fitted.scale)
    cleanup()

    const restoredScene = renderViewport()
    expect(readTransform(restoredScene).scale).toBeCloseTo(zoomedScale)
  })

  it('updates the scene during resize and defers marker culling until resizing settles', () => {
    vi.useFakeTimers()
    const dimensions = { width: 600, height: 600 }
    installViewportMocks(dimensions)
    const triggerResize = installResizeObserverMock()
    const farMarker: MapItem = {
      id: 'far-marker',
      kind: 'players',
      name: 'Far marker',
      x: 0,
      y: (6000 / MAP_SIZE) * 200 - 100,
      map: layer.id
    }
    const scene = renderViewport([farMarker], new Set<ItemKind>(['players']))
    const viewport = screen.getByRole('application')

    for (let index = 0; index < 10; index++) {
      fireEvent.wheel(viewport, { clientX: 300, clientY: 300, deltaY: -100 })
    }
    expect(screen.queryByRole('button', { name: 'Far marker' })).not.toBeInTheDocument()

    const initialTransform = scene.style.transform
    dimensions.width = 900
    triggerResize()
    const intermediateTransform = scene.style.transform
    expect(intermediateTransform).not.toBe(initialTransform)

    dimensions.width = 1200
    triggerResize()
    expect(scene.style.transform).not.toBe(intermediateTransform)
    expect(screen.queryByRole('button', { name: 'Far marker' })).not.toBeInTheDocument()

    act(() => vi.advanceTimersByTime(119))
    expect(screen.queryByRole('button', { name: 'Far marker' })).not.toBeInTheDocument()

    act(() => vi.advanceTimersByTime(1))
    expect(screen.getByRole('button', { name: 'Far marker' })).toBeInTheDocument()

    dimensions.width = 1100
    triggerResize()
    expect(vi.getTimerCount()).toBe(1)
    cleanup()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('MapViewport marker clustering', () => {
  it('renders exactly the marker budget as individual markers', () => {
    installViewportMocks()
    const scene = renderViewport(markerGrid(MARKER_BUDGET, 20), new Set<ItemKind>(['effigies']))

    expect(markerNodes(scene)).toHaveLength(MARKER_BUDGET)
    expect(scene.querySelectorAll('.map-cluster')).toHaveLength(0)
  })

  it('keeps dense zoom levels within budget without dropping represented items', () => {
    installViewportMocks()
    const items = markerGrid(1_600, 40)
    const scene = renderViewport(items, new Set<ItemKind>(['effigies']))
    const viewport = screen.getByRole('application')

    expect(representedItems(scene)).toBe(items.length)

    let sawClusters = false
    let sawIndividualView = false
    for (let step = 0; step < 16; step++) {
      const markers = markerNodes(scene)
      const clusters = scene.querySelectorAll('.map-cluster')
      expect(markers.length).toBeLessThanOrEqual(MARKER_BUDGET)
      sawClusters ||= clusters.length > 0
      sawIndividualView ||= clusters.length === 0 && markers.length > 0

      if (step < 15) {
        fireEvent.wheel(viewport, {
          clientX: VIEWPORT_WIDTH / 2,
          clientY: VIEWPORT_HEIGHT / 2,
          deltaY: -100
        })
      }
    }

    expect(sawClusters).toBe(true)
    expect(sawIndividualView).toBe(true)
  })

  it('keeps a selected marker individual and inside the total budget', () => {
    installViewportMocks()
    const items = markerGrid(1_600, 40)
    const target = items[Math.floor(items.length / 2)]
    const ref = createRef<MapViewportHandle>()
    const { container } = render(
      <MapViewport
        ref={ref}
        activeLayer={layer}
        items={items}
        enabledKinds={new Set<ItemKind>(['effigies'])}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        <button type="button" onClick={(event) => ref.current?.focusItem(target, event.currentTarget)}>
          Focus dense target
        </button>
      </MapViewport>
    )
    const scene = container.querySelector<HTMLElement>('.map-scene')
    if (!scene) throw new Error('Expected map scene')

    fireEvent.click(screen.getByRole('button', { name: 'Focus dense target' }))

    expect(screen.getByRole('button', { name: target.name })).toHaveClass('selected')
    expect(scene.querySelectorAll('.map-cluster').length).toBeGreaterThan(0)
    expect(markerNodes(scene).length).toBeLessThanOrEqual(MARKER_BUDGET)
    expect(representedItems(scene)).toBe(items.length)
  })

  it('keeps the marker budget after zooming into a cluster', () => {
    const advanceFrame = installViewportMocks()
    const scene = renderViewport(markerGrid(1_600, 40), new Set<ItemKind>(['effigies']))
    const cluster = scene.querySelector<HTMLButtonElement>('.map-cluster')
    if (!cluster) throw new Error('Expected a marker cluster')
    const fitted = readTransform(scene)

    fireEvent.click(cluster)
    advanceFrame(420)

    expect(readTransform(scene).scale).toBeGreaterThan(fitted.scale)
    expect(markerNodes(scene).length).toBeLessThanOrEqual(MARKER_BUDGET)
  })
})

describe('MapViewport marker stacking', () => {
  it('keeps companion Pals off the map and searches them through their exact owner', () => {
    installViewportMocks()
    const items: MapItem[] = [
      { id: 'player-luke', kind: 'players', name: 'Luke', x: 0, y: 0, map: layer.id },
      { id: 'player-robin', kind: 'players', name: 'Robin', x: 10, y: 10, map: layer.id },
      {
        id: 'companion-spark',
        kind: 'companions',
        name: 'Spark',
        ownerId: 'player-luke',
        x: 1,
        y: 1,
        map: layer.id
      },
      {
        id: 'companion-lookalike',
        kind: 'companions',
        name: 'Suffix',
        ownerId: 'player-luke-suffix',
        x: 2,
        y: 2,
        map: layer.id
      }
    ]
    const props = {
      activeLayer: layer,
      items,
      enabledKinds: new Set<ItemKind>(['players', 'companions']),
      enabledPlayerStatuses: new Set(['online', 'offline'] as const),
      hiddenIds: new Set<string>(),
      onShowItem: () => undefined,
      inspectorOpen: false
    }
    const { rerender } = render(
      <MapViewport {...props} search="">
        {null}
      </MapViewport>
    )

    expect(screen.getByRole('button', { name: 'Luke' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Robin' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Spark' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Suffix' })).not.toBeInTheDocument()

    rerender(
      <MapViewport {...props} search="Spark">
        {null}
      </MapViewport>
    )

    expect(screen.getByRole('button', { name: 'Luke' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Robin' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Spark' })).not.toBeInTheDocument()

    rerender(
      <MapViewport {...props} search="Suffix">
        {null}
      </MapViewport>
    )

    expect(screen.queryByRole('button', { name: 'Luke' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Robin' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Suffix' })).not.toBeInTheDocument()
  })

  it('keeps category precedence at one coordinate and raises the selected marker above it', () => {
    installViewportMocks()
    const items: MapItem[] = [
      { id: 'player', kind: 'players', name: 'Player marker', x: 0, y: 0, map: layer.id },
      { id: 'base', kind: 'bases', name: 'Base marker', x: 0, y: 0, map: layer.id },
      { id: 'worker', kind: 'workers', name: 'Worker marker', x: 0, y: 0, map: layer.id }
    ]
    renderViewport(items, new Set<ItemKind>(['players', 'bases', 'workers']))

    const player = screen.getByRole('button', { name: 'Player marker' })
    const base = screen.getByRole('button', { name: 'Base marker' })
    const worker = screen.getByRole('button', { name: 'Worker marker' })
    const stackOf = (marker: HTMLElement) => Number(marker.style.getPropertyValue('--marker-stack'))

    expect(player.style.left).toBe(base.style.left)
    expect(base.style.left).toBe(worker.style.left)
    expect(player.style.top).toBe(base.style.top)
    expect(base.style.top).toBe(worker.style.top)
    expect(stackOf(player)).toBeGreaterThan(stackOf(base))
    expect(stackOf(base)).toBeGreaterThan(stackOf(worker))
    fireEvent.click(worker)

    expect(worker).toHaveClass('selected')
    expect(stackOf(worker)).toBeGreaterThan(stackOf(player))

    fireEvent.click(base)

    expect(base).toHaveClass('selected')
    expect(worker).not.toHaveClass('selected')
    expect(stackOf(worker)).toBeLessThan(stackOf(base))
    expect(stackOf(base)).toBeGreaterThan(stackOf(player))
  })

  it('focuses a fixed shared position at the encoded zoom without rendering a marker', () => {
    installViewportMocks()
    const ref = createRef<MapViewportHandle>()
    const sharedPosition = { region: layer.id, x: 0, y: 0, zoom: 8 }
    const { container } = render(
      <MapViewport
        ref={ref}
        activeLayer={layer}
        items={[]}
        enabledKinds={new Set<ItemKind>()}
        enabledPlayerStatuses={new Set(['online', 'offline'])}
        hiddenIds={new Set<string>()}
        search=""
        onShowItem={() => undefined}
        inspectorOpen={false}
      >
        {null}
      </MapViewport>
    )

    expect(screen.queryByRole('img', { name: /Shared position/ })).not.toBeInTheDocument()

    act(() => ref.current?.focusPosition(sharedPosition))

    const scene = container.querySelector<HTMLElement>('.map-scene')
    if (!scene) throw new Error('Expected map scene')
    const view = readTransform(scene)
    expect(4096 * view.scale + view.x).toBeCloseTo(VIEWPORT_WIDTH / 2)
    expect(4096 * view.scale + view.y).toBeCloseTo(VIEWPORT_HEIGHT / 2)
    expect(ref.current?.getZoomRatio()).toBeCloseTo(8)
    expect(screen.queryByRole('img', { name: /Shared position/ })).not.toBeInTheDocument()
  })
})
