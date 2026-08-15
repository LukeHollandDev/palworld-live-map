import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DetailsDialog } from './DetailsDialog'
import { PlayerClaimProvider } from './PlayerClaimPanel'

const player = {
  id: 'player-public',
  kind: 'players' as const,
  name: 'Luke',
  level: 55,
  online: true,
  x: 10,
  y: 20,
  map: 'palpagos'
}

const base = {
  id: 'base-public',
  kind: 'bases' as const,
  name: 'Home',
  x: 30,
  y: 40,
  map: 'palpagos'
}

function renderDetails(item: typeof player | typeof base, playerClaimsEnabled: boolean) {
  return render(
    <PlayerClaimProvider enabled={playerClaimsEnabled}>
      <DetailsDialog
        detail={{ kind: 'item', itemId: item.id }}
        items={[item]}
        layers={[{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }]}
        playerClaimsEnabled={playerClaimsEnabled}
        returnFocus={null}
        fallbackFocus={null}
        onClose={() => undefined}
        onSelectItem={() => undefined}
        onSelectGuild={() => undefined}
        onSelectLeaderboard={() => undefined}
      />
    </PlayerClaimProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('DetailsDialog player claims', () => {
  it('shows the identity control only for player details when the server opts in', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'authentication_required' }), { status: 401 }))
    )

    const disabled = renderDetails(player, false)
    expect(screen.queryByText('Private progress')).not.toBeInTheDocument()
    expect(fetch).not.toHaveBeenCalled()
    disabled.unmount()

    const nonPlayer = renderDetails(base, true)
    expect(screen.queryByText('Private progress')).not.toBeInTheDocument()
    expect(fetch).toHaveBeenCalledTimes(1)
    nonPlayer.unmount()

    renderDetails(player, true)
    expect(await screen.findByRole('button', { name: 'This is me' })).toBeVisible()
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
