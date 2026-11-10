package interactive

import (
	"reflect"
	"testing"
)

func wuxiaPowerFixture(t *testing.T) (AdventureRules, AdventureActor) {
	t.Helper()
	r, a := wuxiaConditionFixture(t)
	for _, id := range []string{"lanchuan-s03", "lanchuan-s07", "lanchuan-s10", "lanchuan-s11"} {
		a.Skills[id] = 1
	}
	r.Opponents[0].Attack = 100
	a.Battle.Attack = 100
	return r, a
}

func TestWuxiaReflectionConsumesActualIncomingForceAndLimitsOverpower(t *testing.T) {
	r, a := wuxiaPowerFixture(t)
	cast := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s03"}
	strong, _, e := ResolveAdventureAction(r, a, cast, "reflect-strong")
	if e != nil {
		t.Fatal(e)
	}
	weak := a
	copyBattle := *a.Battle
	weak.Battle = &copyBattle
	weak.Battle.Attack = 30
	small, _, e := ResolveAdventureAction(r, weak, cast, "reflect-weak")
	if e != nil {
		t.Fatal(e)
	}
	if a.Battle.Health-strong.Battle.Health <= a.Battle.Health-small.Battle.Health || strong.Health >= a.Health {
		t.Fatal("reflection must depend on actual incoming force, with remaining personal harm")
	}
	r.Opponents[0].SkillIDs = []string{"lanchuan-s12"}
	quietCast := cast
	quietCast.enemyChoice = "skill:lanchuan-s12"
	quiet, _, e := ResolveAdventureAction(r, a, quietCast, "reflect-no-force")
	if e != nil {
		t.Fatal(e)
	}
	if quiet.Battle.Health != a.Battle.Health || quiet.Health != a.Health {
		t.Fatal("a seal with no incoming strike generated reflected damage")
	}
	r.Opponents[0].SkillIDs = nil
	a.Rank = 7
	over, _, e := ResolveAdventureAction(r, a, cast, "overpowered")
	if e != nil {
		t.Fatal(e)
	}
	if over.Battle.Health != a.Battle.Health || a.Health-over.Health <= a.Battle.Attack-a.Rank || over.Injury <= a.Injury {
		t.Fatal("a much stronger opponent was reflected without actual backlash")
	}
}

func TestWuxiaEnergyDrainConservesPoolsAndHonorsExistingZenCounter(t *testing.T) {
	r, a := wuxiaPowerFixture(t)
	a.Energy = 20
	a.Battle.EnemyEnergy = 5
	r.Opponents[0].SkillIDs = []string{"lanchuan-s12"}
	cast := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s07"}
	drained, _, e := ResolveAdventureAction(r, a, cast, "drain")
	if e != nil {
		t.Fatal(e)
	}
	if drained.Energy != 13 || drained.Battle.EnemyEnergy != 0 || drained.Battle.Health != a.Battle.Health || drained.RushBlocked != 0 || drained.Injury <= a.Injury {
		t.Fatalf("drain did not transfer just the existing five units and change the enemy's next skill eligibility: own%d enemy%d", drained.Energy, drained.Battle.EnemyEnergy)
	}
	r.Opponents[0].SkillIDs = []string{"lanchuan-s02"}
	blocked, _, e := ResolveAdventureAction(r, a, cast, "zen-counter")
	if e != nil {
		t.Fatal(e)
	}
	if blocked.Battle.EnemyEnergy != a.Battle.EnemyEnergy || blocked.Energy != a.Energy-12 || blocked.Injury <= drained.Injury || blocked.Health >= drained.Health {
		t.Fatal("the source Zen counter did not prevent transfer and cause backlash")
	}
	r.Opponents[0].SkillIDs = nil
	a.Energy = a.MaxEnergy
	a.Battle.EnemyEnergy = 100
	full, _, e := ResolveAdventureAction(r, a, cast, "full-pool")
	if e != nil {
		t.Fatal(e)
	}
	if full.Energy != full.MaxEnergy || full.Battle.EnemyEnergy != 88 {
		t.Fatal("drain created energy beyond remaining pool capacity")
	}
}

