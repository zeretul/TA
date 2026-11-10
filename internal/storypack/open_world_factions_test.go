package storypack

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
)

// Exercise the real author pack -> WorldInputs -> persisted lore -> cold scoped
// read boundary. Validating the source string alone misses NormalizeType's
// fallback to other and cannot prove a faction reaches the library shelf.
func TestSourceOpenWorldFactionsSurviveWorldBookMaterialization(t *testing.T) {
	root := t.TempDir()
	seeder := booklore.NewStore(root)
	for _, spec := range []struct {
		pack, prefix string
		count        int
	}{{"lanchuan-qunxia", "lanchuan", 21}, {"xinghai-689", "xinghai", 12}} {
		t.Run(spec.pack, func(t *testing.T) {
			pack, e := NewLibrary(t.TempDir()).Get(spec.pack)
			if e != nil {
				t.Fatal(e)
			}
			inputs := map[string]booklore.ItemInput{}
			for _, input := range pack.WorldInputs() {
				inputs[input.ID] = input
			}
			for i := 1; i <= spec.count; i++ {
				id := interactive.PackEntryIDForRevision(fmt.Sprintf("%s-o%02d", spec.prefix, i), pack.ContentHash())
				input, ok := inputs[id]
				if !ok {
					t.Fatalf("source faction missing %s", id)
				}
				if _, e = seeder.Create(input); e != nil {
					t.Fatal(e)
				}
				// A new Store reads the persisted item rather than the in-memory input.
				cold := booklore.NewScopedStore(root, pack.WorldScope())
				item, e := cold.Get(id)
				if e != nil {
					t.Fatal(e)
				}
				if item.Type != "faction" {
					t.Errorf("%s (%s) reached the world book as %q, want faction", input.Name, id, item.Type)
				}
				if item.Name != input.Name || item.Content != input.Content || item.Scope != pack.WorldScope() {
					t.Errorf("faction materialization lost authored text or pack isolation: %s", id)
				}
			}
		})
	}
}

func TestOpenWorldEnglishLifeRulesMaterializeWithoutChangingCanonicalSource(t *testing.T) {
	for _, spec := range []struct{ pack, prefix string }{{"lanchuan-qunxia", "lanchuan"}, {"xinghai-689", "xinghai"}} {
		t.Run(spec.pack, func(t *testing.T) {
			pack, e := NewLibrary(t.TempDir()).Get(spec.pack)
			if e != nil {
				t.Fatal(e)
			}
			before, e := json.Marshal(pack)
			if e != nil {
				t.Fatal(e)
			}
			english := pack.Localize("en-US")
			source := map[string]WorldEntry{}
			shown := map[string]WorldEntry{}
			for _, x := range pack.World {
				source[x.ID] = x
			}
			for _, x := range english.World {
				shown[x.ID] = x
			}
			root := t.TempDir()
			book := booklore.NewStore(root)
			for _, input := range english.WorldInputs() {
				if strings.HasPrefix(input.ID, spec.prefix+"-life-") {
					if _, e = book.Create(input); e != nil {
						t.Fatal(e)
					}
				}
			}
			for _, suffix := range []string{"identity", "work", "news"} {
				id := spec.prefix + "-life-" + suffix
				s, ok := source[id]
				x := shown[id]
				if !ok || x.ID != s.ID || x.Type != s.Type || x.LoadMode != s.LoadMode || x.Content == s.Content || x.Name == s.Name || strings.TrimSpace(x.Content) == "" {
					t.Fatalf("localized life rule lost source identity or independent English: %s", id)
				}
				actual, e := booklore.NewScopedStore(root, english.WorldScope()).Get(interactive.PackEntryIDForRevision(id, english.ContentHash()))
				if e != nil || actual.Type != "rule" || actual.Content != x.Content {
					t.Fatalf("English life rule did not survive the real book boundary: %s %v", id, e)
				}
			}
			after, e := json.Marshal(pack)
			if e != nil || string(before) != string(after) {
				t.Fatal("locale view mutated canonical author facts", e)
			}
		})
	}
}
