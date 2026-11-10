package interactive

import (
	"fmt"
	"reflect"
	"testing"
)

func personalFleetFixture(t *testing.T, rank int, skill string, mastery int) (AdventureRules, AdventureActor) {
	t.Helper()
	r := authoredAdventureRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Rank = rank
	a.MaxHealth, a.Health = 100+20*(rank-1), 100+20*(rank-1)
	a.MaxEnergy, a.Energy = 100+10*(rank-1), 100+10*(rank-1)
	if skill != "" {
		a.Skills[skill] = mastery
	}
	return r, a
}

func startPersonalFleet(t *testing.T, r AdventureRules, a AdventureActor, opponent, capability string) AdventureActor {
	t.Helper()
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: opponent, Scale: "person", SkillID: capability}, "personal-start")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPersonalFleetRequiresLearnedCapabilityAndFrozenTarget(t *testing.T) {
	r, a := personalFleetFixture(t, 41, "", 0)
	in := AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-frigate", Scale: "person", SkillID: "xinghai-space-dominion"}
	if _, _, err := ResolveAdventureAction(r, a, in, "rank-alone"); err == nil {
		t.Fatal("rank alone bypassed the actual learned capability")
	}
	a.Skills["xinghai-space-dominion"] = 1
	a.Rank = 33
	if _, _, err := ResolveAdventureAction(r, a, in, "unqualified"); err == nil {
		t.Fatal("unqualified capability entered combat")
	}
	a.Rank = 41
	a.Energy = 0
	if _, _, err := ResolveAdventureAction(r, a, in, "empty"); err == nil {
		t.Fatal("empty energy entered personal vacuum combat")
	}
	a.Energy = a.MaxEnergy
	in.OpponentID = "xinghai-e05"
	if _, _, err := ResolveAdventureAction(r, a, in, "wrong-target"); err == nil {
		t.Fatal("unregistered cross-scale encounter bypassed its conditions")
	}
	// The old same-scale route continues to need an actual deployed ship.
	in.OpponentID, in.Scale = "xinghai-raider-frigate", "ship"
	if _, _, err := ResolveAdventureAction(r, a, in, "no-free-ship"); err == nil {
		t.Fatal("personal capability fabricated a ship")
	}
}

func TestPersonalFleetMatchupsDependOnBuildAndFormation(t *testing.T) {
	cases := []struct {
		name, skill, opponent, choice, attackSkill string
		rank, mastery                              int
		status                                     string
	}{
		{"A breaks light vessel", "xinghai-space-breach", "xinghai-raider-frigate", "attack", "", 33, 6, "victory"},
		{"A defeats light patrol", "xinghai-space-breach", "xinghai-raider-patrol", "attack", "", 33, 6, "victory"},
		{"A loses to interceptor", "xinghai-space-breach", "xinghai-heavy-interceptor", "attack", "", 33, 6, "defeat"},
		{"mobility is not hull force", "xinghai-space-slip", "xinghai-raider-patrol", "attack", "", 33, 6, "defeat"},
		{"Beyond A breaks flotilla", "xinghai-space-dominion", "xinghai-hunter-flotilla", "attack", "", 41, 6, "victory"},
		{"Beyond A is not universal victory", "xinghai-space-dominion", "xinghai-strategic-blockade", "attack", "", 41, 6, "defeat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, a := personalFleetFixture(t, tc.rank, tc.skill, tc.mastery)
			a = startPersonalFleet(t, r, a, tc.opponent, tc.skill)
			energyBefore, moneyBefore := a.Energy, a.Currency
			var duration int64
			for i := 0; i < 60 && a.Battle.Status == "active"; i++ {
				next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: tc.choice, SkillID: tc.attackSkill, enemyChoice: "ordinary"}, fmt.Sprintf("match-%d", i))
				if err != nil {
					t.Fatal(err)
				}
				a = next
				duration += out.DurationSeconds
			}
			if a.Battle.Status != tc.status {
				t.Fatalf("status %s != %s: %+v", a.Battle.Status, tc.status, a)
			}
			if a.Energy >= energyBefore || duration != int64(a.Battle.Round)*adventureRoundSeconds {
				t.Fatal("combat skipped real energy or time")
			}
			if tc.status != "victory" && (a.Currency != moneyBefore || a.Defeated[tc.opponent]) {
				t.Fatal("failure produced reward")
			}
			t.Logf("status=%s rounds=%d hp=%d/%d energy=%d/%d enemy=%d time=%ds", a.Battle.Status, a.Battle.Round, a.Health, a.MaxHealth, a.Energy, energyBefore, a.Battle.Health, duration)
		})
	}
}

