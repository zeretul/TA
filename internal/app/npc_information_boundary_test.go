package app

import (
	"errors"
	"testing"

	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
)

// This file is the minimal reproduction of the remaining NPC information-boundary
// leaks the audit found by reading the payload paths rather than the interface.
//
// §四 of 战斗建议复核与NPC信息边界 requires the boundary to hold on every payload
// that leaves the process — the character window, the battle window, the
// snapshot, the history, the stream, the nested TurnResult.StateUpdates, and the
// cultivation and rest entries — and forbids reading or steering another
// character's real pools, realm, arts, techniques or stock through an arbitrary
// actor_id. Hiding the entry in the browser is explicitly not the fix.
//
// Each test below fails on the code as it stood and states the rule it checks.

// boundaryTestStory builds a story with the real xiuxian rules and a second
// tracked character, which is what makes both sides carry numbers the rules own.
func boundaryTestStory(t *testing.T) (*App, string) {
	t.Helper()
	return combatTestApp(t, "condensed_blade")
}

// TestCultivationEntryRefusesAnotherActorsPosition is the audit's first leak.
//
// GET /cultivation?actor_id=<npc> used to pass the id straight through to the
// rules' own readers, so the answer carried that character's true realm, banked
// progress and pools. A player must not be able to read another character's
// dossier by naming them.
func TestCultivationEntryRefusesAnotherActorsPosition(t *testing.T) {
	application, storyID := boundaryTestStory(t)

	view, err := application.CultivationStateFor(storyID, "main", "rival")
	if err == nil {
		t.Fatalf("another character's cultivation position is not the player's to read: %#v", view)
	}
	if !errors.Is(err, interactive.ErrCultivationUnknownActor) {
		t.Fatalf("the refusal must be the rules' own unknown-actor sentinel: %v", err)
	}
}

// TestRecoveryEntryRefusesAnotherActorsPools is the same leak through the rest
// entry, which read the same pools a second way.
func TestRecoveryEntryRefusesAnotherActorsPools(t *testing.T) {
	application, storyID := boundaryTestStory(t)

	view, err := application.SafeRecoveryStateFor(storyID, "main", "rival")
	if err == nil {
		t.Fatalf("another character's pools are not the player's to read: %#v", view)
	}
	if !errors.Is(err, interactive.ErrRecoveryUnknownActor) {
		t.Fatalf("the refusal must be the rules' own unknown-actor sentinel: %v", err)
	}
}

// TestCultivationWriteCannotAdvanceAnotherActor is the control half of the same
// rule: naming another character must not train them.
//
// §四 forbids the player from steering an NPC's real state, and the write path
// accepted any actor id. The character's stored position must be exactly where
// it was after the refusal.
func TestCultivationWriteCannotAdvanceAnotherActor(t *testing.T) {
	application, storyID := boundaryTestStory(t)

	before, err := application.interactive.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	beforeRealm := boundaryActorField(t, before, "rival", combatTestRealmField)
	beforeClock, _, err := application.interactive.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}

	_, err = application.ApplyCultivationInterval(CultivationAppRequest{
		StoryID: storyID, BranchID: "main", ActorID: "rival",
		Value: 30, Unit: "day", CommandID: "cmd-boundary-cultivation",
	})
	if err == nil {
		t.Fatal("the player must not be able to train another character")
	}
	if !errors.Is(err, interactive.ErrCultivationUnknownActor) {
		t.Fatalf("the refusal must be the rules' own unknown-actor sentinel: %v", err)
	}

	after, err := application.interactive.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got := boundaryActorField(t, after, "rival", combatTestRealmField); got != beforeRealm {
		t.Fatalf("a refused interval must leave the other character's realm untouched: %v -> %v", beforeRealm, got)
	}
	afterClock, _, err := application.interactive.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if beforeClock.Seconds != afterClock.Seconds {
		t.Fatalf("a refused interval must spend no formal time: %d -> %d", beforeClock.Seconds, afterClock.Seconds)
	}
}

