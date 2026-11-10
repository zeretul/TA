package lore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Store) Read(id string) (Item, error) {
	id = normalizeLoreID(id)
	if id == "" {
		return Item{}, errors.New("资料 ID 不能为空")
	}
	items, err := s.List()
	if err != nil {
		return Item{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return Item{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *Store) ReadAny(id string) (Item, error) {
	id = normalizeLoreID(id)
	if id == "" {
		return Item{}, errors.New("资料 ID 不能为空")
	}
	items, err := s.ListAll()
	if err != nil {
		return Item{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return Item{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *Store) SetImage(id string, image *Image) (Item, error) {
	id = normalizeLoreID(id)
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
		collection.Items[i].Image = normalizeLoreItemImage(image)
		collection.Items[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		collection.Items[i] = normalizeLoreItem(collection.Items[i])
		if err := s.save(collection); err != nil {
			return Item{}, err
		}
		return collection.Items[i], nil
	}
	return Item{}, fmt.Errorf("资料不存在: %s", id)
}

func (s *Store) ReadMany(ids []string) (ReadResult, error) {
	if len(ids) == 0 {
		return ReadResult{}, errors.New("资料 ID 列表不能为空")
	}
	wanted := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = normalizeLoreID(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	if len(wanted) == 0 {
		return ReadResult{}, errors.New("资料 ID 列表不能为空")
	}
	items, err := s.List()
	if err != nil {
		return ReadResult{}, err
	}
	byID := make(map[string]Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := ReadResult{Items: make([]Item, 0, len(wanted))}
	for _, id := range wanted {
		item, ok := byID[id]
		if !ok {
			result.Missing = append(result.Missing, id)
			continue
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// ReadManyNames resolves user-facing unique lore names in request order.
//
// Names are unique per scope, not per workspace, so a shared entry and a world
// entry may share a display name. A bound store answers with its own world's
// entry when one exists and falls back to the shared entry otherwise; an
// unbound store (the lore editor) matches on the plain name.
func (s *Store) ReadManyNames(names []string) (ReadResult, error) {
	if len(names) == 0 {
		return ReadResult{}, errors.New("资料名称列表不能为空")
	}
	items, err := s.List()
	if err != nil {
		return ReadResult{}, err
	}
	byScopedName := make(map[string]Item, len(items))
	byPlainName := make(map[string]Item, len(items))
	for _, item := range items {
		if key := loreScopedNameKey(item.Scope, item.Name); key != "" {
			if _, ok := byScopedName[key]; !ok {
				byScopedName[key] = item
			}
		}
		if key := loreNameKey(item.Name); key != "" {
			if _, ok := byPlainName[key]; !ok {
				byPlainName[key] = item
			}
		}
	}
	result := ReadResult{Items: make([]Item, 0, len(names))}
	seen := map[string]bool{}
	for _, name := range names {
		key := loreNameKey(name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		item, ok := s.lookupName(byScopedName, byPlainName, name)
		if !ok {
			result.Missing = append(result.Missing, strings.TrimSpace(name))
			continue
		}
		result.Items = append(result.Items, item)
	}
	if len(result.Items) == 0 && len(result.Missing) == 0 {
		return ReadResult{}, errors.New("资料名称列表不能为空")
	}
	return result, nil
}

// lookupName resolves one display name against a store's own scope first. The
// world's own entry is the more specific answer, so it wins over a shared entry
// of the same name.
func (s *Store) lookupName(byScopedName, byPlainName map[string]Item, name string) (Item, bool) {
	if !s.Bound() {
		item, ok := byPlainName[loreNameKey(name)]
		return item, ok
	}
	if item, ok := byScopedName[loreScopedNameKey(s.scope, name)]; ok {
		return item, true
	}
	item, ok := byScopedName[loreScopedNameKey(ScopeShared, name)]
	return item, ok
}

func (s *Store) Search(query, itemType string, limit int) ([]Item, error) {
	items, err := s.List()
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	itemType = normalizeOptionalLoreType(itemType)
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	result := make([]Item, 0, limit)
	for _, item := range items {
		if itemType != "" && item.Type != itemType {
			continue
		}
		if query != "" && !loreItemMatchesQuery(item, query) {
			continue
		}
		result = append(result, item)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *Store) SearchIndexMarkdown(options IndexOptions) (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	entries, matchedTotal, libraryTotal := filterLoreIndexEntries(items, options)
	if len(entries) == 0 && options.OmitTitle {
		return "", nil
	}
	return renderLoreIndexMarkdown(entries, matchedTotal, libraryTotal, options), nil
}

// ResidentIndexMarkdown returns a bounded discovery index containing only
// enabled resident lore. Bodies stay behind read_lore_items so specialized
// agents can review relevant rules without injecting the complete library.
func (s *Store) ResidentIndexMarkdown(maxBytes int) (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	entries := make([]loreIndexEntry, 0, len(items))
	for _, item := range items {
		if item.LoadMode == LoadModeResident {
			entries = append(entries, loreIndexEntry{Item: item})
		}
	}
	sortLoreIndexEntries(entries, false)
	return renderLoreIndexMarkdown(entries, len(entries), len(entries), IndexOptions{MaxBytes: maxBytes}), nil
}

// NameRosterMarkdown returns a compact, deterministic discovery roster.
// It intentionally excludes briefs and bodies so callers can expose many
// names without treating every lore item as active model context.
func (s *Store) NameRosterMarkdown(maxBytes int, excludeResident bool) (string, error) {
	return s.NameCatalogMarkdown(NameCatalogOptions{
		MaxBytes:        maxBytes,
		ExcludeResident: excludeResident,
		OmitWhenEmpty:   true,
	})
}

func (s *Store) ResidentContextMarkdown() (string, error) {
	items, err := s.List()
	if err != nil {
		return "", err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var sb strings.Builder
	for _, item := range items {
		if item.LoadMode != LoadModeResident {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		sb.WriteString(formatLoreItemMarkdown(item, true))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

// ResidentContentBytes returns the exact UTF-8 size of enabled resident lore
// bodies for UI guidance and model-context safety checks.
func (s *Store) ResidentContentBytes() (int, error) {
	items, err := s.List()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, item := range items {
		if item.LoadMode == LoadModeResident {
			total += len([]byte(strings.TrimSpace(item.Content)))
		}
	}
	return total, nil
}

func (s *Store) IndexMarkdown() (string, error) {
	return s.SearchIndexMarkdown(IndexOptions{ExcludeResident: true, OmitTitle: true})
}

func (s *Store) ProgressiveContextMarkdown() (string, error) {
	resident, err := s.ResidentContextMarkdown()
	if err != nil {
		return "", err
	}
	catalog, err := s.NameCatalogMarkdown(NameCatalogOptions{
		MaxBytes:        IndexDefaultMaxBytes,
		ExcludeResident: true,
		OmitWhenEmpty:   true,
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if resident != "" {
		sb.WriteString("## Resident Lore\n\n")
		sb.WriteString(resident)
		sb.WriteString("\n\n")
	}
	if catalog != "" {
		fmt.Fprintf(&sb, "## On-demand Lore Name Catalog (source: %s, max 64 KiB)\n\n", ItemsRelativePath)
		sb.WriteString(strings.TrimSpace(strings.TrimPrefix(catalog, "# Lore Name Catalog")))
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}
