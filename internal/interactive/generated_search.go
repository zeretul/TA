package interactive

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// GeneratedDefinitionSearchRequest reads registered definitions, never grants
// stock or mastery. Cursor pins a page to one branch head and one query.
type GeneratedDefinitionSearchRequest struct {
	ID       string `json:"id,omitempty" jsonschema_description:"Exact generated definition ID, when known."`
	Query    string `json:"query,omitempty" jsonschema_description:"Name or ID substring; omit to browse the registered archive."`
	Category string `json:"category,omitempty" jsonschema:"enum=skill,enum=equipment,enum=craft,enum=technique"`
	Cursor   string `json:"cursor,omitempty" jsonschema_description:"Opaque next_cursor from the same query; restart after the branch changes."`
	Limit    int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=50"`
	MaxBytes int    `json:"-"`
}

type GeneratedDefinitionMatch struct {
	ActorRecords     []GeneratedDefinitionActorRecord `json:"actor_records,omitempty"`
	ActorRecordCount int                              `json:"actor_record_count,omitempty"`
	AdventureUsage   map[string]any                   `json:"adventure_usage,omitempty"`
	ID               string                           `json:"id"`
	Category         string                           `json:"category"`
	Name             string                           `json:"name"`
	TemplateID       string                           `json:"template_id"`
	Source           string                           `json:"source"`
	Description      string                           `json:"description,omitempty"`
	Definition       any                              `json:"definition"`
	Product          any                              `json:"product,omitempty"`
}

type GeneratedDefinitionActorRecord struct {
	Field string         `json:"field"`
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
}

type GeneratedDefinitionSearchResult struct {
	Head         string                     `json:"head"`
	ArchiveCount int                        `json:"archive_count"`
	Entries      []GeneratedDefinitionMatch `json:"entries"`
	NextCursor   string                     `json:"next_cursor,omitempty"`
	Notice       string                     `json:"notice"`
}

type generatedSearchCursor struct{ Head, ID, Query, Category, Last string }
type generatedSearchCandidate struct {
	id, category, name, template string
	record                       map[string]any
	adventure                    *AdventureGeneratedDefinition
}

func (s *Store) SearchGeneratedDefinitions(storyID, branchID string, req GeneratedDefinitionSearchRequest) (GeneratedDefinitionSearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return GeneratedDefinitionSearchResult{}, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	p, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return GeneratedDefinitionSearchResult{}, err
	}
	system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
	return searchGeneratedDefinitions(system, p.State, storyID+"/"+branchID+"/"+p.Head, req)
}

func searchGeneratedDefinitions(system StoryDirectorActorStateSystem, state map[string]any, head string, req GeneratedDefinitionSearchRequest) (GeneratedDefinitionSearchResult, error) {
	out := GeneratedDefinitionSearchResult{Head: head, Entries: []GeneratedDefinitionMatch{}, Notice: "已登记定义不等于已学会、持有或满足使用条件；实际学习、制作、使用仍由本局规则结算。"}
	req.ID, req.Query, req.Category = strings.TrimSpace(req.ID), strings.ToLower(strings.TrimSpace(req.Query)), strings.TrimSpace(req.Category)
	if len(req.ID) > 300 || len(req.Query) > 256 || len(req.Cursor) > 2048 {
		return out, fmt.Errorf("generated archive query too large")
	}
	if req.Category != "" && req.Category != "skill" && req.Category != "equipment" && req.Category != "craft" && req.Category != "technique" {
		return out, fmt.Errorf("unknown generated definition category")
	}
	if req.Limit <= 0 {
		req.Limit = 8
	}
	req.Limit = min(req.Limit, 50)
	if req.MaxBytes <= 0 || req.MaxBytes > 64*1024 {
		req.MaxBytes = 64 * 1024
	}
	cursor := generatedSearchCursor{Head: head, ID: req.ID, Query: req.Query, Category: req.Category}
	if req.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(req.Cursor)
		var previous generatedSearchCursor
		if err != nil || json.Unmarshal(data, &previous) != nil || previous.Head != head || previous.ID != req.ID || previous.Query != req.Query || previous.Category != req.Category {
			return out, fmt.Errorf("generated definition cursor is stale or belongs to another query; restart the search")
		}
		cursor.Last = previous.Last
	}
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	candidates := []generatedSearchCandidate{}
	for id, value := range records {
		record, ok := value.(map[string]any)
		if !ok {
			continue
		}
		category := xiuxianEntryText(record, "类别")
		if category == "" {
			category = "skill"
		}
		candidates = append(candidates, generatedSearchCandidate{id: id, category: category, name: xiuxianEntryText(record, "名称"), template: xiuxianEntryText(record, "模板"), record: record})
	}
	var adventureActor *AdventureActor
	if system.AdventureRules != nil {
		if actor, err := AdventureActorFromState(state, DefaultActorID); err == nil {
			adventureActor = &actor
			for i := range actor.GeneratedDefinitions {
				d := actor.GeneratedDefinitions[i]
				id, err := d.id()
				if err != nil {
					continue
				}
				candidates = append(candidates, generatedSearchCandidate{id: id, category: d.Category, name: d.Name, template: d.TemplateID, adventure: &d})
				if d.Category == "craft" && d.Design != nil {
					candidates = append(candidates, generatedSearchCandidate{id: adventureGeneratedProductID(d), category: "equipment", name: d.Design.ProductName, template: d.Design.ProductTemplateID, adventure: &d})
				}

			}
		}
	}
	out.ArchiveCount = len(candidates)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].id < candidates[j].id })
	for index, candidate := range candidates {
		if candidate.id <= cursor.Last || req.ID != "" && candidate.id != req.ID || req.Category != "" && candidate.category != req.Category || req.Query != "" && !strings.Contains(strings.ToLower(candidate.name+" "+candidate.id), req.Query) {
			continue
		}
		match, ok := resolveGeneratedSearchCandidate(system, candidate)
		if !ok {
			continue
		}
		if candidate.adventure != nil && adventureActor != nil {
			equipped := []string{}
			for slot, id := range adventureActor.Equipment {
				if id == candidate.id {
					equipped = append(equipped, slot)
				}
			}
			sort.Strings(equipped)
			match.AdventureUsage = map[string]any{"skill_mastery": adventureActor.Skills[candidate.id], "training_hours": adventureActor.Training[candidate.id], "inventory": adventureActor.Inventory[candidate.id], "equipped_slots": equipped}
		} else {
			match.ActorRecords, match.ActorRecordCount = generatedDefinitionActorRecords(state, candidate.id)
		}
		next := out
		next.Entries = append(append([]GeneratedDefinitionMatch(nil), out.Entries...), match)
		encoded, err := json.Marshal(next)
		if err != nil {
			return out, err
		}
		// Reserve bounded cursor/envelope room. Never return half a definition.
		if len(encoded)+2048 > req.MaxBytes {
			if len(out.Entries) == 0 {
				return out, fmt.Errorf("one definition exceeds the search result budget; increase the result budget")
			}
			data, _ := json.Marshal(cursor)
			out.NextCursor = base64.RawURLEncoding.EncodeToString(data)
			return out, nil
		}
		out = next
		cursor.Last = candidate.id
		if req.ID == "" && len(out.Entries) >= req.Limit && index < len(candidates)-1 {
			data, _ := json.Marshal(cursor)
			out.NextCursor = base64.RawURLEncoding.EncodeToString(data)
			break
		}
	}
	return out, nil
}

