package app

import (
	"encoding/json"
	"reflect"
	"ta/config"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestQualityEnglishOnlyRevisionKeepsModulesAndFreezesEachNewStory(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	dataRoot, workspace := t.TempDir(), t.TempDir()
	library := storypack.NewLibrary(dataRoot)
	pack, err := library.Get("lanchuan-qunxia")
	if err != nil {
		t.Fatal(err)
	}
	moduleID := pack.Story.ModuleRefs.ActorStateID
	modules := interactive.NewActorStateLibrary(dataRoot)
	originalModule, err := modules.Get(moduleID)
	if err != nil {
		t.Fatal(err)
	}
	moduleHash := interactive.ContentHashOf(originalModule)
	canonicalHash := pack.ContentHash()
	canonicalBytes, _ := json.Marshal(pack)
	skillID := pack.Story.Gameplay.AdventureRules.StartingSkills[0]
	originalEnglish := pack.Localizations["en-US"].Definitions[skillID]
	revised := pack
	revised.Localizations = map[string]storypack.Localization{}
	locale := pack.Localizations["en-US"]
	locale.Definitions = map[string]cultivation.DefinitionProse{}
	for id, prose := range pack.Localizations["en-US"].Definitions {
		locale.Definitions[id] = prose
	}
	nextEnglish := originalEnglish
	nextEnglish.Description = "An independently revised English introduction."
	locale.Definitions[skillID] = nextEnglish
	revised.Localizations["en-US"] = locale
	revisedBytes, _ := json.Marshal(revised)
	if revised.ContentHash() != canonicalHash || string(revisedBytes) != string(canonicalBytes) || !reflect.DeepEqual(pack.Story.ModuleRefs, revised.Story.ModuleRefs) {
		t.Fatal("English-only sidecar edit changed canonical pack/hash/module identity")
	}
	store := interactive.NewStore(workspace)
	application := &App{workspace: workspace, cfg: &config.Config{DenovaDir: dataRoot}, interactive: store}
	service := &InteractiveAppService{app: application}
	create := func(source storypack.Pack) string {
		req, err := service.withStoryCreationDefaults(source.StoryRequest())
		if err != nil {
			t.Fatal(err)
		}
		// The exact production projection receives the freshly decoded sidecar.
		// Loading is separate because delivered sidecars are compiled assets, not
		// a new mutable translation-directory framework.
		if err := freezePackDefinitionDetails(req.ActorState, source); err != nil {
			t.Fatal(err)
		}
		actor := interactive.NewAdventureActor(*req.ActorState.AdventureRules)
		raw, _ := json.Marshal(actor)
		var state map[string]any
		if err := json.Unmarshal(raw, &state); err != nil {
			t.Fatal(err)
		}
		req.ActorState.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: state}}}
		req.FrozenModules.ActorState = *req.ActorState
		story, err := store.CreateStory(req)
		if err != nil {
			t.Fatal(err)
		}
		return story.ID
	}
	oldID := create(pack)
	newID := create(revised)
	check := func(id, branch, description string) {
		view, err := application.AdventureWindow(id, branch, "en-US")
		if err != nil {
			t.Fatal(err)
		}
		for _, skill := range view.Skills {
			if skill.ID == skillID {
				if skill.Description != description {
					t.Fatalf("wrong frozen revision for %s/%s: %q", id, branch, skill.Description)
				}
				return
			}
		}
		t.Fatal("known skill missing from public projection")
	}
	check(oldID, "main", originalEnglish.Description)
	check(newID, "main", nextEnglish.Description)
	// Re-reading/materializing the canonical world must keep the reusable module
	// byte-identical; English belongs only to each Story's private frozen copy.
	reloaded, err := library.Get(pack.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterModule, err := modules.Get(moduleID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ContentHash() != canonicalHash || reloaded.Story.ModuleRefs.ActorStateID != moduleID || interactive.ContentHashOf(afterModule) != moduleHash {
		t.Fatal("Story-side locale injection changed reusable canonical modules")
	}
	turn, _, err := store.AppendTurnWithState(oldID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "You wait."})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(oldID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Fork"})
	if err != nil {
		t.Fatal(err)
	}
	check(oldID, branch.ID, originalEnglish.Description)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(workspace)
	defer store.Close()
	application.interactive = store
	check(oldID, "main", originalEnglish.Description)
	check(oldID, branch.ID, originalEnglish.Description)
	check(newID, "main", nextEnglish.Description)
}
