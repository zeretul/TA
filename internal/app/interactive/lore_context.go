package interactiveapp

import (
	"fmt"
	"sort"
	"strings"

	"ta/internal/book/lore"
	"ta/internal/character"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

const ResolvedLoreContextMaxBytes = interactive.StoryContextMaxBytes

// The working set is a page, not the world book. A pack's world book is ~250 KB
// and grows with every authored person, so the turn selects the entries its
// action, its scene and its cast actually need.
const (
	// resolvedLoreMaxItems bounds the whole working set. Plan references are
	// counted first because they are the author's explicit instruction.
	resolvedLoreMaxItems = 12
	// resolvedLorePresentCastMax is how many established characters present in
	// the scene contribute their profile body (and its dialogue examples). It is
	// deliberately small: this is a voice reference, not the cast roster, which
	// the cast block already carries.
	resolvedLorePresentCastMax = 3
	// resolvedLoreTopicMaxItems bounds the people selected for matching what the
	// player asked about.
	resolvedLoreTopicMaxItems = 4
	// resolvedLoreIndexMaxItems bounds the semantic index pass that reaches
	// non-character entries and user-authored lore.
	resolvedLoreIndexMaxItems = 6
	// resolvedLoreNoteMaxBytes bounds the ledger note so per-turn logs stay
	// metadata rather than a second copy of the context.
	resolvedLoreNoteMaxBytes = 900
)

// loreSelection is one turn's lore working set together with the bounded record
// of how it was chosen.
//
// The record exists because "the model forgot" is undiagnosable without it: a
// reader of the run ledger can tell which entry was selected, why, and which
// entries were dropped for capacity. It carries ids, reasons and sizes only —
// never the entry body, which the ledger already hashes and measures.
type loreSelection struct {
	Context string
	Note    string
}

// loreSelectionInput is everything the selection reads. The catalog and the
// character projection are passed in rather than re-read so one turn's working
// set, cast block and discovery page all describe the same instant.
type loreSelectionInput struct {
	Visibility    func(lore.Item) bool
	Workspace     string
	Scope         string
	PlanMarkdown  string
	UserAction    string
	Location      string
	ContentHash   string
	Catalog       catalogSource
	Scene         character.Scene
	Characters    character.State
	HasCharacters bool
}

type selectedLoreItem struct {
	Item   lore.Item
	Reason string
}

// loreAdder takes one entry into the working set. referenced says whether this
// turn actually asked for the entry, which is the only thing that may take a
// manual entry: a present person and a matching topic are circumstances, not
// requests.
type loreAdder func(item lore.Item, reason string, referenced bool) bool

// buildInteractiveStoryLoreContext selects the lore this turn needs.
//
// Selection order is priority order, and each entry carries the reason it was
// taken so the model (and the ledger) can tell an explicit author reference from
// a topic match:
//
//  1. names referenced by the current branch plan;
//  2. entries the player named in this turn's action;
//  3. the profiles of established characters who are present in the scene, which
//     is where a character's own voice examples live;
//  4. people the world's own topic vocabulary matches to this action, as long as
//     they are reachable from here;
//  5. the lore index, queried with the same topic vocabulary, which is the path
//     that reaches non-character entries and user-authored lore.
//
// Resident lore is never selected here: it is already a stable-prefix fragment,
// and duplicating it would move it out of the cacheable prefix.
//
// A manual entry is selected only by (1) and (2). "指定时参考" means the entry
// body is read when someone actually asked for it, and a scene that happens to
// contain the person, or a request whose words touch the entry's topic, is not
// that ask: those are the two ways a manual entry used to be pulled in silently,
// which made the setting a no-op for anything the index could reach.
func buildInteractiveStoryLoreContext(in loreSelectionInput) (loreSelection, error) {
	items, err := lore.NewScopedStore(in.Workspace, in.Scope).WithVisibility(in.Visibility).List()
	if err != nil {
		return loreSelection{}, fmt.Errorf("read interactive-story lore: %w", err)
	}
	byName := loreItemsByName(items)
	byID := make(map[string]lore.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}

	selected := make([]selectedLoreItem, 0, resolvedLoreMaxItems)
	seen := map[string]bool{}
	omitted := make([]string, 0)
	// add takes a manual entry only when referenced is true, which callers set
	// for an explicit reference and leave false for a coincidental match.
	add := func(item lore.Item, reason string, referenced bool) bool {
		if item.LoadMode == lore.LoadModeResident || seen[item.ID] {
			return false
		}
		if item.LoadMode == lore.LoadModeManual && !referenced {
			return false
		}
		if len(selected) >= resolvedLoreMaxItems {
			omitted = append(omitted, item.ID)
			return false
		}
		seen[item.ID] = true
		selected = append(selected, selectedLoreItem{Item: item, Reason: reason})
		return true
	}

	for _, name := range interactive.ParseLoreReferences(in.PlanMarkdown) {
		item, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			continue
		}
		add(item, "branch plan reference", true)
	}
	for _, item := range items {
		if loreItemMentionedByName(item, in.UserAction) {
			add(item, "named in the player's action", true)
		}
	}

	terms := catalogTopicTerms(in.Catalog)
	addPresentCastLore(add, in, byID)
	addTopicLore(add, in, byID, terms)
	addIndexLore(add, in, terms, items)

	selectedContext, err := formatBoundedCompleteLoreSection(
		"Current Branch Lore Working Set (source: branch plan, this turn's action, the scene's cast, and this world's index; complete)",
		selected, ResolvedLoreContextMaxBytes,
	)
	if err != nil {
		return loreSelection{}, err
	}
	return loreSelection{Context: selectedContext, Note: loreSelectionNote(selected, omitted)}, nil
}

