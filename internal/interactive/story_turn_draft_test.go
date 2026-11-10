package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"ta/internal/agents/conversationjournal"
)

func TestTurnDraftRebuildRetainsPartialModulesRulesAndFullNarrative(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	story, err := store.CreateStory(CreateStoryRequest{Title: "Draft recovery"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "draft-input", OperationID: "draft-run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "Continue the journey")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	system, state := turnSubmissionTestState()
	validation := TurnSubmissionContext{ActorState: system, CurrentState: state, ChoiceCount: 5, PlanningMode: StoryPlanningModeEnabled,
		CurrentPlan: &BranchPlan{Markdown: "## Direction\n\nOld direction.\n\n## Next\n\nOld next."}}
	prepared, receipt := PrepareTurnSubmission(validation, nil, DecodeInteractiveTurnSubmissionInput(`{"state_changes":[],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Direction","markdown":"Accepted direction."},{"heading":"Next","markdown":"## Invalid heading\n\nRejected"}]}}`))
	if receipt.Ready || receipt.ModuleStatus.StateChanges != TurnSubmissionModuleAccepted || receipt.ModuleStatus.PlanUpdate != TurnSubmissionModuleRejected {
		t.Fatalf("initial receipt=%#v", receipt)
	}
	draft := TurnDraft{Identity: identity, Narrative: strings.Repeat("Complete accepted prose. ", 1500), NarrativeSource: "complete_model_response",
		Submission: prepared.Progress(), RuleResolution: &RuleResolution{ID: "locked-roll"}}
	if err := store.SaveTurnDraft(story.ID, "main", draft, nil); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(store.storyPath(story.ID) + ".pre-resilience-v1.bak")
	if err != nil || strings.Contains(string(backup), `"type":"turn_draft"`) {
		t.Fatalf("pre-upgrade backup invalid: %v", err)
	}
	// Drop every derived locator; the canonical log alone must retain progress.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(conversationjournal.SidecarPath(store.storyPath(story.ID))); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	store = NewStore(workspace)
	restored, found, err := store.LoadTurnDraft(story.ID, "main", identity)
	if err != nil || !found || !sameCanonicalJSON(restored, draft) {
		t.Fatalf("restored draft identity=%#v submission=%#v full_narrative=%t found=%t error=%v", restored.Identity, restored.Submission, restored.Narrative == draft.Narrative, found, err)
	}
	final, receipt := PrepareTurnSubmission(validation, restored.Submission.Prepared(), DecodeInteractiveTurnSubmissionInput(`{"choices":["One","Two","Three","Four","Five"],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Next","markdown":"Repaired next."}]}}`))
	if !receipt.Ready || final.TurnResult().PlanUpdate == nil || !strings.Contains(*final.TurnResult().PlanUpdate, "Accepted direction.") {
		t.Fatalf("partial plan was lost: %#v", receipt)
	}
	if restored.RuleResolution.ID != "locked-roll" || len(restored.Narrative) <= 16<<10 {
		t.Fatal("draft was reduced to feedback text")
	}
}

func TestFinalTurnEndsDraftInTheSameTransaction(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Draft commit"})
	if err != nil {
		t.Fatal(err)
	}
	identity := DomainCommitIdentity{CommandID: "input", OperationID: "run", Cycle: 1}
	input, err := NewPlayerInputIntent(identity, "main", "Continue")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTurnDraft(story.ID, "main", TurnDraft{Identity: identity, Narrative: "Accepted prose.", NarrativeSource: "complete_model_response"}, nil); err != nil {
		t.Fatal(err)
	}
	request := AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "Accepted prose.",
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
	first, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := store.AppendTurnWithState(story.ID, request)
	if err != nil || first.ID != second.ID {
		t.Fatalf("duplicate output=%s error=%v", second.ID, err)
	}
	if _, found, err := store.LoadTurnDraft(story.ID, "main", identity); err != nil || found {
		t.Fatalf("committed draft remained active: found=%t error=%v", found, err)
	}
}

func TestTurnDraftReloadCommitsNativeEquipmentAndRestOnce(t *testing.T) {
	r, a := fantasyOrigin(t, "sg-start-road")
	a.Energy = 1
	store, id := adventureFixtureStoryWithActor(t, r, a, "sg-l01")
	root := store.Root()
	actions := []AdventureAction{{Action: "equip", ItemID: "sg-travel-blade"}, {Action: "rest", Hours: 16}}
	for _, action := range actions {
		identity := DomainCommitIdentity{CommandID: "draft-" + action.Action, OperationID: "run-" + action.Action, Cycle: 1}
		user := "按实际题材规则执行" + action.Action
		input, err := NewPlayerInputIntent(identity, "main", user)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.CommitPlayerInput(id, input); err != nil {
			t.Fatal(err)
		}
		prepared := &PreparedTurnSubmission{result: TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: user, Adventure: &action}}, stateUpdatesAccepted: true, choicesAccepted: true, worldAccepted: true}
		draft := TurnDraft{Identity: identity, Narrative: "执行已经确认的正式行动。", NarrativeSource: "complete_model_response", Submission: prepared.Progress()}
		if err = store.SaveTurnDraft(id, "main", draft, nil); err != nil {
			t.Fatal(err)
		}
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(conversationjournal.SidecarPath(store.storyPath(id))); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		store = NewStore(root)
		restored, found, err := store.LoadTurnDraft(id, "main", identity)
		if err != nil || !found {
			t.Fatalf("load: found=%v err=%v", found, err)
		}
		result := restored.Submission.Prepared().TurnResult()
		if result.World == nil || result.World.Adventure == nil || *result.World.Adventure != action {
			t.Fatal("accepted native action disappeared from durable draft")
		}
		req := AppendTurnWithStateRequest{BranchID: "main", User: user, Narrative: draft.Narrative, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: 1, TurnResult: &result}
		first, _, err := store.AppendTurnWithState(id, req)
		if err != nil {
			t.Fatal(err)
		}
		again, _, err := store.AppendTurnWithState(id, req)
		if err != nil || first.ID != again.ID {
			t.Fatalf("retry committed twice: %v", err)
		}
	}
	window, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if window.Actor.Equipment["person:weapon"] != "sg-travel-blade" || window.Actor.Energy != window.Actor.MaxEnergy || window.ClockSeconds != 16*3600 {
		t.Fatalf("restored actions did not settle once: %+v", window.Actor)
	}
	store.Close()
}

func TestTurnDraftRoundTripRetainsAllOptionalSettlementInputs(t *testing.T) {
	prepared := &PreparedTurnSubmission{result: TurnResult{
		CharacterFacts: &TurnCharacterFactsInput{}, Cultivation: &TurnCultivationInput{}, Skills: &TurnSkillLearningInput{}, Techniques: &TurnTechniqueInput{}, Items: &TurnItemsInput{}, Trade: &TurnTradeInput{}, Rest: &TurnRestInput{}, Craft: &TurnCraftInput{}, World: &TurnWorldActionInput{Action: "wait", DurationSeconds: 3600, SourceText: "等一个小时"},
	}}
	want := prepared.TurnResult()
	data, err := json.Marshal(prepared.Progress())
	if err != nil {
		t.Fatal(err)
	}
	var progress TurnSubmissionProgress
	if err = json.Unmarshal(data, &progress); err != nil {
		t.Fatal(err)
	}
	if got := progress.Prepared().TurnResult(); !reflect.DeepEqual(want, got) {
		t.Fatalf("settlement input lost: want=%+v got=%+v", want, got)
	}
}
