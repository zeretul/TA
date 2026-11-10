package interactive

import (
	"sort"

	interactivestate "ta/internal/interactive/state"
)

// rebindTurnStateUpdatesToEstablishedActors keeps a turn's numbers on the same
// person its prose lands on.
//
// The model mints an id from the spelling it used this turn. When that person is
// already established under another id — matched by the author's source key, not
// by name — a create under the minted id would stage a second Actor, and the
// character record would then register the line against the first one. The
// player would see one person in the panel and a different one taking damage in
// the battle window.
//
// The rewrite is deliberately conservative:
//   - A create for an id that is already established becomes replace updates for
//     the fields it named, so the numbers land on the existing Actor and the
//     compiler still checks every field against that Actor's own template.
//   - A create for an id whose profile exists but whose Actor does not (the
//     person was registered from character_facts alone) is retargeted in place,
//     so they finally get an Actor without a second identity.
//   - Every other update under a rebound id is retargeted.
//
// Nothing here invents an actor: an id with no alias passes through unchanged,
// and a create with no alias is left exactly as submitted.
func rebindTurnStateUpdatesToEstablishedActors(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update, aliases map[string]string) []interactivestate.Update {
	if len(updates) == 0 || len(aliases) == 0 {
		return updates
	}
	system = normalizeActorStateSystem(system)
	out := make([]interactivestate.Update, 0, len(updates))
	for _, update := range updates {
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) == 0 {
			out = append(out, update)
			continue
		}
		establishedID, ok := aliases[normalizeStatePanelActorID(segments[0])]
		if !ok || establishedID == "" {
			out = append(out, update)
			continue
		}
		if update.Op != interactivestate.Create {
			out = append(out, interactivestate.Update{
				Op: update.Op, Value: update.Value,
				Path: interactivestate.FormatPath(append([]string{establishedID}, segments[1:]...)),
			})
			continue
		}
		actorExists := getPath(currentState, actorStateRoot+"."+establishedID) != nil
		if !actorExists {
			out = append(out, retargetActorCreate(update, establishedID))
			continue
		}
		// The person already has an Actor. The create is redundant, but the values
		// it carried are this turn's actual writes, so they become replace updates
		// against the established Actor and are checked against its template.
		out = append(out, createAsReplaceUpdates(system, currentState, establishedID, update)...)
	}
	return out
}

// retargetActorCreate moves a create onto an established id. The name is the
// established person's own: a later turn's spelling of them is not a rename.
func retargetActorCreate(update interactivestate.Update, establishedID string) interactivestate.Update {
	value, ok := update.Value.(map[string]any)
	if !ok {
		return interactivestate.Update{Op: update.Op, Path: interactivestate.FormatPath([]string{establishedID}), Value: update.Value}
	}
	retargeted := make(map[string]any, len(value))
	for key, item := range value {
		retargeted[key] = item
	}
	retargeted["name"] = establishedID
	return interactivestate.Update{Op: interactivestate.Create, Path: interactivestate.FormatPath([]string{establishedID}), Value: retargeted}
}

// createAsReplaceUpdates converts a redundant create into the field writes it
// actually carried.
func createAsReplaceUpdates(system StoryDirectorActorStateSystem, currentState map[string]any, establishedID string, update interactivestate.Update) []interactivestate.Update {
	value, _ := update.Value.(map[string]any)
	rawState, _ := value["state"].(map[string]any)
	if len(rawState) == 0 {
		return nil
	}
	templateID, _ := actorTemplateIDFromStateOrSystem(currentState, system, establishedID)
	template := actorStateTemplateByID(system, templateID)
	fieldByReference := actorStateFieldsByReference(template)
	out := make([]interactivestate.Update, 0, len(rawState))
	for _, key := range sortedStringKeys(rawState) {
		field, ok := fieldByReference[actorStateFieldNameKey(key)]
		if !ok {
			// A field this Actor's template does not define is not a value that can
			// be written to them; leaving it out is what the create's own validation
			// would have done for an unknown field.
			continue
		}
		out = append(out, interactivestate.Update{
			Op:    interactivestate.Replace,
			Path:  interactivestate.FormatPath([]string{establishedID, actorStateFieldID(field)}),
			Value: rawState[key],
		})
	}
	return out
}

func sortedStringKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
