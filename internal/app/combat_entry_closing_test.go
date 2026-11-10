package app

import (
	"errors"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests close the two ends of the natural battle entry.
//
// The entry itself was already working: a turn creates an opponent, the scene
// declares the attack, and the window derives both sides without anything being
// injected. What was missing was what happens at either end of that path:
//
//   - A fight that reached a result kept being offered by the window, because the
//     declaration survived the settlement and the instruction to clear it is a
//     request rather than a guarantee. A second battle could be opened for a fight
//     already won.
//
//   - The main story could still commit a turn while the fight was live, so the
//     prose could say 沈青倒地不起 while the window still showed round 1 and full
//     pools on both sides. That is the "one dice_check writes the whole battle"
//     gap in its most direct form: the story simply narrates the result the rules
//     have not settled.

// TestASettledFightStopsBeingOffered is the closing end: after the fight settles,
// the window must not offer the same opponent again.
func TestASettledFightStopsBeingOffered(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "沈青"},
		map[string]any{"沈青": "沈青拔刀劈向主角"})

	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "close-open"})
	if err != nil {
		t.Fatal(err)
	}
	current := opened
	for round := 1; round <= 80 && !current.Ended; round++ {
		current, err = application.PlayCombatRound(CombatRoundRequest{
			StoryID: storyID, SessionID: opened.SessionID,
			CommandID: "close-round-" + itoa(round), SkipNarration: true,
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "沈青"},
			},
		})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	if !current.Ended {
		t.Fatal("the fixture must reach a result")
	}

	// The scene no longer declares the fight: the settlement retired it.
	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey != "combatWindow.encounter.noEnemy" {
		t.Fatalf("a settled fight must not still be offered: %#v", window.Encounter)
	}

	// And reopening it is refused rather than starting a second battle.
	if _, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "close-reopen"}); err == nil {
		t.Fatal("a settled fight must not be restartable")
	} else if !errors.Is(err, ErrCombatNoEncounter) {
		t.Fatalf("the refusal must be the encounter refusal: %v", err)
	}
}

// TestTheMainStoryCannotAdvanceThroughALiveFight is the other end: while the
// window holds an unfinished battle, a main-story turn must be refused, and the
// refusal must say where the fight belongs.
func TestTheMainStoryCannotAdvanceThroughALiveFight(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "沈青"},
		map[string]any{"沈青": "沈青拔刀劈向主角"})

	opened, err := application.PlayCombatRound(CombatRoundRequest{StoryID: storyID, CommandID: "live-open"})
	if err != nil {
		t.Fatal(err)
	}
	if opened.Ended {
		t.Fatal("the battle must be live for this test to mean anything")
	}

	_, _, err = application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "一刀砍倒他", Narrative: "沈青倒地不起。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	})
	if !errors.Is(err, interactive.ErrStoryTurnDuringBattle) {
		t.Fatalf("a main-story turn must not commit through a live battle: %v", err)
	}

	// The refusal changed nothing: the fight is still where it was.
	window, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: opened.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if window.Ended || window.Round != opened.Round {
		t.Fatalf("the refusal must not settle or advance the battle: round=%d ended=%t", window.Round, window.Ended)
	}
}

// TestTheDeclaredFightBlocksTheNextTurnButNotTheTurnThatDeclaredIt keeps the
// guard from biting the turn that starts the fight.
//
// The declaration is committed by a turn, and that turn has to be allowed to
// commit: it is the scene saying blows are being exchanged, and it is also the
// turn whose prose describes the attack. Only the turns after it are refused.
func TestTheDeclaredFightBlocksTheNextTurnButNotTheTurnThatDeclaredIt(t *testing.T) {
	application, storyID := combatEngagementFixture(t)

	// This is the turn that declares the attack, and it commits.
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "沈青"},
		map[string]any{"沈青": "沈青拔刀劈向主角"})

	// The next one may not narrate the fight away.
	_, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "迎上去", Narrative: "两人交手数合，主角一剑刺穿沈青。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	})
	if !errors.Is(err, interactive.ErrStoryTurnDuringBattle) {
		t.Fatalf("the turn after the declaration must be refused: %v", err)
	}
}

// TestAThreatThatIsNotAFightLeavesTheStoryAlone is the boundary the task names:
// 威慑、回避和未形成交战的试探仍按原规则处理.
//
// A present, tracked, disliked character with no declaration and no session is an
// ordinary scene. Nothing in it may be refused.
func TestAThreatThatIsNotAFightLeavesTheStoryAlone(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID, "沈青"}, nil)

	if _, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "拔剑吓他一下", Narrative: "主角按住剑柄，沈青退了半步，终究没有动手。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatalf("a threat that never became a fight must be decided by the ordinary rules: %v", err)
	}
}

// TestADeclarationTheWindowCannotOpenDoesNotFreezeTheStory keeps the guard from
// becoming a trap.
//
// A declaration naming someone who is not present is a claim the window cannot
// act on — it would refuse to open. Blocking the story on it would leave the game
// with a fight nobody can fight and no way to advance.
func TestADeclarationTheWindowCannotOpenDoesNotFreezeTheStory(t *testing.T) {
	application, storyID := combatEngagementFixture(t)
	settleCombatScene(t, application.interactive, storyID,
		[]any{interactive.DefaultActorID},
		map[string]any{"陆远": "陆远在别处放了冷箭"})

	window, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID})
	if err != nil {
		t.Fatal(err)
	}
	if window.Encounter == nil || window.Encounter.ReasonKey == "" {
		t.Fatalf("an absent declarer is not an encounter the window can open: %#v", window.Encounter)
	}

	if _, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "循着箭来的方向找去", Narrative: "主角顺着山脊摸过去。",
		TurnResult: &interactive.TurnResult{Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"}},
	}); err != nil {
		t.Fatalf("a fight nobody can open must not stop the story: %v", err)
	}
}
