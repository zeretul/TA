package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// The tests here cover 《自制-装备栏与法宝御使》修订2 as the code implements it:
// 武器1件、衣服／法袍1件、攻击／防御／辅助法宝各1件, no extra global
// single-treasure restriction, and every bonus still summed under the old
// 60%／40% ceiling.
//
// The three slots are ordinary fields read by the same settlement that prices the
// implement and the armour. What these tests pin down is the part that is not
// ordinary: that three treasures can be equipped at once and each pays only what
// its own definition published, that a fourth copy of one kind is refused rather
// than stacked, that one real object cannot be hung in two slots, and that a
// world pack written before these slots existed keeps working.

// treasureState builds a character holding the given slots, already at a stage
// that can operate all three registered treasures.
func treasureState(t *testing.T, stage string, slots map[string]any) map[string]any {
	t.Helper()
	return xiuxianTestState(t, stage, slots)
}

// treasureSlotEntry is one equipped copy as the item module records it.
func treasureSlotEntry(defID, name string) map[string]any {
	return map[string]any{"名称": name, "定义": defID}
}

func treasureNumber(t *testing.T, state map[string]any, field string) float64 {
	t.Helper()
	value, ok := actorStateNumber(actorStateFieldValue(state, "protagonist", field))
	if !ok {
		t.Fatalf("%s was not settled", field)
	}
	return value
}

// §5.1: 武器1、法袍1、三类法宝各1，上限由后端执行；同一实物不占两处.
//
// At qi-refining layer 7 the bare attributes are 44 attack, 22 defence and 22
// perception. The attack treasure adds its definition's 15%, the defence
// treasure its 15%, and the support treasure its 18% perception — three separate
// slots, each contributing only the one attribute its own definition names.
func TestAllThreeTreasureSlotsCanBeEquippedAtOnce(t *testing.T) {
	state := treasureState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure:  treasureSlotEntry("treasure.attack.middle", "赤焰葫芦"),
		xiuxianFieldDefenseTreasure: treasureSlotEntry("treasure.defense.middle", "护身镜"),
		xiuxianFieldSupportTreasure: treasureSlotEntry("treasure.support.upper.effect", "千里眼"),
	})
	settled := xiuxianSettle(t, state)

	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 51 {
		t.Errorf("attack with an attack treasure = %v, want 51 (44 × 1.15)", got)
	}
	if got := treasureNumber(t, settled, xiuxianFieldDefense); got != 25 {
		t.Errorf("defence with a defence treasure = %v, want 25 (22 × 1.15)", got)
	}
	if got := treasureNumber(t, settled, xiuxianFieldPerception); got != 26 {
		t.Errorf("perception with a support treasure = %v, want 26 (22 × 1.18)", got)
	}
}

// §5.1: 三类法宝各1 — a fourth treasure cannot become a fourth position. With all
// three slots filled, equipping another copy displaces the one in its kind's
// slot; the equipped count stays three and the total bonus does not grow.
func TestAFourthTreasureCannotBecomeAFourthSlot(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure:  map[string]any{"名称": "赤焰葫芦", "定义": "treasure.attack.middle"},
		xiuxianFieldDefenseTreasure: map[string]any{"名称": "护身镜", "定义": "treasure.defense.middle"},
		xiuxianFieldSupportTreasure: map[string]any{"名称": "千里眼", "定义": "treasure.support.upper.effect"},
		xiuxianFieldImportantItems: map[string]any{
			"第四件葫芦": map[string]any{"定义": "treasure.attack.middle", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "第四件葫芦"}},
	}, "ev1")
	if len(result.Equipped) != 1 {
		t.Fatalf("equipped = %+v, want the copy to land in its kind's existing slot", result.Equipped)
	}
	applied := appliedXiuxianState(t, state, ops)

	// Still exactly three equipped slots, and the displaced copy is owned rather
	// than lost.
	equippedCount := 0
	for _, slot := range xiuxianEquipmentSlots {
		if _, ok := xiuxianEquipmentEntry(applied, "protagonist", slot); ok {
			equippedCount++
		}
	}
	if equippedCount != 3 {
		t.Errorf("equipped slots = %d, want 3; a fourth treasure opened a fourth position", equippedCount)
	}
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if _, returned := pack["赤焰葫芦"]; !returned {
		t.Error("the displaced treasure was destroyed instead of returning to the pack")
	}

	// And the numbers are one attack treasure's, not two.
	settled := xiuxianSettle(t, applied)
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 51 {
		t.Errorf("attack with a fourth treasure attempted = %v, want 51 (one slot's 15%%)", got)
	}
}

