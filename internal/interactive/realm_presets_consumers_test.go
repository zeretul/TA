package interactive

import (
	"ta/internal/cultivation"
	"testing"
)

func TestRealmPresetGeneratedStudyTradeEquipAndBattle(t *testing.T) {
	for _, realm := range []string{"foundation", "core", "nascent", "spirit", "void", "integration", "mahayana"} {
		t.Run(realm, func(t *testing.T) {
			system := combatTestSystem()
			// Use the complete cultivation schema: the minimal combat fixture
			// deliberately omits the carried inventory and merchant records.
			spec := xiuxianActorStatePresetSpec()
			system.Templates = []ActorStateTemplate{
				protagonistStateTemplate(cultivation.ReferenceRules(),
					spec), storyContextStateTemplate(spec),
				{ID: "rival", Name: "对手", Fields: protagonistStateTemplate(cultivation.ReferenceRules(),
					spec).Fields},
			}
			merchant := "坊市器阁"
			locations := map[string]string{"void": "spirit.sunset.market", "integration": "spirit.tianyuan.market", "mahayana": "spirit.quiet.valley"}
			merchants := map[string]string{"void": "百工坊书器铺", "integration": "南坊修器铺", "mahayana": "栖霞旧卷斋"}
			if merchants[realm] != "" {
				merchant = merchants[realm]
			}
			for i := range system.InitialActors {
				if system.InitialActors[i].ID == DefaultStoryContextActorID && locations[realm] != "" {
					if system.InitialActors[i].State == nil {
						system.InitialActors[i].State = map[string]any{}
					}
					system.InitialActors[i].State[storySceneLocationField] = journeyPlaceLabel(locations[realm])
				}
				if system.InitialActors[i].ID != DefaultStoryContextActorID {
					system.InitialActors[i].State[xiuxianFieldRealm] = realm + "_early"
					system.InitialActors[i].State[xiuxianFieldCurrency] = float64(100000)
				}
			}
			store := NewStore(t.TempDir())
			defer store.Close()
			story, err := store.CreateStory(CreateStoryRequest{Title: "境界预设消费者", ActorState: &system})
			if err != nil {
				t.Fatal(err)
			}
			if locations[realm] != "" {
				talismanTurn(t, store, story.ID, "main", "购买本境完整注本", &TurnResult{Trade: &TurnTradeInput{Merchant: merchant, Buys: []TurnTradeBuy{{DefID: "manual.spirit." + realm, Name: "本境完整注本"}}}})
			}
			templateID := realm + "_pierce"
			id := cultivation.GeneratedSkillID(templateID, "云纹贯锋")
			talismanTurn(t, store, story.ID, "main", "依完整注本研习云纹贯锋", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: templateID, Name: "云纹贯锋", Source: "完整贯锋注本", Minutes: 10080}}}})
			state := combatTestState(t, store, story.ID)
			registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
				state)
			if !learnedSkillsFromState(state, DefaultActorID, registry).Knows(id) {
				t.Fatal("paid study did not establish mastery")
			}
			generated, ok := registry.Current(id)
			if !ok || generated.ApplicableStage != realm+"_early" {
				t.Fatal("Story replay lost generated realm gate")
			}
			low := xiuxianSkillTestState(t, "qi_13", nil)
			if skillLearnableBy(generated, low, DefaultActorID) == nil {
				t.Fatal("lower realm can study generated high art")
			}
			talismanTurn(t, store, story.ID, "main", "购买本境灵剑", &TurnResult{Trade: &TurnTradeInput{Merchant: merchant, Buys: []TurnTradeBuy{{DefID: "equipment." + realm + "_blade", Name: "新购灵剑"}, {DefID: "equipment." + realm + "_robe", Name: "新购法袍"}}}})
			state = combatTestState(t, store, story.ID)
			if xiuxianStoredInt(state, DefaultActorID, xiuxianFieldCurrency) >= 100000 {
				t.Fatal("purchase did not charge real money")
			}
			talismanTurn(t, store, story.ID, "main", "装备新购灵剑", &TurnResult{Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "新购灵剑"}, {InstanceID: "新购法袍"}}}})
			opened, err := store.StartBattle(story.ID, BattleStartRequest{BranchID: "main", Combatants: combatTestSpecs(), CommandID: "realm-open", CommandHash: "realm-open"})
			if err != nil {
				t.Fatal(err)
			}
			hero := opened.Session.CombatantByID(DefaultActorID)
			if _, ok := hero.EquipmentInSlot(cultivation.EquipmentSlotWeapon); !ok {
				t.Fatal("purchased copy was not equipped")
			}
			command := combatFatalityCommandForHit(opened.Session, DefaultActorID, "rival")
			played, err := store.ResolveBattleRound(story.ID, BattleRoundRequest{BranchID: "main", SessionID: opened.Session.ID, CommandID: command, CommandHash: command, Actions: map[string]cultivation.CombatAction{DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: cultivation.SkillDef{ID: id}, TargetID: "rival"}, "rival": {Kind: cultivation.ActionRest}}})
			if err != nil {
				t.Fatal(err)
			}

			weapon, _ := hero.EquipmentInSlot(cultivation.EquipmentSlotWeapon)
			if weapon.OperatingManaCost <= 0 || hero.Attributes.Attack <= cultivation.BaseAttributes(hero.Cultivation).Attack {
				t.Fatal("equipped purchase lost its actual attack bonus or operating cost")
			}
			robe, hasRobe := hero.EquipmentInSlot(cultivation.EquipmentSlotGarment)
			if weapon.DefensePierce <= 0 || !hasRobe || robe.StrainReduction <= 0 {
				t.Fatal("real equipped copy lost its rated special effect")
			}
			step := findCombatStep(t, played.Outcome, DefaultActorID)
			if step.Cast.BurdenCost >= generated.Burden || step.OperatingManaCost < weapon.OperatingManaCost+robe.OperatingManaCost {
				t.Fatalf("robe relief or its real payment was omitted: %+v", step)
			}
			if step.ActionRejected || step.Cast.ManaCost <= 0 || step.Resolution.HealthLost <= 0 || played.Session.CombatantByID(DefaultActorID).Attributes.Mana >= hero.Attributes.Mana {
				t.Fatalf("cast failed real consumer: %+v", step)
			}
		})
	}
}

