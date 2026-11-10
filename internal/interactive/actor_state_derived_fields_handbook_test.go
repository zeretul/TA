package interactive

import (
	"strings"
	"testing"
)

// TestTheHandbookSaysWhichFieldsTheStageTableOwns is the G11 encounter's first
// blocker, stated at the place the model reads.
//
// A real-model run put the boss on the road and then failed to create him: the
// create's initial_state carried 攻击, 生命上限, 灵力上限, 神识, 身法 and 防御, the
// frozen-schema check refused those six as cultivation-owned, and because the
// refusal is per submission the whole state_changes module was rejected. The
// encounter happened in prose and no Actor existed, so nothing downstream —
// combat, loot, the story's own record of who was there — could happen at all.
//
// The handbook had said only "omit a field that has a default", which reads as
// advice about brevity rather than a rule about who owns the number. The six
// derived fields now say it themselves, and the create example says it too.
func TestTheHandbookSaysWhichFieldsTheStageTableOwns(t *testing.T) {
	system := xiuxianTestSystem()
	context := ActorStateRuntimeContext(system, map[string]any{}, StoryContextMaxBytes, 3)

	for _, fieldID := range []string{
		xiuxianFieldHealthMax, xiuxianFieldManaMax, xiuxianFieldAttack,
		xiuxianFieldDefense, xiuxianFieldAgility, xiuxianFieldPerception,
	} {
		block := actorRuntimeFieldBlock(t, context, fieldID)
		if !strings.Contains(block, "Derived from") {
			t.Fatalf("field %s does not say the stage table owns it:\n%s", fieldID, block)
		}
	}
	if !strings.Contains(context, "leave it out entirely") {
		t.Fatal("the create guidance does not warn that a derived field rejects the whole submission")
	}
}

// TestTheHandbookLeavesOtherGenresAlone keeps the new sentence inside the
// system that has the rule. A preset without a stage field derives nothing, so
// telling its model to omit these six would be an instruction about a mechanic
// the story does not have.
func TestTheHandbookLeavesOtherGenresAlone(t *testing.T) {
	system := defaultActorStateSystem()
	if ActorStateSystemIsXiuxian(system) {
		t.Fatal("the default preset is not supposed to carry cultivation rules")
	}
	context := ActorStateRuntimeContext(system, map[string]any{}, StoryContextMaxBytes, 3)
	if strings.Contains(context, "Derived from") || strings.Contains(context, "leave it out entirely") {
		t.Fatalf("a non-cultivation story was told about stage-derived fields:\n%s", context)
	}
}

// actorRuntimeFieldBlock returns the handbook text from a field's heading under
// the template section up to the next heading, so an assertion cannot pass on a
// sentence written under a different field — or on the `Current value` line,
// which names the same field higher up the page.
func actorRuntimeFieldBlock(t *testing.T, context, fieldID string) string {
	t.Helper()
	templates := strings.Index(context, "## Templates Available to New Actors")
	if templates < 0 {
		t.Fatalf("the handbook has no template section:\n%s", context)
	}
	section := context[templates:]
	marker := "#### " + fieldID
	start := strings.Index(section, marker)
	if start < 0 {
		t.Fatalf("the template section does not describe field %s", fieldID)
	}
	rest := section[start+len(marker):]
	if next := strings.Index(rest, "\n#### "); next >= 0 {
		return rest[:next]
	}
	return rest
}
