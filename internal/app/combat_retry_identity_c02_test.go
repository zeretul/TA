package app

import (
	"errors"
	"fmt"
	"testing"

	"ta/internal/cultivation"
	"ta/internal/interactive"
)

// These tests cover the two retry gaps C01/F06 and C01/F10 describe. They need no
// model: every narration case either commits the passage directly through the
// store, or relies on the fixture having no model runtime so that any attempt to
// generate a passage fails visibly instead of being paid for.

const c02Branch = "main"

// c02PlayTwoRounds opens one battle and settles two exchanges of it without
// writing prose, so both rounds are committed and neither has a passage.
//
// The opening is not a round, so the first returned window is the one that
// follows the first *played* exchange; the encounter's own state is available
// through sessionID.
func c02PlayTwoRounds(t *testing.T) (*App, string, CombatWindow, CombatWindow) {
	t.Helper()
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "c02-open", Combatants: combatTestSpecs(), SkipNarration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened.LastRound != nil || len(opened.History) != 0 {
		t.Fatalf("opening a battle must not settle a round: %#v", opened)
	}
	first, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "c02-round-1", SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, SessionID: opened.SessionID, CommandID: "c02-round-2", SkipNarration: true,
		Actions: map[string]cultivation.CombatAction{
			interactive.DefaultActorID: {Kind: cultivation.ActionTechnique, Skill: combatTestStrike(t), TargetID: "rival"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.LastRound == nil || second.LastRound == nil || first.LastRound.EventID == second.LastRound.EventID {
		t.Fatalf("the fixture must commit two distinct rounds: %#v / %#v", first.LastRound, second.LastRound)
	}
	return application, storyID, first, second
}

// c02Commit stores one passage the way a successful generation would, under the
// same fingerprint NarrateCombatRound computes.
func c02Commit(t *testing.T, application *App, storyID, sessionID, roundEventID, commandID, text string) interactive.CombatNarration {
	t.Helper()
	committed, err := application.interactive.AppendCombatNarration(storyID, interactive.AppendCombatNarrationRequest{
		BranchID: c02Branch, SessionID: sessionID, RoundEventID: roundEventID,
		Narrative: text, StyleID: "source-drama", CommandID: commandID,
		CommandHash: combatNarrationCommandHash(storyID, c02Branch, sessionID, roundEventID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return committed
}

func c02NarrationCount(t *testing.T, application *App, storyID, sessionID string) int {
	t.Helper()
	narrations, err := application.interactive.CombatNarrations(storyID, c02Branch, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return len(narrations)
}

func c02RoundIDs(t *testing.T, application *App, storyID, sessionID string) []string {
	t.Helper()
	history, err := application.interactive.CombatHistory(storyID, c02Branch, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(history))
	for _, record := range history {
		ids = append(ids, record.EventID)
	}
	return ids
}

func c02SameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}

// TestC02NarrationRetryAfterLostResponseReadsTheCommittedPassage covers a passage
// that was committed while its response was lost: the retry carries the same
// request identity and must read the original prose back. The fixture has no
// model runtime, so a retry that tried to generate would fail rather than pass.
func TestC02NarrationRetryAfterLostResponseReadsTheCommittedPassage(t *testing.T) {
	application, storyID, first, _ := c02PlayTwoRounds(t)
	roundID := first.LastRound.EventID

	// Precondition: without a committed passage, this fixture cannot generate one.
	if _, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
		StoryID: storyID, BranchID: c02Branch, SessionID: first.SessionID,
		RoundEventID: roundID, CommandID: "c02-probe",
	}); err == nil {
		t.Fatal("this fixture has no model runtime, so generating a passage must fail")
	}
	if got := c02NarrationCount(t, application, storyID, first.SessionID); got != 0 {
		t.Fatalf("a failed attempt must leave no passage, found %d", got)
	}

	roundsBefore := c02RoundIDs(t, application, storyID, first.SessionID)
	committed := c02Commit(t, application, storyID, first.SessionID, roundID, "c02-stable-id", "第一合的原文。")

	// The same identity again, as the manual retry or a reopened panel sends it.
	for attempt := 0; attempt < 3; attempt++ {
		result, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
			StoryID: storyID, BranchID: c02Branch, SessionID: first.SessionID,
			RoundEventID: roundID, CommandID: "c02-stable-id",
		})
		if err != nil {
			t.Fatalf("a retry under the committed identity must read the passage back: %v", err)
		}
		if !result.Replayed || result.Narrative != "第一合的原文。" || result.EventID != committed.EventID {
			t.Fatalf("the retry must return the original passage record: %#v", result)
		}
	}
	if got := c02NarrationCount(t, application, storyID, first.SessionID); got != 1 {
		t.Fatalf("retries must not write a second passage, found %d", got)
	}
	if after := c02RoundIDs(t, application, storyID, first.SessionID); !c02SameIDs(roundsBefore, after) {
		t.Fatalf("a narration retry must not touch the settled rounds: %v -> %v", roundsBefore, after)
	}
}

// TestC02NarrationWithoutAPassageStaysRetryable covers a failed generation: no
// passage exists, the round stands, and the same identity can still be used to
// commit the passage later.
func TestC02NarrationWithoutAPassageStaysRetryable(t *testing.T) {
	application, storyID, first, _ := c02PlayTwoRounds(t)
	roundID := first.LastRound.EventID
	roundsBefore := c02RoundIDs(t, application, storyID, first.SessionID)

	request := CombatNarrationRequest{
		StoryID: storyID, BranchID: c02Branch, SessionID: first.SessionID,
		RoundEventID: roundID, CommandID: "c02-stable-id",
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := application.NarrateCombatRound(t.Context(), request); err == nil {
			t.Fatal("this fixture has no model runtime, so the attempt must fail")
		}
	}
	if got := c02NarrationCount(t, application, storyID, first.SessionID); got != 0 {
		t.Fatalf("failed attempts must not leave a passage, found %d", got)
	}
	if after := c02RoundIDs(t, application, storyID, first.SessionID); !c02SameIDs(roundsBefore, after) {
		t.Fatalf("a failed narration must leave the settled rounds as they were: %v -> %v", roundsBefore, after)
	}

	// The identity is not spent by the failures: a later success under it commits
	// and is then read back.
	c02Commit(t, application, storyID, first.SessionID, roundID, "c02-stable-id", "补写成功的原文。")
	result, err := application.NarrateCombatRound(t.Context(), request)
	if err != nil || !result.Replayed || result.Narrative != "补写成功的原文。" {
		t.Fatalf("the identity must remain usable after failed attempts: %#v, %v", result, err)
	}
}

// TestC02NarrationIdentityIsNotReusedForAnotherRound keeps one round's identity
// from answering for a different round, and keeps a different round's identity
// from being served the first round's prose.
func TestC02NarrationIdentityIsNotReusedForAnotherRound(t *testing.T) {
	application, storyID, first, second := c02PlayTwoRounds(t)
	c02Commit(t, application, storyID, first.SessionID, first.LastRound.EventID, "c02-id-round-1", "第一合。")

	// Round 1's identity pointed at round 2 is a conflict, not round 1's prose.
	_, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
		StoryID: storyID, BranchID: c02Branch, SessionID: second.SessionID,
		RoundEventID: second.LastRound.EventID, CommandID: "c02-id-round-1",
	})
	if !errors.Is(err, interactive.ErrCombatCommandConflict) {
		t.Fatalf("one round's identity must not answer for another round, got %v", err)
	}
	// Round 2's own identity has nothing committed, so it must not be replayed.
	if _, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
		StoryID: storyID, BranchID: c02Branch, SessionID: second.SessionID,
		RoundEventID: second.LastRound.EventID, CommandID: "c02-id-round-2",
	}); err == nil {
		t.Fatal("round 2 has no passage and this fixture cannot generate one, so the call must fail")
	}
	narrations, err := application.interactive.CombatNarrations(storyID, c02Branch, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrations) != 1 || narrations[first.LastRound.EventID].Narrative != "第一合。" {
		t.Fatalf("only round 1 may have a passage: %#v", narrations)
	}
}

