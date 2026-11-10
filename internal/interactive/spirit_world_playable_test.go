package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"testing"
)

// Qualified high-stage fixtures isolate access and acquisition. No story claims
// that these actors naturally trained from qi to the fixture's starting stage.
// Every item used below is bought or earned through the real Story consumers.
func spiritWorldStory(t *testing.T, stage, location string, cash int) (*Store, string, string) {
	t.Helper()
	data, err := os.ReadFile("../storypack/assets/fanren-jie.json")
	if err != nil {
		t.Fatal(err)
	}
	var pack struct {
		ContentVersion int                 `json:"content_version"`
		Characters     []PackCatalogPerson `json:"characters"`
	}
	if err = json.Unmarshal(data, &pack); err != nil {
		t.Fatal(err)
	}
	sys := xiuxianTestSystem()
	sys.WorldContent = authoredWorldContent(t, "fanren-jie")
	st, _ := cultivation.ParseStageKey(stage)
	base := cultivation.BaseAttributes(st)
	sys.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "远行者", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: stage, xiuxianFieldCurrency: float64(cash), xiuxianFieldHealth: float64(base.HealthMax), xiuxianFieldMana: float64(base.ManaMax)}},
		{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: ActorStateStoryContextTemplateID, State: map[string]any{storySceneLocationField: journeyPlaceLabel(location)}},
	}
	root := t.TempDir()
	s := NewStore(root)
	t.Cleanup(func() { _ = s.Close() })
	meta, err := s.CreateStory(CreateStoryRequest{Title: "灵界地域真实链", ActorState: &sys, PackBinding: fanrenBinding(pack.ContentVersion), PackCatalog: &PackCatalogSnapshot{ContentVersion: pack.ContentVersion, Characters: pack.Characters}})
	if err != nil {
		t.Fatal(err)
	}
	talismanTurn(t, s, meta.ID, "main", "查看当前位置", &TurnResult{})
	return s, meta.ID, root
}
func spiritBuy(t *testing.T, s *Store, id, merchant, def, name string, n int) {
	t.Helper()
	talismanTurn(t, s, id, "main", "付钱购买"+name, &TurnResult{Trade: &TurnTradeInput{Merchant: merchant, Buys: []TurnTradeBuy{{DefID: def, Name: name, Quantity: n}}}})
}
func spiritJourney(t *testing.T, s *Store, id, route string) JourneyResult {
	t.Helper()
	return journeyTestApply(t, s, id, JourneyRequest{RouteID: route, Arrive: true, CommandID: route, CommandHash: route})
}
func spiritMeet(t *testing.T, s *Store, id, key, name string) {
	t.Helper()
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "与" + name + "当面谈起此地见闻", Narrative: "两人在此地当面交谈。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{DefaultActorID, key}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: key, Name: name, SourceKey: key}}}}})
	if err != nil {
		t.Fatal(err)
	}
}
func spiritQuest(t *testing.T, s *Store, id string, req QuestRequest) QuestResult {
	t.Helper()
	req.BranchID = "main"
	out, err := s.ApplyQuest(id, req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func spiritCount(t *testing.T, s *Store, id, branch, item string) int {
	return realmExploreQuantity(t, s, id, branch, "fanren-jie", item)
}
func spiritRealmAction(t *testing.T, s *Store, id, instance, realm, action, method string) RealmResult {
	t.Helper()
	return realmTestApply(t, s, id, realmExploreRequest(instance, realm, action, realm+"."+method, realm+":"+method))
}
func TestSpiritWorldPreparedCrossingAcquisitionColdReplayAndBranch(t *testing.T) {
	s, id, root := spiritWorldStory(t, "void_early", "yue.market", 20000)
	if _, err := s.ApplyJourney(id, JourneyRequest{RouteID: spiritAscentRoute, Arrive: true}); err == nil {
		t.Fatal("unprepared remote crossing succeeded")
	}
	spiritBuy(t, s, id, "坊市阵材商", "material.spirit.boundary_jade", "护界玉", 1)
	spiritBuy(t, s, id, "坊市阵材商", "material.stone.ward", "阵石", 20)
	spiritJourney(t, s, id, "spirit-world.human-road:out")
	d := exploration.RealmSpiritBoundary
	entered := realmTestApply(t, s, id, realmExploreRequest("", d, RealmActionEnter, "", "enter-boundary"))
	spiritRealmAction(t, s, id, entered.InstanceID, d, "survey", "survey")
	spiritRealmAction(t, s, id, entered.InstanceID, d, "resolve", "anchor")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionExit, "", "exit-boundary"))
	if spiritCount(t, s, id, "main", "material.spirit.boundary_anchor") != 1 || spiritCount(t, s, id, "main", "material.spirit.boundary_jade") != 0 || spiritCount(t, s, id, "main", "material.stone.ward") != 0 {
		t.Fatal("preparation did not consume purchased materials into one anchor")
	}
	checkpoint := talismanTurn(t, s, id, "main", "整理行囊，仍可选择下山", &TurnResult{})
	before, _ := s.Snapshot(id, "main")
	req := JourneyRequest{RouteID: spiritAscentRoute, TargetMinutes: 60, CommandID: "cross-first-hour", CommandHash: "cross-first-hour"}
	first := journeyTestApply(t, s, id, req)
	paid, _ := s.Snapshot(id, "main")
	stage, _ := cultivation.ParseStageKey("void_early")
	base := cultivation.BaseAttributes(stage)
	if first.Arrived || spiritCount(t, s, id, "main", "material.spirit.boundary_anchor") != 0 || xiuxianStoredInt(paid.State, DefaultActorID, xiuxianFieldHealth) != xiuxianStoredInt(before.State, DefaultActorID, xiuxianFieldHealth)-base.HealthMax/4 || xiuxianStoredInt(paid.State, DefaultActorID, xiuxianFieldMana) != xiuxianStoredInt(before.State, DefaultActorID, xiuxianFieldMana)-base.ManaMax/2 {
		t.Fatal("departure cost was not committed exactly")
	}
	retry := journeyTestApply(t, s, id, req)
	if !retry.Replayed {
		t.Fatal("crossing retry was not idempotent")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	defer s.Close()
	arrived := journeyTestApply(t, s, id, JourneyRequest{JourneyID: first.JourneyID, Arrive: true, CommandID: "continue-crossing", CommandHash: "continue-crossing"})
	after, _ := s.Snapshot(id, "main")
	if !arrived.Arrived || journeyTestLocation(t, s, id, "main") != journeyPlaceLabel("spirit.border.camp") || xiuxianStoredInt(after.State, DefaultActorID, xiuxianFieldMana) != xiuxianStoredInt(paid.State, DefaultActorID, xiuxianFieldMana) {
		t.Fatal("cold continuation teleported or charged crossing twice")
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "选择下山"})
	if err != nil {
		t.Fatal(err)
	}
	if spiritCount(t, s, id, fork.ID, "material.spirit.boundary_anchor") != 1 {
		t.Fatal("future crossing consumed the other branch's anchor")
	}
	journeyTestApply(t, s, id, JourneyRequest{BranchID: fork.ID, RouteID: "spirit-world.human-road:back", Arrive: true, CommandID: "return-human", CommandHash: "return-human"})
	if journeyTestLocation(t, s, id, fork.ID) != journeyPlaceLabel("yue.market") {
		t.Fatal("refusing ascent trapped the branch")
	}
	spiritMeet(t, s, id, "spirit_qiao_yan", "乔砚")
	q := "quest.spirit_rift_review"
	spiritQuest(t, s, id, QuestRequest{TaskID: q, Action: QuestActionAccept})
	d = exploration.RealmSpiritRift
	entered = realmTestApply(t, s, id, realmExploreRequest("", d, RealmActionEnter, "", "enter-rift"))
	spiritRealmAction(t, s, id, entered.InstanceID, d, "survey", "survey")
	spiritRealmAction(t, s, id, entered.InstanceID, d, "approach", "wait")
	spiritRealmAction(t, s, id, entered.InstanceID, d, "resolve", "recover")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionExit, "", "exit-rift"))
	if spiritCount(t, s, id, "main", "manual.spirit.void") != 1 || spiritCount(t, s, id, "main", "equipment.void_blade") != 1 {
		t.Fatal("real exploration failed to grant the source and gear")
	}
	// Leaving and re-entering preserves spent salvage rather than generating loot.
	re := realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionReenter, "", "return-rift"))
	if _, err := s.ApplyRealm(id, realmExploreRequest(re.InstanceID, d, "resolve", d+".recover", "recover-again")); err == nil {
		t.Fatal("finite salvage refreshed")
	}
	realmTestApply(t, s, id, realmExploreRequest(re.InstanceID, d, RealmActionExit, "", "exit-rift-again"))
	spiritMeet(t, s, id, "spirit_qiao_yan", "乔砚")
	// A different branch loses the sample after recovery. Historical proof may
	// complete the objective, but cannot fund a physical examination without it.
	reviewCheckpoint := talismanTurn(t, s, id, "main", "拿出样石准备复验", &TurnResult{})
	lost, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: reviewCheckpoint.ID, Title: "遗失样石后不能空手复验"})
	if err != nil {
		t.Fatal(err)
	}
	lostState := combatTestState(t, s, id)
	pack, _ := xiuxianEquipmentEntry(lostState, DefaultActorID, xiuxianFieldImportantItems)
	kept := map[string]any{}
	for k, raw := range pack {
		e, _ := raw.(map[string]any)
		if xiuxianEntryText(e, "定义") != "material.spirit.rift_sample" {
			kept[k] = raw
		}
	}
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: lost.ID, User: "样石已遗失", Narrative: "行囊里已没有样石。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldImportantItems, Value: kept}}, TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	lostBefore, _ := s.Snapshot(id, lost.ID)
	if _, err = s.ApplyQuest(id, QuestRequest{BranchID: lost.ID, TaskID: q, Action: QuestActionSettle}); err == nil {
		t.Fatal("empty-handed review paid on historical evidence alone")
	}
	lostAfter, _ := s.Snapshot(id, lost.ID)
	if !reflect.DeepEqual(lostBefore.State, lostAfter.State) || !reflect.DeepEqual(lostBefore.Clock, lostAfter.Clock) {
		t.Fatal("refused review changed money or time")
	}
	wallet := xiuxianStoredInt(combatTestState(t, s, id), DefaultActorID, xiuxianFieldCurrency)
	spiritQuest(t, s, id, QuestRequest{TaskID: q, Action: QuestActionSettle})
	if xiuxianStoredInt(combatTestState(t, s, id), DefaultActorID, xiuxianFieldCurrency) != wallet+25000 {
		t.Fatal("in-person examination did not pay the agreed amount")
	}
	spiritJourney(t, s, id, "spirit-world.sunset-road:out")
	spiritBuy(t, s, id, "百工坊书器铺", "equipment.void_robe", "购得五行法袍", 1)
	talismanTurn(t, s, id, "main", "装备本次带回的灵剑与购得法袍", &TurnResult{Items: &TurnItemsInput{Equip: []TurnItemEquip{{InstanceID: "equipment.void_blade"}, {InstanceID: "购得五行法袍"}}}})
	method, _ := cultivation.TechniqueByID("method.void_unification")
	talismanTurn(t, s, id, "main", "依取得的完整炼虚注本研习一百八十日", &TurnResult{Techniques: &TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: method.ID, Source: "本次裂脉旧匣中的完整炼虚三法注本", Minutes: float64(method.StandardLearningMinutes)}}}})
	talismanTurn(t, s, id, "main", "循注本研习炼虚灵罩七日", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{DefID: "void_ward", Source: "持有的炼虚三法注本", Minutes: 10080}}}})
	state := combatTestState(t, s, id)
	records, _ := readTechniqueRecords(state, DefaultActorID)
	mastered := false
	for _, r := range records {
		if r.DefID == method.ID && r.Mastered {
			mastered = true
		}
	}
	if !mastered {
		t.Fatal("acquired source did not support paid method mastery")
	}
	if !learnedSkillsFromState(state, DefaultActorID, storySkillRegistryFromState(cultivation.ReferenceRules(),
		state)).Knows("void_ward") {
		t.Fatal("held source did not support paid skill learning")
	}
	beforeCold, _ := s.Snapshot(id, "main")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	defer s.Close()
	cold, _ := s.Snapshot(id, "main")
	if !reflect.DeepEqual(beforeCold.State, cold.State) || !reflect.DeepEqual(beforeCold.Clock, cold.Clock) {
		t.Fatal("cold rebuild lost acquisitions, source, study or time")
	}
	if spiritCount(t, s, id, fork.ID, "manual.spirit.void") != 0 {
		t.Fatal("far-side inheritance leaked to the human branch")
	}
}

