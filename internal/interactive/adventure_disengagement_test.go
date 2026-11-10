package interactive

import (
	"fmt"
	"reflect"
	"slices"
	"ta/internal/character"
	"testing"
)

func movementFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := wuxiaConditionFixture(t)
	for i := range r.Skills {
		if r.Skills[i].ID == "lanchuan-s26" {
			r.Skills[i].Mobility = 40
		}
	}
	return r, a
}

func TestAdventureDisengagementUsesPaidAbilityAndOpposedPursuit(t *testing.T) {
	r, a := movementFixture(t)
	plain, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", enemyChoice: "pursue"}, "plain-retreat")
	if err != nil || plain.Battle.Status != "active" || plain.Battle.PlayerRetreatProgress != 0 {
		t.Fatalf("unassisted actor outran dedicated equal pursuit: %+v %v", plain.Battle, err)
	}
	beforeEnergy := a.Energy
	first, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", SkillID: "lanchuan-s26", enemyChoice: "pursue"}, "paid-retreat-1")
	if err != nil || first.Battle.PlayerRetreatProgress <= 0 || first.Energy != beforeEnergy-9 || first.Health != a.Health {
		t.Fatalf("paid mobility was free, missing or attacked: %+v %v", first, err)
	}
	second, _, err := ResolveAdventureAction(r, first, AdventureAction{Action: "battle_round", Choice: "escape", SkillID: "lanchuan-s26", enemyChoice: "pursue"}, "paid-retreat-2")
	if err != nil || second.Battle.Status != "escaped" || second.Energy != beforeEnergy-18 || second.Currency != a.Currency || second.Experience != a.Experience || second.Defeated[second.Battle.OpponentID] {
		t.Fatalf("real escape minted victory or ignored costs: %+v %v", second, err)
	}
	a.Rank, a.Battle.EnemyRank = 40, 40
	high, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", enemyChoice: "pursue"}, "high-rank-not-auto")
	if err != nil || high.Battle.Status != "active" || high.Battle.PlayerRetreatProgress != 0 {
		t.Fatal("high rank became an automatic escape", err)
	}
}

func TestAdventureDisengagementEnemyCanLeaveAndCannotAttackForFree(t *testing.T) {
	r, a := movementFixture(t)
	r.Opponents[0].SkillIDs = []string{"lanchuan-s26"}
	a.Battle.EnemySkillRanks = map[string]int{"lanchuan-s26": 4}
	left, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "escape:lanchuan-s26"}, "enemy-leaves")
	if err != nil || left.Battle.Status != "enemy_escaped" || left.Health != a.Health || left.Battle.EnemyEnergy != a.Battle.EnemyEnergy-9 || left.Battle.EnemyLastSkillID != "lanchuan-s26" || left.Defeated[left.Battle.OpponentID] {
		t.Fatalf("enemy escape bypassed payment, caused a free strike or granted victory: %+v %v", left, err)
	}
	a.Skills["lanchuan-s26"] = 4
	pursued, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "pursue", SkillID: "lanchuan-s26", enemyChoice: "escape:lanchuan-s26"}, "matching-pursuit")
	if err != nil || pursued.Battle.Status != "active" || pursued.Battle.EnemyRetreatProgress != 0 || pursued.Energy != a.Energy-9 || pursued.Health != a.Health || pursued.Battle.Health != a.Battle.Health {
		t.Fatalf("equal paid pursuit did not intercept without free attacks: %+v %v", pursued, err)
	}
}

func TestAdventureDisengagementRefusesUnavailableMovementAtomically(t *testing.T) {
	for _, mode := range []string{"unlearned", "energy", "sealed", "softened"} {
		t.Run(mode, func(t *testing.T) {
			r, a := movementFixture(t)
			switch mode {
			case "unlearned":
				delete(a.Skills, "lanchuan-s26")
			case "energy":
				a.Energy = 8
			case "sealed":
				a.RushBlocked = 1
			case "softened":
				a.SoftLimbs = true
			}
			got, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", SkillID: "lanchuan-s26", enemyChoice: "pursue"}, "refused-movement")
			if err == nil || !reflect.DeepEqual(got, a) {
				t.Fatalf("unavailable movement spent state: %v", err)
			}
		})
	}
}

