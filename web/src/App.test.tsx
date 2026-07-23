import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const responses: Record<string, unknown> = {
  '/api/config': {
    pollIntervalMs: 60_000,
    worldPollIntervalMs: 60_000,
    worldDataEnabled: true,
    layers: [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }],
    landmarks: [],
    landmarkCatalogue: {
      gameVersion: '1.0.1.100619',
      generator: 'palworld-map-exporter/2',
      decoder: 'CUE4Parse'
    }
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
      return body
        ? new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
        : new Response(null, { status: 404 })
    })
  )
}

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
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

  it('renders live server data and opens player details', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)

    const serverTitle = await screen.findByRole('heading', { name: 'Test Realm' })
    expect(serverTitle).toBeInTheDocument()
    expect(serverTitle).toHaveClass('font-bold', 'text-[21px]')
    const statusBar = screen.getByRole('banner')
    expect(statusBar).toHaveClass('absolute', 'pointer-events-none')
    expect(statusBar).toHaveClass('min-[1600px]:inset-x-[386px]', 'min-[1600px]:px-0')
    expect(statusBar).not.toHaveClass('bg-[#0f1b21]/98')
    expect(serverTitle.parentElement?.parentElement).toHaveClass('pal-glass-surface', 'h-[54px]')
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
    expect(screen.getByRole('link', { name: "Luke Holland's website" })).toHaveAttribute(
      'href',
      'https://lukeholland.dev'
    )

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

  it('starts with only player status groups and Guilds expanded and enabled', async () => {
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
    expect(within(explorer).getByRole('button', { name: 'Collapse Offline Players section' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Collapse Guilds section' })).toBeVisible()

    for (const category of ['Companion Pals', 'Wild Pals', 'Alpha Pals', 'Tower Bosses', 'NPCs']) {
      const checkbox = within(explorer).getByRole('checkbox', { name: `Show ${category}` })
      expect(checkbox).not.toBeChecked()
      expect(checkbox).toBeEnabled()
      expect(within(explorer).getByRole('button', { name: `Expand ${category} section` })).toBeVisible()
    }
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
    expect(within(explorer).getByText(/Landmark catalogue version mismatch:/)).toHaveTextContent(
      'locations were exported for Palworld 1.0.1.100619, but this server reports v1.0.1.100620'
    )
    expect(within(explorer).getByText(/Landmark catalogue version mismatch:/)).toHaveTextContent('make game-assets')
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
    expect(within(explorer).queryByText(/Landmark catalogue version mismatch:/)).not.toBeInTheDocument()
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
    expect(within(explorer).queryByText(/Landmark catalogue/)).not.toBeInTheDocument()
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
    expect(within(explorer).getByText(/Landmark catalogue version mismatch:/)).toBeVisible()
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
    await user.type(screen.getByRole('searchbox'), 'Luke')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    const opener = within(explorer).getByRole('button', { name: 'View Luke · Lv 55' })
    await user.click(opener)
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Luke' })).toHaveFocus())
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

    expect(screen.getByRole('button', { name: 'Spark · Lv 12' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Spark' })).toHaveFocus())
    expect(within(explorer).getByText('Unnamed guild')).toBeVisible()

    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Open leaderboards' })).toHaveFocus())
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
    expect(within(explorer).getByRole('button', { name: 'Collapse Guilds section' })).toBeVisible()
    const guildOpener = within(explorer).getByRole('button', { name: 'View guild Builders' })
    const guildDisclosure = within(explorer).getByRole('button', { name: 'Expand Builders' })
    expect(guildDisclosure).toHaveAttribute('aria-expanded', 'false')

    await user.click(guildOpener)
    let inspector = screen.getByRole('dialog')
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

    await user.click(within(inspector).getByRole('button', { name: 'Close details' }))
    await waitFor(() => expect(guildOpener).toHaveFocus())
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

    const filterPanel = screen.getByRole('complementary', { name: 'Map filters' })
    const searchbox = within(filterPanel).getByRole('searchbox')
    expect(searchbox).toHaveAttribute('type', 'search')
    expect(searchbox).toHaveAttribute('placeholder', 'Filter map results…')
    expect(screen.queryByText('/')).not.toBeInTheDocument()
    await user.type(searchbox, 'missing')
    expect(screen.getAllByRole('button', { name: 'Clear search' })).toHaveLength(1)
    expect(screen.getByText('No online players match “missing”.')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Collapse map filter' }))
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument())
    expect(document.querySelector('#map-filter-panel')).toBe(filterPanel)
    expect(filterPanel).toHaveAttribute('aria-hidden', 'true')
    expect(filterPanel).toHaveAttribute('inert')
    expect(screen.queryByRole('searchbox')).not.toBeInTheDocument()
    const reopenFilter = screen.getByRole('button', { name: 'Open map filters, current search: missing' })
    expect(reopenFilter).toHaveFocus()
    expect(reopenFilter).toHaveClass('size-12')
    expect(reopenFilter.querySelector('svg')).toHaveClass('size-6')
    expect(reopenFilter).not.toHaveTextContent('Map filter')
    expect(reopenFilter).not.toHaveTextContent('Filters')
    expect(document.querySelector('#map-search')).toHaveValue('missing')

    await user.keyboard('/')
    expect(screen.getByRole('complementary', { name: 'Map filters' })).toBe(filterPanel)
    expect(screen.getByRole('searchbox')).toHaveValue('missing')
    await waitFor(() => expect(screen.getByRole('searchbox')).toHaveFocus())
    expect(screen.queryByRole('button', { name: 'Open map filters' })).not.toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.getByRole('searchbox')).toHaveValue('')
    expect(within(filterPanel).getByRole('button', { name: 'Expand NPCs section' })).toBeVisible()
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Open map filters' })).toHaveFocus())
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
    expect(
      within(explorer)
        .getByRole('button', { name: 'Collapse Offline Players section' })
        .querySelector('[data-marker-kind="players"]')
    ).toHaveAttribute('data-player-status', 'offline')
    expect(within(explorer).queryByRole('button', { name: 'View guild Save Crew' })).not.toBeInTheDocument()

    await user.click(offlineVisibility)
    expect(screen.queryByRole('button', { name: 'Zoe · Lv 60' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Bob · Lv 50' })).toBeInTheDocument()
    await user.click(offlineVisibility)
    expect(screen.getByRole('button', { name: 'Zoe · Lv 60' })).toBeInTheDocument()

    const leaderboardOpener = screen.getByRole('button', { name: 'Open leaderboards' })
    expect(within(screen.getByRole('main')).getByRole('button', { name: 'Open leaderboards' })).toBe(leaderboardOpener)
    expect(
      within(screen.getByRole('banner')).queryByRole('button', { name: 'Open leaderboards' })
    ).not.toBeInTheDocument()
    expect(leaderboardOpener.querySelector('svg')).toHaveClass('size-6')
    expect(leaderboardOpener).not.toHaveTextContent('Leaderboards')
    expect(leaderboardOpener).toHaveClass('size-12')
    await user.click(leaderboardOpener)
    expect(leaderboardOpener).toHaveAttribute('aria-hidden', 'true')
    expect(leaderboardOpener).toHaveAttribute('inert')
    expect(leaderboardOpener).toHaveClass('opacity-0')
    expect(screen.queryByRole('button', { name: 'Open leaderboards' })).not.toBeInTheDocument()
    const leaderboard = screen.getByRole('dialog')
    expect(leaderboard).toHaveClass('top-[78px]', 'bottom-4', 'w-[350px]')
    expect(explorer).toHaveClass('top-[78px]', 'bottom-4', 'w-[350px]')
    expect(leaderboard.querySelector('header')).toHaveClass('min-h-[78px]')
    expect(explorer.firstElementChild).toHaveClass('min-h-[78px]')
    expect(within(leaderboard).getByRole('heading', { name: 'Leaderboards' })).toBeVisible()
    expect(within(leaderboard).getByRole('heading', { name: 'Player levels' })).toBeVisible()
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
    expect(within(explorer).getByRole('button', { name: 'Expand Alpha Pals section' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Expand Tower Bosses section' })).toBeVisible()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Alpha Pals' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Tower Bosses' })).not.toBeChecked()
    expect(screen.queryByRole('button', { name: 'Penking · Lv 15' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).not.toBeInTheDocument()

    await user.type(screen.getByRole('searchbox'), 'penking')
    await user.click(within(explorer).getByRole('button', { name: 'View Penking · Lv 15' }))
    expect(within(screen.getByRole('dialog')).getByRole('heading', { name: 'Penking' })).toBeVisible()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Alpha Pals' })).toBeChecked()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))
    await user.click(screen.getByRole('button', { name: 'Clear search' }))

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Tower Bosses' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Alpha Pals section' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Tower Bosses section' }))
    expect(screen.getByRole('button', { name: 'Penking · Lv 15' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Zoe & Grizzbolt · Lv 10' })).toBeInTheDocument()
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

  it('collapses and expands individual filter sections', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const collapse = screen.getByRole('button', { name: 'Collapse Online Players section' })
    await user.click(collapse)
    expect(collapse).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: 'View Luke · Lv 55' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Expand Online Players section' }))
    expect(screen.getByRole('button', { name: 'View Luke · Lv 55' })).toBeInTheDocument()
  })

  it('keeps companion Pals in their own map-filter category and preserves owner details', async () => {
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
    const companionVisibility = within(explorer).getByRole('checkbox', { name: 'Show Companion Pals' })
    await waitFor(() => expect(companionVisibility).toBeEnabled())
    expect(companionVisibility).not.toBeChecked()
    expect(within(explorer).getByRole('button', { name: 'Expand Companion Pals section' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Spark · Lv 12' })).not.toBeInTheDocument()

    await user.click(companionVisibility)
    await user.click(within(explorer).getByRole('button', { name: 'Expand Companion Pals section' }))
    expect(within(explorer).getByText('Online Players (1)')).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Spark · Lv 12' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Traveler' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Drifter' })).toBeVisible()

    await user.click(within(explorer).getByRole('button', { name: 'View Spark · Lv 12' }))
    expect(await screen.findByRole('heading', { name: 'Spark' })).toBeVisible()
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'View owner Luke · Lv 55' })).toBeVisible()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Luke · Lv 55' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Spark · Lv 12' })).toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Traveler' })).toBeChecked()
    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Companion Pals' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Spark · Lv 12' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Traveler' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Drifter' })).not.toBeChecked()
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
    const inspectorHeader = inspector.querySelector('header')
    const inspectorBody = inspector.querySelector('[data-details-body]')
    expect(inspectorHeader?.parentElement).toBe(inspector)
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
      id: `wild-${index}`,
      kind: 'wild-pals',
      name: `Wild Pal ${index.toString().padStart(4, '0')}`,
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
    const wildVisibility = within(explorer).getByRole('checkbox', { name: 'Show Wild Pals' })
    await waitFor(() => expect(wildVisibility).toBeEnabled())
    await user.click(wildVisibility)
    await user.click(within(explorer).getByRole('button', { name: 'Expand Wild Pals section' }))
    await waitFor(() =>
      expect(screen.getAllByRole('button', { name: /Zoom to \d+ nearby map items/ }).length).toBeGreaterThan(0)
    )
    expect(screen.getByText('Search to inspect 750 more wild pals.')).toBeVisible()
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

  it('caps companion Pals in their independent filter category', async () => {
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
    expect(within(explorer).getAllByRole('button', { name: /View Companion \d+/ })).toHaveLength(250)
    expect(within(explorer).getByText('50 more matches. Refine your search to inspect them.')).toBeVisible()
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

    expect(screen.getAllByRole('button', { name: /View Worker \d+/ })).toHaveLength(250)
    expect(screen.getByText('300 assigned Pals')).toBeVisible()
    expect(
      screen.getByText('50 more assigned Pals omitted. Refine your search or expand fewer bases to inspect them.')
    ).toBeVisible()
  })
})
