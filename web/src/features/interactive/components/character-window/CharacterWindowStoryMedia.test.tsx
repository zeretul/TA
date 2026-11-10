import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterWindow } from '../../types'

const fixture = vi.hoisted(() => ({ data: null as unknown }))
vi.mock('./use-character-window', () => ({
  useCharacterWindow: () => ({ data: fixture.data, loading: false, sending: false, loadingEarlier: false, hasMore: false, reload: vi.fn(), markMessage: vi.fn(), send: vi.fn() }),
}))
vi.mock('../use-player-attention', () => ({ usePlayerAttention: () => ({ data: { characters: [] }, busy: false, setFollowing: vi.fn() }) }))
vi.mock('./CharacterPictureControl', () => ({ useCharacterPicture: () => ({ state: null, setState: vi.fn() }), CharacterPictureControl: () => null }))
vi.mock('./PlacePictureControl', () => ({ usePlacePicture: () => ({ state: null, setState: vi.fn() }), PlacePictureControl: () => null }))
import { CharacterWindowPanel } from './CharacterWindowPanel'

const caption = '奥罗拉·辛尼斯塔 · 人物参考图（视觉细节为原创演绎）'
const scoped = (head: string) => `/api/interactive/stories/s/media/art?actor=sinistra&branch=main&head=${head}&revision=1`
function state(head?: string): CharacterWindow {
  return {
    story_id: 's', branch_id: 'main', scene: { node_id: 't', title: '夜间交谈', location: '天文塔', world_time: '1991年9月1日', present: ['protagonist', 'sinistra'] },
    characters: [{ id: 'sinistra', name: '奥罗拉·辛尼斯塔', known_as: '奥罗拉·辛尼斯塔', present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 0, affinity_band: 'neutral', avatar: '', avatar_lore_path: '', avatar_display: head ? scoped(head) : '', image_caption: head ? caption : '' }],
    exchanges: [], matters: [], messages: [], has_more: false,
  }
}
function Panel() {
  return <CharacterWindowPanel storyId="s" branchId="main" active characterId="sinistra" draft="" onDraftChange={() => {}} onSelectCharacter={() => {}} onClose={() => {}} />
}

describe('story-scoped portrait display', () => {
  beforeEach(() => { fixture.data = state('first') })
  it('does not create an image request before explicit open and then shows the neutral caption', async () => {
    const user = userEvent.setup()
    const { container } = render(<Panel />)
    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
    await user.click(screen.getByTestId('character-window-portrait-toggle'))
    expect(screen.getByRole('img', { name: '奥罗拉·辛尼斯塔的画像' })).toHaveAttribute('src', scoped('first'))
    expect(screen.getByTestId('character-window-image-caption')).toHaveTextContent(caption)
  })
  it('never falls back to a public URL after denial and resets failure only when a new scoped URL arrives', async () => {
    const user = userEvent.setup()
    const { container, rerender } = render(<Panel />)
    await user.click(screen.getByTestId('character-window-portrait-toggle'))
    fireEvent.error(screen.getByRole('img', { name: '奥罗拉·辛尼斯塔的画像' }))
    expect(container.querySelector('img')).toBeNull()
    expect(container.querySelector('img[src^="/api/media/"]')).toBeNull()
    fixture.data = state('new-head')
    rerender(<Panel />)
    await waitFor(() => expect(screen.getByRole('img', { name: '奥罗拉·辛尼斯塔的画像' })).toHaveAttribute('src', scoped('new-head')))
    fixture.data = state()
    rerender(<Panel />)
    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByText(caption)).not.toBeInTheDocument()
  })
})
