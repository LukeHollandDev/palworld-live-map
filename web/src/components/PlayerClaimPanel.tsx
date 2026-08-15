import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState
} from 'react'

const CLAIM_POLL_INTERVAL_MS = 30_000
const SESSION_REVALIDATE_INTERVAL_MS = 30_000
const CLAIM_HEADER = 'X-Palworld-Live-Map'
const CLAIM_PAIR_COUNT = 7
export const PLAYER_CLAIM_GLOBAL_CONTROL_ID = 'private-player-claim-control'

interface ClaimPair {
  slotA: number
  slotB: number
}

interface ClaimInstructions {
  kind: 'inventory_swap_sequence'
  phase: 'prove' | 'restore'
  step: 1 | 2
  totalSteps: 2
  pairs: ClaimPair[]
}

type ChallengePhase = 'arming' | 'ready' | 'checking' | 'pending' | 'unavailable' | 'expired' | 'proof-passed'

interface ChallengeState {
  playerId: string
  instructions: ClaimInstructions | null
  expiresAt: number
  phase: ChallengePhase
  recoveryAcknowledged: boolean
}

export type PlayerClaimSessionState =
  | { phase: 'loading' }
  | { phase: 'anonymous' }
  | { phase: 'connected'; playerId: string; sessionEpoch: number; expiresAt: number }
  | { phase: 'unavailable' }

type Notice = 'unavailable' | 'rejected' | null

interface PlayerClaimContextValue {
  enabled: boolean
  session: PlayerClaimSessionState
  challenge: ChallengeState | null
  notice: Notice
  starting: boolean
  disconnecting: boolean
  startClaim: (playerId: string) => Promise<void>
  verifyClaim: () => Promise<void>
  loadSession: () => Promise<PlayerClaimSessionState | null>
  invalidateSession: () => void
  disconnect: () => Promise<void>
  acknowledgeRecovery: () => void
}

const PlayerClaimContext = createContext<PlayerClaimContextValue | null>(null)

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function positiveInteger(value: unknown): value is number {
  return Number.isInteger(value) && Number(value) > 0
}

function parseSessionExpiry(value: Record<string, unknown>, now = Date.now()): number | null {
  if (typeof value.idleExpiresAt !== 'string' || typeof value.absoluteExpiresAt !== 'string') return null
  const idleExpiresAt = Date.parse(value.idleExpiresAt)
  const absoluteExpiresAt = Date.parse(value.absoluteExpiresAt)
  if (!Number.isFinite(idleExpiresAt) || !Number.isFinite(absoluteExpiresAt)) return null
  const expiresAt = Math.min(idleExpiresAt, absoluteExpiresAt)
  return expiresAt > now ? expiresAt : null
}

function parseInstructions(value: unknown): ClaimInstructions | null {
  if (!isRecord(value) || value.kind !== 'inventory_swap_sequence') return null
  if (value.phase !== 'prove' && value.phase !== 'restore') return null
  if (value.totalSteps !== 2 || (value.step !== 1 && value.step !== 2)) return null
  if ((value.phase === 'prove' && value.step !== 1) || (value.phase === 'restore' && value.step !== 2)) return null
  if (!Array.isArray(value.pairs) || value.pairs.length !== CLAIM_PAIR_COUNT) return null
  if (typeof value.snapshotAt !== 'string' || !Number.isFinite(Date.parse(value.snapshotAt))) return null

  const pairs: ClaimPair[] = []
  const outerSlots = new Set<number>()
  let anchor: number | null = null
  for (const rawPair of value.pairs) {
    if (!isRecord(rawPair) || !positiveInteger(rawPair.slotA) || !positiveInteger(rawPair.slotB)) return null
    if (rawPair.slotA === rawPair.slotB) return null
    if (anchor === null) anchor = rawPair.slotA
    if (rawPair.slotA !== anchor || outerSlots.has(rawPair.slotB) || rawPair.slotB === anchor) return null
    outerSlots.add(rawPair.slotB)
    pairs.push({ slotA: rawPair.slotA, slotB: rawPair.slotB })
  }
  return {
    kind: 'inventory_swap_sequence',
    phase: value.phase,
    step: value.step,
    totalSteps: 2,
    pairs
  }
}

function reverses(prove: ClaimInstructions, restore: ClaimInstructions) {
  if (prove.phase !== 'prove' || restore.phase !== 'restore' || prove.pairs.length !== restore.pairs.length)
    return false
  return restore.pairs.every((pair, index) => {
    const expected = prove.pairs[prove.pairs.length - 1 - index]
    return pair.slotA === expected.slotA && pair.slotB === expected.slotB
  })
}

