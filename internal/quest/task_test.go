package quest

import (
	"testing"
)

// These tests cover the rule that makes a task a fact rather than a promise: an
// objective is satisfied only by the committed thing it names, and a settled task
// is settled for good.

// TestTheAuthoredTasksAreValid keeps a registered task from being unwinnable.
func TestTheAuthoredTasksAreValid(t *testing.T) {
	for _, definition := range Definitions() {
		if diagnostics := ValidateDefinition(definition); len(diagnostics) != 0 {
			t.Fatalf("%s must validate: %#v", definition.ID, diagnostics)
		}
	}
}

// TestAPersonalTaskMustNameItsPerson keeps an errand from wearing the personal
// label.
func TestAPersonalTaskMustNameItsPerson(t *testing.T) {
	definition, ok := DefinitionByID(TaskSuQianMaterials)
	if !ok {
		t.Fatal("the personal task must be registered")
	}
	definition.Giver = ""
	diagnostics := ValidateDefinition(definition)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "missing_giver" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a personal task without a person must be refused: %#v", diagnostics)
	}
}

// TestAnObjectiveMustNameAFact keeps a goal nobody can reach out of the register.
func TestAnObjectiveMustNameAFact(t *testing.T) {
	definition, ok := DefinitionByID(TaskHerbHandover)
	if !ok {
		t.Fatal("the errand must be registered")
	}
	definition.Objectives[0].Evidence = nil
	diagnostics := ValidateDefinition(definition)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "missing_evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("an objective with no fact must be refused: %#v", diagnostics)
	}
}

// TestAFactSatisfiesOnlyTheObjectiveItNames is the whole rule: no fuzzy match, no
// near-enough reference.
func TestAFactSatisfiesOnlyTheObjectiveItNames(t *testing.T) {
	definition, _ := DefinitionByID(TaskSuQianMaterials)

	matched := definition.SatisfiedBy(Fact{Kind: EvidenceJourneyArrival, RefID: "yue.market"})
	if len(matched) != 1 || matched[0] != "quest.su_qian_materials.source" {
		t.Fatalf("the arrival must satisfy its own objective: %#v", matched)
	}
	if other := definition.SatisfiedBy(Fact{Kind: EvidenceJourneyArrival, RefID: "yue.market.outskirts"}); len(other) != 0 {
		t.Fatalf("a different place must satisfy nothing: %#v", other)
	}
	if other := definition.SatisfiedBy(Fact{Kind: EvidenceRealmReward, RefID: "yue.market"}); len(other) != 0 {
		t.Fatalf("a fact of another kind must satisfy nothing: %#v", other)
	}
}

// TestOnlyAnAcceptedTaskAdvances is §4.4's boundary in the rules layer.
func TestOnlyAnAcceptedTaskAdvances(t *testing.T) {
	definition, _ := DefinitionByID(TaskHerbHandover)
	fact := Fact{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}

	offered := Instance{TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusOffered}
	advanced, added := offered.ApplyFact(definition, fact, "realm.reward", 0)
	if len(added) != 0 || advanced.Status != StatusOffered {
		t.Fatalf("an offer must not advance on its own: %#v %#v", advanced, added)
	}

	accepted := Instance{TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusAccepted}
	advanced, added = accepted.ApplyFact(definition, fact, "realm.reward", 0)
	if len(added) != 1 || advanced.Status != StatusCompleted {
		t.Fatalf("an accepted task must advance to completed: %#v %#v", advanced, added)
	}
	if advanced.SourceRefs["quest.herb_handover.pill"] != "realm.reward" {
		t.Fatalf("the tick must record what caused it: %#v", advanced.SourceRefs)
	}
}

