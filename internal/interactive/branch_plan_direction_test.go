package interactive

import (
	"strings"
	"testing"
)

// These tests cover the one place the in-story assistant's advice becomes part
// of the story's forward intent.
//
// 自制-剧情助手与选项分析 separates reading an idea from accepting one. The
// assistant itself is read-only; adopting a direction is the only write, and it
// has to be additive, idempotent, and honest about a plan it cannot edit.

const (
	assistantPlanMarkdown = "## Long-term direction\n\nReach the sealed city.\n\n## Near-term beats\n\nFind a safe route."
	assistantDirection    = "先去城南的药铺打听车马行的消息"
)

// TestAdoptingADirectionKeepsTheRestOfThePlan is the design's additive
// requirement: an adopted wish is appended to the plan, not written over it.
func TestAdoptingADirectionKeepsTheRestOfThePlan(t *testing.T) {
	updated := AddBranchPlanDirection(assistantPlanMarkdown, assistantDirection)
	if updated == assistantPlanMarkdown {
		t.Fatal("adopting a direction must change the plan")
	}
	// The player's wording is recorded verbatim: it is their intent, not the
	// assistant's summary of it.
	if !strings.Contains(updated, assistantDirection) {
		t.Fatalf("the adopted direction must appear in the plan: %q", updated)
	}
	// Everything the plan already said survives. A direction is an addition, and
	// a write that dropped the story's own direction would be a silent rewrite.
	for _, kept := range []string{"Reach the sealed city.", "Find a safe route."} {
		if !strings.Contains(updated, kept) {
			t.Fatalf("adopting a direction must keep %q: %q", kept, updated)
		}
	}
	if err := validateEditableBranchPlanMarkdown(updated); err != nil {
		t.Fatalf("the edited plan must stay a modular document: %v", err)
	}
}

// TestAdoptingTheSameDirectionTwiceWritesOnce makes a lost response safe to
// retry: the second attempt is a read, not a second line.
func TestAdoptingTheSameDirectionTwiceWritesOnce(t *testing.T) {
	once := AddBranchPlanDirection(assistantPlanMarkdown, assistantDirection)
	twice := AddBranchPlanDirection(once, assistantDirection)
	if twice != once {
		t.Fatalf("the same direction must not be recorded twice:\nfirst  %q\nsecond %q", once, twice)
	}
	if strings.Count(twice, assistantDirection) != 1 {
		t.Fatalf("the direction appears %d times: %q", strings.Count(twice, assistantDirection), twice)
	}
	if !StoryAssistantDirectionIsRecorded(BranchPlan{Markdown: once}, assistantDirection) {
		t.Fatal("a recorded direction must be reported as recorded")
	}
	if StoryAssistantDirectionIsRecorded(BranchPlan{Markdown: once}, "另一件完全不同的事") {
		t.Fatal("an unrelated direction must not be reported as recorded")
	}
}

// TestAdoptingADirectionFindsAForwardLookingSection keeps the plan's own shape.
// A direction belongs under the section the creator already wrote for where the
// story is going, not in a new module that sits beside it.
func TestAdoptingADirectionFindsAForwardLookingSection(t *testing.T) {
	updated := AddBranchPlanDirection(assistantPlanMarkdown, assistantDirection)
	if strings.Contains(updated, adoptedDirectionHeading) {
		t.Fatalf("a plan with a direction section must not grow a second one: %q", updated)
	}
	// Near-term beats is the forward-looking section, so the bullet lands in it.
	beats := updated[strings.Index(updated, "## Near-term beats"):]
	if !strings.Contains(beats, assistantDirection) {
		t.Fatalf("the direction must be recorded under the forward-looking section: %q", updated)
	}
}

