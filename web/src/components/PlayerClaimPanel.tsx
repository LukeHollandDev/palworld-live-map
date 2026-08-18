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
import type { MapItem } from '../types'

export const PLAYER_CLAIM_GLOBAL_CONTROL_ID = 'private-player-claim-control'

interface QuizQuestion {
  id: string
  prompt: string
  options: string[]
  canCycle: boolean
}

interface ChallengeState {
  playerId: string
  token: string
  question: QuizQuestion
  answer: number | null
  expiresAt: number
  phase: 'ready' | 'checking' | 'unavailable' | 'expired'
}

export type PlayerClaimSessionState =
  | { phase: 'anonymous' }
  | { phase: 'connected'; playerId: string; sessionEpoch: number; expiresAt: number; bearer: string }

type Notice = 'unavailable' | 'incorrect' | 'no-suitable-question' | 'expired' | null

interface PlayerClaimContextValue {
  enabled: boolean
  session: PlayerClaimSessionState
  challenge: ChallengeState | null
  notice: Notice
  starting: boolean
  disconnecting: boolean
  cycling: boolean
  startClaim: (playerId: string) => Promise<void>
  verifyClaim: () => Promise<void>
  cycleQuestion: () => Promise<void>
  setAnswer: (answer: number) => void
  invalidate: () => void
  disconnect: () => Promise<void>
}

const PlayerClaimContext = createContext<PlayerClaimContextValue | null>(null)

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function parseQuestion(value: unknown): QuizQuestion | null {
  if (
    !isRecord(value) ||
    value.kind !== 'inventory_quiz' ||
    !Array.isArray(value.questions) ||
    value.questions.length !== 1
  )
    return null
  const question = value.questions[0]
  if (
    !isRecord(question) ||
    typeof question.id !== 'string' ||
    !question.id ||
    typeof question.prompt !== 'string' ||
    !question.prompt ||
    typeof question.canCycle !== 'boolean' ||
    !Array.isArray(question.options) ||
    question.options.length < 3 ||
    question.options.length > 8 ||
    !question.options.every((option): option is string => typeof option === 'string' && option.length > 0)
  )
    return null
  return { id: question.id, prompt: question.prompt, canCycle: question.canCycle, options: [...question.options] }
}

function parseReady(value: unknown) {
  if (!isRecord(value) || value.status !== 'ready' || typeof value.challengeToken !== 'string') return null
  const expiresAt = typeof value.expiresAt === 'string' ? Date.parse(value.expiresAt) : Number.NaN
  const question = parseQuestion(value.instructions)
  if (!question || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) return null
  return { token: value.challengeToken, question, expiresAt }
}

function parseCycled(value: unknown) {
  if (!isRecord(value) || value.status !== 'ready') return null
  const expiresAt = typeof value.expiresAt === 'string' ? Date.parse(value.expiresAt) : Number.NaN
  const question = parseQuestion(value.instructions)
  if (!question || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) return null
  return { question, expiresAt }
}

function parseVerified(value: unknown) {
  if (!isRecord(value) || value.status !== 'verified' || typeof value.sessionToken !== 'string') return null
  const idle = typeof value.idleExpiresAt === 'string' ? Date.parse(value.idleExpiresAt) : Number.NaN
  const absolute = typeof value.absoluteExpiresAt === 'string' ? Date.parse(value.absoluteExpiresAt) : Number.NaN
  const expiresAt = Math.min(idle, absolute)
  if (!Number.isFinite(expiresAt) || expiresAt <= Date.now()) return null
  return { bearer: value.sessionToken, expiresAt }
}

async function readError(response: Response) {
  try {
    const body: unknown = await response.json()
    return isRecord(body) && typeof body.error === 'string' ? body.error : ''
  } catch {
    return ''
  }
}

function postJSON(path: string, body: unknown, bearer?: string) {
  return fetch(path, {
    method: 'POST',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...(bearer ? { Authorization: `Bearer ${bearer}` } : {})
    },
    body: JSON.stringify(body)
  })
}

