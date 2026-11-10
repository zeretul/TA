package interactive

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ta/internal/presetlayout"
)

// writeStateBoundRuleModule installs a rule system that declares the actor-state
// system its checks read from. That declaration is what makes a pairing
// forgeable: the rules resolve, the state system resolves, and only the pair is
// wrong.
func writeStateBoundRuleModule(t *testing.T, novaDir, moduleID, actorStateID string) {
	t.Helper()
	dir := presetlayout.RuleSystems(novaDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	document := fmt.Sprintf(`{
  "version": 1,
  "id": %q,
  "name": "限定状态系统的检定",
  "actor_state_id": %q,
  "trpg_system": {
    "rule_templates": [{"id": "bound", "label": "限定检定", "dice": "1d20"}]
  }
}`, moduleID, actorStateID)
	if err := os.WriteFile(filepath.Join(dir, moduleID+".json"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCreateStoryRefusesARuleSystemOnAForeignStateSystem covers the last clause
// of the requirement: the backend refuses a forged incompatible combination
// rather than accepting it and running checks that can never fire.
func TestCreateStoryRefusesARuleSystemOnAForeignStateSystem(t *testing.T) {
	novaDir := t.TempDir()
	writeStateBoundRuleModule(t, novaDir, "detective-rules", "detective-state")
	store := NewStoreWithNovaDir(t.TempDir(), novaDir)

	refs := StoryDirectorModuleRefs{RuleSystemID: "detective-rules", ActorStateID: "xiuxian-state"}
	_, err := store.CreateStory(CreateStoryRequest{Title: "错配", ModuleRefs: &refs})
	if err == nil {
		t.Fatal("a story started on a rule system bound to a different state system")
	}
	if !strings.Contains(err.Error(), "detective-rules") || !strings.Contains(err.Error(), "detective-state") {
		t.Fatalf("the refusal does not name the pair: %v", err)
	}

	// The matching pairing is accepted, so the check refuses the combination
	// rather than the rule system itself.
	matched := StoryDirectorModuleRefs{RuleSystemID: "detective-rules", ActorStateID: "detective-state"}
	if _, err := store.CreateStory(CreateStoryRequest{Title: "匹配", ModuleRefs: &matched}); err != nil {
		t.Fatalf("a compatible pairing was refused: %v", err)
	}
}

// TestUpdateStoryRefusesRepointingTheRulePair covers the save path. Only a save
// that actually changes the pair is checked, so a story already running on a
// pairing its preset no longer declares stays savable.
func TestUpdateStoryRefusesRepointingTheRulePair(t *testing.T) {
	novaDir := t.TempDir()
	writeStateBoundRuleModule(t, novaDir, "detective-rules", "detective-state")
	store := NewStoreWithNovaDir(t.TempDir(), novaDir)

	refs := StoryDirectorModuleRefs{NarrativeStyleID: "noir", RuleSystemID: "detective-rules", ActorStateID: "detective-state"}
	story, err := store.CreateStory(CreateStoryRequest{Title: "调查", ModuleRefs: &refs})
	if err != nil {
		t.Fatal(err)
	}

	broken := StoryDirectorModuleRefs{NarrativeStyleID: "noir", RuleSystemID: "detective-rules", ActorStateID: "xiuxian-state"}
	if _, err := store.UpdateStory(story.ID, UpdateStoryRequest{ModuleRefs: &broken}); err == nil {
		t.Fatal("a save repointed the rule system onto a foreign state system")
	}

	// Changing only the narrative style keeps the pairing untouched and must be
	// accepted even though the stored pairing is the one being checked.
	restyled := StoryDirectorModuleRefs{NarrativeStyleID: "rhythm", RuleSystemID: "detective-rules", ActorStateID: "detective-state"}
	updated, err := store.UpdateStory(story.ID, UpdateStoryRequest{ModuleRefs: &restyled})
	if err != nil {
		t.Fatalf("a save that left the rule pair alone was refused: %v", err)
	}
	if updated.ModuleRefs == nil || updated.ModuleRefs.NarrativeStyleID != "rhythm" {
		t.Fatalf("the accepted save did not persist: %#v", updated.ModuleRefs)
	}
}

// TestStoryModuleCompatibilityIsSkippedWithoutACatalog pins the degradation
// mode shared with ResolveStoryDirectorModuleErrors: an in-memory caller has no
// catalog to compare against, so it is never failed for a pairing it cannot be
// checked on.
func TestStoryModuleCompatibilityIsSkippedWithoutACatalog(t *testing.T) {
	refs := StoryDirectorModuleRefs{RuleSystemID: "detective-rules", ActorStateID: "xiuxian-state"}
	if err := ValidateStoryModuleCompatibility("", refs); err != nil {
		t.Fatalf("an in-memory caller was refused: %v", err)
	}

	store := NewStore(t.TempDir())
	if _, err := store.CreateStory(CreateStoryRequest{Title: "无目录", ModuleRefs: &refs}); err != nil {
		t.Fatalf("a store with no data root refused the pairing: %v", err)
	}
}

// TestStoryModulePairChangeIgnoresUnrelatedModules keeps the guard narrow: a
// save that only touches the image preset is not a repointing of the pair.
func TestStoryModulePairChangeIgnoresUnrelatedModules(t *testing.T) {
	base := StoryDirectorModuleRefs{RuleSystemID: "a", ActorStateID: "b", ImagePresetID: "realistic"}
	reimaged := StoryDirectorModuleRefs{RuleSystemID: "a", ActorStateID: "b", ImagePresetID: "cartoon"}
	if StoryModulePairChanged(base, reimaged) {
		t.Fatal("an image preset change counted as a rule-pair change")
	}
	if !StoryModulePairChanged(base, StoryDirectorModuleRefs{RuleSystemID: "c", ActorStateID: "b"}) {
		t.Fatal("a rule system change was not detected")
	}
	if !StoryModulePairChanged(base, StoryDirectorModuleRefs{RuleSystemID: "a", ActorStateID: "d"}) {
		t.Fatal("a state system change was not detected")
	}
}

func TestChangingChecksAfterOpeningUpdatesFrozenRulesOnly(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	store := NewStoreWithNovaDir(root, data)
	base := DefaultActorStateModule().ActorState
	refs := DefaultStoryDirectorModuleRefs()
	policy := StoryStateSchemaPolicy{Mode: StoryStateSchemaModeFixedTemplate}
	story, err := store.CreateStory(CreateStoryRequest{Title: "Rule switch", ActorState: &base, ModuleRefs: &refs, StateSchemaPolicy: &policy})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{User: "观察", Narrative: "你站在窗前。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	selected := DefaultRuleSystemModule()
	selected.ID = "alternative-checks"
	selected.Name = "Alternative"
	selected.TRPGSystem.RuleTemplates[0].Label = "Alternative resolution"
	selected, err = NewRuleSystemLibrary(data).Create(selected)
	if err != nil {
		t.Fatal(err)
	}
	refs.RuleSystemID = selected.ID
	if _, err = store.UpdateStory(story.ID, UpdateStoryRequest{ModuleRefs: &refs}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStoreWithNovaDir(root, data)
	defer store.Close()
	after, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.State, after.State) || !reflect.DeepEqual(before.ActorStateSchema.System, after.ActorStateSchema.System) || after.TurnCount != before.TurnCount {
		t.Fatal("switch reset state or progress")
	}
	if after.ActorStateSchema.TRPGSystem.RuleTemplates[0].Label != "Alternative resolution" {
		t.Fatal("frozen checks still use the old system")
	}
}
