package interactive

import (
	"reflect"
	"testing"

	"ta/internal/exploration"
)

// This is a deterministic acquisition-and-use chain through formal turns, not an
// AI transcript: the rope is bought with actual wages, not seeded into a pack.
func TestRealmExploreWuxiaEarnBuyRopePreserveAndReturn(t *testing.T) {
	realm := exploration.RealmStrandedTideBoat
	s, id, _ := realmExploreStory(t, "lanchuan-qunxia", "澜口城", nil, map[string]int{"lanchuan-s32": 1})
	start, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if start.Actor.Currency != 0 || start.Actor.Inventory["lanchuan-i28"] != 0 {
		t.Fatal("fixture must begin without wages or rope")
	}
	worldFlowTurn(t, s, id, "在码头搬运四小时", "完成四小时码头搬运。", TurnWorldActionInput{Action: "adventure", SourceText: "在码头搬运四小时", Adventure: &AdventureAction{Action: "work", MarketID: "lanchuan-market-l01", Hours: 4}})
	earned, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if earned.Actor.Currency != 40 || worldFlowClock(t, s, id).Seconds != 14400 {
		t.Fatal("work did not settle earned pay and its real interval")
	}
	worldFlowTurn(t, s, id, "买一条三爪带钩生麻绳", "付钱买下绳索。", TurnWorldActionInput{Action: "adventure", SourceText: "买一条三爪带钩生麻绳", Adventure: &AdventureAction{Action: "buy", MarketID: "lanchuan-market-l01", ItemID: "lanchuan-i28", Quantity: 1}})
	bought, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if bought.Actor.Currency != 10 || bought.Actor.Inventory["lanchuan-i28"] != 1 {
		t.Fatal("rope must cost thirty actual coins")
	}
	worldFlowTurn(t, s, id, "去潮信渡", "走到潮信渡。", TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "lanchuan-route-l01-l02", Arrive: true})
	entered := realmTestApply(t, s, id, realmExploreRequest("", realm, RealmActionEnter, "", "enter"))
	realmExploreDo(t, s, id, entered.InstanceID, realm, "approach", "rope")
	afterRope, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if afterRope.Actor.Inventory["lanchuan-i28"] != 1 || afterRope.Actor.Energy != bought.Actor.Energy {
		t.Fatal("holding a rope must neither destroy it nor charge internal energy")
	}
	realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "outer")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "core")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "preserve")
	view := realmExploreViewAt(t, s, id, "main")
	if view.Explore.Pressure != 2 || view.Explore.ReturnDueSeconds-worldFlowClock(t, s, id).Seconds != 21600 {
		t.Fatal("tool preparation must retain pressure room and use a six-hour return delay")
	}
	realmExploreMove(t, s, id, entered.InstanceID, realm, "entry", "retreat")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionExit, "", "exit"))
	worldFlowTurn(t, s, id, "休整六小时", "在渡边休整六小时。", TurnWorldActionInput{Action: "adventure", SourceText: "休整六小时", Adventure: &AdventureAction{Action: "rest", Hours: 6}})
	if realmExploreQuantity(t, s, id, "main", "lanchuan-qunxia", "lanchuan-herbs") != 1 {
		t.Fatal("rest or outside read granted the unopened return cache")
	}
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, realm, RealmActionReenter, "", "reenter"))
	realmExploreMove(t, s, id, entered.InstanceID, realm, "core", "return-core")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "approach", "return")
	realmExploreMove(t, s, id, entered.InstanceID, realm, "side", "side")
	realmExploreDo(t, s, id, entered.InstanceID, realm, "resolve", "cache")
	if realmExploreQuantity(t, s, id, "main", "lanchuan-qunxia", "lanchuan-herbs") != 3 || realmExploreQuantity(t, s, id, "main", "lanchuan-qunxia", "lanchuan-i28") != 1 {
		t.Fatal("finite loot or bought rope did not survive the return chain")
	}
	// The recipe skill is an explicit qualification fixture. All three herbs
	// came from this realm's finite rewards; no material was seeded or bought.
	beforeCraft := worldFlowClock(t, s, id).Seconds
	worldFlowTurn(t, s, id, "用带回的三份药草配制四小时金疮药", "取用探索所得药草配制金疮药。", TurnWorldActionInput{Action: "adventure", SourceText: "用带回的三份药草配制四小时金疮药", Adventure: &AdventureAction{Action: "craft", RecipeID: "lanchuan-prepare-salve", Hours: 4, Quantity: 1}})
	if realmExploreQuantity(t, s, id, "main", "lanchuan-qunxia", "lanchuan-herbs") != 0 || realmExploreQuantity(t, s, id, "main", "lanchuan-qunxia", "lanchuan-i18") != 1 || worldFlowClock(t, s, id).Seconds != beforeCraft+14400 {
		t.Fatal("authored crafting did not consume all three real exploration herbs into one salve over four hours")
	}

}

