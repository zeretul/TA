import { useState } from 'react'
import { cn } from '@/lib/utils'
import { projectFileAssetURL } from '@/lib/api-client/project-files'

/**
 * One person's portrait in a window that names people.
 *
 * The design draws a portrait only where the player has asked to look at a
 * person — the character window, a group roster, a conversation — never in the
 * main story text. So this is a small, shared shape rather than a story-stage
 * feature: a circle, the person's own picture when the world gave them one, and
 * a neutral initial when it did not.
 *
 * Two kinds of picture can answer. `avatar` is already a URL the read returned.
 * `lorePath` is the world book entry's own picture, which the read reports as a
 * project-relative path because only the client knows which project it is
 * reading; it is addressed through the project asset route here.
 *
 * A person invented by the model has no authored key and therefore no picture;
 * that is an ordinary answer, not a failure, and it must not leave a broken
 * image on screen. A picture that fails to load is treated the same way, because
 * the roster is still usable without it.
 */
export function personAvatarSource(avatar?: string, lorePath?: string, projectId?: string): string {
  if (avatar) return avatar
  return lorePath && projectId ? projectFileAssetURL(projectId, lorePath) : ''
}

export function PersonAvatar({
  avatar,
  lorePath,
  projectId,
  name,
  size = 'sm',
  className,
}: {
  avatar?: string
  lorePath?: string
  projectId?: string
  name?: string
  /**
   * `md` is the conversation-list size: a list row is what the player scans to
   * find a person, so its picture is bigger than a roster's inline mark and
   * smaller than a portrait.
   */
  size?: 'xs' | 'sm' | 'md'
  className?: string
}) {
  const [failedSource, setFailedSource] = useState('')
  const label = (name || '').trim().slice(0, 1) || '?'
  const box = size === 'xs' ? 'h-4 w-4 text-[9px]' : size === 'md' ? 'h-10 w-10 text-[15px]' : 'h-8 w-8 text-[12px]'
  const src = personAvatarSource(avatar, lorePath, projectId)
  if (!src || failedSource === src) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          'flex shrink-0 items-center justify-center rounded-full border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[var(--nova-text-muted)]',
          box,
          className,
        )}
      >
        {label}
      </span>
    )
  }
  return (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      onError={() => setFailedSource(src)}
      className={cn('shrink-0 rounded-full border border-[var(--nova-border)] object-cover', box, className)}
    />
  )
}

/**
 * The same person's picture, larger, for the detail of a person.
 *
 * §4 asks for a modest enlargement beside their name, keeping the picture's own
 * proportions rather than stretching or cropping it to a shape the world never
 * gave it. A circle crops a tall painting at the chin, so this one is a plain
 * rounded rectangle that fits inside a fixed box and never distorts. When there
 * is no picture it falls back to the same neutral initial the roster uses, so the
 * detail keeps its shape instead of collapsing.
 */
export function PersonPortrait({
  avatar,
  lorePath,
  projectId,
  name,
  caption,
  className,
}: {
  avatar?: string
  lorePath?: string
  projectId?: string
  name?: string
  /** Shown only while this source is available; hidden on image failure. */
  caption?: string
  className?: string
}) {
  const [failedSource, setFailedSource] = useState('')
  const label = (name || '').trim().slice(0, 1) || '?'
  const src = personAvatarSource(avatar, lorePath, projectId)
  if (!src || failedSource === src) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          'flex h-24 w-24 shrink-0 items-center justify-center rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] text-[28px] text-[var(--nova-text-muted)]',
          className,
        )}
      >
        {label}
      </span>
    )
  }
  const portrait = (
    <img
      src={src}
      alt=""
      aria-hidden="true"
      onError={() => setFailedSource(src)}
      className={cn('h-24 w-24 shrink-0 rounded-lg border border-[var(--nova-border)] bg-[var(--nova-surface-2)] object-contain', className)}
    />
  )
  return caption ? (
    <figure className="flex max-w-[192px] shrink-0 flex-col items-start gap-1">
      {portrait}
      <figcaption className="text-[11px] text-[var(--nova-text-muted)]">{caption}</figcaption>
    </figure>
  ) : portrait
}
