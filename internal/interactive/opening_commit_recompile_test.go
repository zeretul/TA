package interactive

import (
	interactivestate "ta/internal/interactive/state"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// openingXiuxianSystem returns the real preset's five-template system, so the
// regression runs against the fields the story actually opens with rather than
// a hand-written stand-in that could drift from the preset.
func openingXiuxianSystem() StoryDirectorActorStateSystem {
	return xiuxianActorStateModule().ActorState
}

// openingXiuxianCreatePayload is the repaired shape the model actually
// submitted on the run that failed: the new important character states where
// they stand (the realm key) and the two current pools, and omits every field
// the cultivation rules derive from the realm.
func openingXiuxianCreatePayload(actorID string) interactivestate.Update {
	return interactivestate.Update{
		Op:   interactivestate.Create,
		Path: "/" + actorID,
		Value: map[string]any{
			"template_id": ActorStateImportantCharacterTemplateID,
			"name":        actorID,
			"state": map[string]any{
				xiuxianFieldRealm:  cultivation.StageKey(cultivation.Qi(1)),
				xiuxianFieldHealth: float64(40),
				xiuxianFieldMana:   float64(30),
			},
		},
	}
}

// openingXiuxianLegitUpdates is one ordinary opening submission: the mandatory
// story-context replacements every turn carries, a legal pool write on the
// protagonist, and the create of a new important character.
func openingXiuxianLegitUpdates(actorID string) []interactivestate.Update {
	return []interactivestate.Update{
		{Op: interactivestate.Replace, Path: "/story/" + storyContextCurrentEventField, Value: "灰袍散修拦住去路"},
		{Op: interactivestate.Replace, Path: "/story/" + storyContextCurrentLocationField, Value: "青石古道"},
		{Op: interactivestate.Replace, Path: "/protagonist/" + xiuxianFieldHealth, Value: float64(37)},
		openingXiuxianCreatePayload(actorID),
	}
}

// recompileAsCommit replays the second compile exactly as the commit performs
// it (story_agent_commit.go:248): same system, same state baseline, same
// options. Anything Prepare accepted must survive this unchanged, because the
// commit is where the turn becomes canonical.
func recompileAsCommit(system StoryDirectorActorStateSystem, state map[string]any, updates []interactivestate.Update) (CompiledTurnStateUpdates, error) {
	return CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{
		SourceTurnID: "turn-1",
	})
}

// TestOpeningSubmissionSurvivesCommitRecompile is the Prepare→commit regression.
//
// The model's legal opening was accepted by PrepareTurnSubmission and then
// refused at commit with "该字段由境界阶段表结算得出，不能在新角色初始状态中直接指定: 攻击".
// The derived fields reappear between the two phases because the create branch
// stores the *normalized* state — which materializes every field that carries a
// default, including the six realm-derived attributes — as the compiled audit
// value, and the commit re-compiles that audit value.
func TestOpeningSubmissionSurvivesCommitRecompile(t *testing.T) {
	system := openingXiuxianSystem()
	state := initialStoryState()
	updates := openingXiuxianLegitUpdates("灰袍散修")

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState:   system,
		CurrentState: state,
		ChoiceCount:  5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &[]string{"拱手回礼", "绕道而行", "暗自戒备", "开口询问", "原地观察"}})
	if !receipt.Ready {
		t.Fatalf("legal opening was refused at submit time: %#v", receipt)
	}

	compiled, err := recompileAsCommit(system, state, prepared.TurnResult().StateUpdates)
	if err != nil {
		t.Fatalf("commit recompile refused a submission Prepare accepted: %v", err)
	}
	if len(compiled.Updates) == 0 {
		t.Fatalf("commit recompile produced no updates")
	}
}

// TestCommitRecompileDoesNotReintroduceDerivedFields pins the mechanism rather
// than only its symptom: the audit value the create branch stores must not
// carry a realm-derived attribute the model never wrote.
func TestCommitRecompileDoesNotReintroduceDerivedFields(t *testing.T) {
	system := openingXiuxianSystem()
	state := initialStoryState()

	compiled, err := CompileTurnStateUpdates(system, state, []interactivestate.Update{
		openingXiuxianCreatePayload("灰袍散修"),
	}, TurnStateUpdateCompileOptions{SourceTurnID: "turn-1"})
	if err != nil {
		t.Fatalf("create compile failed: %v", err)
	}
	for _, update := range compiled.Updates {
		if update.Op != interactivestate.Create {
			continue
		}
		object, ok := update.Value.(map[string]any)
		if !ok {
			t.Fatalf("create audit value is not an object: %#v", update.Value)
		}
		rawState, ok := object["state"].(map[string]any)
		if !ok {
			continue
		}
		for fieldID := range xiuxianDerivedFields() {
			if _, present := rawState[fieldID]; present {
				t.Fatalf("create audit value reintroduced the derived field %q the model never wrote: %#v", fieldID, rawState)
			}
		}
	}
}

