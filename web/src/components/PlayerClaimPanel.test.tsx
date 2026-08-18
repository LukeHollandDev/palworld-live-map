import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MapItem } from '../types'
import {
  PlayerClaimIdentityChooser,
  PlayerClaimProvider,
  PlayerClaimSessionControl,
  usePlayerClaimSession
} from './PlayerClaimPanel'

const player: MapItem = {
  id: 'offline-player',
  kind: 'players',
  name: 'Luke',
  map: 'palpagos',
  x: 1,
  y: 2,
  online: false,
  level: 79
}

function response(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  )
}

function Harness() {
  const claim = usePlayerClaimSession()
  return (
    <>
      <PlayerClaimSessionControl players={[player]} />
      <PlayerClaimIdentityChooser players={[player]} />
      <output data-testid="phase">{claim.session.phase}</output>
    </>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('question-only character connection', () => {
  it('connects an offline character with one question and keeps the bearer in memory', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock
      .mockImplementationOnce(() =>
        response(
          {
            status: 'ready',
            challengeToken: 'c'.repeat(43),
            expiresAt: '2099-01-01T00:00:00Z',
            instructions: {
              kind: 'inventory_quiz',
              snapshotAt: '2026-01-01T00:00:00Z',
              questions: [{ id: 'q1', prompt: 'What helmet was equipped?', options: ['A', 'B', 'C'], canCycle: true }]
            }
          },
          201
        )
      )
      .mockImplementationOnce(() =>
        response({
          status: 'verified',
          sessionToken: 's'.repeat(43),
          idleExpiresAt: '2099-01-01T00:00:00Z',
          absoluteExpiresAt: '2099-01-02T00:00:00Z'
        })
      )

    render(
      <PlayerClaimProvider enabled>
        <Harness />
      </PlayerClaimProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'This is me' }))
    await screen.findByText('What helmet was equipped?')
    fireEvent.change(screen.getByLabelText('What helmet was equipped?'), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect character' }))
    await screen.findByText('Connected save')
    expect(screen.getByTestId('phase')).toHaveTextContent('connected')
    expect(fetchMock.mock.calls[0][1]).not.toHaveProperty('credentials')
    expect(fetchMock.mock.calls[1][1]).not.toHaveProperty('credentials')
    expect(document.cookie).not.toContain('palworld')
  })

  it('shows an actionable explanation when no suitable question exists', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(() => response({ error: 'no_suitable_question' }, 409))
    render(
      <PlayerClaimProvider enabled>
        <Harness />
      </PlayerClaimProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'This is me' }))
    expect(await screen.findByText('This character cannot currently be connected.')).toBeInTheDocument()
    expect(screen.getByText(/Add at least three different items or Pal species/)).toBeInTheDocument()
    expect(screen.getByText(/Wait until the map has read a completed backup containing the change/)).toBeInTheDocument()
  })

  it('cycles the single question without creating another challenge', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock
      .mockImplementationOnce(() =>
        response(
          {
            status: 'ready',
            challengeToken: 'c'.repeat(43),
            expiresAt: '2099-01-01T00:00:00Z',
            instructions: {
              kind: 'inventory_quiz',
              questions: [{ id: 'q1', prompt: 'First?', options: ['A', 'B', 'C'], canCycle: true }]
            }
          },
          201
        )
      )
      .mockImplementationOnce(() =>
        response({
          status: 'ready',
          expiresAt: '2099-01-01T00:00:00Z',
          instructions: {
            kind: 'inventory_quiz',
            questions: [{ id: 'q2', prompt: 'Different question?', options: ['D', 'E', 'F'], canCycle: false }]
          }
        })
      )
    render(
      <PlayerClaimProvider enabled>
        <Harness />
      </PlayerClaimProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'This is me' }))
    await screen.findByText('First?')
    fireEvent.click(screen.getByRole('button', { name: 'Change to a different question' }))
    await screen.findByText('Different question?')
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[1][0]).toBe('/api/player-claims/questions/cycle')
  })

  it('disconnects with an Authorization bearer and clears immediately', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    fetchMock
      .mockImplementationOnce(() =>
        response(
          {
            status: 'ready',
            challengeToken: 'c'.repeat(43),
            expiresAt: '2099-01-01T00:00:00Z',
            instructions: {
              kind: 'inventory_quiz',
              questions: [{ id: 'q1', prompt: 'Question?', options: ['A', 'B', 'C'], canCycle: false }]
            }
          },
          201
        )
      )
      .mockImplementationOnce(() =>
        response({
          status: 'verified',
          sessionToken: 's'.repeat(43),
          idleExpiresAt: '2099-01-01T00:00:00Z',
          absoluteExpiresAt: '2099-01-02T00:00:00Z'
        })
      )
      .mockImplementationOnce(() => response({ authenticated: false }))
    render(
      <PlayerClaimProvider enabled>
        <Harness />
      </PlayerClaimProvider>
    )
    fireEvent.click(screen.getByRole('button', { name: 'This is me' }))
    await screen.findByText('Question?')
    fireEvent.change(screen.getByLabelText('Question?'), { target: { value: '1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Connect character' }))
    await screen.findByText('Connected save')
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    await waitFor(() => expect(screen.getByTestId('phase')).toHaveTextContent('anonymous'))
    const headers = fetchMock.mock.calls[2][1]?.headers as Record<string, string>
    expect(headers.Authorization).toBe(`Bearer ${'s'.repeat(43)}`)
  })
})
