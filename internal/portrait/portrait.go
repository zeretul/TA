// Package portrait serves the shipped default portraits for the formal
// character catalog.
//
// The default package was delivered as 101 main images plus two thumbnails
// each, 236 MB in all. That whole directory is a review artifact, not a runtime
// asset: what the character window draws is an 8-pixel-radius circle, so only
// the 128px thumbnails are shipped here (about 3.3 MB) and nothing is generated
// at runtime.
//
// Two things this package deliberately does not do:
//
//   - It does not decide who a person is. A portrait is keyed by the author's
//     stable source key, never by a display name, because two worlds may hold
//     same-named people and a name match would put one person's face on another.
//   - It does not expose paths, prompts, hashes, or the review state of the
//     source package. A caller learns only whether a key has a portrait, and a
//     URL that resolves to it.
package portrait

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
)

// PathPrefix is where the browser reads a portrait from. It is served by the
// API's own read-only route, so the assets are not copied into the web build and
// a production bundle needs no extra directory on disk.
const PathPrefix = "/api/portraits/"

// DisplayQuery selects the larger derivative on the same route. The thumbnail is
// what a roster row draws; a person's own window can show the picture bigger
// without shipping the 225 MB review originals.
const DisplayQuery = "size=display"

//go:embed assets/portraits.json assets/*.png assets/display/*.png
var assets embed.FS

// Entry is one default portrait: which author key it belongs to, what the
// author called that person, and the picture's own fingerprint.
type Entry struct {
	SourceKey   string
	DisplayName string
	SHA256      string
	// File is the name inside the embedded assets directory. It is derived from
	// the source key rather than read from the source manifest, so a malformed
	// key cannot become a path.
	File string
	// Display is the same picture at 192px, for a window that shows one person
	// larger than a roster row. It is a derivative of the same selected default,
	// never a second face.
	DisplayFile   string
	DisplaySHA256 string
}

type catalog struct {
	Schema            string  `json:"schema"`
	SourceManifest    string  `json:"source_manifest"`
	SourceManifestSHA string  `json:"source_manifest_sha256"`
	Derivation        string  `json:"derivation"`
	Entries           []entry `json:"entries"`
}