export function PlayerClaimProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const [session, setSession] = useState<PlayerClaimSessionState>({ phase: 'anonymous' })
  const [challenge, setChallenge] = useState<ChallengeState | null>(null)
  const [notice, setNotice] = useState<Notice>(null)
  const [starting, setStarting] = useState(false)
  const [cycling, setCycling] = useState(false)
  const [disconnecting, setDisconnecting] = useState(false)
  const epochRef = useRef(0)

  const invalidate = useCallback(() => {
    setSession({ phase: 'anonymous' })
    setChallenge(null)
  }, [])

  useEffect(() => {
    if (session.phase !== 'connected') return
    const remaining = session.expiresAt - Date.now()
    if (remaining <= 0) {
      invalidate()
      return
    }
    const timer = window.setTimeout(invalidate, Math.min(remaining, 2_147_483_647))
    return () => window.clearTimeout(timer)
  }, [invalidate, session])

  const startClaim = useCallback(
    async (playerId: string) => {
      if (!enabled || starting) return
      setStarting(true)
      setNotice(null)
      setChallenge(null)
      try {
        const response = await postJSON('/api/player-claims', { playerId })
        if (!response.ok) {
          setNotice((await readError(response)) === 'no_suitable_question' ? 'no-suitable-question' : 'unavailable')
          return
        }
        const ready = parseReady(await response.json())
        if (!ready) {
          setNotice('unavailable')
          return
        }
        setChallenge({ playerId, ...ready, answer: null, phase: 'ready' })
      } catch {
        setNotice('unavailable')
      } finally {
        setStarting(false)
      }
    },
    [enabled, starting]
  )

  const cycleQuestion = useCallback(async () => {
    if (challenge?.phase !== 'ready' || cycling || !challenge.question.canCycle) return
    setCycling(true)
    setNotice(null)
    try {
      const response = await postJSON('/api/player-claims/questions/cycle', {
        challengeToken: challenge.token,
        questionId: challenge.question.id
      })
      if (!response.ok) {
        setNotice('unavailable')
        return
      }
      const next = parseCycled(await response.json())
      if (!next) {
        setNotice('unavailable')
        return
      }
      setChallenge((current) =>
        current && current.token === challenge.token
          ? { ...current, question: next.question, expiresAt: next.expiresAt, answer: null }
          : current
      )
    } catch {
      setNotice('unavailable')
    } finally {
      setCycling(false)
    }
  }, [challenge, cycling])

  const verifyClaim = useCallback(async () => {
    if (challenge?.phase !== 'ready' || challenge.answer === null) return
    if (challenge.expiresAt <= Date.now()) {
      setChallenge({ ...challenge, phase: 'expired' })
      setNotice('expired')
      return
    }
    setChallenge({ ...challenge, phase: 'checking' })
    setNotice(null)
    try {
      const response = await postJSON('/api/player-claims/verify', {
        challengeToken: challenge.token,
        answers: [{ questionId: challenge.question.id, option: challenge.answer }]
      })
      if (!response.ok) {
        const error = await readError(response)
        setChallenge({ ...challenge, phase: 'expired' })
        setNotice(
          error === 'verification_failed'
            ? 'incorrect'
            : error === 'invalid_or_expired_challenge'
              ? 'expired'
              : 'unavailable'
        )
        return
      }
      const verified = parseVerified(await response.json())
      if (!verified) {
        setChallenge({ ...challenge, phase: 'unavailable' })
        setNotice('unavailable')
        return
      }
      epochRef.current += 1
      setSession({ phase: 'connected', playerId: challenge.playerId, sessionEpoch: epochRef.current, ...verified })
      setChallenge(null)
    } catch {
      setChallenge({ ...challenge, phase: 'unavailable' })
      setNotice('unavailable')
    }
  }, [challenge])

  const setAnswer = useCallback((answer: number) => {
    setChallenge((current) => (current?.phase === 'ready' ? { ...current, answer } : current))
  }, [])

  const disconnect = useCallback(async () => {
    if (session.phase !== 'connected' || disconnecting) return
    const bearer = session.bearer
    setDisconnecting(true)
    setSession({ phase: 'anonymous' })
    setChallenge(null)
    try {
      await postJSON('/api/logout', {}, bearer)
    } finally {
      setDisconnecting(false)
    }
  }, [disconnecting, session])

  const value = useMemo<PlayerClaimContextValue>(
    () => ({
      enabled,
      session,
      challenge,
      notice,
      starting,
      disconnecting,
      cycling,
      startClaim,
      verifyClaim,
      cycleQuestion,
      setAnswer,
      invalidate,
      disconnect
    }),
    [
      enabled,
      session,
      challenge,
      notice,
      starting,
      disconnecting,
      cycling,
      startClaim,
      verifyClaim,
      cycleQuestion,
      setAnswer,
      invalidate,
      disconnect
    ]
  )
  return <PlayerClaimContext.Provider value={value}>{children}</PlayerClaimContext.Provider>
}

export function usePlayerClaimSession() {
  const claim = useContext(PlayerClaimContext)
  return {
    enabled: claim?.enabled === true,
    session: claim?.session || ({ phase: 'anonymous' } as const),
    invalidate: claim?.invalidate
  }
}

function claimPlayerLabel(playerId: string, players: readonly MapItem[]) {
  return players.find((item) => item.kind === 'players' && item.id === playerId)?.name || `Player ${playerId}`
}

