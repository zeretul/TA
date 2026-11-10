package interactive

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"ta/internal/cultivation"
	"time"
)

const definitionArtEventName = "generate_definition_art"

// DefinitionArtJob names an already registered immutable definition. It cannot
// create a definition, change a grade, grant mastery or add equipment.
type DefinitionArtJob struct {
	DefinitionID string `json:"definition_id"`
	Version      int    `json:"version"`
	Category     string `json:"category"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Style        string `json:"style"`
}

func (j DefinitionArtJob) Key() string {
	return fmt.Sprintf("definition-art-%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", j.DefinitionID, j.Version, j.Style))))
}
func (j DefinitionArtJob) Validate() error {
	if j.DefinitionID == "" || len(j.DefinitionID) > 200 || j.Version < 1 || j.Name == "" || len(j.Name) > 500 || len(j.Description) > 8192 || j.Style == "" || len(j.Style) > 100 {
		return fmt.Errorf("invalid definition art job")
	}
	switch j.Category {
	case "skill", "technique", "equipment", "craft":
		return nil
	}
	return fmt.Errorf("invalid definition art category")
}

type DefinitionArtReceipt struct {
	Job   DefinitionArtJob           `json:"job"`
	Phase string                     `json:"phase"`
	Art   *cultivation.DefinitionArt `json:"art,omitempty"`
}

// GeneratedDefinitionArtJobs discovers only committed Story-local definitions.
// Static catalogs never become an automatic bulk-illustration backlog.
func GeneratedDefinitionArtJobs(state map[string]any, style string, systems ...StoryDirectorActorStateSystem) []DefinitionArtJob {
	return generatedDefinitionArtJobs(state, style, nil, systems...)
}

func generatedDefinitionArtJobs(state map[string]any, style string, selected map[string]bool, systems ...StoryDirectorActorStateSystem) []DefinitionArtJob {
	records, _ := xiuxianEquipmentEntry(state, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	out := []DefinitionArtJob{}
	system := StoryDirectorActorStateSystem{}
	if len(systems) > 0 {
		system = systems[0]
	}
	ids := sortedKeys(records)
	if selected != nil {
		ids = make([]string, 0, len(selected))
		for id := range selected {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	for _, id := range ids {
		record, ok := records[id].(map[string]any)
		if !ok {
			continue
		}
		job := DefinitionArtJob{DefinitionID: id, Version: 1, Name: xiuxianEntryText(record, "名称"), Description: xiuxianEntryText(record, "描述"), Style: style}
		switch xiuxianEntryText(record, "类别") {
		case "":
			def, ok := generatedSkillFromRecord(record, system.CultivationRules)
			if !ok || id != def.ID {
				continue
			}
			job.Category, job.Version, job.Name = "skill", def.Version, def.Name
		case "technique":
			def, ok := generatedTechniqueFromRecord(cultivationRulesFromSystems(systems),
				record)
			if !ok || id != def.ID {
				continue
			}
			job.Category, job.Name = "technique", def.Name
		case "equipment":
			def, ok := generatedItemFromRecord(cultivationRulesFromSystems(systems),
				record)
			if !ok || id != def.ID {
				continue
			}
			job.Category = "equipment"
		case "craft":
			method, ok := generatedCraftFromRecord(system, record)
			if !ok || id != method.ID {
				continue
			}
			job.Category = "craft"
		default:
			continue
		}
		if job.Validate() == nil {
			out = append(out, job)
		}
	}
	if system.AdventureRules != nil {
		if actor, err := AdventureActorFromState(state, DefaultActorID); err == nil {
			if selected != nil {
				filtered := []AdventureGeneratedDefinition{}
				for _, d := range actor.GeneratedDefinitions {
					id, _ := d.id()
					if selected[id] {
						filtered = append(filtered, d)
					}
				}
				actor.GeneratedDefinitions = filtered
			}
			if _, err := adventureRulesForActor(*system.AdventureRules, actor); err == nil {
				for _, def := range actor.GeneratedDefinitions {
					id, _ := def.id()
					job := DefinitionArtJob{DefinitionID: id, Version: 1, Category: def.Category, Name: def.Name, Description: def.Description, Style: style}
					if def.Category == "craft" && def.Design != nil {
						productJob := DefinitionArtJob{DefinitionID: adventureGeneratedProductID(def), Version: 1, Category: "equipment", Name: def.Design.ProductName, Description: def.Description, Style: style}
						if productJob.Validate() == nil {
							out = append(out, productJob)
						}
					}
					if job.Validate() == nil {
						out = append(out, job)
					}
				}
			}
		}
	}
	return out
}

func definitionArtReceipts(lines []StoryEventRecord) map[string]DefinitionArtReceipt {
	out := map[string]DefinitionArtReceipt{}
	for _, line := range lines {
		if line.Envelope.Type != StoryEventTypeTurnDisplayAppended {
			continue
		}
		var event TurnDisplayAppendedEvent
		if mapToStruct(line.Raw, &event) != nil || event.Display.Name != definitionArtEventName {
			continue
		}
		var receipt DefinitionArtReceipt
		if json.Unmarshal([]byte(event.Display.Result), &receipt) == nil && receipt.Job.Validate() == nil && event.Display.ID == receipt.Job.Key() {
			out[receipt.Job.Key()] = receipt
		}
	}
	return out
}

// TransitionDefinitionArt persists before any billable call. A queued job may
// enter executing once; executing/failed jobs are never retried automatically,
// including after process loss. A success may arrive after a newer story turn.
// This narrow display-only annotation does not edit historical game facts.
func (s *Store) TransitionDefinitionArt(storyID, branchID, turnID string, job DefinitionArtJob, from, to string, art *cultivation.DefinitionArt) (bool, error) {
	if err := job.Validate(); err != nil {
		return false, err
	}
	if !(from == "" && to == "queued" || from == "queued" && to == "executing" || (from == "queued" || from == "executing") && to == "failed" || from == "executing" && to == "succeeded") {
		return false, fmt.Errorf("invalid definition art transition")
	}
	if to == "succeeded" {
		if art == nil || art.Source != "generated" {
			return false, fmt.Errorf("definition art result missing")
		}
		if err := art.Validate(); err != nil {
			return false, err
		}
	} else if art != nil {
		return false, fmt.Errorf("unsettled art job cannot publish art")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return false, err
	}
	defer release()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return false, err
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return false, fmt.Errorf("art branch unavailable")
	}
	_, pathSet, err := walkEventPath(branch.Head, eventsByID(lines))
	if err != nil {
		return false, err
	}
	if !pathSet[turnID] {
		return false, fmt.Errorf("art registration turn no longer belongs to branch")
	}
	if from == "" {
		p, err := s.storyBranchProjectionLocked(storyID, branchID)
		if err != nil {
			return false, err
		}
		valid := false
		system := actorStateSystemFromSnapshot(meta.ActorStateSchema, s.storyDirectorForMeta(meta).ActorState)
		if ActorStateSystemIsXiuxian(system) {
			switch job.Category {
			case "skill":
				def, ok := storySkillRegistryFromState(cultivationRulesForMeta(meta),
					p.State).Current(job.DefinitionID)
				valid = ok && def.Version == job.Version
			case "technique":
				_, valid = resolveTechnique(cultivationRulesForMeta(meta),
					job.DefinitionID, job.Version, p.State)
			case "craft":
				method, found := storyCraftRegistryFromState(system, p.State).Method(job.DefinitionID)
				valid = found && method.Version == job.Version
			case "equipment":
				_, valid = storyItemCatalog(cultivationRulesForMeta(meta),
					p.State).Definition(job.DefinitionID)
				valid = valid && job.Version == 1
			}
		} else if system.AdventureRules != nil && job.Version == 1 {
			if actor, err := AdventureActorFromState(p.State, DefaultActorID); err == nil {
				if rules, err := adventureRulesForActor(*system.AdventureRules, actor); err == nil {
					system.AdventureRules = &rules
				}
			}
			if job.Category == "craft" {
				for _, def := range system.AdventureRules.Recipes {
					valid = valid || def.ID == job.DefinitionID
				}
			}
			if job.Category == "skill" {
				for _, def := range system.AdventureRules.Skills {
					if def.ID == job.DefinitionID {
						valid = true
					}
				}
			}
			if job.Category == "equipment" {
				for _, def := range system.AdventureRules.Items {
					if def.ID == job.DefinitionID {
						valid = true
					}
				}
			}
		}
		if !valid {
			return false, fmt.Errorf("art target definition is not registered")
		}
	}
	receipt, exists := definitionArtReceipts(lines)[job.Key()]
	if from == "" && exists || from != "" && (!exists || receipt.Phase != from) {
		return false, nil
	}
	receipt = DefinitionArtReceipt{Job: job, Phase: to, Art: art}
	payload, _ := json.Marshal(receipt)
	status := "running"
	if to == "succeeded" {
		status = "success"
	}
	if to == "failed" {
		status = "error"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnDisplayAppendedEvent{V: schemaVersion, Type: StoryEventTypeTurnDisplayAppended, ID: newID("tda"), ParentID: turnID, BranchID: branchID, Ts: now, TurnID: turnID, Display: DisplayEvent{ID: job.Key(), Role: "tool_call", Name: definitionArtEventName, Content: definitionArtEventName, Status: status, Result: string(payload)}}
	meta.UpdatedAt = now
	if err := s.appendStoryTransactionLocked(storyID, meta, event); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DefinitionArtReferences(storyID, branchID string) (map[string]cultivation.DefinitionArt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, lines, err := s.readStoryLocked(storyID)
	if err != nil {
		return nil, err
	}
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return nil, fmt.Errorf("art branch unavailable")
	}
	_, pathSet, err := walkEventPath(branch.Head, eventsByID(lines))
	if err != nil {
		return nil, err
	}
	out := map[string]cultivation.DefinitionArt{}
	// Ordered canonical rows make the latest explicit style result deterministic.
	// Only registration turns on this branch may supply an image.
	for _, line := range lines {
		if line.Envelope.Type != StoryEventTypeTurnDisplayAppended {
			continue
		}
		var event TurnDisplayAppendedEvent
		if mapToStruct(line.Raw, &event) != nil || !pathSet[event.TurnID] || event.Display.Name != definitionArtEventName {
			continue
		}
		var receipt DefinitionArtReceipt
		if json.Unmarshal([]byte(event.Display.Result), &receipt) != nil || receipt.Job.Validate() != nil || event.Display.ID != receipt.Job.Key() {
			continue
		}
		if receipt.Phase == "succeeded" && receipt.Art != nil && receipt.Art.Validate() == nil {
			out[fmt.Sprintf("%s:%d", receipt.Job.DefinitionID, receipt.Job.Version)] = *receipt.Art
		}
	}
	return out, nil
}

// NewGeneratedDefinitionArtJobs compares committed snapshots, so rejected or
// merely proposed definitions never enqueue images and old entries do not
// become a surprise bulk-generation backlog.
func NewGeneratedDefinitionArtJobs(before, after map[string]any, style string, systems ...StoryDirectorActorStateSystem) []DefinitionArtJob {
	oldRecords, _ := xiuxianEquipmentEntry(before, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	records, _ := xiuxianEquipmentEntry(after, DefaultStoryContextActorID, xiuxianFieldSkillRegistry)
	selected := map[string]bool{}
	for id, value := range records {
		record, _ := value.(map[string]any)
		old, _ := oldRecords[id].(map[string]any)
		if old == nil || fmt.Sprint(old["版本"]) != fmt.Sprint(record["版本"]) {
			selected[id] = true
		}
	}
	if len(systems) > 0 && systems[0].AdventureRules != nil {
		old, _ := AdventureActorFromState(before, DefaultActorID)
		current, _ := AdventureActorFromState(after, DefaultActorID)
		known := map[string]bool{}
		for _, d := range old.GeneratedDefinitions {
			id, _ := d.id()
			known[id] = true
		}
		for _, d := range current.GeneratedDefinitions {
			id, _ := d.id()
			if !known[id] {
				selected[id] = true
			}
		}
	}
	if len(selected) == 0 {
		return []DefinitionArtJob{}
	}
	return generatedDefinitionArtJobs(after, style, selected, systems...)
}