// TestRecoveryWriteCannotRestoreAnotherActor is the same control check for rest.
func TestRecoveryWriteCannotRestoreAnotherActor(t *testing.T) {
	application, storyID := boundaryTestStory(t)

	before, _, err := application.interactive.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.ApplySafeRecovery(SafeRecoveryAppRequest{
		StoryID: storyID, BranchID: "main", ActorID: "rival",
		Mode: "health_rest", DurationSeconds: 8 * 60 * 60, CommandID: "cmd-boundary-recovery",
	})
	if err == nil {
		t.Fatal("the player must not be able to rest another character")
	}
	if !errors.Is(err, interactive.ErrRecoveryUnknownActor) {
		t.Fatalf("the refusal must be the rules' own unknown-actor sentinel: %v", err)
	}
	after, _, err := application.interactive.CultivationState(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.Seconds != after.Seconds {
		t.Fatalf("a refused rest must spend no formal time: %d -> %d", before.Seconds, after.Seconds)
	}
}

// boundaryActorField reads one actor's field out of a snapshot's state map.
func boundaryActorField(t *testing.T, snapshot interactive.Snapshot, actorID, fieldID string) any {
	t.Helper()
	actors, ok := snapshot.State["actors"].(map[string]any)
	if !ok {
		t.Fatalf("the snapshot must carry the actor root: %#v", snapshot.State)
	}
	record, ok := actors[actorID].(map[string]any)
	if !ok {
		t.Fatalf("the snapshot must carry actor %q", actorID)
	}
	state, _ := record["state"].(map[string]any)
	return state[fieldID]
}

// boundaryTestTurn commits one turn whose TurnResult names a private write on
// both sides, which is the shape the audit found surviving into the player's
// payloads.
//
// The turn is committed through the ordinary path, so what the test reads back is
// the compiled form the store really keeps rather than a hand-built struct.
func boundaryTestTurn(t *testing.T, application *App, storyID string) string {
	t.Helper()
	turn, _, err := application.interactive.AppendTurnWithState(storyID, interactive.AppendTurnWithStateRequest{
		BranchID: "main",
		User:     "交手",
		Narrative: "对手施展了功法，主角看清了来路。",
		TurnResult: &interactive.TurnResult{
			Choices: []string{"继续行动", "观察环境", "询问同伴", "检查状态", "暂时等待"},
			StateUpdates: []interactivestate.Update{
				{Op: interactivestate.Replace, Path: "/rival/" + combatTestCarriedField, Value: map[string]any{
					"令牌": map[string]any{"说明": "洞口的旧令牌", "数量": float64(1)},
				}},
				{Op: interactivestate.Replace, Path: "/rival/当前处境", Value: "守住洞口"},
				{Op: interactivestate.Replace, Path: "/protagonist/" + combatTestCarriedField, Value: map[string]any{
					"符纸": map[string]any{"说明": "路上捡到的符纸", "数量": float64(2)},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return turn.ID
}

// TestPlayerProjectionWithholdsPrivateStateUpdates is the audit's fourth leak.
//
// TurnResult.StateUpdates is persisted on every committed turn and is the one
// nested object the existing projection never walked, so the stream and the
// snapshot carried the opponent's private paths and values verbatim.
func TestPlayerProjectionWithholdsPrivateStateUpdates(t *testing.T) {
	application, storyID := boundaryTestStory(t)
	turnID := boundaryTestTurn(t, application, storyID)

	snapshot, err := application.interactive.Snapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil {
		t.Fatal("the test story must freeze a schema")
	}
	turn := boundarySnapshotTurn(t, snapshot, turnID)
	projected := playerKnownTurn(turn, snapshot.ActorStateSchema)

	if projected.TurnResult == nil {
		t.Fatal("the projection must keep the turn's own result")
	}
	for _, update := range projected.TurnResult.StateUpdates {
		if update.Path == "/rival/"+combatTestCarriedField {
			t.Fatalf("§四: the stream must not carry the opponent's private write: %#v", update)
		}
	}
	var keptOpen, keptOwn bool
	for _, update := range projected.TurnResult.StateUpdates {
		switch update.Path {
		case "/rival/当前处境":
			keptOpen = true
		case "/protagonist/" + combatTestCarriedField:
			keptOwn = true
		}
	}
	if !keptOpen {
		t.Fatalf("the opponent's open write is what the player saw and must survive: %#v", projected.TurnResult.StateUpdates)
	}
	if !keptOwn {
		t.Fatalf("the player's own write must survive: %#v", projected.TurnResult.StateUpdates)
	}
}

// TestInteractiveSnapshotProjectsTurnResultStateUpdates is the same rule at the
// snapshot entry, which is what the browser actually receives on open.
func TestInteractiveSnapshotProjectsTurnResultStateUpdates(t *testing.T) {
	application, storyID := boundaryTestStory(t)
	turnID := boundaryTestTurn(t, application, storyID)

	service := &InteractiveAppService{app: application}
	snapshot, err := service.InteractiveSnapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	turn := boundarySnapshotTurn(t, snapshot, turnID)
	if turn.TurnResult == nil {
		t.Fatal("the snapshot must carry the turn's result")
	}
	for _, update := range turn.TurnResult.StateUpdates {
		if update.Path == "/rival/"+combatTestCarriedField {
			t.Fatalf("§四: the snapshot must not carry the opponent's private write: %#v", update)
		}
	}
}

// TestInteractiveHistoryPageProjectsTurnResultStateUpdates is the audit's third
// leak: the history page was returned straight from the store, so an older page
// of turns carried the same private writes the snapshot was projecting away.
func TestInteractiveHistoryPageProjectsTurnResultStateUpdates(t *testing.T) {
	application, storyID := boundaryTestStory(t)
	turnID := boundaryTestTurn(t, application, storyID)

	service := &InteractiveAppService{app: application}
	page, err := service.InteractiveHistoryPage(storyID, "main", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, turn := range page.Turns {
		if turn.ID != turnID {
			continue
		}
		found = true
		if turn.TurnResult == nil {
			t.Fatal("the history page must carry the turn's result")
		}
		for _, update := range turn.TurnResult.StateUpdates {
			if update.Path == "/rival/"+combatTestCarriedField {
				t.Fatalf("§四: the history page must not carry the opponent's private write: %#v", update)
			}
		}
	}
	if !found {
		t.Fatalf("the history page must include the committed turn: %#v", page.Turns)
	}
}

// boundarySnapshotTurn finds one turn by id in a snapshot's page.
func boundarySnapshotTurn(t *testing.T, snapshot interactive.Snapshot, turnID string) interactive.TurnEvent {
	t.Helper()
	for _, turn := range snapshot.Turns {
		if turn.ID == turnID {
			return turn
		}
	}
	if snapshot.CurrentTurn != nil && snapshot.CurrentTurn.ID == turnID {
		return *snapshot.CurrentTurn
	}
	t.Fatalf("the snapshot must include turn %q: %#v", turnID, snapshot.Turns)
	return interactive.TurnEvent{}
}
