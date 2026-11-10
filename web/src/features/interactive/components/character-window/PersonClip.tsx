import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pause, Play, RotateCcw, Volume2, VolumeX } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { CharacterMediaClip } from '../../types'
import { claimClipPlayback, clipAutoPlayed, releaseClipPlayback, rememberClipAutoPlay } from './clip-playback-memory'
import { CLIP_PLAYBACK_ATTRIBUTE, clipsAllowed } from './clip-playback-preference'

/**
 * One reviewed clip, played where the person's still picture would be.
 *
 * The rules, in the order they matter here:
 *
 *  - It is muted, inline, and plays once. No loop, no fullscreen, no focus. A
 *    clip is a moment of a person going about their day, not a background; when
 *    it ends the still picture is what remains, and the player presses play to
 *    see it again. Sound is a deliberate press, never a surprise.
 *  - It never advances anything. Playing, pausing and replaying are local to
 *    this element: no turn is sent, no clock moves, no resource changes.
 *  - It plays itself at most once per person per scene, and only when the caller
 *    supplies an `autoPlayKey` for a sighting that has not played yet. A new line
 *    of dialogue, a redraw, scrolling, reopening the window, a reload, or a cold
 *    restart all leave it still. The memory of what has already played lives in
 *    `clip-playback-memory`, outside this component, so remounting cannot reset
 *    it.
 *  - Only one clip moves at a time. Starting this one pauses any other.
 *  - It stops when the player stops looking at it: on unmount, on switching to
 *    another person, when the page is hidden, and when the element scrolls out
 *    of view. Coming back does not restart it.
 *  - A clip that will not load, or that has finished, gives way to the person's
 *    still picture. The player never gets an empty frame, and a broken file
 *    costs them a clip rather than a picture.
 *
 * When the app's motion preference is off or reduced, or the system asks for
 * reduced motion, the clip starts still and stays still until the player presses
 * play. The controls remain, so the picture is never taken away — only the
 * unasked-for movement.
 */
