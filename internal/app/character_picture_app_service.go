package app

import (
	"errors"
	"fmt"
	"strings"

	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
)

// ErrStoryPictureManaged prevents ignored writes to story-managed portraits.
var ErrStoryPictureManaged = errors.New("this portrait is managed by story rules and cannot be replaced or reset")

// CharacterPictureImport is one picture the player picked for one person, or one
// they picked to clear.
//
// The request names a story and a person, never a file path and never a binding.
// The server works out the binding from what the story already committed: an
// author key when the person has one, and the story plus actor ID when the story
// invented them. That is the whole point of putting this in the app layer — a
// page that could name its own binding could also name another story's actor or
// another world's person, and the picture would be drawn for the wrong face.
type CharacterPictureImport struct {
	StoryID     string
	BranchID    string
	CharacterID string
	// Purpose is 'avatar' or 'portrait'. Empty means avatar, which is what the
	// list row draws and what the open portrait falls back to.
	Purpose string
	// Filename is the name the file arrived under. Only its extension is read,
	// and only after the bytes have been decoded.
	Filename string
	Data     []byte
}

// CharacterPictureImportResult is what the page needs to draw the new picture
// without a second read: the entry it now holds and the URL to fetch it from.
type CharacterPictureImportResult struct {
	CharacterID string       `json:"character_id"`
	Asset       media.Asset  `json:"asset"`
	URL         string       `json:"url"`
	Picture     PictureState `json:"picture"`
}

// CharacterPictureState is one person's picture as the window would draw it now.
//
// It is deliberately the same question the window asks, answered once so the
// author's own preview cannot disagree with the game. `Source` says which layer
// answered, which is what lets the editor offer "restore the default" only when
// there is an override to remove.
type PictureState struct {
	Caption string `json:"caption,omitempty"`
	// URL is what the page draws. Empty means nothing is drawn: the person has no
	// picture of any kind, and the placeholder initial is the honest answer.
	URL string `json:"url,omitempty"`
	// Source is 'story', 'chosen', 'external', 'lore', 'shipped' or ''.
	Source string `json:"source"`
	// Overridden says whether this person has an external picture the player can
	// remove. A chosen avatar is not an override: it is a different field.
	Overridden bool `json:"overridden"`
}

// The layers a picture can come from, in the order the window tries them.
const (
	PictureSourceChosen   = "chosen"
	PictureSourceExternal = "external"
	PictureSourceLore     = "lore"
	PictureSourceShipped  = "shipped"
)

// CharacterPictureImport copies one uploaded picture into the media directory and
// binds it to the person the story already has.
//
// The file is stored by content hash, so importing the same picture twice keeps
// one file and replacing a picture produces a new name — which is why a replaced
// picture cannot keep being served from a cache.
func (a *App) CharacterPictureImport(request CharacterPictureImport) (CharacterPictureImportResult, error) {
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return CharacterPictureImportResult{}, fmt.Errorf("a picture belongs to a story")
	}
	library := a.Media()
	if library == nil {
		return CharacterPictureImportResult{}, media.ErrNoMediaDir
	}
	purpose := strings.TrimSpace(request.Purpose)
	if purpose == "" {
		purpose = media.PurposeAvatar
	}
	if purpose != media.PurposeAvatar && purpose != media.PurposePortrait {
		return CharacterPictureImportResult{}, fmt.Errorf("a person's picture is an avatar or a portrait")
	}
	target, sources, err := a.pictureTarget(storyID, request.BranchID, request.CharacterID)
	if err != nil {
		return CharacterPictureImportResult{}, err
	}
	// Ownership is independent of visibility: locked and visible story media
	// both ignore external overrides. Reject before any Import or Remove write.
	if sources.storyManaged[target.SourceKey] {
		return CharacterPictureImportResult{}, ErrStoryPictureManaged
	}
	asset, err := library.Import(media.ImportRequest{
		World:     sources.world,
		Subject:   media.SubjectCharacter,
		SourceKey: strings.TrimSpace(target.SourceKey),
		Scope:     strings.TrimSpace(target.StoryID),
		Actor:     strings.TrimSpace(target.ID),
		Purpose:   purpose,
		Filename:  request.Filename,
		Data:      request.Data,
	})
	if err != nil {
		return CharacterPictureImportResult{}, err
	}
	return CharacterPictureImportResult{
		CharacterID: target.ID,
		Asset:       asset,
		URL:         media.URL(asset),
		Picture:     pictureState(target, sources),
	}, nil
}

