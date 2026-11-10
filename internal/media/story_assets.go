package media

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Story assets are a separate, private catalogue. They never enter media.json
// or the global URL resolver. Authored rules are data, not per-world code.
type StoryRule struct {
	// current_observation is for reviewed images without authored calendar or
	// substage prerequisites. Empty/explicit_year preserves the legacy gate.
	TimeSemantics string `json:"time_semantics,omitempty"`
	Kind          string `json:"kind"`
	Year          int    `json:"year"`
	Form          string `json:"form"`
	Lifeform      string `json:"lifeform"`
	Gender        string `json:"gender"`
	KnownAs       string `json:"known_as"`
	// KnownAsAliases contains only exact, author-reviewed public identities.
	// It never teaches an identity or replaces committed recognition evidence.
	KnownAsAliases []string `json:"known_as_aliases,omitempty"`
	CanonicalName  string   `json:"canonical_name"`
	Role           string   `json:"role"`
	// Empty preserves the original substantive role gate. Only reviewed authored
	// mappings may classify a role as background; model drafts cannot select it.
	RoleSemantics string `json:"role_semantics,omitempty"`
	// RoleRecognition is an explicitly reviewed occupational phrase, never an
	// automatically shortened role. Empty preserves full-role source validation.
	RoleRecognition string `json:"role_recognition,omitempty"`
	AdultIdentity   bool   `json:"adult_identity,omitempty"`
	// Location is required only by observed_named_portrait_at_location. The new
	// kind makes older consumers fail closed rather than ignore this restriction.
	// Its exact authored value neither resolves prose nor creates game facts.
	Location string `json:"location,omitempty"`
}

// AllowsKnownAs validates the entire opt-in list before matching. Malformed
// authored data fails closed, including for the canonical identity; no trimming
// or prefix matching can silently expand an authorization.
func (r StoryRule) AllowsKnownAs(knownAs string) bool {
	if r.KnownAs == "" {
		return false
	}
	seen := map[string]bool{r.KnownAs: true}
	matched := knownAs == r.KnownAs
	for _, alias := range r.KnownAsAliases {
		if alias == "" || strings.TrimSpace(alias) != alias || !utf8.ValidString(alias) || seen[alias] {
			return false
		}
		for _, ch := range alias {
			if unicode.IsControl(ch) || unicode.Is(unicode.Cf, ch) {
				return false
			}
		}
		seen[alias] = true
		if knownAs == alias {
			matched = true
		}
	}
	return matched
}

// AllowsLocation checks an authored restriction against the committed scene's
// projected Location value. The location-specific kind is a required discriminator
// so older consumers reject the rule instead of ignoring an unknown JSON field.
// Exact matching never expands to aliases, sublocations, IDs, substrings, or
// normalized text. The app still requires a matching live visual observation and
// all identity, presence, time, and provenance checks.
func (r StoryRule) AllowsLocation(sceneLocation string) bool {
	switch r.Kind {
	case "observed_named_portrait":
		return r.Location == ""
	case "observed_named_portrait_at_location":
		if r.Location == "" || strings.TrimSpace(r.Location) != r.Location || !utf8.ValidString(r.Location) {
			return false
		}
		for _, ch := range r.Location {
			if unicode.IsControl(ch) || unicode.Is(unicode.Cf, ch) {
				return false
			}
		}
		return sceneLocation == r.Location
	default:
		return false
	}
}

type StoryAsset struct {
	ID         string          `json:"id"`
	World      string          `json:"world"`
	SourceKey  string          `json:"source_key"`
	File       string          `json:"file"`
	SHA256     string          `json:"sha256"`
	Bytes      int64           `json:"bytes"`
	Reviewed   bool            `json:"reviewed"`
	Rule       *StoryRule      `json:"rule,omitempty"`
	Caption    string          `json:"caption,omitempty"`
	Provenance json.RawMessage `json:"provenance,omitempty"`
}
type StoryManifest struct {
	Schema     string              `json:"schema"`
	PackHashes map[string][]string `json:"pack_hashes"`
	PackWorlds map[string]string   `json:"pack_worlds"`
	Assets     []StoryAsset        `json:"assets"`
}
type StoryLibrary struct {
	dir         string
	manifest    StoryManifest
	manifestSHA string
}

