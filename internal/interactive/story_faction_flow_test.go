package interactive

import (
	"reflect"
	"testing"

	"ta/internal/character"
)

// These are explicit rule fixtures with real frozen recipes and NPC abilities,
// not evidence that a live language model naturally acquired their cooperation.
func factionFlowStory(t *testing.T, pack, recipeID, source string) (*Store, string, AdventureRecipe, AdventureSkill) {
	t.Helper()
	rules := authoredQuestRules(t, pack)
	var npc AdventureActor
	for _, n := range rules.NPCs {
		if n.CharacterSourceKey == source {
			npc = n.Actor
			npc.SourceKey = source
			break
		}
	}
	recipe, skill, err := factionRecipe(rules, npc, recipeID)
	if err != nil {
		t.Fatal(err)
	}
	player := NewAdventureActor(rules)
	for id, n := range recipe.Materials {
		player.Inventory[id] = n
	}
	values := func(a AdventureActor) map[string]any {
		state := map[string]any{}
		for _, op := range adventureActorOps(a, "faction-fixture") {
			state[op.FieldID] = op.Value
		}
		return state
	}
	system := StoryDirectorActorStateSystem{WorldContent: rules.world, AdventureRules: &rules, Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: AdventureStateFields(rules)},
		{ID: AdventureNPCTemplateID, Name: "本人", Fields: AdventureNPCStateFields(rules)},
		{ID: DefaultStoryContextActorID, Name: "场景", Fields: []ActorStateField{{Name: storySceneLocationField, Type: "string", Default: "共同工坊"}, listStateField("scene.present_actors", storyScenePresentField, "Present people", "当前场景")}},
	}, InitialActors: []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: values(player)},
		{ID: "artisan", Name: "已认识的匠人", TemplateID: AdventureNPCTemplateID, State: values(npc)},
		{ID: DefaultStoryContextActorID, Name: "场景", TemplateID: DefaultStoryContextActorID, State: map[string]any{storySceneLocationField: "共同工坊", storyScenePresentField: []any{"artisan"}}},
	}}
	s := NewStore(t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	story, err := s.CreateStory(CreateStoryRequest{Title: "Faction affair rule fixture", ActorState: &system, PackBinding: &StoryPackBinding{PackID: pack, ContentVersion: 3, Scope: pack}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{BranchID: "main", User: "我在工坊停下。", Narrative: "你在已认识的匠人身旁停下。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendCharacterFacts(story.ID, AppendCharacterFactsRequest{BranchID: "main", WorldTime: "现在", Profiles: []character.Profile{{ID: "artisan", Name: "已认识的匠人", SourceKey: source, Revision: 1}}}); err != nil {
		t.Fatal(err)
	}
	return s, story.ID, recipe, skill
}

func factionFlowRequest(t *testing.T, s *Store, id, branch, command, action, project, recipe string, hours int) AppendTurnWithStateRequest {
	t.Helper()
	text := "我自愿与他安排这件共同制作，材料由我提供，成品交给我。"
	if action == "work" {
		text = "我按已商定的分工做这一段工作，若他仍愿意就共同完成。"
	}
	if action == "cancel" {
		text = "我不再继续这件事务，已经投入的不必退还。"
	}
	identity := b1Identity(command, 1)
	intent, err := NewPlayerInputIntent(identity, branch, text)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitPlayerInput(id, intent); err != nil {
		t.Fatal(err)
	}
	f := &FactionActionInput{Action: action, ProjectID: project, Hours: hours}
	if action == "start" {
		f.GroupName = "工坊互助"
		f.PartnerID = "artisan"
		f.RecipeID = recipe
	}
	result := &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: TurnWorldActionFaction, SourceText: text, Faction: f}}
	if action != "cancel" {
		f.SourceExchangeIndex = 1
		result.CharacterFacts = &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "artisan", Speaker: "character", Channel: "scene", AddressedTo: DefaultActorID, Text: "我愿意按这件事的分工，与你做这一段。", ParticipationDecision: &character.ParticipationDecision{Kind: character.ParticipationFaction, TargetID: project, Decision: character.ParticipationAccept}}}}
	}
	return AppendTurnWithStateRequest{BranchID: branch, User: text, Narrative: "你们尊重彼此答复，按实际投入继续这件事务。", TurnResult: result, AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle}
}

