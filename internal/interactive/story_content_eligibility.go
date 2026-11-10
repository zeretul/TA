package interactive

import (
	"strings"

	"ta/internal/exploration"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

// This file is the one place that decides whether a piece of authored content
// belongs to the story being played, and where the character really stands.
//
// The review of F6—F9 found the same defect in four shapes: a realm, a task and
// a public event were all offered from the shared authored set without asking
// whether this story's world pack contains them or whether the character is
// anywhere near them, and the submit paths repeated (or skipped) those checks
// independently of what the entry showed. The fix is not four local patches but
// one predicate per question, called by both the read and the commit:
//
//   - Where is the character? The committed scene text is free prose, so the
//     only honest answer is the registered place it names. Substring matching on
//     an event's own region string would call any sentence mentioning 坊市 a
//     match; resolving the text to a place identifier first makes the question
//     answerable and makes it the same question everywhere.
//
//   - Does this story's pack contain it? A story bound to a world pack is
//     playing that world, so content authored for another pack or another
//     revision of it is not part of this world. A story with no binding has no
//     world to contradict, so it is left unrestricted.
//
//   - Was the giver authored here? §4.10 of 自制-事件与任务 wants a formal
//     source key bound to this story's own cast rather than a name match, so a
//     task naming a giver is offered only when that key is in the story's own
//     frozen catalogue.

// storyLocationID resolves the branch's committed scene text to one registered
// place identifier.
//
// The second result is false when the text names no registered place, or when two
// places are named equally specifically. The most specific wording wins: a scene
// that says "回到越国坊市外围散修人家" names the settlement, not the market it
// mentions in passing, and a rule built on the answer would be wrong if the
// shorter wording won. A genuine tie is refused rather than guessed, because
// every rule built on this answer — an event's region, a realm's entrance, a
// route's origin — would inherit the guess.
func storyLocationID(state map[string]any, worlds ...*WorldContent) (string, bool) {
	return registeredPlaceInText(storySceneText(state, storySceneLocationField), worlds...)
}

// registeredPlaceInText resolves one scene sentence to the registered place it
// names. It is the same answer storyLocationID gives, for callers that hold a
// proposed value rather than committed state — the state compiler, which has to
// tell a local step from a move the world has not settled.
func registeredPlaceInText(scene string, worlds ...*WorldContent) (string, bool) {
	labels := journeyPlaceLabels
	if len(worlds) > 0 && worlds[0] != nil {
		labels = worlds[0].Labels
	}
	current := strings.TrimSpace(scene)
	if current == "" {
		return "", false
	}
	if id, ok := matchRegisteredPlace(current, journeyPlaceCandidates(worlds...)); ok {
		return id, true
	}
	// The prose normally uses the place's wording, but a story may carry the
	// identifier itself — an older journal, or a scene a model copied from a
	// reference. Identifiers are matched only when no wording matched, so a
	// label always wins over a coincidental substring of an id.
	candidates := make([]placeCandidate, 0, len(labels))
	for id := range labels {
		candidates = append(candidates, placeCandidate{ID: id, Text: id})
	}
	return matchRegisteredPlace(current, candidates)
}

// placeCandidate is one authored wording for one registered place.
type placeCandidate struct {
	ID   string
	Text string
}

// journeyPlaceCandidates lists every wording a registered place is named by: its
// label and the aliases prose really uses.
func journeyPlaceCandidates(worlds ...*WorldContent) []placeCandidate {
	labels, aliases := journeyPlaceLabels, journeyPlaceAliases
	if len(worlds) > 0 && worlds[0] != nil {
		labels, aliases = worlds[0].Labels, worlds[0].Aliases
	}
	candidates := make([]placeCandidate, 0, len(labels))
	for id, label := range labels {
		if label != "" {
			candidates = append(candidates, placeCandidate{ID: id, Text: label})
		}
		for _, alias := range aliases[id] {
			if alias != "" && alias != label {
				candidates = append(candidates, placeCandidate{ID: id, Text: alias})
			}
		}
	}
	return candidates
}

// matchRegisteredPlace returns the place whose wording the scene names most
// specifically, refusing a tie.
func matchRegisteredPlace(scene string, places []placeCandidate) (string, bool) {
	best, bestLen := "", 0
	tied := false
	for _, place := range places {
		if place.Text == "" || !strings.Contains(scene, place.Text) {
			continue
		}
		switch {
		case len(place.Text) > bestLen:
			best, bestLen, tied = place.ID, len(place.Text), false
		case len(place.Text) == bestLen && place.ID != best:
			tied = true
		}
	}
	if best == "" || tied {
		return "", false
	}
	return best, true
}

// storyLocationIsAny reports whether the character stands at one of the named
// registered places.
func storyLocationIsAny(state map[string]any, locationIDs []string, worlds ...*WorldContent) bool {
	if len(locationIDs) == 0 {
		return false
	}
	current, ok := storyLocationID(state, worlds...)
	if !ok {
		return false
	}
	for _, id := range locationIDs {
		if id == current {
			return true
		}
	}
	return false
}

// storyPackAllows uses the frozen package owner, not a program-maintained
// list of approved world names or content revision numbers.
func storyPackAllows(meta StoryMeta, packageID, packageVersion string) bool {
	if meta.PackBinding == nil {
		return true
	}
	c := worldContentForMeta(meta)
	return meta.ActorStateSchema != nil && meta.ActorStateSchema.System.WorldContent != nil && c.PackID == meta.PackBinding.PackID && (packageID == "" || packageID == c.PackID)
}

// storyGiverAuthored reports whether the task's giver is part of this story's own
// cast.
//
// A task with no giver is an ordinary errand and passes. A task that names one
// passes only when the story's frozen catalogue holds that source key. A bound
// story whose catalogue is missing cannot confirm the giver, so it refuses
// rather than offering a task from a cast it cannot read.
func storyGiverAuthored(meta StoryMeta, giver string) bool {
	if giver == "" {
		return true
	}
	if meta.PackBinding == nil && meta.PackCatalog == nil {
		return true
	}
	if meta.PackCatalog == nil {
		return false
	}
	for _, person := range meta.PackCatalog.Characters {
		if person.SourceKey == giver {
			return true
		}
	}
	return false
}

// questDefinitionEligible is the single answer to "may this task be offered
// here", used by both the read and the commit.
func questDefinitionEligible(meta StoryMeta, definition quest.Definition) bool {
	if !storyPackAllows(meta, definition.PackageID, definition.PackageVersion) {
		return false
	}
	return storyGiverAuthored(meta, definition.Giver)
}

// publicEventDefinitionEligible is the single answer to "is this event this
// character's to know about", used by both the read and the commit.
//
// The place is part of the answer, because §6 makes hearing about an event a
// fact about standing somewhere rather than a fact about the event existing.
func publicEventDefinitionEligible(meta StoryMeta, definition publiquevent.Definition, state map[string]any) bool {
	if !storyPackAllows(meta, definition.PackageID, definition.PackageVersion) {
		return false
	}
	return storyLocationIsAny(state, definition.RegionLocationIDs, worldContentForMeta(meta))
}

// realmDefinitionEligible is the single answer to "can this realm be entered
// from here", used by both the read and the commit.
func realmDefinitionEligible(meta StoryMeta, definition exploration.DungeonDefinition, state map[string]any) bool {
	if !storyPackAllows(meta, definition.PackageID, definition.PackageVersion) {
		return false
	}
	return spiritRealmEligible(meta, definition, state) && storyLocationIsAny(state, definition.EntryLocationIDs, worldContentForMeta(meta))
}
