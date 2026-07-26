import { IconX } from '@tabler/icons-react'
import type { ComponentPropsWithoutRef, ElementType, ReactNode, PointerEvent as ReactPointerEvent, Ref } from 'react'
import { useCallback, useEffect, useRef, useState } from 'react'

type MapPanelSide = 'left' | 'right'
type MapPanelMobileSize = 'content' | 'fixed'

type MapPanelShellProps = Omit<ComponentPropsWithoutRef<'aside'>, 'children'> & {
  children: ReactNode
  mobileSize: MapPanelMobileSize
  mobileSheetActive?: boolean
  mobileSheetLabel?: string
  side: MapPanelSide
}

interface MobileSheetDrag {
  compactHeight: number
  currentY: number
  expandedHeight: number
  frameId: number | null
  pendingHeight: number | null
  pointerId: number
  startExpanded: boolean
  startHeight: number
  startY: number
}

const MOBILE_SHEET_DRAG_THRESHOLD = 48
const MOBILE_SHEET_MOVE_TOLERANCE = 6

const panelSideClass: Record<MapPanelSide, string> = {
  left: 'left-4',
  right: 'right-4'
}

const panelMobileSizeClass: Record<MapPanelMobileSize, string> = {
  content: 'max-sm:max-h-[49dvh]',
  fixed: 'map-panel-mobile-sheet'
}

function mobileSheetHeights(shell: HTMLElement, fallbackHeight: number) {
  const committedState = shell.dataset.mapPanelMobileState || 'compact'
  const dragging = shell.dataset.mapPanelDragging
  const dragHeight = shell.style.getPropertyValue('--map-panel-drag-height')
  shell.dataset.mapPanelMeasuring = 'true'
  delete shell.dataset.mapPanelDragging
  shell.style.removeProperty('--map-panel-drag-height')
  shell.dataset.mapPanelMobileState = 'compact'
  const measuredCompactHeight = shell.getBoundingClientRect().height
  shell.dataset.mapPanelMobileState = 'expanded'
  const measuredExpandedHeight = shell.getBoundingClientRect().height
  shell.dataset.mapPanelMobileState = committedState
  if (dragging) shell.dataset.mapPanelDragging = dragging
  if (dragHeight) shell.style.setProperty('--map-panel-drag-height', dragHeight)
  delete shell.dataset.mapPanelMeasuring

  const compactHeight = measuredCompactHeight > 0 ? measuredCompactHeight : Math.max(1, fallbackHeight)
  return {
    compactHeight,
    expandedHeight: Math.max(compactHeight, measuredExpandedHeight > 0 ? measuredExpandedHeight : compactHeight)
  }
}

function mobileSheetSnapAfterDrag(drag: MobileSheetDrag, releaseY: number, cancelled: boolean) {
  if (cancelled) return drag.startExpanded
  const distance = drag.startY - releaseY
  const snapDistance = (drag.expandedHeight - drag.compactHeight) / 2
  if (snapDistance <= 0) {
    if (distance >= MOBILE_SHEET_DRAG_THRESHOLD) return true
    if (distance <= -MOBILE_SHEET_DRAG_THRESHOLD) return false
    return drag.startExpanded
  }
  const threshold = Math.min(MOBILE_SHEET_DRAG_THRESHOLD, snapDistance)
  const finalHeight = Math.min(drag.expandedHeight, Math.max(drag.compactHeight, drag.startHeight + distance))
  return drag.startExpanded
    ? drag.expandedHeight - finalHeight < threshold
    : finalHeight - drag.compactHeight >= threshold
}

