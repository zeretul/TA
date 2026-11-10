package interactive

import (
	"encoding/json"
	"reflect"
	"ta/internal/cultivation"
	"testing"
)

func TestGeneratedEquipmentUsesStoryRegistry(t *testing.T) {
	system := xiuxianTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	ops, result, _ := xiuxianItemOps(state, system, DefaultActorID, TurnItemsInput{
		Acquire: []TurnItemAcquire{{TemplateID: "implement.lower.common", Name: "雨痕剑", Source: "渡口铁匠交付", Description: "刃上留着雨丝般的纹路。"}},
		Equip:   []TurnItemEquip{{InstanceID: "雨痕剑"}},
	}, "generated-item")
	if len(result.Refused) != 0 || len(result.Acquired) != 1 || len(result.Equipped) != 1 {
		t.Fatalf("generated acquisition/equip: %+v", result)
	}
	after := appliedXiuxianState(t, state, ops)
	id := result.Acquired[0].DefID
	def, ok := storyItemCatalog(cultivation.ReferenceRules(),
		after).Definition(id)
	if !ok || def.ID == "implement.lower.common" || def.Implement == nil {
		t.Fatalf("generated definition unavailable: %+v", def)
	}
	if _, ok := storyItemCatalog(cultivation.ReferenceRules(),
		state).Definition(id); ok {
		t.Fatal("definition leaked to pre-registration branch")
	}
	base, _ := storyItemCatalog(cultivation.ReferenceRules()).Definition("implement.lower.common")
	if def.Implement.AttackBonus != base.Implement.AttackBonus || def.BasePrice != base.BasePrice {
		t.Fatal("generated equipment changed template mechanics")
	}
	if cost := xiuxianImplementOperatingCost(cultivation.ReferenceRules(),
		after, DefaultActorID); cost != base.OperatingManaCost(cultivation.Qi(1)) {
		t.Fatalf("operating cost %d", cost)
	}
	jobs := GeneratedDefinitionArtJobs(after, "ink")
	if len(jobs) != 1 || jobs[0].DefinitionID != id || jobs[0].Category != "equipment" {
		t.Fatalf("generated equipment art: %+v", jobs)
	}
}