// TestTheSameFactDoesNotTickTwice keeps a replay from counting one deed twice.
func TestTheSameFactDoesNotTickTwice(t *testing.T) {
	definition, _ := DefinitionByID(TaskSuQianMaterials)
	fact := Fact{Kind: EvidenceRealmReward, RefID: "realm.herb_ref"}
	instance := Instance{TaskID: definition.ID, TaskVersion: definition.Version, Status: StatusAccepted}

	instance, first := instance.ApplyFact(definition, fact, "realm.reward", 0)
	if len(first) != 1 {
		t.Fatalf("the first fact must tick: %#v", first)
	}
	instance, second := instance.ApplyFact(definition, fact, "realm.reward", 0)
	if len(second) != 0 {
		t.Fatalf("the same fact must not tick again: %#v", second)
	}
}

// TestATaskCompletesOnlyWhenEveryObjectiveIsDone keeps "partial" an honest state.
func TestATaskCompletesOnlyWhenEveryObjectiveIsDone(t *testing.T) {
	definition, _ := DefinitionByID(TaskSuQianMaterials)
	if definition.IsComplete(nil) {
		t.Fatal("an untouched task must not read as complete")
	}
	if definition.IsComplete([]string{"quest.su_qian_materials.source"}) {
		t.Fatal("one of two objectives must not read as complete")
	}
	missing := definition.MissingObjectives([]string{"quest.su_qian_materials.source"})
	if len(missing) != 1 || missing[0] != "quest.su_qian_materials.check" {
		t.Fatalf("the open objective must be named: %#v", missing)
	}
	if !definition.IsComplete([]string{
		"quest.su_qian_materials.source", "quest.su_qian_materials.check",
	}) {
		t.Fatal("both objectives must read as complete")
	}
}

// TestASettledInstanceCannotClaimAnObjectiveItNeverHad is the guard behind a
// stored row: a payment on work the task never declared is refused.
func TestASettledInstanceCannotClaimAnObjectiveItNeverHad(t *testing.T) {
	definition, _ := DefinitionByID(TaskHerbHandover)
	instance := Instance{
		TaskID: definition.ID, TaskVersion: definition.Version,
		Status: StatusCompleted, SettledRewards: true,
		CompletedObjectiveIDs: []string{"quest.not_declared"},
	}
	diagnostics := ValidateInstance(definition, instance)
	codes := map[string]bool{}
	for _, diagnostic := range diagnostics {
		codes[diagnostic.Code] = true
	}
	if !codes["unknown_objective"] || !codes["incomplete_completion"] {
		t.Fatalf("an invented objective must be refused: %#v", diagnostics)
	}
}

// TestAnInstanceBoundToAnotherVersionIsRefused keeps a changed definition from
// silently re-reading an old instance.
func TestAnInstanceBoundToAnotherVersionIsRefused(t *testing.T) {
	definition, _ := DefinitionByID(TaskHerbHandover)
	instance := Instance{TaskID: definition.ID, TaskVersion: "0.9", Status: StatusOffered}
	diagnostics := ValidateInstance(definition, instance)
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "version_mismatch" {
			return
		}
	}
	t.Fatalf("a version mismatch must be refused: %#v", diagnostics)
}

// TestTheRegisterReturnsCopies keeps a caller from rewriting an authored task.
func TestTheRegisterReturnsCopies(t *testing.T) {
	definition, _ := DefinitionByID(TaskHerbHandover)
	definition.Objectives[0].Description = "changed"
	definition.Rewards[0].Quantity = 999
	again, _ := DefinitionByID(TaskHerbHandover)
	if again.Objectives[0].Description == "changed" || again.Rewards[0].Quantity == 999 {
		t.Fatal("the register must not expose its own definition")
	}
}

// TestAnAbandonedTaskCannotHavePaid keeps the two terminal states apart.
func TestAnAbandonedTaskCannotHavePaid(t *testing.T) {
	definition, _ := DefinitionByID(TaskHerbHandover)
	instance := Instance{
		TaskID: definition.ID, TaskVersion: definition.Version,
		Status: StatusAbandoned, SettledRewards: true,
		CompletedObjectiveIDs: []string{"quest.herb_handover.pill"},
	}
	for _, diagnostic := range ValidateInstance(definition, instance) {
		if diagnostic.Code == "settled_abandoned" {
			return
		}
	}
	t.Fatalf("an abandoned task cannot have paid: %#v", ValidateInstance(definition, instance))
}
