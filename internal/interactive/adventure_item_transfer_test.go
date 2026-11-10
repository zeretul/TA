package interactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ta/internal/character"
	"ta/internal/quest"
)

// Qualified isolated fixtures establish only the person, existing ownership and
// prerequisites. The actual author choice decides success and settles both sides.
func uniqueGiftProjection(t *testing.T) (AdventureRules, *storyBranchProjection, quest.Definition, quest.Instance) {
	t.Helper()
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	a := NewAdventureActor(r)
	a.Rank = 3
	a.Inventory["lanchuan-i54"] = 1
	a.Skills["lanchuan-s35"] = 2
	state := map[string]any{}
	for _, op := range adventureActorOps(a, "fixture") {
		setPath(state, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
	}
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storySceneLocationField), journeyPlaceLabel("lanchuan-l04"))
	setPath(state, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{"owner"})
	d, ok := quest.DefinitionByID("lanchuan-source-i04")
	if !ok {
		t.Fatal("missing actual authored gift")
	}
	i := quest.Instance{TaskID: d.ID, TaskVersion: d.Version, Status: quest.StatusAccepted}
	p := &storyBranchProjection{State: state, Characters: &character.State{Version: character.StateVersion, Profiles: []character.Profile{{ID: "owner", StoryID: "fixture", Name: "谷怀远", SourceKey: "lanchuan-p07", Revision: 1}}}, Quests: map[string]quest.Instance{d.ID: i, "lanchuan-a02": {Status: quest.StatusCompleted}}}
	return r, p, d, i
}

func TestAdventureUniqueNPCRewardCannotBeMintedOrConfiscated(t *testing.T) {
	r, p, d, i := uniqueGiftProjection(t)
	before, _ := json.Marshal(p)
	if _, err := AdventureRewardOps(p.State, StoryDirectorActorStateSystem{AdventureRules: &r}, 0, map[string]int{"lanchuan-i04": 1}, 0, "unproven"); err == nil {
		t.Fatal("generic payment duplicated another person's unique weapon")
	}
	for _, tc := range []struct {
		name  string
		alter func(*storyBranchProjection, *quest.Definition, *quest.Instance)
	}{
		{"absent", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) {
			setPath(p.State, actorStateFieldPath(DefaultStoryContextActorID, storyScenePresentField), []any{})
		}},
		{"unestablished", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) { p.Characters = nil }},
		{"different_giver", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) { d.Giver = "lanchuan-p14" }},
		{"no_promise_even_after_victory", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) {
			d.Giver = ""
			a, _ := AdventureActorFromState(p.State, DefaultActorID)
			a.Defeated["lanchuan-e01"] = true
			for _, op := range adventureActorOps(a, "") {
				setPath(p.State, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
			}
		}},
		{"unaccepted", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) { i.Status = quest.StatusOffered }},
		{"unfulfilled", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) {
			delete(p.Quests, "lanchuan-a02")
		}},
		{"book_missing", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) {
			a, _ := AdventureActorFromState(p.State, DefaultActorID)
			a.Inventory["lanchuan-i54"] = 0
			for _, op := range adventureActorOps(a, "") {
				setPath(p.State, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
			}
		}},
		{"owner_lost_item", func(p *storyBranchProjection, d *quest.Definition, i *quest.Instance) {
			var a AdventureActor
			for _, npc := range r.NPCs {
				if npc.CharacterSourceKey == "lanchuan-p07" {
					a = npc.Actor
				}
			}
			raw, _ := json.Marshal(a)
			a = AdventureActor{}
			_ = json.Unmarshal(raw, &a)
			a.Inventory["lanchuan-i04"] = 0
			delete(a.Equipment, "person:weapon")
			for _, op := range actorOpsForAdventureOwner(a, "owner", "") {
				setPath(p.State, actorStateFieldPath(op.ActorID, op.FieldID), op.Value)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, p, d, i := uniqueGiftProjection(t)
			tc.alter(p, &d, &i)
			before, _ := json.Marshal(p)
			_, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: "lanchuan-source-i04.work"})
			if err == nil {
				t.Fatal("unproved handover accepted")
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) {
				t.Fatal("refusal mutated money, ownership or scene")
			}
		})
	}
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("generic refusal mutated state")
	}
	// The separate sword is not a gift merely because its owner can be fought.
	if _, err := AdventureRewardOps(p.State, StoryDirectorActorStateSystem{AdventureRules: &r}, 0, map[string]int{"lanchuan-i10": 1}, 0, d.ID); err == nil {
		t.Fatal("hearing a reward name confiscated the other unique sword")
	}
	_ = i
}

