package storypack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"ta/internal/cultivation"
	"ta/internal/revisionfile"
)

// WorldEntryDraft is one entry to file into an existing world.
//
// It is the world book's own shape rather than a lore item's, because the world
// book is where a collected entry has to end up: a second store would be a
// second copy of the same fact. The ID is the pack's stable entry id, so a
// collection that names an entry already there updates it and a collection that
// names a new one appends.
type WorldEntryDraft struct {
	Audience    string                      `json:"audience,omitempty"`
	CraftMethod *cultivation.CraftMethodDef `json:"craft_method,omitempty"`
	// ID is the pack-internal entry id. Empty asks the library to derive one.
	ID               string   `json:"id,omitempty"`
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	Importance       string   `json:"importance,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	LoadMode         string   `json:"load_mode,omitempty"`
	BriefDescription string   `json:"brief_description,omitempty"`
	Content          string   `json:"content"`
	// SourceRef is the story entity this entry came from, as
	// "<story id>/<entity id>". Two drafts naming the same source are the same
	// entry even when their names differ, so a retried collection cannot leave
	// a duplicate behind under a slightly different title.
	SourceRef string `json:"source_ref,omitempty"`
}

// WorldEntryCollectResult reports what a collection wrote.
type WorldEntryCollectResult struct {
	CraftMethod *cultivation.CraftMethodDef `json:"craft_method,omitempty"`
	PackID      string                      `json:"pack_id"`
	// EntryID is the pack-internal id of the entry that was written.
	EntryID string `json:"entry_id"`
	// Created says whether the entry is new or replaced an existing one.
	Created bool `json:"created"`
	// ContentVersion is the pack's new content revision. It moved because the
	// world book changed, which is what gives the next new game a new lore
	// scope while every story already running keeps reading the old one.
	ContentVersion int `json:"content_version"`
	// Entry is the entry as it now stands, so the caller can read back what it
	// wrote without a second round trip.
	Entry WorldEntry `json:"entry"`
}

// ErrBuiltinPackReadOnly reports an attempt to collect into a world that ships
// with the build. Those files are not on disk under the data root, so writing
// one would either fail or create a user file that shadows the shipped world for
// every story started from it — the overwrite a delivered asset must not take.
var ErrBuiltinPackReadOnly = errors.New("内置世界包不可直接写入")

// IsBuiltinPackReadOnly reports whether a collection failed because the world
// ships with the build.
func IsBuiltinPackReadOnly(err error) bool {
	return errors.Is(err, ErrBuiltinPackReadOnly)
}

// WorldBookSnapshot is one world's book as a collecting assistant reads it.
//
// Writable is resolved here rather than left to a failed write: the assistant
// needs to know before it proposes a collection whether this world can take one,
// and a builtin's file is not on disk at all.
type WorldBookSnapshot struct {
	CraftMethods []cultivation.CraftMethodDef `json:"craft_methods,omitempty"`
	PackID       string                       `json:"pack_id"`
	Writable     bool                         `json:"writable"`
	Entries      []WorldEntry                 `json:"entries"`
}

// WorldBook reads one world's authored entries without requiring the modules it
// binds to be installed. A collection only needs the book, so an uninstalled
// director module must not make the assistant unable to read what is there.
func (l *Library) WorldBook(packID string) (WorldBookSnapshot, error) {
	packID = strings.TrimSpace(packID)
	if err := validatePackID(packID); err != nil {
		return WorldBookSnapshot{}, err
	}
	path := filepath.Join(l.dir(), packID+fileExtension)
	pack, err := l.GetContent(packID)
	if err != nil {
		return WorldBookSnapshot{}, err
	}
	_, statErr := os.Stat(path)
	return WorldBookSnapshot{CraftMethods: pack.Story.Gameplay.CraftMethods, PackID: packID, Writable: statErr == nil, Entries: pack.World}, nil
}

// CollectWorldEntry files one entry into an existing user world.
//
// This is the one write the lore library's 本局新建 page needs: a story's own
// entity becomes part of the world it was played in, through the same pack file
// the world page edits. It is a read-modify-write under the pack file's
// compare-and-swap, so a world edited in another window between the read and the
// write is reported instead of overwritten.
//
// A builtin world is refused rather than shadowed. The caller turns that into
// "save this world as your own copy first", which keeps the delivered asset
// intact and keeps every story of the builtin reading the world it started on.
//
// Nothing outside the world book is touched: the catalog, the templates, the
// settings and the module bindings are the author's, and collecting a fact from
// a story says nothing about who is discoverable, what a new game starts with,
// or which rules it runs.
func (l *Library) CollectWorldEntry(ctx context.Context, packID string, draft WorldEntryDraft) (WorldEntryCollectResult, error) {
	packID = strings.TrimSpace(packID)
	if err := validatePackID(packID); err != nil {
		return WorldEntryCollectResult{}, err
	}
	draft.Name = strings.TrimSpace(draft.Name)
	if draft.Name == "" {
		return WorldEntryCollectResult{}, fmt.Errorf("收录的世界条目缺少 name")
	}
	if strings.TrimSpace(draft.Content) == "" {
		return WorldEntryCollectResult{}, fmt.Errorf("收录的世界条目缺少正文: %s", draft.Name)
	}

	path := filepath.Join(l.dir(), packID+fileExtension)
	if _, err := readPackFile(path); err != nil {
		if os.IsNotExist(err) {
			// No user file: the id either names a builtin or nothing at all.
			for _, builtin := range builtinPacks() {
				if builtin.ID == packID {
					return WorldEntryCollectResult{}, fmt.Errorf("%w: %s", ErrBuiltinPackReadOnly, packID)
				}
			}
			return WorldEntryCollectResult{}, fmt.Errorf("剧本包不存在: %s", packID)
		}
		return WorldEntryCollectResult{}, err
	}

	var collected WorldEntryCollectResult
	var previousVersion int
	result, err := revisionfile.Mutate(ctx, path, revisionfile.Options{FileMode: 0o644, DirectoryMode: 0o755}, func(current revisionfile.Snapshot) ([]byte, error) {
		collected = WorldEntryCollectResult{}
		if !current.Exists {
			return nil, fmt.Errorf("剧本包文件在写入前消失: %s", packID)
		}
		var pack Pack
		if err := json.Unmarshal(current.Content, &pack); err != nil {
			return nil, fmt.Errorf("解析剧本包失败 path=%s: %w", path, err)
		}
		previousVersion = pack.ContentVersion
		methodChanged, err := upsertPackCraftMethod(&pack, draft.CraftMethod)
		if err != nil {
			return nil, err
		}
		entry, created, err := upsertWorldEntry(pack.World, draft)
		if err != nil {
			return nil, err
		}
		if created {
			pack.World = append(pack.World, entry)
		} else {
			for index := range pack.World {
				if strings.TrimSpace(pack.World[index].ID) != strings.TrimSpace(entry.ID) {
					continue
				}
				// The entry already reads exactly like this. Bumping the revision
				// here would move every new game of this world to a fresh lore
				// scope while the book itself changed by nothing, so the file is
				// handed back untouched and Mutate reports no change.
				if !methodChanged && reflect.DeepEqual(pack.World[index], entry) {
					collected = WorldEntryCollectResult{
						PackID: packID, EntryID: entry.ID,
						ContentVersion: previousVersion, Entry: entry, CraftMethod: draft.CraftMethod,
					}
					return current.Content, nil
				}
				pack.World[index] = entry
				break
			}
		}
		// The content revision is the author's own counter and moves with the
		// world book, so a reader can tell one revision from the next without
		// hashing anything.
		pack.ContentVersion++
		if err := pack.Validate(); err != nil {
			return nil, err
		}
		if err := pack.ValidateModules(l.dataRoot); err != nil {
			return nil, err
		}
		next, err := json.MarshalIndent(pack, "", "  ")
		if err != nil {
			return nil, err
		}
		next = append(next, '\n')
		collected = WorldEntryCollectResult{
			PackID: packID, EntryID: entry.ID, Created: created,
			ContentVersion: pack.ContentVersion, Entry: entry, CraftMethod: draft.CraftMethod,
		}
		return next, nil
	})
	if err != nil {
		return WorldEntryCollectResult{}, err
	}
	if !result.Changed {
		// The entry was already exactly this. Reporting the state is the honest
		// answer; bumping the revision for a no-op would move every new game of
		// this world to a fresh lore scope for nothing. Mutate wrote nothing, so
		// the counter the mutator bumped never reached disk and must not be
		// reported as if it had.
		collected.PackID = packID
		collected.ContentVersion = previousVersion
	}
	return collected, nil
}

// upsertWorldEntry finds the entry a draft addresses and returns it written.
//
// Identity is the source reference first and the entry id second. A source
// reference is what makes a retried collection idempotent even after the model
// reworded the title; the id is what lets an author edit the entry by hand and
// have a later collection update it in place.
func upsertWorldEntry(entries []WorldEntry, draft WorldEntryDraft) (WorldEntry, bool, error) {
	sourceRef := strings.TrimSpace(draft.SourceRef)
	entryID := strings.TrimSpace(draft.ID)

	for index := range entries {
		existing := entries[index]
		if sourceRef != "" && strings.TrimSpace(existing.SourceRef) == sourceRef {
			merged := mergeWorldEntry(existing, draft)
			merged.ID = strings.TrimSpace(existing.ID)
			return merged, false, nil
		}
		if entryID != "" && strings.TrimSpace(existing.ID) == entryID {
			merged := mergeWorldEntry(existing, draft)
			merged.ID = strings.TrimSpace(existing.ID)
			return merged, false, nil
		}
	}

	// A new entry. The name still has to be unique inside the book, because two
	// entries with one name make every later lookup ambiguous — and a same-name
	// entry that came from a different story is a different person, so it is
	// reported rather than silently renamed into the same book.
	for _, existing := range entries {
		if strings.EqualFold(strings.TrimSpace(existing.Name), draft.Name) {
			return WorldEntry{}, false, fmt.Errorf("世界书已有同名条目: %s（id=%s）。同名不同人是两个人，请用不同的名字，或指名要更新的条目 id", draft.Name, existing.ID)
		}
	}
	created := WorldEntry{
		Audience:         strings.TrimSpace(draft.Audience),
		ID:               entryID,
		Name:             draft.Name,
		Type:             strings.TrimSpace(draft.Type),
		Importance:       strings.TrimSpace(draft.Importance),
		Tags:             clipStrings(draft.Tags, 24, 40),
		Keywords:         clipStrings(draft.Keywords, 24, 40),
		LoadMode:         strings.TrimSpace(draft.LoadMode),
		BriefDescription: strings.TrimSpace(draft.BriefDescription),
		Content:          strings.TrimSpace(draft.Content),
		SourceRef:        sourceRef,
	}
	if created.ID == "" {
		created.ID = uniqueWorldEntryID(entries, created.Name)
	}
	return created, true, nil
}

// mergeWorldEntry applies a draft on top of an existing entry. Fields the draft
// leaves empty keep their current value, so a collection that only rewords the
// body does not drop the entry's tags or its load mode.
func mergeWorldEntry(existing WorldEntry, draft WorldEntryDraft) WorldEntry {
	merged := existing
	if value := strings.TrimSpace(draft.Audience); value != "" {
		merged.Audience = value
	}
	if name := strings.TrimSpace(draft.Name); name != "" {
		merged.Name = name
	}
	if value := strings.TrimSpace(draft.Type); value != "" {
		merged.Type = value
	}
	if value := strings.TrimSpace(draft.Importance); value != "" {
		merged.Importance = value
	}
	if tags := clipStrings(draft.Tags, 24, 40); len(tags) > 0 {
		merged.Tags = tags
	}
	if keywords := clipStrings(draft.Keywords, 24, 40); len(keywords) > 0 {
		merged.Keywords = keywords
	}
	if value := strings.TrimSpace(draft.LoadMode); value != "" {
		merged.LoadMode = value
	}
	if value := strings.TrimSpace(draft.BriefDescription); value != "" {
		merged.BriefDescription = value
	}
	if value := strings.TrimSpace(draft.Content); value != "" {
		merged.Content = value
	}
	if value := strings.TrimSpace(draft.SourceRef); value != "" {
		merged.SourceRef = value
	}
	return merged
}

// uniqueWorldEntryID derives a readable id that the book does not already use.
// It keeps the name's own letters and CJK so an author reading the file can tell
// which entry is which; the numeric suffix is what makes a second entry with the
// same derived id a separate entry rather than an overwrite.
func uniqueWorldEntryID(entries []WorldEntry, name string) string {
	var sb strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '-', r == '_':
			sb.WriteRune(r)
		case r >= 0x4e00 && r <= 0x9fff:
			sb.WriteRune(r)
		}
		if sb.Len() >= 32 {
			break
		}
	}
	base := strings.Trim(sb.String(), "-_")
	if base == "" {
		base = "entry"
	}
	taken := make(map[string]bool, len(entries))
	for _, entry := range entries {
		taken[strings.TrimSpace(entry.ID)] = true
	}
	if !taken[base] {
		return base
	}
	for suffix := 2; suffix < 1000; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !taken[candidate] {
			return candidate
		}
	}
	return base
}
