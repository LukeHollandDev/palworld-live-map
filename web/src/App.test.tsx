import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const responses: Record<string, unknown> = {
  '/api/config': {
    pollIntervalMs: 60_000,
    worldPollIntervalMs: 60_000,
    worldDataEnabled: true,
    layers: [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }]
  },
  '/api/players': {
    server: { name: 'Test Realm', description: 'A test server', version: '1.0' },
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
    players: [{ id: 'player-luke', name: 'Luke', level: 55, x: 10, y: 20, map: 'palpagos' }]
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

    expect(await screen.findByRole('heading', { name: 'Test Realm' })).toBeInTheDocument()
    expect(screen.queryByText('Demo data')).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('1 / 32 players')
    expect(screen.getByText('Server FPS')).toBeVisible()
    expect(screen.getByText('16.7 ms')).toBeVisible()
    expect(screen.getByText('Uptime')).toBeVisible()
    expect(screen.getByText('Bases')).toBeVisible()
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
    const detailsTitle = screen.getByRole('heading', { name: 'Luke' })
    expect(detailsTitle).toBeInTheDocument()
    await waitFor(() => expect(detailsTitle).toHaveFocus())
    expect(screen.getByText('X 10 · Y 20')).toBeInTheDocument()
    expect(screen.getByText('No guild membership is known for this player.')).toBeVisible()
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
    await waitFor(() => expect(screen.getByRole('button', { name: 'Collapse map search' })).toHaveFocus())
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

  it('filters explorer items and keeps global search available when the intel drawer collapses', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const searchbox = screen.getByRole('searchbox')
    expect(searchbox).toHaveAttribute('type', 'search')
    expect(screen.queryByText('/')).not.toBeInTheDocument()
    await user.type(searchbox, 'missing')
    expect(screen.getAllByRole('button', { name: 'Clear search' })).toHaveLength(1)
    expect(screen.getByText('No players or companion Pals match “missing”.')).toBeInTheDocument()

    const filterPanel = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(screen.getByRole('button', { name: 'Collapse map filter' }))
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument())
    expect(document.querySelector('#map-filter-panel')).toBe(filterPanel)
    expect(filterPanel).toHaveAttribute('aria-hidden', 'true')
    expect(filterPanel).toHaveAttribute('inert')
    expect(screen.getByRole('button', { name: 'Open map filters' })).toHaveFocus()
    expect(screen.getByRole('searchbox')).toHaveValue('missing')
    await user.click(screen.getByRole('button', { name: 'Open map filters' }))
    expect(screen.getByRole('complementary', { name: 'Map filters' })).toBe(filterPanel)
    expect(screen.getByRole('button', { name: 'Collapse map filter' })).toHaveFocus()
    expect(screen.queryByRole('button', { name: 'Open map filters' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Collapse map search' }))
    expect(screen.queryByRole('searchbox')).not.toBeInTheDocument()
    const reopenSearch = screen.getByRole('button', { name: 'Open map search, current query: missing' })
    await waitFor(() => expect(reopenSearch).toHaveFocus())

    await user.click(reopenSearch)
    expect(screen.getByRole('searchbox')).toHaveValue('missing')
    await waitFor(() => expect(screen.getByRole('searchbox')).toHaveFocus())

    await user.keyboard('{Escape}')
    expect(screen.getByRole('searchbox')).toHaveValue('')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.getByRole('button', { name: 'Open map search' })).toHaveFocus())

    await user.keyboard('/')
    await waitFor(() => expect(screen.getByRole('searchbox')).toHaveFocus())
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
    expect(within(explorer).getByRole('button', { name: 'View Luke · Lv 55' })).toBeVisible()
    expect(within(explorer).getByText('Lv 55 · Builders')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Luke · Lv 55' })).toBeInTheDocument()
  })

  it('collapses and expands individual filter sections', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    const collapse = screen.getByRole('button', { name: 'Collapse Players section' })
    await user.click(collapse)
    expect(collapse).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('button', { name: 'View Luke · Lv 55' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Expand Players section' }))
    expect(screen.getByRole('button', { name: 'View Luke · Lv 55' })).toBeInTheDocument()
  })

  it('nests companion Pals under their exact online owner and labels unresolved owners', async () => {
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
    expect(within(explorer).queryByRole('button', { name: /Companion Pals section/ })).not.toBeInTheDocument()
    expect(within(explorer).getByText('Players (1)')).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'Expand companion Pals for Luke' })).toBeVisible()
    expect(within(explorer).queryByRole('button', { name: 'View Spark · Lv 12' })).not.toBeInTheDocument()

    await user.type(screen.getByRole('searchbox'), 'Spark')
    expect(within(explorer).getByRole('button', { name: 'Collapse companion Pals for Luke' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Spark · Lv 12' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Clear search' }))
    expect(within(explorer).getByRole('button', { name: 'Expand companion Pals for Luke' })).toBeVisible()
    await user.click(within(explorer).getByRole('button', { name: 'Expand companion Pals for Luke' }))
    const playerDisclosure = within(explorer).getByRole('button', { name: 'Collapse companion Pals for Luke' })
    const playerContent = document.getElementById(playerDisclosure.getAttribute('aria-controls') || '')
    if (!playerContent) throw new Error('Expected a companion Pal group for Luke')
    const sparkButton = within(playerContent).getByRole('button', { name: 'View Spark · Lv 12' })
    expect(within(playerContent).queryByRole('button', { name: 'View Traveler' })).not.toBeInTheDocument()
    expect(within(playerContent).queryByRole('button', { name: 'View Drifter' })).not.toBeInTheDocument()

    await user.click(sparkButton)
    expect(await screen.findByRole('heading', { name: 'Spark' })).toBeVisible()
    expect(within(screen.getByRole('dialog')).getByRole('button', { name: 'View owner Luke · Lv 55' })).toBeVisible()
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close details' }))

    const otherMap = within(explorer).getByRole('group', {
      name: 'Companion Pals with an owner on another map'
    })
    expect(within(otherMap).getByText('Owner on another map')).toBeVisible()
    expect(within(otherMap).getByRole('button', { name: 'View Traveler' })).toBeVisible()
    const noOwner = within(explorer).getByRole('group', { name: 'Companion Pals with no online owner' })
    expect(within(noOwner).getByText('No online owner')).toBeVisible()
    expect(within(noOwner).getByRole('button', { name: 'View Drifter' })).toBeVisible()

    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Luke · Lv 55' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Spark · Lv 12' })).not.toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Traveler' })).toBeChecked()
    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Players' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Spark · Lv 12' })).toBeChecked()
    expect(within(explorer).getByRole('checkbox', { name: 'Show Drifter' })).toBeChecked()
    await user.click(within(explorer).getByRole('checkbox', { name: 'Show Players' }))
    expect(within(explorer).getByRole('checkbox', { name: 'Show Traveler' })).not.toBeChecked()
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
    expect(screen.getByText('X 10 · Y 20')).toBeInTheDocument()

    const pollsBeforeMove = playerPolls
    moved = true
    await waitFor(() => expect(playerPolls).toBeGreaterThan(pollsBeforeMove))
    expect(await screen.findByText('X 80 · Y 70')).toBeInTheDocument()
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
    expect(screen.queryByText('Server FPS')).not.toBeInTheDocument()
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
    expect(screen.queryByText('Server FPS')).not.toBeInTheDocument()
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

    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })
    expect(screen.getAllByRole('button', { name: /Zoom to \d+ nearby map items/ }).length).toBeGreaterThan(0)
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

  it('caps companion Pals across expanded player groups', async () => {
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
    expect(
      within(explorer).getByText(
        '50 more companion Pals omitted. Refine your search or expand fewer players to inspect them.'
      )
    ).toBeVisible()
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