function sameInstructions(left: ClaimInstructions, right: ClaimInstructions) {
  return (
    left.phase === right.phase &&
    left.step === right.step &&
    left.totalSteps === right.totalSteps &&
    left.pairs.length === right.pairs.length &&
    left.pairs.every(
      (pair, index) => pair.slotA === right.pairs[index].slotA && pair.slotB === right.pairs[index].slotB
    )
  )
}

function instructionTransition(current: ClaimInstructions | null, next: ClaimInstructions): 'same' | 'advance' | null {
  if (!current) return next.phase === 'prove' ? 'advance' : null
  if (sameInstructions(current, next)) return 'same'
  return reverses(current, next) ? 'advance' : null
}

async function responseJSON(response: Response): Promise<unknown> {
  try {
    return await response.json()
  } catch {
    return null
  }
}

function mutationInit(body: object, signal: AbortSignal): RequestInit {
  return {
    method: 'POST',
    credentials: 'same-origin',
    cache: 'no-store',
    signal,
    headers: {
      'Content-Type': 'application/json',
      [CLAIM_HEADER]: '1'
    },
    body: JSON.stringify(body)
  }
}

function buttonClass(secondary = false) {
  return `pal-glass-control pal-interactive min-h-11 cursor-pointer px-3 text-xs focus-visible:outline-none disabled:cursor-wait disabled:opacity-60 ${secondary ? 'text-[#b9c9cd]' : 'text-[#e9fbfd]'}`
}

function statusClass(tone: 'normal' | 'success' | 'warning' = 'normal') {
  if (tone === 'success') return 'm-0 text-xs leading-5 text-[#8be2ab]'
  if (tone === 'warning') return 'm-0 text-xs leading-5 text-[#efc779]'
  return 'm-0 text-xs leading-5 text-[#a9bbc0]'
}