// packContentHash is the revision the story's world book was written under. The
// catalog's LoreID is an authored id, and the stored entry is that id qualified
// by the revision, so the selection cannot address a body without it.
func packContentHash(binding *interactive.StoryPackBinding) string {
	if binding == nil {
		return ""
	}
	return strings.TrimSpace(binding.ContentHash)
}

// loreSelectionCatalog is the catalog the lore selection reads. It is the same
// resolution the discovery page uses, so a story is never selecting people from
// one revision while being offered people from another. A story whose catalog
// cannot be read still gets its plan references, named entries and index
// matches; it simply cannot resolve a catalog person's body by source key.
func loreSelectionCatalog(c *Conversation, storyCtx interactive.StoryContext) catalogSource {
	if c == nil || c.novaDir == "" || storyCtx.Meta.PackBinding == nil {
		return nil
	}
	resolved, err := c.resolveDiscoveryCatalog(storyCtx)
	if err != nil || resolved.revisionMismatch {
		return nil
	}
	return resolved.catalog
}

// catalogTopicTerms returns the world's own topic vocabulary. It is read by type
// because the vocabulary is data each concrete catalog carries, while the
// catalogSource interface is deliberately only the query surface both answer.
func catalogTopicTerms(catalog catalogSource) map[string][]string {
	switch typed := catalog.(type) {
	case *storypack.Frozen:
		return typed.TopicTerms
	case storypack.Pack:
		return typed.TopicTerms
	default:
		return nil
	}
}