func TestAdventureCeasefireRequiresBothSidesAndKeepsHonestStalemate(t *testing.T) {
	r, _ := enemyTacticsFixture(t, "space")
	r.Opponents[0].SkillIDs = nil
	r.Opponents[0].WeaponID = ""
	r.Opponents[0].Attack = 1
	r.Opponents[0].Defense = 1000
	// Open the actual unarmed profile, rather than editing an author after
	// its previously wielded weapon has already entered the battle snapshot.
	a, _, err := ResolveAdventureAction(r, NewAdventureActor(r), AdventureAction{Action: "battle_start", OpponentID: "rival"}, "unarmed-standoff")
	if err != nil {
		t.Fatal(err)
	}
	for turn := 0; turn < 4; turn++ {
		var err error
		a, _, err = ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, "mutual-armor")
		if err != nil {
			t.Fatal(err)
		}
		if a.Battle.Status != "active" {
			t.Fatal("round count forced an outcome")
		}
	}
	if !a.Battle.EnemyCeasefireOffer || a.Battle.Health != 100 || a.Health != a.MaxHealth {
		t.Fatalf("zero-damage stalemate not retained or no ceasefire offered: %+v", a)
	}
	ended, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "accept_ceasefire"}, "accept-stalemate")
	if err != nil || ended.Battle.Status != "standoff" || ended.Currency != a.Currency || ended.Experience != a.Experience || len(ended.Defeated) != 0 {
		t.Fatalf("ceasefire awarded victory: %+v %v", ended, err)
	}
	r, a = enemyTacticsFixture(t, "space")
	rejected, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "ceasefire", enemyChoice: "ordinary"}, "unilateral")
	if err != nil || rejected.Battle.Status != "active" || rejected.Health >= a.Health || !rejected.Battle.PlayerCeasefireOffer {
		t.Fatal("one side forced safe ceasefire", err)
	}
	if _, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "accept_ceasefire"}, "imaginary-offer"); err == nil {
		t.Fatal("accepted an unspoken offer")
	}
}