export function MapPanelShell({
  children,
  className = '',
  id,
  mobileSheetActive = true,
  mobileSheetLabel,
  mobileSize,
  side,
  style,
  ...props
}: MapPanelShellProps) {
  const shellRef = useRef<HTMLElement>(null)
  const resizeHandleRef = useRef<HTMLButtonElement>(null)
  const dragRef = useRef<MobileSheetDrag | null>(null)
  const suppressClickRef = useRef(false)
  const suppressClickTimerRef = useRef<number | null>(null)
  const [mobileExpanded, setMobileExpanded] = useState(false)
  const mobileSheet = mobileSize === 'fixed' && Boolean(mobileSheetLabel)

  const clearSuppressClickTimer = useCallback(() => {
    if (suppressClickTimerRef.current === null) return
    window.clearTimeout(suppressClickTimerRef.current)
    suppressClickTimerRef.current = null
  }, [])

  const commitMobileSheet = useCallback((expanded: boolean) => {
    const shell = shellRef.current
    const drag = dragRef.current
    if (drag?.frameId !== null && drag?.frameId !== undefined) window.cancelAnimationFrame(drag.frameId)
    dragRef.current = null
    if (shell) {
      shell.dataset.mapPanelMobileState = expanded ? 'expanded' : 'compact'
      delete shell.dataset.mapPanelDragging
      shell.style.removeProperty('--map-panel-drag-height')
    }
    setMobileExpanded(expanded)
  }, [])

  const resetMobileSheet = useCallback(() => {
    const drag = dragRef.current
    const handle = resizeHandleRef.current
    if (drag && handle?.hasPointerCapture?.(drag.pointerId)) handle.releasePointerCapture(drag.pointerId)
    clearSuppressClickTimer()
    suppressClickRef.current = false
    commitMobileSheet(false)
  }, [clearSuppressClickTimer, commitMobileSheet])

  const clearMobileSheet = useCallback(() => {
    const drag = dragRef.current
    const handle = resizeHandleRef.current
    if (drag && handle?.hasPointerCapture?.(drag.pointerId)) handle.releasePointerCapture(drag.pointerId)
    dragRef.current = null
    clearSuppressClickTimer()
    suppressClickRef.current = false
    const shell = shellRef.current
    if (shell) {
      delete shell.dataset.mapPanelDragging
      delete shell.dataset.mapPanelMobileState
      shell.style.removeProperty('--map-panel-drag-height')
    }
    setMobileExpanded(false)
  }, [clearSuppressClickTimer])

  useEffect(() => {
    if (!mobileSheet) clearMobileSheet()
    else if (!mobileSheetActive) resetMobileSheet()
  }, [clearMobileSheet, mobileSheet, mobileSheetActive, resetMobileSheet])

  useEffect(() => {
    if (!mobileSheet) return
    const reconcileViewport = () => {
      if (window.innerWidth >= 640) {
        resetMobileSheet()
        return
      }
      const drag = dragRef.current
      const shell = shellRef.current
      if (!drag || !shell) return
      if (drag.frameId !== null) {
        window.cancelAnimationFrame(drag.frameId)
        drag.frameId = null
      }
      const currentHeight = shell.getBoundingClientRect().height
      const { compactHeight, expandedHeight } = mobileSheetHeights(shell, currentHeight)
      const rebasedHeight = Math.min(expandedHeight, Math.max(compactHeight, currentHeight))
      drag.compactHeight = compactHeight
      drag.expandedHeight = expandedHeight
      drag.pendingHeight = null
      drag.startHeight = rebasedHeight
      drag.startY = drag.currentY
      shell.style.setProperty('--map-panel-drag-height', `${rebasedHeight}px`)
    }
    window.addEventListener('resize', reconcileViewport)
    window.visualViewport?.addEventListener('resize', reconcileViewport)
    return () => {
      window.removeEventListener('resize', reconcileViewport)
      window.visualViewport?.removeEventListener('resize', reconcileViewport)
    }
  }, [mobileSheet, resetMobileSheet])

  useEffect(
    () => () => {
      clearSuppressClickTimer()
      const drag = dragRef.current
      if (drag?.frameId !== null && drag?.frameId !== undefined) window.cancelAnimationFrame(drag.frameId)
      const shell = shellRef.current
      if (!shell) return
      delete shell.dataset.mapPanelDragging
      shell.style.removeProperty('--map-panel-drag-height')
    },
    [clearSuppressClickTimer]
  )

  const startMobileDrag = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (window.innerWidth >= 640 || event.isPrimary === false) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const shell = shellRef.current
    if (!shell) return
    clearSuppressClickTimer()
    suppressClickRef.current = false
    const measuredHeight = shell.getBoundingClientRect().height
    const { compactHeight, expandedHeight } = mobileSheetHeights(shell, measuredHeight)
    dragRef.current = {
      compactHeight,
      currentY: event.clientY,
      expandedHeight,
      frameId: null,
      pendingHeight: null,
      pointerId: event.pointerId,
      startExpanded: mobileExpanded,
      startHeight: measuredHeight > 0 ? measuredHeight : mobileExpanded ? expandedHeight : compactHeight,
      startY: event.clientY
    }
    shell.dataset.mapPanelDragging = 'true'
    shell.style.setProperty('--map-panel-drag-height', `${dragRef.current.startHeight}px`)
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }

  const moveMobileDrag = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const drag = dragRef.current
    const shell = shellRef.current
    if (!drag || !shell || event.pointerId !== drag.pointerId) return
    event.preventDefault()
    drag.currentY = event.clientY
    const distance = drag.startY - event.clientY
    if (Math.abs(distance) >= MOBILE_SHEET_MOVE_TOLERANCE) suppressClickRef.current = true
    const height = Math.min(drag.expandedHeight, Math.max(drag.compactHeight, drag.startHeight + distance))
    drag.pendingHeight = height
    if (drag.frameId !== null) return
    drag.frameId = window.requestAnimationFrame(() => {
      if (dragRef.current !== drag || drag.pendingHeight === null) return
      shell.style.setProperty('--map-panel-drag-height', `${drag.pendingHeight}px`)
      drag.frameId = null
    })
  }

  const finishMobileDrag = (event: ReactPointerEvent<HTMLButtonElement>, cancelled = false) => {
    const drag = dragRef.current
    if (!drag || event.pointerId !== drag.pointerId) return
    const expanded = mobileSheetSnapAfterDrag(drag, event.clientY, cancelled)
    if (expanded !== drag.startExpanded) suppressClickRef.current = true
    commitMobileSheet(expanded)
    if (event.currentTarget.hasPointerCapture?.(event.pointerId))
      event.currentTarget.releasePointerCapture(event.pointerId)
    if (!suppressClickRef.current) return
    clearSuppressClickTimer()
    suppressClickTimerRef.current = window.setTimeout(() => {
      suppressClickRef.current = false
      suppressClickTimerRef.current = null
    }, 0)
  }

  const cancelLostMobileDrag = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (!dragRef.current) return
    finishMobileDrag(event, true)
  }

  const toggleMobileSheet = () => {
    if (suppressClickRef.current) {
      suppressClickRef.current = false
      clearSuppressClickTimer()
      return
    }
    commitMobileSheet(!mobileExpanded)
  }

  const resizeWithKeyboard = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowUp' || event.key === 'End') {
      event.preventDefault()
      commitMobileSheet(true)
    } else if (event.key === 'ArrowDown' || event.key === 'Home') {
      event.preventDefault()
      commitMobileSheet(false)
    }
  }

  return (
    <aside
      ref={shellRef}
      {...props}
      id={id}
      style={style}
      className={`pal-glass-panel absolute top-[78px] bottom-4 z-[24] flex w-[350px] min-h-0 flex-col overflow-hidden text-[#e5f0f2] max-sm:inset-x-0 max-sm:top-auto max-sm:bottom-0 max-sm:w-auto max-sm:border-x-0 max-sm:border-b-0 ${panelMobileSizeClass[mobileSize]} ${panelSideClass[side]} ${className}`}
      data-map-panel-shell
      data-map-panel-side={side}
      data-map-panel-mobile-size={mobileSize}
      data-map-panel-mobile-state={mobileSheet ? (mobileExpanded ? 'expanded' : 'compact') : undefined}
    >
      {mobileSheet && (
        <button
          ref={resizeHandleRef}
          type="button"
          className="map-panel-resize-handle absolute top-0 left-1/2 z-[3] flex h-6 w-24 -translate-x-1/2 cursor-ns-resize touch-none select-none items-center justify-center border-0 bg-transparent p-0 text-[#88abb1] sm:hidden focus-visible:outline-offset-[-3px]"
          data-map-panel-resize-handle
          aria-label={`Use expanded ${mobileSheetLabel} panel`}
          aria-controls={id}
          aria-pressed={mobileExpanded}
          title={mobileExpanded ? `Reduce ${mobileSheetLabel} panel` : `Expand ${mobileSheetLabel} panel`}
          onClick={toggleMobileSheet}
          onKeyDown={resizeWithKeyboard}
          onPointerDown={startMobileDrag}
          onPointerMove={moveMobileDrag}
          onPointerUp={finishMobileDrag}
          onPointerCancel={(event) => finishMobileDrag(event, true)}
          onLostPointerCapture={cancelLostMobileDrag}
        >
          <span
            className={`block h-1 rounded-full transition-[width,background-color] duration-150 ${
              mobileExpanded ? 'w-12 bg-[#72d7e5]' : 'w-9 bg-[#68858b]'
            }`}
            aria-hidden="true"
          />
        </button>
      )}
      {children}
    </aside>
  )
}

