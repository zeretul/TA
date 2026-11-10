import { CharacterPictureControl, useCharacterPicture } from '../character-window/CharacterPictureControl'
import { PlacePictureControl, usePlacePicture } from '../character-window/PlacePictureControl'
import type { LoreEntry } from '../lore-overview/model'

/**
 * 素材：本局新建页给一条资料换图。
 *
 * 本局新建 lists what this story registered, and a person or a place it
 * registered can be illustrated the same way the character window illustrates
 * them. The two controls are reused rather than reimplemented: they already bind
 * a file to an identity the server works out from the story itself, and a second
 * copy here would be a second binding path that could disagree with the first.
 *
 * Only people and places get one. A faction and a holding have no picture chain
 * in the game, so offering a button would promise something no window draws.
 */
export function LocalEntryPicture({
  storyId,
  branchId,
  entry,
}: {
  storyId: string
  branchId?: string
  entry: LoreEntry
}) {
  if (entry.tab === 'people') {
    return <LocalCharacterPicture storyId={storyId} branchId={branchId} characterId={entry.id} name={entry.title} />
  }
  if (entry.tab === 'places') {
    return <LocalPlacePicture storyId={storyId} place={entry.title} />
  }
  return null
}

function LocalCharacterPicture({ storyId, branchId, characterId, name }: {
  storyId: string
  branchId?: string
  characterId: string
  name: string
}) {
  const picture = useCharacterPicture(storyId, branchId || '', characterId, true)
  return (
    <CharacterPictureControl
      storyId={storyId}
      branchId={branchId || ''}
      characterId={characterId}
      name={name}
      state={picture.state}
      onStateChange={picture.setState}
    />
  )
}

function LocalPlacePicture({ storyId, place }: { storyId: string; place: string }) {
  const picture = usePlacePicture(storyId, place, true)
  return <PlacePictureControl storyId={storyId} place={place} state={picture.state} onStateChange={picture.setState} />
}
