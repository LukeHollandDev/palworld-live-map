import { afterEach, describe, expect, it } from 'vitest'
import type { MapItem } from '../types'
import {
  activeLocalCompletionProfile,
  createDefaultLocalCompletionState,
  DEFAULT_COMPLETION_PROFILE_ID,
  DEFAULT_COMPLETION_PROFILE_NAME,
  LOCAL_COMPLETION_STORAGE_KEY,
  loadLocalCompletionState,
  manualCompletionIDs,
  saveLocalCompletionState,
  setManualLandmarkCompletion,
  setRemainingOnly,
  summarizeCompletion,
  summarizeManualCompletion
} from './completion'

afterEach(() => window.localStorage.clear())

describe('local completion profiles', () => {
  it('creates a default manual-only checklist profile', () => {
    const state = createDefaultLocalCompletionState(new Date('2026-08-15T10:00:00Z'))
    const profile = activeLocalCompletionProfile(state)

    expect(state).toMatchObject({
      version: 1,
      activeProfileId: DEFAULT_COMPLETION_PROFILE_ID,
      remainingOnly: false
    })
    expect(profile).toEqual({
      id: DEFAULT_COMPLETION_PROFILE_ID,
      name: DEFAULT_COMPLETION_PROFILE_NAME,
      source: 'manual',
      createdAt: '2026-08-15T10:00:00.000Z',
      manualMarks: []
    })
  })

  it('marks and unmarks landmarks without mutating the prior state', () => {
    const initial = createDefaultLocalCompletionState(new Date('2026-08-15T10:00:00Z'))
    const marked = setManualLandmarkCompletion(initial, 'catalogue:effigy:one', true, new Date('2026-08-15T10:01:00Z'))

    expect(manualCompletionIDs(activeLocalCompletionProfile(initial))).toEqual(new Set())
    expect(activeLocalCompletionProfile(marked).manualMarks).toEqual([
      { landmarkId: 'catalogue:effigy:one', completedAt: '2026-08-15T10:01:00.000Z' }
    ])
    expect(setManualLandmarkCompletion(marked, 'catalogue:effigy:one', true)).toBe(marked)

    const unmarked = setManualLandmarkCompletion(marked, 'catalogue:effigy:one', false)
    expect(activeLocalCompletionProfile(unmarked).manualMarks).toEqual([])
    expect(setManualLandmarkCompletion(unmarked, 'catalogue:effigy:one', false)).toBe(unmarked)
  })

  it('round-trips durable manual marks and the remaining-only preference', () => {
    let state = createDefaultLocalCompletionState(new Date('2026-08-15T10:00:00Z'))
    state = setManualLandmarkCompletion(state, 'catalogue:journal:one', true, new Date('2026-08-15T10:02:00Z'))
    state = setRemainingOnly(state, true)

    saveLocalCompletionState(state)
    expect(loadLocalCompletionState()).toEqual(state)
  })

  it('allowlists manual browser state instead of persisting future private evidence', () => {
    const state = Object.assign(createDefaultLocalCompletionState(new Date('2026-08-15T10:00:00Z')), {
      saveSnapshot: { completedStateKeys: ['private-key'], completedIds: ['private-save-id'] },
      claimToken: 'secret-token',
      challengeToken: 'private-challenge-token'
    })
    Object.assign(state.profiles[0], { saveEvidence: { playerId: 'private-player' } })

    saveLocalCompletionState(state)

    const raw = window.localStorage.getItem(LOCAL_COMPLETION_STORAGE_KEY) || ''
    expect(raw).not.toContain('private-key')
    expect(raw).not.toContain('private-save-id')
    expect(raw).not.toContain('secret-token')
    expect(raw).not.toContain('private-challenge-token')
    expect(raw).not.toContain('private-player')
    expect(JSON.parse(raw)).toEqual(createDefaultLocalCompletionState(new Date('2026-08-15T10:00:00Z')))
  })

  it('rejects corrupt or unsupported payloads and sanitizes malformed marks', () => {
    window.localStorage.setItem(LOCAL_COMPLETION_STORAGE_KEY, '{not-json')
    expect(activeLocalCompletionProfile(loadLocalCompletionState()).name).toBe(DEFAULT_COMPLETION_PROFILE_NAME)

    window.localStorage.setItem(
      LOCAL_COMPLETION_STORAGE_KEY,
      JSON.stringify({ version: 99, activeProfileId: 'other', profiles: [] })
    )
    expect(loadLocalCompletionState().version).toBe(1)

    window.localStorage.setItem(
      LOCAL_COMPLETION_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        activeProfileId: 'missing',
        remainingOnly: 'yes',
        profiles: [
          {
            id: 'manual:kept',
            name: 'Kept checklist',
            source: 'manual',
            createdAt: '2026-08-15T10:00:00Z',
            manualMarks: [
              { landmarkId: 'valid', completedAt: '2026-08-15T10:01:00Z' },
              { landmarkId: 'missing-time' },
              { landmarkId: '', completedAt: '2026-08-15T10:01:00Z' }
            ],
            saveEvidence: { completed: ['private'] }
          }
        ]
      })
    )
    const state = loadLocalCompletionState()
    expect(state.activeProfileId).toBe('manual:kept')
    expect(state.remainingOnly).toBe(false)
    expect(activeLocalCompletionProfile(state).manualMarks).toEqual([
      { landmarkId: 'valid', completedAt: '2026-08-15T10:01:00Z' }
    ])
    expect(activeLocalCompletionProfile(state)).not.toHaveProperty('saveEvidence')
  })

  it('summarizes only unique checklist landmarks in the requested region', () => {
    const items: MapItem[] = [
      { id: 'effigy', kind: 'effigies', name: 'Effigy', x: 1, y: 1, map: 'palpagos' },
      { id: 'effigy', kind: 'effigies', name: 'Duplicate', x: 1, y: 1, map: 'palpagos' },
      { id: 'boss', kind: 'bosses', name: 'Tower', x: 2, y: 2, map: 'world-tree' },
      { id: 'player', kind: 'players', name: 'Player', x: 3, y: 3, map: 'palpagos' }
    ]

    expect(summarizeCompletion(items, new Set(['effigy', 'player']), 'palpagos')).toEqual({
      total: 1,
      completed: 1,
      remaining: 0
    })
    expect(summarizeManualCompletion(items, new Set(['effigy', 'boss']))).toEqual({
      total: 2,
      completed: 2,
      remaining: 0
    })
  })
})