export function PlayerClaimProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const [session, setSession] = useState<PlayerClaimSessionState>({ phase: enabled ? 'loading' : 'anonymous' })
  const [challenge, setChallenge] = useState<ChallengeState | null>(null)
  const [notice, setNotice] = useState<Notice>(null)
  const [starting, setStarting] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)
  const challengeRef = useRef<ChallengeState | null>(null)
  const challengeTokenRef = useRef<string | null>(null)
  const challengeEpochRef = useRef(0)
  const verifyControllerRef = useRef<AbortController | null>(null)
  const startControllerRef = useRef<AbortController | null>(null)
  const sessionRef = useRef<PlayerClaimSessionState>({ phase: enabled ? 'loading' : 'anonymous' })
  const sessionEpochRef = useRef(0)
  const sessionRequestRef = useRef<{ id: number; controller: AbortController } | null>(null)
  const sessionRequestIDRef = useRef(0)
  const controllersRef = useRef(new Set<AbortController>())
  const mountedRef = useRef(false)

  const controller = useCallback(() => {
    const next = new AbortController()
    controllersRef.current.add(next)
    return next
  }, [])

  const releaseController = useCallback((released: AbortController) => {
    controllersRef.current.delete(released)
  }, [])

  const commitChallenge = useCallback((next: ChallengeState | null) => {
    challengeRef.current = next
    setChallenge(next)
  }, [])

  const updateChallenge = useCallback(
    (updater: (current: ChallengeState | null) => ChallengeState | null) => {
      commitChallenge(updater(challengeRef.current))
    },
    [commitChallenge]
  )

  const resetProofPassedChallenge = useCallback(() => {
    if (challengeRef.current?.phase !== 'proof-passed') return
    challengeEpochRef.current++
    challengeTokenRef.current = null
    verifyControllerRef.current?.abort()
    verifyControllerRef.current = null
    startControllerRef.current?.abort()
    startControllerRef.current = null
    commitChallenge(null)
  }, [commitChallenge])

  const commitSession = useCallback((next: PlayerClaimSessionState) => {
    sessionRef.current = next
    setSession(next)
  }, [])

  const cancelSessionLoad = useCallback(() => {
    sessionRequestIDRef.current++
    sessionRequestRef.current?.controller.abort()
    sessionRequestRef.current = null
  }, [])

  const invalidateSession = useCallback(() => {
    cancelSessionLoad()
    commitSession({ phase: 'anonymous' })
    resetProofPassedChallenge()
  }, [cancelSessionLoad, commitSession, resetProofPassedChallenge])

  const loadSession = useCallback(async () => {
    if (!enabled) return null
    cancelSessionLoad()
    const requestController = controller()
    const requestID = sessionRequestIDRef.current
    sessionRequestRef.current = { id: requestID, controller: requestController }
    if (sessionRef.current.phase !== 'connected') commitSession({ phase: 'loading' })
    const isCurrent = () =>
      mountedRef.current &&
      !requestController.signal.aborted &&
      sessionRequestRef.current?.id === requestID &&
      sessionRequestRef.current.controller === requestController
    try {
      const response = await fetch('/api/me', {
        cache: 'no-store',
        credentials: 'same-origin',
        signal: requestController.signal
      })
      if (!isCurrent()) return null
      if (response.status === 401) {
        const next = { phase: 'anonymous' } as const
        commitSession(next)
        resetProofPassedChallenge()
        return next
      }
      if (!response.ok) {
        const next = { phase: 'unavailable' } as const
        commitSession(next)
        return next
      }
      const body = await responseJSON(response)
      if (!isCurrent()) return null
      const expiresAt = isRecord(body) ? parseSessionExpiry(body) : null
      if (
        !isRecord(body) ||
        body.authenticated !== true ||
        typeof body.playerId !== 'string' ||
        !body.playerId ||
        expiresAt === null
      ) {
        const next = { phase: 'unavailable' } as const
        commitSession(next)
        return next
      }
      const current = sessionRef.current
      const sessionEpoch =
        current.phase === 'connected' && current.playerId === body.playerId
          ? current.sessionEpoch
          : ++sessionEpochRef.current
      setNotice(null)
      const next = { phase: 'connected', playerId: body.playerId, sessionEpoch, expiresAt } as const
      commitSession(next)
      return next
    } catch {
      if (isCurrent()) {
        const next = { phase: 'unavailable' } as const
        commitSession(next)
        return next
      }
      return null
    } finally {
      if (sessionRequestRef.current?.controller === requestController) sessionRequestRef.current = null
      releaseController(requestController)
    }
  }, [cancelSessionLoad, commitSession, controller, enabled, releaseController, resetProofPassedChallenge])

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      challengeRef.current = null
      challengeTokenRef.current = null
      challengeEpochRef.current++
      sessionRequestIDRef.current++
      verifyControllerRef.current = null
      startControllerRef.current = null
      sessionRequestRef.current = null
      for (const activeController of controllersRef.current) activeController.abort()
      controllersRef.current.clear()
    }
  }, [])

  useEffect(() => {
    if (enabled) {
      void loadSession()
      const interval = window.setInterval(() => void loadSession(), SESSION_REVALIDATE_INTERVAL_MS)
      return () => window.clearInterval(interval)
    }
    cancelSessionLoad()
    challengeEpochRef.current++
    challengeTokenRef.current = null
    verifyControllerRef.current?.abort()
    verifyControllerRef.current = null
    startControllerRef.current?.abort()
    startControllerRef.current = null
    commitChallenge(null)
    commitSession({ phase: 'anonymous' })
  }, [cancelSessionLoad, commitChallenge, commitSession, enabled, loadSession])

  useEffect(() => {
    if (session.phase !== 'connected') return
    const remaining = session.expiresAt - Date.now()
    if (remaining <= 0) {
      invalidateSession()
      return
    }
    const timeout = window.setTimeout(invalidateSession, remaining)
    return () => window.clearTimeout(timeout)
  }, [invalidateSession, session])

  const expireChallenge = useCallback(() => {
    challengeEpochRef.current++
    challengeTokenRef.current = null
    verifyControllerRef.current?.abort()
    verifyControllerRef.current = null
    updateChallenge((current) =>
      current
        ? {
            ...current,
            phase: 'expired',
            recoveryAcknowledged: current.instructions === null
          }
        : current
    )
  }, [updateChallenge])

  useEffect(() => {
    if (!challenge || challenge.phase === 'expired' || challenge.phase === 'proof-passed') return
    const timeout = window.setTimeout(expireChallenge, Math.max(0, challenge.expiresAt - Date.now()))
    return () => window.clearTimeout(timeout)
  }, [challenge, expireChallenge])

  const startClaim = useCallback(
    async (playerId: string) => {
      if (
        !enabled ||
        startControllerRef.current ||
        (challenge?.phase === 'expired' && challenge.instructions && !challenge.recoveryAcknowledged)
      )
        return
      challengeEpochRef.current++
      const requestEpoch = challengeEpochRef.current
      challengeTokenRef.current = null
      verifyControllerRef.current?.abort()
      verifyControllerRef.current = null
      setStarting(true)
      setNotice(null)
      commitChallenge(null)
      const requestController = controller()
      startControllerRef.current = requestController
      const isCurrent = () =>
        mountedRef.current &&
        !requestController.signal.aborted &&
        challengeEpochRef.current === requestEpoch &&
        startControllerRef.current === requestController
      try {
        const response = await fetch('/api/player-claims', mutationInit({ playerId }, requestController.signal))
        const body = await responseJSON(response)
        if (!isCurrent()) return
        if (!response.ok) {
          setNotice(response.status === 403 ? 'rejected' : 'unavailable')
          return
        }
        if (
          response.status !== 201 ||
          !isRecord(body) ||
          typeof body.challengeToken !== 'string' ||
          body.challengeToken.length === 0 ||
          body.status !== 'arming' ||
          body.instructions !== undefined ||
          typeof body.expiresAt !== 'string'
        ) {
          setNotice('unavailable')
          return
        }
        const expiresAt = Date.parse(body.expiresAt)
        if (!Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
          setNotice('unavailable')
          return
        }
        challengeTokenRef.current = body.challengeToken
        commitChallenge({
          playerId,
          instructions: null,
          expiresAt,
          phase: 'arming',
          recoveryAcknowledged: false
        })
      } catch {
        if (isCurrent()) setNotice('unavailable')
      } finally {
        releaseController(requestController)
        if (startControllerRef.current === requestController) {
          startControllerRef.current = null
          if (mountedRef.current) setStarting(false)
        }
      }
    },
    [challenge, commitChallenge, controller, enabled, releaseController]
  )

  const verifyClaim = useCallback(async () => {
    const challengeToken = challengeTokenRef.current
    if (!challenge || challenge.phase === 'checking' || !challengeToken || verifyControllerRef.current) return
    if (challenge.expiresAt <= Date.now()) {
      expireChallenge()
      return
    }
    const requestEpoch = challengeEpochRef.current
    updateChallenge((current) => (current ? { ...current, phase: 'checking' } : current))
    const requestController = controller()
    verifyControllerRef.current = requestController
    const isCurrent = () =>
      mountedRef.current &&
      !requestController.signal.aborted &&
      challengeEpochRef.current === requestEpoch &&
      challengeTokenRef.current === challengeToken &&
      verifyControllerRef.current === requestController
    try {
      const response = await fetch(
        '/api/player-claims/verify',
        mutationInit({ challengeToken }, requestController.signal)
      )
      const body = await responseJSON(response)
      if (!isCurrent()) return
      if (response.status === 401) {
        if (challenge.instructions?.phase === 'restore') {
          const recoveredSession = await loadSession()
          if (!isCurrent()) return
          if (recoveredSession?.phase === 'connected' && recoveredSession.playerId === challenge.playerId) {
            challengeEpochRef.current++
            challengeTokenRef.current = null
            updateChallenge(() => null)
            return
          }
        }
        expireChallenge()
        return
      }
      if (!response.ok || !isRecord(body)) {
        updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
        return
      }
      if (body.status === 'arming') {
        if (response.status !== 202 || body.instructions !== undefined || challenge.instructions !== null) {
          updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
          return
        }
        updateChallenge((current) => (current ? { ...current, phase: 'arming' } : current))
        return
      }
      if (body.status === 'ready') {
        const nextInstructions = parseInstructions(body.instructions)
        const refreshedExpiresAt = typeof body.expiresAt === 'string' ? Date.parse(body.expiresAt) : Number.NaN
        const transition = nextInstructions ? instructionTransition(challenge.instructions, nextInstructions) : null
        const validTransition =
          response.status === 202 &&
          nextInstructions !== null &&
          Number.isFinite(refreshedExpiresAt) &&
          refreshedExpiresAt > Date.now() &&
          transition !== null
        if (!validTransition || !nextInstructions) {
          updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
          return
        }
        updateChallenge((current) =>
          current
            ? { ...current, instructions: nextInstructions, expiresAt: refreshedExpiresAt, phase: 'ready' }
            : current
        )
        return
      }
      if (body.status === 'pending') {
        if (response.status !== 202) {
          updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
          return
        }
        // An in-flight duplicate can return a bare pending response in either
        // phase. Keep polling without inventing instructions.
        if (body.instructions === undefined) {
          updateChallenge((current) =>
            current ? { ...current, phase: current.instructions ? 'pending' : 'arming' } : current
          )
          return
        }

        // The server replays the current sequence after a ready response is
        // lost. Validate it just as strictly as a fresh ready transition, then
        // show an unseen sequence as ready for action. A replay of the sequence
        // already shown remains pending while its backup is observed.
        const replayedInstructions = parseInstructions(body.instructions)
        const refreshedExpiresAt = typeof body.expiresAt === 'string' ? Date.parse(body.expiresAt) : Number.NaN
        const transition = replayedInstructions
          ? instructionTransition(challenge.instructions, replayedInstructions)
          : null
        if (
          !replayedInstructions ||
          transition === null ||
          !Number.isFinite(refreshedExpiresAt) ||
          refreshedExpiresAt <= Date.now()
        ) {
          updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
          return
        }
        updateChallenge((current) =>
          current
            ? {
                ...current,
                instructions: replayedInstructions,
                expiresAt: refreshedExpiresAt,
                phase: transition === 'advance' ? 'ready' : 'pending'
              }
            : current
        )
        return
      }
      if (
        body.status !== 'verified' ||
        response.status !== 200 ||
        body.instructions !== undefined ||
        challenge.instructions?.phase !== 'restore'
      ) {
        updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
        return
      }

      challengeTokenRef.current = null
      updateChallenge((current) => (current ? { ...current, phase: 'proof-passed' } : current))
      const establishedSession = await loadSession()
      if (
        establishedSession?.phase === 'connected' &&
        establishedSession.playerId === challenge.playerId &&
        challengeRef.current?.phase === 'proof-passed'
      ) {
        challengeEpochRef.current++
        updateChallenge(() => null)
      }
    } catch {
      if (isCurrent()) {
        updateChallenge((current) => (current ? { ...current, phase: 'unavailable' } : current))
      }
    } finally {
      if (verifyControllerRef.current === requestController) verifyControllerRef.current = null
      releaseController(requestController)
    }
  }, [challenge, controller, expireChallenge, loadSession, releaseController, updateChallenge])

  useEffect(() => {
    if (challenge?.phase !== 'arming' && challenge?.phase !== 'pending') return
    const timeout = window.setTimeout(() => void verifyClaim(), CLAIM_POLL_INTERVAL_MS)
    return () => window.clearTimeout(timeout)
  }, [challenge?.phase, verifyClaim])

  const disconnect = useCallback(async () => {
    if (!enabled || disconnecting) return
    cancelSessionLoad()
    setDisconnecting(true)
    setNotice(null)
    const requestController = controller()
    try {
      const response = await fetch('/api/logout', mutationInit({}, requestController.signal))
      if (requestController.signal.aborted || !mountedRef.current) return
      if (!response.ok) {
        setNotice(response.status === 403 ? 'rejected' : 'unavailable')
        return
      }
      challengeEpochRef.current++
      challengeTokenRef.current = null
      verifyControllerRef.current?.abort()
      verifyControllerRef.current = null
      startControllerRef.current?.abort()
      startControllerRef.current = null
      commitChallenge(null)
      commitSession({ phase: 'anonymous' })
    } catch {
      if (!requestController.signal.aborted && mountedRef.current) setNotice('unavailable')
    } finally {
      releaseController(requestController)
      if (mountedRef.current) setDisconnecting(false)
    }
  }, [cancelSessionLoad, commitChallenge, commitSession, controller, disconnecting, enabled, releaseController])

  const acknowledgeRecovery = useCallback(() => {
    updateChallenge((current) =>
      current?.phase === 'expired' && current.instructions ? { ...current, recoveryAcknowledged: true } : current
    )
  }, [updateChallenge])

  const value = useMemo<PlayerClaimContextValue>(
    () => ({
      enabled,
      session,
      challenge,
      notice,
      starting,
      disconnecting,
      startClaim,
      verifyClaim,
      loadSession,
      invalidateSession,
      disconnect,
      acknowledgeRecovery
    }),
    [
      acknowledgeRecovery,
      challenge,
      disconnect,
      disconnecting,
      enabled,
      invalidateSession,
      loadSession,
      notice,
      session,
      startClaim,
      starting,
      verifyClaim
    ]
  )

  return <PlayerClaimContext.Provider value={value}>{children}</PlayerClaimContext.Provider>
}

