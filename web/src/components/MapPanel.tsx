import { IconX } from '@tabler/icons-react'
import type { ComponentPropsWithoutRef, ElementType, ReactNode, Ref } from 'react'

type MapPanelSide = 'left' | 'right'
type MapPanelMobileSize = 'content' | 'fixed'

type MapPanelShellProps = Omit<ComponentPropsWithoutRef<'aside'>, 'children'> & {
  children: ReactNode
  mobileSize: MapPanelMobileSize
  side: MapPanelSide
}

const panelSideClass: Record<MapPanelSide, string> = {
  left: 'left-4',
  right: 'right-4'
}

const panelMobileSizeClass: Record<MapPanelMobileSize, string> = {
  content: 'max-sm:max-h-[49dvh]',
  fixed: 'max-sm:h-[min(52dvh,480px)]'
}

export function MapPanelShell({ children, className = '', mobileSize, side, ...props }: MapPanelShellProps) {
  return (
    <aside
      {...props}
      className={`pal-glass-panel absolute top-[78px] bottom-4 z-[24] flex w-[350px] min-h-0 flex-col overflow-hidden text-[#e5f0f2] max-sm:inset-x-0 max-sm:top-auto max-sm:bottom-0 max-sm:w-auto max-sm:border-x-0 max-sm:border-b-0 ${panelMobileSizeClass[mobileSize]} ${panelSideClass[side]} ${className}`}
      data-map-panel-shell
      data-map-panel-side={side}
      data-map-panel-mobile-size={mobileSize}
    >
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
