package interactive

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"ta/internal/cultivation"
)

// These tests cover the store-level round API — the middle between the pure
// rules engine and the journal. The engine's own tests prove an exchange settles
// correctly; what is proven here is that a round played through the window is
// built from the characters' real state, that the opposition decides for itself,
// that a retry re-reads rather than re-settles, and that the rounds a player can
// look back over are the rounds that were actually committed.

// TestStartBattleBuildsBothSidesFromTheirOwnState is §七.1's floor for the write
// path: a battle opened through the window must be built from the characters'
// stored numbers, not from anything the caller supplied.
func TestStartBattleBuildsBothSidesFromTheirOwnState(t *testing.T) {
	store, storyID := combatTestStory(t)

	result, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.ID == "" {
		t.Fatal("opening a battle must mint a session id")
	}
	if len(result.Session.Combatants) != 2 {
		t.Fatalf("a battle must carry both sides: %#v", result.Session.Combatants)
	}
	for _, combatant := range result.Session.Combatants {
		if combatant.Attributes.HealthMax != 200 || combatant.Attributes.ManaMax != 160 {
			t.Errorf("combatant %s must be built at the stage's real pools: %#v", combatant.ID, combatant.Attributes)
		}
		if combatant.Attributes.Attack != 40 {
			t.Errorf("combatant %s must carry the stage's real attack: %#v", combatant.ID, combatant.Attributes)
		}
	}
	// The enemy is the side the caller did not act for, so its action in the
	// opening round is the decision function's, not a default the engine filled in.
	enemy := combatTestCombatant(t, result.Session, "rival")
	if enemy.Side != cultivation.SideEnemy {
		t.Fatalf("the rival must be on the enemy side: %#v", enemy.Side)
	}
}

// TestStartBattleIsIdempotentOnItsCommand is §七.2: a repeated open must report
// the battle the first attempt committed rather than starting a second one.
func TestStartBattleIsIdempotentOnItsCommand(t *testing.T) {
	store, storyID := combatTestStory(t)

	request := BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	}
	first, err := store.StartBattle(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.StartBattle(storyID, request)
	if err != nil {
		t.Fatalf("a retry of the same command must not fail: %v", err)
	}
	if second.Session.ID != first.Session.ID {
		t.Fatalf("a retry must address the same battle: first=%s second=%s", first.Session.ID, second.Session.ID)
	}
	if second.EventID != first.EventID {
		t.Fatalf("a retry must report the committed record: first=%s second=%s", first.EventID, second.EventID)
	}

	// And only one battle exists, which is the fact the id check is a proxy for.
	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("a retry must not open a second battle: %#v", sessions)
	}
}

// TestStartBattleOnlyEstablishesTheEncounter is R3's floor: opening a fight
// establishes it and commits it, and plays nothing.
//
// §七.1's opening is the encounter, not the first exchange. Both sides stand at
// the numbers they already had until the player submits a legal first action, so
// opening must spend no health, no mana and no carried stock, and the round
// history must be empty. The battle is nevertheless on the record: the window,
// a restart and the one-battle-per-actor guard all read it from the journal.
func TestStartBattleOnlyEstablishesTheEncounter(t *testing.T) {
	store, storyID := combatTestStory(t)
	healthBefore := combatTestProtagonistHealth(t, store, storyID)
	manaBefore := combatTestProtagonistMana(t, store, storyID)

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Session.ID == "" || opened.EventID == "" {
		t.Fatalf("opening must establish and commit the encounter: %#v", opened)
	}
	if opened.Settled {
		t.Fatal("opening a battle settles no exchange")
	}
	if opened.Outcome.Round != 0 {
		t.Fatalf("the opening is not a round: got round %d", opened.Outcome.Round)
	}
	// The exchange the player is now being asked to play is the first one.
	if opened.Session.Round != 1 {
		t.Fatalf("an opened battle waits on round 1, got %d", opened.Session.Round)
	}
	if opened.Session.Ended {
		t.Fatal("an opened battle has not ended")
	}

	// Nobody was charged for a round nobody played.
	if got := combatTestProtagonistHealth(t, store, storyID); got != healthBefore {
		t.Fatalf("opening spent health: %v -> %v", healthBefore, got)
	}
	if got := combatTestProtagonistMana(t, store, storyID); got != manaBefore {
		t.Fatalf("opening spent mana: %v -> %v", manaBefore, got)
	}
	for _, combatant := range opened.Session.Combatants {
		if combatant.Attributes.Health != combatant.Attributes.HealthMax ||
			combatant.Attributes.Mana != combatant.Attributes.ManaMax {
			t.Fatalf("combatant %s must stand at its own pools: %#v", combatant.ID, combatant.Attributes)
		}
	}

	// The history is empty: no exchange has been played.
	history, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("an opening commits no round, got %d", len(history))
	}

	// But the fight itself is committed and readable after a restart, which is
	// what lets a reopened window wait on the same first action.
	reopened := NewStore(store.root)
	sessions, err := reopened.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[opened.Session.ID].Round != 1 {
		t.Fatalf("the encounter must survive a restart waiting on round 1: %#v", sessions)
	}
}

