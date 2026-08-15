import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const responses: Record<string, unknown> = {
  '/api/config': {
    pollIntervalMs: 60_000,
    worldPollIntervalMs: 60_000,
    worldDataEnabled: true,
    layers: [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }],
    catalogueUrl: '/assets/test-world-catalogue.json',
    landmarks: [],
    landmarkCatalogue: {
      gameVersion: '1.0.1.100619',
      generator: 'palworld-asset-exporter/3',
      decoder: 'CUE4Parse'
    }
  },
  '/assets/test-world-catalogue.json': {
    gameVersion: '1.0.1.100619',
    generator: 'palworld-asset-exporter/4',
    decoder: 'CUE4Parse',
    locations: []
  },
  '/api/players': {
    server: { name: 'Test Realm', description: 'A test server', version: 'v1.0.1.100619' },
    metrics: {
      currentPlayers: 1,
      maxPlayers: 32,
      serverFps: 60,
      serverFrameTime: 16.7,
      uptimeSeconds: 3600,
      baseCount: 1,
      days: 42
    },
    metricsAvailable: true,
    metricsStale: false,
    connected: true,
    stale: false,
    lastSuccessAt: new Date().toISOString(),
    saveEnabled: false,
    saveAvailable: false,
    saveStale: false,
    players: [{ id: 'player-luke', name: 'Luke', level: 55, online: true, x: 10, y: 20, map: 'palpagos' }]
  },
  '/api/objects': {
    enabled: true,
    available: true,
    stale: false,
    unsupported: false,
    truncated: false,
    total: 1,
    objects: [
      {
        id: 'base-1',
        kind: 'bases',
        name: 'Home',
        baseId: 'base-1',
        guildKey: 'guild-1',
        x: 30,
        y: 40,
        map: 'palpagos'
      }
    ]
  }
}

function mockAPI(resolve: (path: string) => unknown = (path) => responses[path]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request) => {
      const path =
        typeof input === 'string' ? input : input instanceof URL ? input.pathname : new URL(input.url).pathname
      const body = resolve(path)
      if (body instanceof Error) throw body
      if (body instanceof Response) return body
      return body
        ? new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
        : new Response(null, { status: 404 })
    })
  )
}

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  window.history.replaceState({}, '', '/')
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  Object.defineProperty(document, 'execCommand', { configurable: true, value: undefined })
})

