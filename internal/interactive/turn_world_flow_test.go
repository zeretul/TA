package interactive

import (
	"testing"

	interactivestate "ta/internal/interactive/state"
)

// These tests are PRIORITY 1 of the 2026-10-18 batch as it happens during play:
// the player writes an ordinary sentence — 去坊市, 接下这桩差事, 看看灵潮, 进洞府看看 —
// and the same committed turn both publishes the prose and settles the world
// fact against the rules the entry paths use.
//
// What they assert is the part that cannot be seen from the module's own tests:
// that the conditions (where the character stands, what the world offers), the
// formal time, the state writes and the side record all land in one transaction,
// and that a refusal leaves the prose committed and the world where it was
// rather than publishing a success nobody settled.

// worldFlowStory builds a story whose scene places the protagonist at the given
// registered place, with one committed opening turn to parent the first world
// record to.
func worldFlowStory(t *testing.T, location string) (*Store, string) {
	t.Helper()
	system := journeyTestSystem(location)
	store := NewStore(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(CreateStoryRequest{Title: "世界行动", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "出门", Narrative: "主角收拾停当，站在门口看了一会儿天色。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// worldFlowTurn commits one turn whose only module is the world action.
func worldFlowTurn(t *testing.T, store *Store, storyID, text, narrative string, world TurnWorldActionInput) TurnEvent {
	t.Helper()
	return worldFlowTurnWithState(t, store, storyID, text, narrative, world, nil)
}

// worldFlowTurnWithState commits the same turn with the narrator's own
// state_changes, which is what a real model sends: prose, choices and the
// scene fields it believes the scene now has.
func worldFlowTurnWithState(t *testing.T, store *Store, storyID, text, narrative string, world TurnWorldActionInput, updates []interactivestate.Update) TurnEvent {
	t.Helper()
	turn, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: text, Narrative: narrative,
		TurnResult: &TurnResult{Choices: testTurnChoices(), World: &world, StateUpdates: updates},
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

// worldFlowClock reads the branch's one formal instant.
func worldFlowClock(t *testing.T, store *Store, storyID string) StoryClock {
	t.Helper()
	clock, _, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	return clock
}

// TestNaturalInputWalksToTheMarket is 去坊市 as the player would write it: the
// turn's world action resolves the authored road, spends its own 30 minutes on
// the one formal clock, writes where the character now stands, and publishes one
// journey record.
func TestNaturalInputWalksToTheMarket(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	before := worldFlowClock(t, store, storyID)

	worldFlowTurn(t, store, storyID, "去坊市看看", "主角顺着土路往坊市走，午前到了坊市口。", TurnWorldActionInput{
		Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true,
	})

	after := worldFlowClock(t, store, storyID)
	if after.Seconds-before.Seconds != 30*60 {
		t.Fatalf("the road must spend exactly its own 30 minutes: before=%d after=%d", before.Seconds, after.Seconds)
	}
	if got := journeyTestLocation(t, store, storyID, "main"); got != "公共坊市" {
		t.Fatalf("the turn must write where the character really stands: %q", got)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("one turn of travel must publish one journey record: %d", count)
	}
	_, journeys, err := store.JourneyState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	finished := 0
	for _, snapshot := range journeys {
		if journeyFinished(snapshot) {
			finished++
		}
	}
	if finished != 1 || len(journeys) != 1 {
		t.Fatalf("the journey must read back finished: %#v", journeys)
	}
	// The next turn's own page now offers the road back from where they stand,
	// which is what makes the arrival a fact rather than a sentence.
	state, err := store.WorldStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Journeys.Routes) != 1 || state.Journeys.Routes[0].RouteID != "route_036:forward" {
		t.Fatalf("the market's own road must be offered from the market: %#v", state.Journeys)
	}
	if state.Location != "公共坊市" {
		t.Fatalf("the world page must report the committed place: %q", state.Location)
	}
}

// TestNaturalInputTakesAnErrand is 接下这桩差事: the task is accepted where the
// character stands, the acceptance is on the journal, and the turn's own state
// writes do not pay anything — payment happens when the work is settled.
func TestNaturalInputTakesAnErrand(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	before := worldFlowClock(t, store, storyID)
	beforeStones := questTestCurrency(t, store, storyID)

	worldFlowTurn(t, store, storyID, "这桩差事我接了", "主角听人说完，答应去古修洞府里取回那只封口药瓶。", TurnWorldActionInput{
		Action: TurnWorldActionQuest, TaskID: "quest.herb_handover", QuestAction: QuestActionAccept,
	})

	state := questTestState(t, store, storyID)
	view := questTestTask(t, state, "quest.herb_handover")
	if view.Status != "accepted" {
		t.Fatalf("the task must read back taken: %#v", view)
	}
	if count := questTestRecordCount(t, store, storyID); count != 1 {
		t.Fatalf("one acceptance must publish one task record: %d", count)
	}
	// Accepting is not doing: the objective is still open and nothing is paid.
	if view.Objectives[0].Completed {
		t.Fatalf("taking a task must not tick its objective: %#v", view.Objectives)
	}
	if after := questTestCurrency(t, store, storyID); after != beforeStones {
		t.Fatalf("accepting must not pay: before=%d after=%d", beforeStones, after)
	}
	// A task decision is not a stretch of road; the formal clock stays put.
	if after := worldFlowClock(t, store, storyID); after.Seconds != before.Seconds {
		t.Fatalf("taking a task must not spend the clock: before=%d after=%d", before.Seconds, after.Seconds)
	}
}

// TestNaturalInputJoinsTheEventInFrontOfThem is 看看灵潮 in the market. The
// participation is recorded at the formal minute the turn stands on, its reward
// is settled once, and the event's stage is read from that same clock.
func TestNaturalInputJoinsTheEventInFrontOfThem(t *testing.T) {
	store, storyID := worldFlowStory(t, "公共坊市")
	before := questTestCurrency(t, store, storyID)

	worldFlowTurn(t, store, storyID, "去帮他们一把", "主角挤进人群，帮着把被挤倒的药材筐扶起来。", TurnWorldActionInput{
		Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide",
		EventAction: PublicEventActionJoin, ParticipationAction: "help",
	})

	state, err := store.PublicEventStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Events) != 1 {
		t.Fatalf("the joined event must be in play: %#v", state.Events)
	}
	event := state.Events[0]
	if !event.Joined || event.Withdrawn || event.Participation != "help" {
		t.Fatalf("the participation must read back as joined by helping: %#v", event)
	}
	if after := questTestCurrency(t, store, storyID); after <= before {
		t.Fatalf("helping must settle its declared reward once: before=%d after=%d", before, after)
	}
	if count := b1RecordCount(t, store, storyID, StoryEventTypePublicEventRecord); count != 1 {
		t.Fatalf("one participation must publish one event record: %d", count)
	}
}

// TestNaturalInputEntersTheRealm is 进洞府看看. The instance is created at the
// declared entrance, the character's place becomes the realm, and a task
// objective waiting on a realm reward is not ticked by entering alone.
func TestNaturalInputEntersTheRealm(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")

	worldFlowTurn(t, store, storyID, "进洞府看看", "主角拨开枯藤，顺着塌了半边的洞口走进去。", TurnWorldActionInput{
		Action: TurnWorldActionRealm, RealmID: "realm.gu_xiu_dong_fu", RealmAction: RealmActionEnter,
	})

	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 {
		t.Fatalf("entering must create one instance: %#v", state.Instances)
	}
	instance := state.Instances[0]
	if instance.Presence != "inside" || instance.View.CurrentArea.ID != "realm.outer_hall" {
		t.Fatalf("the character must stand in the declared entry area: %#v", instance)
	}
	if count := realmTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("one entry must publish one realm record: %d", count)
	}
	// Nothing deeper is known, and nothing is taken by walking in.
	if len(instance.View.Areas) != 1 || len(instance.Granted) != 0 {
		t.Fatalf("entering must reveal only the entry and grant nothing: %#v", instance)
	}
}

// TestANarratedArrivalDoesNotMoveTheCharacterBeforeTheRulesDo is the defect
// live play found, written down as a test.
//
// A model asked to 进洞府看看 writes the whole scene: the prose walks the
// character in, and the state_changes describe the hall they are now standing
// in. The rules then judge the entrance — and if they judge it against that
// write, the character is "already inside", the registered entrance is nowhere
// in the sentence, and a legal entry is refused after the prose has been
// committed. That is a turn that claims success and settles nothing.
//
// A precondition is about where the turn found the character. The narrator's
// description of the result cannot be the evidence for its own permission.
func TestANarratedArrivalDoesNotMoveTheCharacterBeforeTheRulesDo(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")

	worldFlowTurnWithState(t, store, storyID, "进洞府看看",
		"主角拨开枯藤，顺着塌了半边的洞口走进外殿。",
		TurnWorldActionInput{
			Action: TurnWorldActionRealm, RealmID: "realm.gu_xiu_dong_fu", RealmAction: RealmActionEnter,
		},
		[]interactivestate.Update{{
			Op: interactivestate.Replace, Path: "/story/" + storySceneLocationField,
			Value: "越国城西废药圃地下的古修洞府外殿",
		}},
	)

	state, err := store.RealmStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Instances) != 1 {
		t.Fatalf("a legal entrance must not be refused by the narrator's own description: %#v", state.Instances)
	}
	if state.Instances[0].Presence != "inside" || state.Instances[0].View.CurrentArea.ID != "realm.outer_hall" {
		t.Fatalf("the character must stand in the declared entry area: %#v", state.Instances[0])
	}
	// The entrance is remembered as where they came from, so leaving puts them
	// back on the map rather than inside the cave's own wording.
	if origin := state.Instances[0].OriginLocation; origin != "越国坊市外围散修人家" {
		t.Fatalf("the instance must remember the committed place it was entered from: %q", origin)
	}
}

// TestARoadTurnCannotBeGivenAFalseStartingPoint is the same protection for a
// road, arriving by the other door.
//
// A journey turn settles the scene location itself, so the narrator is not
// allowed to write it at all: the refusal happens in validation, before the road
// is ever planned. That is why the road never needed the committed-state split —
// but it is worth pinning, because if the field ever became narratable again the
// origin check would start reading the narrator's own claim.
func TestARoadTurnCannotBeGivenAFalseStartingPoint(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	before := worldFlowClock(t, store, storyID)

	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "回坊市去", Narrative: "主角到了坊市，站在巷口。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			World: &TurnWorldActionInput{
				Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true,
			},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/" + storySceneLocationField,
				Value: "公共坊市",
			}},
		},
	})
	if err == nil {
		t.Fatal("a road turn must not accept the narrator's own scene location")
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 0 {
		t.Fatalf("a rejected turn must publish no journey record: %d", count)
	}
	if after := worldFlowClock(t, store, storyID); after.Seconds != before.Seconds {
		t.Fatalf("a rejected turn must not spend the clock: before=%d after=%d", before.Seconds, after.Seconds)
	}
}

