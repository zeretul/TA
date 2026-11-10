package storypack

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
)

// Exercise the persisted, localized world-book boundary, not just JSON strings.
// A regional addition is useful only if the new world's actual scope can read it
// while the independent display translation leaves canonical authored facts alone.
func TestRegionalWorldContentMaterializesInBothLanguagesWithoutMutatingFrozenSource(t *testing.T) {
	for _, spec := range []struct {
		pack, prefix string
		regionalIDs  []string
	}{
		{"fanren-jie", "fanren", []string{"fanren-region-market-stream", "fanren-region-yuanwu-artisans", "fanren-region-beast-gardens"}},
		{"lanchuan-qunxia", "lanchuan", nil},
		{"xinghai-689", "xinghai", nil},
	} {
		t.Run(spec.pack, func(t *testing.T) {
			p, err := NewLibrary(t.TempDir()).Get(spec.pack)
			if err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			ids := append([]string(nil), spec.regionalIDs...)
			if len(ids) == 0 {
				for n := 1; n <= 8; n++ {
					ids = append(ids, fmt.Sprintf("%s-r%02d", spec.prefix, n))
				}
			}
			ids = append(ids, spec.prefix+"-life-unfinished-paths")
			for _, locale := range []string{"zh-CN", "en-US"} {
				shown := p.Localize(locale)
				root := t.TempDir()
				book := booklore.NewStore(root)
				inputs := map[string]booklore.ItemInput{}
				for _, input := range shown.WorldInputs() {
					inputs[input.ID] = input
				}
				for _, id := range ids {
					stable := interactive.PackEntryIDForRevision(id, shown.ContentHash())
					input, ok := inputs[stable]
					if !ok {
						t.Fatalf("regional resource missing from real world inputs: %s", id)
					}
					if _, err := book.Create(input); err != nil {
						t.Fatal(err)
					}
					got, err := booklore.NewScopedStore(root, shown.WorldScope()).Get(stable)
					if err != nil || got.Content != input.Content || got.Name != input.Name || got.Scope != shown.WorldScope() {
						t.Fatalf("regional content failed cold scope read: %s %v", id, err)
					}
					if locale == "en-US" && strings.IndexFunc(got.Content, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
						t.Fatalf("English regional body leaked Chinese: %s", id)
					}
				}
			}
			after, err := json.Marshal(p)
			if err != nil || string(before) != string(after) {
				t.Fatal("display materialization changed canonical source", err)
			}
		})
	}
}
