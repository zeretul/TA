package app

import (
	"encoding/json"
	"reflect"
	"ta/config"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

// Exercise the actual application seeding path for every authored origin, then
// prove that journal replay keeps the frozen catalog and the committed training.
func TestAdventureAllOriginsCreateFreezeTrainAndReload(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(packID, func(t *testing.T) {
			root := t.TempDir()
			service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
			pack, err := storypack.NewLibrary(root).Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			if len(pack.ProtagonistTemplates) != 4 {
				t.Fatal("expected three ordinary origins and one established heir")
			}
			catalog, _ := json.Marshal(pack.Story.Gameplay.AdventureRules)
			t.Logf("catalog bytes=%d context bytes=%d", len(catalog), len(interactive.AdventureRulesContext(*pack.Story.Gameplay.AdventureRules)))
			for _, origin := range pack.ProtagonistTemplates {
				t.Run(origin.ID, func(t *testing.T) {
					req, _, err := pack.StoryRequestForTemplate(origin.ID)
					if err != nil {
						t.Fatal(err)
					}
					req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: packID, TemplateID: origin.ID}
					req, err = service.withStoryCreationDefaults(req)
					if err != nil {
						t.Fatal(err)
					}
					req, err = service.withPackTemplate(req)
					if err != nil {
						t.Fatal(err)
					}
					workspace := t.TempDir()
					store := interactive.NewStore(workspace)
					story, err := store.CreateStory(req)
					if err != nil {
						t.Fatal(err)
					}
					before, err := store.AdventureWindow(story.ID, "")
					if err != nil {
						t.Fatal(err)
					}
					var expected interactive.AdventureActor
					raw, _ := json.Marshal(origin.State[interactive.AdventureStateField])
					if err = json.Unmarshal(raw, &expected); err != nil {
						t.Fatal(err)
					}
					if before.Actor.Currency != expected.Currency || !reflect.DeepEqual(before.Actor.Inventory, expected.Inventory) || !reflect.DeepEqual(before.Actor.Skills, expected.Skills) {
						t.Fatalf("origin state lost: %+v", before.Actor)
					}
					var skillID string
					for _, skill := range before.Rules.Skills {
						if before.Actor.Skills[skill.ID] > 0 {
							skillID = skill.ID
							break
						}
					}
					if skillID == "" {
						t.Fatal("origin has no trainable skill")
					}
					_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "练习一小时", Narrative: "练习结束。", TurnResult: &interactive.TurnResult{Choices: []string{"继续练习", "检查行囊", "休息"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: "练习一小时", Adventure: &interactive.AdventureAction{Action: "train", SkillID: skillID, Hours: 1}}}})
					if err != nil {
						t.Fatal(err)
					}
					after, err := store.AdventureWindow(story.ID, "")
					if err != nil {
						t.Fatal(err)
					}
					if after.Actor.Training[skillID] <= before.Actor.Training[skillID] && after.Actor.Skills[skillID] <= before.Actor.Skills[skillID] {
						t.Fatal("training did not advance")
					}
					if err = store.Close(); err != nil {
						t.Fatal(err)
					}
					reopened := interactive.NewStore(workspace)
					defer reopened.Close()
					restored, err := reopened.AdventureWindow(story.ID, "")
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(after.Actor, restored.Actor) || !reflect.DeepEqual(before.Rules, restored.Rules) {
						t.Fatal("frozen state or rules changed on reopen")
					}
				})
			}
		})
	}
}

func TestAdventureAuthoredNPCPlansGrowOnStoryTime(t *testing.T) {
	for _, packID := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(packID, func(t *testing.T) {
			root := t.TempDir()
			service := &InteractiveAppService{app: &App{workspace: root, cfg: &config.Config{DenovaDir: root}}}
			pack, err := storypack.NewLibrary(root).Get(packID)
			if err != nil {
				t.Fatal(err)
			}
			var selected interactive.AdventureNPC
			for _, npc := range pack.Story.Gameplay.AdventureRules.NPCs {
				if npc.Plan.Action == "train" {
					selected = npc
					break
				}
			}
			if selected.CharacterSourceKey == "" {
				t.Fatal("authored world has no sourced active training plan")
			}
			origin := pack.ProtagonistTemplates[0]
			req, _, err := pack.StoryRequestForTemplate(origin.ID)
			if err != nil {
				t.Fatal(err)
			}
			req.PackTemplate = &interactive.StoryProtagonistTemplateRef{PackID: packID, TemplateID: origin.ID}
			req, err = service.withStoryCreationDefaults(req)
			if err != nil {
				t.Fatal(err)
			}
			req, err = service.withPackTemplate(req)
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(t.TempDir())
			defer store.Close()
			story, err := store.CreateStory(req)
			if err != nil {
				t.Fatal(err)
			}
			var name string
			for _, person := range pack.Characters {
				if person.SourceKey == selected.CharacterSourceKey {
					name = person.Name
				}
			}
			_, err = store.AppendCharacterFacts(story.ID, interactive.AppendCharacterFactsRequest{BranchID: "main", WorldTime: "第1日", Scene: character.Scene{Location: "客舍", Present: []string{name}}, Profiles: []character.Profile{{ID: name, Name: name, SourceKey: selected.CharacterSourceKey, Revision: 1}}})
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "休息四小时", Narrative: "歇下后再行商议。", TurnResult: &interactive.TurnResult{Choices: []string{"继续休息", "交谈", "检查行囊"}, World: &interactive.TurnWorldActionInput{Action: "adventure", SourceText: "休息四小时", Adventure: &interactive.AdventureAction{Action: "rest", Hours: 4}}}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := store.StoryContext(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			npc, err := interactive.AdventureActorFromState(ctx.Snapshot.State, name)
			if err != nil {
				t.Fatal(err)
			}
			if npc.Growth == nil || npc.Growth.Status != "settled" || npc.Training[selected.Plan.SkillID] != 1 {
				t.Fatalf("authored source plan did not receive one real training hour: %+v", npc)
			}
		})
	}
}