func TestPersonalFleetProtectsReserveAssetsAndChargesCombinedCosts(t *testing.T) {
	r, a := personalFleetFixture(t, 33, "xinghai-space-breach", 6)
	// This ship remains parked; it must not provide armor, fuel, damage or HP.
	var ship AdventureItem
	for _, item := range r.Items {
		if item.Kind == "ship" && !item.SupportOnly {
			ship = item
			break
		}
	}
	a.Inventory[ship.ID] = 1
	a.Equipment["ship:asset"] = ship.ID
	a.Assets[ship.ID] = AdventureAsset{Health: ship.Durability, MaxHealth: ship.Durability, Shield: ship.Power, Fuel: 77}
	reserve := a.Assets[ship.ID]
	a = startPersonalFleet(t, r, a, "xinghai-heavy-interceptor", "xinghai-space-breach")
	if a.Battle.AssetID != "" || AdventurePlayerBattleScale(a.Battle) != "person" || a.Battle.Scale != "ship" {
		t.Fatal("mixed-scale opening aliased the ship")
	}
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "incoming")
	if err != nil {
		t.Fatal(err)
	}
	if next.Health >= a.Health || next.Assets[ship.ID] != reserve || out.DurationSeconds != 20 {
		t.Fatal("wrong resource pool received incoming fire")
	}
	// Skill cost must be affordable after, not instead of, round upkeep.
	a.Energy = 30
	before := a
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-space-breach"}, "combined-cost"); err == nil {
		t.Fatal("round upkeep replaced technique cost")
	}
	if !reflect.DeepEqual(a, before) {
		t.Fatal("refused combined cost mutated input")
	}
}

func TestPersonalFleetNoChipDamageOrBiologicalControlAgainstHulls(t *testing.T) {
	r, a := personalFleetFixture(t, 33, "xinghai-space-slip", 6)
	a = startPersonalFleet(t, r, a, "xinghai-raider-patrol", "xinghai-space-slip")
	before := a.Battle.Health
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "no-chip")
	if err != nil {
		t.Fatal(err)
	}
	if next.Battle.Health != before {
		t.Fatal("minimum damage bypassed hull threshold")
	}
	// Real carried personal explosives retain their authored scale and force.
	r.Items = append(r.Items, AdventureItem{ID: "test-grenade", Name: "Grenade", Kind: "consumable", Scale: "person", Effect: "attack", Power: 100, Acquisition: "reward"})
	a.Inventory["test-grenade"] = 1
	next, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "item", ItemID: "test-grenade"}, "grenade")
	if err != nil || next.Battle.Health != before || next.Inventory["test-grenade"] != 0 {
		t.Fatalf("grenade inherited structural capability: %v", err)
	}
	r.Skills = append(r.Skills, AdventureSkill{ID: "test-control", Name: "Pressure", Kind: "control", Scale: "person", Power: 20, Cost: 5, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "test-control")
	a.Skills["test-control"] = 1
	if _, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "test-control"}, "control"); err == nil {
		t.Fatal("personal control shut down a fleet")
	}
}

func TestPersonalFleetLegacyScaleAndCapabilityTampering(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	a.Inventory["ship"] = 1
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "deploy", ItemID: "ship"}, "legacy-deploy")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: "frigate"}, "legacy-start")
	if err != nil {
		t.Fatal(err)
	}
	a.Battle.PlayerScale = ""
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "legacy-round")
	if err != nil || next.Assets["ship"].Fuel != 99 || next.Health != a.Health {
		t.Fatalf("legacy asset fight changed pools: %v", err)
	}
	r, a = personalFleetFixture(t, 33, "xinghai-space-breach", 6)
	a = startPersonalFleet(t, r, a, "xinghai-raider-patrol", "xinghai-space-breach")
	a.Battle.ShipCombat.HullThreshold = 0
	if ValidateAdventureActor(r, a) == nil {
		t.Fatal("restored fleet defense bypassed frozen source")
	}
}