// TestAnOpeningRetryReadsBackTheEncounter is §七.2 for the opening: the repeated
// open answers with the committed encounter and still settles nothing.
func TestAnOpeningRetryReadsBackTheEncounter(t *testing.T) {
	store, storyID := combatTestStory(t)
	request := BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	}
	first, err := store.StartBattle(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.StartBattle(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.EventID != first.EventID || second.Session.ID != first.Session.ID {
		t.Fatalf("a retry must answer from the committed encounter: %#v vs %#v", second, first)
	}
	if second.Settled {
		t.Fatal("a retry of the opening must not report a settled round")
	}
}

// TestResolveBattleRoundAdvancesTheLiveBattle proves a round played against an
// open battle moves that battle rather than starting a new one.
func TestResolveBattleRoundAdvancesTheLiveBattle(t *testing.T) {
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
	// The session a resolved round returns is the one the next round will be
	// fought at, so Round counts the exchange still to come.
	roundsSettled := opened.Session.Round - 1

	second, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: sessionID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-2", CommandHash: "hash-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Session.ID != sessionID {
		t.Fatalf("a round must advance the battle it names: got %s want %s", second.Session.ID, sessionID)
	}
	if second.Outcome.Round != roundsSettled+1 {
		t.Fatalf("the round played must be the one the battle was waiting for: settled %d, played %d",
			roundsSettled, second.Outcome.Round)
	}
	if second.Session.Round != roundsSettled+2 {
		t.Fatalf("one round must advance the battle by exactly one exchange: was waiting on %d, now %d",
			roundsSettled+1, second.Session.Round)
	}

	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("playing a round must not open a second battle: %#v", sessions)
	}
}

// TestResolveBattleRoundRefusesAnUnknownBattle keeps the window from inventing a
// fight: a session id that is not live is an error, not a new battle.
func TestResolveBattleRoundRefusesAnUnknownBattle(t *testing.T) {
	store, storyID := combatTestStory(t)

	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: "cbt-not-a-battle",
		CommandID: "cmd-x", CommandHash: "hash-x",
	}); err == nil {
		t.Fatal("advancing a battle that is not live must be refused")
	}
}

// TestBattleRoundIsIdempotentOnItsCommand is §七.2 for the round path: the same
// command must not resolve twice, and — because the draws are derived from the
// command identity — must not re-roll either.
func TestBattleRoundIsIdempotentOnItsCommand(t *testing.T) {
	store, storyID := combatTestStory(t)

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-2", CommandHash: "hash-2",
	}
	first, err := store.ResolveBattleRound(storyID, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ResolveBattleRound(storyID, request)
	if err != nil {
		t.Fatalf("a retry of the same round must not fail: %v", err)
	}
	if second.EventID != first.EventID {
		t.Fatalf("a retry must report the committed round: first=%s second=%s", first.EventID, second.EventID)
	}
	// The retry is a read, so it must not have advanced the battle again.
	if second.Session.Round != first.Session.Round {
		t.Fatalf("a retry must not advance the battle: first round=%d second round=%d",
			first.Session.Round, second.Session.Round)
	}

	// One round was committed, not two. The opening is the encounter, not an
	// exchange, so it is not part of the round history.
	history, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("one played round is one committed exchange, got %d", len(history))
	}
}

