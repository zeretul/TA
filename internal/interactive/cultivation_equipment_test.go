package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// The tests here cover the join between the recorded equipment a story holds and
// the attribute aggregation of 《自制-修仙数值规则》 §3.4 and
// 《自制-装备与物品品级》 §4, §5 and §10.
//
// Before this wiring existed the settlement passed no modifiers at all, so every
// rule below was implemented and tested in internal/cultivation but unreachable
// during play: an equipped implement, a worn piece of armour, a deployed
// formation and a lasting boost were all invisible to the numbers the player
// reads. These tests assert the numbers the panel would actually show.

func xiuxianSettle(t *testing.T, state map[string]any) map[string]any {
	t.Helper()
	ops, ok := settleXiuxianActorState(state, xiuxianTestSystem(), "protagonist", "ev1")
	if !ok {
		t.Fatal("a tracked character must settle")
	}
	return appliedXiuxianState(t, state, ops)
}

func xiuxianSettledNumber(t *testing.T, state map[string]any, field string) float64 {
	t.Helper()
	value, ok := actorStateNumber(actorStateFieldValue(state, "protagonist", field))
	if !ok {
		t.Fatalf("%s was not settled", field)
	}
	return value
}

// equipmentState builds a character state holding one equipment slot.
func equipmentState(t *testing.T, stage string, slot string, entry map[string]any) map[string]any {
	t.Helper()
	return xiuxianTestState(t, stage, map[string]any{slot: entry})
}

func TestAnEquippedImplementRaisesTheSettledAttack(t *testing.T) {
	// 中品稀有 with the whole rarity budget on the main attribute is the
	// published 15% ceiling for that pair. At qi-refining layer 6 the bare
	// attack is 40, so 40 × 1.15 = 46.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "青锋剑", "品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 46 {
		t.Errorf("attack with a middle-grade rare implement = %v, want 46 (40 × 1.15)", got)
	}
	// The implement is a main-attack item: it must not also move defence.
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldDefense); got != 20 {
		t.Errorf("an implement changed defence to %v, want the bare 20", got)
	}
}

func TestAWhiteImplementIsFivePercentAndCannotBeThirtyFive(t *testing.T) {
	// §10: 白装＋5%且不能变成＋35%. 40 × 1.05 = 42.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "铁剑", "品级": "下品", "稀有度": "常见", "主要属性预算": float64(0),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 42 {
		t.Errorf("attack with a white implement = %v, want 42 (40 × 1.05)", got)
	}
	// Claiming the top-tier spend on a white item cannot reach 35%: the budget is
	// clamped to the rarity's allowance, which for 常见 is nothing.
	inflated := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "假宝", "品级": "下品", "稀有度": "常见", "主要属性预算": float64(10),
	})
	settled = xiuxianSettle(t, inflated)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 42 {
		t.Errorf("a white implement claiming the unique budget = %v, want 42", got)
	}
}

func TestTopUniqueIsTheOnlyPairReachingThirtyFive(t *testing.T) {
	// 顶品独特: 25% grade plus the 10% rarity budget. 40 × 1.35 = 54.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "斩仙剑", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 54 {
		t.Errorf("attack with a top unique implement = %v, want 54 (40 × 1.35)", got)
	}
}

func TestArmourRaisesDefenceAndNotAttack(t *testing.T) {
	// §4.1: 同档护具主要增加防御，不把这一个百分比同时加到全部六属性.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldArmor, map[string]any{
		"名称": "玄龟甲", "品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldDefense); got != 23 {
		t.Errorf("defence with middle-grade rare armour = %v, want 23 (20 × 1.15)", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("armour changed attack to %v, want the bare 40", got)
	}
}

