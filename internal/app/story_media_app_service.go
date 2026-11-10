package app

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
)

type storyPicture struct {
	URL     string
	Caption string
}

// storyAssetAllowed checks provenance already committed to the branch. It
// never interprets a model's statement as a permission, nor infers a year or
// form from a source episode, canonical name, image, or starting candidate.
func storyAssetAllowed(asset media.StoryAsset, ctx interactive.StoryContext, state character.State, actorID string) bool {
	rule := asset.Rule
	if !asset.Reviewed || rule == nil || (rule.Kind != "observed_named_portrait" && rule.Kind != "observed_named_portrait_at_location") || rule.Form == "" || rule.KnownAs == "" || rule.Lifeform == "" || rule.Gender == "" || rule.Role == "" || rule.CanonicalName == "" {
		return false
	}
	// Yearless rules are opt-in and cannot weaken a calendar-specific rule.
	switch rule.TimeSemantics {
	case "", "explicit_year":
		if rule.Year <= 0 {
			return false
		}
	case "current_observation":
		if rule.Year != 0 {
			return false
		}
	default:
		return false
	}
	// Fail closed on unrecognized mapping semantics. Background classification
	// does not require the label as a current occupation or spoken role.
	// The committed observation must still match the actual current profile.
	switch rule.RoleSemantics {
	case "", "current_role", "background_classification":
	default:
		return false
	}
	if rule.RoleRecognition != "" && (rule.RoleSemantics == "background_classification" || strings.TrimSpace(rule.RoleRecognition) != rule.RoleRecognition || !strings.Contains(rule.Role, rule.RoleRecognition)) {
		return false
	}
	p, ok := state.Profile(actorID)
	if !ok || p.StoryID != ctx.Snapshot.StoryID || p.SourceKey != asset.SourceKey || !rule.AllowsKnownAs(p.KnownAs) || p.Lifeform != rule.Lifeform || p.Gender != rule.Gender || strings.TrimSpace(p.Role) == "" || (rule.RoleSemantics != "background_classification" && p.Role != rule.Role) || (rule.AdultIdentity && p.Age > 0 && p.Age < 18) {
		return false
	}
	catalogued := false
	if ctx.Meta.PackCatalog != nil {
		for _, person := range ctx.Meta.PackCatalog.Characters {
			if person.SourceKey == p.SourceKey && person.Name == p.Name && person.Name == rule.CanonicalName && !person.RecordOnly {
				catalogued = true
			}
		}
	}
	if !catalogued {
		return false
	}
	scene := interactive.StoryScene(ctx.Snapshot)
	// An observation matching the current scene is not enough when the author
	// restricted this image to a particular place. Never infer it from the image.
	if !rule.AllowsLocation(scene.Location) {
		return false
	}
	if rule.TimeSemantics == "current_observation" && strings.TrimSpace(scene.WorldTime) == "" {
		return false
	}
	// A rule without an authored year still preserves the scene's actual year.
	expectedYear := rule.Year
	if rule.TimeSemantics == "current_observation" {
		expectedYear = character.ExplicitVisualYear(scene.WorldTime)
	}
	appearance := interactive.StoryVisualAppearance(ctx.Snapshot.State, p.ID)
	if character.ExplicitVisualYear(scene.WorldTime) != expectedYear || appearance == "" || !character.VisualAppearanceSupports(appearance, rule.Form) || !character.ContainsID(scene.Present, p.ID) || !character.ContainsID(scene.Present, interactive.DefaultActorID) {
		return false
	}
	exchanges := map[string]character.Exchange{}
	for _, e := range state.Exchanges {
		exchanges[e.ID] = e
	}
	for _, k := range state.Knowledge {
		v := k.Visual
		if v == nil || !k.Live() || k.Nature != character.NatureExperienced || k.CharacterID != interactive.DefaultActorID || !character.HolderCanKnow(k, interactive.DefaultActorID) || k.FormalCommitID == "" || k.SourceEventID != k.FormalCommitID || k.SourceTurnID == "" || k.SourceExchangeID != v.ContactExchangeID {
			continue
		}
		if v.SubjectID != p.ID || v.StoryID != ctx.Snapshot.StoryID || v.SourceKey != p.SourceKey || v.KnownAs != p.KnownAs || !character.VisualRoleSnapshotMatches(*v, p) || v.Year != expectedYear || v.Form != rule.Form || v.Appearance != appearance || v.Location != scene.Location || v.Lifeform != p.Lifeform || v.Gender != p.Gender || v.Age != p.Age || character.ExplicitVisualYear(k.WorldTime) != expectedYear {
			continue
		}
		// No temporal generalization is allowed for a yearless observation.
		if rule.TimeSemantics == "current_observation" && k.WorldTime != scene.WorldTime {
			continue
		}
		contact, cok := exchanges[v.ContactExchangeID]
		recognition, rok := exchanges[v.RecognitionExchangeID]
		if !cok || !rok || contact.Speaker != character.SpeakerCharacter || contact.CharacterID != p.ID || contact.AddressedTo != interactive.DefaultActorID || !character.VisualIdentitySupports(recognition, p, interactive.DefaultActorID) {
			continue
		}
		// Only the utterance check uses the reviewed phrase. The full current role
		// and the committed observation snapshot have already matched above.
		recognitionProfile := p
		if rule.RoleRecognition != "" {
			recognitionProfile.Role = rule.RoleRecognition
		}
		if rule.RoleSemantics != "background_classification" && !character.VisualRecognitionSupports(recognition, recognitionProfile, interactive.DefaultActorID) {
			continue
		}
		valid := true
		for _, e := range []character.Exchange{contact, recognition} {
			if e.StoryID != v.StoryID || e.BranchID != v.BranchID || e.Channel != character.ChannelScene || e.TurnID != k.SourceTurnID || e.SourceEventID != k.FormalCommitID || e.WorldTime != k.WorldTime || !character.ContainsID(e.Audience, interactive.DefaultActorID) || !character.ContainsID(e.Audience, p.ID) {
				valid = false
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func scopedStoryMediaURL(ctx interactive.StoryContext, actorID, assetID string) string {
	q := url.Values{"branch": {ctx.Snapshot.BranchID}, "head": {interactive.StoryMediaHead(ctx.Snapshot)}, "revision": {strconv.FormatUint(ctx.Snapshot.ContextRevision, 10)}, "actor": {actorID}}
	return "/api/interactive/stories/" + url.PathEscape(ctx.Snapshot.StoryID) + "/media/" + url.PathEscape(assetID) + "?" + q.Encode()
}

// storyPictures uses the caller's exact snapshot generation. A concurrent
// commit yields no image rather than combining old scene with new knowledge.
func (a *App) storyPictures(ctx interactive.StoryContext) (map[string]bool, map[string]storyPicture) {
	managed := map[string]bool{}
	pictures := map[string]storyPicture{}
	if a.storyMedia == nil || ctx.Meta.PackBinding == nil {
		return managed, pictures
	}
	binding := ctx.Meta.PackBinding
	if ctx.Meta.PackCatalog != nil {
		for _, p := range ctx.Meta.PackCatalog.Characters {
			if len(a.storyMedia.Assets(binding.PackID, p.SourceKey)) > 0 {
				managed[p.SourceKey] = true
			}
		}
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return managed, pictures
	}
	current, state, err := store.StoryMediaSnapshot(ctx.Snapshot.StoryID, ctx.Snapshot.BranchID)
	if err != nil || current.Snapshot.ContextRevision != ctx.Snapshot.ContextRevision || interactive.StoryMediaHead(current.Snapshot) != interactive.StoryMediaHead(ctx.Snapshot) || interactive.StoryMediaHead(ctx.Snapshot) == "" || !a.storyMedia.PackAllowed(binding.PackID, binding.ContentHash) {
		return managed, pictures
	}
	for _, p := range state.Profiles {
		for _, asset := range a.storyMedia.Assets(binding.PackID, p.SourceKey) {
			managed[p.SourceKey] = true
			if storyAssetAllowed(asset, current, state, p.ID) {
				pictures[p.ID] = storyPicture{URL: scopedStoryMediaURL(current, p.ID, asset.ID), Caption: asset.Caption}
				break
			}
		}
	}
	if !a.storyMedia.Current() {
		return managed, map[string]storyPicture{}
	}
	return managed, pictures
}

// OpenStoryMedia is the only byte consumer. Metadata, GET, HEAD and Range all
// use this same gate. No error discloses whether the guessed asset exists.
func (a *App) OpenStoryMedia(storyID, branchID, head, revision, actorID, assetID string) (*os.File, media.StoryAsset, error) {
	deny := func() (*os.File, media.StoryAsset, error) { return nil, media.StoryAsset{}, os.ErrNotExist }
	if a == nil || a.storyMedia == nil || storyID == "" || branchID == "" || head == "" || actorID == "" {
		return deny()
	}
	rev, err := strconv.ParseUint(revision, 10, 64)
	if err != nil {
		return deny()
	}
	a.mu.RLock()
	store := a.interactive
	a.mu.RUnlock()
	if store == nil {
		return deny()
	}
	ctx, state, err := store.StoryMediaSnapshot(storyID, branchID)
	if err != nil || ctx.Snapshot.StoryID != storyID || ctx.Snapshot.BranchID != branchID || ctx.Snapshot.ContextRevision != rev || interactive.StoryMediaHead(ctx.Snapshot) != head || ctx.Meta.PackBinding == nil {
		return deny()
	}
	asset, ok := a.storyMedia.Asset(assetID)
	binding := ctx.Meta.PackBinding
	if !ok || !a.storyMedia.MatchesWorld(asset, binding.PackID) || !a.storyMedia.PackAllowed(binding.PackID, binding.ContentHash) || !storyAssetAllowed(asset, ctx, state, actorID) {
		return deny()
	}
	file, err := a.storyMedia.Open(asset)
	if err != nil {
		return deny()
	}
	// The hash read may take time. Refuse if the head, side-fact revision, or
	// character projection changed before handing the open handle to the stream.
	latest, latestState, err := store.StoryMediaSnapshot(storyID, branchID)
	if err != nil || latest.Snapshot.ContextRevision != rev || interactive.StoryMediaHead(latest.Snapshot) != head || latestState.Revision != state.Revision || latestState.Cursor != state.Cursor || !a.storyMedia.Current() {
		file.Close()
		return deny()
	}
	return file, asset, nil
}
