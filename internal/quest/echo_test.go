package quest

import (
	"encoding/json"
	"math"
	"testing"
)

func TestCompletionEchoRejectsOverflowAndPrematureAcknowledgement(t *testing.T) {
	for _, e := range []EchoResult{{AtSeconds: math.MaxInt64 - 5, DueSeconds: math.MinInt64 + 4}, {AtSeconds: 10, DueSeconds: 20, AcknowledgedBy: "npc", AcknowledgedAtSeconds: 19}, {AtSeconds: 10, DueSeconds: 20, AcknowledgedBy: "npc", AcknowledgedAtSeconds: 20}} {
		if e.Valid(10) {
			t.Fatal("invalid maturity was accepted")
		}
	}
	if !(EchoResult{AtSeconds: 10, DueSeconds: 20, Resolved: true, AcknowledgedBy: "npc", AcknowledgedAtSeconds: 20}).Valid(10) {
		t.Fatal("actual mature acknowledgement rejected")
	}
}

func TestSavedChoiceKeepsItsOriginalDueInstantAfterContentTuning(t *testing.T) {
	d, _ := DefinitionByID("xinghai-a01")
	c := d.Choices[2]
	// Earlier release receipts waited six hours. New content may use a full
	// audit day, but loading the old journal must not rewrite or reject it.
	saved := Instance{TaskID: d.ID, TaskVersion: d.Version, Status: StatusCompleted, CompletedObjectiveIDs: []string{c.ObjectiveID}, Choices: map[string]ChoiceResult{c.ObjectiveID: {ChoiceID: c.ID, Outcome: "success", AtSeconds: 100, EchoDueSeconds: 21700}}}
	if diagnostics := ValidateInstance(d, saved); len(diagnostics) != 0 {
		t.Fatalf("old legitimate receipt rejected: %+v", diagnostics)
	}
	r := saved.Choices[c.ObjectiveID]
	r.EchoDueSeconds = 99
	saved.Choices[c.ObjectiveID] = r
	if len(ValidateInstance(d, saved)) == 0 {
		t.Fatal("backdated due instant accepted")
	}
}

func TestVisibleChoicesHideUnchosenAndFutureOutcomes(t *testing.T) {
	d, _ := DefinitionByID("xinghai-a01")
	original, _ := json.Marshal(d)
	c := d.Choices[0]
	results := map[string]ChoiceResult{c.ObjectiveID: {ChoiceID: c.ID, Outcome: "success", EchoResolved: true, EchoEncounterRequired: true}}
	for _, test := range []struct {
		name  string
		known map[string]ChoiceResult
	}{{"available", nil}, {"unheard", results}} {
		t.Run(test.name, func(t *testing.T) {
			for n, choice := range VisibleChoices(d, test.known) {
				if choice.SuccessReaction != nil || choice.FailureReaction != nil || choice.SuccessEcho != "" || choice.FailureEcho != "" || choice.Failure != "" {
					t.Fatal("future or alternate outcome leaked")
				}
				if (n != 0 || test.known == nil) && choice.Success != "" {
					t.Fatal("unperformed method disclosed its outcome")
				}
			}
		})
	}
	r := results[c.ObjectiveID]
	r.EchoAcknowledgedBy = "actual-person"
	results[c.ObjectiveID] = r
	known := VisibleChoices(d, results)
	if known[0].SuccessEcho == "" || known[1].SuccessEcho != "" {
		t.Fatal("actual response mixed with an unchosen method")
	}
	// Receipts written before the encounter rule retain the already public echo.
	r.EchoEncounterRequired = false
	r.EchoAcknowledgedBy = ""
	results[c.ObjectiveID] = r
	if VisibleChoices(d, results)[0].SuccessEcho == "" {
		t.Fatal("legacy known response was hidden")
	}
	after, _ := json.Marshal(d)
	if string(original) != string(after) {
		t.Fatal("display redaction mutated author definitions")
	}
}
