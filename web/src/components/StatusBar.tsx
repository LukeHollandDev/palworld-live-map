import { useEffect, useState } from 'react'
import { formatUptime } from '../lib/map'
import type { PlayerState, ServerMetrics } from '../types'

interface StatusBarProps {
  playerState: PlayerState | null
  offline: boolean
}

function updateAge(lastSuccessAt?: string): string {
  if (!lastSuccessAt) return 'Waiting for data'
  const seconds = Math.max(0, Math.round((Date.now() - new Date(lastSuccessAt).getTime()) / 1000))
  return seconds < 2 ? 'Updated now' : `Updated ${seconds}s ago`
}

const metricClass =
  'grid min-w-0 content-center justify-items-center gap-0.5 px-2 text-center max-lg:px-1 max-md:min-w-[70px] max-md:px-2'

const metricTones = {
  neutral: 'text-[#e5f7f8]',
  live: 'text-[#8fe0c2]',
  stale: 'text-[#e3c894]',
  offline: 'text-[#eda69d]',
  players: 'text-[#8edceb]',
  performance: 'text-[#a7dfcf]',
  uptime: 'text-[#c5c0ee]',
  world: 'text-[#e3c894]'
} as const

type MetricTone = keyof typeof metricTones

function Metric({ label, value, tone = 'neutral' }: { label: string; value: React.ReactNode; tone?: MetricTone }) {
  return (
    <div className={metricClass}>
      <dt className="w-full overflow-hidden text-center text-ellipsis whitespace-nowrap text-[8px] font-semibold tracking-[.08em] text-[#789096] uppercase max-md:text-[8px]">
        {label}
      </dt>
      <dd
        className={`m-0 w-full overflow-hidden text-center text-ellipsis whitespace-nowrap text-xs font-medium max-md:text-[10px] ${metricTones[tone]}`}
      >
        {value}
      </dd>
    </div>
  )
}

function Metrics({ metrics, status, age }: { metrics: ServerMetrics; status: string; age: string }) {
  const statusTone: MetricTone = status === 'live' ? 'live' : status === 'stale' ? 'stale' : 'offline'
  return (
    <div className="contents max-md:col-start-1 max-md:row-start-2 max-md:flex max-md:min-w-0 max-md:overflow-x-auto max-md:overscroll-x-contain">
      <dl className="col-start-1 row-start-1 grid min-w-0 grid-cols-3 divide-x divide-white/10 max-md:flex max-md:flex-none">
        <Metric label={status} value={age} tone={statusTone} />
        <Metric label="Players" value={`${metrics.currentPlayers}/${metrics.maxPlayers}`} tone="players" />
        <Metric label="Server FPS" value={metrics.serverFps} tone="performance" />
      </dl>
      <span className="col-start-2 row-start-1 max-md:hidden" aria-hidden="true" />
      <dl className="col-start-3 row-start-1 grid min-w-0 grid-cols-4 divide-x divide-white/10 max-md:flex max-md:flex-none">
        <Metric label="Frame" value={`${metrics.serverFrameTime.toFixed(1)} ms`} tone="performance" />
        <Metric label="Uptime" value={formatUptime(metrics.uptimeSeconds)} tone="uptime" />
        <Metric label="Bases" value={metrics.baseCount} tone="world" />
        <Metric label="Day" value={metrics.days} tone="world" />
      </dl>
    </div>
  )
}

function StatusSummary({ status, age, text }: { status: string; age: string; text: string }) {
  const statusTone: MetricTone = status === 'live' ? 'live' : status === 'stale' ? 'stale' : 'offline'
  return (
    <div className="contents max-md:col-start-1 max-md:row-start-2 max-md:flex max-md:min-w-0 max-md:overflow-x-auto">
      <dl className="col-start-1 row-start-1 grid min-w-0 grid-cols-1 divide-x divide-white/10 max-md:flex-none">
        <Metric label={status} value={age} tone={statusTone} />
      </dl>
      <span className="col-start-2 row-start-1 max-md:hidden" aria-hidden="true" />
      <p className="col-start-3 row-start-1 m-0 grid min-w-0 place-items-center px-3 text-center text-xs text-[#e5f7f8] max-md:min-w-[240px]">
        {text}
      </p>
    </div>
  )
}

