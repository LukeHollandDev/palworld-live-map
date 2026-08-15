import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import type { MapItem, MapLayer } from '../types'
import { DetailsDialog } from './DetailsDialog'

const layers: MapLayer[] = [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }]

function renderItem(item: MapItem) {
  render(
    <DetailsDialog
      detail={{ kind: 'item', itemId: item.id }}
      items={[item]}
      layers={layers}
      returnFocus={null}
      fallbackFocus={null}
      onClose={() => undefined}
      onSelectItem={() => undefined}
      onSelectGuild={() => undefined}
      onSelectLeaderboard={() => undefined}
    />
  )
  return screen.getByRole('dialog', { name: item.name })
}

afterEach(cleanup)

describe('rich landmark details', () => {
  it('presents journal text as a readable preview with altitude', () => {
    const preview = 'This world harbors a secret. The island is hidden beyond a wall of fog...'
    const dialog = renderItem({
      id: 'journal:test',
      kind: 'journals',
      name: "Castaway's Journal - Day XX",
      detail: preview,
      x: 10,
      y: 20,
      z: 5924.4,
      map: 'palpagos'
    })

    expect(within(dialog).getByRole('heading', { name: 'Journal preview' })).toBeVisible()
    expect(within(dialog).getByText(preview)).toBeVisible()
    expect(within(dialog).getByText('Altitude')).toBeVisible()
    expect(within(dialog).getByText('59 m')).toBeVisible()
    expect(within(dialog).queryByText('Type')).not.toBeInTheDocument()
  })

  it('lists structured Ancient Shrine rewards without repeating the legacy summary', () => {
    const dialog = renderItem({
      id: 'ancient-shrine:test',
      kind: 'ancient-shrine-pickups',
      name: 'Crossbow Schematic 3',
      detail: '+20 Dog Coin',
      x: 10,
      y: 20,
      z: 1317,
      map: 'palpagos',
      rewards: [
        { name: 'Crossbow Schematic 3', count: 1 },
        { name: 'Dog Coin', count: 20 }
      ]
    })

    expect(within(dialog).getByRole('heading', { name: 'Rewards' })).toBeVisible()
    const rewardList = within(dialog).getByRole('list')
    expect(within(rewardList).getByText('Crossbow Schematic 3')).toBeVisible()
    expect(within(rewardList).getByText('Dog Coin')).toBeVisible()
    expect(within(rewardList).getByText('Quantity 1')).toBeInTheDocument()
    expect(within(rewardList).getByText('Quantity 20')).toBeInTheDocument()
    expect(within(dialog).getByText('13 m')).toBeVisible()
    expect(within(dialog).queryByText('+20 Dog Coin')).not.toBeInTheDocument()
  })
})
