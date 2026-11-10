package interactive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

func generatedCapacityRecords(count int) map[string]any {
	records := make(map[string]any, count)
	for i := 0; i < count; i++ {
		category, template := "equipment", "implement.lower.common"
		if i%3 == 1 {
			category, template = "craft", "craft.smithing.iron_sword"
		}
		if i%3 == 2 {
			category, template = "technique", "method.basic_breathing"
		}
		name := fmt.Sprintf("旧录%05d", i)
		idCategory := category
		if category == "equipment" {
			idCategory = "item"
		}
		id, _ := generatedContentID(idCategory, template, name)
		records[id] = generatedContentRecord(category, id, template, name, "实录所载的传授或交付", "旧记录")
	}
	return records
}

func generatedCapacityState(records map[string]any) map[string]any {
	state := map[string]any{}
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, xiuxianFieldSkillRegistry), records)
	return state
}

func generatedCapacityActor(count int) AdventureActor {
	a := NewAdventureActor(adventureTestRules())
	for i := 0; i < count; i++ {
		a.GeneratedDefinitions = append(a.GeneratedDefinitions, AdventureGeneratedDefinition{Category: "craft", TemplateID: "make-blade", Name: fmt.Sprintf("锻法%05d", i), Source: "既有船坞锻造记录"})
	}
	return a
}

func TestGeneratedArchiveCultivationLookupScalesWithRequestedIDs(t *testing.T) {
	records := generatedCapacityRecords(4096)
	state := generatedCapacityState(records)
	itemID, _ := generatedContentID("item", "implement.lower.common", "旧录00000")
	methodID, _ := generatedContentID("craft", "craft.smithing.iron_sword", "旧录00001")
	if _, ok := storyItemCatalogFromState(cultivation.ReferenceRules(),
		state).Definition(itemID); !ok {
		t.Fatal("old item unavailable")
	}
	if _, ok := storyCraftRegistryFromState(xiuxianTestSystem(), state).Method(methodID); !ok {
		t.Fatal("old recipe unavailable")
	}
	// A malformed unrelated record must neither poison this lookup nor be
	// silently accepted if its own ID is requested.
	records["gen.invalid"] = map[string]any{"类别": "equipment", "模板": "implement.lower.common", "名称": "坏身份", "来源": "坏记录", "定义": itemID}
	if _, ok := storyItemCatalogFromState(cultivation.ReferenceRules(),
		state).Definition("gen.invalid"); ok {
		t.Fatal("mismatched archive key accepted")
	}
	tiny := generatedCapacityState(generatedCapacityRecords(12))
	small := testing.AllocsPerRun(10, func() {
		_, _ = storyItemCatalogFromState(cultivation.ReferenceRules(),
			tiny).Definition(itemID)
	})
	large := testing.AllocsPerRun(10, func() {
		_, _ = storyItemCatalogFromState(cultivation.ReferenceRules(),
			state).Definition(itemID)
	})
	if large > small+20 {
		t.Fatalf("lookup allocations grew with archive: small %.0f, large %.0f", small, large)
	}
	// A new legal record is not blocked at the old 512-item threshold.
	candidateID, _ := generatedContentID("item", "implement.lower.common", "新收剑")
	candidate := generatedContentRecord("equipment", candidateID, "implement.lower.common", "新收剑", "真实交付", "")
	if !generatedRegistryCanAdd(records, candidate) {
		t.Fatal("ordinary archive growth still blocked")
	}
	old := generatedCapacityState(records)
	newer := generatedCapacityRecords(4096)
	newer[candidateID] = candidate
	jobs := NewGeneratedDefinitionArtJobs(old, generatedCapacityState(newer), "ink")
	if len(jobs) != 1 || jobs[0].DefinitionID != candidateID {
		t.Fatalf("historical illustration backlog: %d jobs", len(jobs))
	}
}

