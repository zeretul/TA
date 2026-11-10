package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestStoryLibraryUnsetDirectoryDoesNotDiscoverWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	manifest := StoryManifest{
		Schema:     "ta.story-media/v1",
		PackHashes: map[string][]string{"pack": {"hash"}},
		PackWorlds: map[string]string{"pack": "world"},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("story-assets.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	// The fixture is loadable only when the caller explicitly selects its directory.
	if l := NewStoryLibrary("."); !l.Current() || !l.PackAllowed("pack", "hash") {
		t.Fatal("explicit working directory did not load the valid fixture")
	}
	for _, dir := range []string{"", " ", "\t\n", "\u3000"} {
		t.Run("unset_"+dir, func(t *testing.T) {
			l := NewStoryLibrary(dir)
			if l.Current() || l.PackAllowed("pack", "hash") || l.manifestSHA != "" || l.manifest.Schema != "" {
				t.Fatal("unset media directory discovered a manifest")
			}
			if len(l.Assets("pack", "npc")) != 0 {
				t.Fatal("unset media directory exposed cached assets")
			}
			// Current must reject an unset directory even if a caller retains a fingerprint.
			sum := sha256.Sum256(raw)
			l.manifestSHA = hex.EncodeToString(sum[:])
			if l.Current() {
				t.Fatal("Current read a working-directory manifest for an unset directory")
			}
		})
	}
}
