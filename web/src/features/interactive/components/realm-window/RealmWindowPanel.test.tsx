import userEvent from '@testing-library/user-event'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { RealmInstance, RealmState } from '../../realm-types'
import { RealmWindowPanel } from './RealmWindowPanel'

const apiMocks = vi.hoisted(() => ({
  getRealm: vi.fn(),
}))

vi.mock('../../api', () => apiMocks)

// The realm panel is information only. These tests cover what it decides on its
// own: that it reads without settling anything, that the rooms and passages come
// from what the instance really discovered, that a taken find is not offered as
// available again, and that there is no control anywhere on it that could enter,
// move, take or leave. The rules, the journal record and the item write are
// asserted in the Go suites and not restated here; the acting path is the main
// story's own world action.

function instance(overrides: Partial<RealmInstance> = {}): RealmInstance {
  return {
    instance_id: 'rlm_1',
    realm_id: 'realm.gu_xiu_dong_fu',
    name: '外殿',
    theme: '越国山中的一座废弃古修洞府。',
    presence: 'inside',
    view: {
      instance_id: 'rlm_1',
      presence: 'inside',
      current_area: { id: 'realm.outer_hall', name: '外殿', description: '洞口进来的第一进石殿。' },
      objectives: [{ id: 'realm.recover.manual', description: '找到古修留下的法器。' }],
      areas: [
        { id: 'realm.outer_hall', name: '外殿', description: '洞口进来的第一进石殿。' },
        { id: 'realm.herb_plot', name: '药圃', description: '半荒药圃。' },
      ],
      connections: [
        { id: 'realm.hall_to_plot', from_area_id: 'realm.outer_hall', to_area_id: 'realm.herb_plot', bidirectional: true, player_visible: true },
      ],
      confirmed_facts: [],
      rumors: [],
      completed_objectives: [],
      resolved_challenges: [],
      granted_rewards: [],
    },
    rewards: [],
    granted: [],
    ...overrides,
  }
}

function entryState(overrides: Partial<RealmState> = {}): RealmState {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    location: '越国坊市外围散修人家',
    instances: [instance()],
    available: [],
    ...overrides,
  }
}

function renderPanel(overrides: Partial<RealmState> = {}) {
  apiMocks.getRealm.mockResolvedValue(entryState(overrides))
  render(<RealmWindowPanel storyId="story-1" branchId="main" active onClose={() => {}} />)
}

beforeEach(() => {
  apiMocks.getRealm.mockReset()
})

describe('RealmWindowPanel as information', () => {
  it('shows the room the character is in without settling anything', async () => {
    renderPanel()
    expect(await screen.findByText('洞口进来的第一进石殿。')).toBeInTheDocument()
    // Opening the panel is a read. It cannot enter, move, take or leave, because
    // the panel holds no write path at all.
    expect(apiMocks.getRealm).toHaveBeenCalledTimes(1)
    expect(Object.keys(apiMocks)).toEqual(['getRealm'])
  })

  it('lists only the rooms and passages the instance has discovered', async () => {
    renderPanel()
    expect(await screen.findByText(/外殿 · 药圃/)).toBeInTheDocument()
    expect(screen.getByText(/外殿 → 药圃/)).toBeInTheDocument()
    // The alchemy chamber was never discovered, so nothing about it appears.
    expect(screen.queryByText('丹房')).toBeNull()
  })

  it('names the authored realm that stands at the door, without an enter control', async () => {
    renderPanel({
      instances: [],
      available: [{
        realm_id: 'realm.gu_xiu_dong_fu',
        realm_version: '1.0',
        name: '外殿',
        theme: '越国山中的一座废弃古修洞府。',
        entry_area_id: 'realm.outer_hall',
      }],
    })
    expect(await screen.findByText('此处可进行的探索')).toBeInTheDocument()
    expect(screen.getByText('外殿')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '进入' })).toBeNull()
  })

  it('does not offer a find that was already taken', async () => {
    renderPanel({
      instances: [instance({
        granted: [{ content_ref_id: 'realm.herb_ref', ref_id: 'pill.restore.health.lower', name_key: 'item.pill.restore_health_lower.name', quantity: 1 }],
      })],
    })
    expect(await screen.findByText(/已经取走/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '取走' })).toBeNull()
  })

  it('drafts only the server-projected current ways, not every known map connection', async () => {
    const user = userEvent.setup(), draft = vi.fn()
    const item = instance({ explore: { pressure: 2, pressure_limit: 4, pressure_label: '渗水压力', pressure_label_key: 'game.realm.explore.herbCave.pressure', setback: false, return_available: false, approaches: [] },
      ways: [{ connection_id: 'open', destination_area_id: 'core', destination_name: '旧阵源', destination_name_key: 'game.realm.explore.herbCave.areas.core.name' }],
    })
    apiMocks.getRealm.mockResolvedValue(entryState({ instances: [item] }))
    render(<RealmWindowPanel storyId="story-1" branchId="main" active onClose={() => {}} onDraftAction={draft} />)
    await user.click(await screen.findByRole('button', { name: '加入打算：前往旧阵源' }))
    expect(draft).toHaveBeenCalledExactlyOnceWith('我想在外殿沿当前可通行的路前往旧阵源。')
    expect(screen.queryByRole('button', { name: /前往药圃/ })).not.toBeInTheDocument()
    expect(apiMocks.getRealm).toHaveBeenCalledTimes(1)
  })

  it('uses the projected adventure reward name, never a raw catalogue id', async () => {
    renderPanel({ instances: [instance({ granted: [{ content_ref_id: 'core', ref_id: 'raw-hidden-id', name_key: '', name: 'Alloy wire', quantity: 2 }] })] })
    expect(await screen.findByText('Alloy wire（已取走）')).toBeInTheDocument()
    expect(document.body.textContent).not.toContain('raw-hidden-id')
  })

  it('reports a failed read and offers a retry', async () => {
    apiMocks.getRealm.mockRejectedValue(new Error('offline'))
    render(<RealmWindowPanel storyId="story-1" branchId="main" active onClose={() => {}} />)
    expect(await screen.findByText('offline')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })
})