func factionActor(t *testing.T, s *Store, id, branch, actor string) AdventureActor {
	t.Helper()
	snapshot, err := s.Snapshot(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	a, err := AdventureActorFromState(snapshot.State, actor)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func factionRecorded(t *testing.T, s *Store, id, branch, project string) character.Matter {
	t.Helper()
	state, err := s.CharacterState(id, branch)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range state.Matters {
		if m.Faction != nil && m.Faction.ProjectID == project {
			return m
		}
	}
	t.Fatalf("missing faction project %s", project)
	return character.Matter{}
}

func TestFactionAffairFormalProductionTwoWorlds(t *testing.T) {
	for _, tc := range []struct{ pack, recipe, source string }{{"lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15"}, {"xinghai-689", "xinghai-assemble-rifle", "xinghai-p26"}} {
		t.Run(tc.pack, func(t *testing.T) {
			s, id, recipe, skill := factionFlowStory(t, tc.pack, tc.recipe, tc.source)
			before, _ := s.Snapshot(id, "main")
			playerBefore := factionActor(t, s, id, "main", DefaultActorID)
			npcBefore := factionActor(t, s, id, "main", "artisan")
			view, err := s.WorldStateFor(id, "main")
			if err != nil || len(view.FactionAffairs.Recipes) == 0 {
				t.Fatalf("missing main-story recipe consumer: %+v %v", view.FactionAffairs, err)
			}
			start := factionFlowRequest(t, s, id, "main", "faction-start", "start", "first", recipe.ID, 0)
			started, _, err := s.AppendTurnWithState(id, start)
			if err != nil {
				t.Fatal(err)
			}
			snap, _ := s.Snapshot(id, "main")
			if SnapshotClockSeconds(snap) != SnapshotClockSeconds(before) || !reflect.DeepEqual(factionActor(t, s, id, "main", DefaultActorID), playerBefore) {
				t.Fatal("agreement spent time or resources")
			}
			partial := factionFlowRequest(t, s, id, "main", "faction-partial", "work", "first", "", 1)
			turn, _, err := s.AppendTurnWithState(id, partial)
			if err != nil {
				t.Fatal(err)
			}
			p := factionActor(t, s, id, "main", DefaultActorID)
			n := factionActor(t, s, id, "main", "artisan")
			m := factionRecorded(t, s, id, "main", "first")
			if m.Faction.WorkedHours != 1 || m.Faction.Phase != "active" || m.Faction.MaterialsSourceID == "" || m.Faction.OutputSourceID != "" {
				t.Fatalf("bad partial work: %+v", m)
			}
			for item, count := range recipe.Materials {
				if p.Inventory[item] != playerBefore.Inventory[item]-count {
					t.Fatal("materials were not spent exactly once")
				}
			}
			if p.Inventory[recipe.OutputID] != playerBefore.Inventory[recipe.OutputID] || p.Energy != playerBefore.Energy-1 || n.Energy != npcBefore.Energy-max(1, skill.Cost) {
				t.Fatal("partial work minted output, skill or energy")
			}
			snap, _ = s.Snapshot(id, "main")
			if SnapshotClockSeconds(snap) != SnapshotClockSeconds(before)+3600 {
				t.Fatal("work did not use the shared formal clock")
			}
			replay, _, err := s.AppendTurnWithState(id, partial)
			if err != nil || replay.ID != turn.ID {
				t.Fatalf("retry failed: %v", err)
			}
			if !reflect.DeepEqual(p, factionActor(t, s, id, "main", DefaultActorID)) {
				t.Fatal("retry spent resources twice")
			}
			fork, err := s.CreateBranch(id, CreateBranchRequest{ParentEventID: started.ID, Title: "Before work"})
			if err != nil {
				t.Fatal(err)
			}
			if got := factionRecorded(t, s, id, fork.ID, "first"); got.Faction.WorkedHours != 0 {
				t.Fatal("fork inherited future work")
			}
			finish := factionFlowRequest(t, s, id, "main", "faction-finish", "work", "first", "", recipe.Hours-1)
			if _, _, err = s.AppendTurnWithState(id, finish); err != nil {
				t.Fatal(err)
			}
			finished := factionActor(t, s, id, "main", DefaultActorID)
			m = factionRecorded(t, s, id, "main", "first")
			if finished.Inventory[recipe.OutputID] != playerBefore.Inventory[recipe.OutputID]+recipe.OutputCount || m.Faction.Phase != "completed" || m.Status != character.MatterSettled || m.Faction.OutputSourceID == "" {
				t.Fatal("completed production missing real output")
			}
			if !reflect.DeepEqual(finished.Skills, playerBefore.Skills) || finished.Experience != playerBefore.Experience {
				t.Fatal("cooperation taught or levelled player without learning")
			}
			s = cooperationFlowCold(t, s, id)
			if !reflect.DeepEqual(finished, factionActor(t, s, id, "main", DefaultActorID)) || !reflect.DeepEqual(m, factionRecorded(t, s, id, "main", "first")) {
				t.Fatal("cold journal replay changed output or work")
			}
			repeated := factionFlowRequest(t, s, id, "main", "faction-repeat", "work", "first", "", 1)
			if _, _, err = s.AppendTurnWithState(id, repeated); err == nil {
				t.Fatal("completed affair paid twice")
			}
			// Inherited agreement progresses independently on the fork with its own materials.
			forkWork := factionFlowRequest(t, s, id, fork.ID, "faction-fork-work", "work", "first", "", 1)
			if _, _, err = s.AppendTurnWithState(id, forkWork); err != nil {
				t.Fatal(err)
			}
			if factionRecorded(t, s, id, fork.ID, "first").Faction.WorkedHours != 1 || factionRecorded(t, s, id, "main", "first").Faction.Phase != "completed" {
				t.Fatal("fork work contaminated another branch")
			}
		})
	}
}

func TestFactionAffairRefusalMissingResourcesAndCancellation(t *testing.T) {
	s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
	req := factionFlowRequest(t, s, id, "main", "start", "start", "refused", recipe.ID, 0)
	req.TurnResult.CharacterFacts.Exchanges[0].ParticipationDecision.Decision = character.ParticipationRefuse
	if _, _, err := s.AppendTurnWithState(id, req); err == nil {
		t.Fatal("refusal became agreement")
	}
	req.TurnResult.World = nil
	req.Narrative = "他不愿参与，你接受了他的决定。"
	if _, _, err := s.AppendTurnWithState(id, req); err != nil {
		t.Fatalf("refusal could not remain ordinary story: %v", err)
	}
	req = factionFlowRequest(t, s, id, "main", "start-accepted", "start", "accepted", recipe.ID, 0)
	if _, _, err := s.AppendTurnWithState(id, req); err != nil {
		t.Fatal(err)
	}
	p, meta := cooperationViewReadFixture(t, s, id)
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	work := factionFlowRequest(t, s, id, "main", "checks", "work", "accepted", "", 1)
	for _, change := range []string{"absent", "no-energy", "no-materials", "no-skill", "refuse", "later-refuse", "false-speaker", "wrong-project"} {
		t.Run(change, func(t *testing.T) {
			scene := cloneStoryState(p.State)
			facts := cloneTurnCharacterFactsInput(work.TurnResult.CharacterFacts)
			actor, _ := AdventureActorFromState(scene, "artisan")
			player, _ := AdventureActorFromState(scene, DefaultActorID)
			switch change {
			case "absent":
				applyActorStateOp(scene, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{}})
			case "no-energy":
				actor.Energy = 0
			case "no-materials":
				for item := range recipe.Materials {
					player.Inventory[item] = 0
				}
			case "no-skill":
				delete(actor.Skills, recipe.SkillID)
			case "refuse":
				facts.Exchanges[0].ParticipationDecision.Decision = character.ParticipationRefuse
			case "later-refuse":
				e := facts.Exchanges[0]
				d := *e.ParticipationDecision
				d.Decision = character.ParticipationRefuse
				e.ParticipationDecision = &d
				facts.Exchanges = append(facts.Exchanges, e)
			case "false-speaker":
				facts.Exchanges[0].Speaker = "player"
			case "wrong-project":
				facts.Exchanges[0].ParticipationDecision.TargetID = "another"
			}
			for _, op := range adventureActorOps(actor, "test") {
				op.ActorID = "artisan"
				applyActorStateOp(scene, op)
			}
			for _, op := range adventureActorOps(player, "test") {
				applyActorStateOp(scene, op)
			}
			scene = cloneStoryState(scene)
			before := cloneStoryState(scene)
			if _, err := planFactionAction(p, scene, meta, system, "main", "turn", *work.TurnResult.World.Faction, facts); err == nil {
				t.Fatal("invalid work accepted")
			}
			if !reflect.DeepEqual(before, scene) {
				t.Fatal("refused work mutated inputs")
			}
		})
	}
	if _, _, err := s.AppendTurnWithState(id, work); err != nil {
		t.Fatal(err)
	}
	cancel := factionFlowRequest(t, s, id, "main", "cancel", "cancel", "accepted", "", 0)
	before := factionActor(t, s, id, "main", DefaultActorID)
	if _, _, err := s.AppendTurnWithState(id, cancel); err != nil {
		t.Fatal(err)
	}
	after := factionActor(t, s, id, "main", DefaultActorID)
	if !reflect.DeepEqual(before, after) || factionRecorded(t, s, id, "main", "accepted").Faction.Phase != "cancelled" {
		t.Fatal("cancellation refunded consumed goods or produced output")
	}
}

func TestFactionAffairRegenerationRestoresAllConsequences(t *testing.T) {
	for _, hours := range []int{1, 4} {
		t.Run(string(rune('0'+hours)), func(t *testing.T) {
			s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
			start := factionFlowRequest(t, s, id, "main", "start", "start", "regen", recipe.ID, 0)
			if _, _, err := s.AppendTurnWithState(id, start); err != nil {
				t.Fatal(err)
			}
			before, _ := s.Snapshot(id, "main")
			beforeChars, _ := s.CharacterState(id, "main")
			work := factionFlowRequest(t, s, id, "main", "work", "work", "regen", "", hours)
			worked, _, err := s.AppendTurnWithState(id, work)
			if err != nil {
				t.Fatal(err)
			}
			current, err := s.AdventureWindow(id, "main")
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err = s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", ExpectedParentID: &current.Head, ReplaceTurnID: worked.ID, User: "我先不做这段工作。", Narrative: "你把材料收好，暂时不开始。", TurnResult: &TurnResult{Choices: testTurnChoices()}}); err != nil {
				t.Fatal(err)
			}
			after, _ := s.Snapshot(id, "main")
			afterChars, _ := s.CharacterState(id, "main")
			if !reflect.DeepEqual(before.State, after.State) || SnapshotClockSeconds(before) != SnapshotClockSeconds(after) || !reflect.DeepEqual(beforeChars.Matters, afterChars.Matters) || !reflect.DeepEqual(beforeChars.Exchanges, afterChars.Exchanges) {
				t.Fatal("regeneration kept work, costs, output, clock or consent from discarded turn")
			}
			s = cooperationFlowCold(t, s, id)
			cold, _ := s.Snapshot(id, "main")
			coldChars, _ := s.CharacterState(id, "main")
			if !reflect.DeepEqual(after.State, cold.State) || !reflect.DeepEqual(afterChars, coldChars) {
				t.Fatal("cold replay resurrected discarded faction work")
			}
		})
	}
}

