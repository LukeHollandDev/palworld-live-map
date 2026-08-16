import { type RefObject, useEffect, useId, useRef } from 'react'
import type { SaveProgressState } from '../hooks/useSaveProgress'
import { formatSaveProgressAge } from '../lib/saveProgress'
import type { MapItem, MapLayer } from '../types'
import { MapPanelHeader, MapPanelShell } from './MapPanel'
import { PlayerClaimIdentityChooser, PlayerClaimSessionControl } from './PlayerClaimPanel'

export interface ProgressChecklistView {
  profileName: string
  completed: number
  total: number
  remaining: number
  remainingOnly: boolean
  saveProgress: SaveProgressState
  onRetrySaveProgress: () => void
  onRemainingOnlyChange: (remainingOnly: boolean) => void
}

interface ProgressPanelProps {
  open: boolean
  activeLayer: MapLayer
  players: readonly MapItem[]
  checklist: ProgressChecklistView
  progressButtonRef: RefObject<HTMLButtonElement | null>
  onClose: () => void
}

function saveProgressDescription(progress: SaveProgressState) {
  if (progress.phase === 'loading') return 'Loading your private save progress…'
  if (progress.phase === 'unavailable')
    return progress.reason === 'catalogue-version'
      ? 'Your save does not match this map catalogue. Manual marks still count.'
      : 'Save progress is temporarily unavailable. Manual marks still count.'
  if (progress.phase === 'inactive') return 'Manual checklist on this browser'
  const age = formatSaveProgressAge(progress.snapshot.snapshotAt)
  if (progress.refreshing) return `Refreshing save progress; keeping the snapshot from ${age}.`
  if (progress.refreshFailed) return `Refresh failed; keeping the snapshot from ${age}.`
  return progress.stale ? `Save snapshot from ${age} may be stale.` : `Save-confirmed · ${age}`
}

export function ProgressPanel({
  open,
  activeLayer,
  players,
  checklist,
  progressButtonRef,
  onClose
}: ProgressPanelProps) {
  const titleId = useId()
  const filterDescriptionId = useId()
  const titleRef = useRef<HTMLHeadingElement>(null)
  const closeRef = useRef<HTMLButtonElement>(null)
  const wasOpenRef = useRef(open)
  const percent = checklist.total > 0 ? Math.round((checklist.completed / checklist.total) * 100) : 0
  const progress = checklist.saveProgress

  useEffect(() => {
    if (open && !wasOpenRef.current) window.requestAnimationFrame(() => closeRef.current?.focus())
    if (!open && wasOpenRef.current) window.requestAnimationFrame(() => progressButtonRef.current?.focus())
    wasOpenRef.current = open
  }, [open, progressButtonRef])

  return (
    <MapPanelShell
      id="progress-panel"
      side="right"
      mobileSize="fixed"
      mobileSheetActive={open}
      mobileSheetLabel="my progress"
      className={`filter-panel-motion max-sm:z-[34] ${open ? 'is-panel-open' : 'is-panel-closed pointer-events-none'}`}
      aria-labelledby={titleId}
      aria-hidden={!open}
      inert={!open}
    >
      <MapPanelHeader
        as="div"
        eyebrow="MY MAP"
        title="My Progress"
        titleId={titleId}
        titleRef={titleRef}
        closeButtonRef={closeRef}
        closeLabel="Close My Progress"
        closeControls="progress-panel"
        closeExpanded
        onClose={onClose}
      />

      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain py-3">
        <section className="pal-glass-inset mx-3.5 mb-3 grid gap-3 px-3 py-3" aria-label="Exploration progress">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="m-0 truncate text-[10px] tracking-[.1em] text-[#77b9c2] uppercase">
                {checklist.profileName}
              </p>
              <h3 className="m-0 mt-0.5 text-sm font-semibold text-[#edf9fb]">{activeLayer.name}</h3>
            </div>
            <strong className="shrink-0 text-lg font-medium text-[#9de4c1] tabular-nums">{percent}%</strong>
          </div>

          <div
            className="h-1.5 overflow-hidden bg-[#162228]"
            role="progressbar"
            aria-label={`${activeLayer.name} completion`}
            aria-valuemin={0}
            aria-valuemax={checklist.total}
            aria-valuenow={checklist.completed}
          >
            <span className="block h-full bg-[#65d4ad] transition-[width]" style={{ width: `${percent}%` }} />
          </div>

          <div className="grid grid-cols-3 divide-x divide-[#caeaef]/15 text-center">
            <div>
              <strong className="block text-sm text-[#eaf7f8] tabular-nums">{checklist.completed}</strong>
              <span className="text-[10px] text-[#78949a] uppercase">Complete</span>
            </div>
            <div>
              <strong className="block text-sm text-[#efc779] tabular-nums">{checklist.remaining}</strong>
              <span className="text-[10px] text-[#78949a] uppercase">Missing</span>
            </div>
            <div>
              <strong className="block text-sm text-[#eaf7f8] tabular-nums">{checklist.total}</strong>
              <span className="text-[10px] text-[#78949a] uppercase">Total</span>
            </div>
          </div>

          <label className="flex min-h-11 cursor-pointer items-center gap-2 border-t border-[#caeaef]/15 pt-2 text-xs text-[#dcebed]">
            <input
              type="checkbox"
              className="size-4 shrink-0 accent-[#6cb4dd]"
              checked={checklist.remainingOnly}
              aria-describedby={filterDescriptionId}
              onChange={(event) => checklist.onRemainingOnlyChange(event.currentTarget.checked)}
            />
            <span>Show only missing on the map</span>
          </label>
          <p id={filterDescriptionId} className="sr-only">
            Hide landmarks completed manually or confirmed by your connected save from the map and Map filters.
          </p>

          <div className="border-t border-[#caeaef]/15 pt-2">
            <p
              role="status"
              aria-live="polite"
              aria-atomic="true"
              className={`m-0 text-[11px] leading-5 ${progress.phase === 'unavailable' || (progress.phase === 'available' && (progress.stale || progress.refreshFailed)) ? 'text-[#d8bc83]' : 'text-[#8ba4a9]'}`}
            >
              {saveProgressDescription(progress)}
            </p>
            <p className="m-0 mt-1 text-[10px] leading-4 text-[#718b91]">
              Connected saves confirm bosses, bounties, fast travel, effigies, journals, and shrine pickups. Dungeons,
              oil rigs, and NPC locations use your manual checklist.
            </p>
            {progress.phase === 'available' ? (
              <time className="sr-only" dateTime={progress.snapshot.snapshotAt}>
                Save snapshot {progress.snapshot.snapshotAt}
              </time>
            ) : null}
            {(progress.phase === 'unavailable' && progress.reason === 'request') ||
            (progress.phase === 'available' && (progress.stale || progress.refreshFailed)) ? (
              <button
                type="button"
                className="pal-interactive mt-2 min-h-8 border border-[#8bb7bd]/25 bg-[#26363b]/55 px-2.5 text-[11px] text-[#d7e8ea]"
                disabled={progress.phase === 'available' && progress.refreshing}
                onClick={checklist.onRetrySaveProgress}
              >
                {progress.phase === 'unavailable'
                  ? 'Retry save progress'
                  : progress.refreshing
                    ? 'Refreshing…'
                    : 'Refresh save progress'}
              </button>
            ) : null}
          </div>
        </section>

        <PlayerClaimSessionControl players={players} />
        <PlayerClaimIdentityChooser players={players} />
      </div>
    </MapPanelShell>
  )
}