func TestAdventureUniqueNPCGiftDebitsEquipmentAtomically(t *testing.T) {
	r, p, d, i := uniqueGiftProjection(t)
	before, _ := json.Marshal(p)
	rulesBefore, _ := json.Marshal(r)
	plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: "lanchuan-source-i04.work"})
	if err != nil {
		t.Fatal(err)
	}
	view := questProjectionAfter(p, plan)
	a, err := AdventureActorFromState(view.State, DefaultActorID)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := AdventureActorFromState(view.State, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if a.Inventory["lanchuan-i04"] != 1 || !a.Acquired["lanchuan-i04"] || owner.Inventory["lanchuan-i04"] != 0 || owner.Equipment["person:weapon"] == "lanchuan-i04" || !owner.Acquired["lanchuan-i04"] {
		t.Fatal("gift did not debit the actual owner and credit player together")
	}
	if plan.ClockAfter.Seconds != 7200 || plan.Instance.Choices[d.Choices[0].ObjectiveID].Outcome != "success" {
		t.Fatal("gift bypassed task effort or result")
	}
	after, _ := json.Marshal(p)
	rulesAfter, _ := json.Marshal(r)
	if string(before) != string(after) || string(rulesBefore) != string(rulesAfter) {
		t.Fatal("planning mutated source branch or frozen author inventory")
	}
	if _, err := planQuestChoice(view, StoryDirectorActorStateSystem{AdventureRules: &r}, d, plan.Instance, true, QuestRequest{ChoiceID: "lanchuan-source-i04.work"}); err == nil {
		t.Fatal("same gift paid twice")
	}
	actorID, encounter, err := adventureEncounterActor(r, view, AdventureAction{Action: "battle_start", OpponentID: "lanchuan-e01"})
	if err != nil || actorID != "owner" || encounter.Inventory["lanchuan-i04"] != 0 || encounter.Equipment["person:weapon"] != "" {
		t.Fatalf("next actual encounter restored gifted weapon: %v %+v", err, encounter)
	}
	// New ownership semantics never rewrite a frozen pre-ownership world.
	for n := range r.NPCs {
		if r.NPCs[n].CharacterSourceKey == "lanchuan-p07" {
			delete(r.NPCs[n].Actor.Inventory, "lanchuan-i04")
			delete(r.NPCs[n].Actor.Equipment, "person:weapon")
		}
	}
	p.Characters = nil
	if _, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: "lanchuan-source-i04.work"}); err != nil {
		t.Fatalf("old frozen reward source was replaced by current ownership: %v", err)
	}
}

