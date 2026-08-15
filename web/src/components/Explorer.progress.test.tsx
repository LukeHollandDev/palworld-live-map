import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { type ComponentProps, createRef } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ItemKind, MapLayer, PlayerStatus } from '../types'
import { Explorer } from './Explorer'

const layer: MapLayer = {
  id: 'palpagos',
  name: 'Palpagos Islands',
  bounds: [100, 100, -100, -100]
}

function explorerProps(overrides: Partial<ComponentProps<typeof Explorer>> = {}): ComponentProps<typeof Explorer> {
  return {
    open: true,
    activeLayer: layer,
    layers: [layer],
    items: [],
    search: '',
    filterButtonRef: createRef<HTMLButtonElement>(),
    searchInputRef: createRef<HTMLInputElement>(),
    enabledKinds: new Set<ItemKind>(['players']),
    enabledPlayerStatuses: new Set<PlayerStatus>(['online', 'offline']),
    hiddenIds: new Set(),
    expandedGuilds: new Set(),
    expandedBases: new Set(),
    manualChecklist: {
      profileName: 'My checklist',
      completedIds: new Set(['done-1', 'done-2']),
      manualCompletedIds: new Set(['done-1', 'done-2']),
      saveCompletedIds: new Set(),
      completed: 2,
      total: 5,
      remaining: 3,
      remainingOnly: false,
      saveProgress: { phase: 'inactive' },
      onRetrySaveProgress: vi.fn(),
      onRemainingOnlyChange: vi.fn()
    },
    dataNotices: [],
    onSearchChange: vi.fn(),
    onCheckAll: vi.fn(),
    onUncheckAll: vi.fn(),
    onToggleKinds: vi.fn(),
    onTogglePlayerStatus: vi.fn(),
    onToggleItems: vi.fn(),
    onToggleGuild: vi.fn(),
    onToggleBase: vi.fn(),
    onFocusItem: vi.fn(),
    onFocusGuild: vi.fn(),
    onClose: vi.fn(),
    onLayerChange: vi.fn(),
    ...overrides
  }
}

afterEach(cleanup)

describe('Explorer exploration progress', () => {
  it('keeps the progress summary and missing-only filter compact while details default collapsed', async () => {
    const user = userEvent.setup()
    const props = explorerProps()
    render(<Explorer {...props} />)

    const progress = screen.getByRole('region', { name: 'Exploration progress' })
    const disclosure = within(progress).getByRole('button', { name: 'Expand exploration progress details' })
    const details = document.getElementById(disclosure.getAttribute('aria-controls') || '')
    if (!details) throw new Error('Expected exploration progress disclosure details')

    expect(disclosure).toHaveAttribute('aria-expanded', 'false')
    expect(details).not.toBeVisible()
    expect(progress.querySelector('strong')).toHaveTextContent('2/5 · 3 missing')
    expect(progress.querySelector('strong')).toHaveAttribute('aria-live', 'polite')

    const missingOnly = within(progress).getByRole('checkbox', { name: 'Show only missing' })
    expect(missingOnly).toBeVisible()
    expect(missingOnly).toHaveAccessibleDescription(
      'Hide landmarks completed manually or confirmed by the connected save from the map and map filter.'
    )
    await user.click(missingOnly)
    expect(props.manualChecklist.onRemainingOnlyChange).toHaveBeenCalledWith(true)

    expect(disclosure).toHaveAccessibleDescription(
      'Manual only · connect with “This is me” for save-confirmed progress.'
    )
    await user.click(disclosure)

    expect(disclosure).toHaveAttribute('aria-expanded', 'true')
    expect(disclosure).toHaveAccessibleName('Collapse exploration progress details')
    expect(disclosure).toHaveFocus()
    expect(details).toBeVisible()
    expect(within(details).getByText('My checklist · manual only')).toBeVisible()
    expect(within(details).getByText('2 of 5 landmarks complete in Palpagos Islands')).toBeVisible()
    expect(
      within(details).getByText(
        'Private saves confirm waypoints and journals only; every other category uses manual marks.'
      )
    ).toBeVisible()
  })

  it('keeps save-progress recovery available inside the expandable details', async () => {
    const user = userEvent.setup()
    const onRetrySaveProgress = vi.fn()
    const props = explorerProps({
      manualChecklist: {
        ...explorerProps().manualChecklist,
        saveProgress: {
          phase: 'unavailable',
          playerId: 'player-1',
          sessionEpoch: 1,
          requestAttempt: 1,
          reason: 'request'
        },
        onRetrySaveProgress
      }
    })
    render(<Explorer {...props} />)

    const disclosure = screen.getByRole('button', { name: 'Expand exploration progress details' })
    expect(disclosure).toHaveAccessibleDescription(
      'Save progress is temporarily unavailable. Manual marks still count.'
    )
    expect(screen.queryByRole('button', { name: 'Retry save progress' })).not.toBeInTheDocument()

    await user.click(disclosure)
    const retry = screen.getByRole('button', { name: 'Retry save progress' })
    await user.click(retry)
    expect(onRetrySaveProgress).toHaveBeenCalledOnce()
  })

  it('labels the visibility-wide controls as show and hide actions', async () => {
    const user = userEvent.setup()
    const props = explorerProps()
    render(<Explorer {...props} />)

    await user.click(screen.getByRole('button', { name: 'Show all' }))
    await user.click(screen.getByRole('button', { name: 'Hide all' }))

    expect(props.onCheckAll).toHaveBeenCalledOnce()
    expect(props.onUncheckAll).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: 'Check all' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Uncheck all' })).not.toBeInTheDocument()
  })
})
