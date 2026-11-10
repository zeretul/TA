package interactive

import (
	"fmt"
	"strings"

	"ta/internal/character"
)

// The story's own scene lives in the story-context Actor's frozen schema. These
// are the stable field identities the built-in presets and the generated core
// both use, so one scene read works for a pack-authored story and a fully
// generated one without the caller knowing which it is.
const (
	storySceneTimeField     = "当前时间"
	storySceneLocationField = "当前详细地点"
	storySceneEventField    = "当前事件"
	storyScenePresentField  = "在场角色"
	// storySceneEngagementField is this scene's declaration of who is fighting
	// the protagonist. It is keyed by the attacker's own Actor ID so the fight
	// uses the identity the story already has.
	storySceneEngagementField = "本次交战"
	// combatLethalIntentsField is the key inside an engagement object that holds
	// the declared intents. The field contract permits the list under an actor's
	// own key or once at the top level beside the per-actor basis strings, so the
	// reader has to know this one name for both shapes.
	combatLethalIntentsField = "lethal_intents"
)

// StoryScene reads the current story state as the scene a conversation happens
// in. It is the projection the character window, incoming messages and
// main-story character speech share, so all three agree about where the
// protagonist is, what time it is and who is there.
//
// The scene comes from committed state rather than from prose, so it cannot
// drift: whatever the last accepted turn wrote is what the window reads.
func StoryScene(snapshot Snapshot) character.Scene {
	return StorySceneFromState(snapshot.State, storySceneNodeID(snapshot))
}

// StorySceneFromState is the same read against a bare state map, for callers
// that hold committed state without a snapshot — the turn commit path, which
// must resolve the scene before the turn it is publishing is readable.
func StorySceneFromState(state map[string]any, nodeID string) character.Scene {
	values := storyContextActorState(state)
	return character.Scene{
		NodeID:    strings.TrimSpace(nodeID),
		Title:     compactText(storySceneFieldText(values, storySceneEventField), 40),
		Location:  storySceneFieldText(values, storySceneLocationField),
		WorldTime: storySceneFieldText(values, storySceneTimeField),
		TurnID:    strings.TrimSpace(nodeID),
		Present:   storySceneFieldList(values, storyScenePresentField),
	}
}

// storySceneNodeID is the scene node the player is currently reading: the
// latest committed turn. A window exchange attaches here, which is what makes
// "answered the guard in the window" the same scene the story resumes from.
func storySceneNodeID(snapshot Snapshot) string {
	if snapshot.CurrentTurn != nil {
		if id := strings.TrimSpace(snapshot.CurrentTurn.ID); id != "" {
			return id
		}
	}
	return strings.TrimSpace(snapshot.BranchID)
}

func storyContextActorState(state map[string]any) map[string]any {
	actors, _ := state[actorStateRoot].(map[string]any)
	actor, _ := actors[normalizeStatePanelActorID(DefaultStoryContextActorID)].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	return values
}

// storySceneFieldText reads one story-context field by its stable field
// identity. A missing or non-text value reads as empty rather than as an error:
// a story whose schema is not initialized yet simply has no scene to report,
// and the caller falls back to the branch head.
func storySceneFieldText(state map[string]any, fieldID string) string {
	value, ok := storySceneFieldValue(state, fieldID)
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// storySceneText reads one story-context field out of a whole state root.
//
// storySceneFieldText takes the story-context actor's own values map. Callers
// that hold committed state rather than that map go through here, so the field
// is never looked for at the wrong level: reading the root directly finds
// nothing for a current-schema story and silently reports an empty scene.
func storySceneText(state map[string]any, fieldID string) string {
	return storySceneFieldText(storyContextActorState(state), fieldID)
}

func storySceneFieldList(state map[string]any, fieldID string) []string {
	value, ok := storySceneFieldValue(state, fieldID)
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			text = fmt.Sprint(item)
		}
		if text = strings.TrimSpace(text); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// storySceneFieldValue resolves a story-context field through the frozen field
// identity, then through the pre-schema state path. The second attempt matters
// for a story whose schema was frozen before the field moved into the Actor:
// its value is still nested at the old location until the snapshot aliases it.
func storySceneFieldValue(state map[string]any, fieldID string) (any, bool) {
	if state == nil {
		return nil, false
	}
	if value, ok := state[normalizeActorStateFieldName(fieldID)]; ok && value != nil {
		return value, true
	}
	if legacy := legacyStoryScenePath(fieldID); legacy != "" {
		if value := getPathExact(state, legacy); value != nil {
			return value, true
		}
	}
	return nil, false
}

func legacyStoryScenePath(fieldID string) string {
	switch normalizeActorStateFieldName(fieldID) {
	case normalizeActorStateFieldName(storySceneTimeField):
		return "scene.current_time"
	case normalizeActorStateFieldName(storySceneLocationField):
		return "scene.location"
	case normalizeActorStateFieldName(storyScenePresentField):
		return "scene.present_actors"
	default:
		return ""
	}
}

// storyContextEngagementRecords reads this scene's declared engagements as the
// raw key/value pairs the model wrote. It takes the story-context actor's own
// values map, like storySceneFieldList.
//
// The value is the traceable basis and is not interpreted here: what matters to
// the encounter is which Actor IDs are declared, and the basis is kept in the
// story record for a reader who asks why. A value that is not an object reads as
// no declaration at all, so a story whose field holds something unexpected opens
// no fight rather than an invented one.
func storyContextEngagementRecords(values map[string]any) map[string]any {
	value, ok := storySceneFieldValue(values, storySceneEngagementField)
	if !ok {
		return nil
	}
	records, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return records
}
