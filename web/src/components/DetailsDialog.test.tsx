import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { MapItem, MapLayer } from '../types'
import { DetailsDialog } from './DetailsDialog'

const layers: MapLayer[] = [{ id: 'palpagos', name: 'Palpagos Islands', bounds: [100, 100, -100, -100] }]

function renderItem(
  item: MapItem,
  options?: {
    manualCompletedIds?: ReadonlySet<string>
    saveCompletedIds?: ReadonlySet<string>
    onSetCompletion?: (landmarkId: string, completed: boolean) => void
    onSharePosition?: (item: MapItem) => Promise<{ copied: boolean; url: string }>
  }
) {
  render(
    <DetailsDialog
      detail={{ kind: 'item', itemId: item.id }}
      items={[item]}
      layers={layers}
      returnFocus={null}
      fallbackFocus={null}
      manualChecklist={
        options
          ? {
              profileName: 'My checklist',
              manualCompletedIds: options.manualCompletedIds || new Set(),
              saveCompletedIds: options.saveCompletedIds || new Set(),
              onSetCompletion: options.onSetCompletion || (() => undefined)
            }
          : undefined
      }
      onClose={() => undefined}
      onSelectItem={() => undefined}
      onSelectGuild={() => undefined}
      onSelectLeaderboard={() => undefined}
      onSharePosition={options?.onSharePosition}
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

  it('composes rich journal content, save completion provenance, and position sharing', async () => {
    const user = userEvent.setup()
    const item: MapItem = {
      id: 'journal:combined',
      kind: 'journals',
      name: 'Combined Journal',
      detail: 'A journal preview that remains readable.',
      x: 10,
      y: 20,
      z: 5924.4,
      map: 'palpagos'
    }
    const onSetCompletion = vi.fn()
    const sharedUrl = 'https://map.test/?share=position&region=palpagos&x=10&y=20&zoom=8'
    const dialog = renderItem(item, {
      saveCompletedIds: new Set([item.id]),
      onSetCompletion,
      onSharePosition: vi.fn(async () => ({ copied: false, url: sharedUrl }))
    })

    expect(within(dialog).getByRole('heading', { name: 'Journal preview' })).toBeVisible()
    expect(within(dialog).getByText('59 m')).toBeVisible()
    expect(dialog.querySelector('[data-completion-source="save"]')).toBeInTheDocument()
    const completion = within(dialog).getByRole('checkbox', { name: 'Mark Combined Journal complete in My checklist' })
    expect(completion).not.toBeChecked()
    expect(within(dialog).getByText('Save-confirmed')).toBeVisible()
    expect(within(dialog).getByText('Also add manual mark')).toBeVisible()

    await user.click(completion)
    expect(onSetCompletion).toHaveBeenCalledWith(item.id, true)
    await user.click(within(dialog).getByRole('button', { name: 'Share position' }))
    expect(await within(dialog).findByDisplayValue(sharedUrl)).toBeVisible()
  })
})
