package storypack

import (
	"encoding/json"
	"ta/internal/book/lore"
	"testing"
)

func TestWorldAudienceFlowsThroughHashAndFrozenLore(t *testing.T) {
	var p Pack
	if err := json.Unmarshal([]byte(`{"id":"audience-world","world":[{"id":"archive","name":"Archive","type":"rule","audience":"gm","content":"SECRET"}]}`), &p); err != nil {
		t.Fatal(err)
	}
	inputs := p.WorldInputs()
	if len(inputs) != 1 {
		t.Fatalf("inputs: %v", inputs)
	}
	root := t.TempDir()
	if _, err := lore.NewStore(root).Create(inputs[0]); err != nil {
		t.Fatal(err)
	}
	item, err := lore.NewStore(root).Read(inputs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(item)
	var saved map[string]any
	_ = json.Unmarshal(data, &saved)
	if saved["audience"] != "gm" {
		t.Fatalf("frozen lore lost GM: %s", data)
	}
	before := p.ContentHash()
	if err := json.Unmarshal([]byte(`{"id":"archive","name":"Archive","type":"rule","audience":"public","content":"SECRET"}`), &p.World[0]); err != nil {
		t.Fatal(err)
	}
	if p.ContentHash() == before {
		t.Fatal("audience change did not change frozen pack hash")
	}
}
