package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"ta/internal/interactive"
	interactivestate "ta/internal/interactive/state"
	"testing"
)

func actorCarrierSchema() *interactive.ActorStateSchemaSnapshot {
	return &interactive.ActorStateSchemaSnapshot{System: interactive.StoryDirectorActorStateSystem{
		Templates: []interactive.ActorStateTemplate{{ID: interactive.ActorStateImportantCharacterTemplateID, Fields: []interactive.ActorStateField{
			{Name: "公开", Type: "object"}, {Name: "钱财", Type: "number", Visibility: interactive.ActorStateVisibilityPrivate},
			{Name: "秘密/名称", Type: "string", Visibility: interactive.ActorStateVisibilityPrivate},
		}}},
		InitialActors: []interactive.ActorStateInitialActor{{ID: "行商", TemplateID: interactive.ActorStateImportantCharacterTemplateID}},
	}}
}
func actorCarrierRecord() map[string]any {
	return map[string]any{"id": "行商", "name": "PUBLIC_TRADER", "template_id": interactive.ActorStateImportantCharacterTemplateID,
		"state":               map[string]any{"公开": map[string]any{"称谓": "PUBLIC_TITLE"}, "钱财": "PRIVATE_WALLET_MARKER", "秘密/名称": "PRIVATE_ESCAPED_MARKER", "未声明": "UNKNOWN_FIELD_MARKER"},
		"undeclared_metadata": "UNKNOWN_METADATA_MARKER",
	}
}
func assertNoPrivateCarrier(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"PRIVATE_WALLET_MARKER", "PRIVATE_ESCAPED_MARKER", "UNKNOWN_FIELD_MARKER", "UNKNOWN_METADATA_MARKER"} {
		if strings.Contains(string(raw), marker) {
			t.Fatalf("player carrier contains %s: %s", marker, raw)
		}
	}
}

func TestPlayerActorCarriersWholeRecordsAndUnknownFields(t *testing.T) {
	record := actorCarrierRecord()
	whole := map[string]any{"actors": map[string]any{"行商": record}}
	turn := interactive.TurnEvent{ID: "carrier", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{
		{Op: "create", Path: "/行商", Value: record},
		{Op: "archive", Path: "/行商", Value: record},
		{Op: "restore", Path: "/行商", Value: record},
	}}, StateDelta: &interactive.StateDelta{Ops: []interactivestate.Op{
		{Op: "set", Path: "actors.行商", Value: record},
		{Op: "merge", Path: "/actors/行商", Value: record},
		{Op: "set", Path: "actors.行商.state", Value: record["state"]},
		{Op: "set", Path: "/actors/行商/state", Value: record["state"]},
		{Op: "set", Path: "actors", Value: whole["actors"]},
		{Op: "set", Path: "/", Value: whole},
	}}}
	before, _ := json.Marshal(turn)
	got := playerKnownTurn(turn, actorCarrierSchema())
	assertNoPrivateCarrier(t, got)
	data, _ := json.Marshal(got)
	if !strings.Contains(string(data), "PUBLIC_TRADER") || !strings.Contains(string(data), "PUBLIC_TITLE") {
		t.Fatalf("public actor card disappeared: %s", data)
	}
	after, _ := json.Marshal(turn)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("projection rewrote canonical turn")
	}
	snapshot := interactive.Snapshot{State: whole, ActorStateSchema: actorCarrierSchema(), Turns: []interactive.TurnEvent{turn}, CurrentTurn: &turn}
	projectInteractiveSnapshotForPlayer(&snapshot)
	assertNoPrivateCarrier(t, snapshot)
	after, _ = json.Marshal(turn)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("snapshot projection mutated shared turn")
	}
	raw, _ := json.Marshal(record)
	if !strings.Contains(string(raw), "PRIVATE_WALLET_MARKER") {
		t.Fatal("canonical NPC truth was deleted")
	}
}

func TestPlayerActorCarriersFieldDeletionAndEscapedPaths(t *testing.T) {
	turn := interactive.TurnEvent{TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{
		{Op: "replace", Path: "/行商/钱财", Value: "PRIVATE_WALLET_MARKER"},
		{Op: "replace", Path: "/行商/秘密~1名称", Value: "PRIVATE_ESCAPED_MARKER"},
		{Op: "replace", Path: "/行商/未声明", Value: "UNKNOWN_FIELD_MARKER"},
		{Op: "replace", Path: "/行商/公开/称谓", Value: "PUBLIC_TITLE"},
	}}, StateDelta: &interactive.StateDelta{Ops: []interactivestate.Op{
		{Op: "set", Path: "actors.行商.state.钱财", Value: "PRIVATE_WALLET_MARKER"},
		{Op: "unset", Path: "actors.行商.state.钱财"},
		{Op: "delete", Path: "/actors/行商/state/秘密~1名称"},
		{Op: "set", Path: "/actors/行商/state/未声明", Value: "UNKNOWN_FIELD_MARKER"},
		{Op: "set", Path: "actors.行商.name", Value: "PUBLIC_TRADER"},
		{Op: "set", Path: "actors.行商.state.公开.称谓", Value: "PUBLIC_TITLE"},
		{Op: "unset", Path: "actors.行商"},
	}, ActorOps: []interactive.ActorStateOp{
		{Op: "unset", ActorID: "行商", FieldID: "钱财", PreviousValue: "PRIVATE_WALLET_MARKER"},
		{Op: "set", ActorID: "行商", FieldID: "未声明", Value: "UNKNOWN_FIELD_MARKER"},
		{Op: "set", ActorID: "行商", FieldID: "公开", Value: "PUBLIC_TITLE"},
	}}}
	got := playerKnownTurn(turn, actorCarrierSchema())
	assertNoPrivateCarrier(t, got)
	if len(got.TurnResult.StateUpdates) != 1 || len(got.StateDelta.Ops) != 3 || len(got.StateDelta.ActorOps) != 1 {
		t.Fatalf("public/deletion operations not preserved, or private paths leaked: %+v", got)
	}
}

