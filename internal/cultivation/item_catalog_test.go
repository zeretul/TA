package cultivation

import (
	"errors"
	"testing"
)

// The tests here cover 《自制-装备与物品品级》V0.1 §4, §7 and §10. The published
// acceptance list is specific about the boundaries: 白装＋5%且不能变成＋35%,
// 中品稀有上限15%, 顶品独特上限35%, 全局60%仍有效, and 背包法器不全加攻.

func TestFirstVersionCatalogValidatesAndRegisters(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	// Every shipped entry must pass the same validation a generated one faces,
	// so an editing mistake in the catalogue is caught here rather than at the
	// first use.
	for _, def := range append(append([]ItemDef(nil), firstVersionCatalog...), spiritWorldItems...) {
		if _, ok := catalog.Definition(def.ID); !ok {
			t.Errorf("entry %s was not registered", def.ID)
		}
	}
	if len(catalog.Definitions()) != len(firstVersionCatalog)+len(spiritWorldItems) {
		t.Errorf("catalogue holds %d entries, want %d", len(catalog.Definitions()), len(firstVersionCatalog)+len(spiritWorldItems))
	}
}

func TestWhiteImplementReachesFivePercentAndNotThirtyFive(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	white, ok := catalog.Definition("implement.lower.common")
	if !ok {
		t.Fatal("the white implement must be registered")
	}
	// 白装＋5%: a 下品常见 implement is 5% and nothing else. The design's
	// acceptance line is that it cannot become 35% — the rarity budget is
	// headroom a definition spends, never a bonus it receives.
	if white.Implement.AttackBonus != 0.05 {
		t.Errorf("the white implement grants %v, want 0.05", white.Implement.AttackBonus)
	}
	ceiling, _ := MainImplementBonus(white.Grade, white.Rarity, white.RaritySpentOnMainStat)
	if ceiling != 0.05 {
		t.Errorf("the white implement's ceiling is %v, want 0.05", ceiling)
	}
	// A definition claiming the top-tier bonus on a white item must be refused
	// rather than capped afterwards.
	inflated := white
	inflated.ID = "implement.lower.common.inflated"
	inflated.Implement = &ImplementDef{AttackBonus: 0.35}
	if err := inflated.Validate(); !errors.Is(err, ErrItemOverBudget) {
		t.Errorf("a white item claiming 35%% returned %v, want ErrItemOverBudget", err)
	}
}

func TestMiddleRareReachesExactlyFifteenPercent(t *testing.T) {
	// 中品稀有上限15%: 10% from the grade plus the 5% rarity budget, spent on the
	// main attribute. This is the published ceiling for the pair.
	ceiling, ok := MainImplementBonus(GradeMiddle, RarityRare, rarityExtraBudget[RarityRare])
	if !ok {
		t.Fatal("a middle-grade rare item must have a ceiling")
	}
	// Compared as a rounded percentage: 0.10 + 0.05 is not exactly 0.15 in
	// binary floating point, and the published table is a percentage.
	if roundHalfUp(ceiling*100) != 15 {
		t.Errorf("middle rare ceiling = %v, want 15%%", ceiling)
	}
	// And it must not silently exceed it when the definition asks for more.
	spend := clampFloat(0.20, 0, rarityExtraBudget[RarityRare])
	clamped, _ := MainImplementBonus(GradeMiddle, RarityRare, spend)
	if roundHalfUp(clamped*100) != 15 {
		t.Errorf("an overspending middle rare item = %v, want the 15%% ceiling", clamped)
	}
}

func TestTopUniqueIsTheOnlyPairThatReachesThirtyFive(t *testing.T) {
	// 顶品独特上界才＋35%: 25% from the grade plus the 10% rarity budget.
	ceiling, _ := MainImplementBonus(GradeTop, RarityUnique, rarityExtraBudget[RarityUnique])
	if ceiling != 0.35 {
		t.Errorf("top unique ceiling = %v, want 0.35", ceiling)
	}
	catalog := NewItemCatalogWithFirstVersion()
	top, ok := catalog.Definition("implement.top.unique")
	if !ok {
		t.Fatal("the top unique implement must be registered")
	}
	if top.Implement.AttackBonus != 0.35 {
		t.Errorf("the top unique implement grants %v, want 0.35", top.Implement.AttackBonus)
	}
}

