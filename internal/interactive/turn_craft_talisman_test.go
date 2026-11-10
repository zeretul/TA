package interactive

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

func talismanStory(t *testing.T, methods ...cultivation.CraftMethodDef) (*Store, string, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	system := StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{protagonistStateTemplate(cultivation.ReferenceRules(),
			spec), storyContextStateTemplate(spec), {ID: "rival", Name: "对手", Fields: protagonistStateTemplate(cultivation.ReferenceRules(),
			spec).Fields}},
		InitialActors: []ActorStateInitialActor{
			{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: "qi_6", xiuxianFieldCurrency: float64(100)}},
			{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{xiuxianFieldRealm: "qi_6"}},
			{ID: DefaultStoryContextActorID, Name: "故事状态", TemplateID: ActorStateStoryContextTemplateID},
		},
	}
	if len(methods) > 0 {
		system.CraftMethods = append(cultivation.NewCraftRegistryWithBuiltins().Methods(), methods...)
	}
	root := t.TempDir()
	store := NewStore(root)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "制符", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	talismanTurn(t, store, story.ID, "main", "准备去符铺", &TurnResult{Choices: testTurnChoices()})
	return store, story.ID, root
}

func talismanTurn(t *testing.T, store *Store, storyID, branchID, user string, result *TurnResult) TurnEvent {
	t.Helper()
	result.Choices = testTurnChoices()
	var identity DomainCommitIdentity
	for cycle := 1; cycle < 400; cycle++ {
		identity = b1Identity("ward-"+user, cycle)
		if craftDraw(identity, "", "craft.talisman.ward") >= .10 {
			break
		}
	}
	if branchID == "main" {
		identity = b1CommitInput(t, store, storyID, identity, user)
	} else {
		intent, err := NewPlayerInputIntent(identity, branchID, user)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CommitPlayerInput(storyID, intent); err != nil {
			t.Fatal(err)
		}
	}
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: branchID, User: user, Narrative: "符铺里灯火明亮。", TurnResult: result,
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

func TestTalismanLearnBuyCraftUseTradeRestoreAndFork(t *testing.T) {
	store, storyID, root := talismanStory(t)
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	craft := &TurnCraftInput{MethodID: method.ID, DurationSeconds: 4 * 60 * 60}
	before, _, _ := store.CultivationState(storyID, "main")
	talismanTurn(t, store, storyID, "main", "未学试制", &TurnResult{Craft: craft})
	after, _, _ := store.CultivationState(storyID, "main")
	if before.Seconds != after.Seconds || len(f16StoredPack(t, store, storyID)) != 0 {
		t.Fatal("unlearned method charged time or produced inventory")
	}
	learnedTurn := talismanTurn(t, store, storyID, "main", "随符师学绘护身符", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: method.ID, Source: "符铺师傅当面传授"}}}})
	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !craftMethodKnown(snapshot.State, DefaultActorID, method) {
		t.Fatal("formal learning did not grant the method")
	}
	if learnedSkillsFromState(snapshot.State, DefaultActorID, storySkillRegistry(cultivation.ReferenceRules())).Knows(method.ID) {
		t.Fatal("craft method became a combat skill")
	}
	preCraft, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: learnedTurn.ID, Title: "只学不做"})
	if err != nil {
		t.Fatal(err)
	}
	craftedTurn := talismanTurn(t, store, storyID, "main", "买料制两张护身符", &TurnResult{
		Trade: &TurnTradeInput{Merchant: "符铺", Buys: []TurnTradeBuy{{DefID: "material.talisman.paper", Name: "符纸", Quantity: 2}, {DefID: "material.talisman.ink", Name: "灵墨", Quantity: 2}}}, Craft: craft,
	})
	pack := f16StoredPack(t, store, storyID)
	for name, want := range map[string]int{"符纸": 0, "灵墨": 0, "talisman.ward.lower": 2} {
		entry, _ := pack[name].(map[string]any)
		if entry == nil || xiuxianCarriedQuantity(entry) != want {
			t.Fatalf("%s inventory = %#v, want %d", name, entry, want)
		}
	}
	after, _, _ = store.CultivationState(storyID, "main")
	if after.Seconds-before.Seconds != craft.DurationSeconds {
		t.Fatalf("craft elapsed %d, want %d", after.Seconds-before.Seconds, craft.DurationSeconds)
	}
	branch, err := store.Snapshot(storyID, preCraft.ID)
	if err != nil {
		t.Fatal(err)
	}
	branchPack, _ := xiuxianEquipmentEntry(branch.State, DefaultActorID, xiuxianFieldImportantItems)
	if len(branchPack) != 0 || !craftMethodKnown(branch.State, DefaultActorID, method) {
		t.Fatal("pre-craft fork lost learning or gained future inventory")
	}

	craftFork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: craftedTurn.ID, Title: "留存护符"})
	if err != nil {
		t.Fatal(err)
	}
	// The first real combat item action consumes the crafted copy, then writes
	// both the shield and remaining inventory into the canonical Story journal.
	session := openCarryingBattle(t, store, storyID)
	next, outcome := playDoseRound(t, store, storyID, session, "ward-use", "talisman.ward.lower")
	if !findCombatStep(t, outcome, DefaultActorID).ItemApplied || next.CombatantByID(DefaultActorID).Shield.Amount <= 0 {
		t.Fatalf("crafted ward had no effect: %+v", outcome)
	}
	if savedCarriedQuantity(t, store, storyID, DefaultActorID, "talisman.ward.lower") != 1 {
		t.Fatal("use did not persist one-copy deduction")
	}
	// Re-appending the committed combat command returns its durable result.
	if _, err := store.AppendCombatRecord(storyID, AppendCombatRecordRequest{BranchID: "main", WorldTime: "第1日 辰时", Session: next, Outcome: outcome, CommandID: "ward-use", CommandHash: "hash-ward-use"}); err != nil {
		t.Fatal(err)
	}
	if savedCarriedQuantity(t, store, storyID, DefaultActorID, "talisman.ward.lower") != 1 {
		t.Fatal("retry consumed a second copy")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := NewStore(root)
	t.Cleanup(func() { _ = reopened.Close() })
	restored, err := reopened.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	restoredSession := restored["cbt-1"]
	if !reflect.DeepEqual(next.CombatantByID(DefaultActorID).Shield, restoredSession.CombatantByID(DefaultActorID).Shield) {
		t.Fatal("cold replay changed ward state")
	}
	if savedCarriedQuantity(t, reopened, storyID, DefaultActorID, "talisman.ward.lower") != 1 {
		t.Fatal("cold replay restored spent copy")
	}
	forkSnapshot, err := reopened.Snapshot(storyID, craftFork.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkPack, _ := xiuxianEquipmentEntry(forkSnapshot.State, DefaultActorID, xiuxianFieldImportantItems)
	forkWard, _ := forkPack["talisman.ward.lower"].(map[string]any)
	if xiuxianCarriedQuantity(forkWard) != 2 {
		t.Fatal("main consumption leaked into pre-battle fork")
	}
	// Trade the remaining copy through the existing economy, retaining its
	// fixed catalogue price and the merchant's already committed cash.
	state := combatTestState(t, reopened, storyID)
	purse := xiuxianStoredInt(state, DefaultActorID, xiuxianFieldCurrency)
	talismanTurn(t, reopened, storyID, "main", "出售剩余护身符", &TurnResult{Trade: &TurnTradeInput{Merchant: "符铺", Sells: []TurnTradeSell{{InstanceID: "talisman.ward.lower", Quantity: 1}}}})
	state = combatTestState(t, reopened, storyID)
	if xiuxianStoredInt(state, DefaultActorID, xiuxianFieldCurrency) <= purse {
		t.Fatal("sale did not pay the real purse")
	}
	if item, ok := f16StoredPack(t, reopened, storyID)["talisman.ward.lower"].(map[string]any); ok && xiuxianCarriedQuantity(item) != 0 {
		t.Fatal("sale left a spendable copy")
	}
}

func TestTalismanLearningRefusesInvalidClaimsAndCraftFailureIsDurable(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	method, _ := cultivation.NewCraftRegistryWithBuiltins().Method("craft.talisman.ward")
	for _, request := range []TurnSkillLearning{{DefID: method.ID}, {DefID: method.ID, Version: 9, Source: "师承"}, {DefID: method.ID, Source: "师承", Quality: "高阶"}} {
		state := combatTestState(t, store, storyID)
		ops, result, _ := xiuxianSkillLearningOps(state, f16SubmissionSystem(t, store, storyID), DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request}}, "learn")
		if len(ops) != 0 || len(result.Refused) != 1 {
			t.Fatalf("invalid learning granted: %+v %+v", ops, result)
		}
	}
	talismanTurn(t, store, storyID, "main", "学法备料", &TurnResult{
		Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: method.ID, Source: "符师指点"}}},
		Trade:  &TurnTradeInput{Merchant: "符铺", Buys: []TurnTradeBuy{{DefID: "material.talisman.paper", Name: "符纸", Quantity: 1}, {DefID: "material.talisman.ink", Name: "灵墨", Quantity: 1}}},
	})
	state := combatTestState(t, store, storyID)
	for _, seconds := range []int64{60, 119 * 60} {
		outcome, refusal := planTurnCraftCommit(StoryDirectorActorStateSystem{}, DefaultActorID, state, &TurnCraftInput{MethodID: method.ID, DurationSeconds: seconds}, DomainCommitIdentity{}, "short")
		if refusal != "" || outcome.elapsed != seconds || len(outcome.ops) == 0 || len(outcome.batch.Products) != 0 {
			t.Fatal("partial batch must save actual work without issuing products")
		}
	}
	var identity DomainCommitIdentity
	for i := 1; i < 400; i++ {
		identity = b1Identity(fmt.Sprintf("ward-loss-%d", i), 1)
		if craftDraw(identity, "", method.ID) < method.LossChance {
			break
		}
	}
	identity = b1CommitInput(t, store, storyID, identity, "试制")
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "试制", Narrative: "符纹在最后一笔散了。", TurnResult: &TurnResult{Choices: testTurnChoices(), Craft: &TurnCraftInput{MethodID: method.ID, DurationSeconds: 120 * 60}}, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle})
	if err != nil {
		t.Fatal(err)
	}
	pack := f16StoredPack(t, store, storyID)
	if _, made := pack["talisman.ward.lower"]; made {
		t.Fatal("failed batch made a ward")
	}
	for _, key := range []string{"符纸", "灵墨"} {
		entry, _ := pack[key].(map[string]any)
		if xiuxianCarriedQuantity(entry) != 0 {
			t.Fatal("failed batch did not consume materials")
		}
	}
	clock, _, err := store.CultivationState(storyID, "main")
	if err != nil || clock.Seconds != 120*60 {
		t.Fatalf("failure time = %+v: %v", clock, err)
	}
	if page := CraftMethodsContext(StoryDirectorActorStateSystem{}, state, DefaultActorID, StoryContextMaxBytes); !strings.Contains(page, "已学会") || !strings.Contains(page, "craft.talisman.ward") {
		t.Fatal("craft context omitted learned method")
	}
}