// addPresentCastLore selects the profile bodies of established characters who
// stand in this scene. That body is where the author's dialogue examples live,
// which is what lets a character speak in their own voice without the narrator
// inventing one.
func addPresentCastLore(add loreAdder, in loreSelectionInput, byID map[string]lore.Item) {
	if in.Catalog == nil || !in.HasCharacters || len(in.Scene.Present) == 0 {
		return
	}
	catalogByKey := in.Catalog.CatalogBySourceKey()
	present := make(map[string]bool, len(in.Scene.Present))
	for _, id := range in.Scene.Present {
		present[id] = true
	}
	profiles := append([]character.Profile(nil), in.Characters.Profiles...)
	sort.SliceStable(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	taken := 0
	for _, profile := range profiles {
		if taken >= resolvedLorePresentCastMax {
			return
		}
		key := strings.TrimSpace(profile.SourceKey)
		if key == "" || !present[profile.ID] {
			continue
		}
		entry, ok := catalogByKey[key]
		if !ok || strings.TrimSpace(entry.LoreID) == "" {
			continue
		}
		item, ok := byID[interactive.PackEntryIDForRevision(entry.LoreID, in.ContentHash)]
		if !ok {
			continue
		}
		if add(item, "present in this scene: "+entry.Name, false) {
			taken++
		}
	}
}

// addTopicLore selects people the world's topic vocabulary matches to this
// turn's action. Reach still applies: a person in another country is not put on
// stage by a matching topic, so only their name or a reachable placement
// selects them.
func addTopicLore(add loreAdder, in loreSelectionInput, byID map[string]lore.Item, terms map[string][]string) {
	if in.Catalog == nil || len(terms) == 0 || strings.TrimSpace(in.UserAction) == "" {
		return
	}
	candidates := in.Catalog.CharacterCandidates(storypack.CandidateRequest{
		Location: in.Location,
		Action:   in.UserAction,
		Named:    in.Catalog.MentionedNames(in.UserAction),
		Limit:    32,
	})
	taken := 0
	for _, candidate := range candidates {
		if taken >= resolvedLoreTopicMaxItems {
			return
		}
		if !candidate.Named && candidate.Reach == storypack.ReachRemote {
			continue
		}
		matched := matchedLoreTopics(candidate.Character.Topics, terms, in.UserAction)
		if !candidate.Named && len(matched) == 0 {
			continue
		}
		if strings.TrimSpace(candidate.Character.LoreID) == "" {
			continue
		}
		item, ok := byID[interactive.PackEntryIDForRevision(candidate.Character.LoreID, in.ContentHash)]
		if !ok {
			continue
		}
		reason := "named in the player's action"
		if len(matched) > 0 {
			reason = "matches this request: " + strings.Join(matched, "、")
		}
		// Being named is an explicit reference to this person, so a manual entry
		// may be taken; a topic match alone is the coincidence the setting exists
		// to exclude.
		if add(item, reason, candidate.Named) {
			taken++
		}
	}
}

// addIndexLore runs the existing lore index over the words this action actually
// touched. It is the path that reaches a setting entry, a rule note or
// user-authored lore the catalog does not index, without adding a second
// retrieval engine.
//
// Two vocabularies feed it. The world's topic vocabulary covers what people and
// places are about; the entries' own keywords cover the rules the world states
// about itself — how a craft is learned, how a beast is approached, what a
// promise does over time. Without the second, an ordinary sentence about
// learning 炼丹 reached nothing, because no topic term names the activity even
// though the rule entry does.
func addIndexLore(add loreAdder, in loreSelectionInput, terms map[string][]string, items []lore.Item) {
	keywords := mergeLoreKeywords(
		loreIntentKeywords(terms, in.UserAction),
		entryKeywordMatches(items, in.UserAction),
	)
	if len(keywords) == 0 {
		return
	}
	matches, err := lore.NewScopedStore(in.Workspace, in.Scope).WithVisibility(in.Visibility).QueryLoreItems(lore.IndexOptions{
		Keywords:        keywords,
		Match:           lore.IndexMatchAny,
		LoadModes:       []string{lore.LoadModeAuto},
		ExcludeResident: true,
		Limit:           resolvedLoreIndexMaxItems,
	})
	if err != nil {
		// The index is a widening pass, not the source of truth: the explicit
		// selections above already succeeded, so a failed index read narrows the
		// working set rather than failing the turn.
		return
	}
	for _, item := range matches {
		// The index is a widening pass on the words the player used, so a hit here
		// is a topic coincidence, never an explicit reference.
		add(item, "lore index match: "+strings.Join(keywords, "、"), false)
	}
}

// entryKeywordMatches returns the stored entries' own keywords that this action
// contains. The index already scores a keyword against an entry's keyword list;
// this only supplies the words to ask about, so it cannot select anything the
// index would not have selected on its own.
func entryKeywordMatches(items []lore.Item, action string) []string {
	action = strings.ToLower(action)
	if strings.TrimSpace(action) == "" {
		return nil
	}
	seen := make(map[string]bool)
	keywords := make([]string, 0)
	for _, item := range items {
		if item.LoadMode == lore.LoadModeResident {
			continue
		}
		for _, keyword := range item.Keywords {
			keyword = strings.TrimSpace(keyword)
			if keyword == "" || seen[keyword] {
				continue
			}
			if !strings.Contains(action, strings.ToLower(keyword)) {
				continue
			}
			seen[keyword] = true
			keywords = append(keywords, keyword)
		}
	}
	sort.Strings(keywords)
	return keywords
}

// mergeLoreKeywords joins two sorted keyword lists without duplicates.
func mergeLoreKeywords(lists ...[]string) []string {
	seen := make(map[string]bool)
	merged := make([]string, 0)
	for _, list := range lists {
		for _, keyword := range list {
			if keyword == "" || seen[keyword] {
				continue
			}
			seen[keyword] = true
			merged = append(merged, keyword)
		}
	}
	sort.Strings(merged)
	return merged
}

// matchedLoreTopics reports which of a person's topics this action touches,
// using the world's own vocabulary rather than a tokenizer.
func matchedLoreTopics(topics []string, terms map[string][]string, action string) []string {
	action = strings.ToLower(action)
	matched := make([]string, 0, len(topics))
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if strings.Contains(action, strings.ToLower(topic)) {
			matched = append(matched, topic)
			continue
		}
		for _, term := range terms[topic] {
			if term != "" && strings.Contains(action, strings.ToLower(term)) {
				matched = append(matched, topic)
				break
			}
		}
	}
	return matched
}

