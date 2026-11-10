package quest

import "testing"

func TestExtendedAdventureChoicesKeepBoundedPreparation(t *testing.T) {
	d, ok := DefinitionByID("lanchuan-source-i04")
	if !ok {
		t.Fatal("missing authored choice")
	}
	c := d.Choices[0]
	for _, capability := range []int{14, 20, 44, 70, MaxChoiceCapability} {
		i := Instance{TaskID: d.ID, TaskVersion: d.Version, Status: StatusCompleted, CompletedObjectiveIDs: []string{c.ObjectiveID}, Choices: map[string]ChoiceResult{c.ObjectiveID: {ChoiceID: c.ID, Outcome: "success", Capability: capability, AtSeconds: c.DurationSeconds, EchoDueSeconds: c.DurationSeconds + c.EchoAfterSeconds}}}
		if diagnostics := ValidateInstance(d, i); len(diagnostics) > 0 {
			t.Fatalf("legal extended preparation %d rejected: %+v", capability, diagnostics)
		}
		result := i.Choices[c.ObjectiveID]
		result.Capability = MaxChoiceCapability + 1
		i.Choices[c.ObjectiveID] = result
		if diagnostics := ValidateInstance(d, i); len(diagnostics) == 0 {
			t.Fatal("unbounded preparation accepted")
		}
	}
	d.Choices[0].RequiredRank = MaxChoiceRank
	if diagnostics := ValidateDefinition(d); len(diagnostics) > 0 {
		t.Fatalf("supported high-stage requirement rejected: %+v", diagnostics)
	}
	d.Choices[0].RequiredRank++
	if diagnostics := ValidateDefinition(d); len(diagnostics) == 0 {
		t.Fatal("out-of-range rank requirement accepted")
	}
}
