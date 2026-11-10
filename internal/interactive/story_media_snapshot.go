package interactive

import (
	"strings"
	"ta/internal/character"
)

// StoryMediaSnapshot returns the scene and character provenance from one
// in-memory journal generation under the store mutex. It never merges two
// independently refreshed reads. A later commit invalidates its media URL.
func (s *Store) StoryMediaSnapshot(storyID, branchID string) (StoryContext, character.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return StoryContext{}, character.State{}, err
	}
	projection, err := s.storyBranchProjectionLocked(storyID, snapshot.BranchID)
	if err != nil {
		return StoryContext{}, character.State{}, err
	}
	state := character.State{Version: character.StateVersion}
	if projection.Characters != nil {
		state = projection.Characters.Normalize()
	}
	return StoryContext{Meta: meta, Snapshot: snapshot}, state, nil
}

func visualObservationContext(state character.State, values map[string]any) *character.VisualObservationContext {
	out := &character.VisualObservationContext{ObserverID: DefaultActorID, Subjects: map[string]character.VisualSubject{}}
	for _, p := range state.Profiles {
		out.Subjects[p.ID] = character.VisualSubject{Profile: p, Appearance: StoryVisualAppearance(values, p.ID)}
	}
	return out
}

// StoryVisualAppearance reads only the actual actor state. A catalogue's
// biography, a portrait, and a candidate starting place are not observations.
func StoryVisualAppearance(state map[string]any, actorID string) string {
	actors, _ := state["actors"].(map[string]any)
	actor, _ := actors[actorID].(map[string]any)
	values, _ := actor["state"].(map[string]any)
	text, _ := values["外貌与装扮"].(string)
	return strings.TrimSpace(text)
}

func StoryMediaHead(snapshot Snapshot) string {
	for _, b := range snapshot.Graph.Branches {
		if b.ID == snapshot.BranchID {
			return b.Head
		}
	}
	return ""
}
