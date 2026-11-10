package interactive

import (
	"reflect"
	"testing"

	"ta/internal/character"
	"ta/internal/exploration"
	interactivestate "ta/internal/interactive/state"
)

// Pure planner fixtures use genuine realm records but deliberately construct an
// already known, present actor. They are not evidence of natural NPC dialogue.
func cooperationPlannerFixture(t *testing.T, project string) (*storyBranchProjection, StoryMeta, *TurnWorldActionInput, *TurnCharacterFactsInput) {
	t.Helper()
	d, _ := cooperationProject(project)
	def, _ := exploration.RealmDefinition(d.RealmID)
	items := map[string]int{}
	if d.PlayerItemID != "" {
		items[d.PlayerItemID] = d.PlayerItemQuantity
	}
	s, id, _ := realmExploreStory(t, def.PackageID, def.EntryLocationIDs[0], items, nil)
	entered := realmTestApply(t, s, id, realmExploreRequest("", d.RealmID, RealmActionEnter, "", "enter"))
	realmExploreDo(t, s, id, entered.InstanceID, d.RealmID, RealmActionSurvey, "survey")
	realmTestApply(t, s, id, realmExploreRequest(entered.InstanceID, d.RealmID, RealmActionExit, "", "exit"))
	p, meta := cooperationViewReadFixture(t, s, id)
	partner := character.Profile{ID: "partner", StoryID: id, SourceKey: d.PartnerSourceKey, Name: "Known partner", Revision: 1}
	p.Characters = &character.State{Version: character.StateVersion, Profiles: []character.Profile{partner}}
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storySceneLocationField, Value: d.LocationID})
	applyActorStateOp(p.State, ActorStateOp{Op: "set", ActorID: DefaultStoryContextActorID, FieldID: storyScenePresentField, Value: []any{"partner"}})
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
	if system.AdventureRules != nil {
		found := false
		for _, n := range system.AdventureRules.NPCs {
			if n.CharacterSourceKey != d.PartnerSourceKey {
				continue
			}
			a := n.Actor
			a.SourceKey = d.PartnerSourceKey
			for _, op := range adventureActorOps(a, "explicit-fixture") {
				op.ActorID = "partner"
				applyActorStateOp(p.State, op)
			}
			found = true
			break
		}
		if !found {
			t.Fatal("frozen NPC absent")
		}
	}
	p.State = cloneStoryState(p.State)
	input := &TurnWorldActionInput{Action: TurnWorldActionCooperation, SourceText: "我把调查线索告诉你，你愿意的话我们一起核对", Cooperation: &CooperationActionInput{ProjectID: project, InstanceID: entered.InstanceID, Action: "work_if_agreed", ShareDiscovery: true}}
	facts := &TurnCharacterFactsInput{Exchanges: []TurnCharacterExchangeInput{{CharacterID: "partner", Speaker: "character", Channel: "scene", Text: "愿意，我们按刚才说的分工核对。", CooperationDecision: &character.CooperationDecision{ProjectID: project, RealmInstanceID: entered.InstanceID, Decision: character.CooperationAccept}}}}
	return p, meta, input, facts
}

func TestCooperationPlannerPricesFrozenActorsAndKeepsInputs(t *testing.T) {
	for _, d := range cooperationProjects {
		t.Run(d.ID, func(t *testing.T) {
			p, meta, input, facts := cooperationPlannerFixture(t, d.ID)
			before := cloneStoryState(p.State)
			system := actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{})
			plan, err := planTurnWorldAction(p, p.State, meta, system, "main", "turn", input, facts)
			if err != nil {
				t.Fatal(err)
			}
			c := plan.cooperation
			if c.Outcome != "worked" || c.Matter.Cooperation.Phase != character.CooperationWorked || c.ClockAfter.Seconds-c.ClockBefore.Seconds != 600 || c.ShareStatement == "" || c.ShareSource == "" {
				t.Fatalf("incomplete plan: %+v", c)
			}
			if !reflect.DeepEqual(before, p.State) {
				t.Fatal("planner mutated committed state")
			}
			after := intervalProjectionAfterOps(p, plan.ops)
			if d.PartnerSkillID != "" {
				old, _ := AdventureActorFromState(p.State, "partner")
				next, _ := AdventureActorFromState(after.State, "partner")
				if old.Energy-next.Energy != 6 {
					t.Fatalf("not actual NPC Cost 6: %d -> %d", old.Energy, next.Energy)
				}
				player, _ := AdventureActorFromState(p.State, DefaultActorID)
				playerAfter, _ := AdventureActorFromState(after.State, DefaultActorID)
				if playerAfter.Energy != player.Energy {
					t.Fatal("player paid NPC energy")
				}
				if d.PlayerItemID != "" && player.Inventory[d.PlayerItemID]-playerAfter.Inventory[d.PlayerItemID] != 1 {
					t.Fatal("wire not consumed once")
				}
			}
		})
	}
}

