package interactive

import (
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover the handoff back to the main story: what the main story is
// told about a battle that was fought in its own window, and — more importantly
// — that it is told exactly once.
//
// The derivation is deliberately not a stored flag. A battle is pending while
// its records sit on the active ancestry after the branch's last Turn, so a Turn
// absorbing the fight is what ends its pendency. These tests pin that structure
// rather than any particular wording of the fragment.

// combatHandoffStory drives a story through a battle that reaches a result, and
// returns the store, the story and the settled session.
//
// It reuses the round API's own fixtures so the handoff is tested against a real
// fight with real numbers rather than a hand-built session that could drift from
// what the rules actually produce.
func combatHandoffStory(t *testing.T) (*Store, string, cultivation.CombatSession) {
	t.Helper()
	store, storyID := combatTestStory(t)
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := opened.Session.ID
	ended := opened.Session.Ended
	var settled cultivation.CombatSession
	for round := 2; round <= 40 && !ended; round++ {
		result, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
			BranchID: "main", SessionID: sessionID,
			Actions: map[string]cultivation.CombatAction{
				DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
			CommandID: commandIDFor(round), CommandHash: "hash-round",
		})
		if err != nil {
			t.Fatal(err)
		}
		ended, settled = result.Session.Ended, result.Session
	}
	if !ended {
		t.Fatal("the fixture must be able to finish a battle against an even opponent")
	}
	return store, storyID, settled
}

// TestBattleFoughtInTheWindowIsPendingForTheMainStory is the reason the handoff
// exists: a battle committed after the branch's last Turn has happened since the
// story last moved, so the story has not seen it.
func TestBattleFoughtInTheWindowIsPendingForTheMainStory(t *testing.T) {
	store, storyID, settled := combatHandoffStory(t)

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 {
		t.Fatalf("a battle fought in the window must be pending for the main story: %#v", handoffs)
	}
	handoff := handoffs[0]
	if handoff.SessionID != settled.ID {
		t.Fatalf("the pending battle must be the one that was fought: got %s want %s", handoff.SessionID, settled.ID)
	}
	if !handoff.Settled {
		t.Fatal("the battle reached a result, so the handoff must report it settled")
	}
	if handoff.Session.Result != settled.Result {
		t.Fatalf("the handoff must carry the settled result: got %q want %q", handoff.Session.Result, settled.Result)
	}
	if handoff.EventID == "" {
		t.Fatal("a handoff must name the committed record it was read from")
	}
}

// TestBattleAbsorbedByATurnIsNoLongerPending is §5.6 and §4's "one result decides
// once": once the main story writes a Turn, that fight is behind the boundary and
// must not be handed over a second time.
func TestBattleAbsorbedByATurnIsNoLongerPending(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	if handoffs, err := store.PendingCombatHandoffs(storyID, "main"); err != nil {
		t.Fatal(err)
	} else if len(handoffs) != 1 {
		t.Fatalf("the fixture must start with the fight pending: %#v", handoffs)
	}

	// This is the player choosing to continue the story: an ordinary main-story
	// turn, which is the explicit act §3 requires.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路",
		Narrative:  "沈砚没有再回头看那道山道。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 0 {
		t.Fatalf("a battle the story has already absorbed must not be handed over again: %#v", handoffs)
	}
}

