package cultivation

import "testing"

func TestRoutineSwordQiDoesNotStrainWhileHeavyCastStillDoes(t *testing.T) {
	s := twoWayBattle(t)
	hero := s.CombatantByID("hero")
	light, heavy := mustSkill("condensed_blade"), mustSkill("heavy_thunder")
	hero.Skills = []SkillDef{light, heavy}
	hero.Burden = BurdenState{Current: 80}
	plan := NextExchangeCastPlanFor(*hero, light, 0)
	if !plan.Check.Allowed || plan.Check.BurdenCost != 0 || plan.Check.ManaCost != light.ManaCost(hero.Cultivation) {
		t.Fatalf("routine sword qi should cost only its usual mana: %+v", plan.Check)
	}
	step := resolveTechnique(&s, hero, CombatAction{Kind: ActionTechnique, Skill: light, TargetID: "foe"}, fixedDraws(0))
	if step.ActionRejected || step.Cast != plan.Check || hero.Burden != (BurdenState{Current: 80}) {
		t.Fatalf("real cast must match its preview without increasing the existing strain: %+v, %+v", step, hero.Burden)
	}
	hero.Burden = BurdenState{Current: 60, Residual: 60}
	if check := NextExchangeCastPlanFor(*hero, light, 0).Check; check.Allowed || check.ReasonKey != ReasonMeridianUnrecovered || check.BurdenCost != light.Burden {
		t.Fatalf("forcing sword qi through an active meridian injury should still strain it: %+v", check)
	}
	if check := NextExchangeCastPlanFor(*hero, heavy, 0).Check; check.Allowed || check.ReasonKey != ReasonMeridianUnrecovered || check.BurdenCost != heavy.Burden {
		t.Fatalf("heavy cast should still be limited by residual strain: %+v", check)
	}
}