// TestANarratedArrivalDoesNotJoinAnEventFromElsewhere is the same split for a
// public event. The event belongs to the market; the character is at the
// outskirts, and a sentence that says they are in the crowd is not a place.
func TestANarratedArrivalDoesNotJoinAnEventFromElsewhere(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	before := questTestCurrency(t, store, storyID)
	beforeLocation := journeyTestLocation(t, store, storyID, "main")

	// The narrator wrote the move into the market and joined an event that
	// belongs to another region. The prose cannot be the evidence for its own
	// permission, so the turn must fail rather than publish the arrival.
	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "去帮他们一把",
		Narrative: "主角挤进坊市人群，帮着扶起被撞倒的药材筐。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			World: &TurnWorldActionInput{
				Action: TurnWorldActionPublicEvent, EventID: "evt.taiyue_spirit_tide",
				EventAction: PublicEventActionJoin, ParticipationAction: "help",
			},
			StateUpdates: []interactivestate.Update{{
				Op: interactivestate.Replace, Path: "/story/" + storySceneLocationField,
				Value: "公共坊市",
			}},
		},
	})
	if err == nil {
		t.Fatal("joining an event of another region must fail the turn")
	}

	if count := b1RecordCount(t, store, storyID, StoryEventTypePublicEventRecord); count != 0 {
		t.Fatalf("an event of another region must not be joined: %d", count)
	}
	if after := questTestCurrency(t, store, storyID); after != before {
		t.Fatalf("a refused participation must pay nothing: before=%d after=%d", before, after)
	}
	// The state writes that described the arrival went with the refused world
	// action; nothing of that success may survive in the committed state.
	if got := journeyTestLocation(t, store, storyID, "main"); got != beforeLocation {
		t.Fatalf("a refused participation must not move the character: %q", got)
	}
}

