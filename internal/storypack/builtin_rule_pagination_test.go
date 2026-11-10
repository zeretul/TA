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

//go:embed assets/rule-candidate-pagination.json
var ruleCandidatePaginationExpectations []byte

// This reads persisted normalized lore, not the raw source JSON. It catches
// loss of payload whitespace at a page boundary during store normalization.
func TestBuiltinRulePaginationSurvivesColdStoreRead(t *testing.T) {
	var expected struct {
		Suffix   string `json:"suffix"`
		Variants map[string]struct {
			Sources []struct {
				OriginalEntryID string `json:"original_entry_id"`
				Hash            string `json:"original_content_sha256"`
				Bytes           int    `json:"original_content_bytes"`
				Pages           []struct {
					ID          string `json:"entry_id"`
					PrefixBytes int    `json:"prefix_bytes"`
					SuffixBytes int    `json:"suffix_bytes"`
					Hash        string `json:"payload_sha256"`
				} `json:"pages"`
			} `json:"sources"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(ruleCandidatePaginationExpectations, &expected); err != nil {
		t.Fatal(err)
	}
	for _, pack := range builtinPacks() {
		spec, ok := expected.Variants[pack.ID]
		if !ok {
			continue
		}
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
				t.Fatalf("seed lost source records: got %d want %d", len(items), len(pack.World))
			}
			revision := pack.ContentHash()
			for _, source := range spec.Sources {
				var restored bytes.Buffer
				for _, page := range source.Pages {
					id := interactive.PackEntryIDForRevision(page.ID, revision)
					item, ok := byID[id]
					if !ok {
						t.Fatalf("page missing after cold read: %s", page.ID)
					}
					if !strings.HasSuffix(item.Content, expected.Suffix) {
						t.Fatalf("page tail framing missing: %s", page.ID)
					}
					raw := []byte(item.Content)
					end := len(raw) - page.SuffixBytes
					if page.PrefixBytes > end {
						t.Fatalf("invalid page frame: %s", page.ID)
					}
					payload := raw[page.PrefixBytes:end]
					sum := sha256.Sum256(payload)
					if hex.EncodeToString(sum[:]) != page.Hash {
						t.Fatalf("payload changed in stored page %s", page.ID)
					}
					restored.Write(payload)
				}
				sum := sha256.Sum256(restored.Bytes())
				if restored.Len() != source.Bytes || hex.EncodeToString(sum[:]) != source.Hash {
					t.Fatalf("stored source reconstruction differs: %s", source.OriginalEntryID)
				}
			}
		})
	}
}
