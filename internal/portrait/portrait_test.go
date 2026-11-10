package portrait

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"
)

// TestTheShippedSetCoversTheFormalCatalog is the promise F1 makes to the
// character window: every person the formal catalog knows has a picture, and the
// picture served is the one the manifest fingerprints.
//
// It reads the shipped assets, so it fails if a file was dropped, truncated or
// swapped. That is the failure worth catching: a missing portrait is a blank
// circle the player cannot explain, and a wrong one is another person's face.
func TestTheShippedSetCoversTheFormalCatalog(t *testing.T) {
	library := NewLibrary()
	keys := library.Keys()
	if len(keys) == 0 {
		t.Fatal("the shipped set is empty")
	}
	for _, key := range keys {
		entry, ok := library.Lookup(key)
		if !ok {
			t.Fatalf("%s is listed but does not resolve", key)
		}
		if entry.File != key+".png" {
			t.Fatalf("%s serves from %q", key, entry.File)
		}
		data, contentType, err := library.Read(entry.File, false)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if contentType != "image/png" {
			t.Fatalf("%s is served as %q", key, contentType)
		}
		if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
			t.Fatalf("%s is not a PNG", key)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != entry.SHA256 {
			t.Fatalf("%s hashes to %s, the manifest says %s", key, got, entry.SHA256)
		}
		if url := library.URL(key); url != PathPrefix+entry.File {
			t.Fatalf("%s is reached at %q", key, url)
		}
	}
	if library.SourceManifestSHA() == "" {
		t.Fatal("the shipped set does not name the delivery it came from")
	}
}

// TestAnUnknownPersonHasNoPortraitAndNoError states the normal case rather than
// an edge case. Most people in a story are invented by the story itself; they
// have no author key, so there is nothing to look up and the window draws its
// placeholder. Returning an error here would make an ordinary story look broken.
func TestAnUnknownPersonHasNoPortraitAndNoError(t *testing.T) {
	library := NewLibrary()
	for _, key := range []string{"", "   ", "npc_nobody_here", "canon_fairy_nichang "} {
		entry, ok := library.Lookup(key)
		if key == "canon_fairy_nichang " {
			// A stray space is trimmed rather than treated as a different person.
			if !ok {
				t.Fatalf("a padded key %q did not resolve", key)
			}
			continue
		}
		if ok {
			t.Fatalf("key %q resolved to %q", key, entry.SourceKey)
		}
		if url := library.URL(key); url != "" {
			t.Fatalf("key %q produced url %q", key, url)
		}
	}
}

// TestTheRouteCannotReachOutsideTheShippedSet pins the route's shape. A key
// becomes a file name and a URL segment, so the reader must reject anything that
// is not one of the manifest's own names — including traversal, absolute paths,
// other extensions and a name that happens to exist in the embedded tree.
func TestTheRouteCannotReachOutsideTheShippedSet(t *testing.T) {
	library := NewLibrary()
	listed := library.Keys()[0]
	rejected := []string{
		"",
		"   ",
		"..",
		"../" + listed + ".png",
		"../portraits.json",
		"assets/" + listed + ".png",
		`assets\` + listed + ".png",
		"/" + listed + ".png",
		"portraits.json",
		listed,
		listed + ".json",
		listed + ".png.png",
		"npc_nobody_here.png",
		"..%2F" + listed + ".png",
	}
	for _, name := range rejected {
		if _, _, err := library.Read(name, false); err == nil {
			t.Fatalf("Read(%q) succeeded", name)
		} else if !isNotExist(err) {
			t.Fatalf("Read(%q) failed with %v, want not-exist", name, err)
		}
	}
	if _, _, err := library.Read(listed+".png", false); err != nil {
		t.Fatalf("Read(%q) failed: %v", listed+".png", err)
	}
}

// TestTheLargerDerivativeIsTheSamePerson states what the single-person window
// relies on: asking for the bigger picture of a key returns that key's own 192px
// image, fingerprinted by the manifest, and asking for anyone else's name still
// fails.
func TestTheLargerDerivativeIsTheSamePerson(t *testing.T) {
	library := NewLibrary()
	keys := library.Keys()
	if len(keys) == 0 {
		t.Fatal("the shipped set is empty")
	}
	for _, key := range keys {
		entry, ok := library.Lookup(key)
		if !ok {
			t.Fatalf("%s is listed but does not resolve", key)
		}
		if entry.DisplayFile != key+".png" {
			t.Fatalf("%s displays from %q", key, entry.DisplayFile)
		}
		data, contentType, err := library.Read(entry.DisplayFile, true)
		if err != nil {
			t.Fatalf("%s display: %v", key, err)
		}
		if contentType != "image/png" {
			t.Fatalf("%s display is served as %q", key, contentType)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != entry.DisplaySHA256 {
			t.Fatalf("%s display hashes to %s, the manifest says %s", key, got, entry.DisplaySHA256)
		}
		if url := library.DisplayURL(key); url != PathPrefix+entry.DisplayFile+"?"+DisplayQuery {
			t.Fatalf("%s display is reached at %q", key, url)
		}
	}
}

// TestTheLargerDerivativeCannotBeUsedToReadAnotherFile keeps the display flag
// from widening the route: the name still has to be the one that key declares.
func TestTheLargerDerivativeCannotBeUsedToReadAnotherFile(t *testing.T) {
	library := NewLibrary()
	keys := library.Keys()
	first, second := keys[0], keys[1]
	if _, _, err := library.Read(second+".png", true); err != nil {
		t.Fatalf("%s display failed: %v", second, err)
	}
	// Reading one person's thumbnail name with the display flag must not return
	// anything, because no key declares it as its own display name.
	for _, name := range []string{"", "..", "../" + first + ".png", "portraits.json", "assets/display/" + first + ".png"} {
		if _, _, err := library.Read(name, true); err == nil {
			t.Fatalf("Read(%q, display) succeeded", name)
		}
	}
	// A key with no display derivative reports none rather than a broken URL.
	if url := library.DisplayURL("npc_nobody_here"); url != "" {
		t.Fatalf("an unknown key produced display url %q", url)
	}
}

func isNotExist(err error) bool {
	return err != nil && strings.Contains(err.Error(), fs.ErrNotExist.Error())
}
