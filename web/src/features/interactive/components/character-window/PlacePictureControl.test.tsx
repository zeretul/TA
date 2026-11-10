import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PlacePictureState } from '../../types'

const mocks = vi.hoisted(() => ({
  getPlacePicture: vi.fn(),
  importPlacePicture: vi.fn(),
  clearPlacePicture: vi.fn(),
}))

vi.mock('../../api', () => ({
  getPlacePicture: mocks.getPlacePicture,
  importPlacePicture: mocks.importPlacePicture,
  clearPlacePicture: mocks.clearPlacePicture,
}))

import { PlacePictureControl, usePlacePicture } from './PlacePictureControl'

/** The host holds the state, exactly as SceneBlock does. */
function Host({ initial }: { initial: PlacePictureState | null }) {
  const [state, setState] = useState<PlacePictureState | null>(initial)
  return (
    <PlacePictureControl
      storyId="story-1"
      place="青石渡口"
      state={state}
      onStateChange={setState}
    />
  )
}

function imported(picture: PlacePictureState) {
  return { place: '青石渡口', world: 'fanren-jie', asset: { id: 'a', file: 'u-abc.png' }, url: '/api/media/u-abc.png', picture }
}

describe('PlacePictureControl', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
  })

  it('says a place with no picture shows only its text', () => {
    render(<Host initial={null} />)

    expect(screen.getByText('这个地方还没有图片，只显示文字。')).toBeInTheDocument()
    // No empty frame: the placeholder is the place's first character.
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    expect(screen.getByText('青')).toBeInTheDocument()
  })

  it('offers no restore until an imported picture is the one answering', () => {
    const first = render(<Host initial={{ overridden: false }} />)
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeDisabled()
    first.unmount()

    render(<Host initial={{ url: '/api/media/u-abc.png', overridden: true }} />)
    expect(screen.getByRole('button', { name: /恢复默认/ })).toBeEnabled()
  })

  it('draws the imported picture and says it is the imported one', () => {
    render(<Host initial={{ url: '/api/media/u-abc.png', overridden: true }} />)

    expect(screen.getByRole('presentation')).toHaveAttribute('src', '/api/media/u-abc.png')
    expect(screen.getByText('当前显示的是导入的地点图片。')).toBeInTheDocument()
  })

  it('sends the chosen file to the place on screen', async () => {
    mocks.importPlacePicture.mockResolvedValue(
      imported({ url: '/api/media/u-abc.png', overridden: true }),
    )
    const user = userEvent.setup()

    render(<Host initial={null} />)
    const file = new File(['png'], 'dukou.png', { type: 'image/png' })
    await user.upload(screen.getByLabelText('选择图片'), file)

    // The author picked a file, not a place: the control is already bound to the
    // place the story stands in.
    expect(mocks.importPlacePicture).toHaveBeenCalledWith('story-1', '青石渡口', file)
    await waitFor(() => {
      expect(screen.getByRole('presentation')).toHaveAttribute('src', '/api/media/u-abc.png')
    })
  })

  it('reports why an import failed instead of pretending it worked', async () => {
    mocks.importPlacePicture.mockRejectedValue(new Error('这个地点没有登记在这个故事里'))
    const user = userEvent.setup()

    render(<Host initial={null} />)
    await user.upload(
      screen.getByLabelText('选择图片'),
      new File(['png'], 'dukou.png', { type: 'image/png' }),
    )

    expect(await screen.findByText('这个地点没有登记在这个故事里')).toBeInTheDocument()
    expect(screen.getByText('这个地方还没有图片，只显示文字。')).toBeInTheDocument()
  })

  it('returns to the text-only state when the override is removed', async () => {
    mocks.clearPlacePicture.mockResolvedValue({ place: '青石渡口', picture: { overridden: false } })
    const user = userEvent.setup()

    render(<Host initial={{ url: '/api/media/u-abc.png', overridden: true }} />)
    await user.click(screen.getByRole('button', { name: /恢复默认/ }))

    expect(mocks.clearPlacePicture).toHaveBeenCalledWith('story-1', '青石渡口')
    await waitFor(() => {
      expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    })
    expect(screen.getByText('这个地方还没有图片，只显示文字。')).toBeInTheDocument()
  })

  it('falls back to the initial when the picture cannot be read', () => {
    render(<Host initial={{ url: '/api/media/u-abc.png', overridden: true }} />)

    fireEvent.error(screen.getByRole('presentation'))

    // A file the manifest still lists but that is gone must not leave a broken
    // frame; the place falls back to what the game itself would draw.
    expect(screen.queryByRole('presentation')).not.toBeInTheDocument()
    expect(screen.getByText('青')).toBeInTheDocument()
  })
})

describe('usePlacePicture', () => {
  beforeEach(() => {
    for (const mock of Object.values(mocks)) mock.mockReset()
  })

  function Probe({ enabled }: { enabled: boolean }) {
    const { state } = usePlacePicture('story-1', '青石渡口', enabled)
    return <span data-testid="probe">{state?.url || 'none'}</span>
  }

  it('reads nothing until the place is asked about', () => {
    render(<Probe enabled={false} />)

    expect(mocks.getPlacePicture).not.toHaveBeenCalled()
    expect(screen.getByTestId('probe')).toHaveTextContent('none')
  })

  it('reads the place once it is asked about', async () => {
    mocks.getPlacePicture.mockResolvedValue({ url: '/api/media/u-abc.png', overridden: true })

    render(<Probe enabled />)

    await waitFor(() => {
      expect(screen.getByTestId('probe')).toHaveTextContent('/api/media/u-abc.png')
    })
    expect(mocks.getPlacePicture).toHaveBeenCalledWith('story-1', '青石渡口')
  })

  it('keeps the window open when the place cannot be read', async () => {
    mocks.getPlacePicture.mockRejectedValue(new Error('这个故事不知道这个地方'))

    render(<Probe enabled />)

    // A place the story never registered is not a reason the scene stops drawing
    // its text, so the read fails quietly.
    await waitFor(() => {
      expect(mocks.getPlacePicture).toHaveBeenCalled()
    })
    expect(screen.getByTestId('probe')).toHaveTextContent('none')
  })
})