func TestAdventureUniqueNPCGiftSurvivesRealQuestBranchAndColdOpen(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	a := NewAdventureActor(r)
	a.Rank = 3
	a.Skills["lanchuan-s34"] = 2
	a.Skills["lanchuan-s35"] = 2
	initial := map[string]any{}
	for _, op := range adventureActorOps(a, "") {
		initial[op.FieldID] = op.Value
	}
	location := journeyPlaceLabel("lanchuan-l03")
	system := StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, "lanchuan-qunxia"), AdventureRules: &r, Templates: []ActorStateTemplate{{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(r)}, {ID: AdventureNPCTemplateID, Name: "人物", Fields: AdventureNPCStateFields(r)}, {ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string"}, {Name: storyScenePresentField, Type: "list"}}}}, InitialActors: []ActorStateInitialActor{{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: initial}, {ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: location}}}}
	s := NewStore(t.TempDir())
	defer s.Close()
	meta, err := s.CreateStory(CreateStoryRequest{Title: "Agreed unique gift", ActorState: &system, PackBinding: &StoryPackBinding{PackID: "lanchuan-qunxia", ContentVersion: 5, Scope: "lanchuan-qunxia"}, PackCatalog: &PackCatalogSnapshot{ContentVersion: 5, Characters: []PackCatalogPerson{{SourceKey: "lanchuan-p07", Name: "谷怀远"}}}})
	if err != nil {
		t.Fatal(err)
	}
	id := meta.ID
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "询问渔场", Narrative: "听到当地的滩涂争执。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "lanchuan-a02", QuestActionAccept, "", "accept-dispute")
	questApplyOn(t, s, id, "main", "lanchuan-a02", QuestActionChoose, "lanchuan-a02.c2", "open-fishing-ground")
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "前往青盐滩见谷怀远", Narrative: "来到约定的青盐滩，谷怀远已在眼前。", ActorOps: []ActorStateOp{{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: journeyPlaceLabel("lanchuan-l04")}, {Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"owner"}}}, TurnResult: &TurnResult{Choices: testTurnChoices(), CharacterFacts: &TurnCharacterFactsInput{Profiles: []TurnCharacterProfileInput{{CharacterID: "owner", Name: "谷怀远", SourceKey: "lanchuan-p07"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	questApplyOn(t, s, id, "main", "lanchuan-source-i04", QuestActionAccept, "", "accept-gift")
	// Branches fork at a committed story turn, not at a standalone quest row.
	_, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "先核对约定与账本", Narrative: "账本仍在手中，赠刀尚未交付。", TurnResult: &TurnResult{Choices: testTurnChoices()}})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.Actor.Inventory["lanchuan-i54"] != 1 || before.Actor.Inventory["lanchuan-i04"] != 0 {
		t.Fatal("gift prerequisites were not earned from actual earlier choice")
	}
	fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: before.Head, Title: "Before handover"})
	if err != nil {
		t.Fatal(err)
	}
	result := questApplyOn(t, s, id, "main", "lanchuan-source-i04", QuestActionChoose, "lanchuan-source-i04.work", "gift-once")
	replay := questApplyOn(t, s, id, "main", "lanchuan-source-i04", QuestActionChoose, "lanchuan-source-i04.work", "gift-once")
	if !replay.Replayed || result.EventID != replay.EventID {
		t.Fatal("retry did not reuse the exact transaction")
	}
	after, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := s.StoryContext(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := AdventureActorFromState(ctx.Snapshot.State, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if after.Actor.Inventory["lanchuan-i04"] != 1 || owner.Inventory["lanchuan-i04"] != 0 || owner.Equipment["person:weapon"] != "" {
		t.Fatal("journal did not settle both owners")
	}
	if _, err = s.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "lanchuan-source-i04", Action: QuestActionChoose, ChoiceID: "lanchuan-source-i04.work"}); err == nil {
		t.Fatal("second command reissued the gift")
	}
	cold := NewStore(s.Root())
	defer cold.Close()
	restored, err := cold.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, after) {
		t.Fatal("cold reopen changed the gift or receipt")
	}
	old, err := cold.AdventureWindow(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Actor, before.Actor) {
		t.Fatal("handover leaked into pre-gift branch")
	}
	// The narrator's main-turn path must commit the same two-sided transfer,
	// not only the standalone quest endpoint used on the original branch.
	_, _, err = cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: fork.ID, User: "按约请谷怀远交付断潮刀", Narrative: "核对账本与新渔场后，谷怀远解下断潮刀交给你。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "lanchuan-source-i04", QuestAction: QuestActionChoose, QuestChoiceID: "lanchuan-source-i04.work", SourceText: "按约请谷怀远交付断潮刀"}}})
	if err != nil {
		t.Fatal(err)
	}
	forkAfter, err := cold.AdventureWindow(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkContext, err := cold.StoryContext(id, fork.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkOwner, err := AdventureActorFromState(forkContext.Snapshot.State, "owner")
	if err != nil || forkAfter.Actor.Inventory["lanchuan-i04"] != 1 || forkOwner.Inventory["lanchuan-i04"] != 0 || forkOwner.Equipment["person:weapon"] != "" {
		t.Fatal("main-turn gift lost a side of the ownership transfer")
	}
	_, _, err = cold.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: "请谷怀远切磋", Narrative: "他不再持有赠出的断潮刀。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: "请谷怀远切磋", Adventure: &AdventureAction{Action: "battle_start", OpponentID: "lanchuan-e01"}}}})
	if err != nil {
		t.Fatal(err)
	}
	combat, err := cold.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if combat.Actor.Battle.EnemyInventory["lanchuan-i04"] != 0 || strings.Contains(combat.Actor.Battle.EnemyEquipment["person:weapon"], "i04") {
		t.Fatal("cold encounter resurrected the original owner's weapon")
	}
}

func TestAdventureUniqueOwnershipBlocksDuplicateCatalogAndCraft(t *testing.T) {
	r := authoredAdventureRules(t, "lanchuan-qunxia")
	r.StartingItems["lanchuan-i04"] = 1
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "multiple initial owners") {
		t.Fatal("catalog granted the same relic to player and NPC")
	}
	r = authoredAdventureRules(t, "lanchuan-qunxia")
	recipe := r.Recipes[0]
	recipe.ID = "forged-unique-copy"
	recipe.OutputID = "lanchuan-i04"
	recipe.OutputCount = 1
	r.Recipes = append(r.Recipes, recipe)
	a := NewAdventureActor(r)
	a.Skills[recipe.SkillID] = recipe.SkillRank
	before, _ := json.Marshal(a)
	_, _, err := ResolveAdventureAction(r, a, AdventureAction{Action: "craft", RecipeID: recipe.ID, Hours: recipe.Hours}, "copy-attempt")
	if err == nil || !strings.Contains(err.Error(), "reserved to an actual owner") {
		t.Fatalf("crafting could duplicate a reserved unique: %v", err)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("rejected copy spent real resources")
	}
}

