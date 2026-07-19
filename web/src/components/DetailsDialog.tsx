import { useEffect } from 'react'
import { formatUptime, kindLabel } from '../lib/map'
import type { MapItem, MapLayer, PlayerState } from '../types'

export type Detail = { kind: 'server' } | { kind: 'item'; item: MapItem }

interface DetailsDialogProps {
  detail: Detail | null
  items: MapItem[]
  layers: MapLayer[]
  playerState: PlayerState | null
  returnFocus: HTMLElement | null
  onClose: () => void
}

export function DetailsDialog({ detail, items, layers, playerState, returnFocus, onClose }: DetailsDialogProps) {
  useEffect(() => {
    if (!detail) return
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return
      onClose()
      window.requestAnimationFrame(() => returnFocus?.isConnected && returnFocus.focus({ preventScroll: true }))
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [detail, onClose, returnFocus])

  if (!detail) return null

  const close = () => {
    onClose()
    window.requestAnimationFrame(() => returnFocus?.isConnected && returnFocus.focus({ preventScroll: true }))
  }

  return (
    <aside className="details-inspector" role="dialog" aria-modal="false" aria-labelledby="details-title">
      <article>
        <header className="inspector-header">
          <div>
            <p>{detail.kind === 'server' ? 'SYSTEM TELEMETRY' : `${kindLabel(detail.item.kind)} INTELLIGENCE`}</p>
            <h2 id="details-title">
              {detail.kind === 'server' ? playerState?.server.name || 'Palworld server' : detail.item.name}
            </h2>
          </div>
          <button type="button" className="inspector-close" aria-label="Close details" onClick={close}>
            ×
          </button>
        </header>
        <div className="inspector-body">
          {detail.kind === 'server' ? (
            <ServerDetails playerState={playerState} />
          ) : (
            <ItemDetails item={detail.item} items={items} layers={layers} />
          )}
        </div>
      </article>
    </aside>
  )
}

function FactList({ entries }: { entries: Array<[string, string | number | undefined]> }) {
  return (
    <dl className="fact-list">
      {entries.map(([label, value]) =>
        value === undefined || value === '' ? null : (
          <div className="contents" key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        )
      )}
    </dl>
  )
}

function ServerDetails({ playerState }: { playerState: PlayerState | null }) {
  const server = playerState?.server
  const metrics = playerState?.metricsAvailable ? playerState.metrics : null
  const entries: Array<[string, string | number | undefined]> = metrics
    ? [
        ['Players', `${metrics.currentPlayers} / ${metrics.maxPlayers}`],
        ['Server FPS', metrics.serverFps],
        ['Average FPS', metrics.averageFps.toFixed(1)],
        ['Frame time', `${metrics.serverFrameTime.toFixed(2)} ms`],
        ['Uptime', formatUptime(metrics.uptimeSeconds)],
        ['Base camps', metrics.baseCount],
        ['In-game day', metrics.days]
      ]
    : [['Metrics', 'Temporarily unavailable']]
  if (server?.version) entries.push(['Version', server.version])
  if (playerState?.metricsUpdatedAt)
    entries.push(['Metrics updated', new Date(playerState.metricsUpdatedAt).toLocaleString()])

  return (
    <>
      {server?.description && <p className="inspector-copy">{server.description}</p>}
      <FactList entries={entries} />
    </>
  )
}

function ItemDetails({ item, items, layers }: { item: MapItem; items: MapItem[]; layers: MapLayer[] }) {
  const base = item.baseId
    ? items.find((candidate) => candidate.kind === 'bases' && candidate.baseId === item.baseId)
    : undefined
  const workers =
    item.kind === 'bases'
      ? items.filter((candidate) => candidate.kind === 'workers' && candidate.baseId === item.baseId)
      : []
  const entries: Array<[string, string | number | undefined]> = []
  if (item.level) entries.push(['Level', item.level])
  if (item.detail && item.kind !== 'players') entries.push([item.kind === 'npcs' ? 'Type' : 'Species', item.detail])
  if (item.kind === 'workers' && base) entries.push(['Assigned base', base.name])
  if (item.kind === 'bases') entries.push(['Workers', workers.length])
  entries.push(['Region', layers.find((layer) => layer.id === item.map)?.name || item.map])
  entries.push(['Coordinates', `X ${Math.round(item.x)} · Y ${Math.round(item.y)}`])

  return (
    <>
      <FactList entries={entries} />
      {item.kind === 'bases' && (
        <section>
          <h3 className="inspector-section-title">Base workers</h3>
          {workers.length === 0 ? (
            <p className="m-0 text-[13px] text-[#8f989d]">No workers are currently reported for this base.</p>
          ) : (
            <ul className="grid list-none gap-1.5 p-0">
              {workers
                .slice()
                .sort((left, right) => left.name.localeCompare(right.name))
                .map((worker) => (
                  <li className="inspector-list-row" key={worker.name}>
                    <span>{worker.level ? `${worker.name} · Lv ${worker.level}` : worker.name}</span>
                    <span className="text-right text-[#9fa7ab]">{worker.detail || 'Pal'}</span>
                  </li>
                ))}
            </ul>
          )}
        </section>
      )}
    </>
  )
}
