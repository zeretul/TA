package cultivation

import (
	"reflect"
	"testing"
)

func TestEnemyShieldRespondsToVisiblePreparationAndProtectsSelf(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Skills = []SkillDef{mustSkill("strike"), mustSkill("shield")}
	session.Preparations = map[string]Preparation{"hero": {Skill: mustSkill("meridian_secret_blade"), TargetID: "foe"}}
	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Skill.ID != "shield" || action.TargetID != "foe" {
		t.Fatalf("ward response: %+v", action)
	}
	next, outcome := ResolveExchange(session, map[string]CombatAction{"foe": action, "hero": {Kind: ActionCancelPreparation}}, fixedDraws(0.5))
	if findStep(t, outcome, "foe").ActionRejected || next.CombatantByID("foe").Shield.Amount == 0 || next.CombatantByID("hero").Shield.Amount != 0 {
		t.Fatalf("shield did not protect its caster: %+v", next)
	}
	delete(session.Preparations, "hero")
	if quiet := DecideEnemyAction(session, "foe", mustSkill("strike")); quiet.Skill.ID != "strike" {
		t.Fatalf("idle ward wasted a turn: %+v", quiet)
	}
}

func TestEnemyChoosesAndExecutesRealMultiTargetTechnique(t *testing.T) {
	session := twoWayBattle(t)
	session.Combatants = append(session.Combatants, combatantAt(t, "ally-b", SidePlayer, 6), combatantAt(t, "ally-a", SidePlayer, 6))
	foe := session.CombatantByID("foe")
	foe.Skills = []SkillDef{mustSkill("heavy_thunder"), mustSkill("flame_wave")}
	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Skill.ID != "flame_wave" || len(action.ExtraTargetIDs) != 2 {
		t.Fatalf("group tactic: %+v", action)
	}
	next, outcome := ResolveExchange(session, map[string]CombatAction{"foe": action, "hero": {Kind: ActionRest}, "ally-a": {Kind: ActionRest}, "ally-b": {Kind: ActionRest}}, fixedDraws(0))
	step := findStep(t, outcome, "foe")
	if step.ActionRejected {
		t.Fatalf("selected action was rejected: %+v", step)
	}
	for _, id := range []string{"hero", "ally-a", "ally-b"} {
		if next.CombatantByID(id).Attributes.Health >= session.CombatantByID(id).Attributes.Health {
			t.Fatalf("target %s was not reached", id)
		}
	}
	session.Combatants = session.Combatants[:2]
	if single := DecideEnemyAction(session, "foe", mustSkill("strike")); single.Skill.ID != "heavy_thunder" || len(single.ExtraTargetIDs) != 0 {
		t.Fatalf("single-target tactic: %+v", single)
	}
}

func TestEnemyVariantsFollowVisibleLastingEffectsAndOwnedResources(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Skills = []SkillDef{mustSkill("fire_bolt")}
	burn := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if burn.Variant != VariantBurn {
		t.Fatalf("healthy exposed target did not invite burn: %+v", burn)
	}
	session.CombatantByID("hero").Burn = NewBurn(30, "earlier")
	if direct := DecideEnemyAction(session, "foe", mustSkill("strike")); direct.Variant != VariantNone {
		t.Fatalf("unneeded reapplication: %+v", direct)
	}
	foe.Attributes.Mana = 0
	if fallback := DecideEnemyAction(session, "foe", mustSkill("strike")); fallback.Skill.ID != "strike" {
		t.Fatalf("unpaid tactic: %+v", fallback)
	}
}

func TestEnemyDecisionIgnoresHiddenTargetSkillsAndCatalogOrder(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Skills = []SkillDef{mustSkill("fire_bolt"), mustSkill("condensed_blade")}
	first := DecideEnemyAction(session, "foe", mustSkill("strike"))
	foe.Skills[0], foe.Skills[1] = foe.Skills[1], foe.Skills[0]
	hero := session.CombatantByID("hero")
	hero.Skills = []SkillDef{mustSkill("meridian_secret_blade")}
	hero.Attributes.Mana = 0
	if next := DecideEnemyAction(session, "foe", mustSkill("strike")); !reflect.DeepEqual(first, next) {
		t.Fatalf("hidden resources or ordering changed choice: %+v -> %+v", first, next)
	}
}

func TestEnemyDecisionPricesActualEquippedWeaponRatherThanStaleOperatingField(t *testing.T) {
	session := twoWayBattle(t)
	foe := session.CombatantByID("foe")
	foe.Skills = []SkillDef{mustSkill("fire_bolt")}
	foe.Attributes.Mana = 8
	foe.Equipment = []EquipmentCopy{{Slot: EquipmentSlotWeapon, OperatingManaCost: 5}}
	if action := DecideEnemyAction(session, "foe", mustSkill("strike")); action.Skill.ID == "fire_bolt" {
		t.Fatalf("unaffordable actual equipment was ignored: %+v", action)
	}
}

