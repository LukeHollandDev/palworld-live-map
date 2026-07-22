import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { createRef, useLayoutEffect } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ItemKind, MapItem, MapLayer } from '../types'
import { MapViewport, type MapViewportHandle } from './MapViewport'

const VIEWPORT_WIDTH = 1200
const VIEWPORT_HEIGHT = 600
const MAP_SIZE = 8192

const layer: MapLayer = {
  id: 'palpagos',
  name: 'Palpagos Islands',
  bounds: [100, 100, -100, -100]
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

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('MapViewport zoom controls', () => {
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

describe('MapViewport marker stacking', () => {
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
})
