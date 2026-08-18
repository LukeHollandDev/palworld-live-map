import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createRef } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MapLayer } from '../types'
import { PlayerClaimProvider } from './PlayerClaimPanel'
import { type ProgressChecklistView, ProgressPanel } from './ProgressPanel'

const layer: MapLayer = {
  id: 'palpagos',
  name: 'Palpagos Islands',
  bounds: [100, 100, -100, -100]
}

function checklist(overrides: Partial<ProgressChecklistView> = {}): ProgressChecklistView {
  return {
    profileName: 'My checklist',
    completed: 2,
    total: 5,
    remaining: 3,
    breakdown: [
      {
        kind: 'effigies',
        label: 'Lifmunk Effigies',
        evidence: 'save-supported',
        completed: 2,
        total: 5,
        remaining: 3
      }
    ],
    remainingOnly: false,
    saveProgress: { phase: 'inactive' },
    onRemainingOnlyChange: vi.fn(),
    ...overrides
  }
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('ProgressPanel', () => {
  it('owns checklist progress and controls the missing-only map filter', async () => {
    const user = userEvent.setup()
    const value = checklist()
    render(
      <PlayerClaimProvider enabled={false}>
        <ProgressPanel
          open
          activeLayer={layer}
          players={[]}
          checklist={value}
          progressButtonRef={createRef<HTMLButtonElement>()}
          onClose={vi.fn()}
        />
      </PlayerClaimProvider>
    )

    const panel = screen.getByRole('complementary', { name: 'My Progress' })
    expect(within(panel).getByRole('heading', { name: 'My Progress' })).toBeVisible()
    expect(within(panel).getByText('Palpagos Islands')).toBeVisible()
    expect(within(panel).getByText('40%')).toBeVisible()
    expect(within(panel).getByRole('progressbar', { name: 'Palpagos Islands completion' })).toHaveAttribute(
      'aria-valuenow',
      '2'
    )
    expect(within(panel).getByText('Manual · this browser')).toBeVisible()
    expect(within(panel).getAllByText('2 / 5')).toHaveLength(2)
    expect(within(panel).getByText('3 missing')).toBeVisible()
    expect(within(panel).getByText('Breakdown')).toBeVisible()
    expect(within(panel).getByText('Lifmunk Effigies')).toBeVisible()
    expect(within(panel).getByText('Save + manual')).toBeVisible()
    expect(within(panel).getAllByText('Manual only')).toHaveLength(1)
    expect(within(panel).getByText('Save-backed character connection is not enabled.', { exact: false })).toBeVisible()

    await user.click(within(panel).getByRole('checkbox', { name: 'Show only missing on the map' }))
    expect(value.onRemainingOnlyChange).toHaveBeenCalledWith(true)
  })

  it('leaves save refresh timing to the server', () => {
    render(
      <ProgressPanel
        open
        activeLayer={layer}
        players={[]}
        checklist={checklist({
          saveProgress: {
            phase: 'unavailable',
            playerId: 'opaque-player',
            sessionEpoch: 1,
            requestAttempt: 1,
            reason: 'request'
          }
        })}
        progressButtonRef={createRef<HTMLButtonElement>()}
        onClose={vi.fn()}
      />
    )

    expect(screen.getByText('Save temporarily unavailable')).toBeVisible()
    expect(screen.queryByRole('button', { name: /save progress/i })).not.toBeInTheDocument()
  })

  it('lets an online player start a private identity check from My Progress', async () => {
    const requests: Array<{ path: string; init?: RequestInit }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const path =
          typeof input === 'string' ? input : input instanceof URL ? input.pathname : new URL(input.url).pathname
        requests.push({ path, init })
        if (path === '/api/player-claims')
          return new Response(
            JSON.stringify({
              status: 'ready',
              challengeToken: 'private-bearer-never-rendered',
              expiresAt: new Date(Date.now() + 10 * 60_000).toISOString(),
              instructions: {
                kind: 'inventory_quiz',
                questions: [{ id: 'q1', prompt: 'What was equipped?', options: ['A', 'B', 'C'], canCycle: false }]
              }
            }),
            { status: 201, headers: { 'Content-Type': 'application/json' } }
          )
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    render(
      <PlayerClaimProvider enabled>
        <ProgressPanel
          open
          activeLayer={layer}
          players={[
            {
              id: 'opaque-public-player',
              kind: 'players',
              name: 'Moss',
              level: 58,
              x: 10,
              y: 20,
              map: 'palpagos',
              online: true
            }
          ]}
          checklist={checklist()}
          progressButtonRef={createRef<HTMLButtonElement>()}
          onClose={vi.fn()}
        />
      </PlayerClaimProvider>
    )

    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    expect(await screen.findByRole('heading', { name: 'Connect Moss' })).toBeVisible()
    expect(document.body).not.toHaveTextContent('opaque-public-player')
    expect(document.body).not.toHaveTextContent('private-bearer-never-rendered')
    const start = requests.find((request) => request.path === '/api/player-claims')
    expect(JSON.parse(String(start?.init?.body))).toEqual({ playerId: 'opaque-public-player' })
  })
})
