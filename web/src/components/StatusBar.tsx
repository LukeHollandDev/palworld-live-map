import { useEffect, useState } from 'react'
import type { PlayerState } from '../types'

interface StatusBarProps {
  demoMode: boolean
  playerState: PlayerState | null
  offline: boolean
  onShowDetails: (returnFocus: HTMLElement) => void
}

function updateAge(lastSuccessAt?: string): string {
  if (!lastSuccessAt) return 'Waiting for data'
  const seconds = Math.max(0, Math.round((Date.now() - new Date(lastSuccessAt).getTime()) / 1000))
  return seconds < 2 ? 'Updated now' : `Updated ${seconds}s ago`
}

export function StatusBar({ demoMode, playerState, offline, onShowDetails }: StatusBarProps) {
  const [age, setAge] = useState(() => updateAge(playerState?.lastSuccessAt))

  useEffect(() => {
    setAge(updateAge(playerState?.lastSuccessAt))
    const timer = window.setInterval(() => setAge(updateAge(playerState?.lastSuccessAt)), 1000)
    return () => window.clearInterval(timer)
  }, [playerState?.lastSuccessAt])

  const metrics = playerState?.metricsAvailable ? playerState.metrics : null
  const playerCount = playerState?.players?.length ?? 0
  const currentPlayers = metrics?.currentPlayers ?? playerCount
  const status = offline
    ? { kind: 'offline', text: 'Map unavailable' }
    : playerState?.connected && !playerState.stale
      ? {
          kind: 'live',
          text: metrics
            ? `${currentPlayers} / ${metrics.maxPlayers} players`
            : `${playerCount} player${playerCount === 1 ? '' : 's'} online`
        }
      : playerState?.stale
        ? {
            kind: 'stale',
            text: `${currentPlayers}${metrics ? ` / ${metrics.maxPlayers}` : ''} last known`
          }
        : { kind: 'offline', text: playerState ? 'Server unavailable' : 'Connecting…' }

  const server = playerState?.server
  const title = server?.name || 'Palworld Live Map'

  return (
    <header className="relative z-20 flex min-w-0 items-center gap-4 border-b border-[#32373b] bg-[#202428] px-4 max-sm:h-12 max-sm:gap-2.5 max-sm:px-2.5">
      <div className="flex min-w-0 items-baseline gap-2">
        <h1 className="shrink-0 whitespace-nowrap text-base font-semibold max-sm:text-sm">{title}</h1>
        {demoMode && (
          <span className="shrink-0 rounded-full border border-[#ae8748] bg-[#3c3222] px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[.04em] text-[#f0c87d]">
            Demo data
          </span>
        )}
        {server?.description && (
          <p
            className="overflow-hidden text-ellipsis whitespace-nowrap text-xs text-[#899298] max-sm:hidden"
            title={server.version ? `Palworld ${server.version}` : undefined}
          >
            {server.description}
          </p>
        )}
      </div>

      <div className="flex min-w-0 items-center gap-1.5 text-[13px] text-[#b9c0c4]">
        <button
          type="button"
          className="flex min-w-0 cursor-pointer items-center gap-1.5 rounded-md px-2 py-1.5 hover:bg-[#2b3034] hover:text-[#eef0f1] focus-visible:bg-[#2b3034] focus-visible:text-[#eef0f1] focus-visible:outline-none"
          aria-haspopup="dialog"
          title="View server details"
          onClick={(event) => onShowDetails(event.currentTarget)}
        >
          <span className={`status-dot status-${status.kind}`} aria-hidden="true" />
          <span role="status" className="whitespace-nowrap">
            {status.text}
          </span>
          {metrics && (
            <span className="whitespace-nowrap max-sm:hidden">
              · {metrics.serverFps} FPS · Up {formatCompactUptime(metrics.uptimeSeconds)} · Day {metrics.days}
            </span>
          )}
        </button>
        <span className="text-[#626a6f] max-sm:hidden">·</span>
        <span className="whitespace-nowrap max-sm:hidden">{age}</span>
      </div>
    </header>
  )
}

function formatCompactUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days) return `${days}d ${hours}h ${minutes}m`
  if (hours) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