// TestBattleRoundRefusesAReusedCommandForADifferentRound keeps one identity from
// standing for two different rounds. Either answer would be wrong, so neither is
// given.
func TestBattleRoundRefusesAReusedCommandForADifferentRound(t *testing.T) {
	store, storyID := combatTestStory(t)

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-2", CommandHash: "hash-2",
	}); err != nil {
		t.Fatal(err)
	}
	// The same command identity, a different declared round.
	_, err = store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionRest},
		},
		CommandID: "cmd-2", CommandHash: "hash-different",
	})
	if err == nil {
		t.Fatal("one command identity used for two different rounds must be refused")
	}
	if !errors.Is(err, ErrCombatCommandConflict) {
		t.Fatalf("the refusal must name the command conflict, got %v", err)
	}
}

// TestCombatHistoryReturnsTheCommittedRoundsOldestFirst is §七.1's "能回看回合":
// the rounds a player can review are the rounds the journal actually holds, in
// the order they were committed.
func TestCombatHistoryReturnsTheCommittedRoundsOldestFirst(t *testing.T) {
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
	for round := 2; round <= 3; round++ {
		if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
			BranchID: "main", SessionID: sessionID,
			Actions: map[string]cultivation.CombatAction{
				DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			},
			CommandID: commandIDFor(round), CommandHash: "hash-round",
		}); err != nil {
			t.Fatal(err)
		}
	}

	history, err := store.CombatHistory(storyID, "main", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("two played rounds are two committed exchanges, got %d", len(history))
	}
	for index, round := range history {
		if round.Outcome.Round != index+1 {
			t.Fatalf("history must be oldest first: index %d carries round %d", index, round.Outcome.Round)
		}
		if round.EventID == "" {
			t.Fatalf("every committed round must name its record: %#v", round)
		}
	}

	// A battle fought on the same branch must not appear in another's history.
	if other, err := store.CombatHistory(storyID, "main", "cbt-some-other-fight"); err != nil {
		t.Fatal(err)
	} else if len(other) != 0 {
		t.Fatalf("a different battle's rounds must not be reported: %#v", other)
	}
}

// TestBattleHistorySurvivesAJournalRebuild proves the rounds a player reviews are
// read from the journal rather than from a process-local log, so a restart shows
// the same history.
func TestBattleHistorySurvivesAJournalRebuild(t *testing.T) {
	store, storyID := combatTestStory(t)

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The opening alone is not a round, so the fixture plays one to have a
	// committed exchange for the restart to reproduce.
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-first", CommandHash: "hash-first",
	}); err != nil {
		t.Fatal(err)
	}
	before, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("the played round must be committed")
	}

	// Reopen the store from the same root, which is the restart the window must
	// survive.
	reopened := NewStore(store.root)
	after, err := reopened.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("a restart must show the same rounds: before=%d after=%d", len(before), len(after))
	}
	for index := range before {
		if after[index].EventID != before[index].EventID {
			t.Fatalf("round %d differs across a restart: %s vs %s", index, before[index].EventID, after[index].EventID)
		}
	}
}

