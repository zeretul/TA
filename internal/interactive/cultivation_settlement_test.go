package interactive

import (
	"testing"

	"ta/internal/cultivation"
)

// xiuxianTestSystem builds the real preset's system so the settlement is
// exercised against the fields it will actually run against, not a hand-written
// stand-in that could drift from the preset.
//
// It goes through the same module builder the application installs, because the
// protagonist template is assembled from the panel fields, the state fields and
// the owned-record fields (技能与能力, 重要物品, 关系) plus the learned-method
// record. Reproducing only the first two here left 重要物品 undeclared, which the
// item rules correctly read as "this template cannot hold a pack".
func xiuxianTestSystem() StoryDirectorActorStateSystem {
	return actorStateSystemForPreset(cultivation.ReferenceRules(),
		xiuxianActorStatePresetSpec())
}

func xiuxianTestState(t *testing.T, stage string, state map[string]any) map[string]any {
	t.Helper()
	if state == nil {
		state = map[string]any{}
	}
	state[xiuxianFieldRealm] = stage
	return map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": state,
	}}}
}

func appliedXiuxianState(t *testing.T, state map[string]any, ops []ActorStateOp) map[string]any {
	t.Helper()
	working := cloneActorStateRoot(state)
	for _, op := range ops {
		applyActorStateOp(working, op)
	}
	return working
}

func TestXiuxianSettlementWritesTheStageTablesRealIntegers(t *testing.T) {
	system := xiuxianTestSystem()
	// The rules fix qi-refining layer 6 at 200 health, 160 mana, and 40 attack
	// with defence, agility and perception at 20. A character with no recorded
	// wound starts at the full pools.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	ops, ok := settleXiuxianActorState(state, system, "protagonist", "ev1")
	if !ok {
		t.Fatal("a tracked character must settle")
	}
	settled := appliedXiuxianState(t, state, ops)
	for field, want := range map[string]float64{
		xiuxianFieldHealthMax:  200,
		xiuxianFieldManaMax:    160,
		xiuxianFieldAttack:     40,
		xiuxianFieldDefense:    20,
		xiuxianFieldAgility:    20,
		xiuxianFieldPerception: 20,
		xiuxianFieldHealth:     200,
		xiuxianFieldMana:       160,
	} {
		got, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", field))
		if got != want {
			t.Errorf("%s = %v, want %v", field, got, want)
		}
	}
}

func TestXiuxianSettlementClampsAStoredWoundToTheDerivedMaximum(t *testing.T) {
	system := xiuxianTestSystem()
	// A real wound is stored, not derived. The settlement must keep it rather
	// than healing the character back to full.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: float64(37),
	})
	ops, ok := settleXiuxianActorState(state, system, "protagonist", "ev1")
	if !ok {
		t.Fatal("a tracked character must settle")
	}
	settled := appliedXiuxianState(t, state, ops)
	health, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealth))
	if health != 37 {
		t.Fatalf("a recorded wound must survive settlement, got %v", health)
	}
}

func TestXiuxianSettlementTrimsAPoolAboveAFallenMaximum(t *testing.T) {
	system := xiuxianTestSystem()
	// Losing a bonus can lower the maximum. The design requires only the excess
	// above the new maximum to be trimmed, not the whole pool to be refilled.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(1)), map[string]any{
		xiuxianFieldHealth: float64(9999),
	})
	ops, ok := settleXiuxianActorState(state, system, "protagonist", "ev1")
	if !ok {
		t.Fatal("a tracked character must settle")
	}
	settled := appliedXiuxianState(t, state, ops)
	health, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealth))
	maximum, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealthMax))
	if health != maximum {
		t.Fatalf("a pool above the maximum must be trimmed to it, got %v with max %v", health, maximum)
	}
}

func TestXiuxianSettlementFollowsTheStoredStageNotTheNarrative(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(1)), nil)
	settledQi, _ := settleXiuxianActorState(state, system, "protagonist", "ev1")
	appliedQi := appliedXiuxianState(t, state, settledQi)

	// Advancing the stored stage is the only way to raise the numbers, and it is
	// the stage key alone that decides them.
	advanced := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(13)), nil)
	settledAdvanced, _ := settleXiuxianActorState(advanced, system, "protagonist", "ev2")
	appliedAdvanced := appliedXiuxianState(t, advanced, settledAdvanced)

	qiAttack, _ := actorStateNumber(actorStateFieldValue(appliedQi, "protagonist", xiuxianFieldAttack))
	advancedAttack, _ := actorStateNumber(actorStateFieldValue(appliedAdvanced, "protagonist", xiuxianFieldAttack))
	if advancedAttack <= qiAttack {
		t.Fatalf("a later stage must settle higher numbers: layer 1 attack %v, layer 13 attack %v", qiAttack, advancedAttack)
	}
}