export function StatusBar({ playerState, offline }: StatusBarProps) {
  const [age, setAge] = useState(() => updateAge(playerState?.lastSuccessAt))

  useEffect(() => {
    setAge(updateAge(playerState?.lastSuccessAt))
    const timer = window.setInterval(() => setAge(updateAge(playerState?.lastSuccessAt)), 1000)
    return () => window.clearInterval(timer)
  }, [playerState?.lastSuccessAt])

  const metrics = playerState?.metricsAvailable && !playerState.metricsStale && !offline ? playerState.metrics : null
  const playerCount = playerState?.players?.length ?? 0
  const currentPlayers = metrics?.currentPlayers ?? playerCount
  const retainedRequestFailed = offline && Boolean(playerState)
  const stale = Boolean(playerState?.stale || playerState?.metricsStale || retainedRequestFailed)
  const status =
    offline && !playerState
      ? { kind: 'offline', text: 'Map unavailable' }
      : playerState?.connected && !stale
        ? {
            kind: 'live',
            text: metrics
              ? `${currentPlayers} / ${metrics.maxPlayers} players; server live`
              : `${playerCount} player${playerCount === 1 ? '' : 's'} online`
          }
        : stale
          ? {
              kind: 'stale',
              text: retainedRequestFailed
                ? `${playerCount} players; map connection interrupted`
                : playerState?.metricsStale && !playerState.stale
                  ? `${playerCount} players; server metrics stale`
                  : `${currentPlayers}${metrics ? ` / ${metrics.maxPlayers}` : ''} players; last known data`
            }
          : { kind: 'offline', text: playerState ? 'Server unavailable' : 'Connecting…' }

  const server = playerState?.server
  const title = server?.name || 'Palworld Live Map'
  return (
    <header className="status-commandbar relative z-30 flex min-w-0 items-center border-b border-[#bfeff6]/45 bg-[#0f1b21]/98 px-[22px] shadow-[0_8px_22px_rgb(0_0_0/24%)] max-md:px-3 max-md:py-1.5">
      <div className="relative z-[1] mx-auto grid h-[42px] w-full max-w-[1180px] min-w-0 grid-cols-[minmax(0,1fr)_clamp(280px,24vw,330px)_minmax(0,1fr)] items-stretch border border-white/15 bg-[#070f13]/60 text-center text-xs text-[#e5f7f8] max-md:h-[72px] max-md:grid-cols-1 max-md:grid-rows-[32px_40px]">
        <span role="status" className="sr-only">
          {status.text}
        </span>
        {metrics ? (
          <Metrics metrics={metrics} status={status.kind} age={age} />
        ) : (
          <StatusSummary status={status.kind} age={age} text={status.text} />
        )}
        <div className="relative z-[2] col-start-2 row-start-1 flex min-w-0 flex-col items-center justify-center border-x border-white/10 px-4 text-center max-lg:px-2 max-md:col-start-1 max-md:h-8 max-md:border-x-0 max-md:border-b">
          <h1 className="m-0 w-full overflow-hidden text-ellipsis whitespace-nowrap text-[15px] leading-4 font-normal tracking-[.035em] text-[#f2fbfc] max-md:text-sm">
            {title}
          </h1>
          {server?.description && (
            <p
              className="m-0 w-full overflow-hidden text-ellipsis whitespace-nowrap text-[9px] leading-3 text-[#9baab0]"
              title={`${server.description}${server.version ? ` · Palworld ${server.version}` : ''}`}
            >
              {server.description}
            </p>
          )}
        </div>
      </div>
    </header>
  )
}