export function usePlayerClaimSession() {
  const claim = useContext(PlayerClaimContext)
  const session = claim?.session || ({ phase: 'anonymous' } as const)
  return {
    enabled: claim?.enabled === true,
    session:
      session.phase === 'connected' && session.expiresAt <= Date.now() ? ({ phase: 'anonymous' } as const) : session,
    refresh: claim?.loadSession,
    invalidate: claim?.invalidateSession
  }
}

export function PlayerClaimSessionControl() {
  const headingId = useId()
  const claim = useContext(PlayerClaimContext)
  if (!claim?.enabled) return null
  const session =
    claim.session.phase === 'connected' && claim.session.expiresAt <= Date.now()
      ? ({ phase: 'anonymous' } as const)
      : claim.session

  if (claim.challenge) {
    return (
      <section
        id={PLAYER_CLAIM_GLOBAL_CONTROL_ID}
        tabIndex={-1}
        className="pal-glass-inset mx-3.5 mb-2 grid gap-3 px-3 py-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[#72d7e5]"
        aria-labelledby={headingId}
      >
        <div className="min-w-0">
          <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
            Active private identity check
          </h3>
          <p className="m-0 mt-0.5 truncate text-[10px] tracking-[.08em] text-[#77b9c2] uppercase">
            Player {claim.challenge.playerId}
          </p>
        </div>
        <ActiveChallenge
          challenge={claim.challenge}
          session={session}
          starting={claim.starting}
          onStart={claim.startClaim}
          onVerify={claim.verifyClaim}
          onLoadSession={claim.loadSession}
          onAcknowledgeRecovery={claim.acknowledgeRecovery}
        />
      </section>
    )
  }

  if (session.phase !== 'connected') return null

  return (
    <section
      id={PLAYER_CLAIM_GLOBAL_CONTROL_ID}
      tabIndex={-1}
      className="pal-glass-inset mx-3.5 mb-2 grid gap-2 px-3 py-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[#72d7e5]"
      aria-labelledby={headingId}
    >
      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="min-w-0">
          <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
            Connected private save
          </h3>
          <p className="m-0 mt-0.5 truncate text-[10px] tracking-[.08em] text-[#77b9c2] uppercase">
            Player {session.playerId}
          </p>
        </div>
        <button
          type="button"
          className="pal-interactive min-h-8 shrink-0 cursor-pointer border border-[#8bb7bd]/25 bg-[#26363b]/55 px-2.5 text-[11px] text-[#d7e8ea] disabled:cursor-wait disabled:opacity-60"
          disabled={claim.disconnecting}
          onClick={() => void claim.disconnect()}
        >
          {claim.disconnecting ? 'Disconnecting…' : 'Disconnect'}
        </button>
      </div>
      <p role="status" aria-live="polite" aria-atomic="true" className="sr-only">
        Private save connected for player {session.playerId}.
      </p>
      {claim.notice ? <ClaimNotice notice={claim.notice} /> : null}
    </section>
  )
}

