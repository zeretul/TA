package interactive

import (
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

// A long authorized interval must not earn more mastery or experience than the
// same productive study in smaller segments. Extra calendar time is not XP.
func TestAdventureTrainingCannotConvertPostMasteryIdleHoursIntoRanks(t *testing.T) {
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		t.Run(pack, func(t *testing.T) {
			r := authoredAdventureRules(t, pack)
			a := NewAdventureActor(r)
			id := r.StartingSkills[0]
			cap := 10
			if r.Family == "wuxia" {
				cap = 4
			}
			var skill AdventureSkill
			for _, candidate := range r.Skills {
				if candidate.ID == id {
					skill = candidate
				}
			}
			needed := skill.TrainingHours * (cap*(cap+1)/2 - 1)
			exact, exactOut, err := ResolveAdventureAction(r, a, AdventureAction{Action: "train", SkillID: id, Hours: needed}, "exact-training")
			if err != nil {
				t.Fatal(err)
			}
			long, longOut, err := ResolveAdventureAction(r, a, AdventureAction{Action: "train", SkillID: id, Hours: 100000}, "long-training")
			if err != nil {
				t.Fatal(err)
			}
			if long.Rank != exact.Rank || long.Experience != exact.Experience || !reflect.DeepEqual(long.Training, exact.Training) || longOut.DurationSeconds != exactOut.DurationSeconds {
				t.Fatalf("idle hours manufactured progression: exact rank=%d xp=%d time=%d, long rank=%d xp=%d time=%d", exact.Rank, exact.Experience, exactOut.DurationSeconds, long.Rank, long.Experience, longOut.DurationSeconds)
			}
			segmented := a
			var seconds int64
			for hour := 0; hour < needed; hour++ {
				next, out, err := ResolveAdventureAction(r, segmented, AdventureAction{Action: "train", SkillID: id, Hours: 1}, "hourly-training")
				if err != nil {
					t.Fatal(err)
				}
				segmented = next
				seconds += out.DurationSeconds
			}
			if !reflect.DeepEqual(exact, segmented) || seconds != exactOut.DurationSeconds {
				t.Fatal("productive training depends on segmentation")
			}
			if _, _, err := ResolveAdventureAction(r, exact, AdventureAction{Action: "train", SkillID: id, Hours: 1}, "past-mastery"); err == nil {
				t.Fatal("mastery ceiling must still end this study")
			}
		})
	}
}

func TestAdventureBoundNPCEquipmentChangesRealAttackAndDefense(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = nil
	r.Items = append(r.Items, AdventureItem{ID: "armor", Name: "Armor", Kind: "armor", Scale: "person", Power: 8, Acquisition: "market", Price: 50})
	npc := NewAdventureActor(r)
	npc.Skills = map[string]int{}
	npc.Inventory = map[string]int{"weapon": 1, "armor": 1, "ammo": 3}
	npc.Equipment = map[string]string{}
	bindAdventureEncounter(a.Battle, "actual-npc", npc, r)
	bare, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "bare")
	if err != nil {
		t.Fatal(err)
	}
	npc.Equipment = map[string]string{"person:weapon": "weapon", "person:armor": "armor"}
	armedStart, _, err := ResolveAdventureAction(r, NewAdventureActor(r), AdventureAction{Action: "battle_start", OpponentID: "rival"}, "armed-start")
	if err != nil {
		t.Fatal(err)
	}
	bindAdventureEncounter(armedStart.Battle, "actual-npc", npc, r)
	armed, _, err := ResolveAdventureAction(r, armedStart, AdventureAction{Action: "battle_round", Choice: "attack"}, "armed")
	if err != nil {
		t.Fatal(err)
	}
	if armed.Health >= bare.Health || armed.Battle.Health <= bare.Battle.Health || armed.Battle.EnemyInventory["ammo"] != 2 {
		t.Fatalf("real equipped power had no effect or wrong ammunition: bare=%+v armed=%+v", bare, armed)
	}
}

