import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterPictureState } from '../../types'

const mocks = vi.hoisted(() => ({
  getCharacterPicture: vi.fn(),
  importCharacterPicture: vi.fn(),
  clearCharacterPicture: vi.fn(),
}))

vi.mock('../../api', () => ({
  getCharacterPicture: mocks.getCharacterPicture,
  importCharacterPicture: mocks.importCharacterPicture,
  clearCharacterPicture: mocks.clearCharacterPicture,
}))

import { CharacterPictureControl } from './CharacterPictureControl'

function Host({ initial }: { initial: CharacterPictureState }) {
  const [state, setState] = useState<CharacterPictureState>(initial)
  return (
    <CharacterPictureControl
      storyId="story-1"
      branchId="branch-1"
      characterId="liuqing"
      name="柳青"
      state={state}
      onStateChange={setState}
    />
  )
}

function imported(picture: CharacterPictureState) {
  return {
    character_id: 'liuqing',
    asset: { id: 'a', file: 'u-abc.png' },
    url: '/api/media/u-abc.png',
    picture,
  }
}

describe('CharacterPictureControl', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
  })

  it('offers restore only when an imported picture is the one answering', () => {
    const { unmount } = render(<Host initial={{ url: '/api/media/u-abc.png', source: 'external', overridden: true }} />)
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeEnabled()
    expect(screen.getByText('当前显示的是导入的图片。')).toBeInTheDocument()
    unmount()

    render(<Host initial={{ url: '/api/portraits/a.png', source: 'shipped', overridden: false }} />)
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeDisabled()
    expect(screen.getByText('当前显示的是程序自带的默认头像。')).toBeInTheDocument()
  })

  it('does not offer restore for an avatar the player chose', () => {
    render(<Host initial={{ url: '/api/portraits/chosen.png', source: 'chosen', overridden: false }} />)
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeDisabled()
    expect(screen.getByText('当前显示的是玩家自己选定的头像。')).toBeInTheDocument()
  })

  it('shows the placeholder initial when the person has no picture', () => {
    render(<Host initial={{ source: '', overridden: false }} />)
    expect(screen.getByText('柳')).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(screen.getByText('这个人还没有图片，显示的是名字首字。')).toBeInTheDocument()
  })

  it('draws the imported picture and offers to restore it', async () => {
    const user = userEvent.setup()
    mocks.importCharacterPicture.mockResolvedValue(
      imported({ url: '/api/media/u-new.png', source: 'external', overridden: true }),
    )
    render(<Host initial={{ source: '', overridden: false }} />)

    const input = screen.getByLabelText('选择图片') as HTMLInputElement
    const file = new File([new Uint8Array([1, 2, 3])], 'face.png', { type: 'image/png' })
    await user.upload(input, file)

    await waitFor(() => {
      expect(mocks.importCharacterPicture).toHaveBeenCalledWith(
        'story-1',
        'liuqing',
        file,
        { branchId: 'branch-1' },
      )
    })
    expect(await screen.findByText('当前显示的是导入的图片。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeEnabled()
  })

  it('reports why an import failed instead of pretending it worked', async () => {
    const user = userEvent.setup()
    mocks.importCharacterPicture.mockRejectedValue(new Error('没有可用的素材目录'))
    render(<Host initial={{ source: '', overridden: false }} />)

    const file = new File([new Uint8Array([1])], 'face.png', { type: 'image/png' })
    await user.upload(screen.getByLabelText('选择图片') as HTMLInputElement, file)

    expect(await screen.findByText('没有可用的素材目录')).toBeInTheDocument()
    // The person still has no picture, so the placeholder is still what is drawn.
    expect(screen.getByText('柳')).toBeInTheDocument()
  })

  it('returns to the default picture when the override is removed', async () => {
    const user = userEvent.setup()
    mocks.clearCharacterPicture.mockResolvedValue(
      imported({ url: '/api/portraits/a.png', source: 'shipped', overridden: false }),
    )
    render(<Host initial={{ url: '/api/media/u-abc.png', source: 'external', overridden: true }} />)

    await user.click(screen.getByRole('button', { name: /恢复默认/ }))

    await waitFor(() => {
      expect(mocks.clearCharacterPicture).toHaveBeenCalledWith(
        'story-1',
        'liuqing',
        { branchId: 'branch-1' },
      )
    })
    expect(await screen.findByText('当前显示的是程序自带的默认头像。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeDisabled()
  })

  it('falls back to the initial when the picture cannot be read', () => {
    render(<Host initial={{ url: '/api/media/gone.png', source: 'external', overridden: true }} />)
    fireEvent.error(screen.getByRole('presentation'))

    // A broken file must not leave a broken frame: the initial is what the game
    // itself would draw, and the hint still says which layer answered.
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    expect(screen.getByText('柳')).toBeInTheDocument()
    expect(screen.getByText('当前显示的是导入的图片。')).toBeInTheDocument()
  })
})