export function PlayerClaimPanel({
  playerId,
  onShowGlobalControl
}: {
  playerId: string
  onShowGlobalControl?: () => void
}) {
  const headingId = useId()
  const claim = useContext(PlayerClaimContext)
  if (!claim?.enabled) return null

  const { challenge, notice, starting } = claim
  const session =
    claim.session.phase === 'connected' && claim.session.expiresAt <= Date.now()
      ? ({ phase: 'anonymous' } as const)
      : claim.session
  const showGlobalControl = () => {
    onShowGlobalControl?.()
    window.requestAnimationFrame(() => {
      document.getElementById(PLAYER_CLAIM_GLOBAL_CONTROL_ID)?.focus({ preventScroll: false })
    })
  }

  return (
    <section aria-labelledby={headingId}>
      <h3
        id={headingId}
        className="m-0 mb-2 border-l-[3px] border-[#a8f6ff] bg-[#38494f]/80 px-2 py-1 text-xs font-normal tracking-[.08em] text-[#edf9fb] uppercase"
      >
        Private progress
      </h3>
      <div className="pal-glass-inset grid gap-3 p-3">
        {challenge ? (
          <GlobalControlPointer
            message={
              challenge.playerId === playerId
                ? 'An identity check for this player is active in Map filters.'
                : 'An identity check for a different player is active in Map filters.'
            }
            buttonLabel="Continue identity check in Map filters"
            onShow={showGlobalControl}
          />
        ) : session.phase === 'connected' ? (
          <GlobalControlPointer
            message={
              session.playerId === playerId
                ? 'Connected as this player. Manage the private session in Map filters.'
                : 'A different player is connected. Manage the private session in Map filters.'
            }
            buttonLabel="Manage private session in Map filters"
            onShow={showGlobalControl}
          />
        ) : session.phase === 'loading' ? (
          <p role="status" aria-live="polite" aria-atomic="true" className={statusClass()}>
            Checking for a private player session…
          </p>
        ) : session.phase === 'unavailable' ? (
          <>
            <p role="status" aria-live="polite" aria-atomic="true" className={statusClass('warning')}>
              Your private player session is temporarily unavailable.
            </p>
            <button type="button" className={buttonClass(true)} onClick={() => void claim.loadSession()}>
              Retry identity check
            </button>
          </>
        ) : (
          <>
            <p className="m-0 text-xs leading-5 text-[#a9bbc0]">
              Prove this is your character to privately connect save-backed completion details.
            </p>
            {notice ? <ClaimNotice notice={notice} /> : null}
            <button
              type="button"
              className={buttonClass()}
              disabled={starting}
              onClick={() => void claim.startClaim(playerId)}
            >
              {starting ? 'Preparing private check…' : 'This is me'}
            </button>
          </>
        )}
      </div>
    </section>
  )
}

