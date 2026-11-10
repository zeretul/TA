package interactive

import "testing"

// The three purposes are one expression configuration, not three world
// execution chains. These tests pin the resolution rules the design fixes:
// an inheriting purpose follows the main narrative, an empty main falls back to
// the story's own style, and changing one purpose leaves the other two alone.

func TestStylePurposesResolveEachPurposeIndependently(t *testing.T) {
	// A story running a serious main narrative with comic dialogue and a
	// hot-blooded combat voice is the combination the design names explicitly.
	purposes := StoryStylePurposes{
		Main:     "source-drama",
		Dialogue: "flippant-banter",
		Combat:   "hotblooded-combat",
	}
	if got := purposes.ResolvedStyleID(PurposeMain, "rhythm"); got != "source-drama" {
		t.Fatalf("main resolved to %q", got)
	}
	if got := purposes.ResolvedStyleID(PurposeDialogue, "rhythm"); got != "flippant-banter" {
		t.Fatalf("dialogue resolved to %q", got)
	}
	if got := purposes.ResolvedStyleID(PurposeCombat, "rhythm"); got != "hotblooded-combat" {
		t.Fatalf("combat resolved to %q", got)
	}
}

// An empty dialogue or combat means "follow the main narrative". Storing the
// main id as a copy would break the moment the main style changed, so the
// resolution has to follow the current main value rather than a frozen one.
func TestStylePurposesInheritAndFollowLaterMainChanges(t *testing.T) {
	inheriting := StoryStylePurposes{Main: "source-drama"}
	if !inheriting.Inherits(PurposeDialogue) || !inheriting.Inherits(PurposeCombat) {
		t.Fatal("empty dialogue and combat did not report as inheriting")
	}
	if got := inheriting.ResolvedStyleID(PurposeDialogue, "rhythm"); got != "source-drama" {
		t.Fatalf("inheriting dialogue resolved to %q, want the main style", got)
	}

	// Changing the main style must carry the inheriting purposes with it.
	changed := StoryStylePurposes{Main: "strange-tales"}
	if got := changed.ResolvedStyleID(PurposeCombat, "rhythm"); got != "strange-tales" {
		t.Fatalf("inheriting combat resolved to %q after the main style changed, want strange-tales", got)
	}
}

// A story that never configured purposes still resolves. Its own stored style
// is what the main narrative uses, so an existing story keeps the style it was
// created under rather than silently moving to the default.
func TestStylePurposesFallBackToTheStoriesOwnStyle(t *testing.T) {
	var unset StoryStylePurposes
	if got := unset.ResolvedStyleID(PurposeMain, "grimdark"); got != "grimdark" {
		t.Fatalf("unset main resolved to %q, want the story's own style", got)
	}
	if got := unset.ResolvedStyleID(PurposeDialogue, "grimdark"); got != "grimdark" {
		t.Fatalf("unset dialogue resolved to %q, want the story's own style", got)
	}
	// A purpose with neither its own style nor a story style still resolves to
	// something; the caller reports the missing preset rather than crashing.
	if got := unset.ResolvedStyleID(PurposeCombat, ""); got != "" {
		t.Fatalf("combat resolved to %q with nothing configured anywhere", got)
	}
}

func TestStylePurposesNormalizeTrimsButKeepsEmpty(t *testing.T) {
	normalized := StoryStylePurposes{Main: "  source-drama  ", Dialogue: "   ", Combat: ""}.Normalize()
	if normalized.Main != "source-drama" {
		t.Fatalf("main normalized to %q", normalized.Main)
	}
	// Whitespace is not a selection. It has to normalize to empty so the purpose
	// reads as inheriting rather than naming a style whose id is a space.
	if normalized.Dialogue != "" || normalized.Combat != "" {
		t.Fatalf("whitespace survived normalization: %#v", normalized)
	}
	if !normalized.Inherits(PurposeDialogue) {
		t.Fatal("a whitespace-only dialogue did not normalize to inheriting")
	}
}

// The partial update is what keeps the three purposes independent. Setting the
// dialogue style must not disturb the main narrative or combat, and clearing a
// purpose back to "" must be distinguishable from saying nothing at all.
func TestStylePurposesPartialUpdateChangesOnlyTheNamedPurpose(t *testing.T) {
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "Purposes",
		StoryTellerID: "source-drama",
		StylePurposes: StoryStylePurposes{
			Main:     "source-drama",
			Dialogue: "flippant-banter",
			Combat:   "hotblooded-combat",
		},
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}

	// Change only combat.
	nextCombat := "restrained-hard"
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{
		StylePurposes: &StoryStylePurposesUpdate{Combat: &nextCombat},
	}); err != nil {
		t.Fatalf("update combat: %v", err)
	}
	meta, _, err := store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatalf("read story: %v", err)
	}
	if meta.StylePurposes.Main != "source-drama" {
		t.Fatalf("changing combat disturbed the main style: %q", meta.StylePurposes.Main)
	}
	if meta.StylePurposes.Dialogue != "flippant-banter" {
		t.Fatalf("changing combat disturbed the dialogue style: %q", meta.StylePurposes.Dialogue)
	}
	if meta.StylePurposes.Combat != "restrained-hard" {
		t.Fatalf("combat was not updated: %q", meta.StylePurposes.Combat)
	}

	// Clear dialogue back to inheriting. A pointer to "" is a real choice and
	// must not be confused with leaving the field out.
	empty := ""
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{
		StylePurposes: &StoryStylePurposesUpdate{Dialogue: &empty},
	}); err != nil {
		t.Fatalf("clear dialogue: %v", err)
	}
	meta, _, err = store.readStoryLocked(story.ID)
	if err != nil {
		t.Fatalf("read story after clearing dialogue: %v", err)
	}
	if !meta.StylePurposes.Inherits(PurposeDialogue) {
		t.Fatalf("cleared dialogue still names a style: %q", meta.StylePurposes.Dialogue)
	}
	if meta.StylePurposes.Combat != "restrained-hard" || meta.StylePurposes.Main != "source-drama" {
		t.Fatalf("clearing dialogue disturbed the other purposes: %#v", meta.StylePurposes)
	}
	if got := meta.StylePurposes.ResolvedStyleID(PurposeDialogue, meta.StoryTellerID); got != "source-drama" {
		t.Fatalf("cleared dialogue resolved to %q, want the main style", got)
	}
}

// The selection is a journal fact, so it survives reopening and reaches the
// catalog projection the story list reads.
func TestStylePurposesSurviveReopenAndReachTheSummary(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	story, err := store.CreateStory(CreateStoryRequest{
		Title:         "Persisted",
		StoryTellerID: "rhythm",
		StylePurposes: StoryStylePurposes{Main: "cinematic-shots", Dialogue: "dialogue-piece"},
	})
	if err != nil {
		t.Fatalf("create story: %v", err)
	}

	reopened := NewStore(dir)
	index, err := reopened.Index()
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var found bool
	for _, summary := range index.Stories {
		if summary.ID != story.ID {
			continue
		}
		found = true
		if summary.StylePurposes.Main != "cinematic-shots" || summary.StylePurposes.Dialogue != "dialogue-piece" {
			t.Fatalf("the catalog lost the style purposes: %#v", summary.StylePurposes)
		}
	}
	if !found {
		t.Fatalf("story %s missing from the reopened index", story.ID)
	}
}
