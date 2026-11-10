package interactive

import (
	"encoding/json"
	"errors"
	"ta/internal/cultivation"
	"testing"
)

func TestWoundedCastFailureCommitsPriorActionsAndCannotReroll(t *testing.T) {
	system := combatTestSystem()
	system.InitialActors[0].State[xiuxianFieldHealth] = 120
	system.InitialActors[0].State[xiuxianFieldBurden] = 40
	system.InitialActors[0].State[xiuxianFieldLearnedSkills] = map[string]any{"重雷": map[string]any{"定义": "heavy_thunder", "版本": 1, "掌握程度": "入门"}}
	store := NewStore(t.TempDir())
	defer func() { _ = store.Close() }()
	story, err := store.CreateStory(CreateStoryRequest{Title: "Battle balance regression", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "对峙", Narrative: "双方对峙。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.StartBattle(story.ID, BattleStartRequest{BranchID: "main", SessionID: "audit-burden-fixed", Combatants: combatTestSpecs(), CommandID: "audit-open", CommandHash: "open"})
	if err != nil {
		t.Fatal(err)
	}
	hero := opened.Session.CombatantByID(DefaultActorID)
	heavy, _ := cultivation.SkillByID("heavy_thunder")
	strike, _ := cultivation.SkillByID("strike")
	if !cultivation.CanPerformWithStrain(heavy, hero.Cultivation, hero.Attributes, hero.Burden, hero.ManaReduction, hero.StrainReduction, hero.HasImplement).Allowed {
		t.Fatal("fixture was not initially legal")
	}
	actions := map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: heavy, TargetID: "rival"}, "rival": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: DefaultActorID}}
	request := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, Actions: actions, CommandID: "audit-draw-1", CommandHash: "same-request"}
	first, err := store.ResolveBattleRound(story.ID, request)
	if err != nil {
		t.Fatalf("a later wound rolled the enemy's legal turn back: %v", err)
	}
	if first.Outcome.Order[0] != "rival" || first.Session.Round != 2 || first.Session.CombatantByID(DefaultActorID).Attributes.Health != 95 || first.Session.CombatantByID(DefaultActorID).Attributes.Mana != hero.Attributes.Mana {
		t.Fatalf("wrong interruption settlement: %+v", first)
	}
	found := false
	for _, step := range first.Outcome.Steps {
		if step.ActorID == DefaultActorID && step.ReasonKey == cultivation.ReasonBurdenCapacity {
			found = true
			if step.ActionRejected || step.Cast.Allowed {
				t.Fatalf("interrupted cast was paid or marked a new-input rejection: %+v", step)
			}
		}
	}
	if !found {
		t.Fatal("the interrupted intent disappeared from history")
	}
	retry, err := store.ResolveBattleRound(story.ID, request)
	if err != nil || first.EventID != retry.EventID || combatBalanceStateJSON(t, first.Session) != combatBalanceStateJSON(t, retry.Session) || combatBalanceStateJSON(t, first.Outcome) != combatBalanceStateJSON(t, retry.Outcome) {
		t.Fatal("same command changed committed interruption", err)
	}
	root := store.Root()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	sessions, err := store.CombatSessions(story.ID, "main")
	if err != nil || combatBalanceStateJSON(t, sessions[opened.Session.ID]) != combatBalanceStateJSON(t, first.Session) {
		t.Fatal("cold replay lost earlier enemy damage", err)
	}
	history, err := store.CombatHistory(story.ID, "main", opened.Session.ID)
	if err != nil || len(history) != 1 {
		t.Fatal("interrupted round was not durably recorded", err)
	}
	// This is now a genuinely invalid declaration at the start of round two.
	// Changing the command does not restore the pre-wound state or a free cast.
	request.CommandID = "audit-draw-0"
	if _, err = store.ResolveBattleRound(story.ID, request); !errors.Is(err, ErrCombatActionRejected) {
		t.Fatalf("initially unaffordable cast stopped being correctable: %v", err)
	}
	sessions, err = store.CombatSessions(story.ID, "main")
	if err != nil || combatBalanceStateJSON(t, sessions[opened.Session.ID]) != combatBalanceStateJSON(t, first.Session) {
		t.Fatal("new command rewrote the committed first exchange", err)
	}
}

func combatBalanceStateJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInitiallyIllegalCastRemainsRejectedWhenEnemyWouldDefeatCasterFirst(t *testing.T) {
	system := combatTestSystem()
	system.InitialActors[0].State[xiuxianFieldHealth] = 1
	system.InitialActors[0].State[xiuxianFieldMana] = 0
	system.InitialActors[0].State[xiuxianFieldLearnedSkills] = map[string]any{"重雷": map[string]any{"定义": "heavy_thunder", "版本": 1, "掌握程度": "入门"}}
	store := NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(CreateStoryRequest{Title: "Initially illegal cast", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "对峙", Narrative: "双方对峙。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := store.StartBattle(story.ID, BattleStartRequest{BranchID: "main", SessionID: "audit-burden-fixed", Combatants: combatTestSpecs(), CommandID: "open", CommandHash: "open"})
	if err != nil {
		t.Fatal(err)
	}
	heavy, _ := cultivation.SkillByID("heavy_thunder")
	strike, _ := cultivation.SkillByID("strike")
	actions := map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: heavy, TargetID: "rival"}, "rival": {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: DefaultActorID}}
	// The counterfactual exchange really can kill the caster before their turn.
	draws := func(purpose, actor, target string) float64 {
		if purpose == "initiative" && actor == "rival" {
			return 1
		}
		return 0
	}
	next, out := cultivation.ResolveExchange(opened.Session, actions, draws)
	if out.Order[0] != "rival" || next.CombatantByID(DefaultActorID).Attributes.Health != 0 {
		t.Fatal("fixture did not expose the death-before-refusal boundary")
	}
	rejected := false
	for _, step := range out.Steps {
		if step.ActorID == DefaultActorID && step.ActionRejected && step.ReasonKey == cultivation.ReasonInsufficientMana {
			rejected = true
		}
	}
	if !rejected {
		t.Fatal("the prior input error disappeared when its actor fell")
	}
	for _, command := range []string{"audit-draw-1", "audit-draw-0"} {
		_, err = store.ResolveBattleRound(story.ID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, Actions: actions, CommandID: command, CommandHash: "same-request"})
		if !errors.Is(err, ErrCombatActionRejected) {
			t.Fatalf("initially invalid cast was committed for %s: %v", command, err)
		}
	}
	actions[DefaultActorID] = cultivation.CombatAction{Kind: cultivation.ActionUseItem, ItemInstanceID: "missing-dose"}
	if _, err = store.ResolveBattleRound(story.ID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, Actions: actions, CommandID: "missing-dose-first", CommandHash: "missing-dose"}); !errors.Is(err, ErrCombatActionRejected) {
		t.Fatalf("missing carried item became a committed fatal turn: %v", err)
	}
	sessions, err := store.CombatSessions(story.ID, "main")
	if err != nil || combatBalanceStateJSON(t, sessions[opened.Session.ID]) != combatBalanceStateJSON(t, opened.Session) {
		t.Fatal("input error spent the lethal enemy action", err)
	}
	history, err := store.CombatHistory(story.ID, "main", opened.Session.ID)
	if err != nil || len(history) != 0 {
		t.Fatal("input error became durable history", err)
	}
}