func TestTheGlobalSixtyPercentCapBindsAStackedKit(t *testing.T) {
	// §10: 全局60%仍有效. Three genuinely different reasons — a top-tier unique
	// implement, a boosting pill and a separate secret technique — sum to 35 + 15
	// + 15 = 65 percentage points, which is past the published 60% ceiling. The
	// aggregation must clamp the sum rather than the individual sources, so the
	// character settles at 40 × 1.60 = 64 and not at 66.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "斩仙剑", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "增攻药", "属性": "攻击", "数值": float64(15), "同类": "增攻药", "剩余轮次": float64(3)},
			map[string]any{"名称": "燃血秘法", "属性": "攻击", "数值": float64(15), "同类": "秘法", "剩余轮次": float64(2)},
		},
	})
	settled := xiuxianSettle(t, state)
	got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack)
	if got != 64 {
		t.Errorf("stacked attack = %v, want 64 (40 × 1.60, the capped sum)", got)
	}
}

func TestTwoBoostsInDifferentGroupsBothCount(t *testing.T) {
	// The other half of 同类取强: a different reason is a different group, and
	// both apply. 15 + 15 = 30 points, so 40 × 1.30 = 52.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "增攻药", "属性": "攻击", "数值": float64(15), "同类": "增攻药", "剩余轮次": float64(3)},
			map[string]any{"名称": "燃血秘法", "属性": "攻击", "数值": float64(15), "同类": "秘法", "剩余轮次": float64(2)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 52 {
		t.Errorf("two different-group boosts = %v, want 52 (40 × 1.30)", got)
	}
}

func TestASecondSameTypeBoostDoesNotStack(t *testing.T) {
	// §4.2: 增攻持续3轮，同类取最强. Two boosting entries in one stacking group
	// leave only the stronger, so attack is 40 × 1.15 = 46 rather than 40 × 1.20.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "下品增攻药", "属性": "攻击", "数值": float64(5), "同类": "增攻药", "剩余轮次": float64(3)},
			map[string]any{"名称": "上品增攻药", "属性": "攻击", "数值": float64(12), "同类": "增攻药", "剩余轮次": float64(3)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 45 {
		t.Errorf("two same-type boosts = %v, want 45 (40 × 1.12, the stronger alone)", got)
	}
}

func TestAnExpiredBoostPaysNothing(t *testing.T) {
	// A recorded effect whose term has run out is over. The story is instructed to
	// clear it, but honouring the countdown here means a stale entry cannot keep
	// paying while the record is being tidied.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "过期增攻药", "属性": "攻击", "数值": float64(15), "同类": "增攻药", "剩余轮次": float64(0)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("an expired boost still applied: attack = %v, want the bare 40", got)
	}
}

func TestADeployedFormationPaysAndAnAbandonedOneDoesNot(t *testing.T) {
	// §4.3: 离开阵区、失去维持能力、阵盘被破坏时失去相应效果. 中品增攻阵 is 8%, so
	// 40 × 1.08 = 43.
	deployed := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldFormation: map[string]any{
			"名称": "聚灵阵", "品级": "中品", "增益": "攻击", "维持中": "是",
		},
	})
	settled := xiuxianSettle(t, deployed)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 43 {
		t.Errorf("attack inside a middle-grade attack formation = %v, want 43 (40 × 1.08)", got)
	}

	abandoned := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldFormation: map[string]any{
			"名称": "聚灵阵", "品级": "中品", "增益": "攻击", "维持中": "否",
		},
	})
	settled = xiuxianSettle(t, abandoned)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("an abandoned formation still applied: attack = %v, want the bare 40", got)
	}
}

func TestAProtectiveFormationRaisesDefenceOnly(t *testing.T) {
	// 中品防护阵 is 10%: 20 × 1.10 = 22.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldFormation: map[string]any{
			"名称": "磐石阵", "品级": "中品", "增益": "防御", "维持中": "是",
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldDefense); got != 22 {
		t.Errorf("defence inside a middle-grade protective formation = %v, want 22 (20 × 1.10)", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("a protective formation changed attack to %v, want the bare 40", got)
	}
}