// TestSettledBattleIsNotAdvancedAgain is §七.8: once a battle has reached a
// result, the result stands. A reopened window shows it rather than offering
// another exchange, so the call reports Settled false and commits nothing.
func TestSettledBattleIsNotAdvancedAgainThroughTheRoundAPI(t *testing.T) {
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

	// Drive the fight to a result. The loop is bounded so a rules change that
	// never ends the battle fails the test rather than hanging it.
	ended := opened.Session.Ended
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
		ended = result.Session.Ended
	}
	if !ended {
		t.Fatal("the fixture must be able to finish a battle against an even opponent")
	}
	committed, err := store.CombatHistory(storyID, "main", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	roundsBefore := len(committed)

	result, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: sessionID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-after-the-end", CommandHash: "hash-after",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Settled {
		t.Fatal("a battle that has reached a result must not resolve another round")
	}
	if !result.Session.Ended {
		t.Fatalf("the settled result must stand: %#v", result.Session)
	}

	after, err := store.CombatHistory(storyID, "main", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != roundsBefore {
		t.Fatalf("a settled battle must commit nothing more: before=%d after=%d", roundsBefore, len(after))
	}
}

// TestEnemyDecidesFromItsOwnLearnedTechniques is §七.4's floor on the write path:
// an enemy the caller did not act for is asked what it does, and the answer comes
// from the enemy's own learned set rather than from the registry's whole table.
//
// The rival is given exactly one learned technique, and the published table holds
// a much stronger one it never learned. A decision that reached into the registry
// would pick the stronger technique; the assertion is that it does not.
func TestEnemyDecidesFromItsOwnLearnedTechniques(t *testing.T) {
	store, storyID := combatTestStoryWithRivalKnowing(t, "fire_bolt")

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	rival := combatTestCombatant(t, opened.Session, "rival")
	if !combatantKnowsSkill(rival, "fire_bolt") {
		t.Fatalf("the fixture must actually give the rival a learned technique: %#v", rival.Skills)
	}

	// The caller declares only its own side. Whatever the rival does must be the
	// decision function's answer.
	actions := battleActions(opened.Session, map[string]cultivation.CombatAction{
		DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
	})
	enemyAction, ok := actions["rival"]
	if !ok {
		t.Fatalf("an enemy left undeclared must still be given an action: %#v", actions)
	}
	if enemyAction.Kind != cultivation.ActionTechnique {
		t.Fatalf("an unhurt enemy with a payable technique must use it: %#v", enemyAction)
	}
	if enemyAction.Skill.ID != "fire_bolt" {
		t.Fatalf("the enemy must use the technique it learned, got %q", enemyAction.Skill.ID)
	}
	// The technique it never learned is stronger, so a decision that consulted the
	// registry instead of the enemy's own list would have chosen it.
	strongest, ok := cultivation.SkillByID("meridian_secret_blade")
	if !ok {
		t.Fatal("the published table must contain the strongest technique")
	}
	if strongest.Power <= enemyAction.Skill.Power {
		t.Fatal("the fixture assumes the unlearned technique is the stronger one")
	}
	if combatantKnowsSkill(rival, "meridian_secret_blade") {
		t.Fatal("the rival must not know the technique the registry holds")
	}
}

// TestDeclaredEnemyActionIsNotOverridden keeps a scripted opponent possible: a
// caller that declares the enemy's action still wins.
func TestDeclaredEnemyActionIsNotOverridden(t *testing.T) {
	store, storyID := combatTestStory(t)

	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  "cmd-open", CommandHash: "hash-open",
	})
	if err != nil {
		t.Fatal(err)
	}
	actions := battleActions(opened.Session, map[string]cultivation.CombatAction{
		"rival": {Kind: cultivation.ActionRest},
	})
	if actions["rival"].Kind != cultivation.ActionRest {
		t.Fatalf("a declared enemy action must not be replaced: %#v", actions["rival"])
	}
}

