package app

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file covers the way out of a fight the interface opens from the scene.
//
// The window derives the encounter and then sends back the participants it was
// shown. A client that sends only the ids and sides is stating the roster, not
// the route: route_exists and retreat are facts about the scene, and the opening
// request carries them. When the interface dropped them, the protagonist lost
// the only legal way out of a fight the story had not confined them to.

// TestCombatWindowEncounterReportsThePlayersRoute is the read side. A player the
// scene has not confined must be reported with a route, or the window cannot
// offer the escape it is entitled to.
func TestCombatWindowEncounterReportsThePlayersRoute(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	system := combatTestSystem(t, "fire_bolt")
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "退路", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	stage := cultivation.StageKey(cultivation.Qi(6))
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "撞见", Narrative: "两人在道旁对峙。",
		TurnResult: &interactive.TurnResult{
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Create, Path: "/拦路人", Value: map[string]any{
					"template_id": interactive.ActorStateOpponentTemplateID,
					"name":        "拦路人",
					"state": map[string]any{
						combatTestRealmField:  stage,
						combatTestSkillsField: learnedSkillRecords([]string{"fire_bolt"}),
					},
				}},
				{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{interactive.DefaultActorID, "拦路人"}},
				{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"拦路人": "拦路人拔刀"}},
			},
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: root, interactive: store}
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil {
		t.Fatal("the scene must settle an encounter")
	}
	var player *CombatWindowCombatant
	for index := range window.Encounter.Combatants {
		if window.Encounter.Combatants[index].ActorID == interactive.DefaultActorID {
			player = &window.Encounter.Combatants[index]
		}
	}
	if player == nil {
		t.Fatalf("the encounter must include the protagonist: %#v", window.Encounter.Combatants)
	}
	if !player.RouteExists {
		t.Fatalf("an unconfined protagonist must be reported with a way out: %#v", player)
	}
}

// TestOpeningFromTheReportedEncounterKeepsThePlayersRoute is the write side.
//
// The interface sends back what the entry read showed it. This test sends the
// same shape the panel builds and asserts the fight it opens still has a way
// out — the fact a client that re-typed only the ids would silently lose.
func TestOpeningFromTheReportedEncounterKeepsThePlayersRoute(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	system := combatTestSystem(t, "fire_bolt")
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "退路", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	stage := cultivation.StageKey(cultivation.Qi(6))
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "撞见", Narrative: "两人在道旁对峙。",
		TurnResult: &interactive.TurnResult{
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Create, Path: "/拦路人", Value: map[string]any{
					"template_id": interactive.ActorStateOpponentTemplateID,
					"name":        "拦路人",
					"state": map[string]any{
						combatTestRealmField:  stage,
						combatTestSkillsField: learnedSkillRecords([]string{"fire_bolt"}),
					},
				}},
				{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{interactive.DefaultActorID, "拦路人"}},
				{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"拦路人": "拦路人拔刀"}},
			},
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: root, interactive: store}
	encounter, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID})
	if err != nil {
		t.Fatal(err)
	}
	// The shape the panel sends is the shape the read reported. Anything else is
	// the interface deciding a rule fact the scene already settled.
	specs := make([]interactive.CombatantSpec, 0, len(encounter.Encounter.Combatants))
	for _, combatant := range encounter.Encounter.Combatants {
		specs = append(specs, interactive.CombatantSpec{
			ActorID:     combatant.ActorID,
			Side:        cultivation.CombatSide(combatant.Side),
			RouteExists: combatant.RouteExists,
			Retreat:     cultivation.RetreatTemplate(combatant.Retreat),
		})
	}
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: story.ID, CommandID: "cmd-route", Combatants: specs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.SessionID == "" {
		t.Fatalf("the fight must open: %#v", opened)
	}
	player := combatWindowSide(t, opened, interactive.DefaultActorID)
	if !player.RouteExists {
		t.Fatalf("the protagonist must keep the way out the scene settled: %#v", player)
	}

	// The committed battle carries the same fact, so closing and reopening the
	// window finds the way out still there rather than only in the response that
	// opened it.
	//
	// This asserts the route is stored. Whether the escape itself then works is a
	// different question, answered by TestEscapingTheFightEndsItAndTheMainStoryAbsorbsIt
	// below — an earlier version of this comment claimed the escape happened while
	// the assertions only ever read RouteExists.
	sessions, err := store.CombatSessions(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	session := sessions[opened.SessionID]
	var self *cultivation.Combatant
	for index := range session.Combatants {
		if session.Combatants[index].ID == interactive.DefaultActorID {
			self = &session.Combatants[index]
		}
	}
	if self == nil || !self.RouteExists {
		t.Fatalf("the opened fight must carry the route the request stated: %#v", session.Combatants)
	}
}