// Synthetic domain integration: actual Store commits/replay, not AI gameplay.
func TestGeneratedContentCommitReloadForkAndRetry(t *testing.T) {
	store, id, root := talismanStory(t)
	before, _ := store.Snapshot(id, "main")
	originalSchema, _ := json.Marshal(before.ActorStateSchema)
	beforeBranch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: before.CurrentTurn.ID, Title: "生成前"})
	if err != nil {
		t.Fatal(err)
	}
	methodID, _ := generatedContentID("craft", "craft.talisman.ward", "松烟护符法")
	generated := talismanTurn(t, store, id, "main", "师父传授松烟护符并交付雨痕剑", &TurnResult{
		Skills: &TurnSkillLearningInput{Skills: []TurnSkillLearning{{TemplateID: "craft.talisman.ward", Name: "松烟护符法", Source: "符师传授", Description: "以松烟留一道回纹。"}, {TemplateID: "fire_bolt", Name: "松火诀", Source: "符师演示", Description: "火光如松针聚散。"}}},
		Items:  &TurnItemsInput{Acquire: []TurnItemAcquire{{TemplateID: "implement.lower.common", Name: "雨痕剑", Source: "铁匠交付", Description: "刃上有雨痕。"}}, Equip: []TurnItemEquip{{InstanceID: "雨痕剑"}}},
		Trade:  &TurnTradeInput{Merchant: "符铺", Buys: []TurnTradeBuy{{DefID: "material.talisman.paper", Name: "符纸", Quantity: 2}, {DefID: "material.talisman.ink", Name: "符墨", Quantity: 2}}},
	})
	afterGeneration, _ := store.Snapshot(id, "main")
	if jobs := NewGeneratedDefinitionArtJobs(before.State, afterGeneration.State, "ink"); len(jobs) != 3 {
		t.Fatalf("same-turn registrations lost: %+v", jobs)
	}
	view, err := store.CultivationEntryDetails(id, "main", "zh-CN")
	if err != nil || len(view.CraftMethods) != 1 || len(view.Items) != 3 || len(view.Skills) != 1 {
		t.Fatalf("owned details: %+v %v", view, err)
	}
	if view.Skills[0].Description != "火光如松针聚散。" || view.CraftMethods[0].Name != "松烟护符法" {
		t.Fatalf("generated prose replaced by template: %+v", view)
	}
	method, ok := storyCraftRegistryFromState(afterGeneration.ActorStateSchema.System, afterGeneration.State).Method(methodID)
	if !ok {
		t.Fatal("generated craft lost after item registration")
	}
	talismanTurn(t, store, id, "main", "按新法制符", &TurnResult{Craft: &TurnCraftInput{MethodID: method.ID, DurationSeconds: 4 * 60 * 60}})
	committed, _ := store.Snapshot(id, "main")
	beforeClock, _, _ := store.CultivationState(id, beforeBranch.ID)
	afterClock, _, _ := store.CultivationState(id, "main")
	if afterClock.Seconds <= beforeClock.Seconds {
		t.Fatal("generated craft did not spend real time")
	}
	if schema, _ := json.Marshal(committed.ActorStateSchema); !reflect.DeepEqual(schema, originalSchema) {
		t.Fatal("generation rewrote frozen author schema")
	}
	pre, _ := store.Snapshot(id, beforeBranch.ID)
	if jobs := GeneratedDefinitionArtJobs(pre.State, "ink"); len(jobs) != 0 {
		t.Fatal("generated content leaked to earlier branch")
	}
	postBranch, err := store.CreateBranch(id, CreateBranchRequest{ParentEventID: generated.ID, Title: "保留新法"})
	if err != nil {
		t.Fatal(err)
	}
	post, _ := store.Snapshot(id, postBranch.ID)
	if len(GeneratedDefinitionArtJobs(post.State, "ink")) != 3 {
		t.Fatal("later branch lost registrations")
	}
	// Optional art is persisted before a billable call and cannot execute twice.
	job := GeneratedDefinitionArtJobs(post.State, "ink")[0]
	for _, transition := range [][2]string{{"", "queued"}, {"queued", "executing"}, {"executing", "failed"}} {
		if admitted, err := store.TransitionDefinitionArt(id, postBranch.ID, generated.ID, job, transition[0], transition[1], nil); err != nil || !admitted {
			t.Fatalf("art admission: %v %v", admitted, err)
		}
	}
	_ = store.Close()
	reopened := NewStore(root)
	defer reopened.Close()
	cold, err := reopened.Snapshot(id, "main")
	if err != nil || !reflect.DeepEqual(cold.State, committed.State) {
		t.Fatalf("cold replay mismatch: %v", err)
	}
	if admitted, err := reopened.TransitionDefinitionArt(id, postBranch.ID, generated.ID, job, "", "queued", nil); err != nil || admitted {
		t.Fatalf("failed art automatically retried after reopen: %v %v", admitted, err)
	}
}

func TestGeneratedCraftMethodLearnsAndResolves(t *testing.T) {
	system := xiuxianSkillTestSystem()
	state := xiuxianSkillTestState(t, learnableStage(t), nil)
	ops, result, _ := xiuxianSkillLearningOps(state, system, DefaultActorID, TurnSkillLearningInput{Skills: []TurnSkillLearning{{
		TemplateID: "craft.talisman.ward", Name: "松烟护身符法", Source: "山居符师传授", Description: "以松烟画出守护的回纹。",
	}}}, "generated-craft")
	if len(result.Refused) != 0 || len(result.CraftMethods) != 1 {
		t.Fatalf("generated craft learning: %+v", result)
	}
	after := appliedXiuxianState(t, state, ops)
	method := result.CraftMethods[0]
	resolved, ok := storyCraftRegistryFromState(system, after).Method(method.ID)
	if !ok || !craftMethodKnown(after, DefaultActorID, resolved) {
		t.Fatal("generated method not registered and learned")
	}
	if _, ok := storyCraftRegistryFromState(system, state).Method(method.ID); ok {
		t.Fatal("method leaked to another branch")
	}
	if len(GeneratedDefinitionArtJobs(after, "ink")) != 1 {
		t.Fatal("generated method has no optional art job")
	}
}