func TestSpiritWorldSourceAndSupplyCannotBeInvented(t *testing.T) {
	s, id, _ := spiritWorldStory(t, "void_early", "yue.market", 100000)
	before := combatTestState(t, s, id)
	if _, err := QuoteTrade(cultivation.ReferenceRules(),
		before, TradeQuoteRequest{Merchant: "坊市器阁", DefID: "manual.spirit.void", Direction: "buy"}); err == nil {
		t.Fatal("human merchant quoted a nonexistent high-tier source")
	}
	def, _ := cultivation.TechniqueByID("method.void_unification")
	ops, out, _ := xiuxianTechniqueOps(before, xiuxianTestSystem(), DefaultActorID, TurnTechniqueInput{Learn: []TurnTechniqueLearning{{DefID: def.ID, Source: "我声称有完整注本", Minutes: float64(def.StandardLearningMinutes)}}}, "fake-source")
	if len(ops) != 0 || len(out.Learned) > 0 || len(out.Refused) != 1 {
		t.Fatalf("prose created a manual: %+v", out)
	}
	low := cloneStoryState(before)
	applyActorStateOp(low, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldRealm, Value: "qi_6"})
	if _, err := spiritCrossingOps(low, []string{DefaultActorID}); err == nil {
		t.Fatal("low realm crossed")
	}

}

