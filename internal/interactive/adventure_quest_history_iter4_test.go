package interactive

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"ta/internal/agents/conversationjournal"
)

func TestAdventureQuestHistoricalGunneryReceiptColdOpensWithoutRetroactiveCostsIter4(t *testing.T) {
	// Construct an isolated historical journal fixture with the old a18.c2 shape:
	// version 1.0, no ship, +50000 Enas, 1800 seconds, and no method/fuel fields.
	// c3 has the identical objective/reward/time layout, so changing only its saved
	// choice identity produces that old record without a production bypass hook.
	s, id, _, dir := authoredQuestStory(t, "xinghai-689", journeyPlaceLabel("xinghai-l21"), 0, map[string]int{"xinghai-s30": 1, "xinghai-s04": 1})
	questApplyOn(t, s, id, "main", "xinghai-a18", QuestActionAccept, "", "accept-old")
	questApplyOn(t, s, id, "main", "xinghai-a18", QuestActionChoose, "xinghai-a18.c3", "old-result")
	before, err := s.AdventureWindow(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	if before.Actor.Currency != 50000 || len(before.Actor.Assets) != 0 || before.ClockSeconds != 1800 {
		t.Fatal("historical fixture does not match old receipt")
	}
	path := s.storyPath(id)
	meta, records, err := s.readStoryJournalLocked(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// The repository's legacy-reader fixtures use flat event rows. Preserve
	// all actual state/clock records and change only the saved choice identity.
	rows := []any{meta}
	changed := false
	for _, record := range records {
		data, err := json.Marshal(record.Raw)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("xinghai-a18.c3")) {
			changed = true
		}
		data = bytes.ReplaceAll(data, []byte("xinghai-a18.c3"), []byte("xinghai-a18.c2"))
		var row map[string]any
		if err = json.Unmarshal(data, &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	if !changed {
		t.Fatal("missing persisted choice")
	}
	if err = writeJSONL(path, rows); err != nil {
		t.Fatal(err)
	}
	receiptData, err := json.Marshal(rows[1:])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(receiptData, []byte("asset_fuel_cost")) || bytes.Contains(receiptData, []byte("use_skill_id")) {
		t.Fatal("historical receipt unexpectedly embeds new method fields")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(conversationjournal.SidecarPath(path)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	cold := NewStore(dir)
	defer cold.Close()
	after, err := cold.AdventureWindow(id, "main")
	if err != nil {
		t.Fatalf("historical receipt cannot cold-open: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cold open reinterpreted history or charged new fuel/effort retroactively")
	}
	q, err := cold.QuestStateFor(id, "main")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range q.Tasks {
		if task.TaskID == "xinghai-a18" {
			found = task.Status == "completed" && task.Resolutions["xinghai-a18.resolve"].ChoiceID == "xinghai-a18.c2"
		}
	}
	if !found {
		t.Fatal("cold journal did not retain the historical completed c2 result")
	}
	if _, err = cold.ApplyQuest(id, QuestRequest{BranchID: "main", TaskID: "xinghai-a18", Action: QuestActionChoose, ChoiceID: "xinghai-a18.c2", CommandID: "new-attempt", CommandHash: "new-attempt"}); err == nil {
		t.Fatal("historical completion paid twice")
	}
	unchanged, err := cold.AdventureWindow(id, "main")
	if err != nil || !reflect.DeepEqual(after, unchanged) {
		t.Fatal("new attempt changed historical settled resources")
	}
	reread, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, reread) {
		t.Fatal("reading or refusing re-execution rewrote the canonical old journal")
	}
}