// TestC02ExplicitNewCommandStillWritesANewVersion keeps the existing ability to
// ask for a different passage with a new command ID, without disturbing the
// original request identity.
func TestC02ExplicitNewCommandStillWritesANewVersion(t *testing.T) {
	application, storyID, first, _ := c02PlayTwoRounds(t)
	roundID := first.LastRound.EventID
	c02Commit(t, application, storyID, first.SessionID, roundID, "c02-stable-id", "原文。")
	rewrite := c02Commit(t, application, storyID, first.SessionID, roundID, "c02-explicit-rewrite", "新版本。")
	if rewrite.Narrative != "新版本。" {
		t.Fatalf("an explicit new command must commit its own passage: %#v", rewrite)
	}

	narrations, err := application.interactive.CombatNarrations(storyID, c02Branch, first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if narrations[roundID].Narrative != "新版本。" {
		t.Fatalf("the window must show the newest explicit version: %#v", narrations[roundID])
	}
	// The original identity still reads the original passage back.
	original, err := application.NarrateCombatRound(t.Context(), CombatNarrationRequest{
		StoryID: storyID, BranchID: c02Branch, SessionID: first.SessionID,
		RoundEventID: roundID, CommandID: "c02-stable-id",
	})
	if err != nil || !original.Replayed || original.Narrative != "原文。" {
		t.Fatalf("the original identity must still read the original passage: %#v, %v", original, err)
	}
}

// c02Actions builds one action map, inserting in the order given so two maps can
// hold the same content built in different orders.
func c02Actions(order []string, actions map[string]cultivation.CombatAction) map[string]cultivation.CombatAction {
	out := make(map[string]cultivation.CombatAction, len(order))
	for _, actorID := range order {
		out[actorID] = actions[actorID]
	}
	return out
}

func c02Hash(actions map[string]cultivation.CombatAction) string {
	return combatRoundCommandHash(CombatRoundRequest{StoryID: "s", SessionID: "cbt-1", Actions: actions}, c02Branch)
}

// TestC02FingerprintCoversTheItemAndTheOfferThatTakeEffect checks the fingerprint
// itself: same content is stable whatever the map's construction order, and a
// different copy or offer is a different command.
func TestC02FingerprintCoversTheItemAndTheOfferThatTakeEffect(t *testing.T) {
	content := map[string]cultivation.CombatAction{
		"hero":  {Kind: cultivation.ActionUseItem, ItemInstanceID: "pill-a"},
		"rival": {Kind: cultivation.ActionSurrender, OfferText: "spare me"},
		"third": {Kind: cultivation.ActionRest},
	}
	orders := [][]string{{"hero", "rival", "third"}, {"third", "rival", "hero"}, {"rival", "third", "hero"}}
	want := c02Hash(c02Actions(orders[0], content))
	for attempt := 0; attempt < 20; attempt++ {
		for _, order := range orders {
			if got := c02Hash(c02Actions(order, content)); got != want {
				t.Fatalf("the fingerprint changed with construction order %v: %q vs %q", order, got, want)
			}
		}
	}

	changedItem := map[string]cultivation.CombatAction{
		"hero":  {Kind: cultivation.ActionUseItem, ItemInstanceID: "pill-b"},
		"rival": content["rival"],
		"third": content["third"],
	}
	if c02Hash(changedItem) == want {
		t.Fatal("a different carried copy must be a different command")
	}
	changedOffer := map[string]cultivation.CombatAction{
		"hero":  content["hero"],
		"rival": {Kind: cultivation.ActionSurrender, OfferText: "forgive me"},
		"third": content["third"],
	}
	if c02Hash(changedOffer) == want {
		t.Fatal("a different offer text must be a different command")
	}

	// The engine trims the copy before matching it, so a padded identifier is the
	// same copy and must be the same command.
	padded := map[string]cultivation.CombatAction{
		"hero":  {Kind: cultivation.ActionUseItem, ItemInstanceID: "  pill-a "},
		"rival": content["rival"],
		"third": content["third"],
	}
	if c02Hash(padded) != want {
		t.Fatal("a padded copy identifier is the same copy the engine would spend")
	}
}

// TestC02FingerprintIgnoresFieldsTheKindDoesNotUse keeps unrelated fields from
// turning an equivalent retry into a conflict, and keeps the pre-existing format
// for actions that carry neither field.
func TestC02FingerprintIgnoresFieldsTheKindDoesNotUse(t *testing.T) {
	plain := map[string]cultivation.CombatAction{"hero": {Kind: cultivation.ActionRest}}
	noisy := map[string]cultivation.CombatAction{
		"hero": {Kind: cultivation.ActionRest, ItemInstanceID: "pill-a", OfferText: "ignored"},
	}
	if c02Hash(plain) != c02Hash(noisy) {
		t.Fatal("fields the action kind never reads must not change its fingerprint")
	}
	// A use_item with no copy named and a surrender with no text add nothing, so
	// they keep the exact fingerprint they had before these fields were covered.
	for _, action := range []cultivation.CombatAction{
		{Kind: cultivation.ActionUseItem}, {Kind: cultivation.ActionSurrender},
		{Kind: cultivation.ActionRest}, {Kind: cultivation.ActionEscape, TargetID: "rival"},
	} {
		got := combatActionFingerprint(map[string]cultivation.CombatAction{"hero": action})
		legacy := fmt.Sprintf("action\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
			"hero", action.Kind, action.Skill.ID, action.Variant, action.TargetID, "")
		if len(got) != 1 || got[0] != legacy {
			t.Fatalf("an action without the new fields must keep its legacy fingerprint: %q vs %q", got, legacy)
		}
	}
}

// TestC02ReusedCommandWithAnotherItemCopyIsRefused covers the behaviour: the same
// command replays normally, a different carried copy is refused as a conflict, and
// neither the journal nor the stock moves.
func TestC02ReusedCommandWithAnotherItemCopyIsRefused(t *testing.T) {
	application, storyID := combatTestAppCarryingWounded(t, map[string]any{
		"回春丹":  map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
		"回春丹乙": map[string]any{"定义": "pill.restore.health.lower", "数量": float64(3)},
	}, 100)
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "c02-open", Combatants: combatTestSpecs(), SkipNarration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dose := func(instanceID string) CombatRoundRequest {
		return CombatRoundRequest{
			StoryID: storyID, SessionID: opened.SessionID, CommandID: "c02-dose", SkipNarration: true,
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionUseItem, ItemInstanceID: instanceID},
			},
		}
	}
	first, err := application.PlayCombatRound(dose("回春丹"))
	if err != nil {
		t.Fatal(err)
	}
	roundsBefore := c02RoundIDs(t, application, storyID, opened.SessionID)

	replayed, err := application.PlayCombatRound(dose("回春丹"))
	if err != nil {
		t.Fatalf("the same command with the same copy must replay: %v", err)
	}
	if replayed.LastRound == nil || replayed.LastRound.EventID != first.LastRound.EventID {
		t.Fatalf("a replay must answer with the committed round: %#v", replayed.LastRound)
	}

	_, err = application.PlayCombatRound(dose("回春丹乙"))
	if !errors.Is(err, interactive.ErrCombatCommandConflict) {
		t.Fatalf("the same command naming another copy must conflict, got %v", err)
	}
	after, err := application.CombatWindow(CombatWindowRequest{StoryID: storyID, BranchID: c02Branch, SessionID: opened.SessionID})
	if err != nil {
		t.Fatal(err)
	}
	if got := c02RoundIDs(t, application, storyID, opened.SessionID); !c02SameIDs(roundsBefore, got) {
		t.Fatalf("a refused command must leave the journal alone: %v -> %v", roundsBefore, got)
	}
	if spent := combatTestWindowItem(t, after, interactive.DefaultActorID, "回春丹").Quantity; spent != 2 {
		t.Fatalf("the original copy must have been spent exactly once, quantity = %d", spent)
	}
	if untouched := combatTestWindowItem(t, after, interactive.DefaultActorID, "回春丹乙").Quantity; untouched != 3 {
		t.Fatalf("the refused copy must not be spent, quantity = %d", untouched)
	}
}

