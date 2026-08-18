import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { App } from './App'
import { LOCAL_COMPLETION_STORAGE_KEY } from './lib/completion'

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

function pathOf(input: string | URL | Request) {
  return input instanceof Request ? new URL(input.url).pathname : new URL(String(input), window.location.href).pathname
}

afterEach(() => {
  cleanup()
  window.localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('authenticated completion overlay', () => {
  it('combines source-labelled progress, filters missing items, and removes only the save layer on disconnect', async () => {
    const manualState = {
      version: 1,
      activeProfileId: 'manual:default',
      remainingOnly: false,
      profiles: [
        {
          id: 'manual:default',
          name: 'My checklist',
          source: 'manual',
          createdAt: '2026-08-15T10:00:00.000Z',
          manualMarks: [
            { landmarkId: 'journal-both', completedAt: '2026-08-15T10:01:00.000Z' },
            { landmarkId: 'effigy-manual', completedAt: '2026-08-15T10:02:00.000Z' }
          ]
        }
      ]
    }
    window.localStorage.setItem(LOCAL_COMPLETION_STORAGE_KEY, JSON.stringify(manualState))

    const snapshotAt = new Date().toISOString()
    const locations = [
      { id: 'waypoint-save', kind: 'waypoints', name: 'Save Waypoint', x: 10, y: 10, map: 'palpagos' },
      { id: 'journal-both', kind: 'journals', name: 'Combined Journal', x: 20, y: 20, map: 'palpagos' },
      { id: 'effigy-manual', kind: 'effigies', name: 'Manual Effigy', x: 30, y: 30, map: 'palpagos' }
    ]
    const requests: Array<{ path: string; init?: RequestInit }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const path = pathOf(input)
        requests.push({ path, init })
        if (path === '/api/config') {
          return json({
            pollIntervalMs: 60_000,
            worldPollIntervalMs: 60_000,
            worldDataEnabled: true,
            playerClaimsEnabled: true,
            layers: [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }],
            catalogueUrl: '/assets/test-world-catalogue.json?v=private-catalogue-hash',
            landmarks: [],
            landmarkCatalogue: { gameVersion: '1.0.1', generator: 'test', decoder: 'test' }
          })
        }
        if (path === '/assets/test-world-catalogue.json') {
          return json({ gameVersion: '1.0.1', generator: 'test', decoder: 'test', locations })
        }
        if (path === '/api/players') {
          return json({
            server: { name: 'Progress Realm', version: 'v1.0.1' },
            metrics: {
              currentPlayers: 1,
              maxPlayers: 32,
              serverFps: 60,
              serverFrameTime: 16,
              uptimeSeconds: 60,
              baseCount: 0,
              days: 1
            },
            metricsAvailable: true,
            metricsStale: false,
            connected: true,
            stale: false,
            saveEnabled: true,
            saveAvailable: true,
            saveStale: false,
            players: [
              {
                id: 'claimed-player',
                kind: 'players',
                name: 'Claimed',
                x: 0,
                y: 0,
                map: 'palpagos',
                online: false,
                level: 50
              }
            ]
          })
        }
        if (path === '/api/objects') {
          return json({
            enabled: true,
            available: true,
            stale: false,
            unsupported: false,
            truncated: false,
            total: 0,
            objects: []
          })
        }
        if (path === '/api/player-claims')
          return json(
            {
              status: 'ready',
              challengeToken: 'challenge-token',
              expiresAt: new Date(Date.now() + 60_000).toISOString(),
              instructions: {
                kind: 'inventory_quiz',
                questions: [{ id: 'q1', prompt: 'What was equipped?', options: ['A', 'B', 'C'], canCycle: false }]
              }
            },
            201
          )
        if (path === '/api/player-claims/verify')
          return json({
            status: 'verified',
            sessionToken: 'session-token',
            idleExpiresAt: new Date(Date.now() + 60 * 60_000).toISOString(),
            absoluteExpiresAt: new Date(Date.now() + 2 * 60 * 60_000).toISOString()
          })
        if (path === '/api/me/progress') {
          return json({
            snapshotAt,
            catalogueVersion: 'private-catalogue-hash',
            domains: [
              { id: 'alpha-pals', coverage: 'complete', completedIds: [], total: 0 },
              { id: 'bosses', coverage: 'complete', completedIds: [], total: 0 },
              { id: 'bounties', coverage: 'complete', completedIds: [], total: 0 },
              { id: 'watchtowers', coverage: 'complete', completedIds: [], total: 0 },
              { id: 'waypoints', coverage: 'complete', completedIds: ['waypoint-save'], total: 1 },
              { id: 'effigies', coverage: 'complete', completedIds: [], total: 0 },
              { id: 'journals', coverage: 'complete', completedIds: ['journal-both'], total: 1 },
              { id: 'ancient-shrine-pickups', coverage: 'complete', completedIds: [], total: 0 }
            ]
          })
        }
        if (path === '/api/logout') return json({ authenticated: false })
        return new Response(null, { status: 404 })
      })
    )

    const user = userEvent.setup()
    render(<App />)
    const explorer = await screen.findByRole('complementary', { name: 'Map filters' })
    await user.click(screen.getByRole('button', { name: 'My Progress' }))
    const progressPanel = await screen.findByRole('complementary', { name: 'My Progress' })
    await user.click(within(progressPanel).getByRole('button', { name: 'This is me' }))
    await user.selectOptions(await within(progressPanel).findByLabelText('What was equipped?'), '1')
    await user.click(within(progressPanel).getByRole('button', { name: 'Connect character' }))
    expect(await within(progressPanel).findByText(/Save synced ·/)).toBeVisible()
    expect(within(progressPanel).getByRole('progressbar', { name: 'Palpagos Islands completion' })).toHaveAttribute(
      'aria-valuenow',
      '3'
    )
    expect(within(progressPanel).getByText('Breakdown')).toBeVisible()
    expect(within(progressPanel).getByText('Fast Travel')).toBeVisible()
    expect(within(progressPanel).getByText('Save + manual')).toBeVisible()
    expect(within(progressPanel).getByRole('heading', { name: 'Connected save' })).toBeVisible()

    await user.click(within(explorer).getByRole('button', { name: 'Expand Waypoints section' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Journals section' }))
    await user.click(within(explorer).getByRole('button', { name: 'Expand Pal Effigies section' }))
    expect(
      within(explorer).getByRole('button', { name: 'View Save Waypoint, save-confirmed completion' })
    ).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Combined Journal, combined completion' })).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Manual Effigy, manual completion' })).toBeVisible()

    await user.click(within(progressPanel).getByRole('checkbox', { name: 'Show only missing on the map' }))
    expect(within(progressPanel).getByText('0 missing')).toBeVisible()
    expect(within(explorer).queryByRole('button', { name: /View Save Waypoint/ })).not.toBeInTheDocument()

    await user.click(within(progressPanel).getByRole('button', { name: 'Disconnect' }))

    await waitFor(() =>
      expect(within(progressPanel).getByRole('progressbar', { name: 'Palpagos Islands completion' })).toHaveAttribute(
        'aria-valuenow',
        '2'
      )
    )
    expect(within(progressPanel).getByText('Manual · this browser')).toBeVisible()
    expect(within(explorer).getByRole('button', { name: 'View Save Waypoint' })).toBeVisible()
    expect(within(explorer).queryByRole('button', { name: /View Combined Journal/ })).not.toBeInTheDocument()

    await waitFor(() =>
      expect(JSON.parse(window.localStorage.getItem(LOCAL_COMPLETION_STORAGE_KEY) || '{}')).toMatchObject({
        ...manualState,
        remainingOnly: true
      })
    )
    const stored = window.localStorage.getItem(LOCAL_COMPLETION_STORAGE_KEY) || ''
    expect(stored).not.toContain('waypoint-save')
    expect(stored).not.toContain('private-catalogue-hash')
    expect(stored).not.toContain(snapshotAt)
    expect(requests.find((request) => request.path === '/api/me/progress')?.init).toMatchObject({
      cache: 'no-store',
      headers: { Authorization: 'Bearer session-token' }
    })
  })
})