func TestEnemyDoesNotSpendAreaTechniqueOnOneTargetWhenOrdinaryAttackIsStronger(t *testing.T) {
	session := twoWayBattle(t)
	session.CombatantByID("foe").Skills = []SkillDef{mustSkill("flame_wave")}
	action := DecideEnemyAction(session, "foe", mustSkill("strike"))
	if action.Skill.ID != "strike" {
		t.Fatalf("paid weaker area attack against one target: %+v", action)
	}
}

func TestUnderRealmTechniqueIsRejectedBeforePaymentAndPreparedCompletion(t *testing.T) {
	session := twoWayBattle(t)
	skill := mustSkill("fire_bolt")
	skill.ApplicableStage = "foundation_early"
	before := session.CombatantByID("hero").Attributes
	next, out := ResolveExchange(session, map[string]CombatAction{"hero": {Kind: ActionTechnique, Skill: skill, TargetID: "foe"}, "foe": {Kind: ActionRest}}, fixedDraws(0))
	step := findStep(t, out, "hero")
	if !step.ActionRejected || step.ReasonKey != ReasonSkillStageTooLow || next.CombatantByID("hero").Attributes.Mana != before.Mana {
		t.Fatalf("under-realm cast was paid or misreported: %+v", step)
	}
	completed := completePreparation(&session, session.CombatantByID("hero"), Preparation{Skill: skill, TargetID: "foe"}, fixedDraws(0))
	if completed.Cast.Allowed || completed.ReasonKey != ReasonSkillStageTooLow || session.CombatantByID("foe").Attributes.Health != session.CombatantByID("foe").Attributes.HealthMax {
		t.Fatalf("under-realm prepared cast completed: %+v", completed)
	}
}

func TestPoweredGarmentPaysOnlyWhenUsedAndLosesBonusWithoutMana(t *testing.T) {
	s := twoWayBattle(t)
	hero := s.CombatantByID("hero")
	hero.Equipment = []EquipmentCopy{{InstanceID: "powered-robe", DefID: "test-robe", Slot: EquipmentSlotGarment, OperatingManaCost: 5, Modifiers: []AttributeModifier{EquipmentModifier("test-robe", AttrDefense, .25)}}}
	hero.recomputeAttributes()
	baseDefense := BaseAttributes(hero.Cultivation).Defense
	if hero.Attributes.Defense != baseDefense {
		t.Fatal("unpowered robe granted a free bonus")
	}
	mana := hero.Attributes.Mana
	if paid := hero.driveTreasure(AttrDefense); paid != 5 || hero.Attributes.Mana != mana-5 || hero.Attributes.Defense <= baseDefense {
		t.Fatal("robe did not pay for its real defense")
	}
	if paid := hero.driveTreasure(AttrDefense); paid != 0 || hero.Attributes.Mana != mana-5 {
		t.Fatal("same exchange charged armor twice")
	}
	hero.Equipment[0].Driven = false
	hero.Attributes.Mana = 4
	hero.recomputeAttributes()
	if paid := hero.driveTreasure(AttrDefense); paid != 0 || hero.Attributes.Defense != baseDefense || hero.Attributes.Mana != 4 {
		t.Fatal("unaffordable armor retained its bonus or consumed mana")
	}
	hero.Equipment[0].OperatingManaCost = 0
	hero.recomputeAttributes()
	if hero.Attributes.Defense <= baseDefense {
		t.Fatal("passive armor lost its free bonus")
	}
}

func TestCirculatingStrainBenefitChangesRealCastAndKeepsItsCeiling(t *testing.T) {
	method, _ := TechniqueByID("method.core_still_path")
	state := CharacterState{Techniques: []TechniqueDef{method, method}}
	if state.StrainReduction() != .10 {
		t.Fatal("duplicated methods stacked strain reduction")
	}
	stage := Cultivation{Realm: RealmCore, Stage: StageEarly}
	attrs := BaseAttributes(stage)
	skill := mustSkill("core_pierce")
	burden := BurdenState{Current: 62}
	if CanPerform(skill, stage, attrs, burden, 0, false).Allowed {
		t.Fatal("unmodified heavy cast unexpectedly fits")
	}
	check := CanPerformWithStrain(skill, stage, attrs, burden, 0, state.StrainReduction(), false)
	if !check.Allowed || check.BurdenCost != 38 {
		t.Fatalf("real reduced burden did not fit: %+v", check)
	}
	if got := CanPerformWithStrain(skill, stage, attrs, BurdenState{}, 0, 99, false).BurdenCost; got != 34 {
		t.Fatalf("strain cap bypassed: %d", got)
	}
	s := CombatSession{ID: "strain", Round: 1, Combatants: []Combatant{{ID: "hero", Side: SidePlayer, Cultivation: stage, Attributes: attrs, Burden: burden, StrainReduction: state.StrainReduction(), Skills: []SkillDef{skill}}, {ID: "foe", Side: SideEnemy, Cultivation: stage, Attributes: attrs}}}
	action := DecideEnemyAction(s, "hero", mustSkill("strike"))
	if action.Skill.ID != skill.ID {
		t.Fatalf("tactic ignored own affordable reduced cast: %+v", action)
	}
	next, out := ResolveExchange(s, map[string]CombatAction{"hero": action, "foe": {Kind: ActionRest}}, fixedDraws(0))
	step := findStep(t, out, "hero")
	if step.ActionRejected || step.Cast.BurdenCost != 38 || next.CombatantByID("hero").Attributes.Mana >= attrs.Mana {
		t.Fatalf("strain benefit never reached settlement: %+v", step)
	}
}

