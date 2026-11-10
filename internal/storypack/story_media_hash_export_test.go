package storypack

import (
	"encoding/json"
	"os"
	"testing"
)

// Explicit opt-in packaging aid. Uses the native loaded content, including its
// normalization, so a JSON-file SHA is never mistaken for a story pack hash.
func TestExportStoryMediaNativePackHashes(t *testing.T) {
	output := os.Getenv("TA_STORY_MEDIA_HASH_OUTPUT")
	if output == "" {
		t.Skip("set TA_STORY_MEDIA_HASH_OUTPUT for isolated packaging")
	}
	manifestPath := os.Getenv("TA_STORY_MEDIA_TEMPLATE")
	if manifestPath == "" {
		t.Fatal("TA_STORY_MEDIA_TEMPLATE required")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		PackWorlds map[string]string `json:"pack_worlds"`
	}
	if err = json.Unmarshal(raw, &template); err != nil {
		t.Fatal(err)
	}
	library := NewLibrary(os.Getenv("TA_STORY_MEDIA_DATA_ROOT"))
	hashes := map[string][]string{}
	for id := range template.PackWorlds {
		pack, err := library.GetContent(id)
		if err != nil {
			t.Fatal(err)
		}
		hashes[id] = []string{pack.ContentHash()}
	}
	raw, err = json.MarshalIndent(hashes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
