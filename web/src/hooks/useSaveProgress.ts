import { useCallback, useEffect, useRef, useState } from 'react'
import {
  isSaveProgressStale,
  parseSaveProgress,
  SAVE_PROGRESS_STALE_AFTER_MS,
  type SaveProgressSnapshot
} from '../lib/saveProgress'

export type SaveProgressState =
  | { phase: 'inactive' }
  | { phase: 'loading'; playerId: string; sessionEpoch: number; requestAttempt: number }
  | {
      phase: 'available'
      playerId: string
      sessionEpoch: number
      requestAttempt: number
      snapshot: SaveProgressSnapshot
      stale: boolean
      refreshing: boolean
      refreshFailed: boolean
    }
  | {
      phase: 'unavailable'
      playerId: string
      sessionEpoch: number
      requestAttempt: number
      reason: 'request' | 'catalogue-version'
    }

export type SaveProgressSession =
  | { phase: 'loading' | 'anonymous' | 'unavailable' }
  | { phase: 'connected'; playerId: string; sessionEpoch: number }

interface SaveProgressOptions {
  expectedCatalogueVersion: string | null
  onUnauthorized?: () => void
}

function belongsToSession(
  state: SaveProgressState,
  playerId: string,
  sessionEpoch: number
): state is Exclude<SaveProgressState, { phase: 'inactive' }> {
  return 'playerId' in state && state.playerId === playerId && state.sessionEpoch === sessionEpoch
}

export function useSaveProgress(session: SaveProgressSession, options: SaveProgressOptions) {
  const [attempt, setAttempt] = useState(0)
  const [loaded, setLoaded] = useState<SaveProgressState>({ phase: 'inactive' })
  const loadedRef = useRef<SaveProgressState>(loaded)
  const connectedPlayerId = session.phase === 'connected' ? session.playerId : null
  const connectedSessionEpoch = session.phase === 'connected' ? session.sessionEpoch : null

  const commit = useCallback((next: SaveProgressState) => {
    loadedRef.current = next
    setLoaded(next)
  }, [])

  useEffect(() => {
    if (connectedPlayerId === null || connectedSessionEpoch === null) {
      commit({ phase: 'inactive' })
      return
    }

    const playerId = connectedPlayerId
    const sessionEpoch = connectedSessionEpoch
    const requestAttempt = attempt
    const previous = loadedRef.current
    const retained =
      previous.phase === 'available' &&
      belongsToSession(previous, playerId, sessionEpoch) &&
      previous.snapshot.catalogueVersion === options.expectedCatalogueVersion
        ? previous
        : null

    if (!options.expectedCatalogueVersion) {
      commit({
        phase: 'unavailable',
        playerId,
        sessionEpoch,
        requestAttempt,
        reason: 'catalogue-version'
      })
      return
    }

    const controller = new AbortController()
    let staleTimer: ReturnType<typeof setTimeout> | undefined
    const currentRequest = () => !controller.signal.aborted
    const armStaleTimer = (snapshot: SaveProgressSnapshot) => {
      if (staleTimer) clearTimeout(staleTimer)
      if (isSaveProgressStale(snapshot)) return
      const staleIn = Date.parse(snapshot.snapshotAt) + SAVE_PROGRESS_STALE_AFTER_MS - Date.now()
      staleTimer = setTimeout(() => {
        if (!currentRequest()) return
        const current = loadedRef.current
        if (
          current.phase === 'available' &&
          current.playerId === playerId &&
          current.sessionEpoch === sessionEpoch &&
          current.snapshot === snapshot
        ) {
          commit({ ...current, stale: true })
        }
      }, Math.max(0, staleIn) + 1)
    }

    if (retained) {
      commit({
        ...retained,
        requestAttempt,
        stale: isSaveProgressStale(retained.snapshot),
        refreshing: true,
        refreshFailed: false
      })
      armStaleTimer(retained.snapshot)
    } else {
      commit({ phase: 'loading', playerId, sessionEpoch, requestAttempt })
    }

    const load = async () => {
      try {
        const response = await fetch('/api/me/progress', {
          credentials: 'same-origin',
          cache: 'no-store',
          signal: controller.signal
        })
        if (!currentRequest()) return
        if (response.status === 401) {
          commit({ phase: 'inactive' })
          options.onUnauthorized?.()
          return
        }
        if (!response.ok) throw new Error(`/api/me/progress returned ${response.status}`)
        const snapshot = parseSaveProgress(await response.json())
        if (!snapshot) throw new Error('/api/me/progress returned an invalid payload')
        if (!currentRequest()) return
        if (snapshot.catalogueVersion !== options.expectedCatalogueVersion) {
          if (retained) {
            commit({
              ...retained,
              requestAttempt,
              stale: isSaveProgressStale(retained.snapshot),
              refreshing: false,
              refreshFailed: true
            })
            armStaleTimer(retained.snapshot)
            return
          }
          commit({
            phase: 'unavailable',
            playerId,
            sessionEpoch,
            requestAttempt,
            reason: 'catalogue-version'
          })
          return
        }
        const stale = isSaveProgressStale(snapshot)
        commit({
          phase: 'available',
          playerId,
          sessionEpoch,
          requestAttempt,
          snapshot,
          stale,
          refreshing: false,
          refreshFailed: false
        })
        armStaleTimer(snapshot)
      } catch {
        if (!currentRequest()) return
        if (retained) {
          commit({
            ...retained,
            requestAttempt,
            stale: isSaveProgressStale(retained.snapshot),
            refreshing: false,
            refreshFailed: true
          })
          armStaleTimer(retained.snapshot)
          return
        }
        commit({ phase: 'unavailable', playerId, sessionEpoch, requestAttempt, reason: 'request' })
      }
    }
    void load()
    return () => {
      controller.abort()
      if (staleTimer) clearTimeout(staleTimer)
    }
  }, [
    attempt,
    commit,
    connectedPlayerId,
    connectedSessionEpoch,
    options.expectedCatalogueVersion,
    options.onUnauthorized
  ])

  const retry = useCallback(() => {
    if (loadedRef.current.phase === 'loading') return
    if (loadedRef.current.phase === 'available' && loadedRef.current.refreshing) return
    setAttempt((current) => current + 1)
  }, [])

  // The effect performs the durable cleanup. This synchronous boundary makes
  // the very first render after logout or same-ID reconnect private-data safe.
  if (connectedPlayerId === null || connectedSessionEpoch === null)
    return { state: { phase: 'inactive' } as const, retry }
  if ('playerId' in loaded && !belongsToSession(loaded, connectedPlayerId, connectedSessionEpoch)) {
    return {
      state: {
        phase: 'loading',
        playerId: connectedPlayerId,
        sessionEpoch: connectedSessionEpoch,
        requestAttempt: attempt
      } as const,
      retry
    }
  }
  return { state: loaded, retry }
}
