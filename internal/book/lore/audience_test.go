package lore

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGMAudienceSurvivesColdReadAndLegacyEdits(t *testing.T) {
	root := t.TempDir()
	var input ItemInput
	if err := json.Unmarshal([]byte(`{"id":"secret","name":"Secret archive","audience":"gm","type":"rule","load_mode":"manual","content":"GM_SECRET_MARKER"}`), &input); err != nil {
		t.Fatal(err)
	}
	item, err := NewStore(root).Create(input)
	if err != nil {
		t.Fatal(err)
	}
	assertGM := func() {
		t.Helper()
		item, err := NewStore(root).Read("secret")
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		var saved map[string]any
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved["audience"] != "gm" {
			t.Fatalf("cold read lost audience: %s", data)
		}
		if !strings.Contains(ReferenceMarkdown(item), "GM_SECRET_MARKER") {
			t.Fatal("referee lost full GM content")
		}
	}
	assertGM()
	_, err = NewStore(root).Update(item.ID, ItemInput{Name: item.Name, Type: item.Type, Content: "GM_SECRET_MARKER edited"})
	if err != nil {
		t.Fatal(err)
	}
	assertGM()
	_, err = NewStore(root).ApplyOperations("", []Operation{{Op: "update", ID: item.ID, Item: ItemInput{Content: "GM_SECRET_MARKER batch"}}})
	if err != nil {
		t.Fatal(err)
	}
	assertGM()
}