export function PersonClip({
  clips,
  name,
  className,
  testId = 'person-clip',
  autoPlayKey,
  stillSrc,
  stillAlt,
}: {
  clips: CharacterMediaClip[]
  name: string
  className?: string
  testId?: string
  /**
   * Identifies one sighting: this person, in this scene, in this story. When it
   * is set and has never played itself before, the clip starts once. Absent
   * means manual playback only.
   */
  autoPlayKey?: string
  /** The person's still picture, shown once the clip ends or cannot be decoded. */
  stillSrc?: string
  stillAlt?: string
}) {
  const { t } = useTranslation()
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [index, setIndex] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [failed, setFailed] = useState(false)
  const [ended, setEnded] = useState(false)
  const [muted, setMuted] = useState(true)
  const [stillBroken, setStillBroken] = useState(false)
  const stillMotion = useStillMotion()
  const clipsOn = useClipsEnabled()

  const clip = clips[index] || clips[0]
  const clipURL = clip?.url || ''
  const resting = ended || failed
  const showStill = resting && Boolean(stillSrc) && !stillBroken

  // Stable, so the one-at-a-time registry can recognise this element's pause.
  const pause = useCallback(() => {
    videoRef.current?.pause()
  }, [])

  // A different person or a different set of clips is a different picture, so
  // nothing about the previous one carries over. The read returns a fresh array
  // each time, so the reset is keyed on the files rather than on the array.
  const signature = clips.map((item) => item.url).join('|')
  const shownSignature = useRef(signature)
  useEffect(() => setStillBroken(false), [stillSrc])
  useEffect(() => {
    // The first run is the first picture, not a switch: there is nothing playing
    // yet, and pausing an element that was just created would count as a stop.
    if (shownSignature.current === signature) return
    shownSignature.current = signature
    // Switching to another person must not leave the previous clip playing: the
    // element survives the switch and would otherwise keep the old picture
    // running under the new name.
    videoRef.current?.pause()
    setIndex(0)
    setFailed(false)
    setEnded(false)
    setPlaying(false)
    setMuted(true)
  }, [signature])

  // Sound is applied to the element rather than left to the attribute, because
  // the attribute alone is not reliably reflected after a source change.
  useEffect(() => {
    const video = videoRef.current
    if (video) video.muted = muted
  }, [muted, clipURL])

  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    const stop = () => {
      video.pause()
      setPlaying(false)
    }
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') stop()
    }
    document.addEventListener('visibilitychange', onVisibility)
    return () => {
      document.removeEventListener('visibilitychange', onVisibility)
      stop()
      releaseClipPlayback(pause)
    }
  }, [pause, clipURL])

  // Leaving the visible area stops the picture. Scrolling a long conversation
  // past the portrait is the player looking away, and returning to it is not a
  // request to start again.
  useEffect(() => {
    const video = videoRef.current
    if (!video || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) video.pause()
      }
    }, { threshold: 0.01 })
    observer.observe(video)
    return () => observer.disconnect()
  }, [clipURL])

  // The one unasked-for start. Everything about it is deliberately narrow: a
  // caller-supplied key that names one sighting, motion that allows it, the
  // player's own playback preference, and a key that has not been seen before.
  // The key is recorded *before* the attempt so a browser that refuses autoplay
  // cannot have it retried on every redraw. A player who turned clips off keeps
  // the play button, so the picture is never taken away.
  useEffect(() => {
    const video = videoRef.current
    if (!video || !clipURL || failed || !autoPlayKey || stillMotion || !clipsOn || document.visibilityState === 'hidden') return
    if (clipAutoPlayed(autoPlayKey)) return
    rememberClipAutoPlay(autoPlayKey)
    setEnded(false)
    video.muted = true
    void video.play().catch(() => {
      // Refused: the play button is the control the player would have pressed
      // anyway, and nothing retries on its own.
      setPlaying(false)
    })
  }, [autoPlayKey, clipURL, failed, stillMotion, clipsOn])

  if (!clip) return null
  // A failed clip with no still to fall back on leaves nothing to look at, and
  // an empty frame is worse than no frame. A clip that merely finished keeps its
  // last frame and its controls, so the player can press play again.
  if (failed && !showStill) return null

  const start = () => {
    const video = videoRef.current
    if (!video) return
    setEnded(false)
    setFailed(false)
    // A finished clip restarts from its first frame; the player asked to see it
    // again rather than to sit on the last frame.
    if (video.currentTime >= (video.duration || 0)) video.currentTime = 0
    void video.play().catch(() => setPlaying(false))
  }

  const toggle = () => {
    const video = videoRef.current
    if (!video) return
    if (video.paused) start()
    else video.pause()
  }

  const replay = () => {
    const video = videoRef.current
    if (!video) return
    setEnded(false)
    setFailed(false)
    video.currentTime = 0
    void video.play().catch(() => setPlaying(false))
  }

  const toggleSound = () => {
    const video = videoRef.current
    const next = !muted
    setMuted(next)
    if (video) video.muted = next
  }

  return (
    <div className={cn('relative overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)]', className)}>
      {(
        <video
          ref={videoRef}
          data-testid={testId}
          data-clip-url={clip.url}
          src={clip.url}
          hidden={failed}
          muted
          playsInline
          preload="metadata"
          width={clip.width || undefined}
          height={clip.height || undefined}
          aria-label={t('characterWindow.clipAlt', { name })}
          className="block max-h-[288px] w-auto max-w-full object-contain"
          onPlay={() => {
            setPlaying(true)
            // Two moving pictures side by side read as one confused scene, so
            // the newest start silences whatever else was running.
            claimClipPlayback(pause)
          }}
          onPause={() => {
            setPlaying(false)
            releaseClipPlayback(pause)
          }}
          onEnded={() => {
            setPlaying(false)
            setEnded(true)
            releaseClipPlayback(pause)
          }}
          onError={() => {
            // A late error from the clip we just switched away from must not
            // blank the one now on screen.
            const video = videoRef.current
            const source = video?.currentSrc || video?.src || ''
            if (source && !source.endsWith(clip.url)) return
            setFailed(true)
            setPlaying(false)
            releaseClipPlayback(pause)
          }}
        />
      )}
      {/* The still sits over the finished clip rather than replacing it, so the
          element keeps its box and the visibility check keeps working across the
          switch from moving picture to still. */}
      {showStill ? (
        <img
          data-testid={`${testId}-still`}
          src={stillSrc}
          alt={stillAlt || t('characterWindow.clipAlt', { name })}
          onError={() => setStillBroken(true)}
          className={cn(
            'block max-h-[288px] w-auto max-w-full object-contain',
            !failed && 'absolute inset-0 h-full w-full',
          )}
        />
      ) : null}
      <div className="absolute bottom-1 right-1 flex items-center gap-1">
        <button
          type="button"
          data-testid={`${testId}-toggle`}
          onClick={toggle}
          aria-label={playing ? t('characterWindow.clipPause') : t('characterWindow.clipPlay')}
          title={playing ? t('characterWindow.clipPause') : t('characterWindow.clipPlay')}
          className="flex h-6 w-6 items-center justify-center rounded-full bg-black/55 text-white/90 backdrop-blur-sm hover:bg-black/70"
        >
          {playing ? <Pause className="h-3 w-3" /> : <Play className="h-3 w-3" />}
        </button>
        <button
          type="button"
          data-testid={`${testId}-replay`}
          onClick={replay}
          aria-label={t('characterWindow.clipReplay')}
          title={t('characterWindow.clipReplay')}
          className="flex h-6 w-6 items-center justify-center rounded-full bg-black/55 text-white/90 backdrop-blur-sm hover:bg-black/70"
        >
          <RotateCcw className="h-3 w-3" />
        </button>
        <button
          type="button"
          data-testid={`${testId}-sound`}
          onClick={toggleSound}
          aria-label={muted ? t('characterWindow.clipUnmute') : t('characterWindow.clipMute')}
          title={muted ? t('characterWindow.clipUnmute') : t('characterWindow.clipMute')}
          className="flex h-6 w-6 items-center justify-center rounded-full bg-black/55 text-white/90 backdrop-blur-sm hover:bg-black/70"
        >
          {muted ? <VolumeX className="h-3 w-3" /> : <Volume2 className="h-3 w-3" />}
        </button>
      </div>
      {clips.length > 1 ? (
        <div className="absolute bottom-1 left-1 flex items-center gap-1">
          {clips.map((item, at) => (
            <button
              key={item.url}
              type="button"
              data-testid={`${testId}-pick-${at}`}
              onClick={() => {
                setIndex(at)
                setEnded(false)
                setFailed(false)
                setPlaying(false)
              }}
              aria-label={t('characterWindow.clipOther', { index: at + 1 })}
              title={t('characterWindow.clipOther', { index: at + 1 })}
              className={cn(
                'h-1.5 w-1.5 rounded-full',
                at === index ? 'bg-white/95' : 'bg-white/45 hover:bg-white/70',
              )}
            />
          ))}
        </div>
      ) : null}
    </div>
  )
}

