import { useEffect, useId, useState } from 'react'
import { formatUptime } from '../lib/map'
import type { PlayerState, ServerMetrics } from '../types'

interface StatusBarProps {
  playerState: PlayerState | null
  offline: boolean
}

function updateAge(lastSuccessAt?: string): string {
  if (!lastSuccessAt) return 'Waiting for data'
  const seconds = Math.max(0, Math.round((Date.now() - new Date(lastSuccessAt).getTime()) / 1000))
  return seconds < 2 ? 'Now' : `${seconds}s ago`
}

const metricClass =
  'grid min-w-0 content-center justify-items-center gap-1 px-4 text-center max-lg:px-2 max-md:gap-0.5 max-md:px-1'

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
type TooltipAlign = 'start' | 'center' | 'end'

function Metric({
  label,
  value,
  tone = 'neutral',
  mobileHidden = false,
  tooltip,
  tooltipAlign = 'center'
}: {
  label: string
  value: React.ReactNode
  tone?: MetricTone
  mobileHidden?: boolean
  tooltip: string
  tooltipAlign?: TooltipAlign
}) {
  const tooltipId = useId()
  return (
    <>
      <button
        type="button"
        className={`${metricClass} metric-tooltip metric-tooltip-${tooltipAlign} appearance-none border-0 bg-transparent [font:inherit] ${mobileHidden ? 'max-md:hidden' : ''}`}
        data-tooltip={tooltip}
        aria-describedby={tooltipId}
      >
        <span className="w-full overflow-hidden text-center text-ellipsis whitespace-nowrap text-[11px] font-semibold tracking-[.09em] text-[#829da4] uppercase max-md:text-[10px]">
          {label}
        </span>
        <span
          className={`m-0 w-full overflow-hidden text-center text-ellipsis whitespace-nowrap text-[15px] font-medium max-md:text-xs ${metricTones[tone]}`}
        >
          {value}
        </span>
      </button>
      <span id={tooltipId} className="sr-only">
        {tooltip}
      </span>
    </>
  )
}

function Metrics({
  metrics,
  status,
  age,
  connectionAvailable,
  onlinePlayerCount
}: {
  metrics: ServerMetrics | null
  status: string
  age: string
  connectionAvailable: boolean
  onlinePlayerCount: number
}) {
  const statusTone: MetricTone = status === 'live' ? 'live' : status === 'stale' ? 'stale' : 'offline'
  const unavailable = 'N/A'
  return (
    <div className="contents max-md:col-start-1 max-md:row-start-2 max-md:grid max-md:min-w-0 max-md:grid-cols-5 max-md:divide-x max-md:divide-white/10">
      <div className="col-start-1 row-start-1 grid min-w-0 grid-cols-3 divide-x divide-white/10 max-md:contents">
        <Metric
          label={status}
          value={connectionAvailable ? age : unavailable}
          tone={statusTone}
          tooltip="Time since the map last received live player and server data successfully."
          tooltipAlign="start"
        />
        <Metric
          label="Players"
          value={
            metrics
              ? `${metrics.currentPlayers}/${metrics.maxPlayers}`
              : connectionAvailable
                ? onlinePlayerCount
                : unavailable
          }
          tone="players"
          tooltip="Players currently connected, followed by the server's maximum player capacity."
        />
        <Metric
          label="Server FPS"
          value={metrics?.serverFps ?? unavailable}
          tone="performance"
          tooltip="The server's current frames per second, as reported by Palworld."
          mobileHidden
        />
      </div>
      <span className="col-start-2 row-start-1 max-md:hidden" aria-hidden="true" />
      <div className="col-start-3 row-start-1 grid min-w-0 grid-cols-3 divide-x divide-white/10 max-md:contents">
        <Metric
          label="Uptime"
          value={metrics ? formatUptime(metrics.uptimeSeconds) : unavailable}
          tone="uptime"
          tooltip="How long the Palworld server has been running since its last start."
        />
        <Metric
          label="Bases"
          value={metrics?.baseCount ?? unavailable}
          tone="world"
          tooltip="The number of base camps currently registered on the server."
        />
        <Metric
          label="Day"
          value={metrics?.days ?? unavailable}
          tone="world"
          tooltip="The server's current in-game day count."
          tooltipAlign="end"
        />
      </div>
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

  const connectionAvailable = Boolean(playerState?.connected && !playerState.stale && !offline)
  const metrics =
    connectionAvailable && playerState?.metricsAvailable && !playerState.metricsStale ? playerState.metrics : null
  const rosterCount = playerState?.players?.length ?? 0
  const onlinePlayerCount = playerState?.players?.filter((player) => player.online !== false).length ?? 0
  const currentPlayers = metrics?.currentPlayers ?? onlinePlayerCount
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
              : `${onlinePlayerCount} player${onlinePlayerCount === 1 ? '' : 's'} online`
          }
        : stale
          ? {
              kind: 'stale',
              text: retainedRequestFailed
                ? `${onlinePlayerCount} players online; map connection interrupted`
                : playerState?.metricsStale && !playerState.stale
                  ? `${onlinePlayerCount} players online; server metrics stale`
                  : `${currentPlayers}${metrics ? ` / ${metrics.maxPlayers}` : ''} players; last known data`
            }
          : {
              kind: 'offline',
              text: playerState
                ? `${onlinePlayerCount} online of ${rosterCount} known; server unavailable`
                : 'Connecting…'
            }

  const server = playerState?.server
  const title = server?.name || 'Palworld Live Map'
  return (
    <header className="status-commandbar pointer-events-none absolute inset-x-0 top-0 z-40 flex min-w-0 px-7 pt-3 min-[1600px]:inset-x-[386px] min-[1600px]:px-0 max-md:px-2.5 max-md:pt-2">
      <div className="pal-glass-surface pointer-events-auto relative z-[1] mx-auto grid h-[54px] w-full max-w-[1240px] min-w-0 grid-cols-[minmax(0,1fr)_clamp(300px,25vw,360px)_minmax(0,1fr)] items-stretch text-center text-xs text-[#e5f7f8] min-[1600px]:max-w-none max-md:h-[70px] max-md:grid-cols-1 max-md:grid-rows-[30px_40px]">
        <span role="status" className="sr-only">
          {status.text}
        </span>
        <Metrics
          metrics={metrics}
          status={status.kind}
          age={age}
          connectionAvailable={connectionAvailable}
          onlinePlayerCount={onlinePlayerCount}
        />
        <div className="relative z-[2] col-start-2 row-start-1 flex min-w-0 flex-col items-center justify-center border-x border-white/10 px-6 text-center max-lg:px-3 max-md:col-start-1 max-md:h-[30px] max-md:border-x-0 max-md:border-b">
          <h1 className="m-0 w-full overflow-hidden text-ellipsis whitespace-nowrap text-[21px] leading-6 font-bold tracking-[.04em] text-[#f2fbfc] max-md:text-[17px] max-md:leading-5">
            {title}
          </h1>
          {server?.description && (
            <p
              className="m-0 w-full overflow-hidden text-ellipsis whitespace-nowrap text-xs leading-4 text-[#a5b7bc] max-md:hidden"
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
