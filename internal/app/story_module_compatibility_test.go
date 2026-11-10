package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ta/internal/interactive"
	"ta/internal/presetlayout"
)

// installStateBoundRuleSystem writes a rule system that declares the actor-state
// system its checks read from, which is the only pairing that can be forged.
func installStateBoundRuleSystem(t *testing.T, novaDir, moduleID, actorStateID string) {
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
  "trpg_system": {"rule_templates": [{"id": "bound", "label": "限定检定", "dice": "1d20"}]}
}`, moduleID, actorStateID)
	if err := os.WriteFile(filepath.Join(dir, moduleID+".json"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCreatingAStoryRefusesAForgedPairBeforeResolvingIt is the create-path half
// of the compatibility rule, and the reason the check cannot live only in the
// store.
//
// ResolveStoryDirectorModules adopts the rule system's own actor-state link, so
// by the time a create request reaches the store a forged pairing has already
// been rewritten into a compatible one. The player asked for the修仙 state
// system and would silently get the detective one instead, with nothing said.
// The refusal therefore has to happen on the request as it was sent.
func TestCreatingAStoryRefusesAForgedPairBeforeResolvingIt(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	dataRoot := application.cfg.DataDir()
	installStateBoundRuleSystem(t, dataRoot, "detective-rules", "detective-state")

	forged := interactive.StoryDirectorModuleRefs{
		RuleSystemID: "detective-rules", ActorStateID: "xiuxian-state",
	}
	_, err := application.CreateInteractiveStoryContext(context.Background(), interactive.CreateStoryRequest{
		Title: "错配", ModuleRefs: &forged,
	})
	if err == nil {
		t.Fatal("a story was created on a rule system bound to a different state system")
	}
	if !strings.Contains(err.Error(), "detective-rules") || !strings.Contains(err.Error(), "detective-state") {
		t.Fatalf("the refusal does not name the pair: %v", err)
	}

	// Nothing was written: the forged request must not leave a story behind.
	index, listErr := application.interactiveService().store().Index()
	if listErr != nil {
		t.Fatal(listErr)
	}
	for _, story := range index.Stories {
		if story.Title == "错配" {
			t.Fatalf("the refused request still created a story: %#v", story)
		}
	}

	// The matching pairing is accepted, so the check refuses the combination and
	// not the rule system.
	matched := interactive.StoryDirectorModuleRefs{
		RuleSystemID: "detective-rules", ActorStateID: "detective-state",
	}
	created, err := application.CreateInteractiveStoryContext(context.Background(), interactive.CreateStoryRequest{
		Title: "匹配", ModuleRefs: &matched,
	})
	if err != nil {
		t.Fatalf("a compatible pairing was refused: %v", err)
	}
	if created.ModuleRefs == nil || created.ModuleRefs.ActorStateID != "detective-state" {
		t.Fatalf("the accepted story did not keep the chosen state system: %#v", created.ModuleRefs)
	}
}

// TestASavedStoryKeepsTheStateSystemItWasGiven pins the other half of the same
// hazard on the save path: a save that repoints the pair is refused, and one
// that leaves it alone is accepted.
func TestASavedStoryKeepsTheStateSystemItWasGiven(t *testing.T) {
	application := newExecutionProfileTestApp(t)
	installStateBoundRuleSystem(t, application.cfg.DataDir(), "detective-rules", "detective-state")

	initial := interactive.StoryDirectorModuleRefs{
		RuleSystemID: "detective-rules", ActorStateID: "detective-state",
	}
	story, err := application.CreateInteractiveStoryContext(context.Background(), interactive.CreateStoryRequest{
		Title: "调查", ModuleRefs: &initial,
	})
	if err != nil {
		t.Fatal(err)
	}

	broken := interactive.StoryDirectorModuleRefs{
		RuleSystemID: "detective-rules", ActorStateID: "xiuxian-state",
	}
	if _, err := application.UpdateInteractiveStory(story.ID, interactive.UpdateStoryRequest{ModuleRefs: &broken}); err == nil {
		t.Fatal("a save repointed the rule system onto a foreign state system")
	}

	unchanged := interactive.StoryDirectorModuleRefs{
		RuleSystemID: "detective-rules", ActorStateID: "detective-state",
	}
	updated, err := application.UpdateInteractiveStory(story.ID, interactive.UpdateStoryRequest{ModuleRefs: &unchanged})
	if err != nil {
		t.Fatalf("a save that kept the pair was refused: %v", err)
	}
	if updated.ModuleRefs == nil || updated.ModuleRefs.ActorStateID != "detective-state" {
		t.Fatalf("the accepted save did not keep the pair: %#v", updated.ModuleRefs)
	}
}