func TestPoweredStabilizationSharesRealCostAndNeverMakesAnUnaffordableCastFree(t *testing.T) {
	s := twoWayBattle(t)
	hero := s.CombatantByID("hero")
	heavy := mustSkill("heavy_thunder")
	hero.Skills = []SkillDef{heavy}
	hero.Burden.Current = 64
	hero.Equipment = []EquipmentCopy{{Slot: EquipmentSlotWeapon, OperatingManaCost: 3}, {Slot: EquipmentSlotGarment, OperatingManaCost: 5, StrainReduction: .15, Modifiers: []AttributeModifier{{Kind: SourceArmor, Attribute: AttrDefense, Value: .25}}}}
	hero.Attributes.Mana = heavy.ManaCost(hero.Cultivation) + 8
	hero.Modifiers = []AttributeModifier{{Kind: SourceTechniqueSpecialisation, Attribute: AttrAgility, Value: .25}} // The paid cast occurs before the return strike.
	hero.recomputeAttributes()
	plan := CastPlanFor(*hero, heavy, 0)
	if !plan.Check.Allowed || plan.Check.BurdenCost != 36 || plan.Check.ManaCost != heavy.ManaCost(hero.Cultivation)+8 || !plan.DriveArmor {
		t.Fatalf("unified cast plan omitted a real benefit or payment: %+v", plan)
	}
	if !enemyCanPerform(*hero, heavy) {
		t.Fatal("NPC decision disagreed with its actual armor plan")
	}
	next, out := ResolveExchange(s, map[string]CombatAction{"hero": {Kind: ActionTechnique, Skill: heavy, TargetID: "foe"}, "foe": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"}}, fixedDraws(.5))
	step := findStep(t, out, "hero")
	if step.ActionRejected || !step.Cast.Allowed || step.Cast.BurdenCost != 36 || step.OperatingManaCost != 8 || next.CombatantByID("hero").Attributes.Mana != 0 {
		t.Fatalf("armor cast price changed during resolution: %+v", step)
	}
	robe, _ := next.CombatantByID("hero").EquipmentInSlot(EquipmentSlotGarment)
	if !robe.Driven || next.CombatantByID("hero").Attributes.Defense <= BaseAttributes(hero.Cultivation).Defense {
		t.Fatal("paid stabilization did not also cover the same-exchange defense")
	}
	hero.Burden.Current = 0
	hero.Attributes.Mana = heavy.ManaCost(hero.Cultivation) + 3
	without := CastPlanFor(*hero, heavy, 0)
	if !without.Check.Allowed || without.DriveArmor || without.Check.BurdenCost != heavy.Burden {
		t.Fatalf("optional armor blocked or freely strengthened an affordable base cast: %+v", without)
	}
	hero.Burden.Current = 64
	if CastPlanFor(*hero, heavy, 0).Check.Allowed {
		t.Fatal("unpaid stabilization made an impossible cast free")
	}
	hero.Attributes.Mana = hero.Attributes.ManaMax
	hero.StrainReduction = .15
	capped := CastPlanFor(*hero, heavy, 0)
	if capped.Check.BurdenCost != 34 {
		t.Fatalf("method plus robe passed the shared 20%% cap: %+v", capped)
	}
	hero.StrainReduction = 0
	hero.Equipment[1].StrainReduction = .00001
	hero.Burden.Current = 0
	if tiny := CastPlanFor(*hero, heavy, 0); tiny.DriveArmor || tiny.ArmorManaCost != 0 {
		t.Fatal("rounded-away stabilization paid for no real benefit")
	}
	hero.Equipment[1].StrainReduction = .15
	hero.Equipment[1].Driven = true
	paid := CastPlanFor(*hero, heavy, 0)
	if paid.ArmorManaCost != 0 || paid.Check.ManaCost != heavy.ManaCost(hero.Cultivation)+3 {
		t.Fatal("armor already paid for defense was charged again")
	}
}

func TestRatedWeaponPierceReachesRealAttackAndSharesTheDefenseIgnoreCap(t *testing.T) {
	run := func(pierce int) (ExchangeStep, int) {
		s := twoWayBattle(t)
		hero := s.CombatantByID("hero")
		hero.Equipment = []EquipmentCopy{{Slot: EquipmentSlotWeapon, OperatingManaCost: 5, DefensePierce: pierce}}
		before := hero.Attributes.Mana
		step := resolveTechnique(&s, hero, CombatAction{Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe"}, fixedDraws(0))
		return step, before - hero.Attributes.Mana
	}
	plain, plainCost := run(0)
	piercing, pierceCost := run(1000000)
	if !piercing.Resolution.Hit || piercing.Resolution.Damage <= plain.Resolution.Damage || plainCost != 5 || pierceCost != 5 {
		t.Fatalf("paid weapon pierce was not resolved: plain=%+v piercing=%+v", plain, piercing)
	}
	attrs := BaseAttributes(Qi(6))
	skill := mustSkill("strike")
	skill.IgnoreDefense = .20
	base := ResolveAttack(attrs, attrs, RealmQi, RealmQi, skill, NeutralDamageContext(), Shield{}, CombatRolls{Hit: 0, Damage: .5})
	combined := ResolveAttack(attrs, attrs, RealmQi, RealmQi, skill, DamageContext{DefensePierce: 1000000}, Shield{}, CombatRolls{Hit: 0, Damage: .5})
	if combined.Damage != base.Damage {
		t.Fatal("weapon and skill ignored more than 20 percent of real defense")
	}
	hero := Combatant{Cultivation: Qi(6), Equipment: []EquipmentCopy{{Slot: EquipmentSlotWeapon, ApplicableStage: "foundation_early", DefensePierce: 10000}}}
	if hero.weaponDefensePierce() != 0 {
		t.Fatal("under-realm weapon granted its special effect")
	}
}

func TestInitialInputErrorsSurviveBeingDefeatedBeforeTheDeclaredAction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action CombatAction
		reason string
	}{
		{"missing dose", CombatAction{Kind: ActionUseItem, ItemInstanceID: "absent"}, "reason.item_not_carried"},
		{"missing spare", CombatAction{Kind: ActionSwapEquipment, SwapInstanceID: "absent"}, "reason.spare_not_carried"},
		{"not equipment", CombatAction{Kind: ActionSwapEquipment, SwapInstanceID: "not-gear"}, "reason.spare_not_equipment"},
		{"continue absent", CombatAction{Kind: ActionContinuePreparation}, "reason.no_preparation"},
		{"cancel absent", CombatAction{Kind: ActionCancelPreparation}, "reason.no_preparation"},
		{"unknown", CombatAction{Kind: "invalid"}, "reason.unknown_action"},
		{"variant", CombatAction{Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "foe", Variant: VariantBurn}, "reason.variant_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := twoWayBattle(t)
			hero := s.CombatantByID("hero")
			hero.Attributes.Health = 1
			hero.Spares = []EquipmentCopy{{InstanceID: "not-gear", Slot: "invalid"}}
			next, out := ResolveExchange(s, map[string]CombatAction{"hero": tc.action, "foe": {Kind: ActionTechnique, Skill: mustSkill("strike"), TargetID: "hero"}}, func(purpose, actor, _ string) float64 {
				if purpose == "initiative" && actor == "foe" {
					return 1
				}
				return 0
			})
			if next.CombatantByID("hero").Attributes.Health != 0 {
				t.Fatal("fixture did not kill the actor first")
			}
			step := findStep(t, out, "hero")
			if !step.ActionRejected || step.ReasonKey != tc.reason {
				t.Fatalf("initial error lost after death: %+v", step)
			}
		})
	}
}