func TestGeneratedArchiveAdventureResolvesOldRecipeBeyondWorkingSet(t *testing.T) {
	r := adventureTestRules()
	a := generatedCapacityActor(2048)
	rules, err := adventureRulesForActor(r, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Recipes) > len(r.Recipes)+generatedContextDefinitionLimit {
		t.Fatalf("archive eagerly expanded: %d", len(rules.Recipes))
	}
	oldID, _ := a.GeneratedDefinitions[0].id()
	for _, method := range rules.Recipes {
		if method.ID == oldID {
			t.Fatal("old unrequested recipe unexpectedly in small working set")
		}
	}
	before := a.Inventory["scrap"]
	next, out, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: oldID, Hours: 2}, "old-recipe")
	if err != nil || next.Inventory["blade"] != 1 || next.Inventory["scrap"] >= before || out.DurationSeconds != 7200 {
		t.Fatalf("old method cannot settle: %v %+v", err, out)
	}
	newDef := AdventureGeneratedDefinition{Category: "craft", TemplateID: "make-blade", Name: "新登记法", Source: "实际传授"}
	next, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "generate", Generate: &newDef}, "add")
	if err != nil || len(next.GeneratedDefinitions) != 2049 {
		t.Fatalf("growth past old limit: %v", err)
	}
	retry, _, err := ResolveAdventureAction(r, next, AdventureAction{Action: "generate", Generate: &newDef}, "repeat")
	if err != nil || !reflect.DeepEqual(next, retry) {
		t.Fatalf("repeat changed archive: %v", err)
	}
	newDef.Description = "attempted rewrite"
	if _, _, err = ResolveAdventureAction(r, next, AdventureAction{Action: "generate", Generate: &newDef}, "conflict"); err == nil {
		t.Fatal("same identity rewrite accepted")
	}
	corrupt := a
	corrupt.GeneratedDefinitions = append(append([]AdventureGeneratedDefinition(nil), a.GeneratedDefinitions...), a.GeneratedDefinitions[0])
	if _, err = adventureRulesForActor(r, corrupt); err == nil {
		t.Fatal("duplicate archive identity accepted")
	}
}

