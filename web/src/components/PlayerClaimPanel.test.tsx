import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  PlayerClaimPanel,
  PlayerClaimProvider,
  PlayerClaimSessionControl,
  usePlayerClaimSession
} from './PlayerClaimPanel'

const challengeToken = 'private-challenge-bearer-that-must-not-leak'
const provePairs = [8, 11, 4, 15, 6, 13, 10].map((slotB) => ({ slotA: 3, slotB }))
const restorePairs = [...provePairs].reverse()

function pathOf(input: string | URL | Request) {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.pathname
  return new URL(input.url).pathname
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' }
  })
}

function authenticatedSession(playerId: string, idleExpiresInMs = 60 * 60_000, absoluteExpiresInMs = 2 * 60 * 60_000) {
  return {
    authenticated: true,
    playerId,
    idleExpiresAt: new Date(Date.now() + idleExpiresInMs).toISOString(),
    absoluteExpiresAt: new Date(Date.now() + absoluteExpiresInMs).toISOString()
  }
}

function startResponse(expiresInMs = 10 * 60_000, token = challengeToken) {
  return {
    challengeToken: token,
    status: 'arming',
    expiresAt: new Date(Date.now() + expiresInMs).toISOString(),
    subject: 'private-world-subject',
    playerUid: 'raw-save-player-id'
  }
}