// TestC02ReusedCommandWithAnotherOfferIsRefused is the same boundary for the text
// a surrender offers.
func TestC02ReusedCommandWithAnotherOfferIsRefused(t *testing.T) {
	application, storyID := combatTestApp(t, "fire_bolt")
	opened, err := application.PlayCombatRound(CombatRoundRequest{
		StoryID: storyID, CommandID: "c02-open", Combatants: combatTestSpecs(), SkipNarration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	offer := func(text string) CombatRoundRequest {
		return CombatRoundRequest{
			StoryID: storyID, SessionID: opened.SessionID, CommandID: "c02-offer", SkipNarration: true,
			Actions: map[string]cultivation.CombatAction{
				interactive.DefaultActorID: {Kind: cultivation.ActionSurrender, OfferText: text},
			},
		}
	}
	first, err := application.PlayCombatRound(offer("求饶"))
	if err != nil {
		t.Fatal(err)
	}
	roundsBefore := c02RoundIDs(t, application, storyID, first.SessionID)

	replayed, err := application.PlayCombatRound(offer("求饶"))
	if err != nil {
		t.Fatalf("the same command with the same offer must replay: %v", err)
	}
	if replayed.SessionID != first.SessionID || replayed.LastRound == nil || replayed.LastRound.EventID != first.LastRound.EventID {
		t.Fatalf("a replay must answer with the committed round: %#v", replayed.LastRound)
	}

	if _, err := application.PlayCombatRound(offer("饶命")); !errors.Is(err, interactive.ErrCombatCommandConflict) {
		t.Fatalf("the same command with another offer must conflict, got %v", err)
	}
	if got := c02RoundIDs(t, application, storyID, first.SessionID); !c02SameIDs(roundsBefore, got) {
		t.Fatalf("a refused command must leave the journal alone: %v -> %v", roundsBefore, got)
	}
	sessions, err := application.interactive.CombatSessions(storyID, c02Branch)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("a refused command must not open another battle: %d", len(sessions))
	}
}