function GlobalControlPointer({
  message,
  buttonLabel,
  onShow
}: {
  message: string
  buttonLabel: string
  onShow: () => void
}) {
  return (
    <>
      <p className={statusClass('success')}>{message}</p>
      <button
        type="button"
        className={buttonClass(true)}
        aria-controls={PLAYER_CLAIM_GLOBAL_CONTROL_ID}
        onClick={onShow}
      >
        {buttonLabel}
      </button>
    </>
  )
}

function ActiveChallenge({
  challenge,
  session,
  starting,
  onStart,
  onVerify,
  onLoadSession,
  onAcknowledgeRecovery
}: {
  challenge: ChallengeState
  session: PlayerClaimSessionState
  starting: boolean
  onStart: (playerId: string) => Promise<void>
  onVerify: () => Promise<void>
  onLoadSession: () => Promise<PlayerClaimSessionState | null>
  onAcknowledgeRecovery: () => void
}) {
  const instructions = challenge.instructions
  const recoveryRequired = challenge.phase === 'expired' && instructions !== null && !challenge.recoveryAcknowledged
  return (
    <>
      {challenge.phase === 'expired' && instructions ? (
        <EmergencyRecovery instructions={instructions} />
      ) : instructions ? (
        <InstructionSequence instructions={instructions} phase={challenge.phase} />
      ) : (
        <BaselineCopy />
      )}
      {challenge.phase !== 'expired' ? (
        <>
          <p className="m-0 border-l-2 border-[#64d7e7]/40 px-2 text-[11px] leading-5 text-[#9ec1c7]">
            Verification reads completed immutable backups for safety, so the baseline and each confirmed sequence can
            require two native backup intervals.
          </p>
          <p className="m-0 text-[10px] leading-4 text-[#81969c]">
            You can close and reopen this player’s details; this check stays in memory and keeps waiting until it is
            ready or expires.
          </p>
        </>
      ) : null}
      <ChallengeStatus challenge={challenge} session={session} />
      {challenge.phase !== 'expired' && challenge.phase !== 'proof-passed' ? (
        <button
          type="button"
          className={buttonClass()}
          disabled={challenge.phase === 'checking'}
          onClick={() => void onVerify()}
        >
          {challenge.phase === 'checking'
            ? instructions
              ? `Checking step ${instructions.step}…`
              : 'Checking baseline…'
            : challenge.phase === 'ready'
              ? `I completed all ${CLAIM_PAIR_COUNT} swaps`
              : challenge.phase === 'arming'
                ? 'Check baseline now'
                : challenge.phase === 'pending'
                  ? `Check step ${instructions?.step || 1} now`
                  : 'Try check again'}
        </button>
      ) : null}
      {challenge.phase === 'expired' ? (
        <>
          {instructions ? (
            <label className="flex min-h-11 cursor-pointer items-start gap-2 text-[11px] leading-5 text-[#dcebed]">
              <input
                type="checkbox"
                className="mt-1 size-3.5 shrink-0 accent-[#6cb4dd]"
                checked={challenge.recoveryAcknowledged}
                onChange={(event) => {
                  if (event.currentTarget.checked) onAcknowledgeRecovery()
                }}
              />
              <span>
                {instructions.phase === 'prove'
                  ? 'I restored the original inventory layout, or I did not perform any of the expired proof swaps.'
                  : 'I completed the restore sequence and confirmed the original inventory layout is back.'}
              </span>
            </label>
          ) : null}
          <button
            type="button"
            className={buttonClass(true)}
            disabled={starting || recoveryRequired}
            onClick={() => void onStart(challenge.playerId)}
          >
            {starting ? 'Preparing private check…' : 'Start a new check'}
          </button>
        </>
      ) : null}
      {challenge.phase === 'proof-passed' ? (
        <button
          type="button"
          className={buttonClass(true)}
          disabled={session.phase === 'loading'}
          onClick={() => void onLoadSession()}
        >
          {session.phase === 'loading' ? 'Checking session…' : 'Check session again'}
        </button>
      ) : null}
    </>
  )
}