func TestPersonalFleetFormalClockRetryBranchAndColdReplay(t *testing.T) {
	r, a := personalFleetFixture(t, 33, "xinghai-space-breach", 6)
	store, id := adventureFixtureStoryWithActor(t, r, a, "碎石带流亡者补给哨站")
	defer store.Close()
	_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "以天灾破舰力场独自迎战三舰编队", Narrative: "你离开舰舱，力场在真空中撑开。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "以天灾破舰力场独自迎战三舰编队", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "xinghai-raider-patrol", Scale: "person", SkillID: "xinghai-space-breach"}}}})
	if err != nil {
		t.Fatal(err)
	}
	start, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: start.Head, Title: "before personal exchange"})
	if err != nil {
		t.Fatal(err)
	}
	in := AdventureAction{Action: "battle_round", Choice: "attack"}
	if _, err = store.CommitAdventureAction(id, "main", "personal-round", start.Head, in); err != nil {
		t.Fatal(err)
	}
	current, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if current.ClockSeconds-start.ClockSeconds != 20 || current.Actor.Energy != start.Actor.Energy-20 || current.Actor.Battle.PlayerScale != "person" || current.Actor.Battle.PersonalCombatSkillID != "xinghai-space-breach" {
		t.Fatal("formal exchange did not preserve scales, time or upkeep")
	}
	if _, err = store.CommitAdventureAction(id, "main", "personal-round", start.Head, in); err != nil {
		t.Fatal(err)
	}
	retry, err := store.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(current.Actor, retry.Actor) || retry.ClockSeconds != current.ClockSeconds {
		t.Fatal("retry charged twice")
	}
	old, err := store.AdventureWindow(id, fork.ID)
	if err != nil || !reflect.DeepEqual(old.Actor, start.Actor) {
		t.Fatalf("fork changed: %v", err)
	}
	root := store.Root()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	cold := NewStore(root)
	defer cold.Close()
	restored, err := cold.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(restored.Actor, current.Actor) || restored.ClockSeconds != current.ClockSeconds {
		t.Fatalf("cold replay changed mixed scale: %v", err)
	}
}

func TestPersonalFleetCatalogRejectsFreeOrInvalidCapabilities(t *testing.T) {
	for _, change := range []func(*AdventureRules){
		func(r *AdventureRules) {
			for i := range r.Skills {
				if r.Skills[i].SpaceCombat != nil {
					v := *r.Skills[i].SpaceCombat
					v.EnergyPerRound = 0
					r.Skills[i].SpaceCombat = &v
					return
				}
			}
		},
		func(r *AdventureRules) {
			for i := range r.Opponents {
				if r.Opponents[i].ShipCombat != nil {
					v := *r.Opponents[i].ShipCombat
					v.Tracking = 101
					r.Opponents[i].ShipCombat = &v
					return
				}
			}
		},
		func(r *AdventureRules) {
			for i := range r.Opponents {
				if r.Opponents[i].ShipCombat != nil {
					r.Opponents[i].CharacterSourceKey = "xinghai-p01"
					return
				}
			}
		},
	} {
		r := authoredAdventureRules(t, "xinghai-689")
		change(&r)
		if r.Validate() == nil {
			t.Fatal("invalid personal/ship capability accepted")
		}
	}
}

