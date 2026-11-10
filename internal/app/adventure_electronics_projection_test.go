package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
)

func TestPersonalPlasmaPublicWindowKeepsSafeActionEligibilityNotPrivateRange(t *testing.T) {
	pack, e := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
	if e != nil {
		t.Fatal(e)
	}
	r := *pack.Story.Gameplay.AdventureRules
	actor := interactive.NewAdventureActor(r)
	actor.Rank = 4
	actor.Skills["xinghai-s23"] = 1
	actor, _, e = interactive.ResolveAdventureAction(r, actor, interactive.AdventureAction{Action: "battle_start", OpponentID: "xinghai-electronic-node"}, "known-node")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(actor)
	var value map[string]any
	_ = json.Unmarshal(b, &value)
	system := interactive.StoryDirectorActorStateSystem{AdventureRules: &r, Templates: []interactive.ActorStateTemplate{{ID: interactive.DefaultActorID, Name: "主角", Fields: interactive.AdventureStateFields(r)}}, InitialActors: []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "主角", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}}
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, e := store.CreateStory(interactive.CreateStoryRequest{Title: "Qualified public-window fixture", ActorState: &system})
	if e != nil {
		t.Fatal(e)
	}
	application := &App{interactive: store}
	window, e := application.AdventureWindow(story.ID, "main", "en-US")
	if e != nil {
		t.Fatal(e)
	}
	wire, e := json.Marshal(window)
	if e != nil {
		t.Fatal(e)
	}
	var public map[string]any
	_ = json.Unmarshal(wire, &public)
	battle := public["actor"].(map[string]any)["battle"].(map[string]any)
	personal := false
	for _, s := range window.Skills {
		if s.ID == "xinghai-s23" {
			personal = s.Scale == "person"
		}
	}
	if battle["can_disrupt_electronics"] != true || !personal || len(window.Actor.Assets) != 0 {
		t.Fatalf("personal action eligibility was dropped or required a machine: battle=%+v skills=%+v assets=%+v", battle, window.Skills, window.Actor.Assets)
	}
	for _, secret := range []string{"electronic_distance_meters", "enemy_inventory", "enemy_energy", "enemy_item_ready_at"} {
		if strings.Contains(string(wire), secret) {
			t.Fatalf("public window leaked %s", secret)
		}
	}
	canonical, e := store.AdventureWindow(story.ID, "main")
	if e != nil || canonical.Actor.Battle.ElectronicDistanceMeters != 8 {
		t.Fatal("public display mutated canonical target facts", e)
	}
}

func TestQualityAdventurePreflightUsesFormalClockWithoutMutatingState(t *testing.T) {
	for _, until := range []int64{60, 7200} {
		t.Run(fmt.Sprint(until), func(t *testing.T) {
			pack, e := storypack.NewLibrary(t.TempDir()).Get("xinghai-689")
			if e != nil {
				t.Fatal(e)
			}
			r := *pack.Story.Gameplay.AdventureRules
			actor := interactive.NewAdventureActor(r)
			actor.Rank = 4
			actor.Skills["xinghai-s32"] = 1
			actor.Inventory["xinghai-frigate"] = 1
			for _, i := range r.Items {
				if i.ID == "xinghai-frigate" {
					actor.Assets[i.ID] = interactive.AdventureAsset{Health: i.Durability, MaxHealth: i.Durability, Shield: i.Power, Fuel: 100, DisabledUntil: until}
				}
			}
			b, _ := json.Marshal(actor)
			var value map[string]any
			_ = json.Unmarshal(b, &value)
			system := interactive.StoryDirectorActorStateSystem{AdventureRules: &r, Templates: []interactive.ActorStateTemplate{{ID: interactive.DefaultActorID, Name: "主角", Fields: interactive.AdventureStateFields(r)}}, InitialActors: []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "主角", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}}
			store := interactive.NewStore(t.TempDir())
			defer store.Close()
			story, e := store.CreateStory(interactive.CreateStoryRequest{Title: "Qualified formal-clock preflight", ActorState: &system})
			if e != nil {
				t.Fatal(e)
			}
			_, _, e = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{BranchID: "main", User: "休养一小时", Narrative: "你在船坞旁休养了一小时。", TurnResult: &interactive.TurnResult{Choices: []string{"继续", "观察", "休息", "询问", "离开"}, World: &interactive.TurnWorldActionInput{Action: "adventure", Adventure: &interactive.AdventureAction{Action: "rest", Hours: 1}, SourceText: "休养一小时"}}})
			if e != nil {
				t.Fatal(e)
			}
			before, e := store.AdventureWindow(story.ID, "main")
			if e != nil {
				t.Fatal(e)
			}
			application := &App{interactive: store}
			view, e := application.AdventureWindow(story.ID, "main", "en-US")
			if e != nil {
				t.Fatal(e)
			}
			expected := until <= before.ClockSeconds
			found := false
			for _, item := range view.Items {
				if item.ID == "xinghai-frigate" {
					found = true
					if item.Deployable == nil || *item.Deployable != expected {
						t.Fatal("deployment details used time zero or ignored current lock")
					}
				}
			}
			after, e := store.AdventureWindow(story.ID, "main")
			if e != nil || !found || !reflect.DeepEqual(before, after) {
				t.Fatal("detail preflight mutated the clock, lock, ownership or head", e)
			}
		})
	}
}