func TestCooperationSubmissionRetainsRefusalButRequiresTruthfulNarrative(t *testing.T) {
	p, meta, input, facts := cooperationPlannerFixture(t, "coop.herbcave.notes")
	facts.Exchanges[0].CooperationDecision.Decision = character.CooperationRefuse
	validation := TurnSubmissionContext{ActorState: actorStateSystemFromSnapshot(meta.ActorStateSchema, StoryDirectorActorStateSystem{}), CurrentState: p.State, Meta: meta, BranchID: "main", CurrentPlayerInput: input.SourceText, KnownCharacters: []string{"partner"}, Scene: StorySceneFromState(p.State, "scene"), ChoiceCount: meta.ChoiceCount, WorldView: &TurnWorldView{Characters: p.Characters, Realms: p.Realms, Clock: p.Clock}}
	choices := testTurnChoices()
	updates := []interactivestate.Update{}
	prepared, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{World: input, CharacterFacts: facts, Choices: &choices, StateUpdates: &updates})
	if receipt.ModuleStatus.World != TurnSubmissionModuleAccepted || receipt.CooperationOutcome != "refused" || !receipt.NarrativeRewriteRequired || prepared.TurnResult().CharacterFacts == nil {
		t.Fatalf("refusal was lost or claimed as work: %+v", receipt)
	}
	plan, err := dryRunTurnWorldPlan(validation, input, facts)
	if err != nil || plan.clockSeconds() != 0 || len(plan.ops) != 0 || plan.cooperation.Matter != nil || plan.cooperation.ShareSource == "" {
		t.Fatalf("refusal cost or disclosure mismatch: %+v %v", plan, err)
	}
}

func TestCooperationCannotAcquireOwnQualificationInSameTurn(t *testing.T) {
	p, meta, input, facts := cooperationPlannerFixture(t, "coop.servicebay.markers")
	scene := cloneStoryState(p.State)
	player, _ := AdventureActorFromState(scene, DefaultActorID)
	delete(player.Inventory, "xinghai-i34")
	for _, op := range adventureActorOps(player, "fixture") {
		applyActorStateOp(scene, op)
	}
	// Proposed post-turn inventory has wire, committed input has none.
	plan, err := planCooperationAction(p, scene, meta, "main", "turn", input.Cooperation, facts)
	if err != nil || plan.Outcome != "missing_material" || plan.Matter == nil || plan.Matter.Cooperation.Phase != character.CooperationAgreed || len(plan.Ops) != 0 || plan.ClockAfter != nil {
		t.Fatalf("same-turn wire financed its own action: %+v %v", plan, err)
	}
	// An existing actor with damaged/incomplete canonical data is not healed from
	// the frozen catalog merely because the cooperation asks for their skill.
	npc, _ := AdventureActorFromState(p.State, "partner")
	npc.Energy = 5
	for _, op := range adventureActorOps(npc, "fixture") {
		op.ActorID = "partner"
		applyActorStateOp(p.State, op)
	}
	plan, err = planCooperationAction(p, p.State, meta, "main", "turn", input.Cooperation, facts)
	if err != nil || plan.Outcome != "partner_energy" || plan.ClockAfter != nil || len(plan.Ops) != 0 {
		t.Fatalf("insufficient NPC energy was repaired: %+v %v", plan, err)
	}
}

func TestCooperationRejectsWrongSpeakerScopeAndImplicitDisclosure(t *testing.T) {
	for _, bad := range []string{"player", "other", "project", "instance", "knowledge", "addressed"} {
		t.Run(bad, func(t *testing.T) {
			p, meta, input, facts := cooperationPlannerFixture(t, "coop.herbcave.notes")
			switch bad {
			case "player":
				facts.Exchanges[0].Speaker = "player"
			case "other":
				facts.Exchanges[0].CharacterID = "other"
			case "project":
				facts.Exchanges[0].CooperationDecision.ProjectID = "fake"
			case "instance":
				facts.Exchanges[0].CooperationDecision.RealmInstanceID = "other"
			case "knowledge":
				facts.Knowledge = []TurnCharacterKnowledgeInput{{CharacterID: "partner", Statement: "I witnessed everything", Nature: "experienced"}}
			case "addressed":
				facts.Exchanges[0].AddressedTo = "bystander"
			}
			if _, err := planCooperationAction(p, p.State, meta, "main", "turn", input.Cooperation, facts); err == nil {
				t.Fatal("unqualified response authorized work")
			}
		})
	}
	p, meta, input, facts := cooperationPlannerFixture(t, "coop.herbcave.notes")
	input.Cooperation.Action = "propose"
	input.Cooperation.ShareDiscovery = false
	plan, err := planCooperationAction(p, p.State, meta, "main", "turn", input.Cooperation, facts)
	if err != nil || plan.Matter != nil || plan.ShareSource != "" || plan.ClockAfter != nil {
		t.Fatalf("asking disclosed clue or issued plan: %+v %v", plan, err)
	}
}
