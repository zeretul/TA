package cultivation

import (
	"errors"
	"testing"
)

// The tests here cover §4.1's two properties that had no rules code behind them
// before this batch: an entry's 适用阶段, which 适用阶段不是点击颜色就能越过 forbids
// the story from skipping, and the real cost of driving an implement, which
// 数值规则 line 128 requires even for an ordinary attack.

func TestAnEntryStatesTheStageItNeeds(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	treasure, ok := catalog.Definition("implement.top.unique")
	if !ok {
		t.Fatal("the top-grade unique implement must be registered")
	}
	// The published floor is 筑基初期. A 炼气 character holding the sword has
	// recorded that they own it, not that they can operate it.
	if err := treasure.UsableBy(Qi(6)); !errors.Is(err, ErrItemStageTooLow) {
		t.Errorf("a 炼气六层 character using a 筑基初期 treasure returned %v, want ErrItemStageTooLow", err)
	}
	if err := treasure.UsableBy(stageAt(t, "foundation_early")); err != nil {
		t.Errorf("the same treasure at its own stage returned %v, want no error", err)
	}
	if err := treasure.UsableBy(stageAt(t, "foundation_middle")); err != nil {
		t.Errorf("a stage above the floor returned %v, want no error", err)
	}
}

// stageAt parses a published stage key, failing the test if the key itself is
// unreadable — which would make the assertion below vacuous.
func stageAt(t *testing.T, key string) Cultivation {
	t.Helper()
	stage, ok := ParseStageKey(key)
	if !ok {
		t.Fatalf("the published stage key %q does not parse", key)
	}
	return stage
}

func TestAnEntryWithNoStatedStageIsUsableByAnyone(t *testing.T) {
	// An empty floor is the honest reading for a definition that never declared
	// one; it is not the same fact as a declared floor that happens to be low.
	plain := ItemDef{ID: "implement.unstated", Kind: KindImplement, Implement: &ImplementDef{AttackBonus: 0.05}}
	if err := plain.UsableBy(Mortal()); err != nil {
		t.Errorf("an entry with no applicable stage returned %v, want no error", err)
	}
}

func TestAStageTheRulesCannotReadIsRefusedAtRegistration(t *testing.T) {
	// A requirement nobody can evaluate would either block the item forever or be
	// silently ignored, so it is refused where the definition is written rather
	// than at the first use.
	broken := ItemDef{
		ID: "implement.bad.stage", Kind: KindImplement, Grade: GradeLower, Rarity: RarityCommon,
		ApplicableStage: "qi_99", Implement: &ImplementDef{AttackBonus: 0.05},
	}
	if err := broken.Validate(); !errors.Is(err, ErrItemUnknownStage) {
		t.Errorf("an unreadable stage returned %v, want ErrItemUnknownStage", err)
	}
}

func TestDrivingAnImplementCostsItsPublishedFraction(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	effect, ok := catalog.Definition("implement.upper.rare.effect")
	if !ok {
		t.Fatal("the effect implement must be registered")
	}
	// 2% of the bare 炼气六层 mana maximum of 160 is 3.2, rounded up to 4.
	if got := effect.OperatingManaCost(Qi(6)); got != 4 {
		t.Errorf("operating cost = %d, want 4 (ceil 0.02 × 160)", got)
	}
	// The ordinary implement publishes no fraction. That is not a missing number
	// and not a zero that was declared: it is free to swing.
	white, _ := catalog.Definition("implement.lower.common")
	if got := white.OperatingManaCost(Qi(6)); got != 0 {
		t.Errorf("the white implement costs %d, want 0", got)
	}
	// A pill is not driven. Only worn equipment can declare this cost.
	pill, _ := catalog.Definition("pill.restore.health.lower")
	if got := pill.OperatingManaCost(Qi(6)); got != 0 {
		t.Errorf("a pill costs %d mana to drive, want 0", got)
	}
}

// TestAPlainAttackWithATreasureInHandIsNotFree is the requirement itself:
// 普通攻击的真实法器操控成本依装备定义，不因按普攻按钮免费施展. The plain attack is the
// built-in `strike`, which costs no mana of its own.
func TestAPlainAttackWithATreasureInHandIsNotFree(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.HasImplement = true
	hero.OperatingManaCost = 4

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
	}, fixedDraws(0.5))
	step := findStep(t, outcome, "hero")
	if !step.Cast.Allowed {
		t.Fatalf("the plain attack was refused: %+v", step.Cast)
	}
	if step.OperatingManaCost != 4 {
		t.Errorf("the step records %d operating mana, want 4", step.OperatingManaCost)
	}
	if got := next.CombatantByID("hero").Attributes.Mana; got != 156 {
		t.Errorf("mana after the exchange = %d, want 156 (160 − 4)", got)
	}
}

// TestAnUnaffordableImplementRefusesRatherThanDraining is §8's "a refused action
// spends nothing" applied to the drive cost: an implement that costs more than
// the caster can pay must refuse the action, not resolve it and leave them at
// zero.
func TestAnUnaffordableImplementRefusesRatherThanDraining(t *testing.T) {
	session := twoWayBattle(t)
	hero := session.CombatantByID("hero")
	hero.HasImplement = true
	hero.OperatingManaCost = 4
	hero.Attributes.Mana = 3

	next, outcome := ResolveExchange(session, map[string]CombatAction{
		"hero": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"},
	}, fixedDraws(0.5))
	step := findStep(t, outcome, "hero")
	if step.Cast.Allowed {
		t.Fatal("an implement the caster cannot pay for must refuse the action")
	}
	if step.ReasonKey != ReasonInsufficientMana {
		t.Errorf("reason = %q, want the mana reason", step.ReasonKey)
	}
	if got := next.CombatantByID("hero").Attributes.Mana; got != 3 {
		t.Errorf("mana = %d, want the untouched 3", got)
	}
	if next.CombatantByID("foe").Attributes.Health != 200 {
		t.Error("a refused action must not damage anyone")
	}
}
