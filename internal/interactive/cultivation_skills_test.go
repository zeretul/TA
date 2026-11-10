package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// xiuxianSkillTestSystem is the settlement fixture plus the learned-technique
// record and the story's own definition registry, because the learning rules
// write into fields the panel fixture does not declare.
func xiuxianSkillTestSystem() StoryDirectorActorStateSystem {
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianLearnedSkillsField())
	return StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Name: "主角", Fields: fields,
	}}}
}

// A template registration is a story fact, so the fixture needs the story-context
// actor the record is written to.
func xiuxianSkillTestState(t *testing.T, stage string, extra map[string]any) map[string]any {
	t.Helper()
	if extra == nil {
		extra = map[string]any{}
	}
	state := xiuxianTestState(t, stage, extra)
	state[actorStateRoot].(map[string]any)[DefaultStoryContextActorID] = map[string]any{
		"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
		"state": map[string]any{},
	}
	return state
}

func learnableStage(t *testing.T) string {
	t.Helper()
	// A technique whose CanCast gate is the earliest realm is learnable by any
	// cultivator, which keeps these tests about the learning path rather than
	// about which realm a particular technique happens to require.
	definition, ok := storySkillRegistry(cultivation.ReferenceRules()).Current("fire_bolt")
	if !ok {
		t.Fatal("the rules' published table must contain the basic strike")
	}
	if !definition.CanCast(cultivation.Qi(1)) {
		t.Skip("the basic strike is not castable at the earliest layer; the fixture needs a different definition")
	}
	return cultivation.StageKey(cultivation.Qi(1))
}

// The design's order is validate, register, grant. The check here is that a
// character who learns a technique ends up with a record the rules can resolve
// back to the registered definition at the registered version — not with a copy
// of the numbers the model supplied.
func TestXiuxianLearningGrantsARegisteredDefinition(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "fire_bolt", Source: "完整火弹篇", Minutes: 1440}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared field must learn")
	}
	if len(result.Learned) != 1 || result.Learned[0].ID != "fire_bolt" {
		t.Fatalf("learned = %+v, want the strike definition", result.Learned)
	}
	if len(result.Refused) != 0 {
		t.Fatalf("a registered definition must not be refused: %+v", result.Refused)
	}
	if len(ops) != 1 || ops[0].SourceKind != StateOpSourceSkillLearning {
		t.Fatalf("ops = %+v, want one learning-sourced write", ops)
	}

	// The record must round-trip through the reader the cast path uses, which is
	// the property that makes the record a rules fact rather than prose.
	applied := appliedXiuxianState(t, state, ops)
	learned := learnedSkillsFromState(applied, "protagonist", storySkillRegistry(cultivation.ReferenceRules()))
	if !learned.Knows("fire_bolt") {
		t.Fatal("the written record must resolve back to the learned technique")
	}
	resolved, ok := learned.Known(storySkillRegistry(cultivation.ReferenceRules()), "fire_bolt")
	if !ok {
		t.Fatal("the learned technique must resolve to a registered definition")
	}
	if resolved.Version != result.Learned[0].Version {
		t.Fatalf("resolved version = %d, want the granted version %d", resolved.Version, result.Learned[0].Version)
	}
	// And the record must not carry the definition's numbers: editing the record
	// must not be able to inflate a technique. 描述 and 来历 are journal wording,
	// not numbers, and nothing prices them.
	entry, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldLearnedSkills)
	record, _ := entry["fire_bolt"].(map[string]any)
	for key := range record {
		switch key {
		case "定义", "名称", "版本", "掌握程度", "描述", "来历", "已修习分钟":
		default:
			t.Fatalf("the learned record carries %q; the numbers belong to the definition, not the record", key)
		}
	}
}

