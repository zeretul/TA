package interactive

import (
	"strings"
	"testing"
)

// The user-reported STY-09 defect is "dialogue disappears after switching
// style". Two mechanisms could produce it and they are not the same defect:
// history could be lost or hidden, or a new narrative could omit the answer
// that was actually spoken. These tests pin the part that is the product's
// responsibility — a style change is a settings change and must leave every
// committed fact, including character speech, exactly where it was.

// A style switch must not rewrite or hide what has already happened.
func TestChangingTheStylePreservesCommittedHistory(t *testing.T) {
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })

	story, err := store.CreateStory(CreateStoryRequest{
		Title: "style switch", StoryTellerID: "source-drama",
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}

	const spoken = "「这枚玉不是你的。」掌柜把算盘一推，声音不高。"
	const narrative = "你把旧玉按回怀里。" + spoken
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "把玉收好", Narrative: narrative,
	}); err != nil {
		t.Fatalf("append turn: %v", err)
	}

	nextMain := "strange-tales"
	nextDialogue := "flippant-banter"
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{
		StylePurposes: &StoryStylePurposesUpdate{Main: &nextMain, Dialogue: &nextDialogue},
	}); err != nil {
		t.Fatalf("switch style: %v", err)
	}

	meta, _, err := store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatalf("read story after switch: %v", err)
	}
	if meta.StylePurposes.Main != nextMain || meta.StylePurposes.Dialogue != nextDialogue {
		t.Fatalf("the switch did not persist: %#v", meta.StylePurposes)
	}

	// Read the turn back through the projection the story stage itself uses,
	// not the raw journal: if the defect were a read-time filter it would show
	// up here and nowhere else.
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("snapshot after switch: %v", err)
	}
	if len(snapshot.Turns) != 1 {
		t.Fatalf("turns readable after the switch = %d, want 1", len(snapshot.Turns))
	}
	if got := snapshot.Turns[0].Narrative; got != narrative {
		t.Fatalf("the committed narrative changed across a style switch:\n got %q\nwant %q", got, narrative)
	}
	if !strings.Contains(snapshot.Turns[0].Narrative, spoken) {
		t.Fatalf("the character's committed line was lost across a style switch: %q", snapshot.Turns[0].Narrative)
	}
}

// A style switch must survive a restart with its history and its other two
// purposes intact. This is the reopen path a player actually takes.
func TestChangingTheStyleSurvivesReopenWithItsHistory(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{
		Title: "reopen", StoryTellerID: "source-drama",
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}
	const narrative = "「往东走三里，见着石桥就拐。」老者抬手指了指。"
	if _, err := store.AppendTurn(story.ID, AppendTurnRequest{
		BranchID: "main", User: "问路", Narrative: narrative,
	}); err != nil {
		t.Fatalf("append turn: %v", err)
	}
	combat := "hotblooded-combat"
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{
		StylePurposes: &StoryStylePurposesUpdate{Combat: &combat},
	}); err != nil {
		t.Fatalf("switch combat style: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened := NewStore(dir)
	t.Cleanup(func() { _ = reopened.Close() })
	meta, _, err := reopened.readStoryLocked(story.ID)
	if err != nil {
		t.Fatalf("reopen story: %v", err)
	}
	if meta.StylePurposes.Combat != combat {
		t.Fatalf("the combat style was lost on reopen: %#v", meta.StylePurposes)
	}
	// Changing one purpose must not reset the others. Dialogue was never named,
	// so it must still inherit rather than have been quietly overwritten.
	if !meta.StylePurposes.Inherits(PurposeDialogue) {
		t.Fatalf("an untouched dialogue purpose came back as %q", meta.StylePurposes.Dialogue)
	}
	if got := meta.StylePurposes.ResolvedStyleID(PurposeMain, meta.StoryTellerID); got != "source-drama" {
		t.Fatalf("reopening moved the main style to %q", got)
	}

	snapshot, err := reopened.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatalf("snapshot after reopen: %v", err)
	}
	if len(snapshot.Turns) != 1 || !strings.Contains(snapshot.Turns[0].Narrative, "往东走三里") {
		t.Fatalf("the committed history did not survive the restart: %#v", snapshot.Turns)
	}
}
