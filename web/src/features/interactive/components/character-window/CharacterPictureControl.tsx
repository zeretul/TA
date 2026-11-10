import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ImagePlus, Loader2, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { clearCharacterPicture, getCharacterPicture, importCharacterPicture } from '../../api'
import type { CharacterPictureState } from '../../types'

const ACCEPT = 'image/png,image/jpeg,image/webp'

/**
 * The shell both material controls share: a preview, a hint about which layer
 * answered, one button to import a file and one to drop an override.
 *
 * It is presentational on purpose. A person's picture and a place's picture ask
 * the same three questions but answer them from different fields, so the shell
 * owns the drawing and the busy/error state and each caller owns what it means.
 */
export function PictureControlFrame({
  testId,
  name,
  state,
  hint,
  canRestore,
  onImport,
  onRestore,
  resetKey,
  className,
}: {
  testId: string
  /** The subject's name, used for the placeholder initial. */
  name: string
  /** What to draw now. Empty means the placeholder initial. */
  state: { url?: string; source?: string } | null
  hint: string
  canRestore: boolean
  onImport: (file: File) => Promise<void>
  onRestore: () => Promise<void>
  /** A change of subject clears the last subject's error and preview. */
  resetKey: string
  className?: string
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState<'' | 'import' | 'clear'>('')
  const [error, setError] = useState('')
  // Bumped after every change so the picture element is rebuilt rather than
  // reusing the frame that held the previous file. A replaced file already has
  // a new name; a restore may return to a name this frame drew before.
  const [revision, setRevision] = useState(0)
  // The URL the preview could not read. A file the server still lists but that
  // is gone or corrupt must not leave a broken frame: the initial is what the
  // game itself would draw.
  const [brokenURL, setBrokenURL] = useState('')
  const inputRef = useRef<HTMLInputElement | null>(null)
  const source = state?.source || ''
  // Story-managed references follow committed visibility, including empty
  // projections. An imported override cannot replace this source.
  const readOnly = source === 'story'

  // A different subject means a different picture, and an error about the last
  // one must not sit under the next.
  useEffect(() => {
    setError('')
    setBusy('')
    setBrokenURL('')
    setRevision((value) => value + 1)
  }, [resetKey])

  const run = (kind: 'import' | 'clear', work: () => Promise<void>) => {
    if (readOnly) return
    setBusy(kind)
    setError('')
    work()
      .then(() => {
        setBrokenURL('')
        setRevision((value) => value + 1)
        setBusy('')
      })
      .catch((failure) => {
        setError(errorText(failure))
        setBusy('')
      })
  }

  const preview = state?.url && state.url !== brokenURL ? state.url : ''
  const displayHint = readOnly && !preview ? t('characterWindow.picture.storyUnavailableHint') : hint
  const working = Boolean(busy)

  return (
    <div data-testid={testId} data-source={source} className={cn('mt-1.5 flex flex-col gap-1.5', className)}>
      <div className="flex items-start gap-2">
        <div className="flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-2)]">
          {preview ? (
            <img
              key={`${preview}#${revision}`}
              src={preview}
              // Decorative: this is a preview of the picture drawn beside it, and
              // the hint below says which layer answered. Naming it again would
              // announce the same image twice.
              alt=""
              onError={() => setBrokenURL(preview)}
              className="h-full w-full object-cover"
            />
          ) : (
            <span aria-hidden="true" className="text-[18px] text-[var(--nova-text-muted)]">{(name || '').trim().slice(0, 1) || '?'}</span>
          )}
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <p className="text-[11px] text-[var(--nova-text-muted)]">{displayHint}</p>
          <div className="flex flex-wrap items-center gap-1.5">
            <input
              ref={inputRef}
              type="file"
              accept={ACCEPT}
              disabled={readOnly}
              className="hidden"
              aria-label={t('characterWindow.picture.pick')}
              onChange={(event) => {
                const file = event.currentTarget.files?.[0]
                event.currentTarget.value = ''
                if (file) run('import', () => onImport(file))
              }}
            />
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 text-[11px]"
              disabled={working || readOnly}
              onClick={() => inputRef.current?.click()}
            >
              {busy === 'import' ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <ImagePlus className="mr-1 h-3 w-3" />}
              {busy === 'import' ? t('characterWindow.picture.importing') : t('characterWindow.picture.pick')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 text-[11px]"
              disabled={working || readOnly || !canRestore}
              onClick={() => run('clear', onRestore)}
            >
              {busy === 'clear' ? <Loader2 className="mr-1 h-3 w-3 animate-spin" /> : <RotateCcw className="mr-1 h-3 w-3" />}
              {t('characterWindow.picture.restore')}
            </Button>
          </div>
        </div>
      </div>
      {error ? <p className="text-[11px] text-[var(--nova-danger)]">{error}</p> : null}
      <p className="text-[10px] text-[var(--nova-text-faint)]">{t(readOnly ? 'characterWindow.picture.storyReadOnlyHint' : 'characterWindow.picture.hint')}</p>
    </div>
  )
}

interface CharacterPictureControlProps {
  storyId: string
  branchId: string
  characterId: string
  /** The person's name, used as the picture's alt text. */
  name: string
  /**
   * The picture as the window would draw it. The caller holds it because the
   * window draws the same picture elsewhere; a second copy here could disagree.
   */
  state: CharacterPictureState | null
  onStateChange: (next: CharacterPictureState) => void
  className?: string
}

/**
 * 素材：给一个人换一张图，或者把他退回原来的图。
 *
 * The author generates a picture elsewhere, drops the file here, and the server
 * copies it into the media directory and writes the manifest. Nothing is
 * compiled and no path is typed: the person is named by their place in this
 * story, and the server works the binding out from what the story committed, so
 * this control cannot point a face at somebody else.
 *
 * It never guesses what the person looks like. The state it draws is the answer
 * the server gave, which is the same chain the game uses — a chosen avatar, an
 * imported picture, the world book's picture, the shipped default.
 */
export function CharacterPictureControl({ storyId, branchId, characterId, name, state, onStateChange, className }: CharacterPictureControlProps) {
  const { t } = useTranslation()
  const source = state?.source || ''
  // Only an imported picture is the author's to remove. A chosen avatar is the
  // player's own field and a world-book picture belongs to the entry, so
  // offering "restore" for either would promise a change that cannot happen.
  const hint = source === 'chosen'
    ? t('characterWindow.picture.chosenHint')
    : source === 'external'
      ? t('characterWindow.picture.externalHint')
      : source === 'lore'
        ? t('characterWindow.picture.loreHint')
        : source === 'shipped'
          ? t('characterWindow.picture.shippedHint')
          : source === 'story'
            ? t('characterWindow.picture.storyHint')
            : t('characterWindow.picture.noneHint')

  return (
    <PictureControlFrame
      testId="character-picture-control"
      name={name}
      state={state}
      hint={hint}
      canRestore={Boolean(state?.overridden)}
      resetKey={characterId}
      className={className}
      onImport={async (file) => {
        const result = await importCharacterPicture(storyId, characterId, file, { branchId: branchId || undefined })
        onStateChange(result.picture)
      }}
      onRestore={async () => {
        const result = await clearCharacterPicture(storyId, characterId, { branchId: branchId || undefined })
        onStateChange(result.picture)
      }}
    />
  )
}

/**
 * Reads one person's picture the first time the control is opened.
 *
 * It is a separate read from the window's own, because the window's cast answer
 * is what the game drew and this one has to survive an import that changed it.
 * Only the first open reads: a picture the author just imported is already the
 * state the caller holds.
 */
export function useCharacterPicture(storyId: string, branchId: string, characterId: string, enabled: boolean) {
  const [state, setState] = useState<CharacterPictureState | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const readSeq = useRef(0)

  useEffect(() => {
    if (!enabled || !storyId || !characterId) return
    const seq = ++readSeq.current
    setLoading(true)
    setError('')
    getCharacterPicture(storyId, characterId, { branchId: branchId || undefined })
      .then((next) => {
        if (seq !== readSeq.current) return
        setState(next)
        setLoading(false)
      })
      .catch((failure) => {
        if (seq !== readSeq.current) return
        setState(null)
        setError(errorText(failure))
        setLoading(false)
      })
    return () => { readSeq.current++ }
  }, [branchId, characterId, enabled, storyId])

  return { state, setState, loading, error }
}

/**
 * The reason a picture could not be imported, replaced, or restored.
 *
 * The server's own message is kept when it has one, because "no media directory
 * is available" is the answer the author needs and a generic sentence would hide
 * it.
 */
function errorText(failure: unknown): string {
  if (failure instanceof Error && failure.message.trim()) return failure.message
  return String(failure)
}