func resolveGeneratedSearchCandidate(system StoryDirectorActorStateSystem, c generatedSearchCandidate) (GeneratedDefinitionMatch, bool) {
	out := GeneratedDefinitionMatch{ID: c.id, Category: c.category, Name: c.name, TemplateID: c.template}
	if c.adventure != nil {
		d := *c.adventure
		out.Source, out.Description = d.Source, d.Description
		rules, err := adventureRulesForActor(*system.AdventureRules, AdventureActor{GeneratedDefinitions: []AdventureGeneratedDefinition{d}}, c.id)
		if err != nil || rules.Validate() != nil {
			return out, false
		}
		switch c.category {
		case "skill":
			for _, v := range rules.Skills {
				if v.ID == c.id {
					out.Definition = v
				}
			}
		case "equipment":
			for _, v := range rules.Items {
				if v.ID == c.id {
					out.Definition = v
				}
			}
		case "craft":
			for _, v := range rules.Recipes {
				if v.ID == c.id {
					out.Definition = v
					for _, item := range rules.Items {
						if item.ID == v.OutputID {
							out.Product = item
							break
						}
					}
				}
			}
		}
		return out, out.Definition != nil
	}
	out.Source, out.Description = xiuxianEntryText(c.record, "来源"), xiuxianEntryText(c.record, "描述")
	switch c.category {
	case "skill":
		v, ok := generatedSkillFromRecord(c.record, system.CultivationRules)
		if !ok || v.ID != c.id {
			return out, false
		}
		out.Definition = v
	case "equipment":
		v, ok := generatedItemFromRecord(system.CultivationRules,
			c.record)
		if !ok || v.ID != c.id {
			return out, false
		}
		out.Definition = v
	case "craft":
		v, ok := generatedCraftFromRecord(system, c.record)
		if !ok || v.ID != c.id {
			return out, false
		}
		out.Definition = v
	case "technique":
		v, ok := generatedTechniqueFromRecord(system.CultivationRules,
			c.record)
		if !ok || v.ID != c.id {
			return out, false
		}
		out.Definition = v
	default:
		return out, false
	}
	return out, true
}

// Usage comes from the same current projection as the definition. It prevents
// an omitted learned/owned entry from being mistaken for an unavailable item.
func generatedDefinitionActorRecords(state map[string]any, id string) ([]GeneratedDefinitionActorRecord, int) {
	out := []GeneratedDefinitionActorRecord{}
	count := 0
	appendRecord := func(field, key string, record map[string]any) {
		count++
		if len(out) < generatedContextDefinitionLimit {
			out = append(out, GeneratedDefinitionActorRecord{Field: field, Key: key, Value: generatedContextEntry(record)})
		}
	}
	fields := append([]string{xiuxianFieldLearnedSkills, xiuxianFieldTechniques, xiuxianFieldImportantItems}, xiuxianSlotOrder...)
	for _, field := range fields {
		records, _ := xiuxianEquipmentEntry(state, DefaultActorID, field)
		if xiuxianEntryText(records, "定义") == id {
			appendRecord(field, field, records)
			continue
		}
		for _, key := range sortedKeys(records) {
			record, _ := records[key].(map[string]any)
			if xiuxianEntryText(record, "定义") == id {
				appendRecord(field, key, record)
			}
		}
	}
	return out, count
}