func TestSpiritWorldArrayChoicesReturnAndArchiveSources(t *testing.T) {
	s, id, root := spiritWorldStory(t, "integration_late", "spirit.tianyuan.market", 1000)
	spiritBuy(t, s, id, "南坊修器铺", "material.stone.ward", "阵石", 10)
	d := exploration.RealmSpiritArray
	entered := realmTestApply(t, s, id, realmExploreRequest("", d, RealmActionEnter, "", "array-enter"))
	spiritRealmAction(t, s, id, entered.InstanceID, d, "survey", "survey")
	checkpoint := talismanTurn(t, s, id, "main", "比较保留和拆取之后的去向", &TurnResult{})
	spiritRealmAction(t, s, id, entered.InstanceID, d, "resolve", "preserve")
	if _, err := s.ApplyRealm(id, realmExploreRequest(entered.InstanceID, d, "resolve", d+".return", "too-soon")); err == nil {
		t.Fatal("return cache opened before settling time")
	}
	if _, err := s.ApplyRealm(id, realmExploreRequest(entered.InstanceID, d, "resolve", d+".strip", "both-choices")); err == nil {
		t.Fatal("exclusive disposition paid twice")
	}
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionExit, "", "array-leave"))
	worldFlowTurn(t, s, id, "在城里等一日", "一日过去。", TurnWorldActionInput{Action: TurnWorldActionWait, DurationSeconds: 86400, SourceText: "在城里等一日"})
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = NewStore(root)
	defer s.Close()
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionReenter, "", "array-return"))
	spiritRealmAction(t, s, id, entered.InstanceID, d, "resolve", "return")
	if spiritCount(t, s, id, "main", "manual.spirit.integration") != 1 || spiritCount(t, s, id, "main", "equipment.integration_robe") != 1 || spiritCount(t, s, id, "main", "equipment.integration_blade") != 0 {
		t.Fatal("preservation reward or rejected alternative is wrong")
	}
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionExit, "", "array-finish"))
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: checkpoint.ID, Title: "拆取阵心"})
	if err != nil {
		t.Fatal(err)
	}
	req := realmExploreRequest(entered.InstanceID, d, "resolve", d+".strip", "fork-strip")
	req.BranchID = fork.ID
	if _, err := s.ApplyRealm(id, req); err != nil {
		t.Fatal(err)
	}
	if spiritCount(t, s, id, fork.ID, "equipment.integration_blade") != 1 || spiritCount(t, s, id, fork.ID, "manual.spirit.integration") != 0 {
		t.Fatal("choice branch inherited an unchosen cache")
	}
	// No teleport or manual seed: the book just recovered is the actual tool at
	// the next optional site, reached over its authored forty-five-day road.
	spiritJourney(t, s, id, "spirit-world.quiet-road:out")
	d = exploration.RealmSpiritArchive
	entered = realmTestApply(t, s, id, realmExploreRequest("", d, RealmActionEnter, "", "archive-enter"))
	spiritRealmAction(t, s, id, entered.InstanceID, d, "survey", "compare")
	spiritRealmAction(t, s, id, entered.InstanceID, d, "resolve", "copy")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d, RealmActionExit, "", "archive-exit"))
	if spiritCount(t, s, id, "main", "manual.spirit.mahayana") != 1 || spiritCount(t, s, id, "main", "manual.spirit.integration") != 1 || spiritCount(t, s, id, "main", "equipment.mahayana_robe") != 1 {
		t.Fatal("held tool was consumed or archive failed to grant source")
	}
	spiritJourney(t, s, id, "spirit-world.quiet-road:back")
	if journeyTestLocation(t, s, id, "main") != journeyPlaceLabel("spirit.tianyuan.market") {
		t.Fatal("optional archive trapped the player")
	}
}