function BaselineCopy() {
  return (
    <p className="m-0 text-xs leading-5 text-[#a9bbc0]">
      First, the map must capture a fresh private baseline. Do not move inventory stacks yet; no action has been
      assigned.
    </p>
  )
}

function EmergencyRecovery({ instructions }: { instructions: ClaimInstructions }) {
  const proving = instructions.phase === 'prove'
  const recoveryPairs = proving ? [...instructions.pairs].reverse() : instructions.pairs
  return (
    <div className="grid gap-2 border border-[#d8a95f]/45 bg-[#5a3d20]/20 p-2.5">
      <h4 className="m-0 text-xs font-semibold text-[#f1d39a]">Restore inventory before restarting</h4>
      <p className="m-0 text-[11px] leading-5 text-[#e4d2b4]">
        {proving
          ? 'If you performed all seven proof swaps, undo them using this exact top-to-bottom sequence. If you stopped partway, undo only the swaps you performed, starting with the last one.'
          : 'Finish the restore sequence from the first swap you have not yet performed. If you already completed all seven restore swaps, do not repeat them.'}
      </p>
      <ol aria-label="Emergency restore ordered inventory swaps" className="m-0 grid gap-1.5 pl-6">
        {recoveryPairs.map((pair) => (
          <li
            key={`${pair.slotA}:${pair.slotB}`}
            className="border-l-2 border-[#d8a95f]/60 bg-[#563d25]/25 px-2 py-1.5 text-xs leading-5 text-[#fff0d3] marker:text-[#efc779]"
          >
            Swap common-inventory slot {pair.slotA} with slot {pair.slotB}.
          </li>
        ))}
      </ol>
      <p className="m-0 text-[11px] leading-5 text-[#e4d2b4]">
        {proving
          ? 'If you did not perform any proof swaps from the expired step, leave your inventory as it is.'
          : 'If you had not begun the expired restore step, perform all seven swaps above in order.'}
      </p>
    </div>
  )
}

