import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { StoryPackWorldDraft, StoryPackWorldDraftResult } from '@/lib/api-client'
import { WorldCreationDialog } from './WorldCreationDialog'

const { generate, preview, save } = vi.hoisted(() => ({
  generate: vi.fn(), preview: vi.fn(), save: vi.fn(),
}))
vi.mock('@/lib/api-client', () => ({
  draftStoryPackWorld: generate,
  previewStoryPackWorld: preview,
  saveStoryPackWorld: save,
}))
vi.mock('@/features/interactive/model-readiness', () => ({ useStoryModelReadiness: () => true }))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

function privateDraft(): StoryPackWorldDraft {
  return {
    name: 'Archive world', title: 'A visitor arrives', origin: 'Original premise',
    world: [{ name: 'Hidden archive', audience: 'gm', content: 'Author-only fact' }],
    gameplay: {
      status_template: 'world-actions-v1', numeric_scale: '1-10', growth: 'none',
      combat: 'none', communication: 'local', opening: 'commission', confirmed: true,
    },
  }
}

function response(draft = privateDraft()): StoryPackWorldDraftResult {
  return { draft, preview: { world_count: 1, character_count: 0, startable: true, notice: '' } }
}

async function showPrivateDraft() {
  generate.mockResolvedValueOnce(response())
  render(<WorldCreationDialog open onOpenChange={vi.fn()} onSaved={vi.fn()} />)
  fireEvent.change(screen.getByPlaceholderText('worldCreation.briefPlaceholder'), { target: { value: 'A town with a hidden archive' } })
  await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.generate' })) })
  expect(generate.mock.calls[0][1]).toBeUndefined()
  expect(generate.mock.calls[0][2]).toBe(false)
}

describe('world draft audience round trips', () => {
  beforeEach(() => {
    generate.mockReset()
    preview.mockReset().mockResolvedValue(response().preview)
    save.mockReset().mockResolvedValue({ pack_id: 'saved-world', summary: { id: 'saved-world', name: 'Archive world' } })
  })

  it('sends the current GM draft when regenerating, while a first draft has no previous document', async () => {
    await showPrivateDraft()
    fireEvent.change(screen.getByLabelText('worldCreation.premise'), { target: { value: 'Edited premise' } })
    generate.mockResolvedValueOnce(response({ ...privateDraft(), origin: 'Rewritten premise' }))
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.rewrite' })) })
    expect(generate.mock.calls[1][1]).toMatchObject({ origin: 'Edited premise', world: [{ audience: 'gm' }] })
    expect(generate.mock.calls[1][2]).toBe(true)
    expect(screen.getByLabelText('worldCreation.premise')).toHaveValue('Rewritten premise')
  })

  it('preserves GM audience through ordinary prose edits and save', async () => {
    await showPrivateDraft()
    fireEvent.change(screen.getByLabelText('worldCreation.premise'), { target: { value: 'Edited premise' } })
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.save' })) })
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ origin: 'Edited premise', world: [{ name: 'Hidden archive', audience: 'gm', content: 'Author-only fact' }] }))
  })

  it('carries the existing GM draft when returning to the seed and generating again', async () => {
    await showPrivateDraft()
    fireEvent.click(screen.getByRole('button', { name: 'worldCreation.modify' }))
    generate.mockResolvedValueOnce(response())
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.generate' })) })
    expect(generate.mock.calls[1][1]).toMatchObject({ world: [{ audience: 'gm' }] })
    expect(generate.mock.calls[1][2]).toBe(false)
  })

  it.each(['reported error', 'request error', 'cancelled request'])('keeps the previous GM draft after a %s', async (failure) => {
    await showPrivateDraft()
    if (failure === 'reported error') {
      generate.mockResolvedValueOnce({ ...response({ ...privateDraft(), origin: 'Incomplete replacement', world: [] }), error_key: 'worldCreation.draftFailed' })
    } else {
      generate.mockRejectedValueOnce(failure === 'cancelled request' ? new DOMException('Cancelled', 'AbortError') : new Error('Request failed'))
    }
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.rewrite' })) })
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByLabelText('worldCreation.premise')).toHaveValue('Original premise')
    expect(screen.getByText('Hidden archive')).toBeInTheDocument()
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: 'worldCreation.save' })) })
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ origin: 'Original premise', world: [{ name: 'Hidden archive', audience: 'gm', content: 'Author-only fact' }] }))
  })
})