function readyResponse(phase: 'prove' | 'restore', expiresInMs = 90 * 60_000) {
  return {
    status: 'ready',
    expiresAt: new Date(Date.now() + expiresInMs).toISOString(),
    instructions: {
      kind: 'inventory_swap_sequence',
      phase,
      step: phase === 'prove' ? 1 : 2,
      totalSteps: 2,
      pairs: phase === 'prove' ? provePairs : restorePairs,
      snapshotAt: new Date(Date.now() - 1_000).toISOString(),
      itemIds: ['ClaimSecretWood', 'ClaimSecretStone']
    }
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function pendingReplayResponse(phase: 'prove' | 'restore') {
  return { ...readyResponse(phase), status: 'pending' }
}

function renderPanel(playerId = 'player-public') {
  return render(
    <PlayerClaimProvider enabled>
      <PlayerClaimSessionControl />
      <PlayerClaimPanel playerId={playerId} onShowGlobalControl={() => undefined} />
    </PlayerClaimProvider>
  )
}

function storedBrowserData() {
  return Array.from({ length: window.localStorage.length }, (_, index) => {
    const key = window.localStorage.key(index) || ''
    return `${key}:${window.localStorage.getItem(key) || ''}`
  }).join('|')
}

async function flushRequests() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  window.localStorage.clear()
  window.history.replaceState({}, '', '/')
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('PlayerClaimPanel', () => {
  it('arms privately before revealing the ordered server sequence and never leaks the bearer', async () => {
    window.history.replaceState({}, '', '/map?focus=player-public#details')
    const originalURL = window.location.href
    window.localStorage.setItem('existing-preference', 'keep-me')
    const requests: Array<{ path: string; init?: RequestInit }> = []
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const path = pathOf(input)
        requests.push({ path, init })
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          return verifyRequests === 1
            ? jsonResponse(readyResponse('prove'), 202)
            : jsonResponse({ status: 'pending' }, 202)
        }
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel()

    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    expect(await screen.findByRole('status')).toHaveTextContent(/waiting for a fresh immutable baseline/i)
    expect(screen.getByText(/do not move inventory stacks yet; no action has been assigned/i)).toBeVisible()
    expect(screen.queryByRole('list', { name: /ordered inventory swaps/i })).not.toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('ClaimSecretWood')
    expect(document.body).not.toHaveTextContent('private-world-subject')
    expect(document.body).not.toHaveTextContent('raw-save-player-id')

    const start = requests.find((request) => request.path === '/api/player-claims')
    expect(start?.init?.credentials).toBe('same-origin')
    expect(new Headers(start?.init?.headers).get('Content-Type')).toBe('application/json')
    expect(new Headers(start?.init?.headers).get('X-Palworld-Live-Map')).toBe('1')
    expect(new Headers(start?.init?.headers).has('Origin')).toBe(false)
    expect(JSON.parse(String(start?.init?.body))).toEqual({ playerId: 'player-public' })

    await user.click(screen.getByRole('button', { name: 'Check baseline now' }))
    expect(await screen.findByText('Step 1 of 2 · Prove control')).toBeVisible()
    const swaps = screen.getByRole('list', { name: 'Step 1 ordered inventory swaps' })
    expect(within(swaps).getAllByRole('listitem')).toHaveLength(7)
    expect(within(swaps).getAllByRole('listitem')[0]).toHaveTextContent('slot 3 with slot 8')
    expect(within(swaps).getAllByRole('listitem')[6]).toHaveTextContent('slot 3 with slot 10')
    expect(document.body).not.toHaveTextContent('ClaimSecretWood')

    await user.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))
    expect(await screen.findByRole('status')).toHaveTextContent(/no safe backup contains the full sequence yet/i)
    const verifies = requests.filter((request) => request.path === '/api/player-claims/verify')
    expect(verifies).toHaveLength(2)
    for (const request of verifies) {
      expect(request.init?.credentials).toBe('same-origin')
      expect(new Headers(request.init?.headers).get('X-Palworld-Live-Map')).toBe('1')
      expect(JSON.parse(String(request.init?.body))).toEqual({ challengeToken })
    }

    expect(window.location.href).toBe(originalURL)
    expect(storedBrowserData()).toBe('existing-preference:keep-me')
    expect(document.body).not.toHaveTextContent(challengeToken)
    expect(storedBrowserData()).not.toContain(challengeToken)
    expect(requests.every((request) => !request.path.includes(challengeToken))).toBe(true)
  })

  it('polls through arming, prove, and reversed restore phases before confirming the session', async () => {
    let meRequests = 0
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') {
          meRequests++
          return meRequests === 1
            ? jsonResponse({ error: 'authentication_required' }, 401)
            : jsonResponse(authenticatedSession('player-public'))
        }
        if (path === '/api/player-claims') return jsonResponse(startResponse(60_001), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          const responses = [
            jsonResponse({ status: 'arming' }, 202),
            jsonResponse(readyResponse('prove'), 202),
            jsonResponse({ status: 'pending' }, 202),
            jsonResponse(readyResponse('restore'), 202),
            jsonResponse({ status: 'pending' }, 202),
            jsonResponse({ status: 'verified' })
          ]
          return responses[verifyRequests - 1] || new Response(null, { status: 500 })
        }
        return new Response(null, { status: 404 })
      })
    )
    renderPanel()

    const start = await screen.findByRole('button', { name: 'This is me' })
    vi.useFakeTimers()
    fireEvent.click(start)
    await flushRequests()
    expect(verifyRequests).toBe(0)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(29_999)
    })
    expect(verifyRequests).toBe(0)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(verifyRequests).toBe(1)
    expect(screen.getByRole('status')).toHaveTextContent(/waiting for a fresh immutable baseline/i)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(verifyRequests).toBe(2)
    expect(screen.getByText('Step 1 of 2 · Prove control')).toBeVisible()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(verifyRequests).toBe(2)

    fireEvent.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))
    await flushRequests()
    expect(verifyRequests).toBe(3)
    expect(screen.getByRole('status')).toHaveTextContent(/step 1 is confirmed locally/i)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(verifyRequests).toBe(4)
    expect(screen.getByText('Step 2 of 2 · Restore inventory')).toBeVisible()
    const restore = screen.getByRole('list', { name: 'Step 2 ordered inventory swaps' })
    expect(within(restore).getAllByRole('listitem')[0]).toHaveTextContent('slot 3 with slot 10')
    expect(within(restore).getAllByRole('listitem')[6]).toHaveTextContent('slot 3 with slot 8')

    fireEvent.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))
    await flushRequests()
    expect(verifyRequests).toBe(5)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(verifyRequests).toBe(6)
    expect(meRequests).toBe(2)
    expect(screen.getByRole('status')).toHaveTextContent('Private save connected for player player-public.')
    expect(document.body).not.toHaveTextContent(challengeToken)
  })

  it('recovers prove and restore instructions when each ready response is dropped', async () => {
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          if (verifyRequests === 1 || verifyRequests === 3) {
            throw new TypeError('ready response was lost after the server advanced')
          }
          if (verifyRequests === 2) return jsonResponse(pendingReplayResponse('prove'), 202)
          if (verifyRequests === 4) return jsonResponse(pendingReplayResponse('restore'), 202)
        }
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel()

    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    await user.click(screen.getByRole('button', { name: 'Check baseline now' }))
    expect(await screen.findByRole('status')).toHaveTextContent(/baseline check is temporarily unavailable/i)

    await user.click(screen.getByRole('button', { name: 'Try check again' }))
    expect(await screen.findByText('Step 1 of 2 · Prove control')).toBeVisible()
    expect(screen.getByRole('button', { name: 'I completed all 7 swaps' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))
    expect(await screen.findByRole('status')).toHaveTextContent(
      /saved-game check for step 1 is temporarily unavailable/i
    )

    await user.click(screen.getByRole('button', { name: 'Try check again' }))
    expect(await screen.findByText('Step 2 of 2 · Restore inventory')).toBeVisible()
    const restore = screen.getByRole('list', { name: 'Step 2 ordered inventory swaps' })
    expect(within(restore).getAllByRole('listitem')[0]).toHaveTextContent('slot 3 with slot 10')
    expect(within(restore).getAllByRole('listitem')[6]).toHaveTextContent('slot 3 with slot 8')
    expect(document.body).not.toHaveTextContent(challengeToken)
    expect(storedBrowserData()).not.toContain(challengeToken)
  })

  it('keeps the complete global challenge reachable when the target player disappears mid-proof', async () => {
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          return jsonResponse(readyResponse('prove'), 202)
        }
        return new Response(null, { status: 404 })
      })
    )
    function Harness({ open }: { open: boolean }) {
      return (
        <PlayerClaimProvider enabled>
          <PlayerClaimSessionControl />
          {open ? <PlayerClaimPanel playerId="player-public" onShowGlobalControl={() => undefined} /> : null}
        </PlayerClaimProvider>
      )
    }
    const view = render(<Harness open />)
    const start = await screen.findByRole('button', { name: 'This is me' })
    vi.useFakeTimers()
    fireEvent.click(start)
    await flushRequests()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(verifyRequests).toBe(1)
    expect(screen.getByText('Step 1 of 2 · Prove control')).toBeVisible()

    view.rerender(<Harness open={false} />)
    expect(screen.queryByText('Private progress')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Active private identity check' })).toBeVisible()
    expect(screen.getByText('Step 1 of 2 · Prove control')).toBeVisible()
    expect(screen.getByRole('button', { name: 'I completed all 7 swaps' })).toBeVisible()
    expect(storedBrowserData()).not.toContain(challengeToken)
  })

  it('rejects a restore phase that is not the exact reverse of the prove sequence', async () => {
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          if (verifyRequests === 1) return jsonResponse(readyResponse('prove'), 202)
          return jsonResponse(
            {
              ...readyResponse('restore'),
              instructions: {
                ...readyResponse('restore').instructions,
                pairs: provePairs
              }
            },
            202
          )
        }
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel()

    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    await user.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await screen.findByText('Step 1 of 2 · Prove control')
    await user.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))

    expect(await screen.findByRole('status')).toHaveTextContent(
      /saved-game check for step 1 is temporarily unavailable/i
    )
    expect(screen.queryByText('Step 2 of 2 · Restore inventory')).not.toBeInTheDocument()
    expect(screen.queryByText('Connected as this player.')).not.toBeInTheDocument()
  })

  it('expires accessibly and stops polling once the persistent provider unmounts', async () => {
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(1_000), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          return jsonResponse({ status: 'arming' }, 202)
        }
        return new Response(null, { status: 404 })
      })
    )
    const view = renderPanel()
    const start = await screen.findByRole('button', { name: 'This is me' })
    vi.useFakeTimers()
    fireEvent.click(start)
    await flushRequests()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000)
    })
    expect(screen.getByRole('status')).toHaveTextContent(/challenge expired/i)
    expect(screen.getByRole('button', { name: 'Start a new check' })).toBeVisible()

    view.unmount()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(verifyRequests).toBe(0)
  })

  it('aborts an expiring verify request and ignores its late response after a new challenge starts', async () => {
    const oldVerify = deferred<Response>()
    const oldToken = 'old-private-challenge-token'
    const newToken = 'new-private-challenge-token'
    let starts = 0
    let verifies = 0
    let oldVerifySignal: AbortSignal | undefined
    const verifyBodies: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: string | URL | Request, init?: RequestInit) => {
        const path = pathOf(input)
        if (path === '/api/me') return Promise.resolve(jsonResponse({ error: 'authentication_required' }, 401))
        if (path === '/api/player-claims') {
          starts++
          return Promise.resolve(
            jsonResponse(startResponse(starts === 1 ? 1_000 : 60_000, starts === 1 ? oldToken : newToken), 201)
          )
        }
        if (path === '/api/player-claims/verify') {
          verifies++
          verifyBodies.push(JSON.parse(String(init?.body)))
          if (verifies === 1) {
            oldVerifySignal = init?.signal || undefined
            return oldVerify.promise
          }
          return Promise.resolve(jsonResponse(readyResponse('prove'), 202))
        }
        return Promise.resolve(new Response(null, { status: 404 }))
      })
    )
    renderPanel()
    const start = await screen.findByRole('button', { name: 'This is me' })
    vi.useFakeTimers()
    fireEvent.click(start)
    await flushRequests()
    fireEvent.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await flushRequests()
    expect(oldVerifySignal?.aborted).toBe(false)

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000)
    })
    expect(oldVerifySignal?.aborted).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: 'Start a new check' }))
    await flushRequests()

    oldVerify.resolve(jsonResponse(readyResponse('prove'), 202))
    await flushRequests()
    expect(screen.getByText(/do not move inventory stacks yet/i)).toBeVisible()
    expect(screen.queryByText('Step 1 of 2 · Prove control')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await flushRequests()
    expect(screen.getByText('Step 1 of 2 · Prove control')).toBeVisible()
    expect(verifyBodies).toEqual([{ challengeToken: oldToken }, { challengeToken: newToken }])
    expect(document.body).not.toHaveTextContent(oldToken)
    expect(document.body).not.toHaveTextContent(newToken)
  })

  it('requires explicit emergency inventory recovery before restarting after prove expires', async () => {
    let verifies = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') return jsonResponse({ error: 'authentication_required' }, 401)
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifies++
          return verifies === 1
            ? jsonResponse(readyResponse('prove', 1_000), 202)
            : jsonResponse({ status: 'pending' }, 202)
        }
        return new Response(null, { status: 404 })
      })
    )
    renderPanel()
    const start = await screen.findByRole('button', { name: 'This is me' })
    vi.useFakeTimers()
    fireEvent.click(start)
    await flushRequests()
    fireEvent.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await flushRequests()
    fireEvent.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))
    await flushRequests()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000)
    })
    expect(screen.getByRole('heading', { name: 'Restore inventory before restarting' })).toBeVisible()
    const recovery = screen.getByRole('list', { name: 'Emergency restore ordered inventory swaps' })
    expect(within(recovery).getAllByRole('listitem')[0]).toHaveTextContent('slot 3 with slot 10')
    expect(within(recovery).getAllByRole('listitem')[6]).toHaveTextContent('slot 3 with slot 8')
    const restart = screen.getByRole('button', { name: 'Start a new check' })
    expect(restart).toBeDisabled()
    expect(screen.getByRole('status')).toHaveTextContent(/restore the original inventory layout/i)

    fireEvent.click(
      screen.getByRole('checkbox', {
        name: /I restored the original inventory layout, or I did not perform any of the expired proof swaps/i
      })
    )
    expect(restart).toBeEnabled()
    expect(screen.getByRole('status')).toHaveTextContent(/recovery confirmed/i)
    expect(storedBrowserData()).not.toContain(challengeToken)
  })

  it('increments the private session epoch when the same player reconnects', async () => {
    let meRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        meRequests++
        return jsonResponse(authenticatedSession('same-public-player'))
      })
    )
    function SessionProbe() {
      const claim = usePlayerClaimSession()
      return (
        <>
          <output>
            {claim.session.phase === 'connected'
              ? `${claim.session.playerId}:${claim.session.sessionEpoch}`
              : claim.session.phase}
          </output>
          <button type="button" onClick={() => claim.invalidate?.()}>
            Invalidate session
          </button>
          <button type="button" onClick={() => void claim.refresh?.()}>
            Reload session
          </button>
        </>
      )
    }
    render(
      <PlayerClaimProvider enabled>
        <SessionProbe />
      </PlayerClaimProvider>
    )
    expect(await screen.findByText('same-public-player:1')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Invalidate session' }))
    expect(screen.getByText('anonymous')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Reload session' }))
    expect(await screen.findByText('same-public-player:2')).toBeVisible()
    expect(meRequests).toBe(2)
  })

  it('stops rendering private session controls at the earlier known server deadline', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        if (pathOf(input) === '/api/me') return jsonResponse(authenticatedSession('connected-player', 1_000, 2_000))
        return new Response(null, { status: 404 })
      })
    )
    renderPanel('connected-player')
    await flushRequests()
    expect(screen.getByRole('heading', { name: 'Connected private save' })).toBeVisible()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(999)
    })
    expect(screen.getByRole('heading', { name: 'Connected private save' })).toBeVisible()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })
    expect(screen.queryByRole('heading', { name: 'Connected private save' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'This is me' })).toBeVisible()
  })

  it('periodically preserves the epoch for the same subject and detects another tab logout', async () => {
    vi.useFakeTimers()
    let meRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        meRequests++
        return meRequests < 3
          ? jsonResponse(authenticatedSession('same-public-player'))
          : jsonResponse({ error: 'authentication_required' }, 401)
      })
    )
    function PeriodicSessionProbe() {
      const claim = usePlayerClaimSession()
      return (
        <output>
          {claim.session.phase === 'connected'
            ? `${claim.session.playerId}:${claim.session.sessionEpoch}`
            : claim.session.phase}
        </output>
      )
    }
    render(
      <PlayerClaimProvider enabled>
        <PlayerClaimSessionControl />
        <PeriodicSessionProbe />
      </PlayerClaimProvider>
    )
    await flushRequests()
    expect(screen.getByText('same-public-player:1')).toBeVisible()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(screen.getByText('same-public-player:1')).toBeVisible()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000)
    })
    expect(screen.getByText('anonymous')).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Connected private save' })).not.toBeInTheDocument()
    expect(meRequests).toBe(3)
  })

  it('clears a proof-passed challenge when session confirmation returns 401', async () => {
    let meRequests = 0
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') {
          meRequests++
          return jsonResponse({ error: 'authentication_required' }, 401)
        }
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          if (verifyRequests === 1) return jsonResponse(readyResponse('prove'), 202)
          if (verifyRequests === 2) return jsonResponse(readyResponse('restore'), 202)
          return jsonResponse({ status: 'verified' })
        }
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel()
    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    await user.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await user.click(await screen.findByRole('button', { name: 'I completed all 7 swaps' }))
    expect(await screen.findByText('Step 2 of 2 · Restore inventory')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))

    expect(await screen.findByRole('button', { name: 'This is me' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Active private identity check' })).not.toBeInTheDocument()
    expect(screen.queryByText(/Both ordered sequences passed/)).not.toBeInTheDocument()
    expect(meRequests).toBe(2)
  })

  it('recovers a lost terminal verify response by probing the session before showing expiry recovery', async () => {
    let meRequests = 0
    let verifyRequests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request) => {
        const path = pathOf(input)
        if (path === '/api/me') {
          meRequests++
          return meRequests === 1
            ? jsonResponse({ error: 'authentication_required' }, 401)
            : jsonResponse(authenticatedSession('player-public'))
        }
        if (path === '/api/player-claims') return jsonResponse(startResponse(), 201)
        if (path === '/api/player-claims/verify') {
          verifyRequests++
          if (verifyRequests === 1) return jsonResponse(readyResponse('prove'), 202)
          if (verifyRequests === 2) return jsonResponse(readyResponse('restore'), 202)
          return jsonResponse({ error: 'invalid_or_expired_challenge' }, 401)
        }
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel()
    await user.click(await screen.findByRole('button', { name: 'This is me' }))
    await user.click(screen.getByRole('button', { name: 'Check baseline now' }))
    await user.click(await screen.findByRole('button', { name: 'I completed all 7 swaps' }))
    expect(await screen.findByText('Step 2 of 2 · Restore inventory')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'I completed all 7 swaps' }))

    expect(await screen.findByRole('heading', { name: 'Connected private save' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'Restore inventory before restarting' })).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Active private identity check' })).not.toBeInTheDocument()
    expect(meRequests).toBe(2)
  })

  it('prevents a stale identity load from restoring a session after logout', async () => {
    const staleSession = deferred<Response>()
    let meRequests = 0
    let staleSignal: AbortSignal | undefined
    vi.stubGlobal(
      'fetch',
      vi.fn((input: string | URL | Request, init?: RequestInit) => {
        const path = pathOf(input)
        if (path === '/api/me') {
          meRequests++
          if (meRequests === 1) return Promise.resolve(jsonResponse(authenticatedSession('connected-player')))
          staleSignal = init?.signal || undefined
          return staleSession.promise
        }
        if (path === '/api/logout') return Promise.resolve(jsonResponse({ authenticated: false }))
        return Promise.resolve(new Response(null, { status: 404 }))
      })
    )
    function RefreshSession() {
      const claim = usePlayerClaimSession()
      return (
        <button type="button" onClick={() => void claim.refresh?.()}>
          Refresh identity
        </button>
      )
    }
    render(
      <PlayerClaimProvider enabled>
        <RefreshSession />
        <PlayerClaimSessionControl />
        <PlayerClaimPanel playerId="connected-player" onShowGlobalControl={() => undefined} />
      </PlayerClaimProvider>
    )
    expect(await screen.findByText(/Connected as this player\. Manage the private session/)).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh identity' }))
    await flushRequests()
    expect(staleSignal?.aborted).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    await flushRequests()
    expect(staleSignal?.aborted).toBe(true)
    expect(screen.getByRole('button', { name: 'This is me' })).toBeVisible()

    staleSession.resolve(jsonResponse(authenticatedSession('connected-player')))
    await flushRequests()
    expect(screen.getByRole('button', { name: 'This is me' })).toBeVisible()
    expect(screen.queryByText(/Connected as this player\. Manage the private session/)).not.toBeInTheDocument()
  })

  it('shows the connected public player ID and disconnects using the protected mutation', async () => {
    const requests: Array<{ path: string; init?: RequestInit }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const path = pathOf(input)
        requests.push({ path, init })
        if (path === '/api/me') return jsonResponse(authenticatedSession('connected-player'))
        if (path === '/api/logout') return jsonResponse({ authenticated: false })
        return new Response(null, { status: 404 })
      })
    )
    const user = userEvent.setup()
    renderPanel('different-player')

    expect(await screen.findByRole('heading', { name: 'Connected private save' })).toBeVisible()
    expect(screen.getByRole('status')).toHaveTextContent('connected-player')
    expect(screen.getByText(/A different player is connected\. Manage the private session/)).toBeVisible()
    expect(screen.queryByRole('button', { name: 'This is me' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Disconnect' }))
    expect(await screen.findByRole('button', { name: 'This is me' })).toBeVisible()

    const logout = requests.find((request) => request.path === '/api/logout')
    expect(logout?.init?.credentials).toBe('same-origin')
    expect(new Headers(logout?.init?.headers).get('X-Palworld-Live-Map')).toBe('1')
    expect(logout?.init?.body).toBe('{}')
  })

  it('reports an unavailable session through a polite live status and can retry', async () => {
    let requests = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        requests++
        return requests === 1
          ? jsonResponse({ error: 'claim_unavailable' }, 503)
          : jsonResponse({ error: 'authentication_required' }, 401)
      })
    )
    renderPanel()

    expect(await screen.findByRole('status')).toHaveTextContent(/session is temporarily unavailable/i)
    fireEvent.click(screen.getByRole('button', { name: 'Retry identity check' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'This is me' })).toBeVisible())
  })
})
