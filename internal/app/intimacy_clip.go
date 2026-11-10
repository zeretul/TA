package app

import (
	"io/fs"
	"net/url"
	"os"
	"strings"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/media"
)

// IntimacyClipRequest identifies a clip and the activity participant allowed to
// see it. All binding facts are read from the current journal, not the URL.
type IntimacyClipRequest struct {
	StoryID, BranchID, ActivityID, ActorID, File string
}

func activityClipQuery(story interactive.StoryContext, state character.State, activity character.IntimacyActivity, actorID string, ages map[string]int) (media.PictureQuery, bool) {
	if activity.Phase != character.IntimacyActive || activity.StoryID != story.Meta.StoryID || story.Meta.PackBinding == nil {
		return media.PictureQuery{}, false
	}
	group, ok := state.Group(activity.GroupID)
	if !ok || !character.ContainsID(group.ActiveMembers(), actorID) || !character.ContainsID(activity.ConsentedRoster(), actorID) {
		return media.PictureQuery{}, false
	}
	profile, ok := state.Profile(actorID)
	if !ok || !intimacyProfileIsAdult(profile, ages) {
		return media.PictureQuery{}, false
	}
	return media.PictureQuery{
		World: strings.TrimSpace(story.Meta.PackBinding.PackID), SourceKey: profile.SourceKey,
		Scope: profile.StoryID, Actor: profile.ID, SceneKey: sceneSeed(activity.Scene), Role: media.RoleIntimate,
	}, true
}

func (a *App) intimacyActivityClips(story interactive.StoryContext, state character.State, activity character.IntimacyActivity, dataDir string) map[string][]MediaClip {
	if activity.Phase != character.IntimacyActive {
		return nil
	}
	ages := intimacyAdultAges(story, dataDir)
	clips := map[string][]MediaClip{}
	for _, actorID := range activity.ConsentedRoster() {
		query, ok := activityClipQuery(story, state, activity, actorID, ages)
		if !ok {
			continue
		}
		assets := a.Media().ClipsFor(query)
		chosen, ok := media.Pick(assets, activity.ID+"/"+actorID)
		if !ok {
			continue
		}
		params := url.Values{"branch": {story.Snapshot.BranchID}, "activity_id": {activity.ID}, "actor": {actorID}}
		prefix := "/api/interactive/stories/" + url.PathEscape(story.Meta.StoryID) + "/intimacy/media/"
		add := func(asset media.Asset) {
			clip := mediaClipView(asset)
			clip.URL = prefix + url.PathEscape(asset.File) + "?" + params.Encode()
			clips[actorID] = append(clips[actorID], clip)
		}
		add(chosen)
		for _, asset := range assets {
			if asset.File != chosen.File {
				add(asset)
			}
		}
	}
	return clips
}

// OpenIntimacyClip rechecks the branch, active group, consent and adult facts
// for every GET, HEAD or Range request, including requests from old player URLs.
func (a *App) OpenIntimacyClip(request IntimacyClipRequest) (*os.File, media.Asset, error) {
	if a == nil || request.StoryID == "" || request.BranchID == "" || request.ActivityID == "" || request.ActorID == "" {
		return nil, media.Asset{}, fs.ErrNotExist
	}
	a.mu.RLock()
	store, cfg := a.interactive, a.cfg
	a.mu.RUnlock()
	if store == nil || cfg == nil {
		return nil, media.Asset{}, fs.ErrNotExist
	}
	story, state, err := store.StoryMediaSnapshot(request.StoryID, request.BranchID)
	if err != nil || story.Snapshot.BranchID != request.BranchID {
		return nil, media.Asset{}, fs.ErrNotExist
	}
	activity, ok := state.IntimacyActivityFor(request.ActivityID)
	if !ok {
		return nil, media.Asset{}, fs.ErrNotExist
	}
	query, ok := activityClipQuery(story, state, activity, request.ActorID, intimacyAdultAges(story, cfg.DataDir()))
	if !ok {
		return nil, media.Asset{}, fs.ErrNotExist
	}
	return a.Media().OpenActivityClip(query, request.File)
}