// TestDeclaredTechniqueMustBeLearned is §6 of the casting design on the write
// path: 秘籍存在、技能被登记、人物已学 are three different facts, and naming a
// published technique is not the same as having learned it.
//
// The registry holds meridian_secret_blade at more than twice the power of
// anything the protagonist has studied. Before this was enforced, a caller could
// declare it and have it cast at full power, because the lookup that accepted it
// only proved the definition exists.
func TestDeclaredTechniqueMustBeLearned(t *testing.T) {
	store, storyID := combatTestStory(t)
	unlearned, ok := cultivation.SkillByID("meridian_secret_blade")
	if !ok {
		t.Fatal("the published table must contain the technique")
	}
	plain := combatTestStrike(t)
	if unlearned.Power <= plain.Power {
		t.Fatal("the fixture assumes the unlearned technique is the stronger one")
	}

	opened := combatTestOpenBattle(t, store, storyID, "cmd-open", "hash-open")
	_, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: unlearned, TargetID: "rival"},
		},
		CommandID: "cmd-round-1", CommandHash: "hash-round-1",
	})
	if !errors.Is(err, ErrCombatTechniqueNotLearned) {
		t.Fatalf("a technique the combatant never learned must be refused, got %v", err)
	}
	// Nothing is settled or committed by a refused round, so the battle stands
	// where the opening left it and the player is not charged for the attempt.
	combats, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	live, ok := combats[opened.Session.ID]
	if !ok || live.Round != 1 {
		t.Fatalf("a refused round must leave the encounter waiting on round 1: %#v", combats)
	}
	history, err := store.CombatHistory(storyID, "main", opened.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("a refused round must commit nothing, got %d", len(history))
	}
}

// TestOrdinaryAttackNeedsNoLearning is the other half of the same rule: the plain
// attack is the one action that requires no study, so a character who has learned
// nothing can still fight. A check that demanded a learned entry for every
// technique would leave a starting character unable to act at all.
func TestOrdinaryAttackNeedsNoLearning(t *testing.T) {
	store, storyID := combatTestStory(t)

	opened := combatTestOpenBattle(t, store, storyID, "cmd-open", "hash-open")
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		CommandID: "cmd-round-1", CommandHash: "hash-round-1",
	})
	if err != nil {
		t.Fatalf("the ordinary attack must always be available: %v", err)
	}
	hero := combatTestCombatant(t, opened.Session, DefaultActorID)
	if len(hero.Skills) != 0 {
		t.Fatalf("the fixture assumes the protagonist has learned nothing: %#v", hero.Skills)
	}
	if len(played.Outcome.Steps) == 0 {
		t.Fatal("the round must have settled an exchange")
	}
	if played.Outcome.Round != 1 {
		t.Fatalf("the first played exchange is round 1, got %d", played.Outcome.Round)
	}
}

// TestLearnedTechniqueIsAccepted proves the check gates on the real learned set
// rather than refusing paid techniques wholesale: the rival genuinely learned
// fire_bolt, so declaring it is legal.
func TestLearnedTechniqueIsAccepted(t *testing.T) {
	store, storyID := combatTestStoryWithRivalKnowing(t, "fire_bolt")
	learned, ok := cultivation.SkillByID("fire_bolt")
	if !ok {
		t.Fatal("the published table must contain the technique")
	}

	opened := combatTestOpenBattle(t, store, storyID, "cmd-open", "hash-open")
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
			"rival":        {Kind: cultivation.ActionTechnique, Skill: learned, TargetID: DefaultActorID},
		},
		CommandID: "cmd-round-1", CommandHash: "hash-round-1",
	}); err != nil {
		t.Fatalf("a technique the combatant really learned must be accepted: %v", err)
	}
	rival := combatTestCombatant(t, opened.Session, "rival")
	if !combatantKnowsSkill(rival, "fire_bolt") {
		t.Fatalf("the fixture must give the rival the technique: %#v", rival.Skills)
	}
}

// combatantKnowsSkill reports whether a combatant's own learned set contains a
// technique.
func combatantKnowsSkill(combatant cultivation.Combatant, skillID string) bool {
	for _, skill := range combatant.Skills {
		if skill.ID == skillID {
			return true
		}
	}
	return false
}

