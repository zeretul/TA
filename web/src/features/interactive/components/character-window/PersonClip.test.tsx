import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { CharacterMediaClip } from '../../types'
import { PersonClip } from './PersonClip'
import { forgetClipAutoPlay } from './clip-playback-memory'
import { applyClipPlaybackPreference } from './clip-playback-preference'

const clips: CharacterMediaClip[] = [
  { url: '/api/media/aaaaaaaaaaaaaaaa.mp4', role: 'general', duration_ms: 5000, width: 640, height: 640 },
]

/**
 * jsdom implements no media element at all: play() is absent and the readyState
 * stays 0 forever. These stubs let a test say "the clip started" and "the clip
 * finished" without pretending to decode anything.
 */
type MediaStub = { plays: number; pauses: number }
const mediaStubs = new Map<HTMLVideoElement, MediaStub>()

/** The counters for one element, created on first touch so a play during the
 *  very first commit is still counted. */
function stubOf(video: HTMLVideoElement): MediaStub {
  const existing = mediaStubs.get(video)
  if (existing) return existing
  const created: MediaStub = { plays: 0, pauses: 0 }
  mediaStubs.set(video, created)
  return created
}

function stubMedia() {
  mediaStubs.clear()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(function (this: HTMLVideoElement) {
    stubOf(this).plays += 1
    this.dispatchEvent(new Event('play'))
    return Promise.resolve()
  })
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(function (this: HTMLVideoElement) {
    stubOf(this).pauses += 1
    this.dispatchEvent(new Event('pause'))
  })
}

beforeEach(() => {
  forgetClipAutoPlay()
  document.documentElement.dataset.novaMotion = 'full'
  applyClipPlaybackPreference(true)
  stubMedia()
})

afterEach(() => {
  vi.restoreAllMocks()
  delete document.documentElement.dataset.novaMotion
  delete document.documentElement.dataset.clipsEnabled
})

