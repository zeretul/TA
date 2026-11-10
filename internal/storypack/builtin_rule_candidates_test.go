package storypack

import "testing"

func TestBuiltinRuleCandidatesListAndLoad(t *testing.T) {
	expected := map[string]int{
		"doupo-continent-ta07-full-gm-candidate":           16,
		"harry-potter-1991-ta07-full-gm-candidate":         8,
		"harry-potter-1991-student-ta07-full-gm-candidate": 6,
		"lord-of-mysteries-ta07-full-gm-candidate":         11,
		"zichuan-xichuan-ta07-full-gm-candidate":           12,
	}
	library := NewLibrary(t.TempDir())
	summaries, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, summary := range summaries {
		if summary.ID == HistoricalSanguoPackID {
			t.Fatal("historical Sanguo is listed as an adventure")
		}
		if _, ok := expected[summary.ID]; !ok {
			continue
		}
		if summary.Invalid || !summary.Builtin {
			t.Fatalf("candidate unavailable: %+v", summary)
		}
		seen[summary.ID] = true
	}
	for id, count := range expected {
		if !seen[id] {
			t.Fatalf("builtin missing from Library.List: %s", id)
		}
		pack, err := library.Get(id)
		if err != nil {
			t.Fatalf("Get(%s): %v", id, err)
		}
		if !pack.Builtin || len(pack.ProtagonistTemplates) != count {
			t.Fatalf("incomplete builtin %s", id)
		}
		if err := pack.Validate(); err != nil {
			t.Fatalf("Validate(%s): %v", id, err)
		}
		if pack.Story.Gameplay.StatusTemplate != WorldStatusGeneralActions {
			t.Fatalf("candidate %s enabled a dedicated engine", id)
		}
		if pack.Story.ModuleRefs.ActorStateID == DefaultActorStateID {
			t.Fatalf("candidate %s did not bind its authored state module", id)
		}
		byID := map[string]bool{}
		byName := map[string]bool{}
		for _, entry := range pack.World {
			if byID[entry.ID] || byName[worldFieldKey(entry.Name)] {
				t.Fatalf("duplicate lore identity in %s: %s", id, entry.ID)
			}
			byID[entry.ID] = true
			byName[worldFieldKey(entry.Name)] = true
		}
		for _, person := range pack.Characters {
			if !byID[person.LoreID] {
				t.Fatalf("missing body for %s/%s", id, person.SourceKey)
			}
		}
	}
}