// worldFlowSubmissionContext builds the validation context the app builds for
// this story: the committed state, the director's actor system and the world
// records the commit would read.
func worldFlowSubmissionContext(t *testing.T, store *Store, storyID string) TurnSubmissionContext {
	t.Helper()
	storyCtx, err := store.StoryContext(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	director := store.storyDirectorForMeta(storyCtx.Meta)
	view, err := store.TurnWorldView(storyID, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	return TurnSubmissionContext{
		ActorState:   actorStateSystemFromSnapshot(storyCtx.Meta.ActorStateSchema, director.ActorState),
		CurrentState: storyCtx.Snapshot.State,
		ChoiceCount:  storyCtx.Meta.ChoiceCount,
		Meta:         storyCtx.Meta,
		BranchID:     "main",
		WorldView:    &view,
	}
}

// TestSubmissionRefusesAWorldActionTheCommitWouldRefuse is the dry run that
// makes the refusal useful.
//
// Without it the model's prose is accepted first and the world planner refuses
// second, after the narrative is locked. Here the same illegal road is offered
// at submission time: the world module is rejected, the state writes that
// described the walk are dropped with it, and the receipt asks for a rewrite —
// all while the model can still produce one.
func TestSubmissionRefusesAWorldActionTheCommitWouldRefuse(t *testing.T) {
	store, storyID := worldFlowStory(t, "公共坊市")
	validation := worldFlowSubmissionContext(t, store, storyID)
	choices := testTurnChoices()
	world := TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true}
	updates := []interactivestate.Update{{
		Op: interactivestate.Replace, Path: "/story/" + storySceneLocationField,
		Value: "越国坊市外围散修人家",
	}}
	_, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{World: &world, Choices: &choices, StateUpdates: &updates})

	if receipt.ModuleStatus.World == TurnSubmissionModuleAccepted {
		t.Fatalf("the submission must refuse the same road the commit refuses: %#v", receipt)
	}
	if receipt.ModuleStatus.StateChanges == TurnSubmissionModuleAccepted {
		t.Fatalf("the state writes that describe the refused walk must be dropped with it: %#v", receipt)
	}
	if !receipt.NarrativeRewriteRequired || receipt.NarrativeRewriteReason == "" {
		t.Fatalf("a refused world action must ask for new prose: %#v", receipt)
	}
}

