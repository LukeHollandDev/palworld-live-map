import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'

const responses: Record<string, unknown> = {
  '/api/config': {
    demoMode: true,
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
      averageFps: 59.5,
      serverFrameTime: 16.7,
      uptimeSeconds: 3600,
      baseCount: 1,
      days: 42
    },
    metricsAvailable: true,
    connected: true,
    stale: false,
    lastSuccessAt: new Date().toISOString(),
    players: [{ name: 'Luke', level: 55, x: 10, y: 20, map: 'palpagos' }]
  },
  '/api/objects': {
    enabled: true,
    available: true,
    stale: false,
    unsupported: false,
    objects: [{ kind: 'bases', name: 'Home', baseId: 'base-1', guildKey: 'guild-1', x: 30, y: 40, map: 'palpagos' }]
  }
}

function mockAPI() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string | URL | Request) => {
      const path =
        typeof input === 'string' ? input : input instanceof URL ? input.pathname : new URL(input.url).pathname
      const body = responses[path]
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
  it('renders live server data and opens player details', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)

    expect(await screen.findByRole('heading', { name: 'Test Realm' })).toBeInTheDocument()
    expect(screen.getByText('Demo data')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('1 / 32 players')

    const explorer = screen.getByRole('complementary', { name: 'Map filters' })
    await user.click(within(explorer).getByRole('button', { name: 'View Luke · Lv 55' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Luke' })).toBeInTheDocument()
    expect(screen.getByText('X 10 · Y 20')).toBeInTheDocument()
  })

  it('filters explorer items and keeps global search available when the intel drawer collapses', async () => {
    mockAPI()
    const user = userEvent.setup()
    render(<App />)
    await screen.findByRole('heading', { name: 'Test Realm' })

    await user.type(screen.getByRole('searchbox'), 'missing')
    expect(screen.getByText('No players match “missing”.')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Collapse map filter' }))
    await waitFor(() => expect(screen.queryByRole('complementary', { name: 'Map filters' })).not.toBeInTheDocument())
    expect(screen.getByRole('searchbox')).toHaveValue('missing')
    expect(screen.getByRole('button', { name: 'Open map filters' })).toBeInTheDocument()
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
    expect(screen.getByRole('searchbox')).toBeInTheDocument()
  })
})
