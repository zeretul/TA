package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/internal/cultivation"
	"ta/internal/interactive"
	"ta/internal/storypack"
	"testing"
	"unicode"
)

func TestFantasyAuthoredCraftCardsRequireRealSkillAndDoNotGrantSupplies(t *testing.T) {
	pack, err := storypack.NewLibrary(t.TempDir()).Get("sui-guan-realms")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pack.FrozenGameplay()
	if err != nil {
		t.Fatal(err)
	}
	for _, mastery := range []int{0, 1, 2} {
		actor := interactive.NewAdventureActor(*rules.AdventureRules)
		actor.Skills = map[string]int{}
		if mastery > 0 {
			actor.Skills["sg-forge"] = mastery
		}
		actor.Inventory = map[string]int{}
		actor.Equipment = map[string]string{}
		raw, _ := json.Marshal(actor)
		var value map[string]any
		if err = json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		system := interactive.StoryDirectorActorStateSystem{AdventureRules: rules.AdventureRules, Templates: []interactive.ActorStateTemplate{{ID: interactive.DefaultActorID, Name: "Player", Fields: interactive.AdventureStateFields(*rules.AdventureRules)}}, InitialActors: []interactive.ActorStateInitialActor{{ID: interactive.DefaultActorID, Name: "Player", TemplateID: interactive.DefaultActorID, State: map[string]any{interactive.AdventureStateField: value}}}}
		store := interactive.NewStore(t.TempDir())
		defer store.Close()
		story, e := store.CreateStory(interactive.CreateStoryRequest{Title: "Known craft cards", ActorState: &system})
		if e != nil {
			t.Fatal(e)
		}
		before, e := store.AdventureWindow(story.ID, "main")
		if e != nil {
			t.Fatal(e)
		}
		application := &App{interactive: store}
		for _, locale := range []string{"zh-CN", "en-US"} {
			shown, e := application.AdventureWindow(story.ID, "main", locale)
			if e != nil {
				t.Fatal(e)
			}
			expected := map[string]bool{}
			for _, recipe := range rules.AdventureRules.Recipes {
				if recipe.SkillID == "sg-forge" && mastery >= max(1, recipe.SkillRank) {
					expected[recipe.ID] = true
				}
			}
			if len(shown.CraftMethods) != len(expected) {
				t.Fatalf("mastery %d got %d cards, want %d", mastery, len(shown.CraftMethods), len(expected))
			}
			for _, method := range shown.CraftMethods {
				if !expected[method.ID] || method.ItemNames[method.RequiredTool] == "" {
					t.Fatalf("unlearned or unnamed method %+v", method)
				}
				if locale == "en-US" {
					labels := []string{method.Name, method.SkillName}
					for _, name := range method.ItemNames {
						labels = append(labels, name)
					}
					for _, name := range labels {
						if strings.IndexFunc(name, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
							t.Fatalf("frozen English card leaked canonical title: %s", name)
						}
					}
				}
				if method.Art != nil {
					t.Fatal("unillustrated recipe borrowed an image")
				}
			}
			if len(shown.Items) != 0 || len(shown.Actor.Inventory) != 0 {
				t.Fatal("method references became possessions")
			}
		}
		after, e := store.AdventureWindow(story.ID, "main")
		if e != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("reading cards changed saved state", e)
		}
	}
}

func TestFantasyGeneratedNamesRemainPlayerAuthored(t *testing.T) {
	details := cultivation.DefinitionDetails{Localizations: &cultivation.DefinitionLocalizations{English: cultivation.DefinitionProse{Name: "Template Sword"}}}
	if got := adventureDefinitionName("gen.equipment.personal", "我的旧剑", details, "en-US"); got != "我的旧剑" {
		t.Fatal("player's chosen name was overwritten", got)
	}
	if got := adventureDefinitionName("sg-travel-blade", "行旅短剑", details, "en-US"); got != "Template Sword" {
		t.Fatal("frozen authored title lost", got)
	}
}
