package interactiveapp

import (
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/interactive/teller"
)

// newStyleTestDataRoot seeds the narrative-style library into a fresh directory
// and returns its path. The resolver reads presets through the same library the
// product uses, so the test exercises the real loading path rather than a stub.
func newStyleTestDataRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Listing the library seeds every builtin, including the twelve migrated
	// styles, exactly as first launch does.
	if _, err := teller.NewLibrary(dir).List(); err != nil {
		t.Fatalf("seed narrative styles: %v", err)
	}
	return dir
}

// The main narrative and the two override purposes must assemble from the
// styles the story actually selected, not from a single stacked prompt.
func TestResolveStoryStyleFragmentsAppliesEachPurpose(t *testing.T) {
	dir := newStyleTestDataRoot(t)
	meta := interactive.StoryMeta{
		StoryTellerID: "source-drama",
		StylePurposes: interactive.StoryStylePurposes{
			Main:     "source-drama",
			Dialogue: "flippant-banter",
			Combat:   "hotblooded-combat",
		},
	}
	fragments := ResolveStoryStyleFragments(dir, meta)

	if fragments.MainStyleID != "source-drama" || fragments.DialogueStyleID != "flippant-banter" || fragments.CombatStyleID != "hotblooded-combat" {
		t.Fatalf("purposes resolved to %q/%q/%q", fragments.MainStyleID, fragments.DialogueStyleID, fragments.CombatStyleID)
	}
	for name, value := range map[string]string{
		"main system": fragments.MainSystem, "main turn": fragments.MainTurn,
		"dialogue": fragments.Dialogue, "combat": fragments.Combat,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s fragment is empty", name)
		}
	}
	// The override fragments must be their own styles' text, not a copy of the
	// main voice. This is the "twelve names, one prompt" failure.
	if fragments.Dialogue == fragments.MainSystem || fragments.Combat == fragments.MainSystem {
		t.Fatal("an override purpose reused the main narrative fragment")
	}
	// A purpose fragment is lean: it must not drag in the main voice's heading.
	if strings.Contains(fragments.Dialogue, "## 叙事声口") {
		t.Fatalf("the dialogue fragment pulled in the main narrative voice:\n%s", fragments.Dialogue)
	}
}

// A purpose with no style of its own follows the main narrative, so its
// fragment is the main style's own fragment for that purpose.
func TestResolveStoryStyleFragmentsInheritsTheMainStyle(t *testing.T) {
	dir := newStyleTestDataRoot(t)
	meta := interactive.StoryMeta{
		StoryTellerID: "source-drama",
		StylePurposes: interactive.StoryStylePurposes{Main: "source-drama"},
	}
	fragments := ResolveStoryStyleFragments(dir, meta)

	if fragments.DialogueStyleID != "source-drama" || fragments.CombatStyleID != "source-drama" {
		t.Fatalf("inheriting purposes resolved to %q/%q", fragments.DialogueStyleID, fragments.CombatStyleID)
	}
	if strings.TrimSpace(fragments.Dialogue) == "" {
		t.Fatal("an inheriting dialogue purpose produced nothing")
	}
}

// A story created before purposes existed has an empty config. It must resolve
// through its own stored style rather than failing or silently changing.
func TestResolveStoryStyleFragmentsHandlesAStoryWithNoPurposeConfig(t *testing.T) {
	dir := newStyleTestDataRoot(t)
	meta := interactive.StoryMeta{StoryTellerID: "rhythm"}
	fragments := ResolveStoryStyleFragments(dir, meta)

	if fragments.MainStyleID != "rhythm" {
		t.Fatalf("a story with no purpose config resolved its main style to %q", fragments.MainStyleID)
	}
	if strings.TrimSpace(fragments.MainSystem) == "" {
		t.Fatal("a pre-migration story produced no main narrative fragment")
	}
	// The four pre-migration styles carry no dialogue or combat fragment. An
	// empty result is correct here: the caller applies nothing rather than a
	// wrong voice, which is the same outcome as inheriting.
	if fragments.Dialogue != "" || fragments.Combat != "" {
		t.Fatalf("a pre-migration style produced override fragments: dialogue=%q combat=%q", fragments.Dialogue, fragments.Combat)
	}
}

// The main narrative instruction carries the resolved purposes, so a style
// changed through the purpose config governs the request that is generated.
func TestStoryTellerSystemInputCarriesTheResolvedPurposes(t *testing.T) {
	dir := newStyleTestDataRoot(t)
	meta := interactive.StoryMeta{
		StoryTellerID: "source-drama",
		StylePurposes: interactive.StoryStylePurposes{Main: "strange-tales", Dialogue: "flippant-banter"},
	}
	fragments := ResolveStoryStyleFragments(dir, meta)
	definition := LoadGameTeller(dir, meta.StoryTellerID)

	input := StoryTellerSystemInput(definition, &fragments)
	// The main voice comes from the resolved purpose, not from the definition.
	if input.StoryTellerSystemPrompt != fragments.MainSystem {
		t.Fatal("the system instruction did not use the resolved main purpose")
	}
	if input.DialogueStylePrompt != fragments.Dialogue {
		t.Fatal("the system instruction did not carry the dialogue purpose")
	}
	// A story that overrides nothing still gets a working main narrative.
	plain := StoryTellerSystemInput(definition, nil)
	if strings.TrimSpace(plain.StoryTellerSystemPrompt) == "" {
		t.Fatal("a definition with no fragments produced no main narrative")
	}
	if plain.DialogueStylePrompt != "" || plain.CombatStylePrompt != "" {
		t.Fatal("a definition with no fragments invented an override")
	}
}