func TestSpiritWorldFrozenCatalogAndGeneratedSourceGuards(t *testing.T) {
	s, id := eligibilityTestStory(t, journeyPlaceLabel("yue.market"), fanrenBinding(30), &PackCatalogSnapshot{ContentVersion: 30}, &WorldContent{PackID: "fanren-jie"})
	options, err := s.JourneyOptionsFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range options.Routes {
		if r.RouteID == "spirit-world.human-road:out" {
			t.Fatal("new continent leaked into old frozen catalog")
		}
	}
	if _, err := s.ApplyJourney(id, JourneyRequest{RouteID: "spirit-world.human-road:out", Arrive: true}); err == nil {
		t.Fatal("old Story bypassed frozen route visibility")
	}
	record := generatedContentRecord("technique", "", "method.void_unification", "改写五行法", "完整传承", "经脉图旁有批注。")
	gid, _ := generatedContentID("technique", "method.void_unification", "改写五行法")
	record["定义"] = gid
	def, ok := generatedTechniqueFromRecord(cultivation.ReferenceRules(),
		record)
	if !ok || def.RequiredManualID != "manual.spirit.void" {
		t.Fatal("generated method lost its physical source")
	}
	template, _ := cultivation.SkillTemplateByID("void_ward")
	skill, err := template.Instantiate("改写灵罩")
	if err != nil || skill.RequiredManualID != "manual.spirit.void" {
		t.Fatal("generated skill lost its physical source")
	}
}