// TestBattlePredatingTheLatestTurnIsNotPending keeps the boundary honest from the
// other side: a battle already in front of the narrator when the last Turn was
// written is not pending, and restating it would rewrite a scene the story
// already told.
func TestBattlePredatingTheLatestTurnIsNotPending(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	// The story continues past the fight, which absorbs it.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "继续赶路",
		Narrative:  "沈砚沿着山道继续上行。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	// A second turn after that must still see nothing pending: the battle is now
	// two Turns back, and no later read may resurface it.
	if _, _, err := store.AppendTurnWithState(storyID, AppendTurnWithStateRequest{
		BranchID: "main", User: "看看天色",
		Narrative:  "天已经全黑了。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 0 {
		t.Fatalf("a battle the story already absorbed must stay absorbed: %#v", handoffs)
	}
}

// TestPendingBattleSurvivesARestart is §8: the result must not vanish because the
// process did. The derivation reads the journal, so a fresh store over the same
// root must reach the same answer without any process-local state.
func TestPendingBattleSurvivesARestart(t *testing.T) {
	store, storyID, settled := combatHandoffStory(t)

	reopened := NewStore(store.root)
	handoffs, err := reopened.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 || handoffs[0].SessionID != settled.ID {
		t.Fatalf("a restart must re-derive the pending battle from the journal: %#v", handoffs)
	}
}

// TestForkThatDidNotContainTheFightDoesNotHandItOver is §6: a branch that forked
// before the battle must not inherit its result. The fork's checkpoint carries
// the parent's battles, but its own head sits before the fight, so the battle is
// not on the fork's ancestry and is not pending there.
func TestForkThatDidNotContainTheFightDoesNotHandItOver(t *testing.T) {
	store, storyID := combatTestStory(t)

	snapshot, err := store.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil {
		t.Fatal("the fixture must have a turn to fork from")
	}
	// Fork at the turn before the fight.
	forked, err := store.CreateBranch(storyID, CreateBranchRequest{
		ParentEventID: snapshot.CurrentTurn.ID, Title: "未开战的分支",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Now fight the battle on main, after the fork point.
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	ended := opened.Session.Ended
	for round := 2; round <= 40 && !ended; round++ {
		result, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
			BranchID: "main", SessionID: opened.Session.ID,
			Actions: map[string]cultivation.CombatAction{
				DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
			CommandID: commandIDFor(round), CommandHash: "hash-round",
		})
		if err != nil {
			t.Fatal(err)
		}
		ended = result.Session.Ended
	}

	// The fight is pending on the branch that fought it.
	if handoffs, err := store.PendingCombatHandoffs(storyID, "main"); err != nil {
		t.Fatal(err)
	} else if len(handoffs) != 1 {
		t.Fatalf("the branch that fought the battle must have it pending: %#v", handoffs)
	}
	// And absent on the branch that forked before it, even though the fork
	// inherited the parent's battle map at its checkpoint.
	if handoffs, err := store.PendingCombatHandoffs(storyID, forked.ID); err != nil {
		t.Fatal(err)
	} else if len(handoffs) != 0 {
		t.Fatalf("a fork that did not contain the fight must not hand it over: %#v", handoffs)
	}
}

// TestHandoffFragmentNamesTheFormalResultAndKeepsDecisionsOpen pins the content
// §5.4 and §5.5 require: the story is told what the rules settled, and is
// explicitly not told to choose the player's next move.
func TestHandoffFragmentNamesTheFormalResultAndKeepsDecisionsOpen(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	fragment := CombatHandoffFragment(handoffs)
	if fragment == "" {
		t.Fatal("a pending battle must produce a handoff fragment")
	}
	if len(fragment) > CombatHandoffFragmentLimit {
		t.Fatalf("the fragment must respect §7's %d-byte limit: %d", CombatHandoffFragmentLimit, len(fragment))
	}
	// §7: the fragment is English, whatever language the narrative is in.
	if strings.ContainsAny(fragment, "，。；：、（）") {
		t.Fatalf("the handoff fragment must be written in English: %q", fragment)
	}
	if !strings.Contains(fragment, "must not be fought again") {
		t.Fatalf("the fragment must forbid restaging the battle: %q", fragment)
	}
	// §5.5: the decisions the player has not made must be named as open rather
	// than resolved for them.
	for _, open := range []string{"pursue", "surrender", "search the fallen"} {
		if !strings.Contains(fragment, open) {
			t.Errorf("the fragment must leave %q to the player: %q", open, fragment)
		}
	}
	// And it must name the result the rules actually settled.
	if !strings.Contains(fragment, string(handoffs[0].Session.Result)) &&
		!strings.Contains(strings.ToLower(fragment), "battle ended") {
		t.Fatalf("the fragment must state the settled result: %q", fragment)
	}
}

// TestHandoffFragmentDistinguishesAYieldedEnemyFromADefeatedOne is §2 and §7 of
// the battle design on the handoff path: a surrendered opponent is alive, and a
// fragment that calls them defeated would have the story write a corpse.
func TestHandoffFragmentDistinguishesAYieldedEnemyFromADefeatedOne(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	handoffs[0].Session.Combatants = []cultivation.Combatant{
		{ID: "protagonist", Side: cultivation.SidePlayer, Attributes: cultivation.Attributes{Health: 30, HealthMax: 200, Mana: 10, ManaMax: 160}},
		{ID: "rival", Side: cultivation.SideEnemy, Surrendered: true, Attributes: cultivation.Attributes{Health: 12, HealthMax: 200, Mana: 0, ManaMax: 160}},
	}
	fragment := CombatHandoffFragment(handoffs)
	if !strings.Contains(fragment, "yielded") || !strings.Contains(fragment, "alive") {
		t.Fatalf("a yielded opponent must read as alive, not defeated: %q", fragment)
	}
	if strings.Contains(fragment, "rival was defeated") {
		t.Fatalf("a yielded opponent must not be reported as defeated: %q", fragment)
	}
}

// TestHandoffFragmentReportsAnUnansweredOfferAsOpen is §4's row for battle: an
// outstanding offer to yield is a proposal, and returning to the story must not
// answer it on the player's behalf.
func TestHandoffFragmentReportsAnUnansweredOfferAsOpen(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	handoffs[0].Session.Combatants = []cultivation.Combatant{
		{ID: "protagonist", Side: cultivation.SidePlayer},
		{ID: "rival", Side: cultivation.SideEnemy, OfferText: "饶我一命，我把身上的灵石都给你。"},
	}
	fragment := CombatHandoffFragment(handoffs)
	if !strings.Contains(fragment, "not been answered") {
		t.Fatalf("an unanswered offer must be reported as still open: %q", fragment)
	}
	if !strings.Contains(fragment, "饶我一命") {
		t.Fatalf("the offer's own words must survive into the fragment: %q", fragment)
	}
}

// TestHandoffFragmentStaysWithinItsLimitAndSaysWhatItDropped is §7 and JOIN11: an
// over-long handoff drops the oldest battles and reports the drop rather than
// silently presenting a partial picture as the whole one.
func TestHandoffFragmentStaysWithinItsLimitAndSaysWhatItDropped(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)

	base, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(base) != 1 {
		t.Fatalf("the fixture must produce one handoff: %#v", base)
	}
	// Many long-named battles, so the combined fragment cannot fit.
	handoffs := make([]PendingCombatHandoff, 0, 200)
	for index := 0; index < 200; index++ {
		handoff := base[0]
		handoff.SessionID = strings.Repeat("c", 64) + string(rune('a'+index%26)) + strings.Repeat("d", 64)
		handoffs = append(handoffs, handoff)
	}
	fragment := CombatHandoffFragment(handoffs)
	if len(fragment) > CombatHandoffFragmentLimit {
		t.Fatalf("an over-long handoff must be reduced to the limit: %d", len(fragment))
	}
	if !strings.Contains(fragment, "not summarised here") {
		t.Fatalf("a reduced handoff must say what it left out rather than pretend it is complete: %q", fragment)
	}
}

// TestHandoffFragmentIsEmptyWhenNothingIsPending keeps the ordinary turn clean:
// a story that has fought nothing must not carry a battle instruction at all.
func TestHandoffFragmentIsEmptyWhenNothingIsPending(t *testing.T) {
	if fragment := CombatHandoffFragment(nil); fragment != "" {
		t.Fatalf("no pending battle must produce no fragment: %q", fragment)
	}
	store, storyID := combatTestStory(t)
	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if fragment := CombatHandoffFragment(handoffs); fragment != "" {
		t.Fatalf("a story that has fought nothing must produce no fragment: %q", fragment)
	}
}

// TestAnUnsettledBattleIsHandedOverAsStillInProgress is the window design's §3
// rule that a fight still running may only be collapsed, never ended: the story
// is told the fight is live so it does not narrate a result that never happened.
func TestAnUnsettledBattleIsHandedOverAsStillInProgress(t *testing.T) {
	store, storyID := combatTestStory(t)
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Session.Ended {
		t.Fatal("the opening round of an even fight must not settle it")
	}

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 {
		t.Fatalf("a live battle is still a battle the story has not absorbed: %#v", handoffs)
	}
	if handoffs[0].Settled {
		t.Fatal("a battle that has not ended must not be reported as settled")
	}
	fragment := CombatHandoffFragment(handoffs)
	if !strings.Contains(fragment, "still in progress") {
		t.Fatalf("an unfinished battle must be reported as unfinished: %q", fragment)
	}
	if strings.Contains(fragment, "The fight is over") {
		t.Fatalf("an unfinished battle must not be reported as over: %q", fragment)
	}
}

// TestEveryExchangeOfOneBattleIsHandedOverOnce keeps a many-round fight from
// reading as many battles: a battle commits one record per exchange, and only
// the latest record per session is its current state.
func TestEveryExchangeOfOneBattleIsHandedOverOnce(t *testing.T) {
	store, storyID := combatTestStory(t)
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Two more exchanges, whether or not they end the fight.
	for round := 2; round <= 3; round++ {
		result, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
			BranchID: "main", SessionID: opened.Session.ID,
			Actions: map[string]cultivation.CombatAction{
				DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
			CommandID: commandIDFor(round), CommandHash: "hash-round",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Session.Ended {
			break
		}
	}
	history, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 2 {
		t.Fatalf("the fixture must have committed more than one exchange: %d", len(history))
	}

	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 1 {
		t.Fatalf("one battle must be handed over once, not once per exchange: %#v", handoffs)
	}
}

// TestTheHandoffDoesNotHandTheModelAnOpponentsExactPools is §三.2 on the handoff
// path: the fragment is model-shared input, so the same audience constraint that
// keeps an opponent's numbers out of the window keeps them out of the prompt.
//
// What the story must continue from is the result and the standings — a fled
// enemy stays fled, a yielded one is alive. What it must not be handed is the
// opponent's exact remaining health and spiritual power, which would let the
// prose state figures the player was never shown.
func TestTheHandoffDoesNotHandTheModelAnOpponentsExactPools(t *testing.T) {
	store, storyID, _ := combatHandoffStory(t)
	handoffs, err := store.PendingCombatHandoffs(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	handoffs[0].Session.Combatants = []cultivation.Combatant{
		{ID: "protagonist", Side: cultivation.SidePlayer,
			Attributes: cultivation.Attributes{Health: 64, HealthMax: 80, Mana: 40, ManaMax: 60}},
		{ID: "rival", Side: cultivation.SideEnemy,
			Attributes: cultivation.Attributes{Health: 37, HealthMax: 120, Mana: 88, ManaMax: 90}},
	}
	fragment := CombatHandoffFragment(handoffs)
	for _, leaked := range []string{"37/120", "88/90", "37 of 120", "88 of 90"} {
		if strings.Contains(fragment, leaked) {
			t.Fatalf("an opponent's exact pool %q reached the handoff fragment: %q", leaked, fragment)
		}
	}
	// The player's own figures are the player's, and the story continues from
	// them, so they stay exact.
	if !strings.Contains(fragment, "64/80") {
		t.Fatalf("the player's own pools must still be handed over: %q", fragment)
	}
	// An opponent is still described by what can be seen of them.
	if !strings.Contains(fragment, "rival") {
		t.Fatalf("the opponent must still be named in the handoff: %q", fragment)
	}
}