interface MapPanelHeaderProps {
  as?: 'div' | 'header'
  closeButtonRef?: Ref<HTMLButtonElement>
  closeControls?: string
  closeExpanded?: boolean
  closeLabel: string
  closeTitle?: string
  eyebrow: string
  onClose: () => void
  title: string
  titleId?: string
  titleRef?: Ref<HTMLHeadingElement>
  titleTabIndex?: number
}

export function MapPanelHeader({
  as: Header = 'header',
  closeButtonRef,
  closeControls,
  closeExpanded,
  closeLabel,
  closeTitle,
  eyebrow,
  onClose,
  title,
  titleId,
  titleRef,
  titleTabIndex
}: MapPanelHeaderProps) {
  return (
    <Header
      className="pal-panel-header relative z-[2] flex min-h-[78px] shrink-0 items-center justify-between gap-3.5 border-b pr-3.5 pl-5 [--pal-panel-accent:#72d7e5]"
      data-map-panel-header
    >
      <div>
        <p className="m-0 mb-1 text-[10px] font-normal tracking-[.14em] text-[#b6f5fc]">{eyebrow}</p>
        <h2
          ref={titleRef}
          id={titleId}
          className="m-0 text-[22px] font-normal text-[#f3fbfc] outline-none"
          tabIndex={titleTabIndex}
        >
          {title}
        </h2>
      </div>
      <button
        ref={closeButtonRef}
        type="button"
        className="pal-interactive grid size-11 cursor-pointer place-items-center border-0 bg-transparent text-xl text-[#d7eef1]"
        aria-label={closeLabel}
        aria-controls={closeControls}
        aria-expanded={closeExpanded}
        title={closeTitle}
        onClick={onClose}
      >
        <IconX className="size-5" aria-hidden="true" />
      </button>
    </Header>
  )
}