func TestAdventureFullControlCannotEraseEveryEnemyResponse(t *testing.T) {
	r, a := enemyTacticsFixture(t, "wuxia")
	r.Skills = append(r.Skills, AdventureSkill{ID: "hold", Name: "Hold", Kind: "control", Scale: "person", Power: 20, Cost: 5, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "hold")
	r.Opponents[0].SkillIDs = nil
	a.Skills["hold"] = 1
	first, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "hold"}, "first-hold")
	if err != nil || first.Health != a.Health {
		t.Fatalf("first control must stop one reply: %v", err)
	}
	second, _, err := ResolveAdventureAction(r, first, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "hold"}, "repeated-hold")
	if err != nil {
		t.Fatal(err)
	}
	if second.Health >= first.Health {
		t.Fatal("repeating control erased every enemy response")
	}
	if second.Energy != first.Energy-5 || second.Battle.Health >= first.Battle.Health {
		t.Fatal("control follow-up must still pay and deal its real damage")
	}
}

func TestAdventureNPCControlLeavesPlayerARecoveryOpportunity(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Skills = append(r.Skills, AdventureSkill{ID: "hold", Name: "Hold", Kind: "control", Scale: "person", Power: 2, Cost: 1, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "hold")
	r.Opponents[0].SkillIDs = []string{"hold"}
	// Exercise a legal model-selected control, independently of heuristic utility.
	first, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "skill:hold"}, "npc-hold-1")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := ResolveAdventureAction(r, first, AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "skill:hold"}, "npc-hold-2")
	if err != nil {
		t.Fatal(err)
	}
	if second.Battle.Health != first.Battle.Health {
		t.Fatal("the first control did not cost the player's next offensive action")
	}
	third, _, err := ResolveAdventureAction(r, second, AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "skill:hold"}, "npc-hold-3")
	if err != nil {
		t.Fatal(err)
	}
	if third.Battle.Health >= second.Battle.Health {
		t.Fatal("NPC permanently removed the player's ability to respond")
	}
}