// §5.1's other half: 同一实物不占两处. A copy already worn cannot be equipped
// into a second slot — the acquisition path and the equip path both refuse it.
func TestOneCopyCannotOccupyTwoSlots(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure: map[string]any{
			"名称": "赤焰葫芦", "定义": "treasure.attack.middle",
		},
	})

	// Acquiring the same name again would be a second copy of the object the
	// character is already driving.
	ops, result, ok := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "treasure.attack.middle", Name: "赤焰葫芦"}},
	}, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared fields must accept the module")
	}
	if len(ops) != 0 {
		t.Fatalf("acquiring an equipped copy wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "同一件实物") {
		t.Errorf("refused = %+v, want one refusal naming the same object", result.Refused)
	}

	// Moving it into the pack and equipping it into the defence slot is a move,
	// not a second copy — but a treasure is bound to its own kind's slot, so the
	// equip path refuses the cross-slot placement rather than hanging it twice.
	packed := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"赤焰葫芦": map[string]any{"定义": "treasure.attack.middle", "数量": float64(1)},
		},
	})
	ops, result, _ = xiuxianItemOps(packed, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "赤焰葫芦"}},
	}, "ev2")
	if len(result.Equipped) != 1 || result.Equipped[0].Slot != xiuxianFieldAttackTreasure {
		t.Fatalf("equipped = %+v, want the attack treasure slot", result.Equipped)
	}
	applied := appliedXiuxianState(t, packed, ops)
	if _, second := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldDefenseTreasure); second {
		t.Error("the same copy was written into a second slot")
	}
}

// §5.1: 三类法宝各1. A second copy of one kind replaces the one in that slot
// rather than adding a fourth position, and the displaced copy returns to the
// pack.
func TestASecondCopyOfOneKindReplacesRatherThanStacks(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure: map[string]any{
			"名称": "赤焰葫芦", "定义": "treasure.attack.middle",
		},
		xiuxianFieldImportantItems: map[string]any{
			"另一只葫芦": map[string]any{"定义": "treasure.attack.middle", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "另一只葫芦"}},
	}, "ev1")
	if len(result.Equipped) != 1 {
		t.Fatalf("equipped = %+v, want one copy", result.Equipped)
	}
	if got := result.Equipped[0].Slot; got != xiuxianFieldAttackTreasure {
		t.Errorf("equipped into %q, want %q", got, xiuxianFieldAttackTreasure)
	}
	applied := appliedXiuxianState(t, state, ops)
	pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
	if _, returned := pack["赤焰葫芦"]; !returned {
		t.Error("the displaced treasure was destroyed instead of returning to the pack")
	}
	// Two copies of the same definition are still one slot's worth of bonus: 15%,
	// not 30%.
	settled := xiuxianSettle(t, applied)
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 51 {
		t.Errorf("attack after replacing a treasure = %v, want 51 (one slot's 15%%)", got)
	}
}

// §5.2: 包里多放十件不加战力. Only the six declared slots are read, so a pack full
// of treasures contributes nothing.
func TestAPackFullOfTreasuresAddsNothing(t *testing.T) {
	bag := map[string]any{}
	for i, name := range []string{
		"备用葫芦一", "备用葫芦二", "备用葫芦三", "备用葫芦四", "备用葫芦五",
		"备用镜一", "备用镜二", "备用镜三", "备用眼一", "备用眼二",
	} {
		defID := "treasure.attack.middle"
		if i >= 5 && i < 8 {
			defID = "treasure.defense.middle"
		} else if i >= 8 {
			defID = "treasure.support.upper.effect"
		}
		bag[name] = map[string]any{"定义": defID, "数量": float64(1)}
	}
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImportantItems: bag,
	})
	settled := xiuxianSettle(t, state)
	for field, want := range map[string]float64{
		xiuxianFieldAttack:     44,
		xiuxianFieldDefense:    22,
		xiuxianFieldPerception: 22,
	} {
		if got := treasureNumber(t, settled, field); got != want {
			t.Errorf("%s with ten spare treasures in the pack = %v, want the bare %v", field, got, want)
		}
	}
}