func TestABackpackOfImplementsDoesNotAllAddAttack(t *testing.T) {
	// §10: 背包法器不全加攻. The slot holds ONE implement, and the other items a
	// character carries are recorded under 重要物品, which the settlement never
	// reads. So carrying more items changes nothing.
	withBag := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImplement: map[string]any{
			"名称": "斩仙剑", "品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
		},
		"重要物品": map[string]any{
			"备用剑": map[string]any{"品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10)},
			"备用甲": map[string]any{"品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10)},
		},
	})
	settled := xiuxianSettle(t, withBag)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 54 {
		t.Errorf("carrying spare implements changed attack to %v, want 54 (one wielded at 1.35)", got)
	}
}

func TestAnUnreadableGradeContributesNothing(t *testing.T) {
	// An item whose grade the rules cannot read has no published ceiling, so it
	// contributes nothing rather than a guessed one. This is what stops a
	// narrative from inventing a grade to obtain a bonus.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "来历不明的剑", "品级": "传说", "稀有度": "独特", "主要属性预算": float64(10),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("an unreadable grade gave attack %v, want the bare 40", got)
	}
}

func TestAPlainNameInASlotIsNotAnItem(t *testing.T) {
	// A story that wrote 主法器: "青锋剑" has recorded a name, not a priced item.
	// Pricing it would require inventing a grade, so it contributes nothing.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldImplement: "青锋剑",
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("a bare name gave attack %v, want the bare 40", got)
	}
}

func TestTheEnglishTableKeysAreAcceptedToo(t *testing.T) {
	// The tables are keyed in English while the field is written in the story's
	// language. Both forms must resolve, so a story that records the rules' own
	// keys is priced identically.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "Middle rare", "品级": "middle", "稀有度": "rare", "主要属性预算": float64(5),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 46 {
		t.Errorf("English table keys gave attack %v, want 46 (40 × 1.15)", got)
	}
}

func TestEquipmentAloneDoesNotHeal(t *testing.T) {
	// §5: 提升上限不免费回满资源. A wound is a real fact, so raising a maximum
	// must not restore the current pool.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: float64(50),
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "固本丹", "属性": "生命上限", "数值": float64(10), "同类": "固本", "剩余轮次": float64(3)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealthMax); got != 220 {
		t.Errorf("health maximum with a 10%% bonus = %v, want 220 (200 × 1.10)", got)
	}
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldHealth); got != 50 {
		t.Errorf("raising the maximum healed the character to %v, want the recorded 50", got)
	}
}

func TestTheSettledNumbersStayStableAcrossReplays(t *testing.T) {
	// The same recorded state must settle to the same integers on every run, or
	// two replays of one branch would show different numbers.
	build := func() map[string]any {
		return xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
			xiuxianFieldImplement: map[string]any{
				"名称": "青锋剑", "品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
			},
			xiuxianFieldEffects: []any{
				map[string]any{"名称": "增攻药", "属性": "攻击", "数值": float64(8), "同类": "增攻药", "剩余轮次": float64(3)},
			},
		})
	}
	first := xiuxianSettle(t, build())
	second := xiuxianSettle(t, build())
	for _, field := range []string{xiuxianFieldAttack, xiuxianFieldDefense, xiuxianFieldHealthMax, xiuxianFieldManaMax} {
		a := xiuxianSettledNumber(t, first, field)
		b := xiuxianSettledNumber(t, second, field)
		if a != b {
			t.Errorf("%s settled to %v then %v; the same state must give the same numbers", field, a, b)
		}
	}
	// And the settled attack is the one aggregation's answer: 40 × 1.23 = 49.2,
	// rounded half up to 49.
	if got := xiuxianSettledNumber(t, first, xiuxianFieldAttack); got != 49 {
		t.Errorf("attack = %v, want 49 (40 × 1.23)", got)
	}
}

