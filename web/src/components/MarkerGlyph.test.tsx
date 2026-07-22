import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ALL_KINDS } from '../types'
import { MarkerGlyph } from './MarkerGlyph'

afterEach(cleanup)

describe('MarkerGlyph', () => {
  it.each(ALL_KINDS)('uses the shared %s glyph contract', (kind) => {
    const { container } = render(<MarkerGlyph kind={kind} />)
    const glyph = container.firstElementChild

    expect(glyph).toHaveAttribute('aria-hidden', 'true')
    expect(glyph).toHaveAttribute('data-marker-kind', kind)
    expect(glyph).toHaveClass('marker-glyph', `kind-${kind}`)

    const [outline, fill] = [...(glyph?.querySelectorAll('path') || [])]
    expect(outline).toHaveClass('marker-glyph-outline')
    expect(outline).toHaveAttribute('stroke-width', '6')
    expect(fill).toHaveClass('marker-glyph-fill')
    expect(fill).toHaveAttribute('stroke-width', '2')
    expect(outline?.getAttribute('d')).toBe(fill?.getAttribute('d'))
  })

  it('keeps Wild and base Pal shapes at the full category footprint', () => {
    const { container } = render(
      <>
        <MarkerGlyph kind="players" />
        <MarkerGlyph kind="workers" />
        <MarkerGlyph kind="companions" />
        <MarkerGlyph kind="wild-pals" />
      </>
    )
    const pathFor = (kind: string) =>
      container.querySelector(`[data-marker-kind="${kind}"] .marker-glyph-fill`)?.getAttribute('d')

    expect(pathFor('workers')).toBe(pathFor('players'))
    expect(pathFor('wild-pals')).toBe(pathFor('companions'))
  })

  it('uses distinct landmark glyphs and online states for players', () => {
    const { container } = render(
      <>
        <MarkerGlyph kind="alpha-pals" />
        <MarkerGlyph kind="bosses" />
        <MarkerGlyph kind="players" online />
        <MarkerGlyph kind="players" online={false} />
      </>
    )
    const pathFor = (kind: string) =>
      container.querySelector(`[data-marker-kind="${kind}"] .marker-glyph-fill`)?.getAttribute('d')
    const players = container.querySelectorAll('[data-marker-kind="players"]')

    expect(pathFor('alpha-pals')).not.toBe(pathFor('bosses'))
    expect(players[0]).toHaveClass('player-online')
    expect(players[0]).toHaveAttribute('data-player-status', 'online')
    expect(players[1]).toHaveClass('player-offline')
    expect(players[1]).toHaveAttribute('data-player-status', 'offline')
  })
})
