package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover the join between the character's real carried stock and the
// battle rules: 《自制-装备与物品品级》 §4.2's dose budgets, §5's single-pass
// re-sum, §7's definition-versus-instance separation and §10's 背包法器不全加攻.
//
// Before this wiring, a pill could be registered, priced and held and still never
// be taken during a fight: nothing read 重要物品 into the battle, and nothing
// wrote a spent dose back.

// combatTestStoryCarrying builds a story whose protagonist already holds the
// given carried record, seeded through the initial actor.
//
// The fields come from the real preset's protagonist template rather than a
// hand-assembled list, because 重要物品 is declared among the actor's owned
// records and a list built from the panel and state fields alone would leave the
// carried record outside the schema — and therefore outside the state.
//
// Seeding through the initial actor rather than through a turn's state updates
// follows the same reasoning the starting realm does: an actor's first
// materialisation is the one place a story fact arrives without a settlement
// having produced it.
func combatTestStoryCarrying(t *testing.T, carried map[string]any, equipment map[string]any) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	fields := protagonistStateTemplate(cultivation.ReferenceRules(),
		spec).Fields
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: fields},
		{ID: "rival", Name: "对手", Fields: append([]ActorStateField{}, fields...)},
	}}
	hero := map[string]any{xiuxianFieldRealm: stage}
	if len(carried) > 0 {
		hero[xiuxianFieldImportantItems] = carried
	}
	for slot, entry := range equipment {
		hero[slot] = entry
	}
	system.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: hero},
		{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{xiuxianFieldRealm: stage}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// carriedDose finds one dose in a combatant's stock by its instance key.
func carriedDose(t *testing.T, session cultivation.CombatSession, actorID, instanceID string) cultivation.CarriedDose {
	t.Helper()
	combatant := session.CombatantByID(actorID)
	if combatant == nil {
		t.Fatalf("no combatant %q in the session", actorID)
	}
	for _, dose := range combatant.Carried {
		if dose.InstanceID == instanceID {
			return dose
		}
	}
	t.Fatalf("combatant %q does not carry %q: %+v", actorID, instanceID, combatant.Carried)
	return cultivation.CarriedDose{}
}

// openCarryingBattle builds the battle from the story's own stored numbers.
func openCarryingBattle(t *testing.T, store *Store, storyID string) cultivation.CombatSession {
	t.Helper()
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	session, _, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-open", combatTestSpecs(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// §7: the definition is a registered entry and the copy is real stock. A pill
// written under 重要物品 becomes a dose the battle can actually spend.
func TestAHeldPillBecomesAUsableDose(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
	}, nil)

	session := openCarryingBattle(t, store, storyID)
	dose := carriedDose(t, session, DefaultActorID, "回春丹")
	if dose.Quantity != 2 {
		t.Errorf("quantity = %d, want 2", dose.Quantity)
	}
	if dose.DefID != "pill.restore.health.lower" {
		t.Errorf("def = %q, want the registered pill", dose.DefID)
	}
	// The numbers come from the catalogue, not from the record: §4.2 fixes a 下品
	// restoring pill at 10% of its registered stage's bare pool.
	if dose.Pill.Purpose != cultivation.PillRestoreHealth {
		t.Errorf("purpose = %q, want a restoring pill", dose.Pill.Purpose)
	}
}

// A dose with no recorded count is one copy. Reading it as zero would be the
// rules inventing an absence out of an omission.
func TestAPillWithNoRecordedCountIsOneDose(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower"},
	}, nil)

	session := openCarryingBattle(t, store, storyID)
	if got := carriedDose(t, session, DefaultActorID, "回春丹").Quantity; got != 1 {
		t.Errorf("quantity = %d, want 1", got)
	}
}

