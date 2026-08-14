import { useEffect, useState } from 'react'

export interface PollResult<T> {
  data: T | null
  error: Error | null
}

export function usePolling<T>(path: string, intervalMs: number, enabled = true): PollResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<Error | null>(null)

  useEffect(() => {
    if (!enabled) return

    const controller = new AbortController()
    let timeout: number | undefined
    let etag: string | null = null

    const refresh = async () => {
      try {
        const headers = new Headers()
        if (etag) headers.set('If-None-Match', etag)
        const response = await fetch(path, { cache: 'no-cache', headers, signal: controller.signal })
        if (response.status === 304) {
          setError(null)
          return
        }
        if (!response.ok) throw new Error(`${path} returned ${response.status}`)
        const nextData = (await response.json()) as T
        etag = response.headers.get('ETag')
        setData(nextData)
        setError(null)
      } catch (cause) {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause : new Error(String(cause)))
      } finally {
        if (!controller.signal.aborted) timeout = window.setTimeout(refresh, intervalMs)
      }
    }

    void refresh()
    return () => {
      controller.abort()
      if (timeout !== undefined) window.clearTimeout(timeout)
    }
  }, [enabled, intervalMs, path])

  return { data, error }
}