// TestXiuxianLearningRecordsTheArtsOwnWording is the journal half of §5: the
// learned entry carries what the art looks like and where it came from, and both
// are written once rather than restyled on a later turn.
func TestXiuxianLearningRecordsTheArtsOwnWording(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		DefID:       "fire_bolt",
		Description: "起手极平，收势也平，唯一的讲究是手腕比眼睛快半拍。",
		Source:      "山门外那位守碑的老卒教的，他说反正也没别人学。", Minutes: 1440,
	}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok || len(result.Learned) != 1 {
		t.Fatalf("learned = %+v ok=%v", result.Learned, ok)
	}
	applied := appliedXiuxianState(t, state, ops)
	entry, _ := xiuxianEquipmentEntry(applied, "protagonist", xiuxianFieldLearnedSkills)
	record, _ := entry["fire_bolt"].(map[string]any)
	if got := xiuxianEntryText(record, "描述"); !strings.Contains(got, "手腕") {
		t.Errorf("recorded description = %q, want the sentence the learning supplied", got)
	}
	if got := xiuxianEntryText(record, "来历"); !strings.Contains(got, "老卒") {
		t.Errorf("recorded source = %q, want the sentence the learning supplied", got)
	}

	// Learning it again does not restyle it: the player already read those two
	// sentences.
	again := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		DefID: "fire_bolt", Description: "一道白光。", Source: "天上掉下来的。",
	}}}
	secondOps, _, _ := xiuxianSkillLearningOps(applied, system, "protagonist", again, "ev2")
	second := appliedXiuxianState(t, applied, secondOps)
	secondEntry, _ := xiuxianEquipmentEntry(second, "protagonist", xiuxianFieldLearnedSkills)
	secondRecord, _ := secondEntry["fire_bolt"].(map[string]any)
	if got := xiuxianEntryText(secondRecord, "描述"); !strings.Contains(got, "手腕") {
		t.Errorf("description after re-learning = %q, want the first wording kept", got)
	}
	if got := xiuxianEntryText(secondRecord, "来历"); !strings.Contains(got, "老卒") {
		t.Errorf("source after re-learning = %q, want the first wording kept", got)
	}
}

// An unknown definition grants nothing and says why. The refusal is the design's
// requirement that a player who was refused mastery is told the real cause rather
// than only that nothing happened.
func TestXiuxianLearningRefusesAnUnregisteredDefinition(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "not_a_real_technique"}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on even when every entry is refused")
	}
	if len(ops) != 0 {
		t.Fatalf("a refused definition must write nothing, wrote %+v", ops)
	}
	if len(result.Learned) != 0 {
		t.Fatalf("learned = %+v, want nothing", result.Learned)
	}
	if len(result.Refused) != 1 || result.Refused[0].Reason == "" {
		t.Fatalf("a refusal must be reported with a reason: %+v", result.Refused)
	}
}

// A version the registry does not hold is refused rather than silently replaced
// with the current definition. §6 forbids rewriting what a character learns.
func TestXiuxianLearningRefusesAnUnregisteredVersion(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "fire_bolt", Version: 999}}}

	_, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on")
	}
	if len(result.Learned) != 0 {
		t.Fatalf("an unregistered version must grant nothing, granted %+v", result.Learned)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one entry", result.Refused)
	}
}

// A mortal may not be handed a technique by the narrative. The check reads the
// stored stage, so a character who has not become a cultivator cannot learn.
func TestXiuxianLearningRefusesAMortal(t *testing.T) {
	system := xiuxianSkillTestSystem()
	// No stored stage key at all: the character has not entered the rules' stage
	// table, which is what being a mortal means here.
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist", "state": map[string]any{},
	}}}
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "fire_bolt"}}}

	_, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on")
	}
	if len(result.Learned) != 0 {
		t.Fatalf("a character with no stage must learn nothing, learned %+v", result.Learned)
	}
	if len(result.Refused) != 1 {
		t.Fatalf("refused = %+v, want one entry", result.Refused)
	}
}

// A second learning of the same technique at the same version must not duplicate
// the record, so a replayed commit is idempotent.
func TestXiuxianLearningIsIdempotent(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "fire_bolt"}}}

	first, _, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("a tracked character must learn")
	}
	applied := appliedXiuxianState(t, state, first)

	second, result, ok := xiuxianSkillLearningOps(applied, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("re-learning must still be acted on")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("re-learning the same version is a no-op, not a refusal: %+v", result.Refused)
	}
	settled := appliedXiuxianState(t, applied, second)
	entry, _ := xiuxianEquipmentEntry(settled, "protagonist", xiuxianFieldLearnedSkills)
	count := 0
	for key := range entry {
		if key == "fire_bolt" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the record holds %d strike entries, want 1", count)
	}
}

// A genre that is not the cultivation system must not gain the field, and the
// learning rules must decline rather than write into a schema that never
// declared it.
func TestXiuxianLearningDeclinesOutsideTheCultivationSystem(t *testing.T) {
	other := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Name: "主角",
		Fields: []ActorStateField{{ID: "当前境界", Name: "当前境界", Type: "enum", Options: []string{"一阶", "二阶"}}},
	}}}
	state := map[string]any{"actors": map[string]any{"protagonist": map[string]any{
		"id": "protagonist", "template_id": "protagonist",
		"state": map[string]any{"当前境界": "一阶"},
	}}}
	if _, _, ok := xiuxianSkillLearningOps(state, other, "protagonist", TurnSkillLearningInput{
		Skills: []TurnSkillLearning{{DefID: "fire_bolt"}},
	}, "ev1"); ok {
		t.Fatal("a non-cultivation genre must not have techniques learned into it")
	}
}

