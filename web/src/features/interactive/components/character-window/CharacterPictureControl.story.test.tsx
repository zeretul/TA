import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { CharacterPictureState } from '../../types'

const mocks = vi.hoisted(() => ({
  getCharacterPicture: vi.fn(),
  importCharacterPicture: vi.fn(),
  clearCharacterPicture: vi.fn(),
  onStateChange: vi.fn(),
}))
vi.mock('../../api', () => ({
  getCharacterPicture: mocks.getCharacterPicture,
  importCharacterPicture: mocks.importCharacterPicture,
  clearCharacterPicture: mocks.clearCharacterPicture,
}))
import { CharacterPictureControl } from './CharacterPictureControl'

const scopedURL = '/api/interactive/stories/story-1/media/art?actor=liuqing&branch=main&head=known-head&revision=1'
const storyHint = '这张参考图按本局当前已知事实展示。'
const storyUnavailableHint = '当前没有可显示的剧情参考图。'
const storyReadOnlyHint = '剧情参考图由本局记录提供，不能在这里替换或恢复。'
const noneHint = '这个人还没有图片，显示的是名字首字。'

function Control({ state }: { state: CharacterPictureState | null }) {
  return <CharacterPictureControl storyId="story-1" branchId="main" characterId="liuqing" name="柳青" state={state} onStateChange={mocks.onStateChange} />
}

beforeEach(() => {
  setConfiguredLocale('zh-CN')
  for (const mock of Object.values(mocks)) mock.mockReset()
})
afterEach(() => { setConfiguredLocale('zh-CN') })

