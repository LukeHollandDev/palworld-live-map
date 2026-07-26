import { cleanup, render } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ALL_KINDS, type ItemKind } from '../types'
import { MarkerGlyph } from './MarkerGlyph'

vi.mock('@tabler/icons-react', async () => {
  const { createElement } = await import('react')
  const icon = (componentName: string) => (props: Record<string, unknown>) =>
    createElement('svg', { ...props, 'data-tabler-component': componentName })

  return {
    IconBuildingArch: icon('IconBuildingArch'),
    IconBuildingFactory2: icon('IconBuildingFactory2'),
    IconBuildingMonument: icon('IconBuildingMonument'),
    IconBuildingWarehouse: icon('IconBuildingWarehouse'),
    IconCrown: icon('IconCrown'),
    IconHammer: icon('IconHammer'),
    IconHeartHandshake: icon('IconHeartHandshake'),
    IconMapPin: icon('IconMapPin'),
    IconMessage2: icon('IconMessage2'),
    IconNotebook: icon('IconNotebook'),
    IconPaw: icon('IconPaw'),
    IconSkull: icon('IconSkull'),
    IconSparkle: icon('IconSparkle'),
    IconTarget: icon('IconTarget'),
    IconTower: icon('IconTower'),
    IconUser: icon('IconUser'),
    IconUserPin: icon('IconUserPin')
  }
})

const EXPECTED_TABLER_COMPONENTS = {
  players: 'IconUser',
  bases: 'IconBuildingWarehouse',
  workers: 'IconHammer',
  companions: 'IconHeartHandshake',
  'wild-pals': 'IconPaw',
  'alpha-pals': 'IconCrown',
  bosses: 'IconSkull',
  bounties: 'IconTarget',
  'oil-rigs': 'IconBuildingFactory2',
  watchtowers: 'IconTower',
  waypoints: 'IconMapPin',
  'dungeon-entrances': 'IconBuildingArch',
  effigies: 'IconBuildingMonument',
  journals: 'IconNotebook',
  'ancient-shrine-pickups': 'IconSparkle',
  'npc-locations': 'IconUserPin',
  npcs: 'IconMessage2'
} satisfies Record<ItemKind, string>

afterEach(cleanup)

describe('MarkerGlyph', () => {
  it.each(ALL_KINDS)('maps %s to its documented Tabler component and preserves the shared contract', (kind) => {
    const { container } = render(<MarkerGlyph kind={kind} />)
    const glyph = container.firstElementChild

    expect(glyph).toHaveAttribute('data-tabler-component', EXPECTED_TABLER_COMPONENTS[kind])
    expect(glyph).toHaveAttribute('aria-hidden', 'true')
    expect(glyph).toHaveAttribute('focusable', 'false')
    expect(glyph).toHaveAttribute('data-marker-kind', kind)
    expect(glyph).not.toHaveAttribute('data-player-status')
    expect(glyph).toHaveClass('marker-glyph', `kind-${kind}`)
  })

  it('uses a distinct Tabler component for every item kind', () => {
    expect(new Set(Object.values(EXPECTED_TABLER_COMPONENTS)).size).toBe(ALL_KINDS.length)
  })

  it('preserves online and offline player states without applying them to other kinds', () => {
    const { container } = render(
      <>
        <MarkerGlyph kind="players" online />
        <MarkerGlyph kind="players" online={false} />
        <MarkerGlyph kind="workers" online />
      </>
    )
    const players = container.querySelectorAll('[data-marker-kind="players"]')
    const worker = container.querySelector('[data-marker-kind="workers"]')

    expect(players[0]).toHaveClass('player-online')
    expect(players[0]).toHaveAttribute('data-player-status', 'online')
    expect(players[1]).toHaveClass('player-offline')
    expect(players[1]).toHaveAttribute('data-player-status', 'offline')
    expect(worker).not.toHaveClass('player-online', 'player-offline')
    expect(worker).not.toHaveAttribute('data-player-status')
  })
})