describe('App', () => {
  it('can retry a transient configuration failure', async () => {
    let configRequests = 0
    mockAPI((path) => {
      if (path === '/api/config' && configRequests++ === 0) return new Error('temporarily unavailable')
      return responses[path]
    })
    const user = userEvent.setup()
    render(<App />)

    await user.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('heading', { name: 'Test Realm' })).toBeVisible()
  })

  it('renders from configuration while the world catalogue hydrates in the background', async () => {
    let resolveCatalogue: (response: Response) => void = () => {
      throw new Error('Catalogue request did not start')
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path =
          typeof input === 'string' ? input : input instanceof URL ? input.pathname : new URL(input.url).pathname
        if (path === '/assets/test-world-catalogue.json') {
          return new Promise<Response>((resolve) => {
            resolveCatalogue = resolve
          })
        }
        const body = responses[path]
        return body
          ? new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
          : new Response(null, { status: 404 })
      })
    )

    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Test Realm' })).toBeVisible()
    expect(screen.queryByText('Loading map…')).not.toBeInTheDocument()
    expect(screen.getByText('Alpha Pals (0)')).toBeVisible()

    resolveCatalogue(
      new Response(
        JSON.stringify({
          ...(responses['/assets/test-world-catalogue.json'] as object),
          locations: [
            {
              id: 'hydrated-alpha',
              kind: 'alpha-pals',
              name: 'Hydrated Alpha',
              x: 12,
              y: 22,
              map: 'palpagos'
            }
          ]
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    expect(await screen.findByText('Alpha Pals (1)')).toBeVisible()
    expect(screen.getByRole('checkbox', { name: 'Show Alpha Pals' })).not.toBeChecked()
    expect(screen.queryByRole('button', { name: 'Hydrated Alpha' })).not.toBeInTheDocument()
  })

  it('keeps the configured map usable and retries a failed catalogue independently', async () => {
    let catalogueRequests = 0
    mockAPI((path) => {
      if (path !== '/assets/test-world-catalogue.json') return responses[path]
      if (catalogueRequests++ === 0) return new Error('temporarily unavailable')
      return {
        ...(responses[path] as object),
        locations: [
          {
            id: 'retried-alpha',
            kind: 'alpha-pals',
            name: 'Retried Alpha',
            x: 12,
            y: 22,
            map: 'palpagos'
          }
        ]
      }
    })
    const user = userEvent.setup()
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Test Realm' })).toBeVisible()
    expect(screen.queryByText('Map unavailable')).not.toBeInTheDocument()
    expect(screen.getByText('Additional static map locations are temporarily unavailable.')).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Retry catalogue' }))
    expect(await screen.findByText('Alpha Pals (1)')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Retry catalogue' })).not.toBeInTheDocument()
    expect(catalogueRequests).toBe(2)
  })

  it('aborts catalogue hydration when the app unmounts', async () => {
    let catalogueSignal: AbortSignal | undefined
    let markCatalogueStarted: () => void = () => undefined
    const catalogueStarted = new Promise<void>((resolve) => {
      markCatalogueStarted = resolve
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const path =
          typeof input === 'string' ? input : input instanceof URL ? input.pathname : new URL(input.url).pathname
        if (path === '/assets/test-world-catalogue.json') {
          catalogueSignal = init?.signal instanceof AbortSignal ? init.signal : undefined
          markCatalogueStarted()
          return new Promise<Response>(() => undefined)
        }
        const body = responses[path]
        return body
          ? new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
          : new Response(null, { status: 404 })
      })
    )

    const view = render(<App />)
    await catalogueStarted
    view.unmount()

    expect(catalogueSignal?.aborted).toBe(true)
  })

  it('renders live server data and opens player details', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)

    const serverTitle = await screen.findByRole('heading', { name: 'Test Realm' })
    expect(serverTitle).toBeInTheDocument()
    expect(serverTitle).toHaveClass('font-bold', 'text-[21px]')
    const statusBar = screen.getByRole('banner')
    expect(statusBar).toHaveClass('absolute', 'pointer-events-none')
    expect(statusBar).toHaveClass('min-[1600px]:inset-x-[324px]', 'min-[1600px]:px-0')
    expect(statusBar).not.toHaveClass('bg-[#0f1b21]/98')
    const commandbarLayout = statusBar.querySelector<HTMLElement>('.status-commandbar-layout')
    const serverSurface = serverTitle.parentElement?.parentElement
    if (!commandbarLayout || !serverSurface) throw new Error('Expected the responsive command bar layout')
    expect(commandbarLayout).toHaveClass(
      'grid-cols-[54px_minmax(0,1fr)_54px_54px]',
      'max-sm:grid-cols-3',
      'max-sm:grid-rows-[70px_44px]'
    )
    expect(serverSurface).toHaveClass(
      'pal-glass-surface',
      'h-[54px]',
      'max-sm:col-span-3',
      'max-sm:col-start-1',
      'row-start-1'
    )
    const filterControl = within(statusBar).getByRole('button', { name: 'Map filters' })
    const progressControl = within(statusBar).getByRole('button', { name: 'My Progress' })
    const leaderboardControl = within(statusBar).getByRole('button', { name: 'Leaderboards' })
    for (const control of [filterControl, progressControl, leaderboardControl]) {
      expect(control).toHaveClass(
        'header-panel-control',
        'relative',
        'h-[54px]',
        'w-full',
        'min-w-0',
        'max-sm:row-start-2',
        'max-sm:h-11'
      )
      expect(control).not.toHaveAttribute('aria-hidden')
      expect(control).not.toHaveAttribute('inert')
    }
    expect(filterControl.parentElement).toBe(progressControl.parentElement)
    expect(progressControl.parentElement).toBe(leaderboardControl.parentElement)
    expect(filterControl).toHaveClass('pal-selected', 'col-start-1', 'max-sm:row-start-2')
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(filterControl.querySelector('svg')).toHaveClass('tabler-icon-filter')
    expect(progressControl).not.toHaveClass('pal-selected')
    expect(progressControl).toHaveClass('col-start-3', 'max-sm:col-start-2', 'max-sm:row-start-2')
    expect(progressControl).toHaveAttribute('aria-expanded', 'false')
    expect(progressControl.querySelector('svg')).toHaveClass('tabler-icon-checklist')
    expect(leaderboardControl).not.toHaveClass('pal-selected')
    expect(leaderboardControl).toHaveClass('col-start-4', 'max-sm:col-start-3', 'max-sm:row-start-2')
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardControl.querySelector('svg')).toHaveClass('tabler-icon-trophy')
    expect(filterControl.compareDocumentPosition(serverSurface) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
    expect(serverSurface.compareDocumentPosition(leaderboardControl) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
    expect(screen.getByRole('main')).toHaveClass('absolute', 'inset-0')
    expect(screen.queryByText('Demo data')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('1 / 32 players')
    const serverFps = screen.getByText('Server FPS')
    expect(serverFps).toBeVisible()
    expect(serverFps).toHaveClass('text-[11px]')
    expect(serverFps.nextElementSibling).toHaveClass('text-[15px]')
    expect(screen.queryByText('Frame')).not.toBeInTheDocument()
    expect(screen.queryByText('16.7 ms')).not.toBeInTheDocument()
    expect(screen.getByText('Uptime')).toBeVisible()
    expect(screen.getByText('Bases')).toBeVisible()
    expect(screen.getByText('Server FPS').closest('[data-tooltip]')).toHaveAttribute(
      'data-tooltip',
      "The server's current frames per second, as reported by Palworld."
    )
    expect(screen.getByRole('link', { name: 'Palworld Live Map on GitHub' })).toHaveAttribute(
      'href',
      'https://github.com/LukeHollandDev/palworld-live-map'
    )
    expect(screen.queryByRole('link', { name: "Luke Holland's website" })).not.toBeInTheDocument()
    expect(screen.queryByText('Built by Luke')).not.toBeInTheDocument()
    expect(
      within(screen.getByRole('navigation', { name: 'Project links' })).queryByRole('button', {
        name: 'Leaderboards'
      })
    ).not.toBeInTheDocument()

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(within(explorer).getByRole('button', { name: 'View Luke · Lv 55' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('PLAYER DETAILS')).toBeVisible()
    const detailsTitle = screen.getByRole('heading', { name: 'Luke' })
    expect(detailsTitle).toBeInTheDocument()
    await waitFor(() => expect(detailsTitle).toHaveFocus())
    expect(screen.getByText(/X 10\s+Y 20/)).toBeInTheDocument()
    expect(screen.getByText('No guild membership is known for this player.')).toBeVisible()
  })

  it('composes player claims with private fixed-position sharing and landmark sharing', async () => {
    mockAPI((path) => {
      if (path === '/api/config') {
        return { ...(responses[path] as Record<string, unknown>), playerClaimsEnabled: true }
      }
      if (path === '/api/me') {
        return new Response(JSON.stringify({ error: 'authentication_required' }), {
          status: 401,
          headers: { 'Content-Type': 'application/json' }
        })
      }
      if (path === '/assets/test-world-catalogue.json') {
        return {
          ...(responses[path] as Record<string, unknown>),
          locations: [
            {
              id: 'waypoint-share-test',
              kind: 'waypoints',
              name: 'Small Settlement',
              x: 15,
              y: 25,
              map: 'palpagos'
            }
          ]
        }
      }
      return responses[path]
    })
    const user = userEvent.setup()
    vi.stubGlobal('navigator', { ...navigator, clipboard: undefined })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: undefined })
    render(<App />)

    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    await user.click(await within(explorer).findByRole('button', { name: 'View Luke · Lv 55' }))
    let inspector = screen.getByRole('dialog', { name: 'Luke' })
    expect(await within(inspector).findByRole('button', { name: 'This is me' })).toBeVisible()
    const sharePlayer = within(inspector).getByRole('button', { name: 'Share position' })
    expect(sharePlayer.querySelector('svg')).toHaveAttribute('aria-hidden', 'true')
    await user.click(sharePlayer)

    let manualLink = await within(inspector).findByRole('textbox', { name: 'Position link for manual copy' })
    let sharedUrl = new URL((manualLink as HTMLInputElement).value)
    expect(sharedUrl.searchParams.get('share')).toBe('position')
    expect(sharedUrl.searchParams.get('region')).toBe('palpagos')
    expect(sharedUrl.searchParams.get('x')).toBe('10')
    expect(sharedUrl.searchParams.get('y')).toBe('20')
    expect(sharedUrl.searchParams.get('zoom')).toBe('8')
    expect([...sharedUrl.searchParams.keys()].sort()).toEqual(['region', 'share', 'x', 'y', 'zoom'])
    expect(sharedUrl.toString()).not.toContain('player-luke')
    expect(sharedUrl.toString()).not.toMatch(/bearer|challenge|claim|subject|token/i)
    expect(within(inspector).getByRole('status')).toHaveTextContent('Automatic copy unavailable. Copy this link:')

    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Waypoints section' }))
    await user.click(await within(explorer).findByRole('button', { name: 'View Small Settlement' }))
    inspector = screen.getByRole('dialog', { name: 'Small Settlement' })
    await user.click(within(inspector).getByRole('button', { name: 'Share position' }))

    manualLink = await within(inspector).findByRole('textbox', { name: 'Position link for manual copy' })
    sharedUrl = new URL((manualLink as HTMLInputElement).value)
    expect(sharedUrl.searchParams.get('x')).toBe('15')
    expect(sharedUrl.searchParams.get('y')).toBe('25')
    expect(sharedUrl.toString()).not.toContain('waypoint-share-test')
  })

  it('opens a shared URL on its encoded region and restores only the camera', async () => {
    const viewportWidth = 1200
    const viewportHeight = 600
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(
      () => new DOMRect(0, 0, viewportWidth, viewportHeight)
    )
    vi.stubGlobal('devicePixelRatio', 1)
    vi.stubGlobal(
      'requestAnimationFrame',
      vi.fn((callback: FrameRequestCallback) => {
        callback(0)
        return 1
      })
    )
    vi.stubGlobal('cancelAnimationFrame', vi.fn())
    window.history.replaceState({}, '', '/?share=position&region=world-tree&x=250&y=350&zoom=12')
    mockAPI((path) =>
      path === '/api/config'
        ? {
            ...(responses[path] as Record<string, unknown>),
            layers: [
              { id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] },
              { id: 'world-tree', name: 'World Tree', bounds: [400, 500, 200, 300] }
            ]
          }
        : responses[path]
    )

    render(<App />)

    expect(await screen.findByRole('application', { name: /World Tree interactive world map/ })).toBeVisible()
    expect(screen.queryByRole('img', { name: /Shared position/ })).not.toBeInTheDocument()
    const scene = document.querySelector<HTMLElement>('.map-scene')
    if (!scene) throw new Error('Expected map scene')
    await waitFor(() => {
      const match = scene.style.transform.match(/^translate\(([-\d.]+)px, ([-\d.]+)px\) scale\(([-\d.]+)\)$/)
      if (!match) throw new Error(`Unexpected map transform: ${scene.style.transform}`)
      const view = { x: Number(match[1]), y: Number(match[2]), scale: Number(match[3]) }
      const fitScale = (viewportHeight - 128) / 8192
      expect(2048 * view.scale + view.x).toBeCloseTo(viewportWidth / 2)
      expect(6144 * view.scale + view.y).toBeCloseTo(viewportHeight / 2)
      expect(view.scale / fitScale).toBeCloseTo(12)
    })
    expect(screen.queryByRole('img', { name: /Shared position/ })).not.toBeInTheDocument()
  })

  it('shows save-game progress when available and omits unknown fields', async () => {
    const lastSeenAt = '2026-07-21T11:32:44.248Z'
    const playerState = responses['/api/players'] as {
      players: Array<Record<string, unknown>>
    }
    mockAPI((path) => {
      if (path !== '/api/players') return responses[path]
      return {
        ...(responses[path] as Record<string, unknown>),
        players: [
          {
            ...playerState.players[0],
            lastSeenAt,
            captureTotal: 4321,
            uniquePalsCaptured: 117,
            paldeckUnlocked: 119,
            arenaRankPoints: 2840,
            fastTravelUnlocked: 73,
            areasDiscovered: 48,
            bossDefeats: 27,
            towerDefeats: 6
          },
          {
            id: 'player-legacy',
            name: 'Legacy',
            level: 12,
            online: false,
            x: 30,
            y: 40,
            map: 'palpagos'
          }
        ]
      }
    })
    const user = userEvent.setup()
    render(<App />)

    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    await user.click(await within(explorer).findByRole('button', { name: 'View Luke · Lv 55' }))

    let inspector = screen.getByRole('dialog')
    const expectedLastSeen = new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short'
    }).format(new Date(lastSeenAt))
    for (const [label, value] of [
      ['Last seen', expectedLastSeen],
      ['Captures', (4321).toLocaleString()],
      ['Species captured', (117).toLocaleString()],
      ['Paldeck unlocked', (119).toLocaleString()],
      ['Arena RP', (2840).toLocaleString()],
      ['Fast-travel points', (73).toLocaleString()],
      ['Areas discovered', (48).toLocaleString()],
      ['Boss clears', (27).toLocaleString()],
      ['Tower clears', (6).toLocaleString()]
    ]) {
      const term = within(inspector).getByText(label)
      expect(term.nextElementSibling).toHaveTextContent(value)
    }

    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Offline Players section' }))
    await user.click(within(explorer).getByRole('button', { name: 'View Legacy · Lv 12' }))

    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByText('Status').nextElementSibling).toHaveTextContent('Offline')
    for (const label of [
      'Last seen',
      'Captures',
      'Species captured',
      'Paldeck unlocked',
      'Arena RP',
      'Fast-travel points',
      'Areas discovered',
      'Boss clears',
      'Tower clears'
    ]) {
      expect(within(inspector).queryByText(label)).not.toBeInTheDocument()
    }
  })

  it('warns when offline player details are temporarily degraded', async () => {
    mockAPI((path) =>
      path === '/api/players'
        ? {
            ...(responses[path] as object),
            saveEnabled: true,
            saveAvailable: true,
            saveStale: false,
            saveLastError: 'resolve-failed'
          }
        : responses[path]
    )
    render(<App />)

    const warning = await screen.findByText(
      'Offline player details are temporarily unavailable. Live players and saved progress remain available.'
    )
    expect(warning).toBeVisible()
    expect(warning).toHaveAttribute('aria-live', 'polite')
  })

  it('keeps fresh filter defaults sparse when additional categories first receive content', async () => {
    const landmarks = [
      {
        id: 'alpha-default',
        kind: 'alpha-pals',
        name: 'Alpha Default',
        x: 11,
        y: 21,
        map: 'palpagos'
      },
      {
        id: 'boss-default',
        kind: 'bosses',
        name: 'Boss Default',
        x: 12,
        y: 22,
        map: 'palpagos'
      }
    ]
    const objects = [
      ...(responses['/api/objects'] as { objects: Array<Record<string, unknown>> }).objects,
      {
        id: 'companion-default',
        kind: 'companions',
        name: 'Companion Default',
        x: 13,
        y: 23,
        map: 'palpagos'
      },
      {
        id: 'wild-default',
        kind: 'wild-pals',
        name: 'Wild Default',
        x: 14,
        y: 24,
        map: 'palpagos'
      },
      {
        id: 'npc-default',
        kind: 'npcs',
        name: 'NPC Default',
        x: 15,
        y: 25,
        map: 'palpagos'
      }
    ]
    mockAPI((path) => {
      if (path === '/api/config') return { ...(responses[path] as Record<string, unknown>), landmarks }
      if (path === '/api/players') {
        const state = responses[path] as { players: Array<Record<string, unknown>> }
        return {
          ...state,
          players: [
            ...state.players,
            { id: 'player-offline', name: 'Offline Player', level: 40, online: false, x: 20, y: 30, map: 'palpagos' }
          ]
        }
      }
      if (path === '/api/objects') {
        return { ...(responses[path] as Record<string, unknown>), total: objects.length, objects }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await waitFor(() => {
      expect(within(explorer).getByRole('checkbox', { name: 'Show Online Players' })).toBeChecked()
      expect(within(explorer).getByRole('checkbox', { name: 'Show Offline Players' })).toBeChecked()
      expect(within(explorer).getByRole('checkbox', { name: 'Show Guilds' })).toBeChecked()
    })
    expect(within(explorer).getByRole('button', { name: 'Collapse Online Players section' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Expand Offline Players section' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Expand Guilds section' })).toBeVisible()

    for (const category of ['Alpha Pals', 'Tower Bosses']) {
      const checkbox = within(explorer).getByRole('checkbox', { name: `Show ${category}` })
      expect(checkbox).not.toBeChecked()
      expect(checkbox).toBeEnabled()
      expect(within(explorer).getByRole('button', { name: `Expand ${category} section` })).toBeVisible()
    }
    expect(screen.queryByRole('button', { name: 'Alpha Default' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Boss Default' })).not.toBeInTheDocument()
    for (const retiredCategory of ['Companion Pals', 'Wild Pals', 'Live NPCs']) {
      expect(within(explorer).queryByText(new RegExp(retiredCategory, 'i'))).not.toBeInTheDocument()
    }
    expect(screen.queryByRole('button', { name: 'Wild Default' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'NPC Default' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Companion Default' })).not.toBeInTheDocument()
  })

  it('warns when the exported landmark catalogue does not match the live server version', async () => {
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          server: { name: 'Test Realm', version: 'v1.0.1.100620' }
        }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).getByText(/World catalogue version mismatch:/)).toHaveTextContent(
      'locations were exported for Palworld 1.0.1.100619, but this server reports v1.0.1.100620'
    )
    expect(within(explorer).getByText(/World catalogue version mismatch:/)).toHaveTextContent('make game-assets')
  })

  it('accepts an exact numeric server release with a leading v', async () => {
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          server: { name: 'Test Realm', version: 'v1.0.1.100619' }
        }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).queryByText(/World catalogue version mismatch:/)).not.toBeInTheDocument()
  })

  it('does not show a catalogue compatibility warning for the fictional demo version', async () => {
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          server: { name: 'Palpagos Live Demo', version: '1.0 demo' }
        }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Palpagos Live Demo' })
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).queryByText(/World catalogue/)).not.toBeInTheDocument()
  })

  it('does not normalize differently formatted version components', async () => {
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          server: { name: 'Test Realm', version: 'v1.0.1.0100619' }
        }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).getByText(/World catalogue version mismatch:/)).toBeVisible()
  })

  it('warns when catalogue compatibility cannot be verified', async () => {
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          server: { name: 'Test Realm', version: 'release-1.0.1.100619+build' }
        }
      }
      return responses[path]
    })
    render(<App />)

    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).getByText(/catalogue compatibility could not be verified/)).toHaveTextContent(
      'server reports an unrecognised version (release-1.0.1.100619+build)'
    )
  })

  it('clears search for relationship navigation and restores a durable close target', async () => {
    const relatedObjects = [
      {
        id: 'base-palbox',
        kind: 'bases',
        name: 'Palbox',
        baseId: 'base-palbox',
        guildKey: 'guild-unnamed',
        x: 30,
        y: 40,
        map: 'palpagos'
      },
      {
        id: 'companion-spark',
        kind: 'companions',
        name: 'Spark',
        detail: 'Sparkit',
        level: 12,
        ownerId: 'player-luke',
        guildKey: 'guild-unnamed',
        x: 11,
        y: 21,
        map: 'palpagos'
      }
    ]
    mockAPI((path) =>
      path === '/api/objects'
        ? { ...(responses[path] as Record<string, unknown>), total: relatedObjects.length, objects: relatedObjects }
        : responses[path]
    )

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const filterControl = within(screen.getByRole('banner')).getByRole('button', { name: 'Map filters' })
    const durableCloseTarget = within(screen.getByRole('banner')).getByRole('button', { name: 'Leaderboards' })
    await user.type(screen.getByRole('searchbox'), 'Luke')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const opener = within(explorer).getByRole('button', { name: 'View Luke · Lv 55' })
    await user.click(opener)
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Luke' })).toHaveFocus())
    expect(filterControl).not.toHaveAttribute('aria-hidden')
    expect(filterControl).not.toHaveAttribute('inert')
    expect(durableCloseTarget).not.toHaveAttribute('aria-hidden')
    expect(durableCloseTarget).not.toHaveAttribute('inert')
    expect(within(screen.getByRole('dialog')).getByText('Unnamed guild')).toBeVisible()
    expect(within(screen.getByRole('dialog')).getByRole('heading', { name: 'Current companion Pals' })).toBeVisible()

    const detailsBody = screen.getByRole('dialog').querySelector<HTMLElement>('[data-details-body]')
    if (!detailsBody) throw new Error('Expected a scrollable details body')
    detailsBody.scrollTop = 100
    opener.remove()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'View guild Unnamed guild' }))

    expect(document.querySelector('#map-search')).toHaveValue('')
    await waitFor(() => expect(detailsBody.scrollTop).toBe(0))
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Unnamed guild' })).toHaveFocus())
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'View guild Pal Spark · Lv 12' }))

    expect(screen.queryByRole('button', { name: 'Spark · Lv 12' })).not.toBeInTheDocument()
    expect(within(explorer).getByRole('button', { name: 'View Spark · Lv 12' })).toBeVisible()
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Spark' })).toHaveFocus())
    expect(within(explorer).getByRole('button', { name: 'Expand Guilds section' })).toBeVisible()

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(durableCloseTarget).toHaveFocus())
  })

  it('shows player, guild, base and Pal relationships in both directions', async () => {
    const relationshipConfig = {
      ...(responses['/api/config'] as Record<string, unknown>),
      layers: [
        { id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] },
        { id: 'world-tree', name: 'World Tree', bounds: [100, 100, -100, -100] }
      ]
    }
    const relationshipPlayers = {
      ...(responses['/api/players'] as Record<string, unknown>),
      players: [
        {
          id: 'player-luke',
          name: 'Luke',
          level: 55,
          x: 10,
          y: 20,
          map: 'palpagos'
        },
        {
          id: 'player-robin',
          name: 'Robin',
          level: 31,
          guildKey: 'guild-builders',
          guildName: 'Builders',
          x: 15,
          y: 25,
          map: 'world-tree'
        }
      ]
    }
    const relationshipObjects = [
      {
        id: 'base-1',
        kind: 'bases',
        name: 'Builders',
        baseId: 'base-1',
        guildKey: 'guild-builders',
        x: 30,
        y: 40,
        map: 'palpagos'
      },
      {
        id: 'base-2',
        kind: 'bases',
        name: 'Builders',
        detail: 'Mountain supply camp',
        baseId: 'base-2',
        guildKey: 'guild-builders',
        x: 50,
        y: 60,
        map: 'world-tree'
      },
      {
        id: 'worker-forge',
        kind: 'workers',
        name: 'Forge',
        detail: 'Anubis',
        level: 44,
        baseId: 'base-2',
        guildKey: 'guild-builders',
        x: 51,
        y: 61,
        map: 'world-tree'
      },
      {
        id: 'companion-spark',
        kind: 'companions',
        name: 'Spark',
        detail: 'Sparkit',
        level: 12,
        ownerId: 'player-luke',
        guildKey: 'guild-builders',
        x: 11,
        y: 21,
        map: 'palpagos'
      },
      {
        id: 'worker-smith',
        kind: 'workers',
        name: 'Smith',
        detail: 'Lamball',
        level: 18,
        baseId: 'base-2',
        guildKey: 'guild-builders',
        x: 52,
        y: 62,
        map: 'world-tree'
      },
      {
        id: 'companion-moss',
        kind: 'companions',
        name: 'Moss',
        detail: 'Lifmunk',
        level: 9,
        ownerId: 'player-robin',
        guildKey: 'guild-builders',
        x: 16,
        y: 26,
        map: 'world-tree'
      }
    ]
    mockAPI((path) => {
      if (path === '/api/config') return relationshipConfig
      if (path === '/api/players') return relationshipPlayers
      if (path === '/api/objects') {
        return {
          ...(responses[path] as Record<string, unknown>),
          total: relationshipObjects.length,
          objects: relationshipObjects
        }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const guildCategory = within(explorer).getByRole('button', { name: 'Expand Guilds section' })
    expect(guildCategory).toBeVisible()
    await user.click(guildCategory)
    const guildOpener = within(explorer).getByRole('button', { name: 'View guild Builders' })
    const guildDisclosure = within(explorer).getByRole('button', { name: 'Expand Builders' })
    expect(guildDisclosure).toHaveAttribute('aria-expanded', 'false')

    const searchbox = within(explorer).getByRole('searchbox')
    await user.type(searchbox, 'Builders')
    await user.click(guildOpener)
    let inspector = screen.getByRole('dialog')
    expect(searchbox).toHaveValue('Builders')
    expect(screen.getByRole('button', { name: 'Map filters' })).toHaveAttribute('aria-expanded', 'true')
    expect(within(inspector).getByText('GUILD DETAILS')).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Builders' })).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Online members' })).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Bases' })).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Pals' })).toBeVisible()
    expect(within(inspector).getAllByRole('button', { name: /View guild base Base [12]/ })).toHaveLength(2)
    expect(within(inspector).getByRole('button', { name: 'View guild member Luke · Lv 55' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View guild member Robin · Lv 31' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View guild Pal Moss · Lv 9' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View guild Pal Forge · Lv 44' })).toBeVisible()

    await user.click(within(inspector).getByRole('button', { name: 'View guild base Base 1' }))
    expect(searchbox).toHaveValue('Builders')
    expect(await screen.findByText('BASE DETAILS')).toBeVisible()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'View guild Builders' }))
    inspector = screen.getByRole('dialog')
    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(guildOpener).toHaveFocus())
    await user.clear(searchbox)
    expect(guildDisclosure).toHaveAttribute('aria-expanded', 'false')
    await user.click(guildDisclosure)
    expect(guildDisclosure).toHaveAttribute('aria-expanded', 'true')

    await user.click(guildOpener)
    inspector = screen.getByRole('dialog')
    await user.click(within(inspector).getByRole('button', { name: 'View guild member Luke · Lv 55' }))
    expect(await screen.findByRole('heading', { name: 'Luke' })).toBeVisible()
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByText('2 online players · 2 bases · 4 Pals')).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View guild Builders' })).toBeVisible()
    expect(within(inspector).queryByRole('heading', { name: 'Online members' })).not.toBeInTheDocument()
    expect(within(inspector).queryByRole('heading', { name: 'Bases' })).not.toBeInTheDocument()

    await user.click(within(inspector).getByRole('button', { name: 'View guild Builders' }))
    expect(await screen.findByText('GUILD DETAILS')).toBeVisible()
    inspector = screen.getByRole('dialog')

    await user.click(within(inspector).getByRole('button', { name: 'View guild Pal Spark · Lv 12' }))
    expect(await screen.findByRole('heading', { name: 'Spark' })).toBeVisible()
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByRole('button', { name: 'View owner Luke · Lv 55' })).toBeVisible()

    await user.click(within(inspector).getByRole('button', { name: 'View guild Builders' }))
    inspector = screen.getByRole('dialog')
    await user.click(within(inspector).getByRole('button', { name: 'View guild base Base 2' }))
    expect(await screen.findByRole('heading', { name: 'Builders' })).toBeVisible()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'World Tree' })).toHaveAttribute('aria-pressed', 'true')
    )
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByText('Description')).toBeVisible()
    expect(within(inspector).getByText('Mountain supply camp')).toBeVisible()
    expect(within(inspector).queryByText('Species')).not.toBeInTheDocument()
    expect(within(inspector).getByRole('button', { name: 'View guild Builders' })).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Assigned Pals' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View assigned Pal Forge · Lv 44' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View assigned Pal Smith · Lv 18' })).toBeVisible()
    expect(within(inspector).queryByText(/closest guild base|closest guild base roster/i)).not.toBeInTheDocument()

    await user.click(within(inspector).getByRole('button', { name: 'View assigned Pal Smith · Lv 18' }))
    expect(await screen.findByRole('heading', { name: 'Smith' })).toBeVisible()
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByRole('button', { name: 'View assigned base Base 2' })).toBeVisible()
    expect(within(inspector).getByRole('heading', { name: 'Other Pals assigned to this base' })).toBeVisible()
    expect(within(inspector).queryByText(/Some Pals are grouped with their closest guild base/)).not.toBeInTheDocument()

    await user.click(within(inspector).getByRole('button', { name: 'View assigned Pal Forge · Lv 44' }))
    expect(await screen.findByRole('heading', { name: 'Forge' })).toBeVisible()
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByRole('button', { name: 'View assigned base Base 2' })).toBeVisible()
    expect(within(inspector).queryByText(/closest base belonging to the same guild/)).not.toBeInTheDocument()

    await user.click(within(inspector).getByRole('button', { name: 'View guild Builders' }))
    inspector = screen.getByRole('dialog')
    await user.click(within(inspector).getByRole('button', { name: 'View guild member Luke · Lv 55' }))
    expect(await screen.findByRole('heading', { name: 'Luke' })).toBeVisible()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Palpagos Islands' })).toHaveAttribute('aria-pressed', 'true')
    )
  })

  it('filters map results inside the filter and reopens search with the slash shortcut', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const filterControl = within(screen.getByRole('banner')).getByRole('button', { name: 'Map filters' })
    const filterPanel = screen.getByRole('complementary', { name: 'Map filters' })
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(filterControl).toHaveClass('header-panel-control', 'pal-selected')
    expect(filterControl).not.toHaveAttribute('aria-hidden')
    expect(filterControl).not.toHaveAttribute('inert')
    expect(filterControl.querySelector('svg')).toHaveClass('tabler-icon-filter')
    const searchbox = within(filterPanel).getByRole('searchbox')
    expect(searchbox).toHaveAttribute('type', 'search')
    expect(searchbox).toHaveAttribute('placeholder', 'Filter map results…')
    expect(screen.queryByText('/')).not.toBeInTheDocument()
    await user.type(searchbox, 'missing')
    expect(filterControl).toHaveAccessibleDescription('Current search: missing')
    expect(screen.getAllByRole('button', { name: 'Clear search' })).toHaveLength(1)
    expect(screen.getByText('No online players or companion Pals match “missing”.')).toBeInTheDocument()

    await user.click(filterControl)
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument())
    expect(document.querySelector('#map-filter-panel')).toBe(filterPanel)
    expect(filterPanel).toHaveAttribute('aria-hidden', 'true')
    expect(filterPanel).toHaveAttribute('inert')
    expect(screen.queryByRole('searchbox')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Map filters' })).toBe(filterControl)
    expect(filterControl).toHaveFocus()
    expect(filterControl).toHaveAttribute('aria-expanded', 'false')
    expect(filterControl).not.toHaveClass('pal-selected')
    expect(filterControl).not.toHaveAttribute('aria-hidden')
    expect(filterControl).not.toHaveAttribute('inert')
    expect(document.querySelector('#map-search')).toHaveValue('missing')

    await user.click(filterControl)
    expect(screen.getByRole('complementary', { name: 'Map filters' })).toBe(filterPanel)
    expect(screen.getByRole('button', { name: 'Map filters' })).toBe(filterControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(filterControl).toHaveClass('pal-selected')
    expect(screen.getByRole('searchbox')).toHaveValue('missing')

    await user.click(filterControl)
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument())
    expect(filterControl).toHaveAttribute('aria-expanded', 'false')
    expect(filterControl).not.toHaveClass('pal-selected')

    await user.keyboard('/')
    expect(screen.getByRole('complementary', { name: 'Map filters' })).toBe(filterPanel)
    expect(screen.getByRole('searchbox')).toHaveValue('missing')
    await waitFor(() => expect(screen.getByRole('searchbox')).toHaveFocus())
    expect(screen.getByRole('button', { name: 'Map filters' })).toBe(filterControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(filterControl).toHaveClass('pal-selected')

    await user.keyboard('{Escape}')
    expect(screen.getByRole('searchbox')).toHaveValue('')
    expect(screen.getByRole('searchbox')).toHaveFocus()
    expect(screen.getByRole('searchbox')).not.toHaveAttribute('aria-describedby')
    expect(within(filterPanel).queryByRole('status', { name: 'Search results' })).not.toBeInTheDocument()
    expect(filterControl).not.toHaveAttribute('aria-describedby')
    expect(within(filterPanel).getByRole('button', { name: 'Expand NPC Locations section' })).toBeVisible()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(filterControl).toHaveFocus())
    expect(filterControl).toHaveAttribute('aria-expanded', 'false')
    expect(filterControl).not.toHaveClass('pal-selected')
  })

  it('keeps the two mobile panel controls from opening overlapping sheets', async () => {
    let viewportWidth = 390
    vi.spyOn(window, 'innerWidth', 'get').mockImplementation(() => viewportWidth)
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const statusBar = screen.getByRole('banner')
    const filterControl = within(statusBar).getByRole('button', { name: 'Map filters' })
    const leaderboardControl = within(statusBar).getByRole('button', { name: 'Leaderboards' })
    const expectMobilePanelShell = (panel: HTMLElement, label: string) => {
      expect(panel).toHaveAttribute('data-map-panel-shell')
      expect(panel).toHaveAttribute('data-map-panel-mobile-size', 'fixed')
      expect(panel).toHaveAttribute('data-map-panel-mobile-state', 'compact')
      expect(panel).toHaveClass(
        'pal-glass-panel',
        'map-panel-mobile-sheet',
        'max-sm:inset-x-0',
        'max-sm:bottom-0',
        'max-sm:w-auto',
        'max-sm:border-x-0',
        'max-sm:border-b-0'
      )
      const resizeHandle = within(panel).getByRole('button', { name: `Use expanded ${label} panel` })
      expect(resizeHandle).toHaveAttribute('data-map-panel-resize-handle')
      expect(resizeHandle).toHaveAttribute('aria-controls', panel.id)
      expect(resizeHandle).toHaveAttribute('aria-pressed', 'false')
      expect(resizeHandle).toHaveClass('touch-none', 'sm:hidden')
      return resizeHandle
    }
    expect(filterControl).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'false')

    await user.click(filterControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    const filterPanel = screen.getByRole('complementary', { name: 'Map filters' })
    expect(filterPanel).toBeVisible()
    const filterResizeHandle = expectMobilePanelShell(filterPanel, 'map filters')
    await user.click(filterResizeHandle)
    expect(filterPanel).toHaveAttribute('data-map-panel-mobile-state', 'expanded')
    expect(filterResizeHandle).toHaveAttribute('aria-pressed', 'true')
    expect(filterResizeHandle).toHaveFocus()
    await user.keyboard('{Enter}')
    expect(filterPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')
    await user.keyboard(' ')
    expect(filterPanel).toHaveAttribute('data-map-panel-mobile-state', 'expanded')
    fireEvent.keyDown(filterResizeHandle, { key: 'ArrowDown' })
    expect(filterPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')
    expect(filterResizeHandle).toHaveAttribute('aria-pressed', 'false')
    fireEvent.keyDown(filterResizeHandle, { key: 'End' })
    expect(filterPanel).toHaveAttribute('data-map-panel-mobile-state', 'expanded')

    await user.click(leaderboardControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'true')
    expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument()
    const leaderboardPanel = screen.getByRole('dialog')
    expect(leaderboardPanel).toHaveAttribute('id', 'leaderboard-panel')
    const leaderboardResizeHandle = expectMobilePanelShell(leaderboardPanel, 'leaderboards')

    fireEvent.pointerDown(leaderboardResizeHandle, {
      button: 0,
      clientY: 500,
      isPrimary: true,
      pointerId: 1,
      pointerType: 'touch'
    })
    fireEvent.pointerMove(leaderboardResizeHandle, {
      clientY: 470,
      isPrimary: true,
      pointerId: 1,
      pointerType: 'touch'
    })
    fireEvent.pointerUp(leaderboardResizeHandle, {
      clientY: 470,
      isPrimary: true,
      pointerId: 1,
      pointerType: 'touch'
    })
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')

    fireEvent.pointerDown(leaderboardResizeHandle, {
      button: 0,
      clientY: 500,
      isPrimary: true,
      pointerId: 2,
      pointerType: 'touch'
    })
    fireEvent.pointerMove(leaderboardResizeHandle, {
      clientY: 430,
      isPrimary: true,
      pointerId: 2,
      pointerType: 'touch'
    })
    fireEvent.pointerCancel(leaderboardResizeHandle, {
      clientY: 430,
      isPrimary: true,
      pointerId: 2,
      pointerType: 'touch'
    })
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')

    fireEvent.pointerDown(leaderboardResizeHandle, {
      button: 0,
      clientY: 500,
      isPrimary: true,
      pointerId: 3,
      pointerType: 'touch'
    })
    fireEvent.pointerMove(leaderboardResizeHandle, {
      clientY: 430,
      isPrimary: true,
      pointerId: 3,
      pointerType: 'touch'
    })
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-dragging', 'true')
    fireEvent.pointerUp(leaderboardResizeHandle, {
      clientY: 430,
      isPrimary: true,
      pointerId: 3,
      pointerType: 'touch'
    })
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-mobile-state', 'expanded')
    expect(leaderboardPanel).not.toHaveAttribute('data-map-panel-dragging')
    expect(leaderboardResizeHandle).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(leaderboardResizeHandle)
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-mobile-state', 'expanded')

    fireEvent.pointerDown(leaderboardResizeHandle, {
      button: 0,
      clientY: 430,
      isPrimary: true,
      pointerId: 4,
      pointerType: 'touch'
    })
    fireEvent.pointerMove(leaderboardResizeHandle, {
      clientY: 500,
      isPrimary: true,
      pointerId: 4,
      pointerType: 'touch'
    })
    fireEvent.pointerUp(leaderboardResizeHandle, {
      clientY: 500,
      isPrimary: true,
      pointerId: 4,
      pointerType: 'touch'
    })
    expect(leaderboardPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')
    expect(leaderboardResizeHandle).toHaveAttribute('aria-pressed', 'false')

    await user.click(filterControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    const reopenedFilterPanel = screen.getByRole('complementary', { name: 'Map filters' })
    expect(reopenedFilterPanel).toBeVisible()
    expect(reopenedFilterPanel).toHaveAttribute('data-map-panel-mobile-state', 'compact')

    viewportWidth = 1024
    await user.click(leaderboardControl)
    expect(filterControl).toHaveAttribute('aria-expanded', 'true')
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'true')
    const leaderboardTitle = screen.getByRole('heading', { name: 'Leaderboards' })
    await waitFor(() => expect(leaderboardTitle).toHaveFocus())

    viewportWidth = 390
    window.dispatchEvent(new Event('resize'))
    await waitFor(() => expect(filterControl).toHaveAttribute('aria-expanded', 'false'))
    expect(leaderboardControl).toHaveAttribute('aria-expanded', 'true')
    expect(leaderboardTitle).toHaveFocus()

    const expandedLeaderboard = screen.getByRole('dialog')
    const expandedLeaderboardHandle = within(expandedLeaderboard).getByRole('button', {
      name: 'Use expanded leaderboards panel'
    })
    fireEvent.keyDown(expandedLeaderboardHandle, { key: 'End' })
    expect(expandedLeaderboard).toHaveAttribute('data-map-panel-mobile-state', 'expanded')
    await user.click(
      within(expandedLeaderboard).getByRole('button', {
        name: 'View leaderboard rank 1: Luke · Lv 55, Online'
      })
    )
    const playerInspector = screen.getByRole('dialog')
    expect(playerInspector).toHaveAttribute('data-map-panel-mobile-size', 'content')
    expect(playerInspector).not.toHaveAttribute('data-map-panel-mobile-state')
    expect(playerInspector.querySelector('[data-map-panel-resize-handle]')).not.toBeInTheDocument()
  })

  it('restores saved filter categories and the active map layer', async () => {
    mockAPI((path) =>
      path === '/api/config'
        ? {
            ...(responses[path] as Record<string, unknown>),
            layers: [
              { id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] },
              { id: 'world-tree', name: 'World Tree', bounds: [100, 100, -100, -100] }
            ]
          }
        : responses[path]
    )
    const user = userEvent.setup()
    const firstRender = render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Online Players' }))
    await user.click(within(explorer).getByRole('button', { name: 'World Tree' }))
    firstRender.unmount()

    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const restoredExplorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(restoredExplorer).getByRole('button', { name: 'World Tree' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(restoredExplorer).getByRole('checkbox', { name: 'Show Online Players' })).not.toBeChecked()
  })

  it('unchecks every filter and keeps later data hidden across reloads', async () => {
    let objectRequests = 0
    let includeEffigy = false
    const base = (responses['/api/objects'] as { objects: Array<Record<string, unknown>> }).objects[0]
    const effigy = {
      id: 'late-effigy',
      kind: 'effigies',
      name: 'Late Effigy',
      x: 15,
      y: 25,
      map: 'palpagos'
    }
    mockAPI((path) => {
      if (path === '/api/config') {
        return { ...(responses[path] as Record<string, unknown>), worldPollIntervalMs: 80 }
      }
      if (path === '/api/objects') {
        objectRequests++
        const objects = includeEffigy ? [base, effigy] : [base]
        return { ...(responses[path] as Record<string, unknown>), objects, total: objects.length }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    const firstRender = render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    let explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await waitFor(() => expect(within(explorer).getByRole('checkbox', { name: 'Show Guilds' })).toBeChecked())

    const uncheckAll = within(explorer).getByRole('button', { name: 'Hide all' })
    await user.click(uncheckAll)
    for (const checkbox of within(explorer).getAllByRole('checkbox')) expect(checkbox).not.toBeChecked()
    expect(uncheckAll).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Luke · Lv 55' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Home' })).not.toBeInTheDocument()

    const requestsBeforeEffigy = objectRequests
    includeEffigy = true
    await waitFor(() => expect(objectRequests).toBeGreaterThan(requestsBeforeEffigy))
    const effigyFilter = within(explorer).getByRole('checkbox', { name: 'Show Pal Effigies' })
    expect(effigyFilter).toBeEnabled()
    expect(effigyFilter).not.toBeChecked()
    expect(screen.queryByRole('button', { name: 'Late Effigy' })).not.toBeInTheDocument()

    firstRender.unmount()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await waitFor(() => expect(within(explorer).getByRole('checkbox', { name: 'Show Pal Effigies' })).toBeEnabled())
    for (const checkbox of within(explorer).getAllByRole('checkbox')) expect(checkbox).not.toBeChecked()
  })

  it('can show a child item after all filters or its category are unchecked', async () => {
    mockAPI((path) => {
      if (path !== '/api/players') return responses[path]
      const state = responses[path] as (typeof responses)['/api/players'] & { players: Array<Record<string, unknown>> }
      return {
        ...state,
        players: [
          ...state.players,
          { id: 'player-anne', name: 'Anne', level: 20, online: true, x: 30, y: 40, map: 'palpagos' }
        ]
      }
    })
    const user = userEvent.setup()
    const firstRender = render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const playerVisibility = within(explorer).getByRole('checkbox', { name: 'Show Luke · Lv 55' })
    const siblingVisibility = within(explorer).getByRole('checkbox', { name: 'Show Anne · Lv 20' })
    const categoryVisibility = within(explorer).getByRole('checkbox', { name: 'Show Online Players' })
    const checkAll = within(explorer).getByRole('button', { name: 'Show all' })

    await user.click(within(explorer).getByRole('button', { name: 'Hide all' }))
    expect(checkAll).toBeEnabled()
    expect(playerVisibility).toBeEnabled()
    expect(playerVisibility).not.toBeChecked()

    await user.click(playerVisibility)
    expect(playerVisibility).toBeChecked()
    expect(siblingVisibility).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Luke · Lv 55' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Anne · Lv 20' })).not.toBeInTheDocument()

    await user.click(categoryVisibility)
    expect(categoryVisibility).toBeChecked()
    await user.click(categoryVisibility)
    expect(playerVisibility).toBeEnabled()
    expect(playerVisibility).not.toBeChecked()

    await user.click(playerVisibility)
    expect(playerVisibility).toBeChecked()
    expect(siblingVisibility).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Luke · Lv 55' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Anne · Lv 20' })).not.toBeInTheDocument()

    await user.click(checkAll)
    expect(checkAll).toBeDisabled()
    expect(categoryVisibility).toBeChecked()
    expect(playerVisibility).toBeChecked()
    expect(siblingVisibility).toBeChecked()
    expect(screen.getByRole('button', { name: 'Luke · Lv 55' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Anne · Lv 20' })).toBeVisible()

    const saved = JSON.parse(window.localStorage.getItem('palworld-live-map.filters.v1') || '{}') as {
      enabledKinds?: string[]
      enabledPlayerStatuses?: string[]
      hiddenIds?: string[]
    }
    expect(saved.enabledPlayerStatuses).toEqual(expect.arrayContaining(['online', 'offline']))
    expect(saved.hiddenIds).toEqual([])

    firstRender.unmount()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const restoredExplorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(restoredExplorer).getByRole('button', { name: 'Show all' })).toBeDisabled()
    expect(within(restoredExplorer).getByRole('checkbox', { name: 'Show Online Players' })).toBeChecked()
    expect(within(restoredExplorer).getByRole('checkbox', { name: 'Show Luke · Lv 55' })).toBeChecked()
    expect(within(restoredExplorer).getByRole('checkbox', { name: 'Show Anne · Lv 20' })).toBeChecked()
  })

  it('finds online players by guild name in the explorer and on the map', async () => {
    mockAPI((path) => {
      if (path !== '/api/players') return responses[path]
      const state = responses[path] as Record<string, unknown> & { players: Array<Record<string, unknown>> }
      return {
        ...state,
        players: state.players.map((player) => ({
          ...player,
          guildKey: 'guild-builders',
          guildName: 'Builders'
        }))
      }
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    await user.type(screen.getByRole('searchbox'), 'Builders')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const explorerPlayer = within(explorer).getByRole('button', { name: 'View Luke · Lv 55' })
    const mapPlayer = screen.getByRole('button', { name: 'Luke · Lv 55' })
    const explorerGlyph = explorerPlayer.querySelector('[data-marker-kind="players"]')
    const mapGlyph = mapPlayer.querySelector('[data-marker-kind="players"]')

    expect(explorerPlayer).toBeVisible()
    expect(within(explorer).getByText('Lv 55 · Builders')).toBeVisible()
    expect(mapPlayer).toBeInTheDocument()
    expect(explorerGlyph).toBeInTheDocument()
    expect(mapGlyph).toBeInTheDocument()
    expect(mapGlyph?.getAttribute('class')).toBe(explorerGlyph?.getAttribute('class'))
  })

  it('separates online and offline player filters and ranks players', async () => {
    const roster = [
      {
        id: 'player-zoe',
        name: 'Zoe',
        level: 60,
        guildKey: 'guild-save',
        guildName: 'Save Crew',
        online: false,
        x: 25,
        y: 35,
        map: 'palpagos'
      },
      {
        id: 'player-bob',
        name: 'Bob',
        level: 50,
        guildKey: 'guild-save',
        guildName: 'Save Crew',
        online: true,
        x: 20,
        y: 30,
        map: 'palpagos'
      },
      {
        id: 'player-alice',
        name: 'Alice',
        level: 50,
        guildKey: 'guild-save',
        guildName: 'Save Crew',
        online: false,
        x: 15,
        y: 25,
        map: 'palpagos'
      }
    ]
    mockAPI((path) => {
      if (path === '/api/players') {
        return {
          ...(responses[path] as Record<string, unknown>),
          metrics: {
            ...(responses[path] as { metrics: Record<string, unknown> }).metrics,
            currentPlayers: 1
          },
          players: roster
        }
      }
      if (path === '/api/objects') {
        return { ...(responses[path] as Record<string, unknown>), available: true, total: 0, objects: [] }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).queryByText(/leaderboard/i)).not.toBeInTheDocument()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Online Players' })).toBeChecked()
    const offlineVisibility = within(explorer).getByRole('checkbox', { name: 'Show Offline Players' })
    expect(offlineVisibility).toBeChecked()
    const offlineCategory = within(explorer).getByRole('button', { name: 'Expand Offline Players section' })
    expect(offlineCategory.querySelector('[data-marker-kind="players"]')).toHaveAttribute(
      'data-player-status',
      'offline'
    )
    await user.click(offlineCategory)
    const offlinePlayer = within(explorer).getByRole('button', { name: 'View Zoe · Lv 60' })
    expect(within(explorer).getByRole('button', { name: 'View Alice · Lv 50' })).toBeVisible()
    const onlinePlayer = within(explorer).getByRole('button', { name: 'View Bob · Lv 50' })
    expect(onlinePlayer.querySelector('[data-marker-kind="players"]')).toHaveAttribute('data-player-status', 'online')
    expect(offlinePlayer.querySelector('[data-marker-kind="players"]')).toHaveAttribute('data-player-status', 'offline')
    expect(
      within(explorer)
        .getByRole('button', { name: 'Collapse Online Players section' })
        .querySelector('[data-marker-kind="players"]')
    ).toHaveAttribute('data-player-status', 'online')
    expect(within(explorer).queryByRole('button', { name: 'View guild Save Crew' })).not.toBeInTheDocument()

    await user.click(offlineVisibility)
    expect(screen.queryByRole('button', { name: 'Zoe · Lv 60' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Bob · Lv 50' })).toBeInTheDocument()
    await user.click(offlineVisibility)
    expect(screen.getByRole('button', { name: 'Zoe · Lv 60' })).toBeInTheDocument()

    const statusBar = screen.getByRole('banner')
    const leaderboardOpener = within(statusBar).getByRole('button', { name: 'Leaderboards' })
    const projectLinks = screen.getByRole('navigation', { name: 'Project links' })
    expect(within(projectLinks).queryByRole('button', { name: 'Leaderboards' })).not.toBeInTheDocument()
    expect(within(screen.getByRole('main')).queryByRole('button', { name: 'Leaderboards' })).not.toBeInTheDocument()
    expect(leaderboardOpener).toHaveClass('header-panel-control')
    expect(leaderboardOpener).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardOpener).not.toHaveClass('pal-selected')
    expect(leaderboardOpener).not.toHaveAttribute('aria-hidden')
    expect(leaderboardOpener).not.toHaveAttribute('inert')
    expect(leaderboardOpener.querySelector('svg')).toHaveClass('tabler-icon-trophy')
    await user.click(leaderboardOpener)
    expect(leaderboardOpener).toHaveAttribute('aria-expanded', 'true')
    expect(leaderboardOpener).toHaveClass('pal-selected')
    expect(leaderboardOpener).not.toHaveAttribute('aria-hidden')
    expect(leaderboardOpener).not.toHaveAttribute('inert')
    expect(screen.getByRole('button', { name: 'Leaderboards' })).toBe(leaderboardOpener)
    expect(projectLinks).toBeVisible()
    expect(projectLinks).toHaveAttribute('aria-hidden', 'false')
    expect(screen.getByRole('dialog')).toBeInTheDocument()

    await user.click(leaderboardOpener)
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'Leaderboards' })).toBe(leaderboardOpener)
    expect(leaderboardOpener).toHaveFocus()
    expect(leaderboardOpener).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardOpener).not.toHaveClass('pal-selected')

    await user.click(leaderboardOpener)
    expect(screen.getByRole('button', { name: 'Leaderboards' })).toBe(leaderboardOpener)
    expect(leaderboardOpener).toHaveAttribute('aria-expanded', 'true')
    expect(leaderboardOpener).toHaveClass('pal-selected')
    const leaderboard = screen.getByRole('dialog')
    expect(leaderboard).toHaveClass('top-[78px]', 'bottom-4', 'w-[350px]')
    expect(explorer).toHaveClass('top-[78px]', 'bottom-4', 'w-[350px]')
    expect(leaderboard.querySelector('header[data-map-panel-header]')).toHaveClass('min-h-[78px]')
    expect(explorer.querySelector('[data-map-panel-header]')).toHaveClass('min-h-[78px]')
    expect(within(leaderboard).getByRole('heading', { name: 'Leaderboards' })).toBeVisible()
    expect(within(leaderboard).getByRole('heading', { name: 'Player levels' })).toBeVisible()
    const leaderboardPicker = within(leaderboard).getByRole('button', { name: 'Leaderboard type Player levels' })
    expect(leaderboardPicker).toHaveTextContent('Player levels')
    expect(leaderboardPicker).toHaveAttribute('aria-expanded', 'false')
    await user.click(leaderboardPicker)
    let leaderboardTypes = within(leaderboard).getByRole('listbox', { name: 'Leaderboard types' })
    expect(within(leaderboardTypes).getAllByRole('option')).toHaveLength(8)
    await user.click(within(leaderboardTypes).getByRole('option', { name: 'Paldeck discoveries' }))
    expect(within(leaderboard).getByRole('heading', { name: 'Paldeck discoveries' })).toBeVisible()
    expect(leaderboardPicker).toHaveAccessibleName('Leaderboard type Paldeck discoveries')
    expect(leaderboardPicker).toHaveFocus()
    await user.click(leaderboardPicker)
    leaderboardTypes = within(leaderboard).getByRole('listbox', { name: 'Leaderboard types' })
    expect(leaderboardPicker).toHaveAttribute('aria-controls', leaderboardTypes.id)
    const selectedPaldeckOption = within(leaderboardTypes).getByRole('option', { name: 'Paldeck discoveries' })
    expect(selectedPaldeckOption).toHaveAttribute('aria-selected', 'true')
    await waitFor(() => expect(selectedPaldeckOption).toHaveFocus())
    await user.keyboard('{End}')
    await waitFor(() => expect(within(leaderboardTypes).getByRole('option', { name: 'Tower clears' })).toHaveFocus())
    await user.keyboard('{Home}')
    await waitFor(() => expect(within(leaderboardTypes).getByRole('option', { name: 'Player levels' })).toHaveFocus())
    await user.keyboard('{Escape}')
    expect(within(leaderboard).queryByRole('listbox', { name: 'Leaderboard types' })).not.toBeInTheDocument()
    await waitFor(() => expect(leaderboardPicker).toHaveFocus())
    expect(leaderboard).toBeVisible()
    await user.click(leaderboardPicker)
    leaderboardTypes = within(leaderboard).getByRole('listbox', { name: 'Leaderboard types' })
    fireEvent.pointerDown(within(leaderboard).getByRole('heading', { name: 'Paldeck discoveries' }))
    expect(within(leaderboard).queryByRole('listbox', { name: 'Leaderboard types' })).not.toBeInTheDocument()
    await user.click(leaderboardPicker)
    leaderboardTypes = within(leaderboard).getByRole('listbox', { name: 'Leaderboard types' })
    await user.click(within(leaderboardTypes).getByRole('option', { name: 'Player levels' }))
    expect(
      within(leaderboard).getByRole('button', { name: 'View leaderboard rank 1: Zoe · Lv 60, Offline' })
    ).toBeVisible()
    expect(
      within(leaderboard).getByRole('button', { name: 'View leaderboard rank 2: Alice · Lv 50, Offline' })
    ).toBeVisible()
    expect(
      within(leaderboard).getByRole('button', { name: 'View leaderboard rank 3: Bob · Lv 50, Online' })
    ).toBeVisible()

    const offlineMarker = screen.getByRole('button', { name: 'Zoe · Lv 60' })
    expect(offlineMarker.querySelector('[data-marker-kind="players"]')).toHaveAttribute('data-player-status', 'offline')

    await user.click(within(leaderboard).getByRole('button', { name: 'View leaderboard rank 1: Zoe · Lv 60, Offline' }))
    const playerInspector = screen.getByRole('dialog')
    expect(leaderboardOpener).toHaveAttribute('aria-expanded', 'false')
    expect(leaderboardOpener).not.toHaveClass('pal-selected')
    expect(leaderboardOpener).not.toHaveAttribute('aria-hidden')
    expect(leaderboardOpener).not.toHaveAttribute('inert')
    expect(within(playerInspector).getByRole('heading', { name: 'Zoe' })).toBeVisible()
    const status = within(playerInspector).getByText('Status')
    expect(status.nextElementSibling).toHaveTextContent('Offline')

    await user.click(within(playerInspector).getByRole('button', { name: 'View guild Save Crew' }))

    const inspector = screen.getByRole('dialog')
    const members = within(inspector).getByText('Members')
    const onlineMembers = within(inspector).getByText('Online members', { selector: 'dt' })
    expect(members.nextElementSibling).toHaveTextContent('3')
    expect(onlineMembers.nextElementSibling).toHaveTextContent('1')
    expect(within(inspector).getByRole('heading', { name: 'Offline members' })).toBeVisible()
    expect(within(inspector).getByRole('button', { name: 'View guild member Zoe · Lv 60' })).toBeVisible()

    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(leaderboardOpener).toHaveFocus())
  })

  it('merges config landmarks into separate Alpha Pal and Tower Boss categories', async () => {
    const landmarks = [
      {
        id: 'alpha-penking',
        kind: 'alpha-pals',
        name: 'Penking',
        detail: 'Penking',
        level: 15,
        x: 12,
        y: 22,
        map: 'palpagos'
      },
      {
        id: 'boss-zoe-grizzbolt',
        kind: 'bosses',
        name: 'Zoe & Grizzbolt',
        detail: 'Rayne Syndicate Tower',
        level: 10,
        x: 32,
        y: 42,
        map: 'palpagos'
      }
    ]
    mockAPI((path) =>
      path === '/api/config'
        ? { ...(responses[path] as Record<string, unknown>), worldDataEnabled: false, landmarks }
        : responses[path]
    )

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })

    expect(within(explorer).getByText('Alpha Pals (1)')).toBeVisible()
    expect(within(explorer).getByText('Tower Bosses (1)')).toBeVisible()
    // Fresh preferences keep static categories available but opt-in.
    expect(within(explorer).getByRole('button', { name: 'Expand Alpha Pals section' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Expand Tower Bosses section' })).toBeVisible()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Alpha Pals' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Tower Bosses' })).not.toBeChecked()
    expect(screen.queryByRole('button', { name: 'Penking · Lv 15' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).not.toBeInTheDocument()

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Alpha Pals' }))
    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Tower Bosses' }))
    expect(screen.getByRole('button', { name: 'Penking · Lv 15' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).toBeInTheDocument()

    await user.click(within(explorer).getByRole('button', { name: 'Expand Alpha Pals section' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Tower Bosses section' }))
    expect(
      screen.getByRole('button', { name: 'Penking · Lv 15' }).querySelector('[data-marker-kind="alpha-pals"]')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' }).querySelector('[data-marker-kind="bosses"]')
    ).toBeInTheDocument()

    await user.type(screen.getByRole('searchbox'), 'tower bosses')
    expect(screen.queryByRole('button', { name: 'Penking · Lv 15' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Clear search' }))

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Alpha Pals' }))
    expect(screen.queryByRole('button', { name: 'Penking · Lv 15' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).toBeInTheDocument()

    await user.click(within(explorer).getByRole('button', { name: 'View Zoe & Grizzbolt · Lv 10' }))
    const inspector = screen.getByRole('dialog')
    expect(within(inspector).getByText('TOWER BOSS DETAILS')).toBeVisible()
    expect(within(inspector).getByText('Encounter')).toBeVisible()
    expect(within(inspector).getByText('Rayne Syndicate Tower')).toBeVisible()
  })

  it('loads the world catalogue, merges legacy landmarks, and suppresses duplicate live NPCs', async () => {
    const legacyLandmarks = [
      {
        id: 'legacy-alpha',
        kind: 'alpha-pals',
        name: 'Legacy Alpha',
        x: 10,
        y: 10,
        map: 'palpagos'
      },
      {
        id: 'shared-bounty',
        kind: 'bounties',
        name: 'Stale Bounty',
        level: 1,
        x: 11,
        y: 11,
        map: 'palpagos'
      }
    ]
    const catalogueLocations = [
      {
        id: 'shared-bounty',
        kind: 'bounties',
        name: 'Pinch',
        detail: 'PIDF Bounty',
        level: 57,
        x: 12,
        y: 12,
        map: 'palpagos'
      },
      {
        id: 'oil-rig',
        kind: 'oil-rigs',
        name: 'Rayne Syndicate Oil Rig',
        level: 55,
        x: 13,
        y: 13,
        map: 'palpagos'
      },
      {
        id: 'watchtower',
        kind: 'watchtowers',
        name: 'Verdant Stream Watchtower',
        x: 14,
        y: 14,
        map: 'palpagos'
      },
      {
        id: 'waypoint',
        kind: 'waypoints',
        name: 'Small Settlement',
        x: 15,
        y: 15,
        map: 'palpagos'
      },
      {
        id: 'dungeon',
        kind: 'dungeon-entrances',
        name: 'Dungeon Entrance',
        detail: 'Grassland',
        x: 16,
        y: 16,
        map: 'palpagos'
      },
      {
        id: 'effigy',
        kind: 'effigies',
        name: 'Lifmunk Effigy',
        x: 17,
        y: 17,
        map: 'palpagos'
      },
      {
        id: 'journal',
        kind: 'journals',
        name: "Castaway's Journal",
        x: 18,
        y: 18,
        map: 'palpagos'
      },
      {
        id: 'shrine-pickup',
        kind: 'ancient-shrine-pickups',
        name: 'Ancient Shrine Pickup',
        x: 19,
        y: 19,
        map: 'palpagos'
      },
      {
        id: 'static-npc',
        kind: 'npc-locations',
        name: 'Static Merchant',
        detail: 'Trader',
        x: 20,
        y: 20,
        map: 'palpagos'
      }
    ]
    const liveNpc = {
      id: 'live-npc',
      kind: 'npcs',
      name: 'Roaming Scout',
      detail: 'Syndicate',
      x: 21,
      y: 21,
      map: 'palpagos'
    }
    mockAPI((path) => {
      if (path === '/api/config') {
        return { ...(responses[path] as Record<string, unknown>), landmarks: legacyLandmarks }
      }
      if (path === '/assets/test-world-catalogue.json') {
        return {
          ...(responses[path] as Record<string, unknown>),
          locations: catalogueLocations
        }
      }
      if (path === '/api/objects') {
        return { ...(responses[path] as Record<string, unknown>), objects: [liveNpc], total: 1 }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    expect(fetch).toHaveBeenCalledWith(
      '/assets/test-world-catalogue.json',
      expect.objectContaining({ cache: 'force-cache' })
    )

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    for (const title of [
      'Bounties',
      'Oil Rigs',
      'Watchtowers',
      'Waypoints',
      'Dungeon Entrances',
      'Pal Effigies',
      'Journals',
      'Ancient Shrine Pickups',
      'NPC Locations'
    ]) {
      expect(within(explorer).getByText(`${title} (1)`)).toBeVisible()
    }
    expect(within(explorer).queryByText(/Live NPCs/)).not.toBeInTheDocument()
    await user.click(within(explorer).getByRole('button', { name: 'Expand Alpha Pals section' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Bounties section' }))
    expect(within(explorer).getByRole('button', { name: 'View Legacy Alpha' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Pinch · Lv 57' })).toBeVisible()
    expect(within(explorer).queryByRole('button', { name: 'View Stale Bounty · Lv 1' })).not.toBeInTheDocument()

    await user.click(within(explorer).getByRole('button', { name: 'Expand NPC Locations section' }))
    expect(
      within(explorer)
        .getByRole('button', { name: 'View Static Merchant' })
        .querySelector('[data-marker-kind="npc-locations"]')
    ).toBeInTheDocument()
    expect(within(explorer).queryByRole('button', { name: 'View Roaming Scout' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Roaming Scout' })).not.toBeInTheDocument()

    await user.type(screen.getByRole('searchbox'), 'static npc')
    expect(within(explorer).getByRole('button', { name: 'View Static Merchant' })).toBeVisible()
    expect(within(explorer).queryByRole('button', { name: 'View Roaming Scout' })).not.toBeInTheDocument()
  })

  it('keeps a durable manual landmark checklist and can show only remaining landmarks', async () => {
    const catalogueLocations = [
      {
        id: 'effigy-complete',
        kind: 'effigies',
        name: 'First Effigy',
        x: 10,
        y: 10,
        map: 'palpagos'
      },
      {
        id: 'effigy-remaining',
        kind: 'effigies',
        name: 'Second Effigy',
        x: 20,
        y: 20,
        map: 'palpagos'
      }
    ]
    mockAPI((path) =>
      path === '/assets/test-world-catalogue.json'
        ? { ...(responses[path] as object), locations: catalogueLocations }
        : responses[path]
    )

    const user = userEvent.setup()
    const view = render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(screen.getByRole('button', { name: 'My Progress' }))
    let progressPanel = screen.getByRole('complementary', { name: 'My Progress' })
    expect(within(progressPanel).getByRole('progressbar', { name: 'Palpagos Islands completion' })).toHaveAttribute(
      'aria-valuenow',
      '0'
    )
    expect(within(progressPanel).getByText('My checklist')).toBeVisible()
    await user.click(within(progressPanel).getByRole('button', { name: 'Close My Progress' }))

    const effigyVisibility = within(explorer).getByRole('checkbox', { name: 'Show Pal Effigies' })
    await user.click(effigyVisibility)
    await user.click(within(explorer).getByRole('button', { name: 'Expand Pal Effigies section' }))
    await user.click(within(explorer).getByRole('button', { name: 'View First Effigy' }))

    const inspector = screen.getByRole('dialog')
    const completion = within(inspector).getByRole('checkbox', {
      name: 'Mark First Effigy complete in My checklist'
    })
    expect(completion).not.toBeChecked()
    expect(completion).toHaveAccessibleDescription(
      'Manual marks stay in this browser. Save-confirmed progress appears only while connected.'
    )
    await user.click(completion)

    expect(completion).toBeChecked()
    expect(within(inspector).getByText('Manual mark saved')).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View First Effigy, manual completion' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'First Effigy · Manual completion' })).toHaveAttribute(
      'data-completion-source',
      'manual'
    )

    await user.click(screen.getByRole('button', { name: 'My Progress' }))
    progressPanel = screen.getByRole('complementary', { name: 'My Progress' })
    expect(within(progressPanel).getByRole('progressbar', { name: 'Palpagos Islands completion' })).toHaveAttribute(
      'aria-valuenow',
      '1'
    )
    const remainingOnly = within(progressPanel).getByRole('checkbox', { name: 'Show only missing on the map' })
    expect(remainingOnly).toHaveAccessibleDescription(
      'Hide landmarks completed manually or confirmed by your connected save from the map and Map filters.'
    )
    await user.click(remainingOnly)

    expect(within(explorer).queryByRole('button', { name: /View First Effigy/ })).not.toBeInTheDocument()
    expect(within(explorer).getByRole('button', { name: 'View Second Effigy' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'First Effigy · Manual completion' })).not.toBeInTheDocument()
    await waitFor(() => {
      const stored = JSON.parse(
        window.localStorage.getItem('palworld-live-map.completion-profiles.v1') || '{}'
      ) as Record<string, unknown>
      expect(stored).toMatchObject({
        version: 1,
        activeProfileId: 'manual:default',
        remainingOnly: true,
        profiles: [
          {
            id: 'manual:default',
            name: 'My checklist',
            source: 'manual',
            manualMarks: [expect.objectContaining({ landmarkId: 'effigy-complete' })]
          }
        ]
      })
      expect(stored).not.toHaveProperty('saveEvidence')
    })

    view.unmount()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const restoredExplorer = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(screen.getByRole('button', { name: 'My Progress' }))
    const restoredProgress = screen.getByRole('complementary', { name: 'My Progress' })
    expect(within(restoredProgress).getByRole('checkbox', { name: 'Show only missing on the map' })).toBeChecked()
    expect(within(restoredProgress).getByRole('progressbar')).toHaveAttribute('aria-valuenow', '1')
    expect(within(restoredExplorer).queryByRole('button', { name: /View First Effigy/ })).not.toBeInTheDocument()
  })

  it('collapses and expands individual filter sections', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const collapse = screen.getByRole('button', { name: 'Collapse Online Players section' })
    const content = document.getElementById(collapse.getAttribute('aria-controls') || '')
    if (!content) throw new Error('Expected the online-player disclosure content')
    expect(within(content).getByRole('button', { name: 'View Luke · Lv 55' })).toBeInTheDocument()
    await user.click(collapse)
    expect(collapse).toHaveAttribute('aria-expanded', 'false')
    expect(collapse).toHaveFocus()
    expect(content).toBeEmptyDOMElement()

    await user.click(screen.getByRole('button', { name: 'Expand Online Players section' }))
    expect(within(content).getByRole('button', { name: 'View Luke · Lv 55' })).toBeInTheDocument()
  })

  it('only builds guild hierarchy rows while the Guilds section is expanded', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const expand = screen.getByRole('button', { name: 'Expand Guilds section' })
    const contentId = expand.getAttribute('aria-controls')
    if (!contentId) throw new Error('Expected the Guilds disclosure to control a content container')
    const content = document.getElementById(contentId)
    if (!content) throw new Error('Expected the Guilds content container to stay mounted')
    expect(content).toBeEmptyDOMElement()

    await user.click(expand)
    expect(within(content).getByRole('button', { name: 'View guild Home' })).toBeInTheDocument()

    const collapse = screen.getByRole('button', { name: 'Collapse Guilds section' })
    await user.click(collapse)
    expect(collapse).toHaveFocus()
    expect(content).toBeEmptyDOMElement()
  })

  it('only mounts simple category rows while their section is expanded', async () => {
    mockAPI((path) =>
      path === '/assets/test-world-catalogue.json'
        ? {
            ...(responses[path] as Record<string, unknown>),
            locations: [
              {
                id: 'alpha-lazy-row',
                kind: 'alpha-pals',
                name: 'Lazy Alpha',
                level: 42,
                x: 15,
                y: 25,
                map: 'palpagos'
              }
            ]
          }
        : responses[path]
    )
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const expand = screen.getByRole('button', { name: 'Expand Alpha Pals section' })
    const contentId = expand.getAttribute('aria-controls')
    if (!contentId) throw new Error('Expected the category disclosure to control a content container')
    const content = document.getElementById(contentId)
    if (!content) throw new Error('Expected the category content container to stay mounted')
    expect(content).toBeEmptyDOMElement()

    await user.click(expand)
    expect(within(content).getByRole('button', { name: 'View Lazy Alpha · Lv 42' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Collapse Alpha Pals section' }))
    expect(content).toBeEmptyDOMElement()
  })

  it('nests companion Pals under online players and exposes them in player details', async () => {
    const playerState = responses['/api/players'] as Record<string, unknown> & {
      players: Array<Record<string, unknown>>
    }
    const config = {
      ...(responses['/api/config'] as Record<string, unknown>),
      layers: [
        { id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] },
        { id: 'world-tree', name: 'World Tree', bounds: [100, 100, -100, -100] }
      ]
    }
    const players = [
      ...playerState.players,
      { id: 'player-robin', name: 'Robin', level: 31, x: 20, y: 30, map: 'world-tree' }
    ]
    const companions = [
      {
        id: 'companion-spark',
        kind: 'companions',
        name: 'Spark',
        detail: 'Sparkit',
        level: 12,
        ownerId: 'player-luke',
        x: 11,
        y: 21,
        map: 'palpagos'
      },
      {
        id: 'companion-traveler',
        kind: 'companions',
        name: 'Traveler',
        ownerId: 'player-robin',
        x: 12,
        y: 22,
        map: 'palpagos'
      },
      {
        id: 'companion-drifter',
        kind: 'companions',
        name: 'Drifter',
        ownerId: 'player-luke-suffix',
        x: 13,
        y: 23,
        map: 'palpagos'
      }
    ]
    mockAPI((path) => {
      if (path === '/api/config') return config
      if (path === '/api/players') return { ...playerState, players }
      if (path === '/api/objects') {
        return { ...(responses[path] as object), objects: companions, total: companions.length }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).queryByRole('checkbox', { name: 'Show Companion Pals' })).not.toBeInTheDocument()
    expect(within(explorer).queryByRole('button', { name: /Companion Pals section/ })).not.toBeInTheDocument()
    expect(within(explorer).getByText('Online Players (1)')).toBeVisible()
    const lukeCompanions = within(explorer).getByRole('group', { name: 'Companion Pals for Luke' })
    const nestedSpark = within(lukeCompanions).getByRole('button', { name: 'View Spark · Lv 12' })
    expect(nestedSpark).toBeVisible()
    expect(within(lukeCompanions).queryByRole('checkbox')).not.toBeInTheDocument()
    expect(within(explorer).queryByRole('button', { name: 'View Traveler' })).not.toBeInTheDocument()
    expect(within(explorer).queryByRole('button', { name: 'View Drifter' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Spark · Lv 12' })).not.toBeInTheDocument()

    await user.click(nestedSpark)
    expect(await screen.findByRole('heading', { name: 'Spark' })).toBeVisible()
    let inspector = screen.getByRole('dialog')
    expect(within(inspector).getByText('Species').nextElementSibling).toHaveTextContent('Sparkit')
    expect(within(inspector).getByRole('button', { name: 'View owner Luke · Lv 55' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Spark · Lv 12' })).not.toBeInTheDocument()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))

    await user.click(within(explorer).getByRole('button', { name: 'View Luke · Lv 55' }))
    inspector = screen.getByRole('dialog')
    expect(within(inspector).getByRole('heading', { name: 'Current companion Pals' })).toBeVisible()
    const modalCompanion = within(inspector).getByRole('button', { name: 'View companion Pal Spark · Lv 12' })
    expect(modalCompanion).toBeVisible()
    await user.click(modalCompanion)
    expect(await within(screen.getByRole('dialog')).findByRole('heading', { name: 'Spark' })).toBeVisible()

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'View owner Luke · Lv 55' }))
    expect(await within(screen.getByRole('dialog')).findByRole('heading', { name: 'Luke' })).toBeVisible()

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))
    await user.click(within(explorer).getByRole('button', { name: 'World Tree' }))
    expect(
      within(within(explorer).getByRole('group', { name: 'Companion Pals for Robin' })).getByRole('button', {
        name: 'View Traveler'
      })
    ).toBeVisible()
  })

  it('uses a non-modal inspector and removes individual markers from the tab order', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const marker = screen.getByRole('button', { name: 'Luke · Lv 55' })
    expect(marker).toHaveAttribute('tabindex', '-1')
    await user.click(marker)
    const inspector = screen.getByRole('dialog')
    expect(inspector).toHaveAttribute('aria-modal', 'false')
    const inspectorHeader = inspector.querySelector('header[data-map-panel-header]')
    const inspectorBody = inspector.querySelector('[data-details-body]')
    expect(inspectorHeader?.parentElement).toBe(inspector)
    expect(inspector).toHaveAttribute('data-map-panel-mobile-size', 'content')
    expect(inspector).not.toHaveAttribute('data-map-panel-mobile-state')
    expect(inspector.querySelector('[data-map-panel-resize-handle]')).not.toBeInTheDocument()
    expect(inspectorHeader).not.toHaveClass('sticky')
    expect(inspectorBody?.parentElement).toBe(inspector)
    expect(inspectorBody).toHaveClass('min-h-0', 'flex-1', 'overflow-y-auto')
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Luke' })).toHaveFocus())
    const mapControls = document.querySelector('fieldset[aria-label="Map controls"]')
    expect(mapControls).toHaveAttribute('aria-hidden', 'true')
    expect(mapControls).toHaveAttribute('inert')
    expect(screen.queryByRole('group', { name: 'Map controls' })).not.toBeInTheDocument()
    expect(screen.getByRole('searchbox')).toBeInTheDocument()

    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(marker).toHaveFocus())
    expect(screen.getByRole('group', { name: 'Map controls' })).toBeInTheDocument()
  })

  it('keeps item identity and open details in sync when a player moves', async () => {
    let moved = false
    let playerPolls = 0
    mockAPI((path) => {
      if (path === '/api/config') return { ...(responses[path] as object), pollIntervalMs: 10 }
      if (path !== '/api/players') return responses[path]
      playerPolls++
      const state = responses[path] as (typeof responses)['/api/players'] & { players: Array<Record<string, unknown>> }
      return {
        ...state,
        players: state.players.map((player) => ({ ...player, x: moved ? 80 : 10, y: moved ? 70 : 20 }))
      }
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    await user.click(screen.getByRole('button', { name: 'View Luke · Lv 55' }))
    expect(screen.getByText(/X 10\s+Y 20/)).toBeInTheDocument()

    const pollsBeforeMove = playerPolls
    moved = true
    await waitFor(() => expect(playerPolls).toBeGreaterThan(pollsBeforeMove))
    expect(await screen.findByText(/X 80\s+Y 70/)).toBeInTheDocument()
  })

  it('keeps a hidden player hidden after their coordinates change', async () => {
    let moved = false
    let playerPolls = 0
    mockAPI((path) => {
      if (path === '/api/config') return { ...(responses[path] as object), pollIntervalMs: 10 }
      if (path !== '/api/players') return responses[path]
      playerPolls++
      const state = responses[path] as (typeof responses)['/api/players'] & { players: Array<Record<string, unknown>> }
      return { ...state, players: state.players.map((player) => ({ ...player, x: moved ? 80 : 10 })) }
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const visibility = screen.getByRole('checkbox', { name: 'Show Luke · Lv 55' })
    await user.click(visibility)
    expect(screen.queryByRole('button', { name: 'Luke · Lv 55' })).not.toBeInTheDocument()

    const pollsBeforeMove = playerPolls
    moved = true
    await waitFor(() => expect(playerPolls).toBeGreaterThan(pollsBeforeMove))
    expect(screen.getByRole('checkbox', { name: 'Show Luke · Lv 55' })).not.toBeChecked()
    expect(screen.queryByRole('button', { name: 'Luke · Lv 55' })).not.toBeInTheDocument()
  })

  it('reports API-level world object failures even when the request returns successfully', async () => {
    mockAPI((path) =>
      path === '/api/objects'
        ? {
            ...(responses[path] as object),
            available: false,
            lastError: 'response-too-large',
            objects: [],
            total: 0
          }
        : responses[path]
    )

    render(<App />)
    expect(await screen.findByText('The world object response exceeded the configured safety limit.')).toBeVisible()
  })

  it('keeps the specific failure and truncation context for a retained world snapshot', async () => {
    mockAPI((path) =>
      path === '/api/objects'
        ? {
            ...(responses[path] as object),
            stale: true,
            truncated: true,
            total: 2,
            lastError: 'response-too-large'
          }
        : responses[path]
    )

    render(<App />)
    expect(
      await screen.findByText(
        'The latest world object response exceeded the safety limit; showing the last successful snapshot. It contains 1 of 2 projected objects.'
      )
    ).toBeVisible()
  })

  it('labels retained metrics as stale instead of presenting them as live', async () => {
    mockAPI((path) =>
      path === '/api/players' ? { ...(responses[path] as object), metricsStale: true } : responses[path]
    )
    render(<App />)

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('server metrics stale'))
    expect(screen.getByText('Server FPS').parentElement).toHaveTextContent('N/A')
    expect(screen.getByText('Uptime').parentElement).toHaveTextContent('N/A')
    expect(screen.queryByText('Frame')).not.toBeInTheDocument()
    expect(screen.queryByTitle('View server details')).not.toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('marks retained browser data stale when the player API becomes unreachable', async () => {
    let playerRequests = 0
    mockAPI((path) => {
      if (path === '/api/config') return { ...(responses[path] as object), pollIntervalMs: 10 }
      if (path === '/api/players' && ++playerRequests > 1) return new Error('connection lost')
      return responses[path]
    })
    render(<App />)

    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('server live'))
    await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('map connection interrupted'))
    expect(screen.getByText('Server FPS').parentElement).toHaveTextContent('N/A')
    expect(screen.getByText('Players').parentElement).toHaveTextContent('N/A')
    expect(screen.getByText('Uptime').parentElement).toHaveTextContent('N/A')
  })

  it('clusters dense map markers and caps long explorer categories', async () => {
    const objects = Array.from({ length: 1_000 }, (_, index) => ({
      id: `effigy-${index}`,
      kind: 'effigies',
      name: `Pal Effigy ${index.toString().padStart(4, '0')}`,
      x: -90 + (index % 40) * 4.5,
      y: -90 + Math.floor(index / 40) * 7.2,
      map: 'palpagos'
    }))
    mockAPI((path) =>
      path === '/api/objects' ? { ...(responses[path] as object), objects, total: objects.length } : responses[path]
    )

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const effigyVisibility = within(explorer).getByRole('checkbox', { name: 'Show Pal Effigies' })
    await waitFor(() => expect(effigyVisibility).toBeEnabled())
    expect(effigyVisibility).not.toBeChecked()
    await user.click(effigyVisibility)
    await user.click(within(explorer).getByRole('button', { name: 'Expand Pal Effigies section' }))
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: /Zoom to \d+ nearby map items/ }).length).toBeGreaterThan(0)
    )
    expect(screen.getByText('Search to inspect 750 more pal effigies.')).toBeVisible()
  })

  it('keeps Pals outside base perimeters inside an explicit guild or fallback group', async () => {
    const objects = [
      {
        id: 'base-builders',
        kind: 'bases',
        name: 'Builders',
        baseId: 'base-builders',
        guildKey: 'guild-builders',
        x: 0,
        y: 0,
        map: 'palpagos'
      },
      {
        id: 'worker-assigned',
        kind: 'workers',
        name: 'Assigned',
        baseId: 'base-builders',
        guildKey: 'guild-builders',
        x: 1,
        y: 1,
        map: 'palpagos'
      },
      {
        id: 'worker-moldron',
        kind: 'workers',
        name: 'Moldron',
        guildKey: 'guild-builders',
        x: 80,
        y: 80,
        map: 'palpagos'
      },
      {
        id: 'worker-drifter',
        kind: 'workers',
        name: 'Drifter',
        x: -80,
        y: -80,
        map: 'palpagos'
      }
    ]
    mockAPI((path) =>
      path === '/api/objects' ? { ...(responses[path] as object), objects, total: objects.length } : responses[path]
    )

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })

    await user.click(within(explorer).getByRole('button', { name: 'Expand Guilds section' }))
    await user.type(screen.getByRole('searchbox'), 'Moldron')
    const guildOutside = within(explorer).getByRole('group', { name: 'Outside base perimeters for Builders' })
    expect(within(guildOutside).getByRole('button', { name: 'View Moldron' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Builders' }))
    const baseDisclosure = within(explorer).getByRole('button', { name: 'Expand Builders Base' })
    const baseContent = document.getElementById(baseDisclosure.getAttribute('aria-controls') || '')
    if (!baseContent) throw new Error('Expected an assigned base group')
    await user.click(baseDisclosure)
    expect(within(baseContent).getByRole('button', { name: 'View Assigned' })).toBeVisible()
    expect(within(baseContent).queryByRole('button', { name: 'View Moldron' })).not.toBeInTheDocument()

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show guild Builders' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Moldron' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Drifter' })).toBeChecked()

    const fallback = within(explorer).getByRole('group', { name: 'Pals with no linked guild' })
    expect(within(fallback).getByText('No linked guild')).toBeVisible()
    expect(within(fallback).getByText('Outside base perimeters')).toBeVisible()
    expect(within(fallback).getByRole('button', { name: 'View Drifter' })).toBeVisible()
  })

  it('caps companion Pals nested under their online player', async () => {
    const companions = Array.from({ length: 300 }, (_, index) => ({
      id: `companion-${index}`,
      kind: 'companions',
      name: `Companion ${index.toString().padStart(3, '0')}`,
      ownerId: 'player-luke',
      x: index / 10,
      y: index / 10,
      map: 'palpagos'
    }))
    mockAPI((path) =>
      path === '/api/objects'
        ? { ...(responses[path] as object), objects: companions, total: companions.length }
        : responses[path]
    )

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    await user.type(screen.getByRole('searchbox'), 'Companion')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).getAllByRole('button', { name: /View Companion \d+/ })).toHaveLength(200)
    expect(within(explorer).getByRole('status', { name: 'Search results' })).toHaveTextContent(
      'Showing 200 of 300 matches for “Companion”. Refine your search to inspect 100 more.'
    )
    expect(within(explorer).queryByText(/more companion matches/)).not.toBeInTheDocument()
    expect(within(explorer).queryByRole('checkbox', { name: 'Show Companion Pals' })).not.toBeInTheDocument()
  })

  it('caps assigned Pals when a broad search expands every base', async () => {
    const base = {
      id: 'base-dense',
      kind: 'bases',
      name: 'Dense Base',
      baseId: 'base-dense',
      guildKey: 'guild-dense',
      x: 0,
      y: 0,
      map: 'palpagos'
    }
    const workers = Array.from({ length: 300 }, (_, index) => ({
      id: `worker-${index}`,
      kind: 'workers',
      name: `Worker ${index.toString().padStart(3, '0')}`,
      baseId: base.id,
      x: index / 10,
      y: index / 10,
      map: 'palpagos'
    }))
    mockAPI((path) =>
      path === '/api/objects'
        ? { ...(responses[path] as object), objects: [base, ...workers], total: workers.length + 1 }
        : responses[path]
    )
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    await user.type(screen.getByRole('searchbox'), 'Worker')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    expect(within(explorer).getAllByRole('button', { name: /View Worker \d+/ })).toHaveLength(200)
    expect(within(explorer).getByRole('status', { name: 'Search results' })).toHaveTextContent(
      'Showing 200 of 300 matches for “Worker”. Refine your search to inspect 100 more.'
    )
    expect(screen.getByText('300 assigned Pals')).toBeVisible()
    expect(within(explorer).queryByText(/more assigned Pals omitted/)).not.toBeInTheDocument()
  })

  it('fairly budgets broad search results across categories and preserves exact-result navigation', async () => {
    const companions = Array.from({ length: 90 }, (_, index) => ({
      id: `shared-companion-${index}`,
      kind: 'companions',
      name: `Shared Result Companion ${index.toString().padStart(3, '0')}`,
      ownerId: 'player-luke',
      x: index / 10,
      y: index / 10,
      map: 'palpagos'
    }))
    const base = {
      id: 'base-dense',
      kind: 'bases',
      name: 'Dense Base',
      baseId: 'base-dense',
      guildKey: 'guild-dense',
      x: 0,
      y: 0,
      map: 'palpagos'
    }
    const workers = Array.from({ length: 90 }, (_, index) => ({
      id: `shared-worker-${index}`,
      kind: 'workers',
      name: `Shared Result Worker ${index.toString().padStart(3, '0')}`,
      baseId: base.id,
      x: index / 10,
      y: index / 10,
      map: 'palpagos'
    }))
    const effigies = Array.from({ length: 90 }, (_, index) => ({
      id: `shared-effigy-${index}`,
      kind: 'effigies',
      name: `Shared Result Effigy ${index.toString().padStart(3, '0')}`,
      x: index / 10,
      y: index / 10,
      map: 'palpagos'
    }))
    const exactTarget = {
      id: 'shared-exact-target',
      kind: 'npc-locations',
      name: 'Shared Result Exact Target',
      detail: 'Merchant',
      x: 25,
      y: 25,
      map: 'palpagos'
    }
    mockAPI((path) => {
      if (path === '/assets/test-world-catalogue.json') {
        return { ...(responses[path] as Record<string, unknown>), locations: [exactTarget] }
      }
      if (path === '/api/objects') {
        const objects = [base, ...companions, ...workers, ...effigies]
        return { ...(responses[path] as Record<string, unknown>), objects, total: objects.length }
      }
      return responses[path]
    })

    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const searchbox = within(explorer).getByRole('searchbox')
    await user.type(searchbox, 'Shared Result')

    expect(within(explorer).getAllByRole('button', { name: /^View Shared Result/ })).toHaveLength(200)
    const status = within(explorer).getByRole('status', { name: 'Search results' })
    expect(status).toHaveTextContent(
      'Showing 200 of 271 matches for “Shared Result”. Refine your search to inspect 71 more.'
    )
    expect(status).toHaveAttribute('aria-live', 'polite')
    expect(status).toHaveAttribute('aria-atomic', 'true')
    expect(searchbox).toHaveAttribute('aria-describedby', status.id)
    expect(within(explorer).getAllByRole('status', { name: 'Search results' })).toHaveLength(1)
    expect(within(explorer).getAllByText(/Refine your search to inspect/)).toHaveLength(1)
    expect(within(explorer).getAllByRole('button', { name: /View Shared Result Companion/ }).length).toBeGreaterThan(0)
    expect(within(explorer).getAllByRole('button', { name: /View Shared Result Worker/ }).length).toBeGreaterThan(0)
    expect(within(explorer).getAllByRole('button', { name: /View Shared Result Effigy/ }).length).toBeGreaterThan(0)
    expect(within(explorer).getByRole('button', { name: 'View Shared Result Exact Target' })).toBeVisible()

    await user.clear(searchbox)
    await user.type(searchbox, 'Shared Result Exact Target')
    expect(within(explorer).getByRole('status', { name: 'Search results' })).toHaveTextContent(
      '1 match for “Shared Result Exact Target”.'
    )
    await user.click(within(explorer).getByRole('button', { name: 'View Shared Result Exact Target' }))

    const inspector = screen.getByRole('dialog', { name: 'Shared Result Exact Target' })
    expect(within(inspector).getByText('NPC LOCATION DETAILS')).toBeVisible()
  })
})
