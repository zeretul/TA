package interactive

import (
	"encoding/json"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

func TestReviewLethalEvidenceDoesNotBorrowWordsFromAnotherSpeaker(t *testing.T) {
	for _, test := range []struct {
		name, user, narrative, actorID, targetID, actorName, targetName string
	}{
		{"bystander threat", "留意动静", "旁观者喝道：‘我要杀死主角！’对手站在远处看着，并没有出声。", "rival", DefaultActorID, "对手", "主角"},
		{"player reports a threat", "我问对手为什么有人要杀死我。", "主角向对手询问传闻。", DefaultActorID, "rival", "主角", "对手"},
		{"skill is not kill", "Listen.", "Rival said Hero should practise a skill.", "rival", DefaultActorID, "Rival", "Hero"},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := TurnEvent{
				V: 1, Type: StoryEventTypeTurn, ID: "review-turn", User: test.user, Narrative: test.narrative,
				StateDelta: &StateDelta{Ops: []interactivestate.Op{{
					Op: "set", Path: "/story/本次交战", SourceTurnID: "review-turn", SourceKind: interactivestate.SourceTurnResult,
					Value: map[string]any{"rival": map[string]any{"lethal_intents": []any{map[string]any{"actor_id": test.actorID, "target_id": test.targetID}}}},
				}}},
			}
			encoded, err := json.Marshal(turn)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			record := StoryEventRecord{Envelope: StoryEventEnvelope{Type: StoryEventTypeTurn, ID: turn.ID}, Raw: raw}
			if got := freezeCombatLethalIntents([]StoryEventRecord{record}, turn.ID, fatalityTestSession(30)); len(got) != 0 {
				t.Fatalf("unrelated or reported words established lethal intent: %#v", got)
			}
		})
	}
}

func TestReviewForkDuringBattleKeepsItsFrozenLethalIntent(t *testing.T) {
	_, store, storyID, _, _ := combatFatalityTestStory(t, DefaultActorID)
	t.Cleanup(func() { _ = store.Close() })
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", SessionID: "review-risk-battle", Combatants: combatTestSpecs(),
		CommandID: "review-risk-open", CommandHash: "review-risk-open-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	rested, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionRest},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: "review-risk-rest", CommandHash: "review-risk-rest-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "留意对手", Narrative: "双方仍在交战，主角留意对手的动作。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	})
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: turn.ID, Title: "交战仍在继续"})
	if err != nil {
		t.Fatal(err)
	}
	intents, err := store.CombatLethalIntents(storyID, fork.ID, opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 || intents[0].ActorID != "rival" || intents[0].TargetID != DefaultActorID {
		t.Fatalf("a fork at the committed opening lost its frozen lethal risk: %#v", intents)
	}
	commandID := combatFatalityCommandForHit(rested.Session, "rival", DefaultActorID)
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: fork.ID, SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionRest},
			"rival":        {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: DefaultActorID},
		}, CommandID: commandID, CommandHash: commandID + "-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(played.Fatalities) != 1 || played.TerminalOutcome == nil || !played.TerminalOutcome.Terminal {
		t.Fatalf("a branch inheriting the same fatal hit must preserve its death rules: %#v", played)
	}
}