describe('PersonClip playback rules', () => {
	 it('removes the resting picture when a new scene starts the same clip', async () => {
		const {rerender}=render(<PersonClip clips={clips} name="宁茯苓" stillSrc="/still.png" autoPlayKey="scene-one" />)
		const video=screen.getByTestId('person-clip') as HTMLVideoElement
		await waitFor(()=>expect(stubOf(video).plays).toBe(1))
		fireEvent.ended(video)
		await waitFor(()=>expect(screen.getByTestId('person-clip-still')).toBeInTheDocument())
		rerender(<PersonClip clips={clips} name="宁茯苓" stillSrc="/still.png" autoPlayKey="scene-two" />)
		await waitFor(()=>expect(stubOf(video).plays).toBe(2))
		expect(screen.queryByTestId('person-clip-still')).not.toBeInTheDocument()
	 })

	 it('lets the player retry after a failed clip falls back to its still', async () => {
		const user=userEvent.setup()
		render(<PersonClip clips={clips} name="宁茯苓" stillSrc="/still.png" />)
		const video=screen.getByTestId('person-clip') as HTMLVideoElement
		fireEvent.error(video)
		await waitFor(()=>expect(screen.getByTestId('person-clip-still')).toBeInTheDocument())
		await user.click(screen.getByTestId('person-clip-replay'))
		await waitFor(()=>expect(stubOf(video).plays).toBe(1))
		expect(screen.queryByTestId('person-clip-still')).not.toBeInTheDocument()
	 })
  // The one unasked-for start: a fresh sighting, which is exactly what the key
  // names. The caller supplies the key only when the player has just opened this
  // person's picture in this scene.
  it('plays itself once for a sighting it has not shown', async () => {
    render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene/npc" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)

    await waitFor(() => expect(mediaStubs.get(video)?.plays).toBe(1))
  })

  // The same key is the same sighting. Reopening the window, a reply, a redraw
  // and a reload all present it again, and none of them may start the clip.
  it('stays still for a sighting that has already played', async () => {
    const { unmount } = render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene/npc" />)
    const first = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(first)
    await waitFor(() => expect(mediaStubs.get(first)?.plays).toBe(1))

    // A remount is what closing and reopening the window does.
    unmount()
    render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene/npc" />)
    const second = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(second)
    await waitFor(() => expect(second).toBeInTheDocument())
    expect(mediaStubs.get(second)?.plays).toBe(0)
  })

  // A scene that has actually changed is a new sighting, so it may play again.
  it('plays again once the scene changes', async () => {
    const { rerender } = render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene-a/npc" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    await waitFor(() => expect(mediaStubs.get(video)?.plays).toBe(1))

    rerender(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene-b/npc" />)
    await waitFor(() => expect(mediaStubs.get(video)?.plays).toBe(2))
  })

  it('never plays itself without a sighting key', async () => {
    render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    await waitFor(() => expect(video).toBeInTheDocument())
    expect(mediaStubs.get(video)?.plays).toBe(0)
  })

  // "关闭动态立绘或系统偏好减少动态时默认静图，仍可手动播放."
  it('stays still when motion is reduced, and still plays when asked', async () => {
    document.documentElement.dataset.novaMotion = 'reduced'
    const user = userEvent.setup()
    render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene/npc" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    await waitFor(() => expect(video).toBeInTheDocument())
    expect(mediaStubs.get(video)?.plays).toBe(0)

    await user.click(screen.getByTestId('person-clip-toggle'))
    expect(mediaStubs.get(video)?.plays).toBe(1)
  })

  // "片段播放用户开关": turning the preference off is the player's own decision
  // about their screen, so the clip waits for the play button — the clip itself
  // and its controls are never taken away.
  it('stays still when the player turned clip playback off, and still plays when asked', async () => {
    applyClipPlaybackPreference(false)
    const user = userEvent.setup()
    render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene/npc" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    await waitFor(() => expect(video).toBeInTheDocument())
    expect(mediaStubs.get(video)?.plays).toBe(0)

    await user.click(screen.getByTestId('person-clip-toggle'))
    expect(mediaStubs.get(video)?.plays).toBe(1)
  })

  // Changing the preference while a picture is on screen takes effect there,
  // rather than at the next mount: a player who just turned clips off does not
  // want the next sighting to surprise them.
  it('follows the preference as it changes', async () => {
    const { rerender } = render(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene-a/npc" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    await waitFor(() => expect(mediaStubs.get(video)?.plays).toBe(1))

    applyClipPlaybackPreference(false)
    // The preference reaches the element through an observer, so let the queued
    // callback and the redraw it causes run before the next sighting arrives.
    await act(async () => {
      await Promise.resolve()
    })

    rerender(<PersonClip clips={clips} name="宁茯苓" autoPlayKey="story/main/scene-b/npc" />)
    await act(async () => {
      await Promise.resolve()
    })
    expect(mediaStubs.get(video)?.plays).toBe(1)
  })

  // Sound is a deliberate press. The element starts muted whatever the player
  // chose last time, because a window that starts speaking takes them out of the
  // reading surface.
  it('starts muted and lets the player turn sound on', async () => {
    const user = userEvent.setup()
    render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    expect(video.muted).toBe(true)

    await user.click(screen.getByTestId('person-clip-sound'))
    expect(video.muted).toBe(false)
    expect(screen.getByTestId('person-clip-sound')).toHaveAttribute('aria-label', '静音')

    await user.click(screen.getByTestId('person-clip-sound'))
    expect(video.muted).toBe(true)
  })

  // "播放结束回到同人物静图."
  it('shows the still picture when the clip finishes', async () => {
    render(<PersonClip clips={clips} name="宁茯苓" stillSrc="/portraits/npc.png" stillAlt="宁茯苓的画像" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)

    video.dispatchEvent(new Event('ended'))
    expect(await screen.findByTestId('person-clip-still')).toHaveAttribute('src', '/portraits/npc.png')
    // The controls stay, so the player can watch it again.
    expect(screen.getByTestId('person-clip-replay')).toBeInTheDocument()
  })

  // "缺失、解码失败或不匹配时回退同人物静图."
  it('falls back to the still picture when the clip cannot be decoded', async () => {
    render(<PersonClip clips={clips} name="宁茯苓" stillSrc="/portraits/npc.png" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)

    video.dispatchEvent(new Event('error'))
    expect(await screen.findByTestId('person-clip-still')).toHaveAttribute('src', '/portraits/npc.png')
  })

  it('renders nothing rather than an empty frame when there is no still', async () => {
    const { container } = render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)

    video.dispatchEvent(new Event('error'))
    await waitFor(() => expect(container).toBeEmptyDOMElement())
  })

  // A clip that has ended keeps its last frame; the player can replay it from
  // the first frame rather than from where it stopped.
  it('replays from the first frame', async () => {
    const user = userEvent.setup()
    render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    video.dispatchEvent(new Event('ended'))

    await user.click(screen.getByTestId('person-clip-replay'))
    expect(video.currentTime).toBe(0)
    expect(mediaStubs.get(video)?.plays).toBe(1)
  })

  // "切人物、切标签、关闭展示区域、页面进入后台或播放器离开可见范围时暂停."
  it('pauses when the page is hidden', async () => {
    render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    const stub = stubOf(video)
    const user = userEvent.setup()
    await user.click(screen.getByTestId('person-clip-toggle'))
    expect(stub.plays).toBe(1)

    const hidden = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden')
    document.dispatchEvent(new Event('visibilitychange'))
    expect(stub.pauses).toBe(1)
    hidden.mockRestore()
  })

  // "切人物不串片": the element survives a switch to another person, so the old
  // picture has to stop and the new one start from nothing.
  it('stops and resets when the person changes', async () => {
    const { rerender } = render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    const stub = stubOf(video)
    const user = userEvent.setup()
    await user.click(screen.getByTestId('person-clip-toggle'))
    expect(stub.plays).toBe(1)

    rerender(<PersonClip clips={[{ ...clips[0], url: '/api/media/bbbbbbbbbbbbbbbb.mp4' }]} name="苏浅" />)
    expect(stub.pauses).toBeGreaterThanOrEqual(1)
    expect(video.getAttribute('src')).toBe('/api/media/bbbbbbbbbbbbbbbb.mp4')
  })

  // A late error from the clip we switched away from must not blank the one on
  // screen now.
  it('ignores an error that belongs to the clip it already left', async () => {
    const { container } = render(<PersonClip clips={clips} name="宁茯苓" />)
    const video = screen.getByTestId('person-clip') as HTMLVideoElement
    stubOf(video)
    // jsdom leaves currentSrc empty, which is the "cannot attribute the error"
    // case; the guard only suppresses an error that names a different file.
    Object.defineProperty(video, 'currentSrc', { configurable: true, value: '/api/media/cccccccccccccccc.mp4' })

    video.dispatchEvent(new Event('error'))
    await waitFor(() => expect(container.querySelector('video')).not.toBeNull())
  })
})
