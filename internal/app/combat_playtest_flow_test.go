package app

import (
	"errors"
	"strings"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// TestOneBattleFromEncounterToMainStory walks batch 1 as one continuous path
// rather than as a set of unrelated unit checks: a neutral character becomes an
// attacker, the fight opens as one session, the player's own first action
// settles round 1, the settled round is readable before its prose exists, a
// retried command reads that round back instead of resolving a second one, the
// result reaches the main story, and the whole thing survives a restart.
//
// Every step reads facts a turn actually committed. The fixture is the real
// xiuxian preset and the scene writes its own 在场角色 / 本次交战, so no step
// below depends on a number or a side this test chose for itself.
func TestOneBattleFromEncounterToMainStory(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	store := application.interactive
	root := application.workspace

	// --- 中立：a present important character, and nobody attacking ------------
	if _, _, err := store.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "走进山道", Narrative: "沈青站在道旁，没有拔刀。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "沈青"}, Reason: "本回合确认的在场角色",
			},
			{
				Op: "set", ActorID: interactive.DefaultActorID,
				FieldID: combatTestCarriedField,
				Value:   map[string]any{"回春丹": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)}},
				Reason:  "随身携带",
			},
		},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	neutral, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if neutral.Encounter == nil || neutral.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("a neutral character standing by is not a fight: %#v", neutral.Encounter)
	}
	if len(neutral.Encounter.Bystanders) != 1 || neutral.Encounter.Bystanders[0] != "沈青" {
		t.Fatalf("the neutral character is reported as standing there: %#v", neutral.Encounter.Bystanders)
	}

	// --- 真实交战：the scene states the attack --------------------------------
	if _, _, err := store.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "继续前行", Narrative: "沈青忽然拔刀劈向主角。",
		ActorOps: []interactive.ActorStateOp{
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "在场角色", Value: []any{interactive.DefaultActorID, "沈青"}, Reason: "本回合确认的在场角色",
			},
			{
				Op: "set", ActorID: interactive.DefaultStoryContextActorID,
				FieldID: "本次交战", Value: map[string]any{"沈青": "沈青拔刀劈向主角"}, Reason: "本回合确认的交战",
			},
		},
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	engaged, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if engaged.Encounter == nil || engaged.Encounter.ReasonKey != "" {
		t.Fatalf("a declared attack is a fight: %#v", engaged.Encounter)
	}
	sides := map[string]string{}
	for _, combatant := range engaged.Encounter.Combatants {
		sides[combatant.ActorID] = combatant.Side
	}
	if sides["沈青"] != string(cultivation.SideEnemy) {
		t.Fatalf("the declared attacker fights on the enemy side: %#v", engaged.Encounter.Combatants)
	}
	if len(engaged.Encounter.Bystanders) != 0 {
		t.Fatalf("nobody else is standing by: %#v", engaged.Encounter.Bystanders)
	}

	// --- 唯一场次：opening establishes the encounter and settles nothing ------
	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "flow-open"})
	if err != nil {
		t.Fatal(err)
	}
	if opened.SessionID == "" || opened.Round != 1 || opened.Ended {
		t.Fatalf("opening establishes one encounter waiting on round 1: %#v", opened)
	}
	if len(opened.History) != 0 || opened.LastRound != nil {
		t.Fatalf("opening settles no exchange: %#v", opened.History)
	}
	player := combatWindowSide(t, opened, interactive.DefaultActorID)
	if *player.Health != *player.HealthMax || *player.Mana != *player.ManaMax {
		t.Fatalf("opening must not spend the player's pools: %#v", player)
	}
	if dose := combatTestWindowItem(t, opened, interactive.DefaultActorID, "回春丹"); dose.Quantity != 3 {
		t.Fatalf("opening must not spend stock: quantity %d", dose.Quantity)
	}

	// The same encounter opened again returns the fight that already exists
	// rather than starting a second one beside it.
	again, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "flow-open-again"})
	if err != nil {
		t.Fatal(err)
	}
	if again.SessionID != opened.SessionID {
		t.Fatalf("reopening the same encounter must return the existing fight: %s vs %s", again.SessionID, opened.SessionID)
	}
	if len(again.History) != 0 {
		t.Fatalf("reopening must not settle a round: %#v", again.History)
	}
	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("one person, one unfinished battle: %#v", sessions)
	}

	// A second fight for the same person is refused and names the running one.
	_, err = application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "flow-second-battle",
		Combatants: []interactive.CombatantSpec{
			{ActorID: interactive.DefaultActorID, Side: cultivation.SidePlayer, RouteExists: true},
			{ActorID: "狼王", Side: cultivation.SideEnemy, RouteExists: true, Retreat: cultivation.RetreatSurvival},
		},
	})
	var conflict *interactive.CombatActorConflict
	if !errors.As(err, &conflict) || conflict.SessionID != opened.SessionID {
		t.Fatalf("a second battle for the same person must be refused with the running one named: %v", err)
	}

	// --- 明确选择首招：the first exchange is the player's own decision ---------
	first, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "flow-round-1",
		SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "沈青"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.LastRound == nil || first.LastRound.Outcome.Round != 1 {
		t.Fatalf("the chosen action must be round 1: %#v", first.LastRound)
	}
	step, ok := combatTestStep(first.LastRound.Outcome, interactive.DefaultActorID)
	if !ok || step.Kind != cultivation.ActionTechnique || step.SkillID != "strike" {
		t.Fatalf("round 1 must be the action the player chose: %#v", first.LastRound.Outcome)
	}
	// The enemy decided for itself in the same round, from its own state.
	if _, acted := combatTestStep(first.LastRound.Outcome, "沈青"); !acted {
		t.Fatalf("the enemy must act in the first round from its own decision: %#v", first.LastRound.Outcome)
	}
	// The strike spends no stock.
	if dose := combatTestWindowItem(t, first, interactive.DefaultActorID, "回春丹"); dose.Quantity != 3 {
		t.Fatalf("an ordinary attack must not spend the carried medicine: quantity %d", dose.Quantity)
	}

	// --- 结算先显示：the committed round is readable before its prose ---------
	if first.LastRound.Narration != "" {
		t.Fatalf("the fixture plays without prose: %#v", first.LastRound)
	}
	if first.NarrationError == "" {
		t.Fatal("a committed round with no prose must offer the recovery entry")
	}

	// The prose is committed the way a successful generation commits it, then
	// read back: filling in the account writes text and resolves nothing.
	roundEventID := first.LastRound.EventID
	if _, err := store.AppendCombatNarration(storyID, interactive.AppendCombatNarrationRequest{
		BranchID: "main", SessionID: opened.SessionID, RoundEventID: roundEventID,
		Narrative: "沈青一刀劈落，主角侧身避开，反手还了一记。",
		CommandID: "flow-narr-1",
		CommandHash: combatNarrationCommandHash(storyID, "main", opened.SessionID, roundEventID),
	}); err != nil {
		t.Fatal(err)
	}
	narrated, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: opened.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if narrated.LastRound == nil || narrated.LastRound.Narration == "" {
		t.Fatalf("the committed passage must be read back: %#v", narrated.LastRound)
	}
	if narrated.NarrationError != "" {
		t.Fatalf("a round with prose owes no recovery entry: %q", narrated.NarrationError)
	}
	if len(narrated.History) != 1 {
		t.Fatalf("writing prose must not settle another round: %d", len(narrated.History))
	}

	// --- 冲突恢复：a retried command reads the committed round -----------------
	retried, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "flow-round-1",
		SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "沈青"},
		},
	})
	if err != nil {
		t.Fatalf("a retry of a committed command must be answered, not refused: %v", err)
	}
	if retried.LastRound == nil || retried.LastRound.EventID != roundEventID {
		t.Fatalf("the retry must report the round that was actually committed: %#v", retried.LastRound)
	}
	if len(retried.History) != 1 {
		t.Fatalf("a retry must not commit a second round: %d", len(retried.History))
	}

	// The same command identity with a different action is a conflict, not a
	// second round: the player's next choice needs a new identity.
	if _, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "flow-round-1",
		SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionRest},
		},
	}); !errors.Is(err, interactive.ErrCombatCommandConflict) {
		t.Fatalf("a reused command for another action must conflict: %v", err)
	}

	// --- 战果：the fight runs to a result --------------------------------------
	current := retried
	for round := 2; round <= 80 && !current.Ended; round++ {
		current, err = application.PlayCombatRound(CombatRoundRequest{
			StoryID: storyID, SessionID: opened.SessionID,
			CommandID: "flow-round-" + itoa(round), SkipNarration: true,
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "沈青"},
			},
		})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if !current.Ended || current.Result == "" || current.Result == string(cultivation.ResultUnresolved) {
		t.Fatalf("the fixture must reach a formal result: ended=%t result=%q", current.Ended, current.Result)
	}

	// --- 保存重开：the journal, not the process, holds the fight --------------
	reopenedStore := interactive.NewStore(root)
	t.Cleanup(func() { _ = reopenedStore.Close() })
	reopened := &App{workspace: root, interactive: reopenedStore}
	afterRestart, err := reopened.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: opened.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !afterRestart.Ended || afterRestart.Result != current.Result {
		t.Fatalf("a restart must show the same result: %#v", afterRestart)
	}
	if len(afterRestart.History) != len(current.History) || afterRestart.History[0].EventID != current.History[0].EventID {
		t.Fatalf("a restart must show the same committed rounds: %d vs %d", len(afterRestart.History), len(current.History))
	}

	// --- 战果回主故事：the main story is told once -----------------------------
	handoffs, err := reopenedStore.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 || !handoffs[0].Settled {
		t.Fatalf("the settled fight is pending for the main story: %#v", handoffs)
	}
	fragment := interactive.CombatHandoffFragment(handoffs)
	if !strings.Contains(fragment, "must not be fought again") {
		t.Fatalf("the handoff must forbid restaging the fight: %q", fragment)
	}
	if !strings.Contains(fragment, "The battle ended") {
		t.Fatalf("the handoff must carry the formal result: %q", fragment)
	}

	// The next main-story turn absorbs it, and it is not handed over twice.
	if _, _, err := reopenedStore.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "从这场战斗里脱身", Narrative: "主角收起架势，接着往前走。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatal(err)
	}
	absorbed, err := reopenedStore.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(absorbed) != 0 {
		t.Fatalf("an absorbed battle must not be handed over again: %#v", absorbed)
	}
	if fragment := interactive.CombatHandoffFragment(absorbed); fragment != "" {
		t.Fatalf("nothing pending must produce no fragment: %q", fragment)
	}
}

// itoa keeps the round commands readable without pulling strconv into the test's
// import list for one call.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