// CharacterPictureClear drops this person's external picture and reports what
// answers instead.
//
// It is one operation rather than "delete the file, then rebuild the window":
// removing the entry is exactly what makes the next layer answer, so the caller
// gets the restored picture in the same answer.
func (a *App) CharacterPictureClear(request CharacterPictureImport) (CharacterPictureImportResult, error) {
	storyID := strings.TrimSpace(request.StoryID)
	if storyID == "" {
		return CharacterPictureImportResult{}, fmt.Errorf("a picture belongs to a story")
	}
	library := a.Media()
	if library == nil {
		return CharacterPictureImportResult{}, media.ErrNoMediaDir
	}
	purpose := strings.TrimSpace(request.Purpose)
	if purpose == "" {
		purpose = media.PurposeAvatar
	}
	if purpose != media.PurposeAvatar && purpose != media.PurposePortrait {
		return CharacterPictureImportResult{}, fmt.Errorf("a person's picture is an avatar or a portrait")
	}
	target, sources, err := a.pictureTarget(storyID, request.BranchID, request.CharacterID)
	if err != nil {
		return CharacterPictureImportResult{}, err
	}
	// Ownership is independent of visibility: locked and visible story media
	// both ignore external overrides. Reject before any Import or Remove write.
	if sources.storyManaged[target.SourceKey] {
		return CharacterPictureImportResult{}, ErrStoryPictureManaged
	}
	asset, ok := library.Picture(media.PictureQuery{
		World: sources.world, SourceKey: strings.TrimSpace(target.SourceKey),
		Scope: strings.TrimSpace(target.StoryID), Actor: strings.TrimSpace(target.ID),
		Purpose: purpose,
	})
	if !ok {
		// Nothing external answers, so there is nothing to clear. Reporting the
		// picture that already stands is more useful than an error: the editor can
		// leave its button disabled and the player is not told they broke something.
		return CharacterPictureImportResult{
			CharacterID: target.ID,
			Picture:     pictureState(target, sources),
		}, nil
	}
	removed, err := library.Remove(asset.ID)
	if err != nil {
		return CharacterPictureImportResult{}, err
	}
	return CharacterPictureImportResult{
		CharacterID: target.ID,
		Asset:       removed,
		Picture:     pictureState(target, sources),
	}, nil
}

// CharacterPicture reports one person's picture as the window would draw it.
func (a *App) CharacterPicture(storyID, branchID, characterID string) (PictureState, error) {
	target, sources, err := a.pictureTarget(storyID, branchID, characterID)
	if err != nil {
		return PictureState{}, err
	}
	return pictureState(target, sources), nil
}

// pictureTarget resolves a person inside one story, and the sources their picture
// may come from.
//
// The person is looked up in this story's own cast, not by ID alone: a profile
// ID that belongs to another story, or that the story has not introduced, is not
// a target. That is what keeps an import from binding a face to somebody the
// player has never met.
func (a *App) pictureTarget(storyID, branchID, characterID string) (character.Profile, pictureSources, error) {
	if a == nil {
		return character.Profile{}, pictureSources{}, ErrNoWorkspace
	}
	personID := strings.TrimSpace(characterID)
	if personID == "" {
		return character.Profile{}, pictureSources{}, fmt.Errorf("a picture belongs to a person")
	}
	a.mu.RLock()
	store := a.interactive
	projectID := ""
	if a.cfg != nil {
		projectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if store == nil {
		return character.Profile{}, pictureSources{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, strings.TrimSpace(branchID))
	if err != nil {
		return character.Profile{}, pictureSources{}, err
	}
	resolvedBranch := strings.TrimSpace(storyContext.Snapshot.BranchID)
	state, err := store.CharacterState(storyID, resolvedBranch)
	if err != nil {
		return character.Profile{}, pictureSources{}, err
	}
	profile, ok := state.Profile(personID)
	if !ok {
		return character.Profile{}, pictureSources{}, fmt.Errorf("this story has no such person")
	}
	scene := interactive.StoryScene(storyContext.Snapshot)
	return profile, a.characterPictures(storyID, projectID, storyContext, scene), nil
}

// pictureState answers the window's own question about one person.
//
// It is written as the same chain the window uses rather than a second copy of
// the rules: a preview that could disagree with the game would be worse than no
// preview at all.
func pictureState(profile character.Profile, sources pictureSources) PictureState {
	if sources.storyManaged[profile.SourceKey] {
		picture := sources.storyPictures[profile.ID]
		return PictureState{URL: picture.URL, Caption: picture.Caption, Source: "story"}
	}
	if strings.TrimSpace(profile.Avatar) != "" {
		return PictureState{URL: strings.TrimSpace(profile.Avatar), Source: PictureSourceChosen}
	}
	external := sources.external(profile, media.PurposePortrait)
	if external == "" {
		external = sources.external(profile, media.PurposeAvatar)
	}
	if external != "" {
		return PictureState{URL: external, Source: PictureSourceExternal, Overridden: true}
	}
	// The world book's picture is a project-relative path, so it cannot be a URL
	// here. The editor resolves it through the project it is already reading, the
	// same way the roster does; reporting the path keeps that decision in one place.
	if path := sources.lore(profile); path != "" {
		return PictureState{URL: path, Source: PictureSourceLore}
	}
	if sources.portraits != nil {
		if url := sources.portraits.WorldURL(sources.world, profile.SourceKey, true); url != "" {
			return PictureState{URL: url, Source: PictureSourceShipped}
		}
	}
	return PictureState{Source: ""}
}