function buttonClass(secondary = false) {
  return `pal-interactive min-h-10 cursor-pointer border px-3 text-xs disabled:cursor-not-allowed disabled:opacity-60 ${secondary ? 'border-[#8bb7bd]/25 bg-[#26363b]/55 text-[#d7e8ea]' : 'border-[#69c8d5]/35 bg-[#263b41]/70 text-[#dff7fa]'}`
}

function statusClass(tone: 'normal' | 'success' | 'warning' = 'normal') {
  return `m-0 text-xs leading-5 ${tone === 'success' ? 'text-[#9be9c4]' : tone === 'warning' ? 'text-[#f2c874]' : 'text-[#a9bbc0]'}`
}

function NoSuitableQuestion() {
  return (
    <div
      role="status"
      className="grid gap-1.5 border border-[#d8a95f]/35 bg-[#5a3d20]/15 p-2.5 text-[11px] leading-5 text-[#e4d2b4]"
    >
      <strong className="text-xs text-[#f1d39a]">This character cannot currently be connected.</strong>
      <span>
        Add at least three different items or Pal species to one supported group: the first two inventory rows, loadout,
        equipment, food pouch, or party. Wait until the map has read a completed backup containing the change, then try
        again.
      </span>
    </div>
  )
}

function ClaimNotice({ notice }: { notice: Exclude<Notice, null> }) {
  if (notice === 'no-suitable-question') return <NoSuitableQuestion />
  const message =
    notice === 'incorrect'
      ? 'That answer did not match the latest completed save. Start a new check to try again.'
      : notice === 'expired'
        ? 'This check expired. Start a new one.'
        : 'Character connection is temporarily unavailable. Please try again.'
  return (
    <p role="status" aria-live="polite" className={statusClass('warning')}>
      {message}
    </p>
  )
}

function QuestionControl({ challenge }: { challenge: ChallengeState }) {
  const claim = useContext(PlayerClaimContext)
  if (!claim) return null
  const busy = challenge.phase === 'checking' || claim.cycling
  return (
    <div className="grid gap-2.5">
      <fieldset className="m-0 grid gap-2 border-0 p-0">
        <legend className="mb-1 text-xs font-medium leading-5 text-[#e5f5f7]">{challenge.question.prompt}</legend>
        <select
          className="pal-glass-inset min-h-11 px-2 text-xs text-[#e5f5f7]"
          aria-label={challenge.question.prompt}
          value={challenge.answer ?? ''}
          disabled={busy || challenge.phase !== 'ready'}
          onChange={(event) => claim.setAnswer(Number(event.currentTarget.value))}
        >
          <option value="" disabled>
            Choose an answer…
          </option>
          {challenge.question.options.map((option, index) => (
            <option key={option} value={index}>
              {option}
            </option>
          ))}
        </select>
      </fieldset>
      <button
        type="button"
        className={buttonClass(true)}
        disabled={busy || !challenge.question.canCycle}
        onClick={() => void claim.cycleQuestion()}
      >
        {claim.cycling
          ? 'Changing question…'
          : challenge.question.canCycle
            ? 'Change to a different question'
            : 'No other question available'}
      </button>
      <button
        type="button"
        className={buttonClass()}
        disabled={busy || challenge.answer === null}
        onClick={() => void claim.verifyClaim()}
      >
        {challenge.phase === 'checking' ? 'Checking…' : 'Connect character'}
      </button>
    </div>
  )
}

export function PlayerClaimSessionControl({ players = [] }: { players?: readonly MapItem[] } = {}) {
  const headingId = useId()
  const claim = useContext(PlayerClaimContext)
  if (!claim?.enabled) return null
  if (claim.challenge) {
    return (
      <section
        id={PLAYER_CLAIM_GLOBAL_CONTROL_ID}
        tabIndex={-1}
        className="pal-glass-inset mx-3.5 mb-2 grid gap-3 px-3 py-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[#72d7e5]"
        aria-labelledby={headingId}
      >
        <div>
          <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
            Connect {claimPlayerLabel(claim.challenge.playerId, players)}
          </h3>
          <p className="m-0 mt-1 text-[11px] text-[#859da2]">
            Answer one question from the character’s latest completed save.
          </p>
        </div>
        {claim.challenge.phase === 'expired' ? (
          <button
            type="button"
            className={buttonClass()}
            onClick={() => void claim.startClaim(claim.challenge?.playerId || '')}
          >
            Start a new check
          </button>
        ) : (
          <QuestionControl challenge={claim.challenge} />
        )}
        {claim.notice ? <ClaimNotice notice={claim.notice} /> : null}
      </section>
    )
  }
  if (claim.session.phase !== 'connected') return null
  return (
    <section
      id={PLAYER_CLAIM_GLOBAL_CONTROL_ID}
      tabIndex={-1}
      className="pal-glass-inset mx-3.5 mb-2 flex items-center justify-between gap-3 px-3 py-2.5 text-xs outline-none focus-visible:ring-2 focus-visible:ring-[#72d7e5]"
      aria-labelledby={headingId}
    >
      <div className="min-w-0">
        <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
          Connected save
        </h3>
        <p className="m-0 mt-0.5 truncate text-[10px] tracking-[.08em] text-[#77b9c2] uppercase">
          {claimPlayerLabel(claim.session.playerId, players)}
        </p>
      </div>
      <button
        type="button"
        className={buttonClass(true)}
        disabled={claim.disconnecting}
        onClick={() => void claim.disconnect()}
      >
        {claim.disconnecting ? 'Disconnecting…' : 'Disconnect'}
      </button>
    </section>
  )
}