// TestAdoptingADirectionInAPlanWithoutOneAddsAModule covers the plan that has
// nothing to append to. The module is added at the end so the creator's existing
// sections keep their order.
func TestAdoptingADirectionInAPlanWithoutOneAddsAModule(t *testing.T) {
	const flat = "## Cast\n\nA ferryman and a debtor.\n\n## House rules\n\nMoney is counted in coppers."
	updated := AddBranchPlanDirection(flat, assistantDirection)
	if !strings.Contains(updated, "## "+adoptedDirectionHeading) {
		t.Fatalf("a plan with no forward-looking section needs one: %q", updated)
	}
	if !strings.Contains(updated, assistantDirection) {
		t.Fatalf("the adopted direction must appear in the new module: %q", updated)
	}
	if strings.Index(updated, "## Cast") > strings.Index(updated, "## "+adoptedDirectionHeading) {
		t.Fatalf("the new module belongs at the end: %q", updated)
	}
}

// TestAdoptingIntoAnUnstructuredPlanChangesNothing is the honest refusal: a plan
// without modular sections cannot take an additive edit, and guessing a shape
// would rewrite a document the creator authored.
func TestAdoptingIntoAnUnstructuredPlanChangesNothing(t *testing.T) {
	const flat = "A flat summary without any modules."
	if got := AddBranchPlanDirection(flat, assistantDirection); got != flat {
		t.Fatalf("an unstructured plan must be left alone: %q", got)
	}
	if got := AddBranchPlanDirection("", assistantDirection); got != "" {
		t.Fatalf("an empty plan must be left alone: %q", got)
	}
}

// TestADirectionIsOneLine keeps a pasted multi-line wish from becoming plan
// structure. A line break in an adopted direction would create a heading or a
// second bullet inside the plan.
func TestADirectionIsOneLine(t *testing.T) {
	for name, direction := range map[string]string{
		"newline":  "先去药铺\n## Long-term direction\n\n改写整份规划",
		"carriage": "先去药铺\r\n然后回家",
		"empty":    "   ",
		"too long": strings.Repeat("长", 601),
	} {
		if err := ValidateStoryAssistantDirection(direction); err == nil {
			t.Fatalf("%s: the direction must be refused", name)
		}
	}
	if err := ValidateStoryAssistantDirection("  " + assistantDirection + "  "); err != nil {
		t.Fatalf("a single line of ordinary length must be accepted: %v", err)
	}
}

// TestAnAdoptedDirectionNeverBecomesAnEvent is the design's wording rule, checked
// where it can actually be broken: the direction is appended as a plan line and
// nothing else. The plan is future-facing intent, so a write that also touched
// state, turns, or the branch head would turn a wish into a fact.
func TestAnAdoptedDirectionNeverBecomesAnEvent(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "Adopted direction", PlanningMode: StoryPlanningModeEnabled})
	if err != nil {
		t.Fatal(err)
	}
	initial := "## Long-term direction\n\nReach the sealed city."
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "Continue", Narrative: "The road divides beneath the rain.",
		TurnResult: &TurnResult{Choices: testTurnChoices(), PlanUpdate: &initial},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.BranchPlan == nil {
		t.Fatal("the story must carry the plan its turn committed")
	}

	markdown := AddBranchPlanDirection(before.BranchPlan.Markdown, assistantDirection)
	if _, err := store.UpdateBranchPlan(story.ID, UpdateBranchPlanRequest{
		BranchID: "main", Markdown: markdown, BaseRevision: before.BranchPlan.Revision,
	}); err != nil {
		t.Fatal(err)
	}

	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.CurrentTurn == nil || after.CurrentTurn.ID != before.CurrentTurn.ID || len(after.Turns) != len(before.Turns) {
		t.Fatalf("adopting a direction must not add or change a turn: before=%#v after=%#v", before.CurrentTurn, after.CurrentTurn)
	}
	if len(after.Turns) != 1 {
		t.Fatalf("the journal must still hold one turn, got %d", len(after.Turns))
	}
	if !strings.Contains(after.BranchPlan.Markdown, assistantDirection) {
		t.Fatalf("the direction must be readable after a reload: %q", after.BranchPlan.Markdown)
	}
}