// TestOpeningSubmissionStillRefusesModelAuthoredDerivedField keeps the hard
// rule: the fix must not come from loosening what the model may write. A model
// that states 攻击 in a new Actor's initial state is still refused.
func TestOpeningSubmissionStillRefusesModelAuthoredDerivedField(t *testing.T) {
	system := openingXiuxianSystem()
	state := initialStoryState()

	for _, derived := range []string{xiuxianFieldAttack, xiuxianFieldDefense, xiuxianFieldAgility, xiuxianFieldPerception, xiuxianFieldHealthMax, xiuxianFieldManaMax} {
		create := openingXiuxianCreatePayload("灰袍散修")
		create.Value.(map[string]any)["state"].(map[string]any)[derived] = float64(12)

		_, receipt := PrepareTurnSubmission(TurnSubmissionContext{
			ActorState:   system,
			CurrentState: state,
			ChoiceCount:  5,
		}, nil, TurnSubmissionInput{StateUpdates: &[]interactivestate.Update{create}})
		if receipt.ModuleStatus.StateChanges != TurnSubmissionModuleRejected {
			t.Fatalf("model-authored derived field %q was not refused: %#v", derived, receipt)
		}
		if !receiptNamesField(receipt, derived) {
			t.Fatalf("refusal for %q did not name the field: %#v", derived, receipt.Diagnostics)
		}
	}
}

// TestRealmInitializationStillDerivesAttributes keeps the other half of the
// hard rule: a legal realm initialization must still carry the realm key and
// the legal pool write into the settled state.
func TestRealmInitializationStillDerivesAttributes(t *testing.T) {
	system := openingXiuxianSystem()
	state := initialStoryState()

	compiled, err := CompileTurnStateUpdates(system, state, []interactivestate.Update{
		openingXiuxianCreatePayload("灰袍散修"),
	}, TurnStateUpdateCompileOptions{SourceTurnID: "turn-1"})
	if err != nil {
		t.Fatalf("create compile failed: %v", err)
	}
	created := appliedXiuxianState(t, state, compiled.ActorOps)
	realm, _ := actorStateFieldValue(created, "灰袍散修", xiuxianFieldRealm).(string)
	if realm != cultivation.StageKey(cultivation.Qi(1)) {
		t.Fatalf("realm initialization was lost: %q", realm)
	}
	if value := actorStateFieldValue(created, "灰袍散修", xiuxianFieldHealth); value == nil {
		t.Fatalf("the legal pool write was lost")
	}
}

// TestRecompileIsSemanticallyConsistentOnTheSameBaseline is requirement 4: a
// second compile of the normalized result against the same baseline must be
// semantically identical to the first, or the commit is judging a different
// submission than the one the model was told was accepted.
func TestRecompileIsSemanticallyConsistentOnTheSameBaseline(t *testing.T) {
	system := openingXiuxianSystem()
	state := initialStoryState()
	updates := openingXiuxianLegitUpdates("灰袍散修")

	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{
		ActorState: system, CurrentState: state, ChoiceCount: 5,
	}, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &[]string{"拱手回礼", "绕道而行", "暗自戒备", "开口询问", "原地观察"}})
	if !receipt.Ready {
		t.Fatalf("legal opening was refused at submit time: %#v", receipt)
	}

	first, err := recompileAsCommit(system, state, updates)
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	second, err := recompileAsCommit(system, state, prepared.TurnResult().StateUpdates)
	if err != nil {
		t.Fatalf("recompile of the normalized result failed: %v", err)
	}
	if !sameCompiledActorState(t, state, first, second) {
		t.Fatalf("recompile changed the settled state:\nfirst=%#v\nsecond=%#v", first.ActorOps, second.ActorOps)
	}
}

func receiptNamesField(receipt TurnSubmissionReceipt, fieldID string) bool {
	for _, diagnostic := range receipt.Diagnostics {
		if strings.Contains(diagnostic.Actual, fieldID) || strings.Contains(diagnostic.Path, fieldID) {
			return true
		}
	}
	return false
}

// sameCompiledActorState compares the state the two compiles actually produce,
// rather than the op lists, so an equivalent-but-reordered compile still passes.
func sameCompiledActorState(t *testing.T, baseline map[string]any, first, second CompiledTurnStateUpdates) bool {
	t.Helper()
	firstState := appliedXiuxianState(t, baseline, first.ActorOps)
	secondState := appliedXiuxianState(t, baseline, second.ActorOps)
	firstJSON, err := json.Marshal(firstState)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	secondJSON, err := json.Marshal(secondState)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	return reflect.DeepEqual(firstState, secondState) && string(firstJSON) == string(secondJSON)
}
