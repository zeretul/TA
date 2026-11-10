package exploration

import (
	"encoding/json"
	"testing"
)

func TestSpiritRegionsOwnInitialVersionAndStableRoundTrip(t *testing.T) {
	for _, id := range []string{RealmSpiritBoundary, RealmSpiritRift, RealmSpiritArray, RealmSpiritArchive} {
		d, ok := RealmDefinition(id)
		if !ok || d.Version != "1.0" || d.ExploreRules == nil || len(d.EntryLocationIDs) != 1 || len(d.ExploreRules.Approaches) < 2 {
			t.Fatalf("new region incomplete: %+v", d)
		}
		before, err := DungeonDefinitionContentHash(d)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		var again DungeonDefinition
		if err = json.Unmarshal(raw, &again); err != nil {
			t.Fatal(err)
		}
		after, err := DungeonDefinitionContentHash(again)
		if err != nil || before != after {
			t.Fatal("new region hash not replay-stable")
		}
		if ds := ValidateDungeonDefinition(again); len(ds) > 0 {
			t.Fatal(ds)
		}
	}
}