// A boosting effect that names a registered pill is priced from the catalogue's
// grade table, not from the number the story wrote beside it. That is what makes
// §4.2's 增攻药 5%／8%／12%／15% a property of the pill: the entry below claims 40%
// and is paid the middle grade's 8%, because 中品增攻药 is 8% by the table.
func TestARegisteredBoostPillIsPricedByItsGradeNotItsRecordedNumber(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{
				"名称": "中品增攻药", "定义": "pill.boost.attack.middle",
				"属性": "攻击", "数值": float64(40), "同类": "增攻药", "剩余轮次": float64(3),
			},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 43 {
		t.Errorf("attack = %v, want 43 (40 × 1.08, the grade's 8%% and not the recorded 40)", got)
	}
}

// The other side of the same rule: an entry that claims a registered definition
// the catalogue does not hold contributes nothing. It must not fall back to the
// recorded number, or a story could write 定义: anything and keep its own figure.
func TestAnEffectNamingAnUnregisteredDefinitionPaysNothing(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{
				"名称": "伪造增攻药", "定义": "pill.boost.attack.made_up",
				"属性": "攻击", "数值": float64(40), "同类": "增攻药", "剩余轮次": float64(3),
			},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack = %v, want the bare 40; an unregistered definition must not be paid", got)
	}
}

// A registered definition that is not an attack-boosting pill has no grade table
// to read, so it also contributes nothing rather than an invented size.
func TestAnEffectNamingARegisteredNonPillPaysNothing(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{
				"名称": "青锋剑", "定义": "implement.lower.common",
				"属性": "攻击", "数值": float64(40), "同类": "法器", "剩余轮次": float64(3),
			},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Errorf("attack = %v, want the bare 40; an implement is not a boosting pill", got)
	}
}

// An effect with no 定义 at all is a blessing, a talisman or another source the
// rules do not catalogue, and keeps its recorded number. That is the honest
// reading for a source with no registered definition, and it is what the other
// effect tests in this file rely on.
func TestAnUncataloguedEffectKeepsItsRecordedNumber(t *testing.T) {
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldEffects: []any{
			map[string]any{"名称": "祖师余荫", "属性": "攻击", "数值": float64(12), "同类": "祝福", "剩余轮次": float64(3)},
		},
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 45 {
		t.Errorf("attack = %v, want 45 (40 × 1.12, the recorded number)", got)
	}
}

// 适用阶段不是点击颜色就能越过: the equip path already refuses a copy above the
// character's stage, but the settlement is what actually prices the slot. A state
// the item module never wrote — a slot an authored state op filled in, or one a
// fork inherited — would otherwise be paid at its full bonus by a character who
// could not wield it, and the panel would read a number the rules never granted.
func TestASlotAboveTheCharactersStageContributesNothing(t *testing.T) {
	// 顶品独特 is foundation_early. At qi-refining layer 6 the bare attack is 40
	// and the blade would claim 35% of it; a character who cannot operate the
	// sword must settle at the bare number.
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(6)), xiuxianFieldImplement, map[string]any{
		"名称": "斩仙剑", "定义": "implement.top.unique",
		"品级": "顶品", "稀有度": "独特", "主要属性预算": float64(10),
	})
	settled := xiuxianSettle(t, state)
	if got := xiuxianSettledNumber(t, settled, xiuxianFieldAttack); got != 40 {
		t.Fatalf("attack with an unreachable blade = %v, want the bare 40", got)
	}
}

// The same rule for the armour slot, because the two are read by the same
// collector and a guard on only one of them would leave the other paying.
// 中品稀有 armour is qi_4, so a qi-refining layer 1 character cannot wear it.
func TestArmourAboveTheCharactersStageContributesNothing(t *testing.T) {
	state := equipmentState(t, cultivation.StageKey(cultivation.Qi(1)), xiuxianFieldArmor, map[string]any{
		"名称": "玄龟甲", "定义": "armor.middle.rare",
		"品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
	})
	settled := xiuxianSettle(t, state)
	bare := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(1)), nil)
	bareSettled := xiuxianSettle(t, bare)
	if got, want := xiuxianSettledNumber(t, settled, xiuxianFieldDefense), xiuxianSettledNumber(t, bareSettled, xiuxianFieldDefense); got != want {
		t.Fatalf("defence with an unreachable armour = %v, want the bare %v", got, want)
	}
}
