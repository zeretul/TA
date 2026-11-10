package publiquevent

import (
	"errors"
	"strings"
	"testing"
)

func authoredTide(t *testing.T) Definition {
	t.Helper()
	definition, ok := DefinitionByID(PublicEventIDTaiyueTide)
	if !ok {
		t.Fatalf("the authored event %s is not registered", PublicEventIDTaiyueTide)
	}
	return definition
}

func TestAuthoredDefinitionValidates(t *testing.T) {
	for _, definition := range Definitions() {
		if diagnostics := ValidateDefinition(definition); len(diagnostics) > 0 {
			t.Fatalf("%s is invalid: %+v", definition.ID, diagnostics[0])
		}
	}
}

// A stage is a reading of committed time, so the same instant must always answer
// the same stage. This is what makes a replayed journal and a live branch agree.
func TestStageAtIsPure(t *testing.T) {
	definition := authoredTide(t)
	cases := []struct {
		seconds int64
		stage   string
		ok      bool
	}{
		{0, "taiyue.rumor", true},
		{6*3600 - 1, "taiyue.rumor", true},
		{6 * 3600, "taiyue.rush", true},
		{48*3600 - 1, "taiyue.rush", true},
		{48 * 3600, "taiyue.surge", true},
		{5*24*3600 - 1, "taiyue.surge", true},
		{5 * 24 * 3600, "taiyue.settle", true},
		{30 * 24 * 3600, "taiyue.settle", true},
	}
	for _, tc := range cases {
		stage, status, ok := definition.StageAt(tc.seconds)
		if ok != tc.ok {
			t.Fatalf("%d seconds: ok=%v want %v", tc.seconds, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if status != StageCurrent {
			t.Fatalf("%d seconds: status %q", tc.seconds, status)
		}
		if stage.ID != tc.stage {
			t.Fatalf("%d seconds: stage %s want %s", tc.seconds, stage.ID, tc.stage)
		}
		// Calling it twice must not drift, and must not depend on order.
		again, _, _ := definition.StageAt(tc.seconds)
		if again.ID != stage.ID {
			t.Fatalf("%d seconds: second reading gave %s", tc.seconds, again.ID)
		}
	}
}

// Before the first stage opens the event is not public. It must not be reported
// as a current stage, or a candidate would read as already happening.
func TestEventBeforeFirstStageIsNotPublic(t *testing.T) {
	definition := Definition{
		SchemaVersion: SchemaVersion, ID: "evt.test", Version: "1.0",
		Title: "t", Brief: "b", Region: "r",
		Stages:  []Stage{{ID: "s1", Title: "t", Summary: "s", OpenAfterSeconds: 100}},
		Actions: []Action{{Kind: ActionObserve, Label: "l", Brief: "b"}},
	}
	if _, _, ok := definition.StageAt(99); ok {
		t.Fatal("an event that has not opened reported a stage")
	}
	if definition.Closed(0) {
		t.Fatal("an event that has not opened reported itself closed")
	}
}

func TestStageStatusesCoverTheLadder(t *testing.T) {
	definition := authoredTide(t)
	statuses := definition.StageStatuses(6 * 3600)
	if len(statuses) != len(definition.Stages) {
		t.Fatalf("got %d statuses for %d stages", len(statuses), len(definition.Stages))
	}
	want := []StageStatus{StagePast, StageCurrent, StageAhead, StageAhead}
	for index, status := range statuses {
		if status != want[index] {
			t.Fatalf("stage %d is %q want %q", index, status, want[index])
		}
	}
}

// The settling stage is what closes the event. A closed event cannot be joined,
// which is how a late character is told the rush is over rather than being
// handed a participation that never happened.
func TestJoinRefusesAClosedEvent(t *testing.T) {
	definition := authoredTide(t)
	settle := definition.Stages[len(definition.Stages)-1].OpenAfterSeconds
	if !definition.Closed(settle) {
		t.Fatal("the authored event does not close at its final stage")
	}
	if _, err := Join(definition, nil, settle, ActionHelp); !errors.Is(err, ErrEventClosed) {
		t.Fatalf("joining a closed event gave %v", err)
	}
}

func TestJoinRecordsTheStageThatWasCurrent(t *testing.T) {
	definition := authoredTide(t)
	instance, err := Join(definition, nil, 7*3600, ActionInvestigate)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if instance.JoinedStage != "taiyue.rush" {
		t.Fatalf("joined stage %s", instance.JoinedStage)
	}
	if instance.JoinedAtSeconds != 7*3600 {
		t.Fatalf("joined at %d", instance.JoinedAtSeconds)
	}
	if instance.Action != ActionInvestigate {
		t.Fatalf("action %s", instance.Action)
	}
}

func TestJoinRefusesASecondParticipation(t *testing.T) {
	definition := authoredTide(t)
	first, err := Join(definition, nil, 3600, ActionObserve)
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	if _, err := Join(definition, &first, 3600, ActionHelp); !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("second join gave %v", err)
	}
}

func TestJoinRefusesAnUnsupportedAction(t *testing.T) {
	definition := Definition{
		SchemaVersion: SchemaVersion, ID: "evt.test", Version: "1.0",
		Title: "t", Brief: "b", Region: "r",
		Stages:  []Stage{{ID: "s1", Title: "t", Summary: "s"}},
		Actions: []Action{{Kind: ActionObserve, Label: "l", Brief: "b"}},
	}
	if _, err := Join(definition, nil, 0, ActionHelp); !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("unsupported action gave %v", err)
	}
}