func TestAdventureCeasefireColdReplayAndPriorBranch(t *testing.T) {
	r, _ := enemyTacticsFixture(t, "wuxia")
	r.Opponents[0].Location = "船坞"
	store, id := adventureTestStoryWithRules(t, r)
	defer func() { _ = store.Close() }()
	_, _, err := store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "对手上前。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "rival"}}}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := store.AdventureWindow(id, "main")
	offer, err := store.CommitAdventureActionWithEnemyDecision(id, "main", "offer-round", before.Head, AdventureAction{Action: "battle_round", Choice: "guard"}, map[string]string{"opponent": "ceasefire"}, "ai")
	if err != nil || !offer.Actor.Battle.EnemyCeasefireOffer {
		t.Fatal("real offer not committed", err)
	}
	root := store.Root()
	store.Close()
	store = NewStore(root)
	current, _ := store.AdventureWindow(id, "main")
	action := AdventureAction{Action: "battle_round", Choice: "accept_ceasefire"}
	ended, err := store.CommitAdventureAction(id, "main", "accept-round", current.Head, action)
	if err != nil || ended.Actor.Battle.Status != "standoff" {
		t.Fatal("reopened offer could not be accepted", err)
	}
	repeat, err := store.CommitAdventureAction(id, "main", "accept-round", current.Head, action)
	if err != nil || !reflect.DeepEqual(repeat, ended) {
		t.Fatal("accepted ceasefire did not replay exactly", err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before ceasefire"})
	if err != nil {
		t.Fatal(err)
	}
	old, _ := store.AdventureWindow(id, fork.ID)
	if old.Actor.Battle.EnemyCeasefireOffer || old.Actor.Battle.Status != "active" {
		t.Fatal("future agreement leaked into old branch")
	}
}

func TestEnemyDecisionPersonalFleetObservationUsesBodyNotParkedAsset(t *testing.T) {
	actor := AdventureActor{Health: 100, MaxHealth: 100, Battle: &AdventureBattle{Scale: "ship", PlayerScale: "person"}, Assets: map[string]AdventureAsset{"parked": {Health: 0, MaxHealth: 1000}}}
	if got := adventureVisibleOpponent(actor); got[0] != "可见对手：未见伤势。" {
		t.Fatal(got)
	}
	actor.Health = 20
	if got := adventureVisibleOpponent(actor); got[0] != "可见对手：重伤。" {
		t.Fatal(got)
	}
}

func TestAdventureDisengagementKeepsCurrentControlThroughInterception(t *testing.T) {
	r, a := movementFixture(t)
	a.Battle.EnemyRank = 40
	active, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", enemyChoice: "ordinary"}, "active-interceptor")
	if err != nil || active.Battle.Status != "active" {
		t.Fatal("fixture should be held by the available interceptor", err)
	}
	a.Battle.EnemyControl = 1
	escaped, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", enemyChoice: "pursue"}, "controlled-interceptor")
	if err != nil || escaped.Battle.Status != "escaped" || escaped.Health != a.Health {
		t.Fatalf("a skipped controlled turn regained free interception: %+v %v", escaped, err)
	}
}

func TestAdventureDisengagementTravelerUsesActualLearnedSpatialAbility(t *testing.T) {
	r := authoredAdventureRules(t, "xinghai-689")
	var traveler AdventureActor
	for _, npc := range r.NPCs {
		if npc.CharacterSourceKey == "xinghai-p48" {
			traveler = npc.Actor
		}
	}
	if traveler.Skills["xinghai-space-slip"] < 1 {
		t.Fatal("current authored traveler has no real spatial movement")
	}
	a := NewAdventureActor(r)
	a.Rank = traveler.Rank
	a.Health, a.MaxHealth = 10000, 10000
	a.Battle = &AdventureBattle{ID: "traveler-check", OpponentID: "traveler-check", Scale: "person", Status: "active", Health: traveler.Health, MaxHealth: traveler.MaxHealth, Attack: 10 + traveler.Rank*3, EnemyActorID: "known-traveler", EnemyRank: traveler.Rank, EnemyEnergy: traveler.Energy, EnemyMaxEnergy: traveler.MaxEnergy, EnemySkillRanks: traveler.Skills, EnemyInventory: traveler.Inventory, EnemyEquipment: traveler.Equipment}
	choice := "escape:xinghai-space-slip"
	found := false
	for _, candidate := range adventureEnemyDecisionCandidates(r, &a) {
		found = found || candidate.ID == choice
	}
	if !found {
		t.Fatal("known spatial escape absent from model's legal choices")
	}
	left, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "pursue", enemyChoice: choice}, "traveler-paid-exit")
	if err != nil || left.Battle.Status != "enemy_escaped" || left.Battle.EnemyEnergy != traveler.Energy-18 || left.Battle.EnemyLastSkillID != "xinghai-space-slip" || left.Health != a.Health {
		t.Fatalf("spatial ability was not paid and actually used: %+v %v", left, err)
	}
	// Merely having the same rank and source name does not grant the ability.
	without := *a.Battle
	without.EnemySkillRanks = map[string]int{}
	a.Battle = &without
	if _, _, legal := adventureEnemyManeuverChoice(r, &a, choice); legal {
		t.Fatal("rank alone granted the traveler's missing learned ability")
	}
}

func TestAdventureDisengagementNamedExitCommitsPresenceAndRestoresBranch(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	var op AdventureOpponent
	for _, candidate := range r.Opponents {
		if candidate.RequiresPresence && candidate.Scale == "person" {
			op = candidate
			break
		}
	}
	if op.ID == "" {
		t.Fatal("missing named encounter fixture")
	}
	for i := range r.NPCs {
		if r.NPCs[i].CharacterSourceKey == op.CharacterSourceKey {
			r.NPCs[i].Actor.Skills["lanchuan-s26"] = 4
		}
	}
	for i := range r.Skills {
		if r.Skills[i].ID == "lanchuan-s26" {
			r.Skills[i].Mobility = 90
		}
	}
	store, id, actorID := npcStageStory(t, r, op, true)
	_, err := store.AppendCharacterFacts(id, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "opening", Scene: character.Scene{Location: op.Location}, Profiles: []character.Profile{{ID: actorID, Name: op.Name, SourceKey: op.CharacterSourceKey, Revision: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	start := func(s *Store, branch string) error {
		_, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: branch, User: "Exchange", Narrative: "A cautious exchange begins.", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "Exchange", Adventure: &AdventureAction{Action: "battle_start", OpponentID: op.ID}}}})
		return e
	}
	if err = start(store, "main"); err != nil {
		t.Fatal(err)
	}
	before, err := store.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before withdrawal"})
	if err != nil {
		t.Fatal(err)
	}
	// A failed retreat does not remove the established person.
	ctx, _ := store.StoryContext(id, "main")
	failed := *before.Actor.Battle
	failed.Status = "active"
	failed.EnemyLastAction = "escape"
	p := &storyBranchProjection{State: ctx.Snapshot.State}
	for _, operation := range adventureEnemySettlementOps(p, r, &failed, "failed-withdrawal") {
		if operation.ActorID == DefaultStoryContextActorID {
			t.Fatal("unsuccessful escape removed scene presence")
		}
	}
	result, err := store.CommitAdventureActionWithEnemyDecision(id, "main", "named-withdrawal", before.Head, AdventureAction{Action: "battle_round", Choice: "guard"}, map[string]string{actorID: "escape:lanchuan-s26"}, "ai")
	if err != nil || result.Actor.Battle.Status != "enemy_escaped" {
		t.Fatalf("named escape failed: %+v %v", result.Actor.Battle, err)
	}
	reopened := NewStore(store.Root())
	defer reopened.Close()
	current, err := reopened.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(StorySceneFromState(current.Snapshot.State, "").Present, actorID) {
		t.Fatal("escaped NPC remained in committed scene after cold reopen")
	}
	npc, err := AdventureActorFromState(current.Snapshot.State, actorID)
	if err != nil || npc.Energy != result.Actor.Battle.EnemyEnergy {
		t.Fatal("departure lost the NPC's paid resources", err)
	}
	if err = start(reopened, "main"); err == nil {
		t.Fatal("departed person was summoned back by an opponent ID")
	}
	prior, err := reopened.StoryContext(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(StorySceneFromState(prior.Snapshot.State, "").Present, actorID) {
		t.Fatal("future absence leaked into prior branch")
	}
}

