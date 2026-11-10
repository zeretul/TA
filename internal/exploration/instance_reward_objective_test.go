package exploration

import (
	"strings"
	"testing"
)

// These tests cover the one rule that makes an objective a fact rather than a
// sentence: taking the declared reward an objective is tied to is what records the
// completion. Nothing else ticks it, so a panel showing a tick is showing something
// that really happened.

// rewardTestDefinition is the real authored realm, whose two rewards are tied to
// the two objectives.
func rewardTestDefinition() DungeonDefinition {
	definition, ok := RealmDefinition(RealmGuXiuDongFu)
	if !ok {
		panic("the authored realm must be registered")
	}
	return definition
}

// rewardTestEnter creates an instance standing in the herb plot, which is where
// the first reward is declared.
func rewardTestEnter(t *testing.T) (DungeonDefinition, DungeonScope, DungeonInstanceSnapshot) {
	t.Helper()
	def := rewardTestDefinition()
	scope := DungeonScope{WorldID: "world.alpha", BranchID: "branch.main"}
	hash, err := DungeonDefinitionContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	permit := InstanceActionPermit{
		Binding:  InstancePermitBinding{InstanceID: "realm.instance", WorldID: scope.WorldID, BranchID: scope.BranchID, DefinitionHash: hash, Revision: 0},
		Decision: DecisionAllowed, Action: ActionEnter, TargetID: "realm.outer_hall", SourceRef: "realm.enter",
	}
	created, err := CreateDungeonInstance(CreateDungeonInstanceInput{
		Definition: def, Scope: scope, InstanceID: "realm.instance",
		EntryAreaID: "realm.outer_hall", Permit: permit,
		Battle: BattleNone, Blocking: BlockingNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := created.Snapshot
	moveCtx := testInstanceContext(snapshot, scope, testInstancePermit(ActionMove, "realm.hall_to_plot", "realm.outer_hall", "realm.herb_plot", "realm.move"))
	moved, err := MoveDungeonInstance(def, scope, snapshot, moveCtx, MoveDungeonInstanceInput{ConnectionID: "realm.hall_to_plot", DestinationAreaID: "realm.herb_plot"})
	if err != nil {
		t.Fatal(err)
	}
	return def, scope, moved.Snapshot
}

// TestTakingADeclaredRewardCompletesItsObjective is the rule itself.
func TestTakingADeclaredRewardCompletesItsObjective(t *testing.T) {
	def, scope, snapshot := rewardTestEnter(t)

	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, "realm.herb_ref", "", "", "realm.reward"))
	transition, reward, err := GrantInstanceReward(def, scope, snapshot, ctx, RealmRewardRequest{
		ContentRefID: "realm.herb_ref", SourceRef: "realm.reward",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reward.RefID != "pill.restore.health.lower" {
		t.Fatalf("the grant must name the declared catalogue entry: %q", reward.RefID)
	}
	if len(transition.Snapshot.CompletedObjectives) != 1 {
		t.Fatalf("taking the tied reward must complete its objective: %#v", transition.Snapshot.CompletedObjectives)
	}
	if transition.Snapshot.CompletedObjectives[0].ID != "realm.herb" {
		t.Fatalf("the completed objective must be the declared one: %#v", transition.Snapshot.CompletedObjectives)
	}
	// The tick is readable through the projection the panel uses.
	view, diagnostics := ProjectDungeonInstanceView(def, scope, transition.Snapshot)
	if len(diagnostics) != 0 {
		t.Fatalf("the transition must project cleanly: %#v", diagnostics)
	}
	if len(view.CompletedObjectives) != 1 || view.CompletedObjectives[0] != "realm.herb" {
		t.Fatalf("the projection must report the completion: %#v", view.CompletedObjectives)
	}
	if len(view.GrantedRewards) != 1 || view.GrantedRewards[0].ID != "realm.herb_ref" {
		t.Fatalf("the projection must report the grant: %#v", view.GrantedRewards)
	}
}

// TestAnUntiedRewardCompletesNothing keeps the rule from being applied to every
// reward: only a reward the definition ties to an objective completes one.
func TestAnUntiedRewardCompletesNothing(t *testing.T) {
	def, scope, snapshot := rewardTestEnter(t)
	// Drop the tie from the reward the character is standing on.
	for index := range def.ContentRefs {
		if def.ContentRefs[index].ID == "realm.herb_ref" {
			def.ContentRefs[index].ObjectiveID = ""
		}
	}
	// The hash changed, so the permit has to be rebuilt against the new definition.
	hash, err := DungeonDefinitionContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.DefinitionHash = hash
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, "realm.herb_ref", "", "", "realm.reward"))
	transition, _, err := GrantInstanceReward(def, scope, snapshot, ctx, RealmRewardRequest{
		ContentRefID: "realm.herb_ref", SourceRef: "realm.reward",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(transition.Snapshot.CompletedObjectives) != 0 {
		t.Fatalf("an untied reward must complete nothing: %#v", transition.Snapshot.CompletedObjectives)
	}
}

// TestARewardTiedToAnUndeclaredObjectiveIsRefused is the guard behind the tie: a
// definition cannot complete a goal it never declared.
func TestARewardTiedToAnUndeclaredObjectiveIsRefused(t *testing.T) {
	def, scope, snapshot := rewardTestEnter(t)
	for index := range def.ContentRefs {
		if def.ContentRefs[index].ID == "realm.herb_ref" {
			def.ContentRefs[index].ObjectiveID = "realm.not_declared"
		}
	}
	hash, err := DungeonDefinitionContentHash(def)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.DefinitionHash = hash
	ctx := testInstanceContext(snapshot, scope, testInstancePermit(ActionDiscover, "realm.herb_ref", "", "", "realm.reward"))
	_, _, err = GrantInstanceReward(def, scope, snapshot, ctx, RealmRewardRequest{
		ContentRefID: "realm.herb_ref", SourceRef: "realm.reward",
	})
	// The definition itself is refused before the action runs, which is the earlier
	// and better place to catch a goal that does not exist.
	if err == nil || !strings.Contains(err.Error(), "objective is not declared") {
		t.Fatalf("a reward tied to an undeclared objective must be refused: %v", err)
	}
}

// TestTheDefinitionRefusesTwoRewardsCompletingOneObjective keeps the panel's tick
// from depending on which reward was taken first.
func TestTheDefinitionRefusesTwoRewardsCompletingOneObjective(t *testing.T) {
	def := rewardTestDefinition()
	def.ContentRefs = append(def.ContentRefs, ContentRef{
		ID: "realm.second_ref", AreaID: "realm.herb_plot", Kind: "reward",
		ReferenceID: "pill.restore.health.lower", Quantity: 1, ObjectiveID: "realm.herb",
	})
	diagnostics := ValidateDungeonDefinition(def)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "duplicate_objective_completion" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("two rewards completing one objective must be refused: %#v", diagnostics)
	}
}

// TestTheDefinitionRefusesANonRewardCompletingAnObjective keeps a resource or an
// enemy from ticking a goal.
func TestTheDefinitionRefusesANonRewardCompletingAnObjective(t *testing.T) {
	def := rewardTestDefinition()
	def.ContentRefs = append(def.ContentRefs, ContentRef{
		ID: "realm.resource_ref", AreaID: "realm.herb_plot", Kind: "resource",
		ReferenceID: "pill.restore.health.lower", Quantity: 1, ObjectiveID: "realm.herb",
	})
	diagnostics := ValidateDungeonDefinition(def)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "content_completes_objective" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("only a reward may complete an objective: %#v", diagnostics)
	}
}