// TestAnOmittedWorldModuleIsMissingNotAccepted is the defect that read as a
// success: the model narrated taking an errand but sent no world module, and the
// receipt said the turn was accepted because "no world action" and "no decision
// at all" looked the same.
//
// A real turn has a committed world view, so it must say what it settles — an
// action, or action=none. Omitting the module is a missing submission, never an
// acceptance, and the turn stays open until the model sends the decision.
func TestAnOmittedWorldModuleIsMissingNotAccepted(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	validation := worldFlowSubmissionContext(t, store, storyID)
	choices := testTurnChoices()
	updates := []interactivestate.Update{{
		Op: interactivestate.Replace, Path: "/story/" + storySceneEventField,
		Value: "主角答应去古修洞府取回药瓶",
	}}

	_, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})

	if receipt.Ready {
		t.Fatalf("a turn that never decided its world action must not be ready: %#v", receipt)
	}
	if receipt.ModuleStatus.World != TurnSubmissionModuleMissing {
		t.Fatalf("an omitted world module must read missing, not %q: %#v", receipt.ModuleStatus.World, receipt)
	}
	if !turnWorldFlowContains(receipt.MissingModules, TurnSubmissionModuleWorld) || !turnWorldFlowContains(receipt.RetryModules, TurnSubmissionModuleWorld) {
		t.Fatalf("the receipt must ask for the world module by name: %#v", receipt)
	}

	// The same turn settles no world action, said explicitly, and is accepted.
	none := TurnWorldActionInput{Action: TurnWorldActionNone}
	_, receipt = PrepareTurnSubmission(validation, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices, World: &none})
	if !receipt.Ready || receipt.ModuleStatus.World != TurnSubmissionModuleAccepted {
		t.Fatalf("action=none must be an accepted decision: %#v", receipt)
	}
}

