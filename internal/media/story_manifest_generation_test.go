package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoryLibraryRevocationInvalidatesCachedManifest(t *testing.T) {
	for _, change := range []string{"review", "rule", "packhash", "asset", "malformed", "deleted"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			body := []byte("private image bytes")
			sum := sha256.Sum256(body)
			sha := hex.EncodeToString(sum[:])
			a := StoryAsset{ID: "private", World: "world", SourceKey: "npc", File: sha + ".png", SHA256: sha, Bytes: int64(len(body)), Reviewed: true, Rule: &StoryRule{Kind: "observed_named_portrait"}}
			m := StoryManifest{Schema: "ta.story-media/v1", Assets: []StoryAsset{a}, PackHashes: map[string][]string{"pack": {"hash"}}, PackWorlds: map[string]string{"pack": "world"}}
			original, _ := json.Marshal(m)
			path := filepath.Join(dir, "story-assets.json")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, a.File), body, 0600); err != nil {
				t.Fatal(err)
			}
			l := NewStoryLibrary(dir)
			if !l.Current() || !l.PackAllowed("pack", "hash") {
				t.Fatal("valid generation refused")
			}
			f, err := l.Open(a)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			switch change {
			case "review":
				m.Assets[0].Reviewed = false
			case "rule":
				m.Assets[0].Rule = nil
			case "packhash":
				m.PackHashes = nil
			case "asset":
				m.Assets = nil
			}
			modified, _ := json.Marshal(m)
			if change == "malformed" {
				modified = []byte("bad JSON")
			}
			if change == "deleted" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, modified, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if l.Current() || l.PackAllowed("pack", "hash") {
				t.Fatal("cached grant survived revocation")
			}
			if f, err = l.Open(a); err == nil {
				f.Close()
				t.Fatal("cached reviewed asset opened after revocation")
			}
			if len(l.Assets("pack", "npc")) != 1 {
				t.Fatal("forgot managed identity and would permit fallback")
			}
		})
	}
}