func TestEnemyRegulatesRatherThanChoosingKnownUnpayableWeapon(t *testing.T) {
	s := twoWayBattle(t)
	foe := s.CombatantByID("foe")
	foe.Attributes.Mana = 0
	foe.Equipment = []EquipmentCopy{{Slot: EquipmentSlotWeapon, OperatingManaCost: 5}}
	action := DecideEnemyAction(s, "foe", mustSkill("strike"))
	if action.Kind != ActionRest {
		t.Fatalf("knowingly chose unpaid action: %+v", action)
	}
	next, out := ResolveExchange(s, map[string]CombatAction{"foe": action, "hero": {Kind: ActionRest}}, fixedDraws(0))
	if findStep(t, out, "foe").ActionRejected || next.CombatantByID("foe").Attributes.Mana <= 0 {
		t.Fatal("regulation did not resolve normally")
	}
}

func TestRealmPresetSkillsResolveDamageAreaAndWard(t *testing.T) {
	for _, realm := range RealmOrder {
		for _, suffix := range []string{"_pierce", "_wave", "_ward"} {
			skill, ok := SkillByID(string(realm) + suffix)
			if !ok {
				continue
			}
			t.Run(skill.ID, func(t *testing.T) {
				stage := Cultivation{Realm: realm, Stage: StageEarly}
				attrs := BaseAttributes(stage)
				hero := Combatant{ID: "hero", Side: SidePlayer, Cultivation: stage, Attributes: attrs, Skills: []SkillDef{skill}}
				s := CombatSession{ID: "realm-kit", Round: 1, Combatants: []Combatant{hero, {ID: "foe", Side: SideEnemy, Cultivation: stage, Attributes: attrs}, {ID: "foe2", Side: SideEnemy, Cultivation: stage, Attributes: attrs}, {ID: "foe3", Side: SideEnemy, Cultivation: stage, Attributes: attrs}}}
				action := CombatAction{Kind: ActionTechnique, Skill: skill, TargetID: "foe"}
				if suffix == "_wave" {
					action.ExtraTargetIDs = []string{"foe2", "foe3"}
				}
				if suffix == "_ward" {
					action.TargetID = "hero"
				}
				next, out := ResolveExchange(s, map[string]CombatAction{"hero": action, "foe": {Kind: ActionRest}, "foe2": {Kind: ActionRest}, "foe3": {Kind: ActionRest}}, fixedDraws(0))
				step := findStep(t, out, "hero")
				if step.ActionRejected || next.CombatantByID("hero").Attributes.Mana >= attrs.Mana {
					t.Fatalf("preset never paid or cast: %+v", step)
				}
				if suffix == "_ward" {
					if next.CombatantByID("hero").Shield.Amount <= 0 {
						t.Fatal("realm ward did not activate")
					}
					_, follow := ResolveExchange(next, map[string]CombatAction{"hero": {Kind: ActionRest}, "foe": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"}, "foe2": {Kind: ActionRest}, "foe3": {Kind: ActionRest}}, fixedDraws(0))
					response := findStep(t, follow, "foe")
					if response.Resolution.ShieldAbsorbed <= 0 || response.Resolution.HealthLost != 0 {
						t.Fatalf("realm ward failed to absorb the real next attack: %+v", response)
					}
					return
				}
				if next.CombatantByID("foe").Attributes.Health >= attrs.Health {
					t.Fatal("realm strike caused no damage")
				}
				for _, id := range []string{"foe2", "foe3"} {
					hit := next.CombatantByID(id).Attributes.Health < attrs.Health
					if hit != (suffix == "_wave") {
						t.Fatalf("wrong area reach %s", id)
					}
				}
			})
		}
	}
}