func TestGeneratedArchiveContextIsBoundedAndAuthoritativeStateUnchanged(t *testing.T) {
	r := adventureTestRules()
	a := NewAdventureActor(r)
	for i := 0; i < 1024; i++ {
		d := AdventureGeneratedDefinition{Category: "skill", TemplateID: "strike", Name: fmt.Sprintf("拳%05d", i), Source: "已学拳法自研"}
		a.GeneratedDefinitions = append(a.GeneratedDefinitions, d)
		id, _ := d.id()
		a.Skills[id] = 1
	}
	catalog, err := adventureRulesForActor(r, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = catalog.Validate(); err != nil {
		t.Fatalf("runtime archive charged to frozen catalog: %v", err)
	}
	context := AdventureRulesContext(catalog)
	if len(context) > 110*1024 || !strings.Contains(context, "未展开") {
		t.Fatalf("unbounded context: %d bytes", len(context))
	}
	projected, _ := adventureGeneratedContextRules(catalog)
	if len(projected.Skills) > len(r.Skills)+generatedContextDefinitionLimit {
		t.Fatal("prompt retained lifetime skill catalog")
	}
	if len(catalog.Skills) != len(r.Skills)+1024 || len(a.GeneratedDefinitions) != 1024 {
		t.Fatal("context trimming modified real definitions")
	}
	records := generatedCapacityRecords(4096)
	projection := generatedRuntimeFieldValue(normalizeActorStateFieldName(xiuxianFieldSkillRegistry), records)
	encoded, _ := json.Marshal(projection)
	if len(encoded) > 16000 || len(records) != 4096 {
		t.Fatalf("handbook archive leaked: %d", len(encoded))
	}
}

func TestGeneratedArchiveByteGuardPreservesExistingIdentity(t *testing.T) {
	records := map[string]any{}
	for i := 0; i < 600; i++ {
		id := fmt.Sprintf("entry%d", i)
		records[id] = map[string]any{"定义": id, "描述": strings.Repeat("x", 8192)}
	}
	candidate := map[string]any{"定义": "new"}
	if generatedRegistryCanAdd(records, candidate) {
		t.Fatal("oversized archive accepted new record")
	}
	if !generatedRegistryCanAdd(records, records["entry0"].(map[string]any)) {
		t.Fatal("safety guard blocked reuse of existing identity")
	}
	if len(records) != 600 {
		t.Fatal("capacity refusal discarded old records")
	}
	a := generatedCapacityActor(600)
	for i := range a.GeneratedDefinitions {
		a.GeneratedDefinitions[i].Description = strings.Repeat("x", 8192)
	}
	if err := validateAdventureGeneratedArchive(a.GeneratedDefinitions); err == nil {
		t.Fatal("adventure archive exceeds journal headroom")
	}
}

// Seed a large historical archive through the real journal, then perform new
// rule-owned registration and learning. This is synthetic scale/replay QA, not
// a claim of thousands of natural model-driven turns.
func TestGeneratedArchiveCultivationStoryColdReplayAndBranch(t *testing.T) {
	store, storyID, root := talismanStory(t)
	initial, _ := store.Snapshot(storyID, "main")
	old, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: initial.CurrentTurn.ID, Title: "登记之前"})
	if err != nil {
		t.Fatal(err)
	}
	records := generatedCapacityRecords(1200)
	if _, err = store.AppendStateDelta(storyID, AppendStateDeltaRequest{BranchID: "main", ActorOps: []ActorStateOp{generatedDefinitionOp(records, initial.CurrentTurn.ID, StateOpSourceSkillLearning)}}); err != nil {
		t.Fatal(err)
	}
	generated := talismanTurn(t, store, storyID, "main", "登记新的火诀", &TurnResult{Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "fire_bolt", Name: "远录新火", Source: "真实手札", Minutes: 1440}}}})
	now, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	archive, _ := xiuxianEquipmentEntry(now.State, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	skillID := cultivation.GeneratedSkillID("fire_bolt", "远录新火")
	if len(archive) != 1201 {
		t.Fatalf("archive truncated: %d", len(archive))
	}
	if _, ok := storySkillRegistryFromState(cultivation.ReferenceRules(),
		now.State).Current(skillID); !ok {
		t.Fatal("new skill missing")
	}
	before, _ := store.Snapshot(storyID, old.ID)
	if _, ok := storySkillRegistryFromState(cultivation.ReferenceRules(),
		before.State).Current(skillID); ok {
		t.Fatal("future skill leaked to earlier branch")
	}
	fork, err := store.CreateBranch(storyID, CreateBranchRequest{ParentEventID: generated.ID, Title: "完整档案"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	cold := NewStore(root)
	defer cold.Close()
	reopened, err := cold.Snapshot(storyID, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, _ := xiuxianEquipmentEntry(reopened.State, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	if !reflect.DeepEqual(archive, restored) {
		t.Fatal("cold fork lost archive")
	}
	page, err := cold.SearchGeneratedDefinitions(storyID, fork.ID, GeneratedDefinitionSearchRequest{ID: skillID})
	if err != nil || len(page.Entries) != 1 || len(page.Entries[0].ActorRecords) == 0 {
		t.Fatalf("cold archive read lacks actual learning fact: %+v %v", page, err)
	}
	absent, err := cold.SearchGeneratedDefinitions(storyID, old.ID, GeneratedDefinitionSearchRequest{ID: skillID})
	if err != nil || len(absent.Entries) != 0 {
		t.Fatalf("read tool leaked future definition: %+v %v", absent, err)
	}

	methodID, _ := generatedContentID("craft", "craft.smithing.iron_sword", "旧录00001")
	if _, ok := storyCraftRegistryFromState(reopened.ActorStateSchema.System, reopened.State).Method(methodID); !ok {
		t.Fatal("old recipe unresolved after cold reopen")
	}
}

func BenchmarkGeneratedArchiveItemLookup(b *testing.B) {
	for _, n := range []int{64, 4096} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			state := generatedCapacityState(generatedCapacityRecords(n))
			id, _ := generatedContentID("item", "implement.lower.common", "旧录00000")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := storyItemCatalogFromState(cultivation.ReferenceRules(),
					state).Definition(id); !ok {
					b.Fatal("missing")
				}
			}
		})
	}
}

