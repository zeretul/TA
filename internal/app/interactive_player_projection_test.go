package app

import (
	"testing"

	"ta/internal/interactive"
)

// These tests hold the player-facing snapshot to the NPC information boundary.
//
// §四 of 战斗建议复核与NPC信息边界 forbids sending an opponent's full state to the
// browser and hiding it in the interface, so the check is made on what the
// snapshot carries rather than on what the panel chooses to draw. The
// protagonist is the one actor whose numbers are the player's own, and the two
// bookkeeping actors carry no private dossier, so both stay intact.

func playerProjectionActors(t *testing.T, snapshot interactive.Snapshot) map[string]any {
	t.Helper()
	raw, ok := snapshot.State["actors"]
	if !ok {
		t.Fatalf("the snapshot must carry the actor root: %#v", snapshot.State)
	}
	actors, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("the actor root must be a map: %#v", raw)
	}
	return actors
}

func playerProjectionActorState(t *testing.T, snapshot interactive.Snapshot, actorID string) map[string]any {
	t.Helper()
	record, ok := playerProjectionActors(t, snapshot)[actorID].(map[string]any)
	if !ok {
		t.Fatalf("the snapshot must carry actor %q", actorID)
	}
	state, _ := record["state"].(map[string]any)
	if state == nil {
		t.Fatalf("actor %q must carry its state: %#v", actorID, record)
	}
	return state
}

func TestInteractiveSnapshotWithholdsAnOpponentsPrivateDossier(t *testing.T) {
	app, storyID := combatTestApp(t, "condensed_blade")
	service := &InteractiveAppService{app: app}

	snapshot, err := service.InteractiveSnapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}

	rival := playerProjectionActorState(t, snapshot, "rival")
	for _, field := range []string{
		combatTestSkillsField,
		combatTestCarriedField,
		combatTestRealmField,
		combatTestHealthField,
		combatTestManaField,
		// T22 names the burden, the residual floor, the progress toward the next
		// stage and the implement in hand as facts a reader must find out rather
		// than open a panel for. They are the same kind of number as the realm
		// above: each one tells the player exactly how much fight is left.
		combatTestBurdenField,
		combatTestBurdenFloorField,
		combatTestRealmProgressField,
		combatTestImplementField,
		// The same list continues past the numbers: what someone is worth, how
		// they spend their time, and where they can be found alone. T22 names
		// 钱袋, 修行安排 and the outing's safe window as withheld, and the reason
		// is the same one the burden has — each one is a fact about that
		// character's present that the player has to be told.
		combatTestCurrencyField,
		combatTestArrangementField,
		combatTestTrainingWindowField,
	} {
		if value, present := rival[field]; present {
			t.Fatalf("§3.2: the player's snapshot must not carry the opponent's %q, got %#v", field, value)
		}
	}

	// §3.2 keeps the player's own numbers exact: withholding is about the
	// opponent, not about the panel.
	protagonist := playerProjectionActorState(t, snapshot, interactive.DefaultActorID)
	if got, _ := protagonist[combatTestRealmField].(string); got == "" {
		t.Fatalf("the player's own realm must survive the projection: %#v", protagonist)
	}
	if _, present := protagonist[combatTestSkillsField]; !present {
		t.Fatalf("the player's own learned set must survive the projection: %#v", protagonist)
	}
	// The player's own burden is the number the panel summarises as 施招负担, and
	// the interface cannot draw it without it.
	if _, present := protagonist[combatTestBurdenField]; !present {
		t.Fatalf("the player's own burden must survive the projection: %#v", protagonist)
	}
	// The purse is the protagonist's own money and the 行囊 section is where they
	// read it, so withholding is about the opponent here too.
	if _, present := protagonist[combatTestCurrencyField]; !present {
		t.Fatalf("the player's own purse must survive the projection: %#v", protagonist)
	}
}

func TestInteractiveSnapshotKeepsBookkeepingActorsWhole(t *testing.T) {
	app, storyID := combatTestApp(t)
	service := &InteractiveAppService{app: app}

	snapshot, err := service.InteractiveSnapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	// The scene and the world are not dossiers: they are what the player is
	// looking at, so projecting them would hide the story from its own reader.
	actors := playerProjectionActors(t, snapshot)
	for _, actorID := range []string{interactive.DefaultStoryContextActorID, interactive.DefaultWorldEntitiesActorID} {
		record, ok := actors[actorID].(map[string]any)
		if !ok {
			t.Fatalf("the snapshot must still carry %q", actorID)
		}
		state, _ := record["state"].(map[string]any)
		if len(state) == 0 {
			t.Fatalf("actor %q must keep its own state: %#v", actorID, record)
		}
	}
}

// The boundary is enforced where the data leaves the process, so the turn's
// own change record takes it too: an opponent's private write must not be
// named in the summary the player reads.
func TestPlayerProjectionWithholdsPrivateTurnOps(t *testing.T) {
	app, storyID := combatTestApp(t, "condensed_blade")
	service := &InteractiveAppService{app: app}

	snapshot, err := service.InteractiveSnapshot(storyID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActorStateSchema == nil {
		t.Fatal("the test story must freeze a schema")
	}
	schema := snapshot.ActorStateSchema
	// combatTestSystem copies the protagonist template as rival. Its public
	// situation field is 当前处境; 当前目标与处境 belongs to another template.
	declaredPublic := false
	for _, template := range schema.System.Templates {
		if template.ID != "rival" {
			continue
		}
		for _, field := range template.Fields {
			if field.Name == "当前处境" && field.Visibility == interactive.ActorStateVisibilityOpen {
				declaredPublic = true
			}
		}
	}
	if !declaredPublic {
		t.Fatal("fixture must use rival's frozen declared public field")
	}

	// The same turn record the stream carries, with one write to each side.
	turn := interactive.TurnEvent{
		ID: "turn-private-ops", BranchID: "main",
		StateDelta: &interactive.StateDelta{
			ActorOps: []interactive.ActorStateOp{
				{ActorID: "rival", FieldID: combatTestSkillsField, Op: "replace", Value: learnedSkillRecords([]string{"condensed_blade"})},
				{ActorID: "rival", FieldID: "当前处境", Op: "replace", Value: "守在门口"},
				{ActorID: interactive.DefaultActorID, FieldID: combatTestSkillsField, Op: "replace", Value: learnedSkillRecords([]string{"strike"})},
			},
		},
		RuleResolution: &interactive.RuleResolution{
			Result: interactive.RuleResult{StateChanges: []interactive.TurnStateChange{
				{ActorID: "rival", FieldID: combatTestRealmField, Change: -1},
				{ActorID: "rival", FieldID: "当前处境", Change: -1},
			}},
		},
	}
	projected := playerKnownTurn(turn, schema)

	for _, op := range projected.StateDelta.ActorOps {
		if op.ActorID == "rival" && op.FieldID == combatTestSkillsField {
			t.Fatalf("§四: the stream must not name the opponent's private write: %#v", op)
		}
	}
	if len(projected.StateDelta.ActorOps) != 2 {
		t.Fatalf("the opponent's open write and the player's own write must survive: %#v", projected.StateDelta.ActorOps)
	}
	for _, change := range projected.RuleResolution.Result.StateChanges {
		if change.ActorID == "rival" && change.FieldID == combatTestRealmField {
			t.Fatalf("§四: the settled opponent realm must not reach the stream: %#v", change)
		}
	}
	if len(projected.RuleResolution.Result.StateChanges) != 1 {
		t.Fatalf("the opponent's open change must survive: %#v", projected.RuleResolution.Result.StateChanges)
	}
}
