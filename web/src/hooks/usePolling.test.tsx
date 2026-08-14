import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { usePolling } from './usePolling'

async function flushAsyncWork() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
  })
}

describe('usePolling', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('revalidates with the response ETag and preserves state on 304', async () => {
    vi.useFakeTimers()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ value: 1 }), {
          status: 200,
          headers: { 'Content-Type': 'application/json', ETag: 'W/"snapshot-one"' }
        })
      )
      .mockResolvedValueOnce(new Response(null, { status: 304 }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderHook(() => usePolling<{ value: number }>('/api/objects', 1000))
    await flushAsyncWork()
    expect(result.current.data).toEqual({ value: 1 })

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })

    expect(fetchMock).toHaveBeenCalledTimes(2)
    const secondOptions = fetchMock.mock.calls[1][1] as RequestInit
    expect(new Headers(secondOptions.headers).get('If-None-Match')).toBe('W/"snapshot-one"')
    expect(secondOptions.cache).toBe('no-cache')
    expect(result.current.data).toEqual({ value: 1 })
    expect(result.current.error).toBeNull()
  })

  it('does not retain an ETag from a malformed response', async () => {
    vi.useFakeTimers()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response('{', { status: 200, headers: { ETag: 'W/"broken"' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ value: 2 }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const { result } = renderHook(() => usePolling<{ value: number }>('/api/objects', 1000))
    await flushAsyncWork()
    expect(result.current.error).not.toBeNull()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    await flushAsyncWork()

    expect(new Headers((fetchMock.mock.calls[1][1] as RequestInit).headers).has('If-None-Match')).toBe(false)
    expect(result.current.data).toEqual({ value: 2 })
  })
})
