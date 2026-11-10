// Package media serves the pictures and clips that live beside the binary.
//
// They are not embedded: a clip is tens of megabytes, a picture may be added at
// any time, and a build without the directory still has to start. They live in
// a directory next to the executable (the packaged layout puts them at
// game/media) and are addressed by a manifest that binds a file to the subject
// it belongs to and the use it is allowed for.
//
// One manifest carries both kinds because they are the same problem: a file, a
// subject, a purpose. A clip is a moment of a person; an image is their face or
// the place they stand in. Adding either is a manifest entry plus a file, never
// a rebuild.
//
// Three things this package deliberately does not do:
//
//   - It does not decide who a person is. A file is bound to the author's stable
//     source key, or to the story's own actor identity, or to a registered place
//     name — never to a display name and never to facial similarity.
//   - It does not expose a directory. A request names a file the manifest
//     already lists, so an arbitrary path on disk cannot be read through it.
//   - It does not transcode. The files are served as they were reviewed.
package media

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "golang.org/x/image/webp"
)

// PathPrefix is where the browser reads a file from. It is served by the API's
// own read-only route, so a production bundle needs no extra web directory.
const PathPrefix = "/api/media/"

// ManifestFile is the name of the manifest inside the media directory.
const ManifestFile = "media.json"

// ManifestSchema is the shape this package writes. Reading accepts an older
// document too: a clip entry that names no purpose is a general-purpose clip.
const ManifestSchema = "ta.media/v2"

// MaxAssetBytes bounds one imported file. A picture is a face or a place, and a
// clip is a few seconds; anything past this is a mistake rather than material.
const MaxAssetBytes = 16 << 20

// Roles a clip can play. An intimate clip is only offered where the existing
// adult, consent and privacy conditions already hold; it is never an ordinary
// page's picture.
const (
	RoleGeneral  = "general"
	RoleIntimate = "intimate"
)

// Purpose is what an asset is for. The same person may have a roster face, a
// larger portrait, and clips, and a place has its own picture.
const (
	PurposeAvatar   = "avatar"
	PurposePortrait = "portrait"
	PurposeScene    = "scene"
	PurposeClip     = "clip"
)

// Subject is what an asset belongs to.
const (
	SubjectCharacter = "character"
	SubjectPlace     = "place"
)

var (
	// ErrNoMediaDir means this build found no media directory, so nothing can be
	// imported. The game still runs; it simply keeps its built-in pictures.
	ErrNoMediaDir = errors.New("no media directory is available")
	// ErrAssetEmpty means the import carried no bytes.
	ErrAssetEmpty = errors.New("the imported file is empty")
	// ErrAssetTooLarge means the file exceeds MaxAssetBytes.
	ErrAssetTooLarge = errors.New("the imported file is too large")
	// ErrAssetInvalid means the bytes are not a picture or clip this build can
	// serve.
	ErrAssetInvalid = errors.New("the imported file is not a supported picture or clip")
	// ErrAssetUnknown means an entry ID names nothing in the manifest.
	ErrAssetUnknown = errors.New("no such asset")
	// ErrAssetStoryRestricted refuses imports that the global byte route cannot serve.
	ErrAssetStoryRestricted = errors.New("story media restrictions do not allow this file to be imported")
)

