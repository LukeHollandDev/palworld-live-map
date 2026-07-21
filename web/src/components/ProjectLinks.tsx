interface ProjectLinksProps {
  hidden: boolean
}

export function ProjectLinks({ hidden }: ProjectLinksProps) {
  return (
    <nav
      className={`absolute right-[172px] bottom-[18px] z-[18] flex h-11 items-center overflow-hidden border border-[#d3eff2]/45 bg-[#070f14]/85 shadow-[0_9px_22px_rgb(0_0_0/28%)] backdrop-blur-sm transition-[opacity,transform] max-sm:right-3.5 max-sm:bottom-[68px] ${
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
        className="grid size-11 place-items-center border-l border-white/10 text-[#789da5] transition-colors hover:bg-[#087fab] hover:text-white focus-visible:bg-[#087fab] focus-visible:text-white focus-visible:outline-none"
        href="https://github.com/LukeHollandDev/palworld-live-map"
        target="_blank"
        rel="noreferrer"
        aria-label="Palworld Live Map on GitHub"
        title="View source on GitHub"
      >
        <svg
          className="size-[19px]"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M15 22v-3.9c0-1 .1-1.4-.5-2 2.8-.3 5.7-1.4 5.7-6.2a4.8 4.8 0 0 0-1.3-3.4 4.5 4.5 0 0 0-.1-3.4s-1-.3-3.5 1.3a12 12 0 0 0-6.4 0C6.5 2.8 5.4 3.1 5.4 3.1a4.5 4.5 0 0 0-.1 3.4A4.8 4.8 0 0 0 4 9.9c0 4.8 2.9 5.9 5.7 6.2-.5.5-.6 1.1-.6 2V22" />
          <path d="M9.1 19c-2.9.9-2.9-1.5-4.1-2" />
        </svg>
        <span className="sr-only">Palworld Live Map on GitHub</span>
      </a>
      <a
        className="flex h-11 items-center border-l border-white/10 px-3 text-[11px] tracking-[.035em] whitespace-nowrap text-[#9bb7bd] transition-colors hover:bg-[#087fab] hover:text-white focus-visible:bg-[#087fab] focus-visible:text-white focus-visible:outline-none"
        href="https://lukeholland.dev"
        target="_blank"
        rel="noreferrer"
        aria-label="Luke Holland's website"
        title="Visit lukeholland.dev"
      >
        Built by Luke
      </a>
    </nav>
  )
}
