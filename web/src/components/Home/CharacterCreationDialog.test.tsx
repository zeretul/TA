vi.mock('@/components/Chat/ModelProfileSwitcher', () => ({ ModelProfileSwitcher: () => null }))
vi.mock('@/features/conversation-config/use-conversation-config', () => ({ useConversationConfig: () => ({ initialized: true, saving: false }) }))
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoryPackIdentityDraft, StoryPackSummary } from '@/lib/api-client'
import { CharacterCreationDialog } from './CharacterCreationDialog'

const { draft } = vi.hoisted(() => ({ draft: vi.fn() }))
vi.mock('@/lib/api-client', () => ({ draftStoryPackIdentity: draft }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

function pack(id: string): StoryPackSummary {
  return { id, name: id, creation: { age_range: [20, 28], genders: [{ id: 'female', name: 'Woman', default: true }], origins: [{ id: 'custom', name: 'Custom', default: true, default_template_id: 'basis' }], state_basis_templates: ['basis'], affiliations: [{ id: 'none', name: 'None' }], locations: [{ id: 'auto', name: 'Auto' }] }, protagonist_templates: [{ id: 'basis', name: 'Basis' }] } as StoryPackSummary
}

describe('character creation scope', () => {
  beforeEach(() => { draft.mockReset() })
  it('lets the model infer a place and confirms that same place', async () => {
    draft.mockResolvedValueOnce({ identity: { name: '叶行舟', age: 24, location_name: '叶氏山庄内院' } })
    const onStart = vi.fn()
    render(<CharacterCreationDialog open pack={pack('world')} onOpenChange={vi.fn()} onStart={onStart} />)
    expect(screen.queryByLabelText('creation.location')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('creation.customLocation')).not.toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: '叶行舟' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    expect(draft.mock.calls[0][1]).toMatchObject({ location_id: 'auto' })
    expect(draft.mock.calls[0][1].location_text).toBeUndefined()
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.start' })) })
    expect(onStart).toHaveBeenCalledWith(expect.objectContaining({ location_text: '叶氏山庄内院' }))
  })

  it('ignores a late draft after the selected world changes', async () => {
    let finish!: (value: StoryPackIdentityDraft) => void
    draft.mockReturnValueOnce(new Promise<StoryPackIdentityDraft>(resolve => { finish = resolve }))
    const props = { open: true, onOpenChange: vi.fn(), onStart: vi.fn() }
    const view = render(<CharacterCreationDialog {...props} pack={pack('world-a')} />)
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'A-person' } })
    fireEvent.click(screen.getByRole('button', { name: 'creation.generate' }))
    expect(draft).toHaveBeenCalledOnce()
    view.rerender(<CharacterCreationDialog {...props} pack={pack('world-b')} />)
    await act(async () => { finish({ identity: { name: 'A-person', age: 24, origin_text: 'World A origin' } } as StoryPackIdentityDraft) })
    expect(screen.queryByText('A-person')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('creation.namePlaceholder')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'creation.generate' })).toBeInTheDocument()
  })

  it('ignores a late start failure after the selected world changes', async () => {
    draft.mockResolvedValueOnce({ identity: { name: 'A-person', age: 24, origin_text: 'World A origin' } })
    let rejectStart!: (reason: Error) => void
    const onStart = vi.fn(() => new Promise<void>((_resolve, reject) => { rejectStart = reject }))
    const props = { open: true, onOpenChange: vi.fn(), onStart }
    const view = render(<CharacterCreationDialog {...props} pack={pack('world-a')} />)
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'A-person' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    fireEvent.click(screen.getByRole('button', { name: 'creation.start' }))
    view.rerender(<CharacterCreationDialog {...props} pack={pack('world-b')} />)
    await act(async () => { rejectStart(new Error('World A could not start')) })
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('creation.namePlaceholder')).toHaveValue('')
  })

  it('does not offer an unsupported custom affiliation for an independent origin', () => {
    const independent = pack('independent')
    independent.creation!.origins![0].affiliation_kind = 'none'
    render(<CharacterCreationDialog open pack={independent} onOpenChange={vi.fn()} onStart={vi.fn()} />)
    expect(screen.queryByLabelText('creation.customAffiliation')).not.toBeInTheDocument()
  })


  it('aborts a draft when closing and keeps the generating form immutable', async () => {
    draft.mockReturnValueOnce(new Promise(() => {}))
    const props = { open: true, onOpenChange: vi.fn(), onStart: vi.fn(), pack: pack('world') }
    const view = render(<CharacterCreationDialog {...props} />)
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    fireEvent.click(screen.getByRole('button', { name: 'creation.generate' }))
    const signal = draft.mock.calls[0][3] as AbortSignal
    expect(signal.aborted).toBe(false)
    expect(screen.getByPlaceholderText('creation.namePlaceholder')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'common.cancel' })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }))
    expect(props.onOpenChange).toHaveBeenCalledWith(false)
    view.rerender(<CharacterCreationDialog {...props} open={false} />)
    expect(signal.aborted).toBe(true)
  })

  for (const family of ['wuxia', 'space'] as const) {
    it(`renders the formal ${family} state without cultivation labels or an empty talent card`, async () => {
      draft.mockResolvedValueOnce({ identity: { name: 'Traveler', age: 24, initial_state: { family, rank: 2, currency: 0, currency_label: 'Local currency', items: ['Owned tool'], techniques: ['Known skill'] } } })
      render(<CharacterCreationDialog open pack={pack(family)} onOpenChange={vi.fn()} onStart={vi.fn()} />)
      fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
      await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
      expect(screen.getByText('creation.rank')).toBeInTheDocument()
      expect(screen.getByText('Local currency')).toBeInTheDocument()
      expect(screen.getByText('0')).toBeInTheDocument()
      expect(screen.getByText(family === 'wuxia' ? 'creation.moves' : 'creation.skills')).toBeInTheDocument()
      expect(screen.queryByText('creation.realm')).not.toBeInTheDocument()
      expect(screen.queryByText('creation.talents')).not.toBeInTheDocument()
    })
  }

})

