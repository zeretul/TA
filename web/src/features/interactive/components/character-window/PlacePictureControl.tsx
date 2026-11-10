import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { clearPlacePicture, getPlacePicture, importPlacePicture } from '../../api'
import type { PlacePictureState } from '../../types'
import { PictureControlFrame } from './CharacterPictureControl'

/**
 * 素材：给一个地点换一张图。
 *
 * A place is not something a story invents, so the control only appears for a
 * place the story's own frozen catalog registered — the server refuses any other
 * name. That is why the window hands it the scene's location rather than asking
 * the author to type one: the place on screen is the place being illustrated.
 */
export function PlacePictureControl({
  storyId,
  place,
  state,
  onStateChange,
  className,
}: {
  storyId: string
  /** The place's name as the scene spells it, or '' when the story stands nowhere. */
  place: string
  state: PlacePictureState | null
  onStateChange: (next: PlacePictureState) => void
  className?: string
}) {
  const { t } = useTranslation()
  return (
    <PictureControlFrame
      testId="place-picture-control"
      name={place}
      state={state}
      hint={state?.url ? t('characterWindow.picture.placeHint') : t('characterWindow.picture.placeNoneHint')}
      canRestore={Boolean(state?.overridden)}
      resetKey={place}
      className={className}
      onImport={async (file) => {
        const result = await importPlacePicture(storyId, place, file)
        onStateChange(result.picture)
      }}
      onRestore={async () => {
        const result = await clearPlacePicture(storyId, place)
        onStateChange(result.picture)
      }}
    />
  )
}

/**
 * Reads the place picture for the place the story already stands in.
 *
 * It reads only once per place: an import the author just made is already the
 * state the caller holds, and a second read would only re-ask the same question.
 */
export function usePlacePicture(storyId: string, place: string, enabled: boolean) {
  const [state, setState] = useState<PlacePictureState | null>(null)
  const readSeq = useRef(0)

  useEffect(() => {
    if (!enabled || !storyId || !place) return
    const seq = ++readSeq.current
    getPlacePicture(storyId, place)
      .then((next) => {
        if (seq !== readSeq.current) return
        setState(next)
      })
      .catch(() => {
        // A place the story never registered is not something the window can
        // illustrate, and the scene still draws its text. Failing quietly here
        // keeps the picture from becoming a reason the window will not open.
        if (seq !== readSeq.current) return
        setState(null)
      })
    return () => { readSeq.current++ }
  }, [enabled, place, storyId])

  return { state, setState }
}