type MapPanelControlKind = 'filters' | 'leaderboards'

interface MapPanelControlProps {
  buttonRef: Ref<HTMLButtonElement>
  children?: ReactNode
  controlsId: string
  describedBy?: string
  dialog?: boolean
  expanded: boolean
  icon: ElementType
  kind: MapPanelControlKind
  label: string
  mobileLabel: string
  onToggle: (button: HTMLButtonElement) => void
}

const controlPlacementClass: Record<MapPanelControlKind, string> = {
  filters: 'col-start-1 max-sm:col-start-1',
  leaderboards: 'col-start-3 max-sm:col-start-2'
}

export function MapPanelControl({
  buttonRef,
  children,
  controlsId,
  describedBy,
  dialog = false,
  expanded,
  icon: PanelIcon,
  kind,
  label,
  mobileLabel,
  onToggle
}: MapPanelControlProps) {
  return (
    <button
      ref={buttonRef}
      type="button"
      className={`header-panel-control pal-glass-control pointer-events-auto relative row-start-1 flex h-[54px] w-full min-w-0 cursor-pointer items-center justify-center self-center overflow-hidden p-0 max-sm:row-start-2 max-sm:h-11 max-sm:gap-2 ${
        controlPlacementClass[kind]
      } ${expanded ? 'pal-selected' : ''}`}
      data-panel-control={kind}
      aria-label={label}
      aria-controls={controlsId}
      aria-describedby={describedBy}
      aria-haspopup={dialog ? 'dialog' : undefined}
      aria-expanded={expanded}
      title={label}
      onClick={(event) => onToggle(event.currentTarget)}
    >
      <PanelIcon className="size-6 shrink-0 max-sm:size-5" stroke={1.8} aria-hidden="true" />
      <span className="hidden min-w-0 overflow-hidden text-[11px] font-semibold tracking-[.09em] text-ellipsis whitespace-nowrap max-sm:inline">
        {mobileLabel}
      </span>
      {children}
    </button>
  )
}
