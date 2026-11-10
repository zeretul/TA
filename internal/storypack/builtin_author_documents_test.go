package storypack

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	booklore "ta/internal/book/lore"
	"ta/internal/interactive"
)

//go:embed assets/rule-author-documents.json
var ruleAuthorDocumentExpectations []byte

// Tests all authored Markdown and JSON files after the real transaction and a new Store
// read. The expected hashes refer to source UTF-8 bytes, not rendered JSON.
func TestBuiltinAuthorSourcesSurviveColdStoreRead(t *testing.T) {
	var expected struct {
		SourceFileCount int    `json:"source_file_count"`
		Tail            string `json:"tail"`
		Variants        map[string]struct {
			Sources []struct {
				Path         string `json:"source_path"`
				Hash         string `json:"sha256"`
				Bytes        int    `json:"bytes"`
				NavigationID string `json:"navigation_id"`
				Pages        []struct {
					ID          string `json:"entry_id"`
					PrefixBytes int    `json:"prefix_bytes"`
					SuffixBytes int    `json:"suffix_bytes"`
					Hash        string `json:"payload_sha256"`
				} `json:"pages"`
			} `json:"sources"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(ruleAuthorDocumentExpectations, &expected); err != nil {
		t.Fatal(err)
	}
	uniqueSources := map[string]bool{}
	for _, variant := range expected.Variants {
		for _, source := range variant.Sources {
			uniqueSources[source.Path] = true
		}
	}
	if len(uniqueSources) != expected.SourceFileCount || expected.SourceFileCount != 137 {
		t.Fatalf("source coverage: %d, expected %d", len(uniqueSources), expected.SourceFileCount)
	}
	seen := map[string]bool{}
	for _, pack := range builtinPacks() {
		spec, ok := expected.Variants[pack.ID]
		if !ok {
			continue
		}
		seen[pack.ID] = true
		t.Run(pack.ID, func(t *testing.T) {
			root := t.TempDir()
			seedPack(t, booklore.NewStore(root), pack)
			items, err := booklore.NewStore(root).ListAll()
			if err != nil {
				t.Fatal(err)
			}
			byID := map[string]booklore.Item{}
			for _, item := range items {
				byID[item.ID] = item
			}
			if len(items) != len(pack.World) {
				t.Fatalf("seed lost entries: got %d want %d", len(items), len(pack.World))
			}
			revision := pack.ContentHash()
			for _, source := range spec.Sources {
				if _, ok := byID[interactive.PackEntryIDForRevision(source.NavigationID, revision)]; !ok {
					t.Fatalf("document navigation missing: %s", source.Path)
				}
				var restored bytes.Buffer
				for _, page := range source.Pages {
					item, ok := byID[interactive.PackEntryIDForRevision(page.ID, revision)]
					if !ok {
						t.Fatalf("source page missing: %s", page.ID)
					}
					rawJSON, err := json.Marshal(item)
					if err != nil {
						t.Fatal(err)
					}
					var record map[string]any
					if err := json.Unmarshal(rawJSON, &record); err != nil {
						t.Fatal(err)
					}
					if record["audience"] != "gm" || item.LoadMode != "manual" {
						t.Fatalf("author document exposed or resident: %s", page.ID)
					}
					if !strings.HasSuffix(item.Content, expected.Tail) {
						t.Fatalf("tail framing missing: %s", page.ID)
					}
					raw := []byte(item.Content)
					end := len(raw) - page.SuffixBytes
					if page.PrefixBytes > end {
						t.Fatalf("invalid framing: %s", page.ID)
					}
					payload := raw[page.PrefixBytes:end]
					sum := sha256.Sum256(payload)
					if hex.EncodeToString(sum[:]) != page.Hash {
						t.Fatalf("page bytes changed after storage: %s", page.ID)
					}
					restored.Write(payload)
				}
				sum := sha256.Sum256(restored.Bytes())
				if restored.Len() != source.Bytes || hex.EncodeToString(sum[:]) != source.Hash {
					t.Fatalf("source file changed after storage: %s", source.Path)
				}
			}
		})
	}
	if len(seen) != len(expected.Variants) {
		t.Fatalf("tested %d variants, want %d", len(seen), len(expected.Variants))
	}
}