// §10: 背包法器不全加攻. An implement recorded under 重要物品 is carried stock, not
// equipped gear, so it must not become a dose and must not move the attributes.
func TestACarriedImplementIsNotAUsableDoseAndAddsNoAttack(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"备用剑": map[string]any{"定义": "implement.top.unique", "数量": float64(1)},
	}, nil)

	session := openCarryingBattle(t, store, storyID)
	combatant := session.CombatantByID(DefaultActorID)
	if len(combatant.Carried) != 0 {
		t.Errorf("a carried implement became stock the rules can spend: %+v", combatant.Carried)
	}
	// The bare qi-refining layer 6 attack is 40; a 顶品独特 implement is 35%, which
	// would be 54 if a carried copy counted.
	if combatant.Attributes.Attack != 40 {
		t.Errorf("attack = %d, want the bare 40", combatant.Attributes.Attack)
	}
}

// §7: an entry naming a definition the catalogue does not hold has no published
// numbers, so it is a narrative fact rather than stock the rules can spend.
func TestAPillNamingAnUnregisteredDefinitionIsNotStock(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"伪造丹": map[string]any{"定义": "pill.restore.health.made_up", "数量": float64(3)},
	}, nil)

	session := openCarryingBattle(t, store, storyID)
	if got := session.CombatantByID(DefaultActorID).Carried; len(got) != 0 {
		t.Errorf("an unregistered definition became stock: %+v", got)
	}
}

// A dose taken in a real round is spent in the session, and the spend is written
// back to the character's own carried record in the same commit.
func TestUsingADoseInARoundSpendsItAndWritesItBack(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)},
	}, nil)
	session := openCarryingBattle(t, store, storyID)
	// Take a real wound first, so the dose has something to restore and is not
	// refused for being used on a full pool.
	hero := session.CombatantByID(DefaultActorID)
	hero.Attributes.Health = hero.Attributes.HealthMax / 2

	next, outcome := playDoseRound(t, store, storyID, session, "cmd-dose", "回春丹")
	if !findCombatStep(t, outcome, DefaultActorID).ItemApplied {
		t.Fatalf("the dose did not take effect: %+v", outcome)
	}
	if got := carriedDose(t, next, DefaultActorID, "回春丹").Quantity; got != 1 {
		t.Fatalf("stock after one dose = %d, want 1", got)
	}
	if got := savedCarriedQuantity(t, store, storyID, DefaultActorID, "回春丹"); got != 1 {
		t.Errorf("saved quantity = %v, want 1", got)
	}
}

// §4.2: 当前库存、消耗一个剂量与主要行动成本都真实保存. A retried round returns the
// committed exchange rather than resolving again, so a repeated click cannot eat
// a second dose.
func TestARetriedDoseRoundDoesNotSpendASecondDose(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, nil)
	session := openCarryingBattle(t, store, storyID)
	session.CombatantByID(DefaultActorID).Attributes.Health = 100

	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	next, outcome, err := ResolveCombatExchange(state, system, "cbt-1", "cmd-dose", combatTestSpecs(), &session,
		map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: "回春丹"},
		})
	if err != nil {
		t.Fatal(err)
	}
	request := AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: next, Outcome: outcome,
		CommandID: "cmd-dose", CommandHash: "hash-dose",
	}
	if _, err := store.AppendCombatRecord(storyID, request); err != nil {
		t.Fatal(err)
	}
	// The retry replays the identical request, as a lost response or a repeated
	// click would.
	if _, err := store.AppendCombatRecord(storyID, request); err != nil {
		t.Fatal(err)
	}

	if got := savedCarriedQuantity(t, store, storyID, DefaultActorID, "回春丹"); got != 2 {
		t.Errorf("saved quantity = %v after a retry, want 2 (one dose, spent once)", got)
	}
}

