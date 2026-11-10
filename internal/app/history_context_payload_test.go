package app

import (
	"bytes"
	"context"
	"encoding/json"
	"ta/internal/history"
	"testing"
)

func TestHistoryContextKeepsCompleteReferenceWorldAndExactNumbers(t *testing.T) {
	s, err := history.NewStore(t.TempDir()).Create(context.Background(), history.CreateRequest{Name: "Payload", CommandID: "create", ScenarioID: "ming-1627"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := historyContextPayload(history.SimulationFacts(s), "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) >= historyModelInputLimit-60000 {
		t.Fatalf("No room for orders and memories: %d bytes", len(b))
	}
	for _, entity := range s.World.Entities {
		if !bytes.Contains(b, []byte(`"id":"`+entity.ID+`"`)) {
			t.Fatalf("Lost entity %s", entity.ID)
		}
	}
	var decoded map[string]json.RawMessage
	if err = json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	numbers, err := historyContextPayload(map[string]any{"balance": int64(9007199254740993), "name": history.Text{ZH: "国库", EN: "Treasury"}}, "en-US")
	if err != nil || !bytes.Contains(numbers, []byte(`9007199254740993`)) || !bytes.Contains(numbers, []byte(`"name":{"en":"Treasury"}`)) {
		t.Fatalf("Context conversion changed facts: %s %v", numbers, err)
	}
}
