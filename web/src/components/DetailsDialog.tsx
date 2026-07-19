import { useEffect, useRef } from 'react'
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
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    const dialog = dialogRef.current
    if (!dialog || !detail) return
    if (!dialog.open) dialog.showModal()
  }, [detail])

  const close = () => dialogRef.current?.close()
  const finishClose = () => {
    onClose()
    window.requestAnimationFrame(() => returnFocus?.isConnected && returnFocus.focus({ preventScroll: true }))
  }

  return (
    <dialog
      ref={dialogRef}
      className="m-auto max-h-[min(680px,calc(100vh-32px))] w-[min(520px,calc(100vw-28px))] overflow-hidden rounded-xl border border-[#41484c] bg-[#191d20] p-0 text-[#f4f5f5] shadow-[0_24px_80px_rgb(0_0_0/55%)] backdrop:bg-black/55"
      aria-labelledby="details-title"
      onClose={finishClose}
      onCancel={(event) => {
        event.preventDefault()
        close()
      }}
      onClick={(event) => {
        if (event.target !== event.currentTarget) return
        const bounds = event.currentTarget.getBoundingClientRect()
        const inside =
          event.clientX >= bounds.left &&
          event.clientX <= bounds.right &&
          event.clientY >= bounds.top &&
          event.clientY <= bounds.bottom
        if (!inside) close()
      }}
      onKeyDown={(event) => {
        if (event.key === 'Escape') close()
      }}
    >
      {detail && (
        <article className="max-h-[inherit] overflow-auto">
          <header className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-[#343a3e] bg-[#202529] px-5 py-4">
            <div>
              <p className="mb-1 text-[11px] font-bold uppercase tracking-[.08em] text-[#8e999f]">
                {detail.kind === 'server' ? 'Server' : kindLabel(detail.item.kind)}
              </p>
              <h2 id="details-title" className="text-xl font-semibold">
                {detail.kind === 'server' ? playerState?.server.name || 'Palworld server' : detail.item.name}
              </h2>
            </div>
            <button
              type="button"
              className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-xl text-[#adb5b9] hover:bg-[#343a3e] hover:text-white focus-visible:outline-2 focus-visible:outline-[#76b5db]"
              aria-label="Close details"
              onClick={close}
            >
              ×
            </button>
          </header>
          <div className="grid gap-5 p-5">
            {detail.kind === 'server' ? (
              <ServerDetails playerState={playerState} />
            ) : (
              <ItemDetails item={detail.item} items={items} layers={layers} />
            )}
          </div>
        </article>
      )}
    </dialog>
  )
}

function FactList({ entries }: { entries: Array<[string, string | number | undefined]> }) {
  return (
    <dl className="grid grid-cols-[minmax(110px,.7fr)_minmax(0,1fr)] gap-x-4 gap-y-2 text-[13px]">
      {entries.map(([label, value]) =>
        value === undefined || value === '' ? null : (
          <div className="contents" key={label}>
            <dt className="text-[#929ca1]">{label}</dt>
            <dd className="m-0 text-right text-[#eef0f1]">{value}</dd>
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
      {server?.description && <p className="m-0 text-sm leading-6 text-[#c7cdd0]">{server.description}</p>}
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
          <h3 className="mb-2 text-sm font-semibold">Base workers</h3>
          {workers.length === 0 ? (
            <p className="m-0 text-[13px] text-[#8f989d]">No workers are currently reported for this base.</p>
          ) : (
            <ul className="grid list-none gap-1.5 p-0">
              {workers
                .slice()
                .sort((left, right) => left.name.localeCompare(right.name))
                .map((worker) => (
                  <li
                    className="flex justify-between gap-3.5 rounded-md border border-[#363c40] bg-[#202529] px-3 py-2 text-[13px]"
                    key={worker.name}
                  >
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