func TestAdventureDisengagementPersonalAbilityPaysUpkeepAndRespectsBlockade(t *testing.T) {
	for _, tc := range []struct {
		opponent string
		escaped  bool
	}{{"xinghai-raider-patrol", true}, {"xinghai-strategic-blockade", false}} {
		t.Run(tc.opponent, func(t *testing.T) {
			r, a := personalFleetFixture(t, 33, "xinghai-space-slip", 6)
			a = startPersonalFleet(t, r, a, tc.opponent, "xinghai-space-slip")
			before := a.Energy
			next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "escape", enemyChoice: "ordinary"}, "spatial-retreat")
			if err != nil {
				t.Fatal(err)
			}
			if (next.Battle.Status == "escaped") != tc.escaped || next.Energy != before-18 || out.DurationSeconds != 20 || next.Defeated[tc.opponent] || next.Currency != a.Currency {
				t.Fatalf("spatial withdrawal ignored real upkeep, blockade or no-reward rule: %+v", next)
			}
			t.Logf("opponent=%s status=%s progress=%d health=%d/%d paid_energy=%d", tc.opponent, next.Battle.Status, next.Battle.PlayerRetreatProgress, next.Health, next.MaxHealth, before-next.Energy)
		})
	}
}

func TestAdventureDisengagementShipOverclockIsOpposedNotRankAutomatic(t *testing.T) {
	r, a := spaceOverclockFixture(t)
	seed := ""
	for i := 0; i < 200; i++ {
		candidate := fmt.Sprintf("opposed-overclock-%d", i)
		roll := adventurePercentile(candidate, "overclock-escape")
		if adventurePercentile(candidate, "engine-melt") >= 10 && roll >= 70 && roll < 90 {
			seed = candidate
			break
		}
	}
	if seed == "" {
		t.Fatal("no deterministic comparison seed")
	}
	action := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "xinghai-s09"}
	weak, _, err := ResolveAdventureAction(r, a, action, seed)
	if err != nil || weak.Battle.Status != "escaped" {
		t.Fatal("weak interception did not match the deterministic fixture", err)
	}
	strongBattle := *a.Battle
	strongBattle.ShipCombat = &AdventureShipCombatProfile{HullThreshold: 1, Tracking: 95, Interdiction: 95, VesselCount: 1}
	// This comparison freezes a different encounter profile in both the catalog
	// and active battle; it does not tamper with a saved profile at runtime.
	for i := range r.Opponents {
		if r.Opponents[i].ID == strongBattle.OpponentID {
			r.Opponents[i].ShipCombat = strongBattle.ShipCombat
		}
	}
	a.Battle = &strongBattle
	strong, _, err := ResolveAdventureAction(r, a, action, seed)
	if err != nil || strong.Battle.Status != "defeat" {
		t.Fatalf("same emergency thrust ignored real blockade: %+v %v", strong.Battle, err)
	}
	if strong.Assets[strong.Battle.AssetID].Fuel != 0 || strong.Energy != 0 {
		t.Fatal("failed emergency escape refunded costs")
	}
}