describe('native adventure starting choices', () => {
  beforeEach(() => { draft.mockReset() })

  it('offers five actual discipline IDs and carries the selected one through preview and confirmation', async () => {
    const space = pack('space-classes')
    space.creation!.adventure_classes = ['pugilist', 'esper', 'mechanic', 'mage', 'psychic'].map(id => ({ id: `xinghai-class-${id}`, name: id, default: id === 'mechanic' }))
    const onStart = vi.fn().mockResolvedValue(undefined)
    draft.mockResolvedValue({ identity: { name: 'Traveler', age: 24, adventure_class_id: 'xinghai-class-psychic', adventure_class_name: 'Psychic', initial_state: { family: 'space', rank: 1, currency: 0, items: ['Focus Baton', 'Psychic Primer'], equipped: ['Focus Baton'], techniques: ['Psychic Pulse'] } } })
    render(<CharacterCreationDialog open pack={space} onOpenChange={vi.fn()} onStart={onStart} />)
    const select = screen.getByLabelText('creation.adventureClass')
    expect(select).toHaveValue('xinghai-class-mechanic')
    expect(select.querySelectorAll('option')).toHaveLength(5)
    expect(screen.queryByLabelText('creation.adventureMethod')).not.toBeInTheDocument()
    fireEvent.change(select, { target: { value: 'xinghai-class-psychic' } })
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    expect(draft.mock.calls[0][1]).toMatchObject({ adventure_class_id: 'xinghai-class-psychic' })
    expect(screen.getByText('Psychic Pulse')).toBeInTheDocument()
    expect(screen.getByText('creation.equipped')).toBeInTheDocument()
    expect(screen.getByText('Psychic')).toBeInTheDocument()
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.start' })) })
    expect(onStart.mock.calls[0][0]).toMatchObject({ adventure_class_id: 'xinghai-class-psychic' })
    expect(onStart.mock.calls[0][0]).not.toHaveProperty('adventure_state')
    expect(onStart.mock.calls[0][0]).not.toHaveProperty('skills')
  })

  it('chooses a real weapon and an independent internal practice without adding cultivation labels', async () => {
    const wuxia = pack('wuxia-paths')
    wuxia.creation!.adventure_weapons = ['blade', 'sword', 'staff', 'spear', 'fist'].map(id => ({ id: `lanchuan-weapon-${id}`, name: id }))
    wuxia.creation!.adventure_methods = ['restorative-breath', 'iron-breath', 'driving-breath'].map(id => ({ id: `lanchuan-method-${id}`, name: id }))
    draft.mockResolvedValue({ identity: { name: 'Traveler', age: 24, adventure_weapon_name: 'Spear', adventure_method_name: 'Rooted Guard', initial_state: { family: 'wuxia', rank: 1, currency: 0, items: ['Training Spear'], equipped: ['Training Spear'], techniques: ['Spear Form', 'Rooted Guard Breathing'] } } })
    render(<CharacterCreationDialog open pack={wuxia} onOpenChange={vi.fn()} onStart={vi.fn()} />)
    expect(screen.queryByLabelText('creation.adventureClass')).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('creation.adventureWeapon'), { target: { value: 'lanchuan-weapon-spear' } })
    fireEvent.change(screen.getByLabelText('creation.adventureMethod'), { target: { value: 'lanchuan-method-iron-breath' } })
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    expect(draft.mock.calls[0][1]).toMatchObject({ adventure_weapon_id: 'lanchuan-weapon-spear', adventure_method_id: 'lanchuan-method-iron-breath' })
    expect(screen.getByText('Spear')).toBeInTheDocument()
    expect(screen.getByText('Rooted Guard')).toBeInTheDocument()
    expect(screen.queryByText('creation.realm')).not.toBeInTheDocument()
    expect(screen.queryByText('creation.root')).not.toBeInTheDocument()
  })

  it('clears native choices when changing to a world that has no such options', async () => {
    const space = pack('native')
    space.creation!.adventure_classes = [{ id: 'psychic', name: 'Psychic', default: true }]
    const props = { open: true, onOpenChange: vi.fn(), onStart: vi.fn() }
    const view = render(<CharacterCreationDialog {...props} pack={space} />)
    expect(screen.getByLabelText('creation.adventureClass')).toHaveValue('psychic')
    view.rerender(<CharacterCreationDialog {...props} pack={pack('other')} />)
    expect(screen.queryByLabelText('creation.adventureClass')).not.toBeInTheDocument()
    draft.mockResolvedValue({ identity: { name: 'Traveler', age: 24 } })
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    expect(draft.mock.calls[0][1].adventure_class_id).toBeUndefined()
    expect(draft.mock.calls[0][1].adventure_weapon_id).toBeUndefined()
    expect(draft.mock.calls[0][1].adventure_method_id).toBeUndefined()
  })
})