// Asset is one file the manifest lists: what it belongs to, what it is for, and
// enough about the picture for a caller to lay it out before it has loaded.
type Asset struct {
	// ID names this entry in the manifest. It is what an import and a removal
	// address, and it is derived from the binding rather than from the file, so
	// replacing a person's picture replaces their entry rather than adding a
	// second one.
	ID string `json:"id"`
	// World is the world pack this asset belongs to. Empty means it may answer
	// for any world; a request that names no world is answered only by an entry
	// that names none either.
	World string `json:"world,omitempty"`
	// Subject is SubjectCharacter or SubjectPlace.
	Subject string `json:"subject,omitempty"`
	// SourceKey is the author's stable identity for a preset person. It is empty
	// for a person the story invented, who is bound by Scope and Actor instead.
	SourceKey string `json:"source_key,omitempty"`
	// Scope is the story a story-invented person belongs to, so one story's face
	// cannot follow the same name into another story.
	Scope string `json:"scope,omitempty"`
	// Actor is the story's own actor ID for a story-invented person.
	Actor string `json:"actor,omitempty"`
	// Place is the registered place name a scene picture belongs to.
	Place string `json:"place,omitempty"`
	// Purpose is PurposeAvatar, PurposePortrait, PurposeScene or PurposeClip.
	Purpose string `json:"purpose,omitempty"`
	// Scene narrows an asset to one scene. Empty means it is the subject's
	// ordinary picture, shown wherever the subject appears.
	Scene string `json:"scene,omitempty"`
	// Reviewed says the author actually looked at this file and accepted it for
	// this subject. It is false unless the manifest says otherwise, so a new
	// entry reaches nobody until someone has watched it: the alternative default
	// would let an unchecked file appear in front of a player on the strength of
	// its tags alone. A picture the player imported themselves is reviewed by
	// that act, because they chose the file for that subject.
	Reviewed bool `json:"reviewed,omitempty"`
	// PlaybackApproval records an installation owner's opt-in independently of visual review.
	PlaybackApproval string `json:"playback_approval,omitempty"`
	Role             string `json:"role,omitempty"`
	File             string `json:"file"`
	SHA256           string `json:"sha256,omitempty"`
	Bytes            int64  `json:"bytes,omitempty"`
	DurationMS       int    `json:"duration_ms,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
}

// Library is the read-only set of shipped and imported files. It is loaded
// lazily so a build without the media directory starts normally and simply has
// no assets.
type Library struct {
	dir string
	// Once installed, any change to the restriction catalogue must fail closed.
	storyRestrictionsSeen bool
	storyRestrictionSHA   [32]byte

	// writeMu serializes manifest rewrites, which read the document, change one
	// entry and write it back. It is separate from mu because a rewrite must not
	// hold the read lock across file I/O.
	writeMu sync.Mutex

	mu      sync.RWMutex
	entries []Asset
	byFile  map[string]Asset
	byKey   map[string][]Asset
	note    string
	loaded  bool
	loadErr error
}

type catalog struct {
	Schema  string        `json:"schema"`
	Note    string        `json:"note,omitempty"`
	Entries []catalogItem `json:"entries"`
}

type catalogItem struct {
	ID               string `json:"id,omitempty"`
	World            string `json:"world,omitempty"`
	Subject          string `json:"subject,omitempty"`
	SourceKey        string `json:"source_key,omitempty"`
	Scope            string `json:"scope,omitempty"`
	Actor            string `json:"actor,omitempty"`
	Place            string `json:"place,omitempty"`
	Purpose          string `json:"purpose,omitempty"`
	Scene            string `json:"scene,omitempty"`
	Reviewed         bool   `json:"reviewed,omitempty"`
	PlaybackApproval string `json:"playback_approval,omitempty"`
	Role             string `json:"role,omitempty"`
	File             string `json:"file"`
	SHA256           string `json:"sha256,omitempty"`
	Bytes            int64  `json:"bytes,omitempty"`
	DurationMS       int    `json:"duration_ms,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
}

// NewLibrary reads the manifest from dir. A missing manifest is not an error:
// the game runs without assets and the window keeps its built-in picture.
func NewLibrary(dir string) *Library {
	library := &Library{dir: strings.TrimSpace(dir)}
	_ = library.storyFileRestriction("")
	return library
}

// Dir is the directory the assets are read from.
func (l *Library) Dir() string { return l.dir }

// Note is the manifest's own note, which the author wrote for the next person to
// open the directory. It is read back so the author's material list can show it.
func (l *Library) Note() string {
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.note
}

// Manifest is the media directory as the author sees it: the schema it is
// written in, and every binding it carries.
//
// The directory's own path is deliberately absent. It is one machine's layout,
// and the design forbids handing a page a path on disk; the player reads files
// through PathPrefix instead.
type Manifest struct {
	Schema string  `json:"schema"`
	Note   string  `json:"note,omitempty"`
	Assets []Asset `json:"assets"`
}

// Imported is what one import produced: the entry that now stands in the
// manifest, and the URL the window will draw it from.
type Imported struct {
	Asset Asset  `json:"asset"`
	URL   string `json:"url"`
}

func (l *Library) load() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loadLocked()
}