// §5.3: 允许共存的效果不被全局单御使限制误清. The revision removed the draft's
// "only one treasure may be driven at a time" rule, so all three equipped
// treasures must reach the aggregation as separate sources at the same time.
func TestAllThreeTreasuresReachTheAggregationTogether(t *testing.T) {
	state := treasureState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure:  treasureSlotEntry("treasure.attack.middle", "赤焰葫芦"),
		xiuxianFieldDefenseTreasure: treasureSlotEntry("treasure.defense.middle", "护身镜"),
		xiuxianFieldSupportTreasure: treasureSlotEntry("treasure.support.upper.effect", "千里眼"),
	})

	modifiers := xiuxianEquipmentModifiers(cultivation.ReferenceRules(),
		state, "protagonist")
	var treasures []cultivation.AttributeModifier
	for _, modifier := range modifiers {
		if modifier.Kind == cultivation.SourceTreasure {
			treasures = append(treasures, modifier)
		}
	}
	if len(treasures) != 3 {
		t.Fatalf("treasure modifiers = %+v, want all three slots to contribute", treasures)
	}
	for _, slot := range xiuxianTreasureSlots {
		found := false
		for _, modifier := range treasures {
			if strings.Contains(modifier.ID, ":"+slot+":") {
				found = true
			}
		}
		if !found {
			t.Errorf("no modifier came from the %s slot; a global single-treasure rule is still cutting the others", slot)
		}
	}
}

// §5.5: 最大合法组合仍守旧预算. A weapon, a robe and all three treasures together
// are five positive sources on attack and defence, and the 60% ceiling still
// binds: 15% (implement) + 15% (attack treasure) is 30%, not 30% plus a free
// second helping from the treasure slots.
func TestTheFullLegalKitStaysUnderTheSixtyPercentCap(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "青锋剑", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
		xiuxianFieldArmor: map[string]any{
			"名称": "玄龟甲", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
		xiuxianFieldAttackTreasure:  treasureSlotEntry("treasure.attack.middle", "赤焰葫芦"),
		xiuxianFieldDefenseTreasure: treasureSlotEntry("treasure.defense.middle", "护身镜"),
		xiuxianFieldSupportTreasure: treasureSlotEntry("treasure.support.upper.effect", "千里眼"),
	})
	settled := xiuxianSettle(t, state)

	// Attack: the top unique implement alone is 35%, plus the attack treasure's
	// 15% is 50%, which is under the cap. 44 × 1.50 = 66.
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 66 {
		t.Errorf("attack with the full kit = %v, want 66 (44 × 1.50)", got)
	}
	// Defence: the same arithmetic on the robe and the defence treasure.
	if got := treasureNumber(t, settled, xiuxianFieldDefense); got != 33 {
		t.Errorf("defence with the full kit = %v, want 33 (22 × 1.50)", got)
	}
	// Perception: the support treasure is the only source, at its own 18%.
	if got := treasureNumber(t, settled, xiuxianFieldPerception); got != 26 {
		t.Errorf("perception with the full kit = %v, want 26 (22 × 1.18)", got)
	}
}

// The same kit with a boosting pill on top is what actually reaches the ceiling:
// 35% implement + 15% attack treasure + 15% pill is 65 points, clamped to 60.
func TestTheFullKitPlusAPillIsClampedAtSixtyPercent(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "青锋剑", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
		xiuxianFieldAttackTreasure: treasureSlotEntry("treasure.attack.middle", "赤焰葫芦"),
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "增攻药", "属性": "攻击", "数值": float64(15), "同类": "增攻药", "剩余轮次": float64(3)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 70 {
		t.Errorf("attack = %v, want 70 (44 × 1.60, the capped sum)", got)
	}
}

// §5.1: 上限由后端执行. The slot a copy occupies is chosen from its definition's
// kind, so a request cannot decide for itself where a treasure is worn.
func TestTheSlotIsChosenFromTheDefinitionNotTheRequest(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"赤焰葫芦": map[string]any{"定义": "treasure.attack.middle", "数量": float64(1)},
			"护身镜":  map[string]any{"定义": "treasure.defense.middle", "数量": float64(1)},
			"千里眼":  map[string]any{"定义": "treasure.support.upper.effect", "数量": float64(1)},
		},
	})

	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{
			{InstanceID: "赤焰葫芦"}, {InstanceID: "护身镜"}, {InstanceID: "千里眼"},
		},
	}, "ev1")
	if len(result.Equipped) != 3 {
		t.Fatalf("equipped = %+v, want all three", result.Equipped)
	}
	want := map[string]string{
		"赤焰葫芦": xiuxianFieldAttackTreasure,
		"护身镜":  xiuxianFieldDefenseTreasure,
		"千里眼":  xiuxianFieldSupportTreasure,
	}
	for _, equipped := range result.Equipped {
		if want[equipped.Name] != equipped.Slot {
			t.Errorf("%s equipped into %q, want %q", equipped.Name, equipped.Slot, want[equipped.Name])
		}
	}
	applied := appliedXiuxianState(t, state, ops)
	if _, stillPacked := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems); stillPacked {
		pack, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldImportantItems)
		if len(pack) != 0 {
			t.Errorf("pack = %+v, want the equipped copies to have left it", pack)
		}
	}
}

