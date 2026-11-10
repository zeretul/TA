package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/config"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestAdventureCreationConfirmedLoadoutsReachRealStoryColdFork(t *testing.T) {
	for _, world := range []string{"xinghai-689", "lanchuan-qunxia"} {
		t.Run(world, func(t *testing.T) {
			dataRoot := t.TempDir()
			service := &InteractiveAppService{app: &App{workspace: dataRoot, cfg: &config.Config{DenovaDir: dataRoot}}}
			pack, err := storypack.NewLibrary(dataRoot).Get(world)
			if err != nil {
				t.Fatal(err)
			}
			var choices []storypack.IdentityChoice
			base := storypack.IdentityChoice{Name: "行舟", Gender: "female", Age: 23, OriginID: pack.Creation.Origins[0].ID, AffiliationID: "none", LocationID: "auto"}
			if world == "xinghai-689" {
				for _, o := range pack.Creation.AdventureClasses {
					c := base
					c.AdventureClassID = o.ID
					choices = append(choices, c)
				}
			} else {
				for _, o := range pack.Creation.AdventureWeapons {
					c := base
					c.AdventureWeaponID = o.ID
					c.AdventureMethodID = "lanchuan-method-iron-breath"
					choices = append(choices, c)
				}
			}
			for _, choice := range choices {
				t.Run(choice.AdventureClassID+choice.AdventureWeaponID, func(t *testing.T) {
					want, err := pack.ResolveIdentity(choice)
					if err != nil {
						t.Fatal(err)
					}
					req, _, err := withConfirmedIdentity(pack, pack.StoryRequest(), choice)
					if err != nil {
						t.Fatal(err)
					}
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
					ctx, err := store.StoryContext(story.ID, "main")
					if err != nil {
						t.Fatal(err)
					}
					actual, err := interactive.AdventureActorFromState(ctx.Snapshot.State, interactive.DefaultActorID)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(actual.Skills, want.AdventureState.Skills) || !reflect.DeepEqual(actual.Inventory, want.AdventureState.Inventory) || !reflect.DeepEqual(actual.Equipment, want.AdventureState.Equipment) {
						t.Fatal("confirmed loadout was lost between preview and initial journal")
					}
					frozenBytes, _ := json.Marshal(ctx.Snapshot.ActorStateSchema.System.AdventureRules.StartingLoadouts)
					// Mutation of the resolved inputs cannot rewrite an existing save.
					req.ActorState.AdventureRules.StartingLoadouts[0].Items = map[string]int{"invented": 999}
					req.IdentityStateOverlay[interactive.AdventureStateField] = map[string]any{"rank": 60, "currency": 999999}
					turn, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "看看行囊", Narrative: "行舟确认了眼下携带的器物。"})
					if err != nil {
						t.Fatal(err)
					}
					fork, err := store.CreateBranch(story.ID, interactive.CreateBranchRequest{ParentEventID: turn.ID, Title: "Starting kit branch"})
					if err != nil {
						t.Fatal(err)
					}
					cold := interactive.NewStore(store.Root())
					defer cold.Close()
					for _, branch := range []string{"main", fork.ID} {
						view, err := cold.StoryContext(story.ID, branch)
						if err != nil {
							t.Fatal(err)
						}
						actor, err := interactive.AdventureActorFromState(view.Snapshot.State, interactive.DefaultActorID)
						if err != nil || !reflect.DeepEqual(actor, actual) {
							t.Fatalf("cold fork changed the actual initial choice: %v", err)
						}
						raw, _ := json.Marshal(view.Snapshot.ActorStateSchema.System.AdventureRules.StartingLoadouts)
						if string(raw) != string(frozenBytes) {
							t.Fatal("current author input rewrote frozen loadout mechanics")
						}
					}
				})
			}
		})
	}
}

func TestAdventureCreationDraftFactsUseActualLoadoutAndLanguage(t *testing.T) {
	pack, err := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := pack.ResolveIdentity(storypack.IdentityChoice{Name: "行舟", Gender: "female", Age: 23, OriginID: pack.Creation.Origins[0].ID, AffiliationID: "none", LocationID: "auto", AdventureClassID: "xinghai-class-psychic"})
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		shown := pack.LocalizeIdentity(identity, locale)
		facts := creationAdventureDraftFacts(shown, locale)
		for _, name := range append(append([]string{}, shown.InitialState.Techniques...), shown.InitialState.Equipped...) {
			if !strings.Contains(facts, name) {
				t.Fatalf("draft omitted real initial ability/equipment %s", name)
			}
		}
		if locale == "en-US" && (!strings.Contains(facts, "Initially equipped") || !strings.Contains(facts, "Psychic Pulse")) {
			t.Fatal("English draft lacks localized loadout facts")
		}
	}
	if creationAdventureDraftFacts(storypack.Identity{}, "en-US") != "" {
		t.Fatal("ordinary creation acquired invented adventure facts")
	}
}