// Study eligibility is not the transient ability to cast right now.
func TestRealmPresetStudyEligibilityIgnoresCombatPools(t *testing.T) {
	skill, _ := cultivation.SkillByID("void_pierce")
	state := xiuxianSkillTestState(t, "void_early", map[string]any{
		xiuxianFieldMana: float64(0), xiuxianFieldBurden: float64(100),
	})
	if err := skillLearnableBy(skill, state, DefaultActorID); err != nil {
		t.Fatalf("empty mana or burden blocked stable study eligibility: %v", err)
	}
	strike, _ := cultivation.SkillByID(cultivation.OrdinaryAttackSkillID)
	if skillLearnableBy(strike, state, DefaultActorID) == nil {
		t.Fatal("ordinary attack became a learned technique")
	}
}

func TestRealmEquipmentGenerationRetainsRatedEffectsAndBudgets(t *testing.T) {
	for _, realm := range []string{"foundation", "core", "nascent", "spirit", "void", "integration", "mahayana"} {
		for _, kind := range []string{"blade", "robe"} {
			template := "equipment." + realm + "_" + kind
			base, _ := cultivation.NewItemCatalogWithFirstVersion().Definition(template)
			id, err := generatedContentID("item", template, "另铸云纹器")
			if err != nil {
				t.Fatal(err)
			}
			record := generatedContentRecord("equipment", id, template, "另铸云纹器", "已实际取得的同阶材料与完整炼器传承", "器纹沿既定经络起伏。")
			derived, ok := generatedItemFromRecord(cultivation.ReferenceRules(),
				record)
			if !ok || derived.EffectStage != base.EffectStage || derived.RaritySpentOnSpecialEffect != base.RaritySpentOnSpecialEffect || derived.RaritySpentOnMainStat != base.RaritySpentOnMainStat {
				t.Fatalf("generated %s lost explicit rated effect budget", template)
			}
			stage, _ := cultivation.ParseStageKey(realm + "_late")
			if derived.DefensePierceAmount(stage) != base.DefensePierceAmount(stage) || derived.EquipmentStrainReduction(stage) != base.EquipmentStrainReduction(stage) {
				t.Fatal("generated item invented or dropped effect capacity")
			}
		}
	}
}