func TestFactionAffairOutputHasOrdinaryItemConsumer(t *testing.T) {
	s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
	for _, action := range []string{"start", "work"} {
		hours := 0
		if action == "work" {
			hours = recipe.Hours
		}
		req := factionFlowRequest(t, s, id, "main", action, action, "medicine", recipe.ID, hours)
		if action == "work" {
			req.TurnResult.World.Faction.RecipeID = ""
		}
		if _, _, err := s.AppendTurnWithState(id, req); err != nil {
			t.Fatal(err)
		}
	}
	produced := factionActor(t, s, id, "main", DefaultActorID)
	if !produced.Acquired[recipe.OutputID] {
		t.Fatal("produced item lacks actual acquisition provenance")
	}
	text := "我使用刚刚共同配制的金疮药。"
	if _, _, err := s.AppendTurnWithState(id, AppendTurnWithStateRequest{BranchID: "main", User: text, Narrative: "你取出实际配好的药，按普通用药规则使用。", TurnResult: &TurnResult{Choices: testTurnChoices(), World: &TurnWorldActionInput{Action: "adventure", SourceText: text, Adventure: &AdventureAction{Action: "use", ItemID: recipe.OutputID}}}}); err != nil {
		t.Fatal(err)
	}
	used := factionActor(t, s, id, "main", DefaultActorID)
	if used.Inventory[recipe.OutputID] != produced.Inventory[recipe.OutputID]-1 || !used.Acquired[recipe.OutputID] {
		t.Fatal("faction product bypassed or failed the ordinary item consumer")
	}
}