// The learning record is rules-owned, so the model must not be able to author it.
func TestXiuxianLearnedSkillsAreRefusedToTheModel(t *testing.T) {
	system := xiuxianSkillTestSystem()
	if !IsXiuxianSettledField(system, xiuxianFieldLearnedSkills) {
		t.Fatal("已学功法 is written by the learning rules and must be reported as owned")
	}
}

// A template-built technique copies every number from the template, and the
// story's own record is what makes it exist for the next read.
func TestXiuxianLearningGeneratesATechniqueFromATemplate(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		TemplateID: "fire_bolt", Name: "赤霞弹", Source: "《赤霞诀》完整火弹篇", Minutes: 1440,
	}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("a tracked character with the declared field must learn")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("an approved template must not be refused: %+v", result.Refused)
	}
	if len(result.Learned) != 1 {
		t.Fatalf("learned = %+v, want one technique", result.Learned)
	}
	generated := result.Learned[0]
	template, ok := cultivation.SkillTemplateByID("fire_bolt")
	if !ok {
		t.Fatal("fire_bolt must be an approved template")
	}
	if generated.ID != cultivation.GeneratedSkillID("fire_bolt", "赤霞弹") {
		t.Fatalf("id = %q, want the derived identity", generated.ID)
	}
	if generated.Name != "赤霞弹" || generated.Version != 1 {
		t.Fatalf("generated = %+v, want the name and first version", generated)
	}
	// The numbers must be the template's, untouched.
	if generated.Power != template.Definition.Power ||
		generated.ManaFraction != template.Definition.ManaFraction ||
		generated.Burden != template.Definition.Burden ||
		generated.Targets != template.Definition.Targets ||
		generated.VariantBase != template.Definition.VariantBase {
		t.Fatalf("generated = %+v, want every number from %+v", generated, template.Definition)
	}

	// Definitions come before the learned entry that references them.
	if len(ops) != 2 || ops[0].FieldID != normalizeActorStateFieldName(xiuxianFieldSkillRegistry) {
		t.Fatalf("ops = %+v, want the definition written first", ops)
	}
	applied := appliedXiuxianState(t, state, ops)
	registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
		applied)
	resolved, found := registry.Version(generated.ID, 1)
	if !found {
		t.Fatal("the story's registry must hold the generated definition")
	}
	if resolved.Name != "赤霞弹" || resolved.Power != template.Definition.Power {
		t.Fatalf("resolved = %+v, want the generated definition", resolved)
	}
	learned := learnedSkillsFromState(applied, "protagonist", registry)
	if !learned.Knows(generated.ID) {
		t.Fatal("the character must know the technique they were granted")
	}

	// The record must not carry numbers: the definition is the only source.
	entry, _ := xiuxianEquipmentEntry(applied, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	record, _ := entry[generated.ID].(map[string]any)
	for key := range record {
		switch key {
		case "定义", "版本", "模板", "名称", "品级", "稀有度", "来源":
		default:
			t.Fatalf("the definition record carries %q; numbers belong to the template", key)
		}
	}
	if record["品级"] != "普通" || record["稀有度"] != "常见" {
		t.Fatalf("grade = %v/%v, want the template's published 普通／常见", record["品级"], record["稀有度"])
	}
}

// §6: "技能品级与稀有度须匹配该模板许可，不由模型改标签后自动增加预算." A mismatched
// pair is refused and nothing at all is written — not the definition, not the
// learned entry.
func TestXiuxianLearningRefusesAGradeTheTemplateDoesNotPublish(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		TemplateID: "fire_bolt", Name: "赤霞弹", Quality: "高阶", Rarity: "珍稀",
	}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on")
	}
	if len(ops) != 0 {
		t.Fatalf("a refused grade must write nothing, wrote %+v", ops)
	}
	if len(result.Learned) != 0 {
		t.Fatalf("learned = %+v, want nothing", result.Learned)
	}
	if len(result.Refused) != 1 || result.Refused[0].Reason == "" {
		t.Fatalf("refused = %+v, want one refusal naming the cause", result.Refused)
	}
}