func TestARarityBudgetSpentOnAnEffectIsNotAlsoCountedAsAttack(t *testing.T) {
	// §4.1: a budget spent on an effect cannot also be added as attack. The
	// catalogue carries one such implement so the rule is exercised by a real
	// entry, not only by a unit case.
	catalog := NewItemCatalogWithFirstVersion()
	effect, ok := catalog.Definition("implement.upper.rare.effect")
	if !ok {
		t.Fatal("the effect implement must be registered")
	}
	if effect.Implement.SpecialEffect == "" {
		t.Fatal("this entry is meant to spend its budget on an effect")
	}
	if effect.Implement.AttackBonus > mainStatOnlyBonus(GradeUpper) {
		t.Errorf("an effect implement carries %v attack, above its grade's %v",
			effect.Implement.AttackBonus, mainStatOnlyBonus(GradeUpper))
	}

	// A definition that both names an effect and claims the full budget as
	// attack must be refused.
	doubleDipped := effect
	doubleDipped.ID = "implement.upper.rare.doubled"
	doubleDipped.RaritySpentOnMainStat = rarityExtraBudget[RarityRare]
	doubleDipped.Implement = &ImplementDef{
		AttackBonus:   0.23, // 18% grade plus the 5% rarity budget
		SpecialEffect: "item.effect.implement.meridian_throughput",
	}
	if err := doubleDipped.Validate(); !errors.Is(err, ErrItemBudgetCountedTwice) {
		t.Errorf("a double-counted rarity budget returned %v, want ErrItemBudgetCountedTwice", err)
	}
}

func TestTheGlobalSixtyPercentCapStillBindsAStackedBackpack(t *testing.T) {
	// 全局60%仍有效: the per-item ceilings are not the whole story. Even if a
	// character equips an implement and wears armour and takes a boosting pill,
	// the aggregated attack may not pass the published 60%.
	base := BaseAttributes(Qi(6))
	modifiers := []AttributeModifier{
		EquipmentModifier("implement:top.unique", AttrAttack, 0.35),
		EquipmentModifier("armor:middle.rare", AttrAttack, 0.15),
		{ID: "pill:attack", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.15, Group: "attack_pill", Temporary: true},
		{ID: "formation:attack", Kind: SourceFormation, Attribute: AttrAttack, Value: 0.15},
	}
	breakdown := AggregateAttribute(AttrAttack, base.Attack, modifiers)
	if breakdown.PositiveSum > 0.60 {
		t.Errorf("aggregated positive attack = %v, above the 0.60 cap", breakdown.PositiveSum)
	}
	// 40 × 1.60 = 64, not 40 × 1.80.
	if breakdown.Value != 64 {
		t.Errorf("stacked attack = %d, want 64 (40 × 1.60)", breakdown.Value)
	}
}

func TestABackpackOfImplementsDoesNotAllAddAttack(t *testing.T) {
	// §10 asks for 背包法器不全加攻 to be shown. Only the one active main
	// implement contributes: the others are carried, not wielded, so the caller
	// passes one implement modifier rather than one per item in the bag.
	base := BaseAttributes(Qi(6))
	wielded := AggregateAttribute(AttrAttack, base.Attack,
		[]AttributeModifier{EquipmentModifier("implement:wielded", AttrAttack, 0.35)})
	if wielded.Value != 54 {
		t.Errorf("one wielded implement gives attack %d, want 54 (40 × 1.35)", wielded.Value)
	}
	// Carrying four more of the same definition adds nothing, because a
	// definition's existence is not stock and a bag is not a bonus.
	carried := AggregateAttribute(AttrAttack, base.Attack,
		[]AttributeModifier{EquipmentModifier("implement:wielded", AttrAttack, 0.35)})
	if carried.Value != wielded.Value {
		t.Errorf("carrying more implements changed attack: %d vs %d", carried.Value, wielded.Value)
	}
}

func TestAnEntryAndItsPayloadMustAgreeAboutTheGrade(t *testing.T) {
	// The grade is stored twice — on the entry and on the pill payload — because
	// PillDef is usable on its own. A disagreement would price the item against
	// one grade while the catalogue displayed the other, so it is refused.
	catalog := NewItemCatalog()
	def := ItemDef{
		ID: "pill.confused", NameKey: "item.pill.confused.name",
		Kind: KindPill, Grade: GradeTop, Rarity: RarityCommon,
		Pill: &PillDef{Purpose: PillRestoreHealth, Grade: GradeLower, RegisteredStage: Qi(6)},
	}
	if err := catalog.Register(def); !errors.Is(err, ErrItemGradeMismatch) {
		t.Errorf("a disagreeing grade returned %v, want ErrItemGradeMismatch", err)
	}
	if _, registered := catalog.Definition("pill.confused"); registered {
		t.Error("a refused entry must not be registered")
	}
}

