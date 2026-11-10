package storypack

import (
	"context"
	"ta/internal/interactive"
	"testing"
)

func testAdventureGameplay(family string) WorldGameplay {
	status := WorldStatusWuxia
	communication := WorldCommunicationLetter
	if family == "space" {
		status = WorldStatusSpace
		communication = WorldCommunicationRadio
	}
	return WorldGameplay{StatusTemplate: status, NumericScale: family, Growth: family, Combat: family, Communication: communication, LetterDeliveryDays: 1, Opening: WorldOpeningScene, Confirmed: true, AdventureRules: &interactive.AdventureRules{Family: family, Currency: "钱币", StartingCurrency: 30, StartingSkills: []string{"basic"}, Skills: []interactive.AdventureSkill{{ID: "basic", Name: "基本功", Kind: "attack", Scale: "person", Power: 10, RequiredRank: 1, TrainingHours: 2}}}}
}

func TestAdventureRulesMaterializeAndRejectMixedBindings(t *testing.T) {
	root := t.TempDir()
	library := NewLibrary(root)
	for _, family := range []string{"wuxia", "space"} {
		draft := draftTestWorld()
		draft.Name = family + "测试世界"
		draft.Gameplay = testAdventureGameplay(family)
		saved, err := library.SaveWorld(context.Background(), draft)
		if err != nil {
			t.Fatal(err)
		}
		pack, err := library.Get(saved.PackID)
		if err != nil {
			t.Fatal(err)
		}
		module, err := interactive.NewActorStateLibrary(root).Get(pack.Story.ModuleRefs.ActorStateID)
		if err != nil {
			t.Fatal(err)
		}
		if module.ActorState.AdventureRules == nil || module.ActorState.AdventureRules.Family != family {
			t.Fatal("pack did not resolve its own genre rules")
		}
		if _, ok := findWorldStateField(module.ActorState, interactive.AdventureStateField); !ok {
			t.Fatal("canonical adventure field missing")
		}
		if _, ok := findWorldStateField(module.ActorState, "境界"); ok {
			t.Fatal("adventure pack inherited cultivation state")
		}
		pack.Story.ModuleRefs.RuleSystemID = DefaultRuleSystemID
		if pack.ValidateModules(root) == nil {
			t.Fatal("accepted a different combat engine binding")
		}
	}
}

func TestAdventureGameplayRejectsUnsupportedCombinations(t *testing.T) {
	for _, mutate := range []func(*WorldGameplay){
		func(g *WorldGameplay) { g.Combat = WorldCombatCultivation },
		func(g *WorldGameplay) { g.Growth = WorldGrowthNone },
		func(g *WorldGameplay) { g.Communication = WorldCommunicationRadio },
		func(g *WorldGameplay) { g.AdventureRules.Family = "space" },
		func(g *WorldGameplay) { g.AdventureRules = nil },
		func(g *WorldGameplay) {
			g.StateFields = []WorldStateField{{Name: "任意力量", Minimum: 0, Maximum: 999999, Initial: 999999}}
		},
	} {
		g := testAdventureGameplay("wuxia")
		mutate(&g)
		if ValidateWorldGameplay(g) == nil {
			t.Fatalf("accepted illegal adventure combination: %+v", g)
		}
	}
}