func TestRealmExploreKeepsLegacyRealmAndRefusesSimultaneousPresence(t *testing.T) {
	s, id := realmTestStory(t)
	old := realmTestEnter(t, s, id)
	before := realmTestRecordCount(t, s, id, "main")
	if _, err := s.ApplyRealm(id, realmExploreRequest("", exploration.RealmMountainHerbCave, RealmActionEnter, "", "nested")); err == nil {
		t.Fatal("one actor entered a second realm while still inside the original realm")
	}
	if realmTestRecordCount(t, s, id, "main") != before {
		t.Fatal("nested entry changed the journal")
	}
	realmExploreReachLegacyExit(t, s, id, old.InstanceID)
	realmTestApply(t, s, id, realmExploreRequest(old.InstanceID, exploration.RealmGuXiuDongFu, RealmActionExit, "", "leave-old"))
	legacyBefore, err := s.RealmStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	newer := realmTestApply(t, s, id, realmExploreRequest("", exploration.RealmMountainHerbCave, RealmActionEnter, "", "new"))
	realmExploreDo(t, s, id, newer.InstanceID, exploration.RealmMountainHerbCave, "resolve", "outer")
	realmTestApply(t, s, id, realmExploreRequest(newer.InstanceID, exploration.RealmMountainHerbCave, RealmActionExit, "", "leave-new"))
	state, err := s.RealmStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 2 {
		t.Fatalf("new place overwrote the existing realm: %+v", state.Instances)
	}
	found := false
	for _, v := range state.Instances {
		if v.InstanceID == old.InstanceID {
			found = true
			if !reflect.DeepEqual(v, legacyBefore.Instances[0]) {
				t.Fatal("new exploration mutated the old realm's known state")
			}
		}
	}
	if !found {
		t.Fatal("old realm disappeared")
	}
	reentered := realmTestApply(t, s, id, realmExploreRequest(old.InstanceID, exploration.RealmGuXiuDongFu, RealmActionReenter, "", "return-old"))
	if reentered.InstanceID != old.InstanceID || reentered.Instance.ExploreState != nil {
		t.Fatal("legacy realm was replaced or retroactively given new exploration mechanics")
	}
}

// Story-local adaptations use their frozen foundation without inventing a copy
// under the original ID. The original source is intentionally absent in these
// isolated persistence fixtures, so a template-only lookup cannot pass them.
func TestRealmExploreAdaptedSkillAndHeldToolKeepRealConsumers(t *testing.T) {
	for _, tc := range []struct {
		name, pack, realm, location, category, template, method string
		energy                                                  int
	}{
		{"adapted rope", "lanchuan-qunxia", exploration.RealmStrandedTideBoat, "潮信渡", "equipment", "lanchuan-i28", "rope", 0},
		{"adapted boatcraft", "lanchuan-qunxia", exploration.RealmStrandedTideBoat, "潮信渡", "skill", "lanchuan-s34", "boatcraft", 6},
		{"adapted repair", "xinghai-689", exploration.RealmShaftServiceBay, "废弃47号排污减压井盲端", "skill", "xinghai-s02", "repair", 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := authoredQuestRules(t, tc.pack)
			a := NewAdventureActor(r)
			if tc.category == "equipment" {
				a.Inventory[tc.template] = 1
			} else {
				a.Skills[tc.template] = 1
			}
			if tc.method == "repair" {
				a.Inventory["xinghai-i34"] = 1
			}
			d := AdventureGeneratedDefinition{Category: tc.category, TemplateID: tc.template, Name: "已验证改制" + tc.method, Source: "由实际持有物或已学技能发展"}
			var err error
			a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "generate", Generate: &d}, "adapt")
			if err != nil {
				t.Fatal(err)
			}
			adaptedID, err := d.id()
			if err != nil {
				t.Fatal(err)
			}
			if tc.category == "skill" {
				delete(a.Skills, tc.template)
			}
			for _, skill := range r.Skills {
				if skill.ID == tc.template && skill.RequiredItem != "" {
					delete(a.Inventory, skill.RequiredItem)
				}
			}
			energy := a.Energy
			s, id := adventureFixtureStoryWithActor(t, r, a, tc.location)
			defer s.Close()
			entered := realmTestApply(t, s, id, realmExploreRequest("", tc.realm, RealmActionEnter, "", "enter"))
			realmExploreDo(t, s, id, entered.InstanceID, tc.realm, "approach", tc.method)
			after, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if after.Actor.Energy != energy-tc.energy {
				t.Fatal("adapted skill did not retain its frozen energy cost")
			}
			if tc.category == "equipment" && (after.Actor.Inventory[adaptedID] != 1 || after.Actor.Inventory[tc.template] != 0) {
				t.Fatal("held adapted rope was consumed or duplicated as its template")
			}
			if tc.category == "skill" && (after.Actor.Skills[adaptedID] != 1 || after.Actor.Skills[tc.template] != 0) {
				t.Fatal("exploration manufactured the unlearned original skill")
			}
			if tc.method == "repair" && after.Actor.Inventory["xinghai-i34"] != 0 {
				t.Fatal("adapted skill bypassed the actual wire cost")
			}
		})
	}
}