// combatTestStoryWithRivalKnowing is combatTestStory with the rival materialized
// already knowing one technique.
//
// The learned set is seeded through the rival's initial actor rather than through
// a turn's state updates, for the same reason the starting realm is: 已学功法 is a
// field the learning settlement owns, and a character's first materialization is
// the one place the rules allow it to arrive as a story fact rather than as a
// settlement's write.
func combatTestStoryWithRivalKnowing(t *testing.T, skillID string) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianLearnedSkillsField())
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: fields},
		{ID: "rival", Name: "对手", Fields: append([]ActorStateField{}, fields...)},
	}}
	system.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: map[string]any{xiuxianFieldRealm: stage}},
		{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{
			xiuxianFieldRealm: stage,
			xiuxianFieldLearnedSkills: map[string]any{
				"已学之术": map[string]any{"定义": skillID, "版本": 1, "掌握程度": "入门"},
			},
		}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。",
		TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}

// commandIDFor builds a distinct command identity per round. It is a plain
// formatter rather than the store's id minter, so a test's identities are
// readable in a failure message.
func commandIDFor(round int) string {
	return "cmd-round-" + strconv.Itoa(round)
}

// combatTestOpenBattle establishes an encounter and fails the test if the opening
// itself did not succeed. Most tests below are about what a played round does, so
// they open through here rather than repeating the same six fields.
func combatTestOpenBattle(t *testing.T, store *Store, storyID, commandID, hash string) BattleRoundOutcome {
	t.Helper()
	opened, err := store.StartBattle(storyID, BattleStartRequest{
		BranchID: "main", WorldTime: "第1日 辰时",
		Combatants: combatTestSpecs(),
		CommandID:  commandID, CommandHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	return opened
}

func TestRejectedActionDoesNotCommitOrAdvancePreparedBattle(t *testing.T) {
	store, storyID := combatTestStoryWithProtagonistKnowing(t, "meridian_secret_blade")
	secret, found := cultivation.SkillByID("meridian_secret_blade")
	if !found {
		t.Fatal("the published catalog must contain the preparation fixture")
	}
	opened := combatTestOpenBattle(t, store, storyID, "prep-open", "prep-open-hash")
	// The preparation is begun by a legal first exchange, which is what the round
	// path now plays.
	if _, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: secret, TargetID: "rival"},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: "prep-start", CommandHash: "prep-start-hash",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: "missing-dose"},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: "invalid-alt", CommandHash: "invalid-alt-hash",
	})
	if !errors.Is(err, ErrCombatActionRejected) {
		t.Fatalf("missing dose must be a correctable rejected action, got %v", err)
	}
	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("rejected action changed the battle index: %#v", sessions)
	}
	var live cultivation.CombatSession
	for _, value := range sessions {
		live = value
	}
	if live.Round != 2 || live.Preparations[DefaultActorID].Skill.ID != "meridian_secret_blade" {
		t.Fatalf("rejected action must preserve round and paid preparation: %#v", live)
	}
	if live.CombatantByID("rival").Attributes.Health != 200 {
		t.Fatal("enemy action must not be committed after player rejection")
	}
}

// §9's acceptance item: 一项必要法器缺失时被合理拒绝. 冲脉秘剑's body requirement is
// held_usable_blade_with_real_equipment_provenance, so the blade has to be in
// hand. The first played round is the case that matters — a rule that answered
// the requirement from the declaration rather than from the combatant refused it
// there even when the blade really was held.
func TestAFocusTechniqueIsCastableInTheOpeningRoundWhenTheBladeIsHeld(t *testing.T) {
	store, storyID := combatTestStoryWithProtagonistKnowingAndBlade(t, "meridian_secret_blade", true)
	secret, found := cultivation.SkillByID("meridian_secret_blade")
	if !found {
		t.Fatal("the published catalog must contain the focus technique")
	}
	opened := combatTestOpenBattle(t, store, storyID, "blade-open", "blade-open-hash")
	played, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: secret, TargetID: "rival"},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: "blade-held", CommandHash: "blade-held-hash",
	})
	if err != nil {
		t.Fatalf("a held blade must not be overridden by the declaration: %v", err)
	}
	player := played.Session.CombatantByID(DefaultActorID)
	prepared, preparing := played.Session.Preparations[DefaultActorID]
	if !preparing || prepared.Skill.ID != secret.ID {
		t.Fatalf("the first played round must begin the paid preparation: %#v", played.Session)
	}
	if player.Attributes.Mana != 96 {
		t.Errorf("mana after the first round = %d, want 96 (160 − 64)", player.Attributes.Mana)
	}
}