func TestPlayerActorCarriersOwnerAndBookkeepingRemainVisible(t *testing.T) {
	schema := actorCarrierSchema()
	own := map[string]any{"template_id": "protagonist", "state": map[string]any{"钱财": "OWN_WALLET"}}
	scene := map[string]any{"template_id": interactive.ActorStateStoryContextTemplateID, "state": map[string]any{"世界时间": "PUBLIC_TIME"}}
	turn := interactive.TurnEvent{TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "create", Path: "/protagonist", Value: own}, {Op: "create", Path: "/story_context", Value: scene}}}, StateDelta: &interactive.StateDelta{Ops: []interactivestate.Op{{Op: "set", Path: "actors.protagonist.state.钱财", Value: "OWN_WALLET"}}}}
	got := playerKnownTurn(turn, schema)
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "OWN_WALLET") || !strings.Contains(string(raw), "PUBLIC_TIME") {
		t.Fatalf("owner/bookkeeping was hidden: %s", raw)
	}
}

func TestPlayerActorCarriersMissingTemplateDoesNotExposeUnknownState(t *testing.T) {
	state := map[string]any{"actors": map[string]any{"陌生人": map[string]any{"name": "PUBLIC_NAME", "state": map[string]any{"未声明": "UNKNOWN_FIELD_MARKER"}}}}
	got := playerKnownState(state, nil)
	assertNoPrivateCarrier(t, got)
	raw, _ := json.Marshal(got)
	if !strings.Contains(string(raw), "PUBLIC_NAME") {
		t.Fatal("unknown template erased public identity")
	}
}

func TestPlayerActorCarriersSchemaSeedsWithoutAdventureRules(t *testing.T) {
	schema := actorCarrierSchema()
	schema.System.InitialActors[0].State = actorCarrierRecord()["state"].(map[string]any)
	schema.System.Templates[0].Fields[1].Default = "PRIVATE_WALLET_MARKER"
	snapshot := interactive.Snapshot{ActorStateSchema: schema}
	projectInteractiveSnapshotForPlayer(&snapshot)
	assertNoPrivateCarrier(t, snapshot)
	raw, _ := json.Marshal(snapshot.ActorStateSchema)
	if !strings.Contains(string(raw), "PUBLIC_TITLE") {
		t.Fatalf("public NPC seed disappeared: %s", raw)
	}
	original, _ := json.Marshal(schema)
	if !strings.Contains(string(original), "PRIVATE_WALLET_MARKER") {
		t.Fatal("projection mutated frozen GM schema")
	}
}

func TestPlayerActorCarriersColdJSONAndBranchCopies(t *testing.T) {
	schema := actorCarrierSchema()
	canonical := interactive.Snapshot{ActorStateSchema: schema, State: map[string]any{"actors": map[string]any{"行商": actorCarrierRecord()}}, Turns: []interactive.TurnEvent{{ID: "fork-source", TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "create", Path: "/行商", Value: actorCarrierRecord()}}}}}}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"main", "fork"} {
		t.Run(branch, func(t *testing.T) {
			var cold interactive.Snapshot
			if err := json.Unmarshal(encoded, &cold); err != nil {
				t.Fatal(err)
			}
			projectInteractiveSnapshotForPlayer(&cold)
			assertNoPrivateCarrier(t, cold)
			raw, _ := json.Marshal(cold)
			if !strings.Contains(string(raw), "PUBLIC_TITLE") || !strings.Contains(string(raw), "行商") {
				t.Fatalf("shared actor identity/public state missing: %s", raw)
			}
		})
	}
	after, _ := json.Marshal(canonical)
	if string(after) != string(encoded) {
		t.Fatal("player copies changed canonical branch source")
	}
}

func TestPlayerActorCarriersRespectExactTemplatePublicField(t *testing.T) {
	schema := actorCarrierSchema()
	schema.System.Templates = append(schema.System.Templates, interactive.ActorStateTemplate{ID: "public_trader", Fields: []interactive.ActorStateField{{Name: "钱财", Type: "number"}}})
	schema.System.InitialActors = append(schema.System.InitialActors, interactive.ActorStateInitialActor{ID: "公示行商", TemplateID: "public_trader"})
	turn := interactive.TurnEvent{TurnResult: &interactive.TurnResult{StateUpdates: []interactivestate.Update{{Op: "replace", Path: "/公示行商/钱财", Value: 17}}}, StateDelta: &interactive.StateDelta{ActorOps: []interactive.ActorStateOp{{ActorID: "公示行商", FieldID: "钱财", Value: 17}}}}
	got := playerKnownTurn(turn, schema)
	if len(got.TurnResult.StateUpdates) != 1 || len(got.StateDelta.ActorOps) != 1 {
		t.Fatal("another template's private name hid this actor's declared public field")
	}
}