// §6: the panel must never receive the secret lines, so the only function that
// assembles what a player may see must not return them.
func TestKnownLinesNeverLeakSecrets(t *testing.T) {
	definition := authoredTide(t)
	for _, stage := range definition.Stages {
		for _, joined := range []bool{false, true} {
			lines := KnownLines(stage, joined)
			if len(lines) == 0 || lines[0] != stage.Summary {
				t.Fatalf("stage %s: public summary is not the first line", stage.ID)
			}
			for _, line := range lines {
				for _, secret := range stage.SecretLines {
					if line == secret {
						t.Fatalf("stage %s leaked a secret line", stage.ID)
					}
				}
			}
		}
	}
}

func TestParticipantLinesNeedParticipation(t *testing.T) {
	definition := authoredTide(t)
	stage := definition.Stages[1]
	if got := len(KnownLines(stage, false)); got != 1 {
		t.Fatalf("an outsider saw %d lines, want only the public summary", got)
	}
	if got := len(KnownLines(stage, true)); got != 1+len(stage.ParticipantLines) {
		t.Fatalf("a participant saw %d lines", got)
	}
}

// A character who joined during the rush keeps what they were there for. They do
// not retroactively learn the next stage's participant lines.
func TestJoinedStageLinesStayAtTheJoinedStage(t *testing.T) {
	definition := authoredTide(t)
	instance, err := Join(definition, nil, 6*3600, ActionHelp)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	rush, _ := definition.StageByID("taiyue.rush")
	lines := JoinedStageLines(definition, instance)
	if len(lines) != 1+len(rush.ParticipantLines) {
		t.Fatalf("joined stage lines = %d", len(lines))
	}
	surge, _ := definition.StageByID("taiyue.surge")
	for _, secret := range surge.ParticipantLines {
		for _, line := range lines {
			if line == secret {
				t.Fatalf("a rush participant learned a surge line: %s", line)
			}
		}
	}
}

func TestValidateInstanceRejectsAVersionMismatch(t *testing.T) {
	definition := authoredTide(t)
	instance, err := Join(definition, nil, 3600, ActionObserve)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	instance.EventVersion = "2.0"
	diagnostics := ValidateInstance(definition, instance)
	if len(diagnostics) == 0 || diagnostics[0].Code != "mismatched_version" {
		t.Fatalf("version mismatch was accepted: %+v", diagnostics)
	}
}

func TestWithdrawKeepsTheParticipation(t *testing.T) {
	definition := authoredTide(t)
	instance, err := Join(definition, nil, 3600, ActionHelp)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	withdrawn, err := Withdraw(instance)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if !withdrawn.Withdrawn || withdrawn.JoinedStage != instance.JoinedStage {
		t.Fatalf("withdrawing lost the participation: %+v", withdrawn)
	}
	if _, err := Withdraw(withdrawn); !errors.Is(err, ErrAlreadyJoined) {
		t.Fatalf("second withdraw gave %v", err)
	}
}

func TestValidateDefinitionRejectsUnorderedStages(t *testing.T) {
	definition := Definition{
		SchemaVersion: SchemaVersion, ID: "evt.test", Version: "1.0",
		Title: "t", Brief: "b", Region: "r",
		Stages: []Stage{
			{ID: "s1", Title: "t", Summary: "s", OpenAfterSeconds: 100},
			{ID: "s2", Title: "t", Summary: "s", OpenAfterSeconds: 50},
		},
		Actions: []Action{{Kind: ActionObserve, Label: "l", Brief: "b"}},
	}
	diagnostics := ValidateDefinition(definition)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "unordered_stage" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unordered stages were accepted: %+v", diagnostics)
	}
}

func TestValidateDefinitionRejectsAFinalStageInTheMiddle(t *testing.T) {
	definition := Definition{
		SchemaVersion: SchemaVersion, ID: "evt.test", Version: "1.0",
		Title: "t", Brief: "b", Region: "r",
		Stages: []Stage{
			{ID: "s1", Title: "t", Summary: "s", Final: true},
			{ID: "s2", Title: "t", Summary: "s", OpenAfterSeconds: 10},
		},
		Actions: []Action{{Kind: ActionObserve, Label: "l", Brief: "b"}},
	}
	diagnostics := ValidateDefinition(definition)
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "final_stage_not_last" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a mid-ladder final stage was accepted: %+v", diagnostics)
	}
}

func TestValidateDefinitionRequiresAWordingForEveryAction(t *testing.T) {
	definition := Definition{
		SchemaVersion: SchemaVersion, ID: "evt.test", Version: "1.0",
		Title: "t", Brief: "b", Region: "r",
		Stages:  []Stage{{ID: "s1", Title: "t", Summary: "s"}},
		Actions: []Action{{Kind: ActionHelp, Label: " ", Brief: "b"}},
	}
	diagnostics := ValidateDefinition(definition)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Path, "label") {
		t.Fatalf("an action with no wording was accepted: %+v", diagnostics)
	}
}

// Standing aside is a real choice, so the authored event must offer it, and it
// must be the one action that pays nothing. Otherwise the "do not get involved"
// option would be a lie about what it costs.
func TestStandingAsideIsOfferedAndPaysNothing(t *testing.T) {
	definition := authoredTide(t)
	action, ok := definition.ActionByKind(ActionWithdraw)
	if !ok {
		t.Fatal("the authored event does not offer standing aside")
	}
	if action.Reward != nil {
		t.Fatalf("standing aside pays %+v", action.Reward)
	}
	if len(definition.Actions) < 3 {
		t.Fatalf("the authored event offers only %d ways to take part", len(definition.Actions))
	}
}