/**
 * Whether moving pictures should stay still until asked.
 *
 * Two sources say so: the app's own motion preference (off, or reduced), and the
 * system's reduced-motion request when the app is following the system. Both are
 * read from the DOM rather than from a hook, because this component is also
 * rendered in tests and in the intimacy surface, neither of which sits under the
 * app's motion provider.
 */
function useStillMotion(): boolean {
  const [still, setStill] = useState(motionIsStill)
  useEffect(() => {
    const media = window.matchMedia?.('(prefers-reduced-motion: reduce)')
    const update = () => setStill(motionIsStill())
    media?.addEventListener?.('change', update)
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-nova-motion'] })
    return () => {
      media?.removeEventListener?.('change', update)
      observer.disconnect()
    }
  }, [])
  return still
}

function motionIsStill(): boolean {
  if (typeof document === 'undefined') return false
  const mode = document.documentElement.dataset.novaMotion
  if (mode === 'off' || mode === 'reduced') return true
  if (mode === 'full') return false
  return Boolean(window.matchMedia?.('(prefers-reduced-motion: reduce)').matches)
}

/**
 * The player's clip playback preference, watched so a change takes effect on the
 * picture already on screen rather than at the next mount.
 */
function useClipsEnabled(): boolean {
  const [enabled, setEnabled] = useState(clipsAllowed)
  useEffect(() => {
    const observer = new MutationObserver(() => setEnabled(clipsAllowed()))
    observer.observe(document.documentElement, { attributes: true, attributeFilter: [CLIP_PLAYBACK_ATTRIBUTE] })
    return () => observer.disconnect()
  }, [])
  return enabled
}
