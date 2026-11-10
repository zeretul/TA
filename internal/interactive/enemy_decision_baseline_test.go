package interactive

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// An opt-in bridge verifies that adding an optional model-selected intent did
// not silently rewrite the period snapshots or deterministic fallback menu.
func TestEnemyDecisionFallbackMatchesNPCCheckpoint(t *testing.T) {
	path := os.Getenv("TA_NPC_BASELINE")
	if path == "" {
		t.Skip("set TA_NPC_BASELINE to the complete frozen numeric checkpoint")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint struct {
		NPCSnapshots []map[string]any `json:"npc_snapshots"`
	}
	if err = json.Unmarshal(raw, &checkpoint); err != nil {
		t.Fatal(err)
	}
	var actual []map[string]any
	for _, pack := range []string{"lanchuan-qunxia", "xinghai-689"} {
		actual = append(actual, adventureNPCBalanceSnapshots(t, authoredAdventureRules(t, pack))...)
	}
	canonical, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err = json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.NPCSnapshots) != 120 || !reflect.DeepEqual(checkpoint.NPCSnapshots, decoded) {
		t.Fatal("period state or two observed-pressure fallback choices changed from the numeric checkpoint")
	}
}