func TestRegistrationStampsThePayloadGradeFromTheEntry(t *testing.T) {
	// An author who writes the grade once, on the entry, gets a definition whose
	// payload carries it too: there is one authoritative grade rather than a
	// second field a caller could find empty.
	catalog := NewItemCatalog()
	def := ItemDef{
		ID: "pill.stamped", NameKey: "item.pill.stamped.name",
		Kind: KindPill, Grade: GradeUpper, Rarity: RarityCommon,
		Pill: &PillDef{Purpose: PillRestoreHealth, RegisteredStage: Qi(6)},
	}
	if err := catalog.Register(def); err != nil {
		t.Fatalf("registering an entry with an ungraded payload failed: %v", err)
	}
	stored, ok := catalog.Definition("pill.stamped")
	if !ok {
		t.Fatal("the entry must be registered")
	}
	if stored.Pill.Grade != GradeUpper {
		t.Errorf("the stored payload grade = %q, want %q", stored.Pill.Grade, GradeUpper)
	}
	// 上品 restores 20% of the registered stage's 200 health.
	if amount := stored.Pill.RestoreAmount(BaseAttributes(Qi(6))); amount != 40 {
		t.Errorf("the stamped pill restores %d, want 40 (20%% of 200)", amount)
	}
}

func TestADefinitionIsNotAnInstanceAndAnInstanceIsNotStock(t *testing.T) {
	// §7: 同一定义可有多份真实实例，不能把定义存在当作库存存在.
	catalog := NewItemCatalogWithFirstVersion()
	if _, ok := catalog.Definition("pill.restore.health.lower"); !ok {
		t.Fatal("the definition must be registered")
	}

	// The definition being registered says nothing about what anyone holds.
	// An instance has to be made explicitly.
	first, err := catalog.NewItemInstance("dose-1", "pill.restore.health.lower", 2)
	if err != nil {
		t.Fatalf("making an instance failed: %v", err)
	}
	second, err := catalog.NewItemInstance("dose-2", "pill.restore.health.lower", 1)
	if err != nil {
		t.Fatalf("making a second instance failed: %v", err)
	}
	if first.Quantity != 2 || second.Quantity != 1 {
		t.Errorf("two instances of one definition must keep their own counts: %+v %+v", first, second)
	}

	// Consuming one instance leaves the other and the definition untouched.
	consumed, err := first.Consume(catalog)
	if err != nil {
		t.Fatalf("consuming a dose failed: %v", err)
	}
	if consumed.Quantity != 1 {
		t.Errorf("after one dose the instance holds %d, want 1", consumed.Quantity)
	}
	if second.Quantity != 1 {
		t.Errorf("consuming one instance changed another: %d", second.Quantity)
	}
	if _, ok := catalog.Definition("pill.restore.health.lower"); !ok {
		t.Error("consuming a dose must not unregister the definition")
	}
}

func TestAnExhaustedInstanceRefusesAndChangesNothing(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	instance, err := catalog.NewItemInstance("dose", "pill.restore.health.lower", 1)
	if err != nil {
		t.Fatalf("making an instance failed: %v", err)
	}
	spent, err := instance.Consume(catalog)
	if err != nil {
		t.Fatalf("the first dose must be spendable: %v", err)
	}
	if spent.Quantity != 0 {
		t.Fatalf("after the last dose the instance holds %d, want 0", spent.Quantity)
	}
	// §8: an illegal use must not deduct. A repeated command therefore cannot
	// eat a second dose, and the refusal is reported rather than silently
	// succeeding.
	again, err := spent.Consume(catalog)
	if !errors.Is(err, ErrItemExhausted) {
		t.Errorf("consuming an empty instance returned %v, want ErrItemExhausted", err)
	}
	if again.Quantity != 0 {
		t.Errorf("a refused consumption changed the count to %d", again.Quantity)
	}
}