func (l *Library) loadLocked() {
	if l.loaded {
		return
	}
	l.loaded = true
	l.entries = nil
	l.byFile = map[string]Asset{}
	l.byKey = map[string][]Asset{}
	l.loadErr = nil
	if l.dir == "" {
		return
	}
	document, err := os.ReadFile(filepath.Join(l.dir, ManifestFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			l.loadErr = fmt.Errorf("read the media manifest: %w", err)
		}
		return
	}
	var parsed catalog
	if err := json.Unmarshal(document, &parsed); err != nil {
		l.loadErr = fmt.Errorf("parse the media manifest: %w", err)
		return
	}
	l.note = strings.TrimSpace(parsed.Note)
	for _, item := range parsed.Entries {
		asset, ok := normalizeItem(item)
		if !ok {
			continue
		}
		l.entries = append(l.entries, asset)
		l.byFile[asset.File] = asset
		if asset.Purpose == PurposeClip && asset.SourceKey != "" {
			l.byKey[asset.SourceKey] = append(l.byKey[asset.SourceKey], asset)
		}
	}
	sort.SliceStable(l.entries, func(i, j int) bool { return l.entries[i].ID < l.entries[j].ID })
	for key := range l.byKey {
		clips := l.byKey[key]
		sort.Slice(clips, func(i, j int) bool {
			if clips[i].Role != clips[j].Role {
				return clips[i].Role < clips[j].Role
			}
			return clips[i].File < clips[j].File
		})
	}
}

// normalizeItem validates one manifest row and fills in the fields an older
// document did not carry. A row that cannot be addressed safely is dropped
// rather than repaired: rewriting a binding would point a picture at a different
// subject.
func normalizeItem(item catalogItem) (Asset, bool) {
	file := strings.TrimSpace(item.File)
	if !validToken(file) {
		return Asset{}, false
	}
	purpose := strings.TrimSpace(item.Purpose)
	if purpose == "" {
		purpose = PurposeClip
	}
	subject := strings.TrimSpace(item.Subject)
	if subject == "" {
		subject = SubjectCharacter
	}
	if subject != SubjectCharacter && subject != SubjectPlace {
		return Asset{}, false
	}
	switch purpose {
	case PurposeAvatar, PurposePortrait, PurposeScene, PurposeClip:
	default:
		return Asset{}, false
	}
	role := strings.TrimSpace(item.Role)
	if role == "" {
		role = RoleGeneral
	} else if role != RoleGeneral && role != RoleIntimate {
		return Asset{}, false
	}
	asset := Asset{
		ID:               strings.TrimSpace(item.ID),
		World:            normalizeWorld(item.World),
		Subject:          subject,
		SourceKey:        strings.TrimSpace(item.SourceKey),
		Scope:            strings.TrimSpace(item.Scope),
		Actor:            strings.TrimSpace(item.Actor),
		Place:            normalizePlace(item.Place),
		Purpose:          purpose,
		Scene:            strings.TrimSpace(item.Scene),
		Reviewed:         item.Reviewed,
		PlaybackApproval: item.PlaybackApproval,
		Role:             role,
		File:             file,
		SHA256:           strings.ToLower(strings.TrimSpace(item.SHA256)),
		Bytes:            item.Bytes,
		DurationMS:       item.DurationMS,
		Width:            item.Width,
		Height:           item.Height,
	}
	if asset.Subject == SubjectCharacter {
		asset.Place = ""
		// A character entry has to name an identity. Without one it would answer
		// for every person in every story, which is exactly the borrowing the
		// design forbids.
		if asset.SourceKey == "" && (asset.Scope == "" || asset.Actor == "") {
			return Asset{}, false
		}
	} else {
		// A place the pack registered belongs to the world and names no scope, so
		// every story in that world draws it. A place one story invented is bound
		// by that story's scope, so the same name in a sibling story is a different
		// place and does not inherit this picture.
		asset.SourceKey, asset.Actor = "", ""
		if asset.Place == "" {
			return Asset{}, false
		}
	}
	if asset.ID == "" {
		asset.ID = assetID(asset)
	}
	return asset, true
}