func TestSpiritWorldEnglishLocationOffersLocalizedRoutes(t *testing.T) {
	s, id, _ := spiritWorldStory(t, "void_early", "spirit.sunset.market", 100000)
	_, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "Look around", Narrative: "You are in the workshops.", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: "Sunset City workshops"}}, TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	options, err := s.JourneyOptionsFor(id, "main")
	if err != nil || options.CurrentLocationID != "spirit.sunset.market" || len(options.Routes) != 2 {
		t.Fatalf("English place lost real routes: %+v %v", options, err)
	}
	for _, r := range options.Routes {
		if r.OriginNameKey == "" || r.DestinationNameKey == "" {
			t.Fatalf("missing localized endpoint: %+v", r)
		}
	}
	spiritBuy(t, s, id, "Sunset Book and Gear", "manual.spirit.void", "Complete Void Manual", 1)
	if spiritCount(t, s, id, "main", "manual.spirit.void") != 1 {
		t.Fatal("English supplier lost physical source")
	}
}

func TestSpiritWorldEquipmentLootPreservesEarlierCopyAndSpentCosts(t *testing.T) {
	state := xiuxianSkillTestState(t, "void_early", nil)
	catalog := storyItemCatalog(cultivation.ReferenceRules())
	blade, _ := catalog.Definition("equipment.void_blade")
	stone, _ := catalog.Definition("material.stone.ward")
	prior := xiuxianItemPackEntryWithFlavor(blade, blade.ID, 1, "旧刃有缺口。", "以前购得。")
	applyActorStateOp(state, ActorStateOp{Op: "set", ActorID: DefaultActorID, FieldID: xiuxianFieldImportantItems, Value: map[string]any{blade.ID: prior, stone.ID: xiuxianItemPackEntry(stone, stone.ID, 2)}})
	def, _ := exploration.RealmDefinition(exploration.RealmSpiritRift)
	reward := exploration.InstanceReward{ID: "realm.spirit_rift.reward_blade", RefID: blade.ID, Quantity: 1}
	ops, err := realmExploreActorOps(state, xiuxianTestSystem(), def, []exploration.ExploreResourceCost{{Kind: exploration.ExploreConsumeItem, RefID: stone.ID, Quantity: 2}}, []exploration.InstanceReward{reward}, "test")
	if err != nil || len(ops) != 1 {
		t.Fatalf("loot failed: %v %+v", err, ops)
	}
	pack := ops[0].Value.(map[string]any)
	if !reflect.DeepEqual(pack[blade.ID], prior) {
		t.Fatal("new reward overwrote older physical equipment")
	}
	blades := 0
	for _, raw := range pack {
		entry, _ := raw.(map[string]any)
		if xiuxianEntryText(entry, "定义") == blade.ID {
			blades += xiuxianCarriedQuantity(entry)
		}
	}
	if blades != 2 || xiuxianCarriedQuantity(pack[stone.ID].(map[string]any)) != 0 {
		t.Fatal("gear was merged or consumed costs refunded")
	}
	reward.Quantity = 2
	if _, err := realmExploreActorOps(state, xiuxianTestSystem(), def, nil, []exploration.InstanceReward{reward}, "invalid"); err == nil {
		t.Fatal("equipment silently stacked")
	}
}