func TestAdventureUniqueGiftEffortDoesNotDiscardOwnerDebit(t *testing.T) {
	r, p, d, i := uniqueGiftProjection(t)
	for n := range r.Skills {
		if r.Skills[n].ID == "lanchuan-s35" {
			r.Skills[n].Cost = 7
		}
	}
	d.Choices[0].UseSkillID = "lanchuan-s35"
	a, _ := AdventureActorFromState(p.State, DefaultActorID)
	plan, err := planQuestChoice(p, StoryDirectorActorStateSystem{AdventureRules: &r}, d, i, true, QuestRequest{ChoiceID: d.Choices[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	view := questProjectionAfter(p, plan)
	got, _ := AdventureActorFromState(view.State, DefaultActorID)
	owner, err := AdventureActorFromState(view.State, "owner")
	if err != nil || got.Energy != a.Energy-7 || got.Inventory["lanchuan-i04"] != 1 || owner.Inventory["lanchuan-i04"] != 0 || owner.Equipment["person:weapon"] != "" {
		t.Fatal("effort write replaced the simultaneous NPC ownership write")
	}
}

func TestAdventureOwnedCatalogRetainsTasksWithoutProgramRevisionWhitelist(t *testing.T) {
	for _, world := range []string{"lanchuan-qunxia", "xinghai-689"} {
		current := StoryMeta{PackBinding: &StoryPackBinding{PackID: world, ContentVersion: 5}, ActorStateSchema: &ActorStateSchemaSnapshot{System: StoryDirectorActorStateSystem{WorldContent: authoredWorldContent(t, world)}}}
		if !storyPackAllows(current, world, "1") {
			t.Fatal("owned content refused")
		}
		current.PackBinding.ContentVersion = 999
		if !storyPackAllows(current, world, "1") {
			t.Fatal("revision whitelist blocked owned content")
		}
		current.ActorStateSchema = nil
		if storyPackAllows(current, world, "1") {
			t.Fatal("missing snapshot borrowed current content")
		}
	}
}