// assetID is the stable name of one binding. It is the identity plus the use, so
// importing a second picture for the same person and purpose replaces the first
// rather than adding a duplicate that would win or lose by accident.
func assetID(asset Asset) string {
	parts := []string{asset.World, asset.Subject}
	if asset.Subject == SubjectPlace {
		// The scope separates a place one story invented from a place of the same
		// name in a sibling story, so the two can hold different pictures at once.
		if asset.Scope != "" {
			parts = append(parts, "scope", asset.Scope)
		}
		parts = append(parts, asset.Place)
	} else if asset.SourceKey != "" {
		parts = append(parts, "src", asset.SourceKey)
	} else {
		parts = append(parts, "actor", asset.Scope, asset.Actor)
	}
	parts = append(parts, asset.Purpose)
	if asset.Scene != "" {
		parts = append(parts, "scene", asset.Scene)
	}
	if asset.Purpose == PurposeClip {
		parts = append(parts, asset.Role, asset.File)
	}
	return strings.Join(parts, ":")
}

func normalizeWorld(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizePlace(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

// validToken accepts a name that is a single path element with no separators and
// no parent reference.
func validToken(name string) bool {
	if name == "" || name != path.Base(name) || strings.ContainsAny(name, `/\`) {
		return false
	}
	return name != "." && name != ".."
}

// Clips returns the clips bound to an author source key, general ones first. An
// empty or unknown key returns nothing, which is the normal answer for a person
// this build has no clip for.
func (l *Library) Clips(sourceKey string) []Asset {
	key := strings.TrimSpace(sourceKey)
	if !validToken(key) {
		return nil
	}
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.loadErr != nil {
		return nil
	}
	clips := l.byKey[key]
	if len(clips) == 0 {
		return nil
	}
	out := make([]Asset, len(clips))
	copy(out, clips)
	return out
}

// General returns the person's ordinary-day clip, or false when this build has
// none. It is the only role an ordinary character window offers.
func (l *Library) General(sourceKey string) (Asset, bool) {
	for _, clip := range l.Clips(sourceKey) {
		if clip.Role == RoleGeneral {
			return clip, true
		}
	}
	return Asset{}, false
}

// Intimate returns the person's private clip. The caller is responsible for
// having established that the existing adult, consent and privacy conditions
// hold before asking.
func (l *Library) Intimate(sourceKey string) (Asset, bool) {
	for _, clip := range l.Clips(sourceKey) {
		if clip.Role == RoleIntimate {
			return clip, true
		}
	}
	return Asset{}, false
}

// PickGeneral is General with a choice among the person's several ordinary-day
// clips, seeded so the same scene keeps the same one.
func (l *Library) PickGeneral(sourceKey, seed string) (Asset, bool) {
	return Pick(byRole(l.Clips(sourceKey), RoleGeneral), seed)
}

// PickIntimate is Intimate with the same seeded choice. The caller still owns
// the adult, consent and privacy conditions.
func (l *Library) PickIntimate(sourceKey, seed string) (Asset, bool) {
	return Pick(byRole(l.Clips(sourceKey), RoleIntimate), seed)
}

func byRole(assets []Asset, role string) []Asset {
	out := make([]Asset, 0, len(assets))
	for _, asset := range assets {
		if asset.Role == role {
			out = append(out, asset)
		}
	}
	return out
}

// URL is the path a browser reads an asset from.
func URL(asset Asset) string {
	if asset.File == "" {
		return ""
	}
	return PathPrefix + asset.File
}

// ContentType is what the route answers with. A picture and a clip share one
// route, so the type comes from the file the manifest listed rather than from
// the request.
func ContentType(file string) string {
	switch strings.ToLower(path.Ext(file)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".webm":
		return "video/webm"
	default:
		return "video/mp4"
	}
}

// Pick chooses one clip out of a person's set, deterministically from seed. A
// person usually has several reviewed clips of different moments, and the window
// shows one rather than a gallery; seeding it from the scene means the same
// scene shows the same clip every time it is reopened, and a different scene
// shows a different one.
func Pick(assets []Asset, seed string) (Asset, bool) {
	switch len(assets) {
	case 0:
		return Asset{}, false
	case 1:
		return assets[0], true
	}
	digest := fnv.New32a()
	_, _ = digest.Write([]byte(seed))
	return assets[int(digest.Sum32()%uint32(len(assets)))], true
}

// PictureQuery names the subject a caller wants a picture for. Every field is
// what the story has already committed: the world it belongs to, the person's
// author key or the story's own actor identity, and the scene on screen.
type PictureQuery struct {
	// Role defaults to the ordinary view. Private activity callers must request
	// the intimate role only after their existing eligibility checks succeed.
	Role string
	// World is the world pack the story is bound to. An entry that names a
	// different world never answers, and an entry that names one does not answer
	// a query that names none.
	World string
	// SourceKey is the author's stable key for a preset person.
	SourceKey string
	// Scope and Actor bind a person the story invented. For a place, Scope alone
	// binds a place that story invented: a place the world pack registered is
	// shared by every story in that world and names no scope, while a place one
	// story made up answers only that story.
	Scope string
	Actor string
	// Place is a place name, for a scene picture.
	Place string
	// Purpose is PurposeAvatar, PurposePortrait or PurposeScene.
	Purpose string
	// SceneKey is the current scene. An entry bound to that scene outranks the
	// subject's ordinary picture; an entry bound to a different scene is not a
	// candidate at all.
	SceneKey string
}

// Picture returns the best picture for a subject.
//
// The order it enforces is the design's: a picture bound to the scene on screen
// beats the subject's ordinary picture, and a picture for another world, another
// story, another place or another purpose is not a candidate. What it does not
// do is fall back to a different subject — a caller that gets nothing draws the
// built-in picture or a placeholder, and never borrows a face.
//
// A query that names a place asks for a place picture and never matches a
// person; a query that names a person never matches a place. Two subjects are
// separate lookups because a scene's own picture must not become a person's.
func (l *Library) Picture(query PictureQuery) (Asset, bool) {
	if query.Purpose == "" || query.Purpose == PurposeClip {
		return Asset{}, false
	}
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.loadErr != nil {
		return Asset{}, false
	}
	best, bestScore := Asset{}, -1
	for _, asset := range l.entries {
		if score := bindingScore(asset, query); score > bestScore {
			best, bestScore = asset, score
		}
	}
	return best, bestScore >= 0
}

// Open resolves a requested file name to an open file and its asset. Only a name
// the manifest lists can be opened, so the route is a lookup rather than a
// directory read.
func (l *Library) Open(file string) (*os.File, Asset, error) {
	name := strings.TrimSpace(file)
	if !validToken(name) {
		return nil, Asset{}, fs.ErrNotExist
	}
	l.load()
	l.mu.RLock()
	asset, ok := l.byFile[name]
	loadErr := l.loadErr
	l.mu.RUnlock()
	if loadErr != nil {
		return nil, Asset{}, loadErr
	}
	// Review is required at the byte-serving boundary too; omitting a URL
	// from a player view does not prevent a direct request for that file.
	if !ok || !asset.Reviewed || asset.isActivityClip() {
		return nil, Asset{}, fs.ErrNotExist
	}
	restriction := l.storyFileRestriction(name, asset.SHA256)
	if restriction == storyRestrictionDenied {
		return nil, Asset{}, fs.ErrNotExist
	}
	handle, err := os.Open(filepath.Join(l.dir, asset.File))
	if err != nil {
		return nil, Asset{}, err
	}
	if restriction == storyRestrictionAbsent {
		// Ordinary never-installed directories keep their original streaming
		// behavior, including HEAD and video Range requests. Recheck after open:
		// a newly discovered catalogue must enter the byte-verification path.
		switch l.storyFileRestriction(name, asset.SHA256) {
		case storyRestrictionAbsent:
			return handle, asset, nil
		case storyRestrictionDenied:
			_ = handle.Close()
			return nil, Asset{}, fs.ErrNotExist
		case storyRestrictionActive:
			// Verify the actual bytes below, regardless of the file extension.
		}
	}
	// A legacy alias may omit or misstate SHA256. The declaration is not
	// evidence of the bytes, so inspect the same open handle that the handler
	// will stream. Otherwise a reviewed alias could expose a story-gated image
	// on the global GET, HEAD, or Range route. Ordinary hashless legacy media
	// remains supported: only bytes in the private catalogue are refused.
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		_ = handle.Close()
		return nil, Asset{}, fs.ErrNotExist
	}
	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if l.storyFileRestriction(name, actualSHA) == storyRestrictionDenied {
		_ = handle.Close()
		return nil, Asset{}, fs.ErrNotExist
	}
	if _, err := handle.Seek(0, io.SeekStart); err != nil {
		_ = handle.Close()
		return nil, Asset{}, fs.ErrNotExist
	}
	return handle, asset, nil
}

// Keys lists the source keys that have at least one clip, in stable order. It
// exists for tests and diagnostics, not for a request path.
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

// Entries lists every asset the manifest lists, in stable order. It exists for
// the author's own material list and for tests, not for a request path.
func (l *Library) Entries() []Asset {
	l.load()
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Asset, len(l.entries))
	copy(out, l.entries)
	return out
}

// Reload re-reads the manifest. Importing writes a file and a manifest row while
// the game is running, so the read that follows it must see both without a
// restart.
func (l *Library) Reload() {
	l.mu.Lock()
	l.loaded = false
	l.loadLocked()
	l.mu.Unlock()
}

// ImportRequest is one file the player or the author added, and what it is for.
type ImportRequest struct {
	World     string
	Subject   string
	SourceKey string
	Scope     string
	Actor     string
	Place     string
	Purpose   string
	Scene     string
	Role      string
	// Filename is the name the file arrived under. Only its extension is used,
	// and only after the bytes have been decoded, so it cannot choose the type.
	Filename string
	Data     []byte
}

// Import copies one file into the managed directory, binds it to a subject and
// rewrites the manifest. The file is named after its content, so importing the
// same picture twice stores one file, and replacing a picture produces a new
// name — which is why a replaced picture cannot keep being served from a cache.
//
// A previous entry for the same subject and purpose is dropped. That is what
// makes "replace this person's picture" one operation rather than two.
func (l *Library) Import(request ImportRequest) (Asset, error) {
	if l.dir == "" {
		return Asset{}, ErrNoMediaDir
	}
	if len(request.Data) == 0 {
		return Asset{}, ErrAssetEmpty
	}
	if len(request.Data) > MaxAssetBytes {
		return Asset{}, ErrAssetTooLarge
	}
	asset := Asset{
		World:     normalizeWorld(request.World),
		Subject:   strings.TrimSpace(request.Subject),
		SourceKey: strings.TrimSpace(request.SourceKey),
		Scope:     strings.TrimSpace(request.Scope),
		Actor:     strings.TrimSpace(request.Actor),
		Place:     normalizePlace(request.Place),
		Purpose:   strings.TrimSpace(request.Purpose),
		Scene:     strings.TrimSpace(request.Scene),
		Role:      strings.TrimSpace(request.Role),
		// The player picked this file for this subject themselves, which is the
		// review the flag stands for. A file nobody chose for a person stays
		// unreviewed until someone says they looked at it.
		Reviewed: true,
	}
	if asset.Subject == "" {
		asset.Subject = SubjectCharacter
	}
	if asset.Purpose == "" {
		asset.Purpose = PurposeAvatar
	}
	if asset.Role == "" {
		asset.Role = RoleGeneral
	} else if asset.Role != RoleGeneral && asset.Role != RoleIntimate {
		return Asset{}, ErrAssetInvalid
	}
	ext, width, height, err := inspect(request.Data)
	if err != nil {
		return Asset{}, err
	}
	if asset.Purpose == PurposeClip {
		if !isClipExtension(ext) {
			return Asset{}, ErrAssetInvalid
		}
	} else if !isImageExtension(ext) {
		return Asset{}, ErrAssetInvalid
	}
	digest := sha256.Sum256(request.Data)
	asset.SHA256 = hex.EncodeToString(digest[:])
	asset.File = importedFileName(asset.SHA256, ext)
	asset.Bytes = int64(len(request.Data))
	asset.Width, asset.Height = width, height
	if asset.Purpose == PurposeClip {
		// A clip's own shape is not read here: the player's browser is the only
		// decoder this build has, and the window lays the element out from the
		// file itself.
		asset.Width, asset.Height = 0, 0
	}
	if _, ok := normalizeItem(catalogItem{
		World: asset.World, Subject: asset.Subject, SourceKey: asset.SourceKey,
		Scope: asset.Scope, Actor: asset.Actor, Place: asset.Place,
		Purpose: asset.Purpose, Scene: asset.Scene, Role: asset.Role, File: asset.File,
	}); !ok {
		return Asset{}, ErrAssetInvalid
	}
	asset.ID = assetID(asset)

	l.writeMu.Lock()
	defer l.writeMu.Unlock()

	// Match the global read boundary before creating files or changing bindings.
	// SHA256 above came from the actual upload, never from client metadata.
	if l.storyFileRestriction(asset.File, asset.SHA256) == storyRestrictionDenied {
		return Asset{}, ErrAssetStoryRestricted
	}

	target := filepath.Join(l.dir, asset.File)
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(l.dir, 0o755); err != nil {
			return Asset{}, fmt.Errorf("create the media directory: %w", err)
		}
		if err := os.WriteFile(target, request.Data, 0o644); err != nil {
			return Asset{}, fmt.Errorf("save the imported file: %w", err)
		}
	}
	replaced := ""
	if err := l.rewrite(func(entries []Asset) []Asset {
		kept := make([]Asset, 0, len(entries)+1)
		for _, existing := range entries {
			if existing.ID == asset.ID {
				replaced = existing.File
				continue
			}
			kept = append(kept, existing)
		}
		return append(kept, asset)
	}); err != nil {
		return Asset{}, err
	}
	l.Reload()
	l.dropUnreferenced(replaced)
	return asset, nil
}

// Remove drops one entry from the manifest. The file goes with it when nothing
// else lists it and it was imported rather than shipped, so removing an override
// returns the subject to the built-in picture and leaves no stray file behind.
func (l *Library) Remove(id string) (Asset, error) {
	if l.dir == "" {
		return Asset{}, ErrNoMediaDir
	}
	name := strings.TrimSpace(id)
	if name == "" {
		return Asset{}, ErrAssetUnknown
	}
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	var removed Asset
	found := false
	if err := l.rewrite(func(entries []Asset) []Asset {
		kept := make([]Asset, 0, len(entries))
		for _, existing := range entries {
			if existing.ID == name {
				removed, found = existing, true
				continue
			}
			kept = append(kept, existing)
		}
		return kept
	}); err != nil {
		return Asset{}, err
	}
	if !found {
		return Asset{}, ErrAssetUnknown
	}
	l.Reload()
	l.dropUnreferenced(removed.File)
	return removed, nil
}

// rewrite applies change to the current entries and writes the document back.
// The caller holds writeMu.
func (l *Library) rewrite(change func([]Asset) []Asset) error {
	document, err := os.ReadFile(filepath.Join(l.dir, ManifestFile))
	note := l.note
	var parsed catalog
	switch {
	case err == nil:
		if jsonErr := json.Unmarshal(document, &parsed); jsonErr != nil {
			return fmt.Errorf("parse the media manifest: %w", jsonErr)
		}
		note = strings.TrimSpace(parsed.Note)
	case errors.Is(err, fs.ErrNotExist):
	default:
		return fmt.Errorf("read the media manifest: %w", err)
	}
	entries := make([]Asset, 0, len(parsed.Entries)+1)
	for _, item := range parsed.Entries {
		if asset, ok := normalizeItem(item); ok {
			entries = append(entries, asset)
		}
	}
	entries = change(entries)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	next := catalog{Schema: ManifestSchema, Note: note, Entries: make([]catalogItem, 0, len(entries))}
	for _, asset := range entries {
		next.Entries = append(next.Entries, itemOf(asset))
	}
	encoded, err := json.MarshalIndent(next, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("create the media directory: %w", err)
	}
	temp := filepath.Join(l.dir, ManifestFile+".tmp")
	if err := os.WriteFile(temp, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write the media manifest: %w", err)
	}
	if err := os.Rename(temp, filepath.Join(l.dir, ManifestFile)); err != nil {
		return fmt.Errorf("replace the media manifest: %w", err)
	}
	l.mu.Lock()
	l.note = note
	l.mu.Unlock()
	return nil
}

func itemOf(asset Asset) catalogItem {
	return catalogItem{
		ID: asset.ID, World: asset.World, Subject: asset.Subject,
		SourceKey: asset.SourceKey, Scope: asset.Scope, Actor: asset.Actor,
		Place: asset.Place, Purpose: asset.Purpose, Scene: asset.Scene,
		Reviewed:         asset.Reviewed,
		PlaybackApproval: asset.PlaybackApproval,
		Role:             asset.Role, File: asset.File, SHA256: asset.SHA256, Bytes: asset.Bytes,
		DurationMS: asset.DurationMS, Width: asset.Width, Height: asset.Height,
	}
}

// dropUnreferenced deletes an imported file nothing else lists. A shipped file
// is never removed: the folder is the author's delivery, and a game that deleted
// part of it would be destroying material it did not create.
//
// The caller reloads first. The question this asks is "does the manifest I just
// wrote still name this file", so reading the listing from before the rewrite
// would answer about the wrong document.
func (l *Library) dropUnreferenced(file string) {
	name := strings.TrimSpace(file)
	if !strings.HasPrefix(name, importedPrefix) || !validToken(name) {
		return
	}
	l.mu.RLock()
	stillListed := false
	for _, asset := range l.entries {
		if asset.File == name {
			stillListed = true
			break
		}
	}
	l.mu.RUnlock()
	if stillListed {
		return
	}
	_ = os.Remove(filepath.Join(l.dir, name))
}

// importedPrefix marks a file this game wrote. It keeps the author's delivery
// distinguishable from the player's own material in the same folder.
const importedPrefix = "u-"

func importedFileName(digest, ext string) string {
	if len(digest) > 16 {
		digest = digest[:16]
	}
	return importedPrefix + digest + ext
}

// inspect reads the shape of a picture and its extension. A clip is not decoded
// here — this build has no decoder for it — so its extension is all that is
// known, and it is accepted only when it names a container browsers play.
func inspect(data []byte) (string, int, int, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		ext := clipExtension(data)
		if ext == "" {
			return "", 0, 0, ErrAssetInvalid
		}
		return ext, 0, 0, nil
	}
	ext := imageExtension(format)
	if ext == "" {
		return "", 0, 0, ErrAssetInvalid
	}
	return ext, config.Width, config.Height, nil
}

func imageExtension(format string) string {
	switch strings.ToLower(format) {
	case "png":
		return ".png"
	case "jpeg":
		return ".jpg"
	case "webp":
		return ".webp"
	case "gif":
		return ".gif"
	default:
		return ""
	}
}

// clipExtension recognises the containers a browser plays, by their own leading
// bytes rather than by a name the caller supplied.
func clipExtension(data []byte) string {
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return ".mp4"
	}
	if len(data) >= 4 && data[0] == 0x1a && data[1] == 0x45 && data[2] == 0xdf && data[3] == 0xa3 {
		return ".webm"
	}
	return ""
}

func isImageExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func isClipExtension(ext string) bool {
	switch ext {
	case ".mp4", ".webm":
		return true
	default:
		return false
	}
}

type storyRestrictionState uint8

const (
	storyRestrictionAbsent storyRestrictionState = iota
	storyRestrictionActive
	storyRestrictionDenied
)

// storyFileRestriction fixes the first installed restriction generation for this
// process. Removing an entry is a revocation, not permission to expose its old
// pixels through a legacy alias. Read once so the bytes used for the generation
// check and the restriction decision cannot come from different manifests.
// Absent means this process has never observed an installed catalogue and it
// is still missing. Only that state permits the original no-scan stream path.
func (l *Library) storyFileRestriction(file string, hashes ...string) storyRestrictionState {
	if l.dir == "" {
		return storyRestrictionAbsent
	}
	raw, err := os.ReadFile(filepath.Join(l.dir, "story-assets.json"))
	sum := sha256.Sum256(raw)
	l.mu.Lock()
	if !l.storyRestrictionsSeen {
		if os.IsNotExist(err) {
			l.mu.Unlock()
			return storyRestrictionAbsent
		}
		l.storyRestrictionsSeen = true
		if err == nil {
			l.storyRestrictionSHA = sum
		}
	}
	unavailable := err != nil || sum != l.storyRestrictionSHA
	l.mu.Unlock()
	if unavailable || storyRestrictedContent(raw, file, hashes...) {
		return storyRestrictionDenied
	}
	return storyRestrictionActive
}