// TestEscapingTheFightEndsItAndTheMainStoryAbsorbsIt plays the way out for real.
//
// The route the panel reports is only a permission; §14.2 makes the escape itself
// a declared action that has to be resolved and committed like any other. This
// sends that action through the window's own write, and checks the four things
// that follow from it: the fight reaches a formal result, the player really left
// it, the wound and the place the fight was fought at are the ones carried
// forward, and the main story is handed the result rather than having to guess it.
//
// The battle's identity is fixed here rather than minted, because the escape draw
// is derived from the command, the session and the round (§七.2). A test that
// wanted a specific disengagement has to know the battle it is disengaging from;
// letting the app mint a random session id would leave the roll to chance.
func TestEscapingTheFightEndsItAndTheMainStoryAbsorbsIt(t *testing.T) {
	root := t.TempDir()
	store := interactive.NewStore(root)
	t.Cleanup(func() { _ = store.Close() })

	const (
		sessionID = "cbt-escape-route"
		// The escape chance between two bare cultivators of the same layer is the
		// published 50% base, so this command id is one whose derived draw lands
		// under it. Nothing else about the round depends on the id.
		escapeCommand = "cmd-escape-7"
		// A registered place, so the fight can be shown not to move the player.
		place = "越国坊市外围散修人家"
	)

	system := combatTestSystem(t, "fire_bolt")
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "退路", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	stage := cultivation.StageKey(cultivation.Qi(6))
	_, _, err = store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "撞见", Narrative: "两人在道旁对峙。",
		ActorOps: []interactive.ActorStateOp{{
			Op: "set", ActorID: interactive.DefaultStoryContextActorID, FieldID: "当前详细地点",
			Value: place, Reason: "本回合确认的地点",
		}},
		TurnResult: &interactive.TurnResult{
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Create, Path: "/拦路人", Value: map[string]any{
					"template_id": interactive.ActorStateOpponentTemplateID,
					"name":        "拦路人",
					"state": map[string]any{
						combatTestRealmField:  stage,
						combatTestSkillsField: learnedSkillRecords([]string{"fire_bolt"}),
					},
				}},
				{Op: interactivestate.Replace, Path: "/story/在场角色", Value: []any{interactive.DefaultActorID, "拦路人"}},
				{Op: interactivestate.Replace, Path: "/story/本次交战", Value: map[string]any{"拦路人": "拦路人拔刀"}},
			},
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	application := &App{workspace: root, interactive: store}
	before, err := application.CombatWindow(CombatWindowRequest{StoryID: story.ID})
	if err != nil {
		t.Fatal(err)
	}
	if before.Encounter == nil {
		t.Fatal("the scene must settle an encounter")
	}
	specs := make([]interactive.CombatantSpec, 0, len(before.Encounter.Combatants))
	for _, combatant := range before.Encounter.Combatants {
		specs = append(specs, interactive.CombatantSpec{
			ActorID:     combatant.ActorID,
			Side:        cultivation.CombatSide(combatant.Side),
			RouteExists: combatant.RouteExists,
			Retreat:     cultivation.RetreatTemplate(combatant.Retreat),
		})
	}

	// Opening is the store's own operation because the identity is supplied here;
	// the window mints a fresh one when it opens, which is what this test cannot
	// use. Everything after the opening goes through the window's write.
	if _, err := store.StartBattle(story.ID, interactive.BattleStartRequest{
		BranchID: "main", SessionID: sessionID, Combatants: specs,
	}); err != nil {
		t.Fatal(err)
	}

	escaped, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: story.ID, SessionID: sessionID, CommandID: escapeCommand, SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionEscape, TargetID: "拦路人"},
		},
	})
	if err != nil {
		t.Fatalf("declaring the way out must be a legal round: %v", err)
	}

	// 1. The exchange really resolved an escape, rather than ending some other way.
	if len(escaped.History) != 1 {
		t.Fatalf("the escape round must be the only committed exchange: %#v", escaped.History)
	}
	step, ok := combatTestStep(escaped.History[0].Outcome, interactive.DefaultActorID)
	if !ok {
		t.Fatalf("the round must report the protagonist's action: %#v", escaped.History[0].Outcome)
	}
	if !step.Escaped {
		t.Fatalf("the declared escape must resolve as an escape, got step %#v", step)
	}

	// 2. The fight is over and it ended with the player leaving, not losing.
	if !escaped.Ended {
		t.Fatalf("leaving the fight ends it: %#v", escaped)
	}
	if escaped.Result != string(cultivation.ResultPlayerFled) {
		t.Fatalf("the result must be that the player left the fight, got %q", escaped.Result)
	}
	player := combatWindowSide(t, escaped, interactive.DefaultActorID)
	if player.Standing != string(cultivation.StandingFled) {
		t.Fatalf("the protagonist must stand as having left, got %q", player.Standing)
	}
	// A fleeing side is not a defeated one: the pools it left with are its own.
	if player.Health == nil || *player.Health <= 0 {
		t.Fatalf("leaving the fight is not being beaten: %#v", player)
	}

	// 3. The fight moved nobody. §14.2 makes the way out an exit from the
	// encounter, not a shortcut to somewhere else, so the scene is where it was.
	if escaped.Scene.Location != place || before.Scene.Location != place {
		t.Fatalf("the fight must not move the player: %q -> %q", before.Scene.Location, escaped.Scene.Location)
	}

	// The wound the exchange dealt is the one carried forward: the story's own
	// pool is the battle's pool, and nothing restored the player on the way out.
	sessions, err := store.CombatSessions(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	committed, ok := sessions[sessionID]
	if !ok {
		t.Fatalf("the battle must still be on the record: %#v", sessions)
	}
	var settledSelf *cultivation.Combatant
	for index := range committed.Combatants {
		if committed.Combatants[index].ID == interactive.DefaultActorID {
			settledSelf = &committed.Combatants[index]
		}
	}
	if settledSelf == nil {
		t.Fatalf("the record must hold the protagonist: %#v", committed.Combatants)
	}
	if *player.Health != settledSelf.Attributes.Health {
		t.Fatalf("the window must report the fight's own pool: %d vs %d", *player.Health, settledSelf.Attributes.Health)
	}
	if settledSelf.Attributes.Health < settledSelf.Attributes.HealthMax {
		storyContext, err := store.StoryContext(story.ID, "main")
		if err != nil {
			t.Fatal(err)
		}
		stored, present := combatTestActorField(t, storyContext.Snapshot.State, interactive.DefaultActorID, combatTestHealthField)
		if !present {
			t.Fatalf("a wound taken in the fight must be written to the story, got no %s", combatTestHealthField)
		}
		if number, ok := stored.(float64); !ok || int(number) != settledSelf.Attributes.Health {
			t.Fatalf("the story must carry the wound the fight settled: %v vs %d", stored, settledSelf.Attributes.Health)
		}
	}

	// 4. The main story is handed the result it did not witness.
	handoffs, err := store.PendingCombatHandoffs(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 || !handoffs[0].Settled || handoffs[0].SessionID != sessionID {
		t.Fatalf("the settled fight must be pending for the main story: %#v", handoffs)
	}
	if handoffs[0].Session.Result != cultivation.ResultPlayerFled {
		t.Fatalf("the handoff must carry the result that was settled, got %q", handoffs[0].Session.Result)
	}
	fragment := interactive.CombatHandoffFragment(handoffs)
	if !strings.Contains(fragment, "the player's side disengaged and left the fight") {
		t.Fatalf("the handoff must say the player left rather than lost: %q", fragment)
	}
	if !strings.Contains(fragment, "protagonist has left the fight.") {
		t.Fatalf("the handoff must say the protagonist is gone from the fight: %q", fragment)
	}
	if strings.Contains(fragment, "was defeated") {
		t.Fatalf("a side that got away must not be reported as beaten: %q", fragment)
	}

	// A finished fight is not fought again: a second command answers with the
	// settled result and commits nothing.
	again, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: story.ID, SessionID: sessionID, CommandID: "cmd-escape-again", SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionEscape, TargetID: "拦路人"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Result != escaped.Result || len(again.History) != len(escaped.History) {
		t.Fatalf("an ended fight must stand: %#v", again)
	}

	// The next main-story turn absorbs the handoff, and it is not handed over twice.
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "顺着土路退回坊市", Narrative: "你退出那条土路，回到坊市外围。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	absorbed, err := store.PendingCombatHandoffs(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(absorbed) != 0 {
		t.Fatalf("an absorbed battle must not be handed over again: %#v", absorbed)
	}
}

// combatTestActorField reads one stored field of one Actor, so a test can compare
// what the story carries against what the fight settled.
func combatTestActorField(t *testing.T, state map[string]any, actorID, fieldID string) (any, bool) {
	t.Helper()
	record, ok := combatActorRecord(state, actorID)
	if !ok {
		return nil, false
	}
	actor, _ := record.(map[string]any)
	values, _ := actor["state"].(map[string]any)
	value, present := values[fieldID]
	return value, present
}