func TestXiuxianSettlementRefusesAnUnreadableStageKey(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, "炼气九层（编造）", nil)
	if _, ok := settleXiuxianActorState(state, system, "protagonist", "ev1"); ok {
		t.Fatal("an unreadable stage key must not settle silently to a mortal")
	}
}

func TestXiuxianSettlementIgnoresAnUntrackedActor(t *testing.T) {
	system := xiuxianTestSystem()
	// A bystander under a template without the rules' stage field is not on the
	// table, and must not gain fields their template never declared.
	state := map[string]any{"actors": map[string]any{
		"protagonist": map[string]any{"id": "protagonist", "template_id": "protagonist", "state": map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6))}},
		"路人":          map[string]any{"id": "路人", "template_id": "bystander", "state": map[string]any{}},
	}}
	system.Templates = append(system.Templates, ActorStateTemplate{ID: "bystander", Fields: []ActorStateField{{Name: "行踪", Type: "text"}}})

	ops := settleXiuxianStoryState(state, system, "ev1")
	for _, op := range ops {
		if op.ActorID == "路人" {
			t.Fatalf("an untracked actor must not be settled: %#v", op)
		}
	}
	if len(ops) == 0 {
		t.Fatal("the tracked protagonist must still settle")
	}
}

func TestXiuxianSettlementIsIdempotentAndSilentWhenNothingChanged(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)

	first, ok := settleXiuxianActorState(state, system, "protagonist", "ev1")
	if !ok || len(first) == 0 {
		t.Fatal("the first settlement must write the derived numbers")
	}
	settled := appliedXiuxianState(t, state, first)

	// Re-settling a character who did not change must write nothing: the journal
	// is the record the player reads, and eight redundant entries per turn would
	// bury the real ones.
	second, ok := settleXiuxianActorState(settled, system, "protagonist", "ev2")
	if !ok {
		t.Fatal("a tracked character must settle")
	}
	if len(second) != 0 {
		t.Fatalf("an unchanged character must settle to no operations, got %#v", second)
	}
}

func TestXiuxianSettlementIsStableAcrossActorOrder(t *testing.T) {
	system := xiuxianTestSystem()
	system.Templates = append(system.Templates, ActorStateTemplate{
		ID:     "rival",
		Fields: append([]ActorStateField{}, xiuxianActorStatePresetSpec().PanelFields...),
	})
	build := func() map[string]any {
		return map[string]any{"actors": map[string]any{
			"protagonist": map[string]any{"id": "protagonist", "template_id": "protagonist", "state": map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6))}},
			"rival":       map[string]any{"id": "rival", "template_id": "rival", "state": map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(3))}},
		}}
	}
	first := settleXiuxianStoryState(build(), system, "ev1")
	second := settleXiuxianStoryState(build(), system, "ev1")
	if len(first) != len(second) {
		t.Fatalf("settlement must be order-stable: %d vs %d operations", len(first), len(second))
	}
	for i := range first {
		if first[i].ActorID != second[i].ActorID || first[i].FieldID != second[i].FieldID {
			t.Fatalf("settlement order differs at %d: %#v vs %#v", i, first[i], second[i])
		}
	}
}

func TestXiuxianSettlementReadsSpecialisationAndQualityFromTheMethodField(t *testing.T) {
	system := xiuxianTestSystem()
	plain := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	plainOps, _ := settleXiuxianActorState(plain, system, "protagonist", "ev1")
	plainSettled := appliedXiuxianState(t, plain, plainOps)
	plainAttack, _ := actorStateNumber(actorStateFieldValue(plainSettled, "protagonist", xiuxianFieldAttack))

	// The method field is prose, but it carries the enumerated template and
	// grade. An attack-specialised advanced technique must settle higher than an
	// undetermined one, and it must do so through the same aggregation path
	// rather than a separate bonus.
	skilled := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldMethod: "主修《烈阳诀》，强攻方向，高阶功法",
	})
	skilledOps, _ := settleXiuxianActorState(skilled, system, "protagonist", "ev1")
	skilledSettled := appliedXiuxianState(t, skilled, skilledOps)
	skilledAttack, _ := actorStateNumber(actorStateFieldValue(skilledSettled, "protagonist", xiuxianFieldAttack))

	if skilledAttack <= plainAttack {
		t.Fatalf("an attack-specialised advanced method must settle higher attack: plain %v, skilled %v", plainAttack, skilledAttack)
	}
}