func NewStoryLibrary(dir string) *StoryLibrary {
	l := &StoryLibrary{dir: dir}
	// An unset media directory disables story media; it is not the working directory.
	if strings.TrimSpace(dir) == "" {
		return l
	}
	raw, err := os.ReadFile(filepath.Join(dir, "story-assets.json"))
	if err != nil {
		return l
	}
	var m StoryManifest
	if json.Unmarshal(raw, &m) != nil || m.Schema != "ta.story-media/v1" {
		return l
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		if a.ID == "" || seen[a.ID] || !safeStoryFile(a) {
			return l
		}
		seen[a.ID] = true
	}
	l.manifest = m
	sum := sha256.Sum256(raw)
	l.manifestSHA = hex.EncodeToString(sum[:])
	return l
}
func safeStoryFile(a StoryAsset) bool {
	sha, err := hex.DecodeString(a.SHA256)
	return err == nil && len(sha) == 32 && a.SHA256 == strings.ToLower(a.SHA256) && a.File == filepath.Base(a.File) && !strings.ContainsAny(a.File, `/\`) && strings.TrimSuffix(a.File, filepath.Ext(a.File)) == a.SHA256 && (filepath.Ext(a.File) == ".png" || filepath.Ext(a.File) == ".jpg" || filepath.Ext(a.File) == ".webp") && a.Bytes > 0
}
func (l *StoryLibrary) PackAllowed(world, hash string) bool {
	if l == nil || hash == "" || !l.Current() {
		return false
	}
	for _, h := range l.manifest.PackHashes[world] {
		if h == hash {
			return true
		}
	}
	return false
}
func (l *StoryLibrary) Assets(world, source string) []StoryAsset {
	if l == nil {
		return nil
	}
	var out []StoryAsset
	for _, a := range l.manifest.Assets {
		if l.MatchesWorld(a, world) && a.SourceKey == source {
			out = append(out, a)
		}
	}
	return out
}
func (l *StoryLibrary) Asset(id string) (StoryAsset, bool) {
	if l != nil {
		for _, a := range l.manifest.Assets {
			if a.ID == id {
				return a, true
			}
		}
	}
	return StoryAsset{}, false
}

// Open is intentionally not a URL. The app must first authorize the current
// story snapshot. Rehashing the open handle prevents a replaced source file
// from changing the reviewed pixels, including on HEAD and Range requests.
func (l *StoryLibrary) Open(a StoryAsset) (*os.File, error) {
	if l == nil || !l.Current() || !a.Reviewed || a.Rule == nil || !safeStoryFile(a) {
		return nil, os.ErrNotExist
	}
	f, err := os.Open(filepath.Join(l.dir, a.File))
	if err != nil {
		return nil, os.ErrNotExist
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != a.Bytes {
		f.Close()
		return nil, os.ErrNotExist
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		f.Close()
		return nil, os.ErrNotExist
	}
	if !l.Current() {
		f.Close()
		return nil, os.ErrNotExist
	}
	if _, err = f.Seek(0, 0); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// StoryRestrictedFile also covers legacy imported filenames. If an installed
// restriction manifest is malformed, the global route fails closed rather
// than silently making potentially private files public.
func StoryRestrictedFile(dir, file string, hashes ...string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, "story-assets.json"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	return storyRestrictedContent(raw, file, hashes...)
}

// storyRestrictedContent evaluates the exact bytes whose generation was checked.
func storyRestrictedContent(raw []byte, file string, hashes ...string) bool {
	var m StoryManifest
	if json.Unmarshal(raw, &m) != nil || m.Schema != "ta.story-media/v1" {
		return true
	}
	for _, a := range m.Assets {
		if !safeStoryFile(a) {
			return true
		}
		for _, hash := range hashes {
			if hash != "" && hash == a.SHA256 {
				return true
			}
		}
		if file == a.File || file == "u-"+a.File {
			return true
		}
	}
	return false
}

func (l *StoryLibrary) MatchesWorld(a StoryAsset, packID string) bool {
	if l == nil {
		return false
	}
	return l.manifest.PackWorlds[packID] != "" && l.manifest.PackWorlds[packID] == a.World
}

// Current fences authorization to exactly the manifest generation reviewed at
// startup. Any edit, revocation, removal, read failure or malformed replacement
// closes the gate immediately. Cached asset identities remain available only
// to mark managed subjects and prevent fallback to unscoped image sources.
// This is fail-closed invalidation, not hot reloading or an immutability claim.
func (l *StoryLibrary) Current() bool {
	if l == nil || strings.TrimSpace(l.dir) == "" || l.manifestSHA == "" {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(l.dir, "story-assets.json"))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) == l.manifestSHA
}
