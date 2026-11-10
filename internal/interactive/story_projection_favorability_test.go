package interactive

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// The story ledger and the character window answer the same question — how does
// this person feel about the protagonist — and before this fix they answered it
// from two stores that nothing connected. The ledger rendered the Actor state's
// 对主角好感度, which the model wrote directly; the window rendered the settled
// AffinityChange the projector computed from character_facts.affinities. In the
// acceptance run every character disagreed and one had the opposite sign.
//
// The tests below fix the resolution: the settled number is the one that
// reaches snapshot state, and a direct write of the field is refused rather
// than allowed to create a second, unbanded number.

// favorabilityMeet creates the character with an introduction, because a person
// with nothing said about them is deliberately not registered, and only a
// registered person has a settled affinity to read.
func favorabilityMeet(t *testing.T, store *Store, storyID, name string) {
	t.Helper()
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "遇见" + name,
		Narrative: name + "站在溪边，看了你一眼。",
		TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{{
			Op:   interactivestate.Create,
			Path: "/" + name,
			Value: map[string]any{
				"template_id": ActorStateImportantCharacterTemplateID,
				"name":        name,
				"state": map[string]any{
					"基本身份":            name + "，越国散修，常在坊市外围采药。",
					xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(2)),
				},
			},
		}}},
	}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func favorabilityActorValue(t *testing.T, store *Store, storyID, actorID string) any {
	t.Helper()
	return actorStateFieldValue(combatTestState(t, store, storyID), actorID, actorStateFavorabilityField)
}

// TestSettledAffinityIsTheNumberTheLedgerShows is the fix itself: one settled
// change moves the Actor state field, so the ledger and the window agree.
func TestSettledAffinityIsTheNumberTheLedgerShows(t *testing.T) {
	store, storyID := f17BeastStory(t)
	favorabilityMeet(t, store, storyID, "陈巧倩")

	f17FlowTurn(t, store, storyID, "驳了她的面子", "你把那杯酒推了回去，当众说清了自己的立场。",
		TurnCharacterFactsInput{
			Affinities: []TurnCharacterAffinityInput{{
				CharacterID: "陈巧倩", Kind: "offense", Delta: -7,
				Basis: "主角拦下她递出的那杯酒，当众驳了她的面子",
			}},
		})

	characters, err := store.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	settled, band := characters.AffinityFor("陈巧倩")
	if settled != -7 {
		t.Fatalf("settled affinity = %d, want -7 (%s)", settled, band)
	}
	if got := favorabilityActorValue(t, store, storyID, "陈巧倩"); got != float64(-7) {
		t.Fatalf("the ledger field reads %#v, want the settled %d", got, settled)
	}

	// A reopen replays the journal; the two readers must still agree.
	reopened := NewStore(store.root)
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, err := reopened.CharacterState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := replayed.AffinityFor("陈巧倩"); value != settled {
		t.Fatalf("affinity after reopen = %d, want %d", value, settled)
	}
	snapshot, err := reopened.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := actorStateFieldValue(snapshot.State, "陈巧倩", actorStateFavorabilityField); got != float64(settled) {
		t.Fatalf("after reopen the ledger field reads %#v, want %d", got, settled)
	}
}

// TestDirectFavorabilityWritesAreRefused keeps the second number from coming
// back. Both a replace and a delta are refused, because a delta would drift the
// field away from the settlement just as surely.
func TestDirectFavorabilityWritesAreRefused(t *testing.T) {
	store, storyID := f17BeastStory(t)
	favorabilityMeet(t, store, storyID, "陈巧倩")

	for _, update := range []interactivestate.Update{
		{Op: interactivestate.Replace, Path: "/陈巧倩/" + actorStateFavorabilityField, Value: float64(30)},
		{Op: interactivestate.Delta, Path: "/陈巧倩/" + actorStateFavorabilityField, Value: 5},
	} {
		_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
			BranchID: "main", User: "改写", Narrative: "气氛变了。",
			TurnResult: &TurnResult{Choices: testTurnChoices(), StateUpdates: []interactivestate.Update{update}},
		})
		var validationError *StateUpdateValidationError
		if !errors.As(err, &validationError) || validationError.Code != "affinity_owned_state_update" {
			t.Fatalf("%s of the settled field should be refused, got %v", update.Op, err)
		}
	}

	// The refusal did not damage the field: it still carries the settlement's 0.
	if got := favorabilityActorValue(t, store, storyID, "陈巧倩"); got != float64(0) {
		t.Fatalf("a refused write changed the field to %#v", got)
	}
}

// TestTheGuardSurvivesTheFrozenSchema is the reason the judgement reads the
// field's name. Freezing strips every field's path, so a guard keyed on the
// preset path would pass against the unfrozen preset and then match nothing in
// the story the player is actually in.
func TestTheGuardSurvivesTheFrozenSchema(t *testing.T) {
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{importantCharacterStateTemplate(cultivation.ReferenceRules(),
		spec)}}
	frozen := FreezeActorStateSchema(system, true)
	for _, template := range frozen.System.Templates {
		for _, field := range template.Fields {
			if field.Path != "" || field.LegacyPath != "" {
				t.Fatalf("freezing is expected to drop paths, so this test's premise is stale: %#v", field)
			}
		}
	}

	compiled, err := CompileTurnStateUpdates(frozen.System, map[string]any{actorStateRoot: map[string]any{"陈巧倩": map[string]any{
		"id": "陈巧倩", "template_id": ActorStateImportantCharacterTemplateID,
		"state": map[string]any{actorStateFavorabilityField: float64(0)},
	}}}, []interactivestate.Update{{
		Op: interactivestate.Replace, Path: "/陈巧倩/" + actorStateFavorabilityField, Value: float64(4),
	}}, TurnStateUpdateCompileOptions{})
	var validationError *StateUpdateValidationError
	if !errors.As(err, &validationError) || validationError.Code != "affinity_owned_state_update" {
		t.Fatalf("a frozen schema must still refuse the write, got %#v / %v", compiled, err)
	}
}

// TestAnUnsettledCharacterKeepsTheFieldAtZero is the other half of the rule: a
// registered character with no settled change reads 0, because 0 is a stranger
// who has not decided rather than a missing line in the ledger.
func TestAnUnsettledCharacterKeepsTheFieldAtZero(t *testing.T) {
	store, storyID := f17BeastStory(t)
	favorabilityMeet(t, store, storyID, "陈巧倩")

	if got := favorabilityActorValue(t, store, storyID, "陈巧倩"); got != float64(0) {
		t.Fatalf("a character with no settled change reads %#v, want 0", got)
	}
}
