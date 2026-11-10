package interactive

import "ta/internal/character"

// The affinity a character holds toward the protagonist has exactly one owner:
// the character projection's settled AffinityChange records. The character
// window, the chat window and the intimacy window all read it through
// character.State.AffinityFor, which is CurrentAffinity over those records.
//
// The Actor state's 对主角好感度 field is a different store that the model
// writes directly. Nothing in the rules connects the two, so the same character
// carried two numbers that disagreed: the story ledger rendered the Actor state
// field while the character window rendered the settled record, and for one
// character in the acceptance run the signs were opposite (12 against -3).
//
// Rather than retire the field — it is a public field the ledger template
// renders for every important character, and a story that has not settled
// anything yet should read 0 rather than lose the line — the settled value is
// written into the field wherever story state leaves the store. Every reader of
// snapshot state (the ledger, the model's own turn context, the director
// console) therefore sees the one number the rules computed.
const actorStateFavorabilityField = "对主角好感度"

// actorStateFieldIsSettledFavorability reports whether a template field is the
// one the affinity settlement owns.
//
// The name is the judgement because the name is the only identity a live story
// has: freezing the schema blanks every field's Path and LegacyPath
// (FreezeActorStateSchemaWithRules), and a frozen schema is what a story
// compiles against. A path test therefore matched the unfrozen preset and
// nothing else, which is how a guard can pass its own test and never fire.
func actorStateFieldIsSettledFavorability(field ActorStateField) bool {
	return normalizeActorStateFieldName(field.Name) == actorStateFavorabilityField
}

// applySettledFavorability stamps the settled affinity onto each Actor that the
// character projection knows about. An Actor the projection does not know keeps
// whatever it carries: a creature or a walk-on the story tracks only in state
// has no settled record, and zeroing it would delete a fact the story wrote.
func applySettledFavorability(state map[string]any, characters *character.State) {
	if state == nil || characters == nil {
		return
	}
	rawActors, ok := state[actorStateRoot].(map[string]any)
	if !ok || len(rawActors) == 0 {
		return
	}
	for actorID, raw := range rawActors {
		if actorID == DefaultActorID {
			continue
		}
		record, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, known := characters.Profile(actorID); !known {
			continue
		}
		actorState, ok := record["state"].(map[string]any)
		if !ok {
			continue
		}
		value, _ := characters.AffinityFor(actorID)
		actorState[actorStateFavorabilityField] = float64(value)
	}
}