func BenchmarkGeneratedArchiveAdventureWorkingSet(b *testing.B) {
	for _, n := range []int{64, 4096} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			r := adventureTestRules()
			a := generatedCapacityActor(n)
			id, _ := a.GeneratedDefinitions[0].id()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := adventureRulesForActor(r, a, id); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestGeneratedArchiveAdventureStoryColdReplay(t *testing.T) {
	r := adventureTestRules()
	store, id := adventureTestStoryWithRules(t, r)
	defer store.Close()
	initial, _ := store.Snapshot(id, "main")
	old, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: initial.CurrentTurn.ID, Title: "旧档案"})
	if err != nil {
		t.Fatal(err)
	}
	a := generatedCapacityActor(1024)
	if _, err = store.AppendStateDelta(id, AppendStateDeltaRequest{BranchID: "main", ActorOps: adventureActorOps(a, initial.CurrentTurn.ID)}); err != nil {
		t.Fatal(err)
	}
	d := AdventureGeneratedDefinition{Category: "craft", TemplateID: "make-blade", Name: "千录新法", Source: "正式船坞传授"}
	if _, _, err = store.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "登记新锻法", Narrative: "记录新法。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "登记新锻法", Adventure: &AdventureAction{Action: "generate", Generate: &d}}}}); err != nil {
		t.Fatal(err)
	}
	window, err := store.AdventureWindow(id, "main")
	if err != nil || len(window.Actor.GeneratedDefinitions) != 1025 {
		t.Fatalf("large registration: %v", err)
	}
	before, err := store.AdventureWindow(id, old.ID)
	if err != nil || len(before.Actor.GeneratedDefinitions) != 0 {
		t.Fatalf("archive leaked to old branch: %v", err)
	}
	root := store.Root()
	_ = store.Close()
	cold := NewStore(root)
	defer cold.Close()
	restored, err := cold.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(restored.Actor, window.Actor) {
		t.Fatalf("cold archive changed: %v", err)
	}
	oldID, _ := restored.Actor.GeneratedDefinitions[0].id()
	if _, _, err = cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "按最早锻法制作两小时", Narrative: "完成锻造。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "按最早锻法制作两小时", Adventure: &AdventureAction{Action: "craft", RecipeID: oldID, Hours: 2}}}}); err != nil {
		t.Fatal(err)
	}
	made, err := cold.AdventureWindow(id, "main")
	if err != nil || made.Actor.Inventory["blade"] != 1 || made.ClockSeconds != 7200 {
		t.Fatalf("old archived craft failed after reopen: %v %+v", err, made.Actor.Inventory)
	}
}