// §5: 所有实际属性从阶段基础值与当前有效来源重新汇总. A dose taken while an implement
// is equipped must aggregate with it in ONE pass. If the battle had not carried
// the character's own modifiers, the recompute would drop the implement's bonus
// entirely and the attack would fall back to the bare stage number.
func TestADoseKeepsTheEquippedImplementsBonus(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t,
		map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(2)}},
		// 中品稀有 with the whole rarity budget on the main attribute is 15%: at
		// qi-refining layer 6 the bare attack of 40 becomes 46.
		map[string]any{xiuxianFieldImplement: map[string]any{
			"名称": "青锋剑", "品级": "中品", "稀有度": "稀有", "主要属性预算": float64(5),
		}})
	session := openCarryingBattle(t, store, storyID)
	hero := session.CombatantByID(DefaultActorID)
	if hero.Attributes.Attack != 46 {
		t.Fatalf("attack with the implement = %d, want 46", hero.Attributes.Attack)
	}
	hero.Attributes.Health = 100

	next, outcome := playDoseRound(t, store, storyID, session, "cmd-dose", "回春丹")
	if !findCombatStep(t, outcome, DefaultActorID).ItemApplied {
		t.Fatal("the dose did not take effect")
	}
	if got := next.CombatantByID(DefaultActorID).Attributes.Attack; got != 46 {
		t.Errorf("attack after a dose = %d, want the implement's 46 preserved", got)
	}
}

// §8: 非法参数、欠缺能力、目标或数量未通过时不扣资源. A refused use is not written
// back, so a dose the rules would not serve stays in the character's record.
func TestARefusedDoseIsNotWrittenBack(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
	}, nil)
	session := openCarryingBattle(t, store, storyID)
	// A full pool refuses the dose, which is §8's ineffective-use case.
	hero := session.CombatantByID(DefaultActorID)
	hero.Attributes.Health = hero.Attributes.HealthMax

	_, outcome := playDoseRound(t, store, storyID, session, "cmd-dose", "回春丹")
	if findCombatStep(t, outcome, DefaultActorID).ItemApplied {
		t.Fatal("a dose was applied to a full pool")
	}
	if got := savedCarriedQuantity(t, store, storyID, DefaultActorID, "回春丹"); got != 1 {
		t.Errorf("saved quantity = %v after a refusal, want the untouched 1", got)
	}
}

// The record's key is the instance identity, so two copies of one definition are
// two separate stocks rather than one merged count. §7 requires that a
// definition's existence never be read as a single pooled inventory.
func TestTwoCopiesOfOnePillAreSeparateStocks(t *testing.T) {
	store, storyID := combatTestStoryCarrying(t, map[string]any{
		"回春丹甲": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(1)},
		"回春丹乙": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, nil)
	session := openCarryingBattle(t, store, storyID)
	if got := session.CombatantByID(DefaultActorID).Carried; len(got) != 2 {
		t.Fatalf("stock = %+v, want two separate copies", got)
	}
	session.CombatantByID(DefaultActorID).Attributes.Health = 100

	next, outcome := playDoseRound(t, store, storyID, session, "cmd-dose", "回春丹乙")
	if !findCombatStep(t, outcome, DefaultActorID).ItemApplied {
		t.Fatal("the dose did not take effect")
	}
	if got := carriedDose(t, next, DefaultActorID, "回春丹甲").Quantity; got != 1 {
		t.Errorf("the untouched copy = %d, want 1", got)
	}
	if got := carriedDose(t, next, DefaultActorID, "回春丹乙").Quantity; got != 2 {
		t.Errorf("the spent copy = %d, want 2", got)
	}
}

// TestTheXiuXianPresetNamesTheDefinitionsItCanPrice covers §7's "应用层装配实际
// 已知物品供AI选择" at the schema: the model is told which definitions exist.
//
// Without this, the only identifier a story could write under 定义 is one the
// catalogue does not hold, and xiuxianCarriedDoses skips exactly those — so a
// dose could never be carried into a real fight however well the battle side
// worked. The check is against the catalogue rather than a written-out list, so
// it cannot pass while the prompt names a set that no longer exists.
func TestTheXiuXianPresetNamesTheDefinitionsItCanPrice(t *testing.T) {
	instruction := xiuxianImportantItemsInstruction()
	if instruction == "" {
		t.Fatal("the xiuxian preset must carry an item record instruction")
	}
	for _, definition := range storyItemCatalog(cultivation.ReferenceRules()).Definitions() {
		if !strings.Contains(instruction, definition.ID) {
			t.Errorf("the model is not told about %q, so it cannot record a dose the rules can price", definition.ID)
		}
	}
	// §10: a pack of implements must not read as a source of attack. The guidance
	// says which kind is spendable rather than leaving the model to assume.
	if !strings.Contains(instruction, "丹药与符箓会成为战斗中真正可消耗的一份") {
		t.Fatalf("the guidance must say only a pill is a dose: %s", instruction)
	}
	// The grade and rarity wording has to be the same words the reader accepts, or
	// the model writes a definition the reader then fails to price.
	for _, wording := range []string{"下品", "中品", "上品", "顶品", "常见", "稀有", "独特"} {
		if !strings.Contains(instruction, wording) {
			t.Errorf("the guidance never uses the published wording %q", wording)
		}
	}
}

