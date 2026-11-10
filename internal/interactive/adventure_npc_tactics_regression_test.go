package interactive

import (
	"fmt"
	"testing"
)

// Explicitly staged player build; the opponent keeps its actual period body,
// learned skills, equipped copies and finite owned consumables.
func npcTacticsBalanceFixture(t *testing.T, pack, source string, rank int, skill string) (AdventureRules, AdventureActor) {
	t.Helper()
	r := authoredAdventureRules(t, pack)
	a := NewAdventureActor(r)
	a.Rank = rank
	a.MaxHealth = 100 + 20*(rank-1)
	a.Health = a.MaxHealth
	a.MaxEnergy = 100 + 10*(rank-1)
	a.Energy = a.MaxEnergy
	if skill != "" {
		a.Skills[skill] = AdventureSkillMasteryCap(r)
	}
	items := map[string]AdventureItem{}
	for _, i := range r.Items {
		items[i.ID] = i
	}
	prefix := "xinghai"
	if r.Family == "wuxia" {
		prefix = "lanchuan"
	}
	for index, stage := range r.Progression.Stages {
		if index == 0 || stage.MinRank > rank {
			continue
		}
		for _, kind := range []string{"weapon", "armor"} {
			id := fmt.Sprintf("%s-advanced-%s-%d", prefix, kind, index)
			a.Inventory[id] = 1
			a.Equipment["person:"+kind] = id
			for material := range items[id].Materials {
				a.Inventory[material] = 100
			}
		}
	}
	for _, op := range r.Opponents {
		if op.CharacterSourceKey != source && op.ID != source {
			continue
		}
		started, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_start", OpponentID: op.ID}, "npc-tactics")
		if err != nil {
			t.Fatal(err)
		}
		if op.CharacterSourceKey == "" {
			return r, started
		}
		for _, n := range r.NPCs {
			if n.CharacterSourceKey == source {
				bindAdventureEncounter(started.Battle, "period-person", n.Actor, r)
				return r, started
			}
		}
	}
	t.Fatal("named encounter fixture missing")
	return r, a
}

func TestAdventurePeriodExpertCountersInsteadOfWaitingToDie(t *testing.T) {
	r, a := npcTacticsBalanceFixture(t, "lanchuan-qunxia", "lanchuan-p36", 10, "lanchuan-s16")
	// Previously the rank27 expert chose a partial guard every round and lost
	// to this rank10 build in six rounds without dealing any damage.
	for round := 0; round < 3 && a.Battle.Status == "active"; round++ {
		next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "skill", SkillID: "lanchuan-s16"}, fmt.Sprintf("expert-counter-%d", round))
		if err != nil {
			t.Fatal(err)
		}
		if round == 0 && (next.Health >= a.Health || next.Battle.EnemyLastSkillID != "lanchuan-wudang-close-strike") {
			t.Fatalf("expert passed a useful counter: %+v", next.Battle)
		}
		a = next
	}
	if a.Battle.Status != "defeat" {
		t.Fatalf("expert remained passive: %s", a.Battle.Status)
	}
}

func TestAdventurePeriodRecoveryExpertDoesNotStallForSmallWounds(t *testing.T) {
	r, a := npcTacticsBalanceFixture(t, "lanchuan-qunxia", "lanchuan-p54", 39, "")
	// Previously ordinary attacks caused sixty recovery-only responses while
	// the player stayed at full health. Recovery remains legal at real peril.
	responded := false
	for round := 0; round < 30 && a.Battle.Status == "active"; round++ {
		next, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "battle_round", Choice: "attack"}, fmt.Sprintf("expert-recovery-%d", round))
		if err != nil {
			t.Fatal(err)
		}
		responded = responded || next.Health < a.Health
		a = next
	}
	if !responded || a.Battle.Status == "active" {
		t.Fatalf("recovery never made combat progress: health=%d status=%s", a.Health, a.Battle.Status)
	}
}