describe('story-scoped picture source hint and read-only controls', () => {
  it.each([
    ['zh-CN', storyHint, '恢复默认', '选择图片', noneHint],
    ['en-US', 'This reference image is shown based on what is currently known in this story.', 'Restore the default', 'Choose a picture', 'This person has no picture yet, so their initial is shown.'],
  ] as const)('describes a scoped reference in %s without offering a picture override', (locale, hint, restore, choose, missing) => {
    setConfiguredLocale(locale)
    render(<Control state={{ source: 'story', url: scopedURL, overridden: false }} />)
    expect(screen.getByText(hint)).toBeInTheDocument()
    expect(screen.queryByText(missing)).not.toBeInTheDocument()
    expect(screen.getByRole('presentation')).toHaveAttribute('src', scopedURL)
    expect(screen.getByRole('button', { name: restore })).toBeDisabled()
    expect(screen.getByRole('button', { name: choose })).toBeDisabled()
    expect(screen.getByLabelText(choose)).toBeDisabled()
    expect(screen.getByTestId('character-picture-control')).toHaveAttribute('data-source', 'story')
  })

  it.each([
    ['zh-CN', storyUnavailableHint, storyReadOnlyHint, storyHint],
    ['en-US', 'No story reference image is currently available to display.', 'Story reference images come from this story’s records and cannot be replaced or restored here.', 'This reference image is shown based on what is currently known in this story.'],
  ] as const)('describes a genuinely empty story-managed projection in %s', (locale, unavailable, readOnly, shown) => {
    setConfiguredLocale(locale)
    render(<Control state={{ source: 'story', url: '', overridden: false }} />)
    expect(screen.getByText(unavailable)).toBeInTheDocument()
    expect(screen.getByText(readOnly)).toBeInTheDocument()
    expect(screen.queryByText(shown)).not.toBeInTheDocument()
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    expect(screen.getByText('柳')).toBeInTheDocument()
  })

  it('shows unavailable when visibility is withdrawn while preserving story ownership', () => {
    const { rerender } = render(<Control state={{ source: 'story', url: scopedURL, overridden: false }} />)
    expect(screen.getByText(storyHint)).toBeInTheDocument()
    rerender(<Control state={{ source: 'story', overridden: false }} />)
    expect(screen.queryByText(storyHint)).not.toBeInTheDocument()
    expect(screen.getByText(storyUnavailableHint)).toBeInTheDocument()
    expect(screen.queryByText(noneHint)).not.toBeInTheDocument()
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择图片' })).toBeDisabled()
  })

  it('reports a failed scoped image accurately and recovers only with a new scoped URL', () => {
    const { container, rerender } = render(<Control state={{ source: 'story', url: scopedURL, overridden: false }} />)
    fireEvent.error(screen.getByRole('presentation'))
    expect(container.querySelector('img')).toBeNull()
    expect(screen.getByText('柳')).toBeInTheDocument()
    expect(screen.getByText(storyUnavailableHint)).toBeInTheDocument()
    expect(screen.queryByText(storyHint)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '恢复默认' })).toBeDisabled()
    expect(screen.getByRole('button', { name: '选择图片' })).toBeDisabled()
    rerender(<Control state={{ source: 'story', url: `${scopedURL}-new`, overridden: false }} />)
    expect(screen.getByRole('presentation')).toHaveAttribute('src', `${scopedURL}-new`)
    expect(screen.getByText(storyHint)).toBeInTheDocument()
    expect(screen.queryByText(storyUnavailableHint)).not.toBeInTheDocument()
  })

  it.each([scopedURL, ''])('never sends an import or restore for a story-managed source with URL %s', async (url) => {
    const user = userEvent.setup()
    // Even a stale override flag must not permit writing to a story source.
    render(<Control state={{ source: 'story', url, overridden: true }} />)
    const input = screen.getByLabelText('选择图片') as HTMLInputElement
    const file = new File([new Uint8Array([1])], 'ignored.png', { type: 'image/png' })
    expect(input).toBeDisabled()
    await user.upload(input, file)
    await user.click(screen.getByRole('button', { name: '选择图片' }))
    await user.click(screen.getByRole('button', { name: '恢复默认' }))
    // A file picker opened before the source changed must also be harmless.
    fireEvent.change(input, { target: { files: [file] } })
    expect(mocks.importCharacterPicture).not.toHaveBeenCalled()
    expect(mocks.clearCharacterPicture).not.toHaveBeenCalled()
    expect(mocks.onStateChange).not.toHaveBeenCalled()
  })

  it.each([
    ['chosen', '当前显示的是玩家自己选定的头像。', false],
    ['external', '当前显示的是导入的图片。', true],
    ['lore', '当前显示的是世界书里的图片。', false],
    ['shipped', '当前显示的是程序自带的默认头像。', false],
  ] as const)('preserves the existing %s source hint and override permission', (source, hint, overridden) => {
    render(<Control state={{ source, url: '/api/portraits/existing.png', overridden }} />)
    expect(screen.getByText(hint)).toBeInTheDocument()
    expect(screen.queryByText(storyHint)).not.toBeInTheDocument()
    expect(screen.queryByText(storyReadOnlyHint)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择图片' })).toBeEnabled()
    expect(screen.getByLabelText('选择图片')).toBeEnabled()
    const restore = screen.getByRole('button', { name: '恢复默认' })
    if (overridden) expect(restore).toBeEnabled()
    else expect(restore).toBeDisabled()
  })

  it.each(['', 'chosen', 'external', 'lore', 'shipped'] as const)('still imports a selected file for ordinary source %s', async (source) => {
    const user = userEvent.setup()
    const imported = { source: 'external', url: '/api/media/imported.png', overridden: true }
    mocks.importCharacterPicture.mockResolvedValue({ picture: imported })
    render(<Control state={{ source, overridden: source === 'external' }} />)
    const file = new File([new Uint8Array([1, 2, 3])], 'face.png', { type: 'image/png' })
    await user.upload(screen.getByLabelText('选择图片') as HTMLInputElement, file)
    await waitFor(() => {
      expect(mocks.importCharacterPicture).toHaveBeenCalledWith('story-1', 'liuqing', file, { branchId: 'main' })
      expect(mocks.onStateChange).toHaveBeenCalledWith(imported)
    })
  })

  it('keeps the ordinary missing-picture hint when no source has answered', () => {
    render(<Control state={null} />)
    expect(screen.getByText(noneHint)).toBeInTheDocument()
    expect(screen.queryByText(storyHint)).not.toBeInTheDocument()
    expect(screen.queryByText(storyUnavailableHint)).not.toBeInTheDocument()
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
  })
})
