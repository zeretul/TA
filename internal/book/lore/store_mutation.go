package lore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// NewStore returns an unbound store: it reads and writes every scope. That is
// what the lore editor and migrations need, so the user can review and repair
// all worlds' entries from one screen.
func NewStore(workspace string) *Store {
	return &Store{workspace: workspace, scope: scopeUnbound, mutationMu: sharedLoreMutationLock(workspace)}
}

// NewScopedStore binds the store to one world resource scope. Reads then return
// shared entries plus that scope, so two worlds sharing a workspace cannot see
// each other's setting book. Writes through a scoped store are stamped with the
// scope, which is why a pack's entries never contend with another pack's.
//
// An empty scope is meaningful: it is the shared scope, so a story that belongs
// to no pack sees only user-authored entries and no pack's world book.
func NewScopedStore(workspace, scope string) *Store {
	return &Store{
		workspace:  workspace,
		scope:      normalizeLoreScope(scope),
		mutationMu: sharedLoreMutationLock(workspace),
	}
}

// Scope reports the world resource scope this store is bound to, or
// ScopeShared when it is unbound or bound to the shared scope.
func (s *Store) Scope() string {
	if s == nil || s.scope == scopeUnbound {
		return ScopeShared
	}
	return s.scope
}

// Bound reports whether this store is restricted to one world's scope. An
// unbound store is the editor; a bound store is a story's model context.
func (s *Store) Bound() bool {
	return s != nil && s.scope != scopeUnbound
}

func sharedLoreMutationLock(workspace string) *sync.Mutex {
	path := ItemsPath(workspace)
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	lock, _ := loreMutationLocks.LoadOrStore(filepath.Clean(path), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *Store) List() ([]Item, error) {
	return s.list(false)
}

func (s *Store) ListAll() ([]Item, error) {
	return s.list(true)
}

// Get resolves one lore item by stable ID, including disabled items. Review
// comments and editor tabs bind to identity, so visibility must not make an
// existing item appear deleted.
//
// Scope still applies: a story bound to one world must not be able to reach
// another world's entry by guessing its ID, which would hand the model exactly
// the lore the scoped list exists to withhold. The unbound editor keeps seeing
// every scope, so identity lookup stays available where the user needs it.
func (s *Store) Get(id string) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, fmt.Errorf("资料 ID 不能为空: %w", os.ErrNotExist)
	}
	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	for _, item := range collection.Items {
		if item.ID != id {
			continue
		}
		if !scopeAllows(s.scope, item.Scope) || (s.visible != nil && !s.visible(item)) || s.shadowedByOwnScope(collection.Items, item) {
			break
		}
		return item, nil
	}
	return Item{}, fmt.Errorf("资料不存在: %s: %w", id, os.ErrNotExist)
}

// Revision identifies the current enabled Lore catalog for model-context
// invalidation. It changes when a name, summary, body, or enabled state
// changes without exposing the full collection to metadata consumers.
func (s *Store) Revision() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12]), nil
}