// A treasure whose stage the character has not reached is refused, which is §3's
// 炼气能使用低阶护身法器，不代表能祭炼元婴人物的本命宝物.
func TestATreasureAboveTheCharactersStageIsRefused(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(4)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"千里眼": map[string]any{"定义": "treasure.support.upper.effect", "数量": float64(1)},
		},
	})
	ops, result, _ := xiuxianItemOps(state, system, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "千里眼"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a refused equip wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "境界不足") {
		t.Errorf("refused = %+v, want one refusal naming the stage", result.Refused)
	}
}

// A treasure the catalogue has no definition for contributes nothing. It is still
// owned and still occupies its slot — that is what the panel shows — but the
// numbers come from the catalogue, never from the recorded wording.
func TestAnUncataloguedTreasureContributesNothing(t *testing.T) {
	state := treasureState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldAttackTreasure: map[string]any{
			"名称": "来历不明的葫芦", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
	})
	settled := xiuxianSettle(t, state)
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 44 {
		t.Errorf("attack with an uncatalogued treasure = %v, want the bare 44", got)
	}
}

// xiuxianSystemWithout builds the real preset with the named fields removed from
// the protagonist template, so a test can reproduce a world pack written before
// those fields existed without hand-writing a template the preset check would
// reject.
func xiuxianSystemWithout(t *testing.T, drop ...string) StoryDirectorActorStateSystem {
	t.Helper()
	dropped := make(map[string]bool, len(drop))
	for _, name := range drop {
		dropped[name] = true
	}
	system := xiuxianTestSystem()
	for i, template := range system.Templates {
		if template.ID != DefaultActorID {
			continue
		}
		kept := make([]ActorStateField, 0, len(template.Fields))
		for _, field := range template.Fields {
			if dropped[actorStateFieldID(field)] {
				continue
			}
			kept = append(kept, field)
		}
		system.Templates[i].Fields = kept
	}
	return system
}

// A world pack written before the three treasure slots existed declares only
// 主法器, 护具 and 布阵. The item module must keep working for it — acquire, equip
// and unequip on the old slots — rather than disabling itself because the newer
// fields are absent.
func TestAnOlderPackWithoutTreasureSlotsStillWorks(t *testing.T) {
	old := xiuxianSystemWithout(t, xiuxianTreasureSlots...)
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, result, ok := xiuxianItemOps(state, old, "protagonist", TurnItemsInput{
		Acquire: []TurnItemAcquire{{DefID: "implement.middle.rare", Name: "青锋剑"}},
		Equip:   []TurnItemEquip{{InstanceID: "青锋剑"}},
	}, "ev1")
	if !ok {
		t.Fatal("a pack that declares the three older slots must still accept the item module")
	}
	if len(result.Acquired) != 1 || len(result.Equipped) != 1 {
		t.Fatalf("acquired = %+v equipped = %+v, want both", result.Acquired, result.Equipped)
	}
	if got := result.Equipped[0].Slot; got != xiuxianFieldImplement {
		t.Errorf("equipped into %q, want %q", got, xiuxianFieldImplement)
	}
	applied := appliedXiuxianState(t, state, ops)
	settled := xiuxianSettle(t, applied)
	if got := treasureNumber(t, settled, xiuxianFieldAttack); got != 46 {
		t.Errorf("attack = %v, want 46; an older pack lost its implement", got)
	}
}

// Equipping a treasure into a pack that declares the older slots but not this
// treasure's slot is refused with the slot named, rather than writing a value the
// interface cannot read.
func TestATreasureIsRefusedWhenTheTemplateHasNoSlotForIt(t *testing.T) {
	partial := xiuxianSystemWithout(t, xiuxianFieldAttackTreasure)
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(7)), map[string]any{
		xiuxianFieldImportantItems: map[string]any{
			"赤焰葫芦": map[string]any{"定义": "treasure.attack.middle", "数量": float64(1)},
		},
	})
	ops, result, _ := xiuxianItemOps(state, partial, "protagonist", TurnItemsInput{
		Equip: []TurnItemEquip{{InstanceID: "赤焰葫芦"}},
	}, "ev1")
	if len(ops) != 0 {
		t.Fatalf("a refused equip wrote state: %+v", ops)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, xiuxianFieldAttackTreasure) {
		t.Errorf("refused = %+v, want one refusal naming the missing slot", result.Refused)
	}
}