// derivative is the manifest's record of a derived image: the name, its hash,
// and the size it was made at.
type derivative struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type entry struct {
	SourceKey   string      `json:"source_key"`
	DisplayName string      `json:"display_name"`
	SHA256      string      `json:"sha256"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Display     *derivative `json:"display"`
}

// Library is the read-only set of shipped default portraits.
type Library struct {
	mu      sync.RWMutex
	byKey   map[string]Entry
	loaded  bool
	loadErr error
}

// NewLibrary returns the library. Reading is lazy: a build without the assets
// still starts, and the failure surfaces as "no portrait" rather than as a
// broken server.
func NewLibrary() *Library { return &Library{} }

func (l *Library) load() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.loaded {
		return
	}
	l.loaded = true
	document, err := assets.ReadFile("assets/portraits.json")
	if err != nil {
		l.loadErr = fmt.Errorf("read the default portrait manifest: %w", err)
		return
	}
	var parsed catalog
	if err := json.Unmarshal(document, &parsed); err != nil {
		l.loadErr = fmt.Errorf("parse the default portrait manifest: %w", err)
		return
	}
	byKey := make(map[string]Entry, len(parsed.Entries))
	for _, item := range parsed.Entries {
		key := strings.TrimSpace(item.SourceKey)
		if !validSourceKey(key) {
			// A row that cannot be named safely is dropped rather than sanitized:
			// rewriting the key would bind a face to a different person.
			continue
		}
		byKey[key] = Entry{
			SourceKey: key, DisplayName: strings.TrimSpace(item.DisplayName),
			SHA256: strings.TrimSpace(item.SHA256), File: key + ".png",
			DisplayFile: displayFile(key, item.Display), DisplaySHA256: displaySHA256(item.Display),
		}
	}
	l.byKey = byKey
}

// Lookup returns the portrait bound to an author source key. An empty or
// unknown key is a normal answer, not an error: most people in a story are
// invented, and the window draws a placeholder for them.
func (l *Library) Lookup(sourceKey string) (Entry, bool) {
	key := strings.TrimSpace(sourceKey)
	if key == "" || !validSourceKey(key) {
		return Entry{}, false
	}
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.loadErr != nil {
		return Entry{}, false
	}
	entry, ok := l.byKey[key]
	return entry, ok
}

// displayFile names the 192px derivative for a key. Like File it is derived from
// the key, and the manifest's own name is only accepted when it agrees, so a
// manifest row cannot point the reader at another person's picture.
func displayFile(key string, declared *derivative) string {
	if declared == nil || strings.TrimSpace(declared.File) != key+".png" {
		return ""
	}
	return declared.File
}

func displaySHA256(declared *derivative) string {
	if declared == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(declared.SHA256))
}

// URL is the path a browser reads a person's default portrait from. It returns
// "" when the person has none, so a caller can pass it straight through and the
// window falls back to its placeholder.
func (l *Library) URL(sourceKey string) string {
	entry, ok := l.Lookup(sourceKey)
	if !ok {
		return ""
	}
	return PathPrefix + url.PathEscape(entry.File)
}

// DisplayURL is the same picture at 192px. It returns "" when the person has no
// portrait, and also when this build has no larger derivative for them, so the
// caller keeps the thumbnail rather than drawing a broken image.
func (l *Library) DisplayURL(sourceKey string) string {
	entry, ok := l.Lookup(sourceKey)
	if !ok || entry.DisplayFile == "" {
		return ""
	}
	return PathPrefix + url.PathEscape(entry.DisplayFile) + "?" + DisplayQuery
}

// Read returns one portrait's bytes and media type. Only a key the manifest
// actually lists can be read, so the route is a lookup rather than a directory
// listing: a request for an unlisted file gets not-found even if a file of that
// name happened to exist in the embedded tree.
//
// With display set, the same person's 192px derivative is returned instead. The
// name still has to be the one that key's own manifest row declares, so the
// larger picture cannot become a way to read a different file.
func (l *Library) Read(file string, display bool) ([]byte, string, error) {
	name := strings.TrimSpace(file)
	if name == "" || strings.ContainsAny(name, `/\`) || name != path.Base(name) {
		return nil, "", fs.ErrNotExist
	}
	key := strings.TrimSuffix(name, ".png")
	if key == name || !validSourceKey(key) {
		return nil, "", fs.ErrNotExist
	}
	entry, ok := l.Lookup(key)
	if !ok {
		return nil, "", fs.ErrNotExist
	}
	assetPath := "assets/" + entry.File
	if display {
		if entry.DisplayFile != name {
			return nil, "", fs.ErrNotExist
		}
		assetPath = "assets/display/" + entry.DisplayFile
	} else if entry.File != name {
		return nil, "", fs.ErrNotExist
	}
	data, err := assets.ReadFile(assetPath)
	if err != nil {
		return nil, "", err
	}
	return data, "image/png", nil
}

// Keys lists the shipped source keys in stable order. It exists for tests and
// diagnostics, not for a request path.
func (l *Library) Keys() []string {
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	keys := make([]string, 0, len(l.byKey))
	for key := range l.byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// SourceManifestSHA reports which delivery of the default package these images
// were taken from. A report can cite it; nothing in the request path reads it.
func (l *Library) SourceManifestSHA() string {
	document, err := assets.ReadFile("assets/portraits.json")
	if err != nil {
		return ""
	}
	var parsed catalog
	if err := json.Unmarshal(document, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.SourceManifestSHA)
}

// validSourceKey accepts the author's own key shape. It is intentionally narrow:
// the key becomes a file name and a URL segment, so allowing anything else would
// turn a data field into a path.
func validSourceKey(key string) bool {
	if key == "" || len(key) > 64 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return !strings.Contains(key, "..")
}
