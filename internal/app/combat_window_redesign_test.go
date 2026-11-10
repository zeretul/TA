package app

import (
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests pin the two server-side facts the battle window's redesign depends
// on: that a technique's effect sentence can be worded from what the rules
// published, and that a round settled without its prose is distinguishable from
// a round whose prose failed. Neither is visible in the UI alone, and both would
// silently degrade into invented copy or a false "failed" if the read regressed.

// TestCombatWindowSendsThePublishedEffectOfEachTechnique is §3's "一句已实现效果"
// as an assertion. The window words that sentence from these properties, so a
// technique whose definition changed cannot leave an old sentence behind, and a
// technique the engine stopped giving a shield stops claiming one.
func TestCombatWindowSendsThePublishedEffectOfEachTechnique(t *testing.T) {
	application, storyID := combatTestAppWithPlayerMana(t, 200, "shield", "meridian_secret_blade")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-effects", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	byID := map[string]CombatWindowSkill{}
	for _, skill := range player.Skills {
		byID[skill.ID] = skill
	}

	shield, ok := byID["shield"]
	if !ok {
		t.Fatalf("the player learned 护体诀, so it must be listed: %#v", player.Skills)
	}
	if shield.Effects.ShieldFraction != 0.20 || shield.Effects.DurationExchanges != 2 {
		t.Fatalf("the shield's size and term must be the rules' own: %#v", shield.Effects)
	}

	blade, ok := byID["meridian_secret_blade"]
	if !ok {
		t.Fatalf("the player learned 冲脉秘剑, so it must be listed: %#v", player.Skills)
	}
	if blade.Effects.ExecutionActions != 2 {
		t.Fatalf("a prepared technique must say it takes two exchanges: %#v", blade.Effects)
	}
	if blade.Effects.IgnoreDefense <= 0 {
		t.Fatalf("the technique really bypasses defense, so the window must be able to say so: %#v", blade.Effects)
	}
	if !blade.Effects.RequiresFocus || !blade.Effects.LeavesResidual {
		t.Fatalf("both published conditions must reach the window: %#v", blade.Effects)
	}
}

// TestCombatWindowNamesTheLastingEffectOfEachVariant covers the other half of the
// same boundary: a form that only reported its power would read as strictly
// worse than the base form, because the thing it buys would be invisible.
func TestCombatWindowNamesTheLastingEffectOfEachVariant(t *testing.T) {
	application, storyID := combatTestAppWithPlayerMana(t, 200, "fire_bolt")
	window, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-variants", Combatants: combatTestSpecs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	player := combatWindowSide(t, window, interactive.DefaultActorID)
	var bolt CombatWindowSkill
	for _, skill := range player.Skills {
		if skill.ID == "fire_bolt" {
			bolt = skill
		}
	}
	if len(bolt.Variants) == 0 {
		t.Fatalf("火弹 publishes a burning form: %#v", bolt)
	}
	for _, variant := range bolt.Variants {
		if variant.Effect != "burn" {
			t.Fatalf("the burning form must name the effect it applies: %#v", variant)
		}
	}
}

// TestCombatWindowMarksACommittedRoundAsAwaitingItsProse is §3's "待生成" state.
//
// The distinction matters because the two cases read differently to a player: a
// round whose prose was never attempted is still being written, while one whose
// call failed carries a reason. The read derives the first from the journal, so a
// process interrupted between settlement and prose lands in it too.
func TestCombatWindowMarksACommittedRoundAsAwaitingItsProse(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	first, _ := combatNarrationTestFirstRound(t, application, storyID)

	window, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: first.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if window.NarrationState != CombatNarrationPending {
		t.Fatalf("a committed round nobody asked to write must read as pending, got %q", window.NarrationState)
	}
	if window.NarrationError == "" {
		t.Fatal("the pending state must still offer the retry entry")
	}
}

// TestSkippingNarrationSettlesTheSameRound covers the split the redesign relies
// on: the window asks for settlement alone, shows it, then asks for the passage
// separately. The round settled must be the one an inline call would have
// settled — skipping may never change what happened, only when it is described.
func TestSkippingNarrationSettlesTheSameRound(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "cmd-open", Combatants: combatTestSpecs(),
		SkipNarration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "cmd-skip",
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
		SkipNarration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if settled.LastRound == nil || settled.LastRound.Outcome.Round != 1 {
		t.Fatalf("the round must still be settled and reported: %#v", settled.LastRound)
	}
	if settled.NarrationState != CombatNarrationPending {
		t.Fatalf("a skipped passage must read as pending, got %q", settled.NarrationState)
	}
	if settled.NarrationError == "" {
		t.Fatal("the response must still offer the missing passage")
	}

	// The prose is then written against the round that was committed. This call
	// carries no runtime, so it fails — and the round must still stand.
	if _, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
		StoryID: storyID, BranchID: "main", SessionID: settled.SessionID,
		RoundEventID: settled.LastRound.EventID, CommandID: "cmd-skip",
	}); err == nil {
		t.Fatal("this fixture has no model runtime, so writing the passage must fail")
	}
	after, err := application.CombatWindow(CombatWindowRequest{
		StoryID: storyID, BranchID: "main", SessionID: settled.SessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.LastRound == nil || after.LastRound.EventID != settled.LastRound.EventID {
		t.Fatalf("a failed passage must not change or replay the round: %#v", after.LastRound)
	}
	if after.Round != 2 {
		t.Fatalf("a failed passage must not advance the battle; next exchange is %d", after.Round)
	}
}