// A capability with no approved template is not a technique yet. §7.2 requires
// the refusal to say so plainly rather than dressing it as a character's failure.
func TestXiuxianLearningRefusesAnUnsupportedTemplate(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		TemplateID: "time_stop", Name: "时停",
	}}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on")
	}
	if len(ops) != 0 || len(result.Learned) != 0 {
		t.Fatalf("an unsupported ability must grant nothing: ops=%+v learned=%+v", ops, result.Learned)
	}
	if len(result.Refused) != 1 || !strings.Contains(result.Refused[0].Reason, "已批准模板") {
		t.Fatalf("refused = %+v, want the unsupported-ability wording", result.Refused)
	}
}

// Two techniques generated in one turn must both be registered. A registry write
// built from the pre-turn state would silently drop the first.
func TestXiuxianLearningRegistersTwoGeneratedTechniquesInOneTurn(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{
		{TemplateID: "fire_bolt", Name: "赤霞弹", Source: "完整火弹篇", Minutes: 1440},
		{TemplateID: "shield", Name: "玄龟罩", Source: "完整护罩篇", Minutes: 1440},
	}}

	ops, result, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("the module must be acted on")
	}
	if len(result.Learned) != 2 || len(result.Refused) != 0 {
		t.Fatalf("learned = %+v refused = %+v, want both granted", result.Learned, result.Refused)
	}
	applied := appliedXiuxianState(t, state, ops)
	registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
		applied)
	for _, definition := range result.Learned {
		if _, found := registry.Version(definition.ID, definition.Version); !found {
			t.Fatalf("%s was granted but the story's registry does not hold it", definition.ID)
		}
	}
}

// A replay of the same generation must not be refused as a non-advancing version:
// it resolves to what the story already registered.
func TestXiuxianLearningRegenerationResolvesToTheRegisteredDefinition(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	input := TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		TemplateID: "fire_bolt", Name: "赤霞弹",
	}}}

	first, _, ok := xiuxianSkillLearningOps(state, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("a tracked character must learn")
	}
	applied := appliedXiuxianState(t, state, first)

	second, result, ok := xiuxianSkillLearningOps(applied, system, "protagonist", input, "ev1")
	if !ok {
		t.Fatal("re-learning must still be acted on")
	}
	if len(result.Refused) != 0 {
		t.Fatalf("re-generating the same technique is a no-op, not a refusal: %+v", result.Refused)
	}
	for _, op := range second {
		if op.FieldID == normalizeActorStateFieldName(xiuxianFieldSkillRegistry) {
			t.Fatal("an already registered definition must not be rewritten")
		}
	}
}

// A record that no longer re-derives is skipped, so a hand-edited journal cannot
// smuggle an unvalidated technique into a cast.
func TestStorySkillRegistrySkipsAnUnreadableRecord(t *testing.T) {
	state := map[string]any{actorStateRoot: map[string]any{
		DefaultStoryContextActorID: map[string]any{
			"id": DefaultStoryContextActorID, "template_id": ActorStateStoryContextTemplateID,
			"state": map[string]any{xiuxianFieldSkillRegistry: map[string]any{
				"gen.fire_bolt.赤霞弹": map[string]any{
					"定义": "gen.fire_bolt.赤霞弹", "版本": float64(1),
					"模板": "no_such_template", "名称": "赤霞弹",
				},
				"gen.fire_bolt.坏名": map[string]any{
					"定义": "gen.fire_bolt.坏名", "版本": float64(1),
					"模板": "fire_bolt", "名称": "   ",
				},
			}},
		},
	}}
	registry := storySkillRegistryFromState(cultivation.ReferenceRules(),
		state)
	for _, id := range registry.IDs() {
		if cultivation.IsGeneratedSkillID(id) {
			t.Fatalf("an unreadable record produced the definition %q", id)
		}
	}
}

// The removal helper must drop a learning and nothing else, which is what lets
// the reroll path re-derive it without touching the narrative's writes.
func TestSkillLearningRemovalDropsOnlyItsOwnWrites(t *testing.T) {
	ops := []ActorStateOp{
		{Op: "set", ActorID: "protagonist", FieldID: xiuxianFieldHealth, Value: float64(1), SourceKind: StateOpSourceCultivationSettlement},
		{Op: "set", ActorID: "protagonist", FieldID: xiuxianFieldLearnedSkills, Value: map[string]any{}, SourceKind: StateOpSourceSkillLearning},
		{Op: "set", ActorID: "protagonist", FieldID: xiuxianFieldRealm, Value: "炼气一层", SourceKind: StateOpSourceCultivationAdvance},
	}
	out := removeSkillLearningActorOps(ops)
	if len(out) != 2 {
		t.Fatalf("removal left %d ops, want 2", len(out))
	}
	for _, op := range out {
		if op.SourceKind == StateOpSourceSkillLearning {
			t.Fatal("a learning write survived its own removal")
		}
	}
}