// TestOnlyTheXiuXianPresetNamesItemDefinitions keeps the catalogue out of the
// presets that have none: a western-fantasy story told to record
// pill.restore.health.lower would be handed a definition its own rules never
// registered.
func TestOnlyTheXiuXianPresetNamesItemDefinitions(t *testing.T) {
	for _, spec := range []actorStatePresetSpec{
		defaultActorStatePresetSpec(), westernFantasyActorStatePresetSpec(),
		apocalypseActorStatePresetSpec(), infiniteFlowActorStatePresetSpec(),
	} {
		for _, field := range actorOwnedRecordFields(cultivation.ReferenceRules(),
			spec, 6, 6, 6, "") {
			if field.Name == "重要物品" && strings.Contains(field.UpdateInstruction, "pill.restore.health.lower") {
				t.Errorf("preset %q must not name the cultivation catalogue's definitions", spec.ID)
			}
		}
	}
}

// xiuxianImportantItemsInstruction returns the 重要物品 instruction the real
// protagonist template publishes, which is the text the model is actually sent.
func xiuxianImportantItemsInstruction() string {
	for _, field := range protagonistStateTemplate(cultivation.ReferenceRules(),
		xiuxianActorStatePresetSpec()).Fields {
		if field.Name == "重要物品" {
			return field.UpdateInstruction
		}
	}
	return ""
}

// playDoseRound resolves one round in which the protagonist takes the named dose,
// commits it, and returns the settled session and outcome.
func playDoseRound(
	t *testing.T, store *Store, storyID string,
	session cultivation.CombatSession, commandID, instanceID string,
) (cultivation.CombatSession, cultivation.ExchangeOutcome) {
	t.Helper()
	state := combatTestState(t, store, storyID)
	system := combatTestSystemFromStore(t, store, storyID)
	next, outcome, err := ResolveCombatExchange(state, system, "cbt-1", commandID, combatTestSpecs(), &session,
		map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: instanceID},
		})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Session: next, Outcome: outcome,
		CommandID: commandID, CommandHash: "hash-" + commandID,
	}); err != nil {
		t.Fatal(err)
	}
	return next, outcome
}

// savedCarriedQuantity reads the committed count of one carried copy.
func savedCarriedQuantity(t *testing.T, store *Store, storyID, actorID, key string) float64 {
	t.Helper()
	state := combatTestState(t, store, storyID)
	record, ok := xiuxianEquipmentEntry(state, actorID, xiuxianFieldImportantItems)
	if !ok {
		t.Fatalf("the carried record is missing: %+v", record)
	}
	entry, ok := record[key].(map[string]any)
	if !ok {
		t.Fatalf("the carried record lost %q: %+v", key, record)
	}
	quantity, _ := actorStateNumber(entry["数量"])
	return quantity
}

// findCombatStep locates one actor's step in a settled exchange.
func findCombatStep(t *testing.T, outcome cultivation.ExchangeOutcome, actorID string) cultivation.ExchangeStep {
	t.Helper()
	for _, step := range outcome.Steps {
		if step.ActorID == actorID {
			return step
		}
	}
	t.Fatalf("no step for %q in %+v", actorID, outcome.Steps)
	return cultivation.ExchangeStep{}
}
