import { IconBrandGithub } from '@tabler/icons-react'
import type { Ref } from 'react'

interface ProjectLinksProps {
  hidden: boolean
  leaderboardButtonRef: Ref<HTMLButtonElement>
  leaderboardOpen: boolean
  onOpenLeaderboards: (focus: HTMLButtonElement) => void
}

export function ProjectLinks({ hidden, leaderboardButtonRef, leaderboardOpen, onOpenLeaderboards }: ProjectLinksProps) {
  return (
    <nav
      className={`pal-glass-surface absolute right-[354px] bottom-[18px] z-[18] flex h-11 items-center overflow-hidden transition-[opacity,transform] max-sm:right-3.5 max-sm:bottom-[68px] ${
        hidden ? 'pointer-events-none translate-y-2 opacity-0' : ''
      }`}
      aria-label="Project links"
      aria-hidden={hidden}
      inert={hidden}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <div className="flex h-full items-center gap-2 px-2.5">
        <img className="size-7 shrink-0" src="/assets/favicon.svg" alt="" aria-hidden="true" draggable={false} />
        <span className="whitespace-nowrap text-xs tracking-[.025em] text-[#e2f3f5]">Palworld Live Map</span>
      </div>
      <a
        className="pal-interactive grid size-11 place-items-center border-l border-white/10 text-[#8bb8c1] focus-visible:outline-none"
        href="https://github.com/LukeHollandDev/palworld-live-map"
        target="_blank"
        rel="noreferrer"
        aria-label="Palworld Live Map on GitHub"
        title="View source on GitHub"
      >
        <IconBrandGithub className="size-[19px]" stroke={1.8} aria-hidden="true" focusable="false" />
        <span className="sr-only">Palworld Live Map on GitHub</span>
      </a>
      <button
        ref={leaderboardButtonRef}
        type="button"
        className="leaderboard-footer-control pal-interactive flex h-11 cursor-pointer items-center border-0 border-l border-white/10 bg-transparent px-3 text-xs font-semibold tracking-[.08em] whitespace-nowrap text-[#9bb7bd] focus-visible:outline-none"
        aria-label="Open leaderboards"
        aria-haspopup="dialog"
        aria-expanded={leaderboardOpen}
        title="Leaderboards"
        onClick={(event) => onOpenLeaderboards(event.currentTarget)}
      >
        LEADERBOARDS
      </button>
    </nav>
  )
}