describe('authored rule-world creation', () => {
  beforeEach(() => { draft.mockReset() })

  it('shows every opaque authored value without invented cultivation or native-family summaries', async () => {
    const facts = [
      { name: '姓名', type: 'string', value: 'Traveler' },
      { name: '钱财', type: 'number', value: 0 },
      { name: '历练状态', type: 'object', value: { family: 'adventure', rank: 3, currency: 50, skills: { learned: 1 }, inventory: { tool: 2 }, ritual: { soul_realm: 'mortal' }, unknown_future_fact: { source: 'authored', count: 7 } } },
      { name: '已有许可', type: 'boolean', value: false },
    ]
    draft.mockResolvedValueOnce({ identity: { name: 'Traveler', age: 24, initial_state: { currency: 0, authored_fields: facts } } })
    render(<CharacterCreationDialog open pack={pack('authored')} onOpenChange={vi.fn()} onStart={vi.fn()} />)
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    for (const field of facts) expect(screen.getByText(field.name)).toBeInTheDocument()
    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.getByText('false')).toBeInTheDocument()
    expect(screen.getByText(/unknown_future_fact/)).toHaveTextContent('"count": 7')
    expect(screen.getByText(/unknown_future_fact/)).toHaveTextContent('"soul_realm": "mortal"')
    expect(screen.queryByText('creation.realm')).not.toBeInTheDocument()
    expect(screen.queryByText('creation.currency')).not.toBeInTheDocument()
    expect(screen.queryByText('creation.rank')).not.toBeInTheDocument()
    expect(screen.getByText('creation.authoredStateHint')).toBeInTheDocument()
  })

  it('filters only the origin picker and custom bases while preserving the chosen authored location ID', async () => {
    const world = pack('restricted')
    world.creation!.origins = [
      { id: 'ordinary', name: 'Ordinary', default: true, template_id: 'basis' },
      { id: 'late', name: 'Late scenario', template_id: 'late-basis' },
      { id: 'custom', name: 'Custom', default_template_id: 'basis' },
    ]
    world.creation!.visible_origin_ids = ['ordinary']
    world.creation!.state_basis_templates = ['basis', 'second-basis', 'late-basis']
    world.creation!.custom_state_basis_templates = ['basis', 'second-basis']
    world.creation!.custom_location_policy = 'authored_only'
    world.creation!.locations = [{ id: 'auto', name: 'Auto' }, { id: 'public-square', name: 'Public square' }]
    world.protagonist_templates = [{ id: 'basis', name: 'Safe basis' }, { id: 'second-basis', name: 'Another safe basis' }, { id: 'late-basis', name: 'Late basis' }] as StoryPackSummary['protagonist_templates']
    const onStart = vi.fn()
    draft.mockResolvedValueOnce({ identity: { name: 'Traveler', age: 24, location_id: 'public-square', location_name: 'Localized square' } })
    render(<CharacterCreationDialog open pack={world} onOpenChange={vi.fn()} onStart={onStart} />)
    expect(screen.getByRole('radio', { name: /Ordinary/ })).toBeInTheDocument()
    expect(screen.queryByRole('radio', { name: /Late scenario/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: /Custom/ }))
    expect(screen.getByRole('option', { name: 'Safe basis' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Late basis' })).not.toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('creation.namePlaceholder'), { target: { value: 'Traveler' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.generate' })) })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'creation.start' })) })
    expect(onStart).toHaveBeenCalledWith(expect.objectContaining({ location_id: 'public-square', state_basis_template_id: 'basis' }))
    expect(onStart.mock.calls[0][0].location_text).toBeUndefined()
  })
})