export function PlayerClaimIdentityChooser({ players }: { players: readonly MapItem[] }) {
  const headingId = useId()
  const claim = useContext(PlayerClaimContext)
  if (!claim || claim.challenge || claim.session.phase === 'connected') return null
  if (!claim.enabled) {
    return (
      <section className="pal-glass-inset mx-3.5 mb-3 grid gap-1.5 px-3 py-2.5" aria-labelledby={headingId}>
        <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
          Connect your character
        </h3>
        <p className="m-0 text-[11px] leading-5 text-[#859da2]">
          Save-backed character connection is not enabled. Manual progress still works in this browser.
        </p>
      </section>
    )
  }
  const roster = players.filter((item) => item.kind === 'players')
  return (
    <section className="pal-glass-inset mx-3.5 mb-3 grid gap-2 px-3 py-2.5" aria-labelledby={headingId}>
      <div>
        <h3 id={headingId} className="m-0 text-xs font-semibold text-[#edf9fb]">
          Connect your character
        </h3>
        <p className="m-0 mt-1 text-[11px] leading-5 text-[#859da2]">
          Choose a saved character and answer one private question.
        </p>
      </div>
      {roster.length ? (
        <ul className="m-0 grid list-none gap-1.5 p-0" aria-label="Characters">
          {roster.map((player) => (
            <li
              key={player.id}
              className="flex min-w-0 items-center justify-between gap-2 border border-[#8bb7bd]/20 bg-[#182329]/65 px-2.5 py-2"
            >
              <div className="min-w-0">
                <p className="m-0 truncate text-xs font-medium text-[#e6f5f7]">{player.name}</p>
                <p className="m-0 truncate text-[10px] text-[#78949a]">
                  {player.online === false ? 'Offline · ' : 'Online · '}
                  {player.guildName ? `${player.guildName} · ` : ''}Level {player.level ?? '?'}
                </p>
              </div>
              <button
                type="button"
                className={buttonClass()}
                disabled={claim.starting}
                onClick={() => void claim.startClaim(player.id)}
              >
                {claim.starting ? 'Preparing…' : 'This is me'}
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="m-0 text-[11px] leading-5 text-[#859da2]">No saved characters are available yet.</p>
      )}
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
  const show = () => {
    onShowGlobalControl?.()
    window.requestAnimationFrame(() => document.getElementById(PLAYER_CLAIM_GLOBAL_CONTROL_ID)?.focus())
  }
  const connectedMessage =
    claim.session.phase === 'connected' && claim.session.playerId === playerId
      ? 'Connected as this character.'
      : 'A different character is connected.'
  return (
    <section aria-labelledby={headingId}>
      <h3
        id={headingId}
        className="m-0 mb-2 border-l-[3px] border-[#a8f6ff] bg-[#38494f]/80 px-2 py-1 text-xs font-normal tracking-[.08em] text-[#edf9fb] uppercase"
      >
        Private progress
      </h3>
      <div className="pal-glass-inset grid gap-3 p-3">
        {claim.challenge || claim.session.phase === 'connected' ? (
          <>
            <p className={statusClass('success')}>
              {claim.challenge ? 'A character check is open in My Progress.' : connectedMessage}
            </p>
            <button type="button" className={buttonClass(true)} onClick={show}>
              Open My Progress
            </button>
          </>
        ) : (
          <>
            <p className={statusClass()}>Connect this character to use save-backed map progress.</p>
            <button
              type="button"
              className={buttonClass()}
              disabled={claim.starting}
              onClick={() => void claim.startClaim(playerId)}
            >
              {claim.starting ? 'Preparing…' : 'This is me'}
            </button>
            {claim.notice ? <ClaimNotice notice={claim.notice} /> : null}
          </>
        )}
      </div>
    </section>
  )
}
