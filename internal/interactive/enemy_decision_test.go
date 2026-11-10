package interactive

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"ta/internal/cultivation"
	"testing"
)

func TestEnemyDecisionBattleSelectionRechecksHeadAndReplays(t *testing.T) {
	store, id := combatTestStory(t)
	defer store.Close()
	opened, err := store.StartBattle(id, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "ai-open", CommandHash: "open"})
	if err != nil {
		t.Fatal(err)
	}
	strike, _ := cultivation.SkillByID("strike")
	req := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: strike, TargetID: "rival"}}, CommandID: "ai-one", CommandHash: "one"}
	prep, err := store.PrepareBattleEnemyDecision(id, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(prep.Snapshot.Enemies) != 1 || prep.Snapshot.Enemies[0].ActorID != "rival" {
		t.Fatalf("wrong audience: %+v", prep)
	}
	// The prompt is unchanged when only the still-unexecuted player declaration changes.
	alternate := req
	alternate.Actions = map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionRest}}
	other, err := store.PrepareBattleEnemyDecision(id, alternate)
	if err != nil || !reflect.DeepEqual(prep.Snapshot, other.Snapshot) {
		t.Fatal("pending player intent leaked into enemy context", err)
	}
	raw, _ := json.Marshal(prep.Snapshot)
	for _, secret := range []string{"Skills", "inventory", "ManaFraction", "heavy_thunder", "PlayerIntent"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("non-whitelisted state leaked: %s", secret)
		}
	}
	req.ExpectedHead = prep.Snapshot.Head
	req.ExpectedRound = prep.Snapshot.Round
	req.EnemyChoices = map[string]string{"rival": "rest"}
	req.DecisionStatus = "ai"
	first, err := store.ResolveBattleRound(id, req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range first.Outcome.Steps {
		if step.ActorID == "rival" {
			found = true
			if step.Kind != cultivation.ActionRest {
				t.Fatalf("valid chosen action was ignored: %+v", step)
			}
		}
	}
	if first.EnemyDecision == nil || first.EnemyDecision.Status != "ai" || !reflect.DeepEqual(first.EnemyDecision.AIActors, []string{"rival"}) {
		t.Fatalf("missing committed AI provenance: %+v", first.EnemyDecision)
	}
	if !found {
		t.Fatal("enemy response absent")
	}
	replay, err := store.PrepareBattleEnemyDecision(id, req)
	if err != nil || !replay.Replayed || replay.Battle.EventID != first.EventID || !reflect.DeepEqual(replay.Battle.EnemyDecision, first.EnemyDecision) {
		t.Fatal("retry did not return committed decision", err)
	}
	stale := req
	stale.CommandID = "ai-stale"
	stale.CommandHash = "different"
	if _, err := store.ResolveBattleRound(id, stale); err == nil {
		t.Fatal("model decision based on an old head committed")
	}
	root := store.Root()
	store.Close()
	store = NewStore(root)
	replay, err = store.PrepareBattleEnemyDecision(id, req)
	if err != nil || !replay.Replayed || replay.Battle.EventID != first.EventID || !reflect.DeepEqual(replay.Battle.EnemyDecision, first.EnemyDecision) {
		t.Fatal("cold replay forgot model-selected settlement", err)
	}
}