function InstructionSequence({ instructions, phase }: { instructions: ClaimInstructions; phase: ChallengePhase }) {
  const restoring = instructions.phase === 'restore'
  return (
    <>
      <div className="grid gap-1">
        <span className="text-[10px] tracking-[.1em] text-[#8fd7df] uppercase">
          Step {instructions.step} of {instructions.totalSteps} · {restoring ? 'Restore inventory' : 'Prove control'}
        </span>
        <p className="m-0 text-xs leading-5 text-[#dcebed]">
          {restoring
            ? 'Restore your inventory by performing all seven server-provided swaps in this exact top-to-bottom order.'
            : 'Perform all seven server-provided inventory swaps in this exact top-to-bottom order.'}{' '}
          Leave the result in place, then confirm the sequence below.
        </p>
      </div>
      <ol aria-label={`Step ${instructions.step} ordered inventory swaps`} className="m-0 grid gap-1.5 pl-6">
        {instructions.pairs.map((pair) => (
          <li
            key={`${pair.slotA}:${pair.slotB}`}
            className="border-l-2 border-[#79dceb]/60 bg-[#29454a]/35 px-2 py-1.5 text-xs leading-5 text-[#edf9fa] marker:text-[#8fd7df]"
          >
            Swap common-inventory slot {pair.slotA} with slot {pair.slotB}.
          </li>
        ))}
      </ol>
      {phase === 'pending' || phase === 'checking' ? (
        <p className="m-0 text-[11px] leading-5 text-[#91a6ac]">
          Keep this sequence’s result in place while the map waits for a completed backup.
        </p>
      ) : null}
    </>
  )
}

function ClaimNotice({ notice }: { notice: Exclude<Notice, null> }) {
  return (
    <p role="status" aria-live="polite" aria-atomic="true" className={statusClass('warning')}>
      {notice === 'rejected'
        ? 'This identity request was rejected. Reload the map and try again.'
        : 'Identity checks are temporarily unavailable. Please try again shortly.'}
    </p>
  )
}

function ChallengeStatus({ challenge, session }: { challenge: ChallengeState; session: PlayerClaimSessionState }) {
  const instructions = challenge.instructions
  let message = 'Waiting for a fresh immutable baseline. We’ll check again in about 30 seconds; do not act yet.'
  let tone: 'normal' | 'success' | 'warning' = 'normal'
  if (challenge.phase === 'ready' && instructions) {
    message = `Step ${instructions.step} is ready. Complete all seven swaps in order, then confirm once.`
  }
  if (challenge.phase === 'checking') {
    message = instructions
      ? `Checking a completed save backup for step ${instructions.step}…`
      : 'Checking for a completed baseline backup…'
  }
  if (challenge.phase === 'pending' && instructions) {
    message = `Step ${instructions.step} is confirmed locally, but no safe backup contains the full sequence yet. We’ll check again in about 30 seconds.`
  }
  if (challenge.phase === 'unavailable') {
    message = instructions
      ? `The saved-game check for step ${instructions.step} is temporarily unavailable. Your challenge is still active.`
      : 'The baseline check is temporarily unavailable. Your challenge is still active.'
    tone = 'warning'
  }
  if (challenge.phase === 'expired') {
    message = instructions
      ? challenge.recoveryAcknowledged
        ? 'Recovery confirmed. You can now start a new identity check.'
        : 'This identity challenge expired. Restore the original inventory layout and confirm recovery before restarting.'
      : 'This identity challenge expired. Start a new check to try again.'
    tone = 'warning'
  }
  if (challenge.phase === 'proof-passed') {
    message =
      session.phase === 'loading'
        ? 'Both ordered sequences passed. Confirming your private session…'
        : 'Both ordered sequences passed, but the private session could not be confirmed. Check the session again.'
    tone = session.phase === 'loading' ? 'success' : 'warning'
  }
  return (
    <p role="status" aria-live="polite" aria-atomic="true" className={statusClass(tone)}>
      {message}
    </p>
  )
}
