package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoryRestrictedBytesNeverUseGlobalOpen(t *testing.T) {
	dir := t.TempDir()
	body := []byte("reviewed private pixels")
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
	file := sha + ".png"
	a := StoryAsset{ID: "asset", File: file, SHA256: sha, Bytes: int64(len(body)), Reviewed: true, Rule: &StoryRule{Kind: "observed_named_portrait"}}
	raw, _ := json.Marshal(StoryManifest{Schema: "ta.story-media/v1", Assets: []StoryAsset{a}})
	if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), body, 0600); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"schema": "ta.media/v2", "entries": []map[string]any{{"id": "legacy", "source_key": "npc", "file": file, "sha256": sha, "bytes": len(body), "reviewed": true, "purpose": "portrait", "subject": "character"}}}
	raw, _ = json.Marshal(legacy)
	if err := os.WriteFile(filepath.Join(dir, "media.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if f, _, err := NewLibrary(dir).Open(file); err == nil {
		f.Close()
		t.Fatal("reviewed legacy URL bypassed story gate")
	}
	l := NewStoryLibrary(dir)
	f, err := l.Open(a)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.WriteFile(filepath.Join(dir, file), []byte("tampered bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := l.Open(a); err == nil {
		f.Close()
		t.Fatal("source substitution accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "story-assets.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if !StoryRestrictedFile(dir, file) {
		t.Fatal("invalid restriction manifest failed open")
	}
}