// TestATurnThatOmitsTheTaskItNarratesSettlesNothing is the same defect as it
// reaches the journal: prose that accepts an errand, state writes that say so,
// and no world module. Committing that would publish an acceptance the world
// never recorded, so the turn is not committed and the task stays untaken.
func TestATurnThatOmitsTheTaskItNarratesSettlesNothing(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	validation := worldFlowSubmissionContext(t, store, storyID)
	choices := testTurnChoices()
	updates := []interactivestate.Update{{
		Op: interactivestate.Replace, Path: "/story/" + storySceneEventField,
		Value: "主角接下了药瓶差事",
	}}
	_, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices})
	if receipt.Ready {
		t.Fatalf("an omitted world module must not commit the narrated acceptance: %#v", receipt)
	}

	state := questTestState(t, store, storyID)
	for _, view := range state.Tasks {
		if view.TaskID == "quest.herb_handover" && view.Status == "accepted" {
			t.Fatalf("the prose alone must not take the task: %#v", view)
		}
	}

	// Sending the acceptance the prose described settles it exactly once.
	world := TurnWorldActionInput{Action: TurnWorldActionQuest, TaskID: "quest.herb_handover", QuestAction: QuestActionAccept}
	_, receipt = PrepareTurnSubmission(validation, nil, TurnSubmissionInput{StateUpdates: &updates, Choices: &choices, World: &world})
	if !receipt.Ready || receipt.ModuleStatus.World != TurnSubmissionModuleAccepted {
		t.Fatalf("the submitted acceptance must be accepted: %#v", receipt)
	}
}

func turnWorldFlowContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestSubmissionAcceptsTheLegalRealmEntryItWillCommit is the other side: the dry
// run must not refuse what the commit accepts. The regression the live run
// found — a narrator describing the hall before the rules allowed the entrance —
// stays fixed here, at submission time.
func TestSubmissionAcceptsTheLegalRealmEntryItWillCommit(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	validation := worldFlowSubmissionContext(t, store, storyID)
	choices := testTurnChoices()
	world := TurnWorldActionInput{
		Action: TurnWorldActionRealm, RealmID: "realm.gu_xiu_dong_fu", RealmAction: RealmActionEnter,
	}
	updates := []interactivestate.Update{{
		Op: interactivestate.Replace, Path: "/story/" + storySceneLocationField,
		Value: "越国城西废药圃地下的古修洞府外殿",
	}}
	_, receipt := PrepareTurnSubmission(validation, nil, TurnSubmissionInput{World: &world, Choices: &choices, StateUpdates: &updates})

	if receipt.ModuleStatus.World != TurnSubmissionModuleAccepted {
		t.Fatalf("a legal entrance must be accepted: %#v", receipt)
	}
	if receipt.NarrativeRewriteRequired {
		t.Fatalf("a legal entrance must not ask for new prose: %#v", receipt)
	}
}