func TestEnemyDecisionAdventureUsesOnlyPaidLearnedCandidates(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	a.Battle.Health = 40
	before, _ := json.Marshal(a)
	menu := adventureEnemyDecisionCandidates(r, &a)
	if len(menu) < 2 {
		t.Fatal("legal learned choices absent")
	}
	request := AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:heal"}
	after, _, err := ResolveAdventureAction(r, a, request, "model-heal")
	if err != nil {
		t.Fatal(err)
	}
	if after.Battle.EnemyLastSkillID != "heal" || after.Battle.EnemyEnergy != 88 || after.Battle.Health != 70 || after.Health != a.Health {
		t.Fatalf("model choice bypassed actual payment or action: %+v", after)
	}
	afterRaw, _ := json.Marshal(a)
	if string(before) != string(afterRaw) {
		t.Fatal("candidate enumeration or resolution mutated original")
	}
	request.enemyChoice = "skill:not-learned"
	invalid, _, err := ResolveAdventureAction(r, a, request, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	request.enemyChoice = ""
	fallback, _, err := ResolveAdventureAction(r, a, request, "fallback")
	if err != nil || !reflect.DeepEqual(invalid, fallback) {
		t.Fatal("invalid choice did not use deterministic fallback", err)
	}
	// External JSON cannot inject the package-private server-side choice.
	raw, _ := json.Marshal(AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:heal"})
	if strings.Contains(string(raw), "heal") || strings.Contains(string(raw), "enemy") {
		t.Fatal("server intent became a client-supplied field")
	}
}

func TestEnemyDecisionAdventureChoicePersistsWithoutFutureIntent(t *testing.T) {
	for _, family := range []string{"wuxia", "space"} {
		t.Run(family, func(t *testing.T) {
			r, _ := enemyTacticsFixture(t, family)
			r.Opponents[0].Location = "船坞"
			r.Opponents[0].SkillIDs = []string{"strike", "guard"}
			store, id := adventureTestStoryWithRules(t, r)
			defer store.Close()
			_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			in := AdventureAction{Action: "battle_round", Choice: "attack"}
			prepared, err := store.PrepareAdventureEnemyDecision(id, "main", "ai-guard", before.Head, in)
			if err != nil {
				t.Fatal(err)
			}
			if len(prepared.Snapshot.Enemies) != 1 || prepared.Snapshot.Enemies[0].ActorID != "opponent" {
				t.Fatal("wrong adventure audience")
			}
			chosen, err := store.CommitAdventureActionWithEnemyDecision(id, "main", "ai-guard", before.Head, in, map[string]string{"opponent": "skill:guard"}, "ai")
			if err != nil {
				t.Fatal(err)
			}
			if chosen.EnemyDecision == nil || chosen.EnemyDecision.Status != "ai" || !reflect.DeepEqual(chosen.EnemyDecision.AIActors, []string{"opponent"}) {
				t.Fatalf("missing adventure provenance: %+v", chosen.EnemyDecision)
			}
			if chosen.Actor.Battle.EnemyLastSkillID != "guard" || chosen.Actor.Battle.EnemyEnergy != 92 || chosen.Actor.Health != before.Actor.Health {
				t.Fatalf("selected guard did not pay and settle: %+v", chosen.Actor.Battle)
			}
			root := store.Root()
			store.Close()
			store = NewStore(root)
			replay, err := store.PrepareAdventureEnemyDecision(id, "main", "ai-guard", before.Head, in)
			if err != nil || !replay.Replayed || !reflect.DeepEqual(*replay.Adventure, chosen) {
				t.Fatal("cold command replay changed paid intent", err)
			}
			duplicate, err := store.CommitAdventureActionWithEnemyDecision(id, "main", "ai-guard", before.Head, in, map[string]string{"opponent": "skill:strike"}, "ai")
			if err != nil || !reflect.DeepEqual(duplicate, chosen) {
				t.Fatal("changed later model output rewrote committed command", err)
			}
			fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before enemy choice"})
			if err != nil {
				t.Fatal(err)
			}
			old, err := store.AdventureWindow(id, fork.ID)
			if err != nil || old.Actor.Battle.EnemyEnergy != 100 || old.Actor.Battle.EnemyLastSkillID != "" {
				t.Fatal("future enemy intent leaked across branch", err)
			}
		})
	}
}

func TestEnemyDecisionVehicleObservationDoesNotUseHiddenPilotPools(t *testing.T) {
	actor := AdventureActor{Health: 1, MaxHealth: 100, Battle: &AdventureBattle{Scale: "ship", AssetID: "frigate"}, Assets: map[string]AdventureAsset{"frigate": {Health: 800, MaxHealth: 1000, Shield: 50, Fuel: 77}}}
	first := adventureVisibleOpponent(actor)
	actor.Health = 100
	asset := actor.Assets["frigate"]
	asset.Fuel = 2
	actor.Assets["frigate"] = asset
	if !reflect.DeepEqual(first, adventureVisibleOpponent(actor)) {
		t.Fatal("hidden pilot health or fuel changed visible hull observation")
	}
	if len(first) != 2 || !strings.Contains(first[0], "轻伤") {
		t.Fatalf("wrong visible hull condition: %v", first)
	}
}

func TestEnemyDecisionPreflightResolvesLearnedGeneratedShield(t *testing.T) {
	store, id := combatTestStory(t)
	defer store.Close()
	talismanTurn(t, store, id, "main", "依完整护罩篇研习自创护盾七日", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "shield", Name: "青岩罩", Source: "完整护罩篇与师傅当面传授", Minutes: 10080}}}})
	opened, err := store.StartBattle(id, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "generated-ai-open", CommandHash: "open"})
	if err != nil {
		t.Fatal(err)
	}
	generatedID := cultivation.GeneratedSkillID("shield", "青岩罩")
	req := BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: "generated-ai-round", CommandHash: "shield", Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: generatedID}, TargetID: DefaultActorID}}}
	prepared, err := store.PrepareBattleEnemyDecision(id, req)
	if err != nil {
		t.Fatalf("bare generated shield did not resolve before preflight: %v", err)
	}
	req.ExpectedHead, req.ExpectedRound = prepared.Snapshot.Head, prepared.Snapshot.Round
	req.EnemyChoices = map[string]string{"rival": "rest"}
	committed, err := store.ResolveBattleRound(id, req)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range committed.Outcome.Steps {
		if step.ActorID == DefaultActorID && step.SkillID == generatedID && step.Cast.Allowed {
			found = true
		}
	}
	if !found || committed.Session.CombatantByID(DefaultActorID).Shield.Amount <= 0 {
		t.Fatal("generated shield did not actually cast")
	}
	req.CommandID, req.CommandHash = "unlearned-ai-round", "unlearned"
	thunder, _ := cultivation.SkillByID("heavy_thunder")
	req.Actions = map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: thunder, TargetID: "rival"}}
	if _, err := store.PrepareBattleEnemyDecision(id, req); !errors.Is(err, ErrCombatTechniqueNotLearned) {
		t.Fatalf("unlearned skill reached model preparation: %v", err)
	}
}