func TestGeneratedArchiveSearchPaginatesValidMechanicsAndPinsBranchHead(t *testing.T) {
	records := generatedCapacityRecords(4096)
	state := generatedCapacityState(records)
	system := xiuxianTestSystem()
	first, err := searchGeneratedDefinitions(system, state, "main/head1", GeneratedDefinitionSearchRequest{Category: "equipment", Limit: 3})
	if err != nil || len(first.Entries) != 3 || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := searchGeneratedDefinitions(system, state, "main/head1", GeneratedDefinitionSearchRequest{Category: "equipment", Limit: 3, Cursor: first.NextCursor})
	if err != nil || len(second.Entries) != 3 || first.Entries[2].ID >= second.Entries[0].ID {
		t.Fatalf("second page overlap: %+v %v", second, err)
	}
	if _, err = searchGeneratedDefinitions(system, state, "main/head2", GeneratedDefinitionSearchRequest{Category: "equipment", Cursor: first.NextCursor}); err == nil {
		t.Fatal("stale head cursor accepted")
	}
	if _, err = searchGeneratedDefinitions(system, state, "fork/head1", GeneratedDefinitionSearchRequest{Category: "equipment", Cursor: first.NextCursor}); err == nil {
		t.Fatal("cursor leaked across branches")
	}
	if _, err = searchGeneratedDefinitions(system, state, "main/head1", GeneratedDefinitionSearchRequest{Category: "craft", Cursor: first.NextCursor}); err == nil {
		t.Fatal("cursor reused for another category")
	}
	found, err := searchGeneratedDefinitions(system, state, "main/head1", GeneratedDefinitionSearchRequest{Query: "旧录04095"})
	if err != nil || len(found.Entries) != 1 || found.Entries[0].Name != "旧录04095" {
		t.Fatalf("name lookup cannot reach older hidden item: %+v %v", found, err)
	}
	item, ok := found.Entries[0].Definition.(cultivation.ItemDef)
	if !ok || item.Implement == nil || item.Implement.AttackBonus != .05 {
		t.Fatal("lookup did not return validated actual mechanics")
	}
	r := adventureTestRules()
	a := generatedCapacityActor(2048)
	adventureState := map[string]any{}
	b, _ := json.Marshal(a)
	var record map[string]any
	_ = json.Unmarshal(b, &record)
	setPath(adventureState, actorStateFieldPath(DefaultActorID, AdventureStateField), record)
	oldest, _ := a.GeneratedDefinitions[0].id()
	page, err := searchGeneratedDefinitions(StoryDirectorActorStateSystem{AdventureRules: &r}, adventureState, "space/head", GeneratedDefinitionSearchRequest{ID: oldest})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Product == nil {
		t.Fatalf("old space recipe unavailable: %v %+v", err, page)
	}
	a.GeneratedDefinitions[0].Design = &AdventureGeneratedDesign{Power: generatedPower(20), ProductName: "存档成品", ProductTemplateID: "blade"}
	productID := adventureGeneratedProductID(a.GeneratedDefinitions[0])
	a.Inventory[productID] = 2
	b, _ = json.Marshal(a)
	_ = json.Unmarshal(b, &record)
	setPath(adventureState, actorStateFieldPath(DefaultActorID, AdventureStateField), record)
	product, err := searchGeneratedDefinitions(StoryDirectorActorStateSystem{AdventureRules: &r}, adventureState, "space/head2", GeneratedDefinitionSearchRequest{ID: productID, Category: "equipment"})
	if err != nil || len(product.Entries) != 1 || product.Entries[0].AdventureUsage["inventory"] != 2 {
		t.Fatalf("generated recipe product cannot be read by its own ID: %+v %v", product, err)
	}

}

func TestGeneratedArchiveFullByteBudgetCannotBeBypassedWithinBatch(t *testing.T) {
	oversized := map[string]any{"定义": "oversized", "来源": strings.Repeat("x", 8193)}
	if generatedRegistryCanAdd(nil, oversized) {
		t.Fatal("single oversized prose field could evade the bounded read contract")
	}
	state := xiuxianSkillTestState(t, "qi_7", nil)
	records := map[string]any{}
	for i := 0; i < 600; i++ {
		id := fmt.Sprintf("full%d", i)
		records[id] = map[string]any{"定义": id, "描述": strings.Repeat("x", 8192)}
	}
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, xiuxianFieldSkillRegistry), records)
	request := TurnSkillLearning{TemplateID: "fire_bolt", Name: "重复提交之火", Source: "真实传授", Minutes: 1440}
	ops, result, _ := xiuxianSkillLearningOps(state, xiuxianSkillTestSystem(), DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{request, request}}, "full")
	if len(result.Refused) != 2 || len(result.Learned) != 0 || len(result.Studied) != 0 || len(ops) != 0 {
		t.Fatalf("refused registration leaked within batch: %+v %+v", result, ops)
	}
}

func TestGeneratedArchiveRuntimeCatalogCannotLeakIntoEmptyBranch(t *testing.T) {
	r := adventureTestRules()
	actor := generatedCapacityActor(64)
	expanded, err := adventureRulesForActor(r, actor)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := adventureRulesForActor(expanded, NewAdventureActor(r))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(empty, r) {
		t.Fatal("runtime definitions leaked into a branch with no archive")
	}
}