// TestARefusedWorldActionFailsTheTurn is the other half of the contract: when
// the rules refuse — the road does not start here — the turn is not committed at
// all. Its prose describes a walk that never happened, and committing it would
// publish the refused success. The model rewrites the turn instead, which the
// submission dry run makes possible before the prose is ever locked.
func TestARefusedWorldActionFailsTheTurn(t *testing.T) {
	store, storyID := worldFlowStory(t, "公共坊市")
	before := worldFlowClock(t, store, storyID)
	beforeLocation := journeyTestLocation(t, store, storyID, "main")

	_, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "回散修人家去", Narrative: "主角说自己要回去一趟。",
		TurnResult: &TurnResult{
			Choices: testTurnChoices(),
			World: &TurnWorldActionInput{
				Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true,
			},
		},
	})
	if err == nil {
		t.Fatal("a road that does not start here must fail the turn")
	}

	// The road begins outside the market, so the market cannot walk it.
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 0 {
		t.Fatalf("a refused road must publish no record: %d", count)
	}
	if after := worldFlowClock(t, store, storyID); after.Seconds != before.Seconds {
		t.Fatalf("a refused road must not spend the clock: before=%d after=%d", before.Seconds, after.Seconds)
	}
	if got := journeyTestLocation(t, store, storyID, "main"); got != beforeLocation {
		t.Fatalf("a refused road must not move the character: %q", got)
	}
}

// TestARepeatedWorldTurnSettlesNothingTwice is the retry clause: the same Agent
// identity arriving twice (a double click, or a lost response) must return the
// first turn and add no second record and no second interval.
func TestARepeatedWorldTurnSettlesNothingTwice(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")
	identity := b1CommitInput(t, store, storyID, DomainCommitIdentity{
		CommandID: "cmd-market", OperationID: "run-market", Cycle: 1,
	}, "去坊市")
	world := TurnWorldActionInput{Action: TurnWorldActionJourney, RouteID: "route_036:reverse", Arrive: true}
	request := AppendTurnWithStateRequest{
		BranchID: "main", User: "去坊市", Narrative: "主角顺着土路往坊市走。",
		TurnResult:     &TurnResult{Choices: testTurnChoices(), World: &world},
		AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
	}
	first, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	firstClock := worldFlowClock(t, store, storyID)
	second, _, err := store.AppendTurnWithState(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("a repeated request must return the first turn: %s vs %s", first.ID, second.ID)
	}
	if after := worldFlowClock(t, store, storyID); after.Seconds != firstClock.Seconds {
		t.Fatalf("a repeat must not walk the road again: %d vs %d", firstClock.Seconds, after.Seconds)
	}
	if count := journeyTestRecordCount(t, store, storyID, "main"); count != 1 {
		t.Fatalf("a repeat must not publish a second record: %d", count)
	}
}

// TestTheWorldPageNamesOnlyWhatThisTurnCouldSettle is the narrator's own view.
// Every id on it is one the commit accepts, and nothing the character cannot
// reach from where they stand appears — otherwise the model would copy an id the
// rules then refuse.
func TestTheWorldPageNamesOnlyWhatThisTurnCouldSettle(t *testing.T) {
	store, storyID := worldFlowStory(t, "越国坊市外围散修人家")

	state, err := store.WorldStateFor(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Journeys.Routes) != 1 || state.Journeys.Routes[0].OriginLocationID != "yue.market.outskirts" {
		t.Fatalf("only the road from here may be offered: %#v", state.Journeys)
	}
	if len(state.Realms.Available) != 2 || state.Realms.Available[0].RealmID != "realm.gu_xiu_dong_fu" || state.Realms.Available[1].RealmID != "realm.mountain_herb_cave" {
		t.Fatalf("the realm at this door must be offered: %#v", state.Realms)
	}
	// The spring fair is heard of in the market, not at the outskirts.
	if len(state.Events.Events) != 0 {
		t.Fatalf("an event of another place must not be offered here: %#v", state.Events)
	}

	// Standing in the market, the reverse is true.
	marketStore, marketStory := worldFlowStory(t, "公共坊市")
	marketState, err := marketStore.WorldStateFor(marketStory, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(marketState.Events.Events) != 1 || marketState.Events.Events[0].EventID != "evt.taiyue_spirit_tide" {
		t.Fatalf("the event of this place must be offered in the market: %#v", marketState.Events)
	}
	if len(marketState.Realms.Available) != 0 {
		t.Fatalf("the realm's entrance is elsewhere: %#v", marketState.Realms)
	}
}