func TestTalismanLearningAndCraftingInOneTurnUsesSamePreviewAndCommit(t *testing.T) {
	store, storyID, _ := talismanStory(t)
	var identity DomainCommitIdentity
	for cycle := 1; cycle < 400; cycle++ {
		identity = b1Identity("learn-buy-craft", cycle)
		if craftDraw(identity, "", "craft.talisman.ward") >= .10 {
			break
		}
	}
	input := DecodeInteractiveTurnSubmissionInput(`{"skills":{"skills":[{"def_id":"craft.talisman.ward","source":"符师当面指点"}]},"trade":{"merchant":"符铺","buys":[{"def_id":"material.talisman.paper","name":"符纸","quantity":1},{"def_id":"material.talisman.ink","name":"灵墨","quantity":1}]},"craft":{"method_id":"craft.talisman.ward","duration_seconds":7200}}`)
	prepared, receipt := PrepareTurnSubmission(TurnSubmissionContext{ActorState: f16SubmissionSystem(t, store, storyID), CurrentState: combatTestState(t, store, storyID), ChoiceCount: 5, CraftIdentity: identity}, nil, input)
	if receipt.ModuleStatus.Craft != TurnSubmissionModuleAccepted || receipt.NarrativeRewriteRequired {
		t.Fatalf("same-turn learning was missing from craft preview: %+v", receipt)
	}
	identity = b1CommitInput(t, store, storyID, identity, "学绘并制一张护身符")
	result := prepared.TurnResult()
	result.Choices = testTurnChoices()
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{BranchID: "main", User: "学绘并制一张护身符", Narrative: "在符师指点下绘成护身符。", TurnResult: &result, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle})
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := f16StoredPack(t, store, storyID)["talisman.ward.lower"].(map[string]any)
	if entry == nil || xiuxianCarriedQuantity(entry) != 1 {
		t.Fatal("preview and commit disagree on craft output")
	}
}
