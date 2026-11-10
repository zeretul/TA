package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"ta/config"
	"ta/internal/interactive"
	"ta/internal/presetlayout"
	"ta/internal/storypack"
	"testing"
)

func TestQualityCultivationPackProseFreezesThroughRealCreationDefaults(t *testing.T) {
	t.Setenv("TA_DIR", t.TempDir())
	dataRoot, workspace := t.TempDir(), t.TempDir()
	store := interactive.NewStore(workspace)
	defer store.Close()
	service := &InteractiveAppService{app: &App{workspace: workspace, cfg: &config.Config{DenovaDir: dataRoot}, interactive: store}}
	pack, err := storypack.NewLibrary(dataRoot).Get("fanren-jie")
	if err != nil {
		t.Fatal(err)
	}
	req, err := service.withStoryCreationDefaults(pack.StoryRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := pack.Story.Gameplay.DefinitionDetails["fire_bolt"]
	if want.Description == "" || req.ActorState.DefinitionDetails["fire_bolt"].Description != want.Description || req.FrozenModules.ActorState.DefinitionDetails["fire_bolt"].Description != want.Description {
		t.Fatal("metadata did not reach authoritative frozen creation request")
	}
	wantEnglish := pack.Localizations["en-US"].Definitions["fire_bolt"]
	if req.ActorState.DefinitionDetails["fire_bolt"].Localizations == nil || req.ActorState.DefinitionDetails["fire_bolt"].Localizations.English != wantEnglish {
		t.Fatal("sidecar English did not reach creation snapshot")
	}
	actorState := *req.ActorState
	actorState.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{"当前境界": "qi_3", "已学功法": map[string]any{"fire_bolt": map[string]any{"定义": "fire_bolt", "版本": float64(1), "掌握程度": "已学会", "描述": "actual own wording", "来源": "actual teacher"}}}}}
	req.ActorState = &actorState
	req.FrozenModules.ActorState = actorState
	story, err := store.CreateStory(req)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	staleRequest := pack.StoryRequest()
	edited := pack
	details := edited.Story.Gameplay.DefinitionDetails["fire_bolt"]
	details.Description = "new author text"
	edited.Story.Gameplay.DefinitionDetails["fire_bolt"] = details
	if err := os.MkdirAll(presetlayout.StoryPacks(dataRoot), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writePackFileForTest(t, filepath.Join(presetlayout.StoryPacks(dataRoot), pack.ID+".json"), edited); err != nil {
		t.Fatal(err)
	}
	after, err := store.StoryContext(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if after.Snapshot.ActorStateSchema.System.DefinitionDetails["fire_bolt"].Description != want.Description || before.Snapshot.ActorStateSchema.System.DefinitionDetails["fire_bolt"].Description != want.Description {
		t.Fatal("mutable pack changed already-created story metadata")
	}
	turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "You wait."})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Fork"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = interactive.NewStore(workspace)
	defer store.Close()
	service.app.interactive = store
	view, err := service.app.CultivationEntryDetails(story.ID, branch.ID, "en-US")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Skills) != 1 || view.Skills[0].Description != wantEnglish.Description || view.Skills[0].Background != wantEnglish.Background || view.Skills[0].Source != "actual teacher" || view.Skills[0].InstanceDescription != "actual own wording" {
		t.Fatalf("cold fork lost sidecar/provenance: %+v", view.Skills)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), `"localizations"`) {
		t.Fatal("public DTO exposed locale maps")
	}
	if _, err := service.withStoryCreationDefaults(staleRequest); err == nil {
		t.Fatal("stale pack revision silently accepted for metadata creation")
	}
}

func TestQualityAdventureSidecarReachesRealCreationAndColdFork(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(packID, func(t *testing.T) {
			t.Setenv("TA_DIR", t.TempDir())
			dataRoot, workspace := t.TempDir(), t.TempDir()
			store := interactive.NewStore(workspace)
			application := &App{workspace: workspace, cfg: &config.Config{DenovaDir: dataRoot}, interactive: store}
			service := &InteractiveAppService{app: application}
			pack, err := storypack.NewLibrary(dataRoot).Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			req, err := service.withStoryCreationDefaults(pack.StoryRequest())
			if err != nil {
				t.Fatal(err)
			}
			rules := req.ActorState.AdventureRules
			if rules == nil || len(rules.StartingSkills) == 0 {
				t.Fatal("real source starting skill missing")
			}
			skillID := rules.StartingSkills[0]
			want := pack.Localizations["en-US"].Definitions[skillID]
			if want.Description == "" {
				t.Fatal("real source sidecar missing")
			}
			actor := interactive.NewAdventureActor(*rules)
			raw, _ := json.Marshal(actor)
			var state map[string]any
			if err := json.Unmarshal(raw, &state); err != nil {
				t.Fatal(err)
			}
			system := *req.ActorState
			system.InitialActors = []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: state}}}
			req.ActorState = &system
			req.FrozenModules.ActorState = system
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			// A mutation of the creation input cannot change persisted bilingual facts.
			for i := range rules.Skills {
				if rules.Skills[i].ID == skillID {
					rules.Skills[i].Localizations.English.Description = "changed creation input"
				}
			}
			turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "Continue", Narrative: "You wait."})
			if err != nil {
				t.Fatal(err)
			}
			branch, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Fork"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store = interactive.NewStore(workspace)
			defer store.Close()
			application.interactive = store
			view, err := application.AdventureWindow(story.ID, branch.ID, "en-US")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, skill := range view.Skills {
				if skill.ID == skillID {
					found = true
					if skill.Description != want.Description || skill.Background != want.Background {
						t.Fatalf("frozen English lost: %+v", skill)
					}
				}
			}
			if !found {
				t.Fatal("known starting skill missing from public DTO")
			}
			data, _ := json.Marshal(view)
			if strings.Contains(string(data), `"localizations"`) {
				t.Fatal("public adventure DTO leaked locale maps")
			}
		})
	}
}
