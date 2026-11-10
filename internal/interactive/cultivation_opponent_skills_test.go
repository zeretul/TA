package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
	interactivestate "ta/internal/interactive/state"
)

// A person who arrives already knowing a registered technique has to reach the
// battle with it. The regression this pins is the one that made a regional boss
// fight a whole encounter with plain attacks: the model filed 冲脉秘剑 under the
// free-form 技能与能力 record, which nothing prices, and left 已学功法 empty. The
// create path always accepted the learned record — the guidance pointed away
// from it — so this test covers the engine half, and the field instructions are
// covered by TestAnOpponentsTechniqueIsDirectedToTheLearnedRecord below.
func TestAnOpponentCanArriveAlreadyKnowingATechnique(t *testing.T) {
	system := xiuxianTestSystem()
	state := map[string]any{"actors": map[string]any{
		DefaultActorID: map[string]any{
			"id": DefaultActorID, "template_id": DefaultActorID,
			"state": map[string]any{xiuxianFieldRealm: cultivation.StageKey(cultivation.Qi(6))},
		},
		DefaultStoryContextActorID: map[string]any{
			"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
			"state": map[string]any{},
		},
	}}

	updates := []interactivestate.Update{{
		Op: "create", Path: "/断道客",
		Value: map[string]any{
			"template_id": ActorStateOpponentTemplateID,
			"name":        "断道客",
			"role":        "劫修",
			"state": map[string]any{
				xiuxianFieldRealm: "qi_5",
				xiuxianFieldLearnedSkills: map[string]any{
					"冲脉秘剑": map[string]any{"定义": "meridian_secret_blade", "版本": 1, "掌握程度": "熟练"},
				},
				xiuxianFieldImplement: map[string]any{"名称": "断水刀"},
				xiuxianFieldImportantItems: map[string]any{
					"断水刀": map[string]any{"定义": "implement.middle.rare", "数量": 1},
				},
			},
		},
	}}
	compiled, err := CompileTurnStateUpdates(system, state, updates, TurnStateUpdateCompileOptions{SourceTurnID: "ev1"})
	if err != nil {
		t.Fatalf("a new opponent may arrive already knowing a registered technique: %v", err)
	}
	for _, op := range compiled.Ops {
		applyStateOp(state, op)
	}
	for _, op := range compiled.ActorOps {
		applyActorStateOp(state, op)
	}

	// The record has to resolve back through the same reader the battle uses,
	// or the technique exists only as a string in the save.
	learned := learnedSkillsFromState(state, "断道客", storySkillRegistry(cultivation.ReferenceRules()))
	if !learned.Knows("meridian_secret_blade") {
		t.Fatal("the created record must resolve back to the learned technique")
	}

	enemy, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), "断道客", CombatantSpec{Side: cultivation.SideEnemy})
	if err != nil {
		t.Fatal(err)
	}
	// The blade is what 冲脉秘剑 requires, and it is in hand because the create
	// said so. Without this the technique would be learned but never castable.
	if !enemy.HasImplement {
		t.Fatal("the implement written at create must reach the battle")
	}
	if len(enemy.Skills) != 1 || enemy.Skills[0].ID != "meridian_secret_blade" {
		t.Fatalf("the battle must see the learned technique: %+v", enemy.Skills)
	}

	// A tactical decision needs a real opposing target, not a one-person
	// encounter. The learned blade remains a useful paid preparation here.
	player, err := combatantFromState(cultivation.ReferenceRules(),
		state, storySkillRegistry(cultivation.ReferenceRules()), DefaultActorID, CombatantSpec{Side: cultivation.SidePlayer})
	if err != nil {
		t.Fatal(err)
	}
	action := cultivation.DecideEnemyAction(cultivation.CombatSession{
		ID: "probe", Combatants: []cultivation.Combatant{enemy, player}, Round: 1,
	}, "断道客", cultivation.SkillDef{ID: "strike", Power: 1.0, Targets: 1})
	if action.Kind != cultivation.ActionTechnique || action.Skill.ID != "meridian_secret_blade" {
		t.Fatalf("the enemy's own technique must be chosen over the fallback: %+v", action)
	}
	if action.Skill.ExecutionActions != 2 {
		t.Fatalf("the secret blade must keep its published two-action preparation: %d", action.Skill.ExecutionActions)
	}
}

// TestAnOpponentsTechniqueIsDirectedToTheLearnedRecord is the guidance half.
//
// The boss fought a whole encounter with plain attacks because the field the
// model was told to use was the free-form one. A technique written there is
// prose: nothing prices it and the battle never sees it. These two instructions
// are what the model reads when it creates an opponent, so they have to name the
// rules-owned record.
func TestAnOpponentsTechniqueIsDirectedToTheLearnedRecord(t *testing.T) {
	spec := xiuxianActorStatePresetSpec()
	byName := map[string]ActorStateField{}
	for _, field := range spec.OpponentFields {
		byName[field.Name] = field
	}
	threat, ok := byName["本相与传承特征"]
	if !ok {
		t.Fatal("the opponent template must declare 本相与传承特征")
	}
	for _, want := range []string{xiuxianFieldLearnedSkills, "create"} {
		if !strings.Contains(threat.Description, want) {
			t.Errorf("本相与传承特征 must send techniques to %q, got %q", want, threat.Description)
		}
	}

	// The learned record itself has to say that a first appearance is exempt
	// from the learning settlement, or a model that reads it will conclude a
	// new opponent cannot be given the technique it obviously knows.
	if !strings.Contains(xiuxianLearnedSkillsField().UpdateInstruction, "create") {
		t.Error("已学功法 must say a new Actor's existing techniques are written at create")
	}
	if !strings.Contains(xiuxianImplementField().UpdateInstruction, "create") {
		t.Error("主法器 must say a new Actor's held implement is written at create")
	}
}
