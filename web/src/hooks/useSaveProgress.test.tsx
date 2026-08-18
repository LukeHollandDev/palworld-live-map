import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LOCAL_COMPLETION_STORAGE_KEY } from '../lib/completion'
import { type SaveProgressSession, useSaveProgress } from './useSaveProgress'

const responsePayload = {
  snapshotAt: '2026-08-15T10:00:00Z',
  catalogueVersion: 'catalogue-content-hash',
  domains: [
    { id: 'alpha-pals', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'bosses', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'bounties', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'watchtowers', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'waypoints', coverage: 'complete', completedIds: ['private-transient-waypoint'], total: 1 },
    { id: 'effigies', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'journals', coverage: 'complete', completedIds: [], total: 0 },
    { id: 'ancient-shrine-pickups', coverage: 'complete', completedIds: [], total: 0 }
  ]
}

const options = { expectedCatalogueVersion: 'catalogue-content-hash' }

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  window.localStorage.clear()
})

describe('useSaveProgress', () => {
  it('loads only for a connected claim and drops the overlay on disconnect without touching manual storage', async () => {
    window.localStorage.setItem(LOCAL_COMPLETION_STORAGE_KEY, '{"manual":"keep"}')
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(responsePayload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    let session: SaveProgressSession = {
      phase: 'connected',
      playerId: 'public-player',
      sessionEpoch: 1,
      bearer: 'session-token'
    }
    const { result, rerender } = renderHook(() => useSaveProgress(session, options))

    await waitFor(() => expect(result.current.state.phase).toBe('available'))
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/me/progress',
      expect.objectContaining({ cache: 'no-store', headers: { Authorization: 'Bearer session-token' } })
    )

    session = { phase: 'anonymous' }
    rerender()
    expect(result.current.state).toEqual({ phase: 'inactive' })
    expect(window.localStorage.getItem(LOCAL_COMPLETION_STORAGE_KEY)).toBe('{"manual":"keep"}')
    expect(JSON.stringify(window.localStorage)).not.toContain('private-transient-waypoint')
  })

  it('shows unavailable state and can retry without exposing a previous snapshot', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{"error":"progress_unavailable"}', { status: 503 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(responsePayload), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useSaveProgress(
        { phase: 'connected', playerId: 'public-player', sessionEpoch: 1, bearer: 'session-token' },
        options
      )
    )

    await waitFor(() => expect(result.current.state).toMatchObject({ phase: 'unavailable' }))
    act(() => result.current.retry())
    await waitFor(() => expect(result.current.state.phase).toBe('available'))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('marks old snapshots stale while retaining their exact completion data', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-15T10:31:00Z'))
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify(responsePayload), { status: 200 }))
    )
    const { result } = renderHook(() =>
      useSaveProgress(
        { phase: 'connected', playerId: 'public-player', sessionEpoch: 1, bearer: 'session-token' },
        options
      )
    )

    await act(async () => {
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(result.current.state).toMatchObject({
      phase: 'available',
      stale: true,
      snapshot: { catalogueVersion: 'catalogue-content-hash' }
    })
  })

  it('invalidates the provider session when progress authentication expires', async () => {
    const onUnauthorized = vi.fn()
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('{"error":"authentication_required"}', { status: 401 }))
    )
    const { result } = renderHook(() =>
      useSaveProgress(
        { phase: 'connected', playerId: 'public-player', sessionEpoch: 1, bearer: 'session-token' },
        { ...options, onUnauthorized }
      )
    )

    await waitFor(() => expect(onUnauthorized).toHaveBeenCalledTimes(1))
    expect(result.current.state).toEqual({ phase: 'inactive' })
  })

  it('does not reuse private progress when the same public player ID reconnects in a new session epoch', async () => {
    const second = deferred<Response>()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(responsePayload), { status: 200 }))
      .mockImplementationOnce(() => second.promise)
    vi.stubGlobal('fetch', fetchMock)
    let session: SaveProgressSession = {
      phase: 'connected',
      playerId: 'public-player',
      sessionEpoch: 1,
      bearer: 'session-token'
    }
    const { result, rerender } = renderHook(() => useSaveProgress(session, options))
    await waitFor(() => expect(result.current.state.phase).toBe('available'))

    session = { phase: 'anonymous' }
    rerender()
    expect(result.current.state).toEqual({ phase: 'inactive' })

    session = { phase: 'connected', playerId: 'public-player', sessionEpoch: 2, bearer: 'new-session-token' }
    rerender()
    expect(result.current.state).toMatchObject({ phase: 'loading', sessionEpoch: 2 })
    expect(result.current.state).not.toHaveProperty('snapshot')

    second.resolve(new Response(JSON.stringify(responsePayload), { status: 200 }))
    await waitFor(() => expect(result.current.state).toMatchObject({ phase: 'available', sessionEpoch: 2 }))
  })

  it('keeps the last valid overlay applied while refresh is pending or fails', async () => {
    const refresh = deferred<Response>()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(responsePayload), { status: 200 }))
      .mockImplementationOnce(() => refresh.promise)
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() =>
      useSaveProgress(
        { phase: 'connected', playerId: 'public-player', sessionEpoch: 1, bearer: 'session-token' },
        options
      )
    )
    await waitFor(() => expect(result.current.state.phase).toBe('available'))

    act(() => result.current.retry())
    await waitFor(() => expect(result.current.state).toMatchObject({ phase: 'available', refreshing: true }))
    expect(
      result.current.state.phase === 'available'
        ? result.current.state.snapshot.domains.find((domain) => domain.id === 'waypoints')?.completedIds[0]
        : undefined
    ).toBe('private-transient-waypoint')

    refresh.resolve(new Response('{"error":"progress_unavailable"}', { status: 503 }))
    await waitFor(() =>
      expect(result.current.state).toMatchObject({ phase: 'available', refreshing: false, refreshFailed: true })
    )
    expect(
      result.current.state.phase === 'available'
        ? result.current.state.snapshot.domains.find((domain) => domain.id === 'waypoints')?.completedIds[0]
        : undefined
    ).toBe('private-transient-waypoint')
  })

  it('rejects progress produced for a different catalogue hash', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(JSON.stringify({ ...responsePayload, catalogueVersion: 'different-catalogue' }), { status: 200 })
      )
    )
    const { result } = renderHook(() =>
      useSaveProgress(
        { phase: 'connected', playerId: 'public-player', sessionEpoch: 1, bearer: 'session-token' },
        options
      )
    )

    await waitFor(() =>
      expect(result.current.state).toMatchObject({ phase: 'unavailable', reason: 'catalogue-version' })
    )
    expect(result.current.state).not.toHaveProperty('snapshot')
  })
})
