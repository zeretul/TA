package interactive

import (
	"strings"
	"testing"
)

// The opening purse was lost to wording, not to a missing rule: the field's
// instruction said the rules own 灵石, the create example says to omit a field
// that has a default, and the model read both as "do not write it here". The
// create branch always accepted the write; nothing told the model it should.
//
// These assertions pin the two sentences that close that gap, so a later edit to
// the prompt cannot quietly reopen it.

func TestTheCreateGuidanceTellsTheModelToStateTheOpeningPurse(t *testing.T) {
	system := xiuxianTestSystem()
	text := actorStateRuntimeSubmissionTemplate(system, map[string]any{}, nil, 5)

	for _, phrase := range []string{
		"灵石 is settled by the trade rules after the opening",
		"write that exact count into `initial_state` at create",
		"the only turn that may set it",
		"Never invent an amount the scene did not state",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("create guidance is missing %q", phrase)
		}
	}
}

func TestTheCurrencyFieldSaysTheOpeningIsTheOneChanceToStateIt(t *testing.T) {
	system := xiuxianTestSystem()
	text := actorStateRuntimeTemplateMarkdown(actorStateFieldTemplateForTest(t, system, ActorStateImportantCharacterTemplateID))

	for _, phrase := range []string{
		"就在 create 的 initial_state 里按实际枚数一次写足",
		"默认的 0 表示确实一文没有",
		"create 之后本项归规则",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("the purse field's instruction is missing %q", phrase)
		}
	}
}

func actorStateFieldTemplateForTest(t *testing.T, system StoryDirectorActorStateSystem, templateID string) ActorStateTemplate {
	t.Helper()
	template := actorStateTemplateByID(system, templateID)
	if template.ID == "" {
		t.Fatalf("template %s is not in the system", templateID)
	}
	return template
}
