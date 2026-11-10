import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { Snapshot, TurnEvent } from '../../types'
import { OverviewView } from './OverviewView'

const turn: TurnEvent = {
  id: 'turn-1',
  parent_id: null,
  branch_id: 'main',
  ts: '2026-08-31T00:00:00Z',
  user: '买下两枚灵石',
  narrative: '沈凝把灵石收进袖中。',
  state_delta: {
    actor_ops: [{ op: 'set', actor_id: 'hero', field_id: '灵石', value: 148, previous_value: 150 }],
  },
}

const snapshot: Snapshot = {
  story_id: 'story-1',
  branch_id: 'main',
  turns: [turn],
  current_turn: turn,
  turn_count: 1,
  state: {
    actors: {
      hero: { name: '沈凝', role: 'protagonist', state: { 灵石: 148 } },
    },
    scene: '旧车站',
  },
}

describe('OverviewView', () => {
  afterEach(() => setConfiguredLocale('zh-CN'))

  it('keeps state compact and opens the shared full-state view', () => {
    render(<OverviewView snapshot={snapshot} />)

    const summary = screen.getByLabelText('本回合变化')
    expect(summary).toHaveTextContent('灵石 -2')
    expect(screen.getByText('1 位角色 · 1 项世界状态')).toBeInTheDocument()
    expect(screen.queryByLabelText('选择状态展示方式')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /完整状态/ }))
    expect(screen.getByRole('dialog', { name: '故事状态' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '沈凝' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '世界状态' })).toBeInTheDocument()
  })

  it('names the starting state instead of itemising a character sheet', () => {
    // The opening has a settled snapshot but no turn, so there is no delta to
    // report. Seventy-odd fields were written to build the character, and none
    // of them is a change the player made — the summary says 初始状态 and stops.
    const opening: Snapshot = { ...snapshot, turns: [], current_turn: undefined, turn_count: 0 }
    render(<OverviewView snapshot={opening} />)

    const summary = screen.getByLabelText('初始状态')
    expect(summary).toHaveTextContent('初始状态')
    expect(summary).not.toHaveTextContent('灵石')
  })

  it('keeps an empty story compact while retaining the full-state entry', () => {
    render(<OverviewView snapshot={null} />)

    expect(screen.getByText('本回合还没有记下新的变化。')).toBeInTheDocument()
    expect(screen.getByText('0 位角色 · 0 项世界状态')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /完整状态/ }))
    expect(screen.getByText('这一条故事线还没有可看的状态')).toBeInTheDocument()
  })

  it('reads the same summary in English', () => {
    // The row is one line of prose, so a missing key does not fall back to
    // anything readable — it prints the key itself into the middle of the
    // sentence. The English build has to say the same four words.
    setConfiguredLocale('en-US')
    render(<OverviewView snapshot={snapshot} />)

    const summary = screen.getByLabelText('Changed this turn')
    expect(summary).toHaveTextContent('Changed this turn')
    expect(summary).toHaveTextContent('灵石 -2')
    expect(summary).not.toHaveTextContent('storyStage.')
  })
})
