package cultivation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEnemyActionOptionsCanonicalizeOwnAffordableAbilities(t *testing.T) {
	own := combatantAt(t, "enemy", SideEnemy, 6)
	other := combatantAt(t, "player", SidePlayer, 6)
	fire, _ := SkillByID("fire_bolt")
	own.Skills = []SkillDef{fire}
	own.Attributes.Mana = 0
	session := CombatSession{Combatants: []Combatant{own, other}}
	before, _ := json.Marshal(session)
	for _, option := range EnemyActionOptions(session, "enemy") {
		if option.Action.Kind == ActionTechnique && !IsOrdinaryAttack(option.Action.Skill) {
			t.Fatalf("unpayable skill offered: %+v", option)
		}
		if strings.Contains(option.ID, "heavy_thunder") {
			t.Fatal("registry-only ability offered")
		}
	}
	if _, ok := EnemyActionForChoice(session, "enemy", "technique:heavy_thunder:player:"); ok {
		t.Fatal("unlearned model technique accepted")
	}
	session.Combatants[0].Attributes.Mana = session.Combatants[0].Attributes.ManaMax
	action, ok := EnemyActionForChoice(session, "enemy", "technique:fire_bolt:player:")
	if !ok || action.Skill.ID != fire.ID || action.Skill.Power != fire.Power {
		t.Fatal("paid canonical learned technique missing")
	}
	session.Combatants[0].Attributes.Mana = 0
	after, _ := json.Marshal(session)
	if string(before) != string(after) {
		t.Fatal("enumeration mutated state")
	}
}

func TestEnemyActionOptionsDoNotDoubleCountOwnMethodReduction(t *testing.T) {
	session := twoWayBattle(t)
	enemy := session.CombatantByID("foe")
	skill := mustSkill("heavy_thunder")
	enemy.Skills = []SkillDef{skill}
	enemy.ManaReduction = .10
	price := NextExchangeCastPlanFor(*enemy, skill, 0).Check
	if !price.Allowed || price.ManaCost < 2 {
		t.Fatalf("invalid paid fixture: %+v", price)
	}
	enemy.Attributes.Mana = price.ManaCost - 1
	key := "technique:heavy_thunder:hero:"
	if _, ok := EnemyActionForChoice(session, "foe", key); ok {
		t.Fatal("own method discount was applied twice to make an unaffordable option legal")
	}
	enemy.Attributes.Mana = price.ManaCost
	selected, ok := EnemyActionForChoice(session, "foe", key)
	if !ok || selected.ManaReduction != 0 {
		t.Fatal("candidate injected a second method discount")
	}
	next, out := ResolveExchange(session, map[string]CombatAction{"foe": selected, "hero": {Kind: ActionRest}}, fixedDraws(.5))
	step := findStep(t, out, "foe")
	if !step.Cast.Allowed || step.Cast.ManaCost != price.ManaCost || next.CombatantByID("foe").Attributes.Mana != 0 {
		t.Fatalf("selected AI spell did not pay the same real price as the fallback: %+v", step)
	}
}
