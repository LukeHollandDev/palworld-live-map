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
    <header className="status-commandbar">
      <div className="server-identity">
        <span className="server-eyebrow">PALWORLD · LIVE OPS</span>
        <div className="flex min-w-0 items-center gap-2">
          <h1>{title}</h1>
          {demoMode && (
            <span className="shrink-0 rounded-full border border-[#ae8748] bg-[#3c3222] px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-[.04em] text-[#f0c87d]">
              Demo data
            </span>
          )}
          {server?.description && (
            <p className="server-description" title={server.version ? `Palworld ${server.version}` : undefined}>
              {server.description}
            </p>
          )}
        </div>
      </div>

      <div className="server-actions">
        <span className="last-sync">{age}</span>
        <button
          type="button"
          className={`server-status-card status-card-${status.kind}`}
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
          <span className="status-disclosure" aria-hidden="true">
            ›
          </span>
        </button>
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