func TestFactionAffairRejectsMixedFieldsAndForgedSource(t *testing.T) {
	s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
	req := factionFlowRequest(t, s, id, "main", "source", "start", "source", recipe.ID, 0)
	req.TurnResult.World.SourceText = "模型自述，不是玩家授权"
	if _, _, err := s.AppendTurnWithState(id, req); err == nil {
		t.Fatal("forged player source accepted")
	}
	req.TurnResult.World.SourceText = req.User
	req.TurnResult.World.RouteID = "hidden-free-journey"
	if d := validateTurnWorldActionModule(req.TurnResult.World); d == nil {
		t.Fatal("mixed world fields accepted")
	}
	req.TurnResult.World.RouteID = ""
	req.TurnResult.World.Action = " faction "
	req.TurnResult.Items = &TurnItemsInput{Acquire: []TurnItemAcquire{{}}}
	if !factionModuleConflict(req.TurnResult) {
		t.Fatal("another inventory writer accepted")
	}
}

func TestFactionAffairBindsLastRepeatedPersonalAnswer(t *testing.T) {
	s, id, recipe, _ := factionFlowStory(t, "lanchuan-qunxia", "lanchuan-prepare-salve", "lanchuan-p15")
	req := factionFlowRequest(t, s, id, "main", "repeat-answer", "start", "last-answer", recipe.ID, 0)
	req.TurnResult.CharacterFacts.Exchanges = append(req.TurnResult.CharacterFacts.Exchanges, req.TurnResult.CharacterFacts.Exchanges[0])
	req.TurnResult.World.Faction.SourceExchangeIndex = 2
	if _, _, err := s.AppendTurnWithState(id, req); err != nil {
		t.Fatal(err)
	}
	state, err := s.CharacterState(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Exchanges) != 2 || factionRecorded(t, s, id, "main", "last-answer").Faction.ConsentExchangeID != state.Exchanges[1].ID {
		t.Fatal("identical repeated speech did not bind the final personal answer")
	}
}