func TestWuxiaSevenInjuriesPaysHarmEvenOnVictoryWithFoundationMitigation(t *testing.T) {
	r, a := wuxiaPowerFixture(t)
	a.Health = 100
	a.MaxHealth = 100
	a.Battle.Health = 1
	cast := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s11"}
	bare, _, e := ResolveAdventureAction(r, a, cast, "seven-unprotected")
	if e != nil {
		t.Fatal(e)
	}
	if bare.Health >= a.Health || bare.Injury <= a.Injury || bare.Battle.Status != "victory" {
		t.Fatal("energy cost alone replaced the required full-power self-injury")
	}
	a.Skills["lanchuan-s01"] = 2
	protected, _, e := ResolveAdventureAction(r, a, cast, "seven-protected")
	if e != nil {
		t.Fatal(e)
	}
	if protected.Health <= bare.Health || protected.Health >= a.Health || protected.Injury >= bare.Injury || protected.Injury == 0 {
		t.Fatal("deep protective foundation must mitigate, not erase, full-power injury")
	}
	delete(a.Skills, "lanchuan-s01")
	a.Health = 1
	if got, _, e := ResolveAdventureAction(r, a, cast, "seven-unaffordable"); e == nil || !reflect.DeepEqual(got, a) {
		t.Fatal("unaffordable self-injury delivered a free final blow or partially charged costs")
	}
}

func TestWuxiaPowerNPCResponsesUseTheSameFinitePricesAndCounters(t *testing.T) {
	r, a := wuxiaPowerFixture(t)
	r.Opponents[0].SkillIDs = []string{"lanchuan-s03"}
	reflected, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack", enemyChoice: "skill:lanchuan-s03"}, "npc-reflect")
	if e != nil {
		t.Fatal(e)
	}
	if reflected.Battle.Health != a.Battle.Health-10 || reflected.Health != a.Health-30 || reflected.Battle.EnemyEnergy != 88 {
		t.Fatal("NPC did not reflect the actual forty-unit incoming blow with its own skill price")
	}
	still, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "npc-no-force")
	if e != nil {
		t.Fatal(e)
	}
	r.Opponents[0].SkillIDs = nil
	ordinary, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard"}, "npc-ordinary")
	if e != nil {
		t.Fatal(e)
	}
	if still.Battle.Health != a.Battle.Health || still.Health != ordinary.Health || still.Battle.EnemyEnergy != a.Battle.EnemyEnergy || still.Battle.EnemyLastSkillID != "" {
		t.Fatal("NPC must skip empty reflection and use only its ordinary attack without the skill price")
	}
	r.Opponents[0].SkillIDs = []string{"lanchuan-s07"}
	a.Energy = 400
	drained, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:lanchuan-s07"}, "npc-drain")
	if e != nil {
		t.Fatal(e)
	}
	if drained.Energy != 388 || drained.Battle.EnemyEnergy != 100 || drained.Health != a.Health || drained.Battle.EnemyInjury == 0 {
		t.Fatal("NPC transfer did not conserve capacity and pay its foreign-energy injury")
	}
	a.Skills["lanchuan-s02"] = 1
	countered, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:lanchuan-s07"}, "player-zen")
	if e != nil {
		t.Fatal(e)
	}
	if countered.Energy != a.Energy || countered.Battle.Health >= a.Battle.Health || countered.Battle.EnemyEnergy != 88 {
		t.Fatal("player's actually learned Zen foundation did not counter the NPC transfer")
	}
	r.Opponents[0].SkillIDs = []string{"lanchuan-s11"}
	hurt, _, e := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "guard", enemyChoice: "skill:lanchuan-s11"}, "npc-seven")
	if e != nil {
		t.Fatal(e)
	}
	if hurt.Battle.Health != a.Battle.Health-80 || hurt.Battle.EnemyInjury != 6 {
		t.Fatal("NPC full-force punch did not pay its own body price")
	}
}

func TestWuxiaEnergyTransferCommitRetryAndColdReplay(t *testing.T) {
	s, id := wuxiaConditionStory(t)
	defer func() { _ = s.Close() }()
	_, _, e := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "迎战", Narrative: "双方摆开架势。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "迎战", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "normal"}}}})
	if e != nil {
		t.Fatal(e)
	}
	w, e := s.AdventureWindow(id, "main")
	if e != nil {
		t.Fatal(e)
	}
	input := AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s07"}
	first, e := s.CommitAdventureAction(id, "main", "drain-commit", w.Head, input)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.CommitAdventureAction(id, "main", "drain-commit", w.Head, input)
	if e != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("retry charged the drain twice", e)
	}
	if first.Actor.Battle.EnemyEnergy != 88 || first.Actor.Energy != first.Actor.MaxEnergy || first.Actor.Injury < 1 {
		t.Fatal("actual commit lost bounded transfer or foreign-energy injury")
	}
	root := s.Root()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s = NewStore(root)
	restored, e := s.AdventureWindow(id, "main")
	if e != nil || !reflect.DeepEqual(restored.Actor, first.Actor) {
		t.Fatal("cold replay changed transfer pools", e)
	}
	snap, e := s.Snapshot(id, "main")
	if e != nil || snap.Clock == nil || snap.Clock.Seconds != 20 {
		t.Fatal("one transfer command advanced time twice", e)
	}
}