// loreIntentKeywords turns the topics an action touches into the words the lore
// index understands. It returns the matched term where one matched, so the index
// ranks on the word the player actually used.
func loreIntentKeywords(terms map[string][]string, action string) []string {
	action = strings.ToLower(action)
	keywords := make([]string, 0, len(terms))
	for topic, values := range terms {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if strings.Contains(action, strings.ToLower(topic)) {
			keywords = append(keywords, topic)
			continue
		}
		for _, term := range values {
			if term != "" && strings.Contains(action, strings.ToLower(term)) {
				keywords = append(keywords, term)
				break
			}
		}
	}
	sort.Strings(keywords)
	return keywords
}

func formatBoundedCompleteLoreSection(title string, items []selectedLoreItem, maxBytes int) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s\n\n", title)
	for _, selected := range items {
		block := formatInteractiveLoreItem(selected)
		if sb.Len()+len([]byte(block))+2 > maxBytes {
			return "", fmt.Errorf("%s exceeds %d bytes in total; the system will not truncate it silently, so shorten lore content, reduce active references, or adjust lore types", title, maxBytes)
		}
		sb.WriteString(block)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String()), nil
}

func formatInteractiveLoreItem(selected selectedLoreItem) string {
	item := selected.Item
	var sb strings.Builder
	fmt.Fprintf(&sb, "### [[%s]] (%s) — %s\n", strings.TrimSpace(item.Name), strings.TrimSpace(item.Type), selected.Reason)
	if brief := strings.TrimSpace(item.BriefDescription); brief != "" {
		fmt.Fprintf(&sb, "Brief: %s\n", brief)
	}
	if content := strings.TrimSpace(item.Content); content != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
	}
	return strings.TrimSpace(sb.String())
}

// loreSelectionNote is the ledger's record of the working set: what was taken,
// why, and what was dropped for capacity. It is bounded so a turn's log stays
// metadata; the entry bodies themselves are already hashed and measured by the
// audit ledger.
func loreSelectionNote(selected []selectedLoreItem, omitted []string) string {
	var sb strings.Builder
	sb.WriteString("complete=true; source=branch plan, this turn's action, the scene's cast, and this world's index")
	fmt.Fprintf(&sb, "; selected=%d [", len(selected))
	for index, item := range selected {
		if index == 8 {
			sb.WriteString(", …")
			break
		}
		if index > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%s: %s (%dB)", item.Item.ID, item.Reason, len([]byte(item.Item.Content)))
	}
	sb.WriteString("]")
	if len(omitted) > 0 {
		fmt.Fprintf(&sb, "; omitted_by_item_cap=%d [", len(omitted))
		for index, id := range omitted {
			if index == 8 {
				sb.WriteString(", …")
				break
			}
			if index > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(id)
		}
		sb.WriteString("]")
	}
	note := sb.String()
	if len([]byte(note)) > resolvedLoreNoteMaxBytes {
		note = strings.ToValidUTF8(note[:resolvedLoreNoteMaxBytes], "") + "…"
	}
	return note
}

func loreItemsByName(items []lore.Item) map[string]lore.Item {
	result := make(map[string]lore.Item, len(items))
	for _, item := range items {
		result[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	return result
}

func loreItemMentionedByName(item lore.Item, text string) bool {
	name := strings.TrimSpace(item.Name)
	return name != "" && strings.Contains(strings.ToLower(text), strings.ToLower(name))
}