func TestEquipmentIsOneObjectAndPillsAreStacks(t *testing.T) {
	catalog := NewItemCatalogWithFirstVersion()
	// An implement is a real object, so asking for five of them yields one.
	implement, err := catalog.NewItemInstance("sword", "implement.middle.rare", 5)
	if err != nil {
		t.Fatalf("making an implement instance failed: %v", err)
	}
	if implement.Quantity != 1 {
		t.Errorf("an implement instance holds %d, want 1", implement.Quantity)
	}
	// And it is not consumable by use.
	if _, err := implement.Consume(catalog); !errors.Is(err, ErrItemNotConsumable) {
		t.Errorf("using up an implement returned %v, want ErrItemNotConsumable", err)
	}
}

func TestAnInstanceCannotReferenceAnUnregisteredDefinition(t *testing.T) {
	catalog := NewItemCatalog()
	if _, err := catalog.NewItemInstance("x", "never.registered", 1); !errors.Is(err, ErrItemDefinitionNotFound) {
		t.Errorf("an instance of an unknown definition returned %v, want ErrItemDefinitionNotFound", err)
	}
}

func TestARestoringPillIsFixedToTheStageItWasRegisteredAt(t *testing.T) {
	// §10: 低阶药不自动变高阶药. The catalogue's restoring pill is registered
	// against qi-refining layer 6, so it gives the same absolute recovery to a
	// stronger character rather than scaling up with them.
	catalog := NewItemCatalogWithFirstVersion()
	def, ok := catalog.Definition("pill.restore.health.lower")
	if !ok {
		t.Fatal("the restoring pill must be registered")
	}
	weak := BaseAttributes(Qi(6))
	strong := BaseAttributes(Qi(13))
	weakDose := def.Pill.RestoreAmount(weak)
	strongDose := def.Pill.RestoreAmount(strong)
	if weakDose != strongDose {
		t.Errorf("a low-grade pill scaled with the user: %d for qi 6, %d for qi 13", weakDose, strongDose)
	}
	// 下品 is 10% of the registered stage's 200 health.
	if weakDose != 20 {
		t.Errorf("the dose restores %d, want 20 (10%% of 200)", weakDose)
	}
}

func TestTheBoostPillAndFormationShareThePublishedAttackBudget(t *testing.T) {
	// Both a boosting pill and an attack formation grant attack, and both are
	// capped by the same global 60% rather than by a private ceiling.
	catalog := NewItemCatalogWithFirstVersion()
	pill, ok := catalog.Definition("pill.boost.attack.middle")
	if !ok {
		t.Fatal("the boosting pill must be registered")
	}
	bonus, isBoost := pill.Pill.AttackBonus()
	if !isBoost || bonus != 0.08 {
		t.Errorf("the middle-grade boosting pill grants %v, want 0.08", bonus)
	}
	formation, ok := catalog.Definition("formation.attack.middle")
	if !ok {
		t.Fatal("the attack formation must be registered")
	}
	if formation.Formation.AttackBonus != 0.08 {
		t.Errorf("the middle-grade attack formation grants %v, want 0.08", formation.Formation.AttackBonus)
	}
}

func TestTheSameTypeTemporaryBoostDoesNotStack(t *testing.T) {
	// §4.2: 同类取强. A second boosting pill replaces the first rather than
	// adding to it, which is enforced by the stacking group rather than by the
	// catalogue.
	base := BaseAttributes(Qi(6))
	one := []AttributeModifier{
		{ID: "pill:a", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.08, Group: "attack_pill", Temporary: true},
	}
	two := []AttributeModifier{
		{ID: "pill:a", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.08, Group: "attack_pill", Temporary: true},
		{ID: "pill:b", Kind: SourceMedicine, Attribute: AttrAttack, Value: 0.15, Group: "attack_pill", Temporary: true},
	}
	single := AggregateAttribute(AttrAttack, base.Attack, one)
	doubled := AggregateAttribute(AttrAttack, base.Attack, two)
	if doubled.PositiveSum != 0.15 {
		t.Errorf("a second same-type pill must leave only the stronger: sum %v, want 0.15", doubled.PositiveSum)
	}
	// 同类取强: the stronger 15% pill survives, not the weaker 8% one and not
	// their sum. 40 × 1.15 = 46.
	if doubled.Value != 46 {
		t.Errorf("the surviving boost gives attack %d, want 46 (40 × 1.15)", doubled.Value)
	}
	if doubled.Value == single.Value {
		t.Error("taking a stronger pill must be visible; the weaker one is not the one that survives")
	}
}