func (s *Store) list(includeDisabled bool) ([]Item, error) {
	collection, err := s.loadOrCreate()
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(collection.Items))
	for _, item := range collection.Items {
		if !includeDisabled && !item.Enabled {
			continue
		}
		if !scopeAllows(s.scope, item.Scope) || (s.visible != nil && !s.visible(item)) {
			continue
		}
		items = append(items, item)
	}
	items = s.shadowSharedNames(items)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Enabled != items[j].Enabled {
			return items[i].Enabled
		}
		if loreImportanceRank(items[i].Importance) != loreImportanceRank(items[j].Importance) {
			return loreImportanceRank(items[i].Importance) < loreImportanceRank(items[j].Importance)
		}
		if items[i].Type != items[j].Type {
			return items[i].Type < items[j].Type
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// shadowSharedNames hides a shared entry whose display name a world entry
// already claims. Both are readable in principle, but a name resolves to one
// fact: leaving both visible would let the shared entry win by storage order
// and hand the model the wrong world's version of a place or a person. The
// shared entry itself is untouched and reappears for every reader that is not
// bound to the shadowing world.
func (s *Store) shadowSharedNames(items []Item) []Item {
	if !s.Bound() || s.scope == ScopeShared {
		return items
	}
	owned := s.ownScopedNameKeys(items)
	if len(owned) == 0 {
		return items
	}
	kept := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Scope == ScopeShared && owned[loreNameKey(item.Name)] {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// shadowedByOwnScope reports whether one shared entry loses to a same-named
// entry of the reader's own world. Get resolves by identity rather than through
// the list, so it needs the same rule spelled out rather than inherited.
func (s *Store) shadowedByOwnScope(items []Item, item Item) bool {
	if !s.Bound() || s.scope == ScopeShared || item.Scope != ScopeShared {
		return false
	}
	return s.ownScopedNameKeys(items)[loreNameKey(item.Name)]
}

func (s *Store) ownScopedNameKeys(items []Item) map[string]bool {
	owned := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Scope == s.scope {
			owned[loreNameKey(item.Name)] = true
		}
	}
	return owned
}

// itemScopeForWrite decides which scope a new item lands in. A bound store
// always writes into its own scope so a world's writer cannot leak entries into
// another world or into the shared book; an unbound store honours the caller's
// scope, which is how a pack seeds its own world book.
func (s *Store) itemScopeForWrite(requested string) string {
	if s.Bound() {
		return s.scope
	}
	return normalizeLoreScope(requested)
}

func (s *Store) Create(input ItemInput) (Item, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := normalizeLoreItem(Item{
		Audience:         input.Audience,
		ID:               input.ID,
		Scope:            s.itemScopeForWrite(input.Scope),
		Enabled:          loreInputEnabled(input.Enabled, true),
		Type:             input.Type,
		TypeSource:       firstNonEmptyLoreValue(input.TypeSource, TypeSourceManual),
		Name:             input.Name,
		Importance:       input.Importance,
		Tags:             input.Tags,
		BriefDescription: input.BriefDescription,
		Keywords:         input.Keywords,
		// A new entry is read on demand unless the author says otherwise.
		// normalizeLoreLoadMode derives resident from importance: major, which
		// would spend a context slot every turn on an entry nobody has filled
		// in yet.
		LoadMode:   firstNonEmptyLoreValue(input.LoadMode, LoadModeAuto),
		Content:    input.Content,
		CreatedAt:  now,
		UpdatedAt:  now,
		Image:      input.Image,
		Provenance: input.Provenance,
	})
	if item.ID == "" {
		item.ID = newUniqueLoreID(collection.Items, item.Name, item.Type)
	}
	if item.Name == "" {
		return Item{}, errors.New("资料名称不能为空")
	}
	if err := validateLoreReferenceName(item.Name); err != nil {
		return Item{}, err
	}
	if loreItemNameIndex(collection.Items, item.Scope, item.Name, "") >= 0 {
		return Item{}, fmt.Errorf("资料名称已存在: %s", item.Name)
	}
	if s.hasItem(collection.Items, item.ID) {
		return Item{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
	}
	collection.Items = append(collection.Items, item)
	if err := s.save(collection); err != nil {
		return Item{}, err
	}
	return item, nil
}

// writableIndex finds an item a store may mutate. A bound store can only write
// items inside its own scope: shared entries and other worlds' entries are
// readable but not editable from a story's context, so a world's writer cannot
// silently rewrite the user's general book or a sibling world.
func (s *Store) writableIndex(items []Item, id string) int {
	index := loreItemIndex(items, id)
	if index < 0 {
		return -1
	}
	if !s.Bound() {
		return index
	}
	if !scopeAllows(s.scope, items[index].Scope) || items[index].Scope != s.scope {
		return -1
	}
	return index
}

func (s *Store) Update(id string, input ItemInput) (Item, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Item{}, errors.New("资料 ID 不能为空")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return Item{}, err
	}
	for i := range collection.Items {
		if collection.Items[i].ID != id {
			continue
		}
		if s.writableIndex(collection.Items, id) < 0 {
			return Item{}, fmt.Errorf("资料不在当前世界范围内，无法修改: %s", id)
		}
		if input.BaseRevision != "" && collection.Items[i].UpdatedAt != input.BaseRevision {
			return Item{}, ErrRevisionConflict
		}
		previous := collection.Items[i]
		typeSource := previous.TypeSource
		if NormalizeType(input.Type) != previous.Type {
			typeSource = TypeSourceManual
		}
		updated := normalizeLoreItem(Item{
			Audience:         firstNonEmptyLoreValue(input.Audience, previous.Audience),
			ID:               id,
			Scope:            collection.Items[i].Scope,
			Enabled:          loreInputEnabled(input.Enabled, collection.Items[i].Enabled),
			Type:             input.Type,
			TypeSource:       typeSource,
			Name:             input.Name,
			Importance:       input.Importance,
			Tags:             input.Tags,
			BriefDescription: input.BriefDescription,
			Keywords:         input.Keywords,
			LoadMode:         input.LoadMode,
			Content:          input.Content,
			CreatedAt:        collection.Items[i].CreatedAt,
			UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
			Image:            firstLoreImage(input.Image, collection.Items[i].Image),
			Provenance:       collection.Items[i].Provenance,
		})
		if updated.Name == "" {
			return Item{}, errors.New("资料名称不能为空")
		}
		if err := validateLoreReferenceName(updated.Name); err != nil {
			return Item{}, err
		}
		if loreItemNameIndex(collection.Items, updated.Scope, updated.Name, id) >= 0 {
			return Item{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
		}
		collection.Items[i] = updated
		if err := s.save(collection); err != nil {
			return Item{}, err
		}
		return updated, nil
	}
	return Item{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *Store) Delete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("资料 ID 不能为空")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return err
	}
	next := make([]Item, 0, len(collection.Items))
	found := false
	for _, item := range collection.Items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return fmt.Errorf("资料不存在: %s", id)
	}
	if s.writableIndex(collection.Items, id) < 0 {
		return fmt.Errorf("资料不在当前世界范围内，无法删除: %s", id)
	}
	collection.Items = next
	return s.save(collection)
}

func (s *Store) ApplyOperations(message string, ops []Operation) (ApplyResult, error) {
	if len(ops) == 0 {
		return ApplyResult{}, errors.New("没有可执行的资料库操作")
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	collection, err := s.loadOrCreate()
	if err != nil {
		return ApplyResult{}, err
	}

	next := append([]Item(nil), collection.Items...)
	result := ApplyResult{Message: strings.TrimSpace(message)}
	for _, op := range ops {
		switch strings.TrimSpace(op.Op) {
		case "create":
			now := time.Now().UTC().Format(time.RFC3339Nano)
			item := normalizeLoreItem(Item{
				Audience:         op.Item.Audience,
				ID:               op.Item.ID,
				Scope:            s.itemScopeForWrite(op.Item.Scope),
				Enabled:          loreInputEnabled(op.Item.Enabled, true),
				Type:             op.Item.Type,
				TypeSource:       firstNonEmptyLoreValue(op.Item.TypeSource, TypeSourceManual),
				Name:             op.Item.Name,
				Importance:       op.Item.Importance,
				Tags:             op.Item.Tags,
				BriefDescription: op.Item.BriefDescription,
				Keywords:         op.Item.Keywords,
				// New entries start on demand; see Create.
				LoadMode:   firstNonEmptyLoreValue(op.Item.LoadMode, LoadModeAuto),
				Content:    op.Item.Content,
				CreatedAt:  now,
				UpdatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
				Image:      op.Item.Image,
				Provenance: op.Item.Provenance,
			})
			if item.Name == "" {
				return ApplyResult{}, errors.New("创建资料时名称不能为空")
			}
			if err := validateLoreReferenceName(item.Name); err != nil {
				return ApplyResult{}, err
			}
			if loreItemNameIndex(next, item.Scope, item.Name, "") >= 0 {
				return ApplyResult{}, fmt.Errorf("资料名称已存在: %s", item.Name)
			}
			if item.ID == "" {
				item.ID = newUniqueLoreID(next, item.Name, item.Type)
			}
			if loreItemIndex(next, item.ID) >= 0 {
				return ApplyResult{}, fmt.Errorf("资料 ID 已存在: %s", item.ID)
			}
			next = append(next, item)
			result.Created = append(result.Created, item)
		case "update":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return ApplyResult{}, errors.New("更新资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return ApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			if s.writableIndex(next, id) < 0 {
				return ApplyResult{}, fmt.Errorf("资料不在当前世界范围内，无法修改: %s", id)
			}
			typeName := firstNonEmptyLoreValue(op.Item.Type, next[idx].Type)
			typeSource := next[idx].TypeSource
			if NormalizeType(typeName) != next[idx].Type {
				typeSource = TypeSourceManual
			}
			updated := normalizeLoreItem(Item{
				Audience:         firstNonEmptyLoreValue(op.Item.Audience, next[idx].Audience),
				ID:               id,
				Scope:            next[idx].Scope,
				Enabled:          loreInputEnabled(op.Item.Enabled, next[idx].Enabled),
				Type:             typeName,
				TypeSource:       typeSource,
				Name:             firstNonEmptyLoreValue(op.Item.Name, next[idx].Name),
				Importance:       firstNonEmptyLoreValue(op.Item.Importance, next[idx].Importance),
				Tags:             op.Item.Tags,
				BriefDescription: firstNonEmptyLoreValue(op.Item.BriefDescription, next[idx].BriefDescription),
				Keywords:         op.Item.Keywords,
				LoadMode:         firstNonEmptyLoreValue(op.Item.LoadMode, next[idx].LoadMode),
				Content:          firstNonEmptyLoreValue(op.Item.Content, next[idx].Content),
				CreatedAt:        next[idx].CreatedAt,
				UpdatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
				Image:            firstLoreImage(op.Item.Image, next[idx].Image),
				Provenance:       next[idx].Provenance,
			})
			if op.Item.Tags == nil {
				updated.Tags = append([]string(nil), next[idx].Tags...)
			}
			if op.Item.Keywords == nil {
				updated.Keywords = append([]string(nil), next[idx].Keywords...)
			}
			if updated.Name == "" {
				return ApplyResult{}, fmt.Errorf("资料名称不能为空: %s", id)
			}
			if err := validateLoreReferenceName(updated.Name); err != nil {
				return ApplyResult{}, err
			}
			if loreItemNameIndex(next, updated.Scope, updated.Name, id) >= 0 {
				return ApplyResult{}, fmt.Errorf("资料名称已存在: %s", updated.Name)
			}
			next[idx] = updated
			result.Updated = append(result.Updated, updated)
		case "delete":
			id := normalizeLoreID(firstNonEmptyLoreValue(op.ID, op.Item.ID))
			if id == "" {
				return ApplyResult{}, errors.New("删除资料时 ID 不能为空")
			}
			idx := loreItemIndex(next, id)
			if idx < 0 {
				return ApplyResult{}, fmt.Errorf("资料不存在: %s", id)
			}
			if s.writableIndex(next, id) < 0 {
				return ApplyResult{}, fmt.Errorf("资料不在当前世界范围内，无法删除: %s", id)
			}
			next = append(next[:idx], next[idx+1:]...)
			result.DeletedIDs = append(result.DeletedIDs, id)
		default:
			return ApplyResult{}, fmt.Errorf("不支持的资料库操作: %s", op.Op)
		}
	}
	collection.Items = next
	if err := s.save(collection); err != nil {
		return ApplyResult{}, err
	}
	result.Items, err = s.List()
	if err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

// WithVisibility narrows all read paths; the unfiltered editor remains unchanged.
func (s *Store) WithVisibility(visible func(Item) bool) *Store {
	copy := *s
	copy.visible = visible
	return &copy
}