func TestAdventureAnonymousOpponentMasteryIsAuthoredAndRealNPCStillOverrides(t *testing.T) {
	r, _ := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = []string{"strike"}
	start := func() AdventureActor {
		a, _, err := ResolveAdventureAction(r, NewAdventureActor(r), AdventureAction{Action: "battle_start", OpponentID: "rival"}, "authored-start")
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	base := start()
	ordinary, _, err := ResolveAdventureAction(r, base, AdventureAction{Action: "battle_round", Choice: "guard"}, "base-skill")
	if err != nil {
		t.Fatal(err)
	}
	r.Opponents[0].SkillRanks = map[string]int{"strike": 3}
	trained := start()
	stronger, _, err := ResolveAdventureAction(r, trained, AdventureAction{Action: "battle_round", Choice: "guard"}, "trained-skill")
	if err != nil || stronger.Health >= ordinary.Health || stronger.Battle.EnemyEnergy != ordinary.Battle.EnemyEnergy {
		t.Fatalf("authored mastery not consumed: ordinary=%d trained=%d err=%v", ordinary.Health, stronger.Health, err)
	}
	npc := NewAdventureActor(r)
	npc.Skills = map[string]int{"strike": 1}
	bindAdventureEncounter(trained.Battle, "actual-rival", npc, r)
	if trained.Battle.EnemySkillRanks["strike"] != 1 {
		t.Fatal("authored anonymous mastery overrode actual learned NPC state")
	}
	r.Opponents[0].SkillRanks["cure"] = 1
	if err := r.Validate(); err == nil {
		t.Fatal("unlisted mastery invented an extra opponent technique")
	}
}

func TestAdventureControlRecoverySurvivesCommitRetryBranchAndColdReplay(t *testing.T) {
	r, _ := enemyTacticsFixture(t, "wuxia")
	r.Skills = append(r.Skills, AdventureSkill{ID: "hold", Name: "Hold", Kind: "control", Scale: "person", Power: 20, Cost: 5, RequiredRank: 1, TrainingHours: 1})
	r.StartingSkills = append(r.StartingSkills, "hold")
	r.Opponents[0].SkillIDs = nil
	r.Opponents[0].Location = "船坞"
	store, id := adventureTestStoryWithRules(t, r)
	defer func() { _ = store.Close() }()
	_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "The rival approaches.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	input := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "hold"}
	first, err := store.CommitAdventureAction(id, "main", "hold-first", before.Head, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Actor.Battle.EnemyControlRecovery != 1 {
		t.Fatal("first response protection not recorded")
	}
	retry, err := store.CommitAdventureAction(id, "main", "hold-first", before.Head, input)
	if err != nil || !reflect.DeepEqual(first, retry) {
		t.Fatal("retry changed control recovery", err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before control"})
	if err != nil {
		t.Fatal(err)
	}
	forked, err := store.AdventureWindow(id, fork.ID)
	if err != nil || forked.Actor.Battle.EnemyControlRecovery != 0 {
		t.Fatal("control recovery leaked into earlier branch", err)
	}
	root := store.Root()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store = NewStore(root)
	cold, err := store.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(cold.Actor, first.Actor) {
		t.Fatal("cold replay lost control recovery", err)
	}
	second, err := store.CommitAdventureAction(id, "main", "hold-second", cold.Head, input)
	if err != nil || second.Actor.Health >= first.Actor.Health {
		t.Fatal("restoring removed the opponent's response opportunity", err)
	}
}

func TestAdventurePhantomEnemyEquipmentCannotGrantPower(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = nil
	a.Battle.EnemyActorID = "actual-npc"
	bare, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "bare")
	if err != nil {
		t.Fatal(err)
	}
	a.Battle.EnemyEquipment = map[string]string{"person:weapon": "weapon"}
	phantom, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "phantom")
	if err != nil || phantom.Health != bare.Health || phantom.Battle.EnemyInventory["ammo"] != bare.Battle.EnemyInventory["ammo"] {
		t.Fatal("unowned slot granted damage or consumed phantom ammunition", err)
	}
}

func TestRealmPoweredGarmentCostReachesBattleCopies(t *testing.T) {
	stage := cultivation.Cultivation{Realm: cultivation.RealmCore, Stage: cultivation.StageEarly}
	catalog := cultivation.NewItemCatalogWithFirstVersion()
	armor, ok := catalog.Definition("equipment.core_robe")
	if !ok {
		t.Fatal("missing core armor")
	}
	entry := map[string]any{"名称": "core robe", "定义": armor.ID}
	copy := xiuxianEquipmentCopy(cultivation.ReferenceRules(),
		xiuxianFieldArmor, cultivation.EquipmentSlotGarment, entry, stage, true)
	if copy.OperatingManaCost != armor.OperatingManaCost(stage) || copy.OperatingManaCost <= 0 {
		t.Fatalf("armor battle copy dropped published cost: %+v", copy)
	}
}

func TestAdventureCompositeGuardPaysOnlyItsPublishedProtection(t *testing.T) {
	r, a := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = nil
	r.Skills = append(r.Skills, AdventureSkill{ID: "hybrid", Name: "Hybrid", Kind: "attack", Scale: "person", Power: 15, Cost: 10, RequiredRank: 1, TrainingHours: 2, Effects: []AdventureEffect{{Kind: "attack", Power: 15}, {Kind: "guard", Power: 1}}})
	r.StartingSkills = append(r.StartingSkills, "hybrid")
	a.Skills["hybrid"] = 1
	strike, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "strike"}, "strike")
	if err != nil {
		t.Fatal(err)
	}
	hybrid, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "hybrid"}, "hybrid")
	if err != nil {
		t.Fatal(err)
	}
	if hybrid.Health != strike.Health+1 || hybrid.Battle.Health != strike.Battle.Health {
		t.Fatalf("one guard point gave free full defensive stance: strikeHP=%d hybridHP=%d", strike.Health, hybrid.Health)
	}
	guarded, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "guard")
	if err != nil {
		t.Fatal(err)
	}
	if guarded.Health <= hybrid.Health {
		t.Fatal("dedicating the action to defense must still protect more than a one-point hybrid")
	}
}