func TestXiuxianSettlementNeverWritesTheRealmItself(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	ops, _ := settleXiuxianActorState(state, system, "protagonist", "ev1")
	// The stage is an input to the settlement, never an output. A settlement that
	// wrote it could silently advance or reset a character's cultivation.
	for _, op := range ops {
		if op.FieldID == xiuxianFieldRealm {
			t.Fatalf("the settlement must not write the stage: %#v", op)
		}
	}
}

func TestXiuxianTurnSettlementUsesTheStageTheTurnReached(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	// A turn that advances the character's cultivation writes the new stage in
	// its own operations. The settlement has to see that stage: settling against
	// the pre-turn state would publish qi-refining numbers beside a
	// foundation-establishment stage key, which is exactly the panel/narrative
	// disagreement the settlement exists to prevent.
	advance := []ActorStateOp{{
		Op: "set", ActorID: "protagonist", FieldID: xiuxianFieldRealm,
		Value: cultivation.StageKey(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly}),
	}}
	ops := settleXiuxianTurnState(state, system, nil, advance, "ev1")
	if len(ops) == 0 {
		t.Fatal("a stage advance must settle new numbers")
	}
	settled := appliedXiuxianState(t, state, append(advance, ops...))

	want := cultivation.BaseAttributes(cultivation.Cultivation{Realm: cultivation.RealmFoundation, Stage: cultivation.StageEarly})
	got, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealthMax))
	if got != float64(want.HealthMax) {
		t.Fatalf("settlement must use the stage the turn reached: health max %v, want %v", got, want.HealthMax)
	}
}

func TestXiuxianTurnSettlementLeavesOtherPresetsAlone(t *testing.T) {
	// The settlement is scoped to the cultivation system. A different genre must
	// not gain settlement writes it never declared.
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "生命", Type: "text"}},
	}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": map[string]any{"生命": "10/10"},
	}}}
	if ops := settleXiuxianTurnState(state, system, nil, nil, "ev1"); len(ops) != 0 {
		t.Fatalf("a non-cultivation system must not settle: %#v", ops)
	}
}

func TestXiuxianTurnSettlementSeesEachOperationExactlyOnce(t *testing.T) {
	system := xiuxianTestSystem()
	// Rule consumption writes its operations into the live state in place, so the
	// caller snapshots the pre-turn state and hands the snapshot over. This pins
	// the contract: the projected state is the snapshot plus the turn's operations
	// applied once each. A caller that passed the live post-rule map instead would
	// have the rule writes land twice, and a relative operation would double.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), map[string]any{
		xiuxianFieldHealth: float64(100),
	})
	// A wound of 30 applied once leaves 70; applied twice it would leave 40. The
	// settlement then clamps against the derived maximum of 200, so the visible
	// number is the wound's result and the double application is detectable.
	wound := []ActorStateOp{{
		Op: "inc", ActorID: "protagonist", FieldID: xiuxianFieldHealth,
		Value: float64(-30), SourceKind: StateOpSourceRuleResolution,
	}}
	ops := settleXiuxianTurnState(state, system, nil, wound, "ev1")
	settled := appliedXiuxianState(t, state, append(wound, ops...))
	health, _ := actorStateNumber(actorStateFieldValue(settled, "protagonist", xiuxianFieldHealth))
	if health != 70 {
		t.Fatalf("a rule operation must reach the projection exactly once: health %v, want 70", health)
	}
}

func TestXiuxianTurnSettlementDoesNotMutateTheSnapshot(t *testing.T) {
	system := xiuxianTestSystem()
	// The projection is a copy. A settlement that wrote through to its input would
	// corrupt the caller's state, which the commit path keeps using afterwards.
	state := xiuxianTestState(t, cultivation.StageKey(cultivation.Qi(6)), nil)
	before, _ := actorStateNumber(actorStateFieldValue(state, "protagonist", xiuxianFieldHealthMax))
	settleXiuxianTurnState(state, system, nil, nil, "ev1")
	after, _ := actorStateNumber(actorStateFieldValue(state, "protagonist", xiuxianFieldHealthMax))
	if before != after {
		t.Fatalf("the settlement must not write through to its input: %v became %v", before, after)
	}
}