func TestPersonalFleetCapabilityRequiresPaidSourceAndStudy(t *testing.T) {
	r, a := personalFleetFixture(t, 33, "", 0)
	in := AdventureAction{Action: "learn", SkillID: "xinghai-space-breach", Hours: 48}
	if _, _, err := ResolveAdventureAction(r, a, in, "no-source"); err == nil {
		t.Fatal("deep-space skill bypassed its source/foundation")
	}
	manual := AdventureItem{}
	for _, item := range r.Items {
		if item.ID == "xinghai-advanced-manual-5" {
			manual = item
		}
	}
	a.Currency = manual.Price
	a, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "buy", ItemID: manual.ID, MarketID: r.Markets[0].ID}, "buy-manual")
	if err != nil {
		t.Fatal(err)
	}
	a, foundation, err := ResolveAdventureAction(r, a, AdventureAction{Action: "learn", SkillID: "xinghai-foundation-5", Hours: 28}, "foundation")
	if err != nil {
		t.Fatal(err)
	}
	a, out, err := ResolveAdventureAction(r, a, in, "paid-study")
	if err != nil || a.Skills["xinghai-space-breach"] != 1 || out.DurationSeconds != 48*3600 || foundation.DurationSeconds != 28*3600 || a.Currency != 0 {
		t.Fatalf("capability skipped source/time: %v", err)
	}
	t.Logf("existing rank qualified fixture paid manual=%d and study=%dh, mastery=%d", manual.Price, (foundation.DurationSeconds+out.DurationSeconds)/3600, a.Skills["xinghai-space-breach"])
}

func TestPersonalFleetPersonSkillsHealBodyAndCannotBorrowMountedSystems(t *testing.T) {
	r, a := personalFleetFixture(t, 33, "xinghai-space-breach", 3)
	r.Skills = append(r.Skills, AdventureSkill{ID: "person-recovery", Name: "Recovery", Kind: "heal", Scale: "person", Power: 20, Cost: 5, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "person-recovery")
	a.Skills["person-recovery"] = 2
	a.Health = 300
	a = startPersonalFleet(t, r, a, "xinghai-raider-frigate", "xinghai-space-breach")
	next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "person-recovery", enemyChoice: "ordinary"}, "body-recovery")
	if err != nil || next.Health != 340 || next.Energy != a.Energy-25 || next.Battle.Health != a.Battle.Health {
		t.Fatalf("personal healing used the enemy scale or skipped cost: %v", err)
	}
	r.Skills = append(r.Skills, AdventureSkill{ID: "ship-system", Name: "Ship cannon", Kind: "attack", Scale: "ship", Power: 100, Cost: 1, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "ship-system")
	a.Skills["ship-system"] = 1
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "ship-system"}, "borrowed-cannon"); err == nil {
		t.Fatal("personal combat borrowed a mounted skill")
	}
}

func TestPersonalFleetRevisionSixKeepsExistingTasksAndRoutes(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	a := NewAdventureActor(r)
	a.Currency = 1000
	values := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		values[op.FieldID] = op.Value
	}
	location := journeyPlaceLabel("xinghai-l21")
	system := StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, "xinghai-689"), AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: location}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}}}}
	store := NewStore(t.TempDir())
	defer store.Close()
	meta, err := store.CreateStory(CreateStoryRequest{Title: "Revision six gameplay compatibility", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "xinghai-689", ContentVersion: 6, Scope: "xinghai-689-v6"}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.AppendTurnWithState(meta.ID, AppendTurnWithStateRequest{BranchID: "main", User: "留意公开求救脉冲", Narrative: "你听到了值班台上的重复求救脉冲。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.QuestStateFor(meta.ID, "main")
	if err != nil || !questAvailable(state, "xinghai-repeating-beacon") {
		t.Fatalf("new bound revision lost an existing task: %v", err)
	}
	if _, err = store.ApplyQuest(meta.ID, QuestRequest{BranchID: "main", TaskID: "xinghai-repeating-beacon", Action: QuestActionAccept}); err != nil {
		t.Fatal(err)
	}
	routes, err := store.JourneyOptionsFor(meta.ID, "main")
	if err != nil || len(routes.Routes) == 0 {
		t.Fatalf("new bound revision lost existing routes: %v", err)
	}
	binding := *meta.PackBinding
	binding.ContentVersion = 999
	unknown := StoryMeta{PackBinding: &binding}
	if storyPackAllows(unknown, "xinghai-689", "4") {
		t.Fatal("unknown future revision admitted existing tasks")
	}
}