// The other half: without the blade the same technique is refused for the real
// reason and costs nothing, rather than resolving at a cost the character never
// paid.
func TestAFocusTechniqueIsRefusedWithoutTheBladeAndSpendsNothing(t *testing.T) {
	store, storyID := combatTestStoryWithProtagonistKnowingAndBlade(t, "meridian_secret_blade", false)
	secret, found := cultivation.SkillByID("meridian_secret_blade")
	if !found {
		t.Fatal("the published catalog must contain the focus technique")
	}
	opened := combatTestOpenBattle(t, store, storyID, "blade-open", "blade-open-hash")
	_, err := store.ResolveBattleRound(storyID, BattleRoundRequest{
		BranchID: "main", SessionID: opened.Session.ID,
		Actions: map[string]cultivation.CombatAction{
			DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: secret, TargetID: "rival"},
			"rival":        {Kind: cultivation.ActionRest},
		}, CommandID: "blade-missing", CommandHash: "blade-missing-hash",
	})
	if !errors.Is(err, ErrCombatActionRejected) {
		t.Fatalf("a focus technique without its focus must be a correctable refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), cultivation.ReasonImplementRequired) {
		t.Fatalf("the refusal must name the missing focus: %v", err)
	}
	// The refusal committed nothing: the encounter still stands at its opening
	// pools and waits on the same first round.
	sessions, err := store.CombatSessions(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	live, ok := sessions[opened.Session.ID]
	if !ok || live.Round != 1 {
		t.Fatalf("a refused first round must leave the encounter waiting on round 1: %#v", sessions)
	}
	if player := live.CombatantByID(DefaultActorID); player.Attributes.Mana != player.Attributes.ManaMax {
		t.Fatalf("a refused cast must spend nothing: %#v", player.Attributes)
	}
}

func combatTestStoryWithProtagonistKnowing(t *testing.T, skillID string) (*Store, string) {
	t.Helper()
	return combatTestStoryWithProtagonistKnowingAndBlade(t, skillID, true)
}

// combatTestStoryWithProtagonistKnowingAndBlade is the same fixture with the
// focus a technique needs either in hand or deliberately absent.
func combatTestStoryWithProtagonistKnowingAndBlade(t *testing.T, skillID string, withBlade bool) (*Store, string) {
	t.Helper()
	spec := xiuxianActorStatePresetSpec()
	fields := append([]ActorStateField{}, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	fields = append(fields, xiuxianLearnedSkillsField())
	stage := cultivation.StageKey(cultivation.Qi(6))
	system := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{
		{ID: DefaultActorID, Name: "主角", Fields: fields},
		{ID: "rival", Name: "对手", Fields: append([]ActorStateField{}, fields...)},
	}}
	protagonist := map[string]any{
		xiuxianFieldRealm:         stage,
		xiuxianFieldLearnedSkills: map[string]any{"已学之术": map[string]any{"定义": skillID, "版本": 1, "掌握程度": "入门"}},
	}
	if withBlade {
		protagonist[xiuxianFieldImplement] = map[string]any{
			"名称": "青锋剑", "品级": "中品", "稀有度": "常见", "主要属性预算": float64(5),
		}
	}
	system.InitialActors = []ActorStateInitialActor{
		{ID: DefaultActorID, Name: "主角", TemplateID: DefaultActorID, State: protagonist},
		{ID: "rival", Name: "对手", TemplateID: "rival", State: map[string]any{xiuxianFieldRealm: stage}},
	}
	store := NewStore(t.TempDir())
	story, err := store.CreateStory(CreateStoryRequest{Title: "战斗记录", ActorState: &system})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, AppendTurnWithStateRequest{
		BranchID: "main", User: "入定", Narrative: "主角与对手对峙。", TurnResult: &TurnResult{Choices: testTurnChoices()},
	}); err != nil {
		t.Fatal(err)
	}
	return store, story.ID
}
