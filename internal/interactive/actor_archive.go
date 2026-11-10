package interactive

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	interactivestate "ta/internal/interactive/state"
)

const (
	actorArchiveRoot           = "actor_archives"
	maxActorArchiveReasonBytes = 1024
)

// ActorArchiveRecord is the replayed participation overlay for an Actor. The
// Actor itself remains under state.actors; presence in actor_archives makes it
// inactive until an explicit restore operation removes this record. A formal
// death carries its rule event and cannot be undone by a narrative restore.
type ActorArchiveRecord struct {
	Reason       string `json:"reason"`
	SourceTurnID string `json:"source_turn_id,omitempty"`
	DeathEventID string `json:"death_event_id,omitempty"`
}

// ActorArchiveSummary is the bounded, read-only archive entry exposed to
// runtime model contexts. It intentionally excludes the Actor's full fields.
type ActorArchiveSummary struct {
	ActorID      string `json:"actor_id"`
	Name         string `json:"name,omitempty"`
	TemplateID   string `json:"template_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SourceTurnID string `json:"source_turn_id,omitempty"`
	DeathEventID string `json:"death_event_id,omitempty"`
}

type actorLifecycleIntent struct {
	Index   int
	ActorID string
	Op      string
	Reason  string
}

func actorArchiveRecordFromState(state map[string]any, actorID string) (ActorArchiveRecord, bool) {
	archives, _ := state[actorArchiveRoot].(map[string]any)
	actorID = normalizeStatePanelActorID(actorID)
	raw, exists := archives[actorID]
	if !exists {
		return ActorArchiveRecord{}, false
	}
	switch typed := raw.(type) {
	case ActorArchiveRecord:
		typed.Reason = strings.TrimSpace(typed.Reason)
		typed.SourceTurnID = strings.TrimSpace(typed.SourceTurnID)
		typed.DeathEventID = strings.TrimSpace(typed.DeathEventID)
		return typed, true
	case map[string]any:
		return ActorArchiveRecord{
			Reason:       actorArchiveString(typed["reason"]),
			SourceTurnID: actorArchiveString(typed["source_turn_id"]),
			DeathEventID: actorArchiveString(typed["death_event_id"]),
		}, true
	default:
		return ActorArchiveRecord{}, true
	}
}

func actorArchiveString(value any) string {
	if value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func actorIsArchived(state map[string]any, actorID string) bool {
	_, archived := actorArchiveRecordFromState(state, actorID)
	return archived
}

func actorArchiveStatePath(actorID string) string {
	return actorArchiveRoot + "." + normalizeStatePanelActorID(actorID)
}

func actorRecordExists(state map[string]any, actorID string) bool {
	actors, _ := state[actorStateRoot].(map[string]any)
	_, exists := actors[normalizeStatePanelActorID(actorID)]
	return exists
}

func actorArchiveReasonFromValue(value any) (string, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("archive/restore 的 value 必须只包含 reason")
	}
	for key := range object {
		if key != "reason" {
			return "", fmt.Errorf("archive/restore 包含未知字段: %s", key)
		}
	}
	reason, ok := object["reason"].(string)
	if !ok || strings.TrimSpace(reason) == "" {
		return "", fmt.Errorf("archive/restore 必须提供非空 reason")
	}
	reason = strings.TrimSpace(reason)
	if len([]byte(reason)) > maxActorArchiveReasonBytes {
		return "", fmt.Errorf("archive/restore reason 超过 %d bytes", maxActorArchiveReasonBytes)
	}
	return reason, nil
}

func actorArchiveProtected(actorID string) bool {
	switch normalizeStatePanelActorID(actorID) {
	case "story", "protagonist", "world":
		return true
	default:
		return false
	}
}

// planActorLifecycleUpdates validates lifecycle transitions against the state
// at the start of the atomic module. The resulting intent map lets ordinary
// field updates compose with archive/restore regardless of submission order.
func planActorLifecycleUpdates(system StoryDirectorActorStateSystem, currentState map[string]any, updates []interactivestate.Update) (map[string]actorLifecycleIntent, error) {
	intents := map[string]actorLifecycleIntent{}
	createIndexes := map[string]int{}
	for index, update := range updates {
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) == 0 {
			continue
		}
		actorID := segments[0]
		if update.Op == interactivestate.Create && len(segments) == 1 {
			if actorRecordExists(currentState, actorID) {
				message := fmt.Sprintf("Actor 已存在，不能再次 create: %s", actorID)
				if actorIsArchived(currentState, actorID) {
					message = fmt.Sprintf("Actor 已归档，重新参与故事必须使用 restore，不能再次 create: %s", actorID)
				}
				return nil, stateUpdateError(index, "actor_already_exists", update.Path, "new actor_id", actorID, fmt.Errorf("%s", message))
			}
			createIndexes[actorID] = index
		}
		if update.Op != interactivestate.Archive && update.Op != interactivestate.Restore {
			continue
		}
		if err := interactivestate.ValidateUpdate(update); err != nil {
			return nil, stateUpdateError(index, "invalid_state_update", update.Path, "archive or restore with actor_id and reason", stateUpdateActual(update.Value), err)
		}
		if len(segments) != 1 {
			return nil, stateUpdateError(index, "invalid_actor_lifecycle_path", update.Path, "/<actor_id>", update.Path, fmt.Errorf("archive/restore 只能作用于 Actor 根路径"))
		}
		if actorID == "" || normalizeStatePanelActorID(actorID) != actorID {
			return nil, stateUpdateError(index, "invalid_actor_id", update.Path, "normalized actor_id", actorID, fmt.Errorf("状态路径包含无效 actor_id: %q", actorID))
		}
		if actorArchiveProtected(actorID) {
			return nil, stateUpdateError(index, "protected_actor_archive", update.Path, "non-system actor_id", actorID, fmt.Errorf("系统 Actor 不能归档或恢复: %s", actorID))
		}
		if previous, duplicate := intents[actorID]; duplicate {
			return nil, stateUpdateError(index, "duplicate_actor_lifecycle", update.Path, "one lifecycle operation per Actor", previous.Op, fmt.Errorf("同一次提交中每个 Actor 只能有一个 archive 或 restore 操作: %s", actorID))
		}
		reason, err := actorArchiveReasonFromValue(update.Value)
		if err != nil {
			return nil, stateUpdateError(index, "invalid_actor_lifecycle_reason", update.Path, "non-empty reason", stateUpdateActual(update.Value), err)
		}
		intents[actorID] = actorLifecycleIntent{Index: index, ActorID: actorID, Op: update.Op, Reason: reason}
	}

	for actorID, intent := range intents {
		_, createdInBatch := createIndexes[actorID]
		exists := actorRecordExists(currentState, actorID)
		if !exists {
			_, exists = actorTemplateIDFromStateOrSystem(currentState, system, actorID)
		}
		archived := actorIsArchived(currentState, actorID)
		switch intent.Op {
		case interactivestate.Archive:
			if archived {
				return nil, stateUpdateError(intent.Index, "actor_already_archived", interactivestate.FormatPath([]string{actorID}), "active actor_id", actorID, fmt.Errorf("Actor 已归档: %s", actorID))
			}
			if !exists && !createdInBatch {
				return nil, stateUpdateError(intent.Index, "actor_not_found", interactivestate.FormatPath([]string{actorID}), "existing actor_id", actorID, fmt.Errorf("归档的 Actor 不存在: %s", actorID))
			}
		case interactivestate.Restore:
			if archive, ok := actorArchiveRecordFromState(currentState, actorID); ok && archive.DeathEventID != "" {
				return nil, stateUpdateError(intent.Index, "actor_formally_dead", interactivestate.FormatPath([]string{actorID}), "living archived actor", actorID, fmt.Errorf("Actor 已由正式战斗结算死亡，普通 restore 不能撤销该结果: %s", actorID))
			}
			if createdInBatch && !exists {
				return nil, stateUpdateError(intent.Index, "actor_lifecycle_conflict", interactivestate.FormatPath([]string{actorID}), "create or restore, not both", actorID, fmt.Errorf("同一次提交不能同时 create 和 restore Actor: %s", actorID))
			}
			if !exists {
				return nil, stateUpdateError(intent.Index, "actor_not_found", interactivestate.FormatPath([]string{actorID}), "existing actor_id", actorID, fmt.Errorf("恢复的 Actor 不存在: %s", actorID))
			}
			if !archived {
				return nil, stateUpdateError(intent.Index, "actor_not_archived", interactivestate.FormatPath([]string{actorID}), "archived actor_id", actorID, fmt.Errorf("Actor 当前未归档，无需 restore: %s", actorID))
			}
		}
	}

	for index, update := range updates {
		if update.Op == interactivestate.Archive || update.Op == interactivestate.Restore {
			continue
		}
		segments, err := interactivestate.ParsePath(update.Path)
		if err != nil || len(segments) == 0 {
			continue
		}
		actorID := segments[0]
		if update.Op == interactivestate.Create {
			continue
		}
		if actorIsArchived(currentState, actorID) && intents[actorID].Op != interactivestate.Restore {
			return nil, stateUpdateError(index, "actor_archived", update.Path, "active actor_id or restore in the same module", actorID, fmt.Errorf("Actor 已归档；必须先在同一次提交中显式 restore 才能更新: %s", actorID))
		}
	}
	return intents, nil
}

func compileActorArchivePresenceCleanup(system StoryDirectorActorStateSystem, state map[string]any, intent actorLifecycleIntent, sourceTurnID string) ([]interactivestate.Op, []ActorStateOp) {
	ops := make([]interactivestate.Op, 0, 1)
	actorOps := make([]ActorStateOp, 0, 1)
	if current := getPathExact(state, "scene.present_actors"); current != nil {
		if next, changed := actorArchiveFilterActorList(current, intent.ActorID); changed {
			ops = append(ops, interactivestate.Op{Op: "set", Path: "scene.present_actors", Value: next, Reason: intent.Reason, SourceTurnID: sourceTurnID, SourceKind: interactivestate.SourceTurnResult})
		}
	}
	templateID, found := actorTemplateIDFromStateOrSystem(state, system, "story")
	if !found {
		return ops, actorOps
	}
	template := actorStateTemplateByID(system, templateID)
	// The field is found by its own name, not by the legacy path it may have been
	// written under. A frozen preset keeps only the field's identity: FreezeActorStateSchemaWithRules
	// strips Path and LegacyPath from every field, so matching on "scene.present_actors"
	// found nothing in any story that had actually been created, and an archived
	// Actor stayed on stage. The name is the identity the schema itself is keyed by.
	presentFieldID := normalizeActorStateFieldName(storyScenePresentField)
	for _, field := range template.Fields {
		if actorStateFieldID(field) != presentFieldID {
			continue
		}
		fieldID := actorStateFieldID(field)
		if next, changed := actorArchiveFilterActorList(actorStateFieldValue(state, "story", fieldID), intent.ActorID); changed {
			actorOps = append(actorOps, ActorStateOp{Op: "set", ActorID: "story", FieldID: fieldID, Value: next, Reason: intent.Reason, SourceTurnID: sourceTurnID, SourceKind: interactivestate.SourceTurnResult})
		}
		break
	}
	return ops, actorOps
}

func actorArchiveFilterActorList(value any, actorID string) ([]any, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	next := make([]any, 0, len(items))
	changed := false
	for _, item := range items {
		text, isString := item.(string)
		if isString && normalizeStatePanelActorID(text) == actorID {
			changed = true
			continue
		}
		next = append(next, item)
	}
	return next, changed
}

// actorProjectionOrder ranks the Actors a bounded projection keeps, so the cap
// cannot drop the state a turn needs in order to run.
//
// Four groups, in order: the foundational Actors (protagonist, story, world)
// that every submission and rule check targets; the characters the current scene
// places on stage, which are the ones this turn's action is about; the Actors
// the player asked to be read every turn; and everyone else, by ID for
// determinism. Sorting the whole set by ID — what the projections used to do —
// left the foundational Actors at the mercy of the alphabet: once a story passed
// the cap with dynamic Actors sorting before "protagonist", the protagonist's own
// state was the part that fell off.
//
// An Actor the player marked 指定取用 is dropped from the projection entirely
// unless the scene places them on stage. That is the same reading the world book
// gives the mode: the entry is handed over when something asks for it, not on
// every turn. The mode is a reading budget and nothing more — it never adds an
// Actor to the scene, and StorySceneFromState never learns about it.
func actorProjectionOrder(actorIDs []string, present []string) []string {
	return actorProjectionOrderWithSettings(actorIDs, present, nil)
}

// actorProjectionOrderWithSettings is actorProjectionOrder with the per-entity
// reading settings the player set on the 本局新建 page.
func actorProjectionOrderWithSettings(actorIDs []string, present []string, settings map[string]string) []string {
	onStage := make(map[string]bool, len(present))
	for _, id := range present {
		if id = strings.TrimSpace(id); id != "" {
			onStage[id] = true
		}
	}
	rank := func(id string) int {
		mode := strings.TrimSpace(settings[id])
		switch {
		case foundationalTurnSubmissionActor(id):
			return 0
		case onStage[id]:
			return 1
		case mode == LocalEntityLoadResident:
			return 2
		default:
			return 3
		}
	}
	ordered := make([]string, 0, len(actorIDs))
	for _, id := range actorIDs {
		// 指定取用 withholds the record until the scene asks for the person. It
		// is a real choice with a real cost, so it is stated rather than folded
		// into "not read this turn".
		if strings.TrimSpace(settings[id]) == LocalEntityLoadManual && !onStage[id] && !foundationalTurnSubmissionActor(id) {
			continue
		}
		ordered = append(ordered, id)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		ri, rj := rank(ordered[i]), rank(ordered[j])
		if ri != rj {
			return ri < rj
		}
		return ordered[i] < ordered[j]
	})
	return ordered
}

// ActorStateRuntimeProjection returns a deterministic model-facing snapshot:
// active Actors retain their full state, while archived Actors appear only in
// a compact provenance index. Counts and truncation are explicit so callers do
// not mistake an omitted Actor for a nonexistent one.
func ActorStateRuntimeProjection(system StoryDirectorActorStateSystem, state map[string]any) map[string]any {
	return ActorStateRuntimeProjectionWithSettings(system, state, nil)
}

// ActorStateRuntimeProjectionWithSettings is ActorStateRuntimeProjection with
// the per-entity reading settings the player set on the 本局新建 page, so a
// bounded projection keeps the Actors marked 常驻 even after the scene has moved
// on. Settings only reorder and withhold the same set: nobody is added, an
// archived Actor stays archived, and the scene's own 在场 list is untouched.
func ActorStateRuntimeProjectionWithSettings(system StoryDirectorActorStateSystem, state map[string]any, settings map[string]string) map[string]any {
	system = normalizeActorStateSystem(system)
	projected := cloneActorStateRoot(state)
	if err := applyMissingInitialActors(projected, system, "运行时 Actor 投影"); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-state] project initial Actors into runtime projection failed err=%v location=internal/interactive/actor_archive.go", err))
	}
	rawActors, _ := projected[actorStateRoot].(map[string]any)
	actorIDs := make([]string, 0, len(rawActors))
	for actorID := range rawActors {
		actorIDs = append(actorIDs, actorID)
	}
	actorIDs = actorProjectionOrderWithSettings(actorIDs, StorySceneFromState(projected, "").Present, settings)
	activeActors := map[string]any{}
	activeTotal := 0
	for _, actorID := range actorIDs {
		if actorIsArchived(projected, actorID) {
			continue
		}
		activeTotal++
		if len(activeActors) < maxInteractiveListItems {
			activeActors[actorID] = rawActors[actorID]
		}
	}

	rawArchives, _ := projected[actorArchiveRoot].(map[string]any)
	archiveIDs := make([]string, 0, len(rawArchives))
	for actorID := range rawArchives {
		archiveIDs = append(archiveIDs, actorID)
	}
	sort.Strings(archiveIDs)
	archived := make([]ActorArchiveSummary, 0, min(len(archiveIDs), maxInteractiveListItems))
	for _, actorID := range archiveIDs {
		if len(archived) >= maxInteractiveListItems {
			break
		}
		record, _ := rawActors[actorID].(map[string]any)
		archive, _ := actorArchiveRecordFromState(projected, actorID)
		archived = append(archived, ActorArchiveSummary{
			ActorID:      actorID,
			Name:         actorArchiveString(record["name"]),
			TemplateID:   normalizeActorStateID(actorArchiveString(record["template_id"])),
			Reason:       archive.Reason,
			SourceTurnID: archive.SourceTurnID,
			DeathEventID: archive.DeathEventID,
		})
	}
	return map[string]any{
		actorStateRoot:   activeActors,
		actorArchiveRoot: archived,
		"projection": map[string]any{
			"source":             "Snapshot.State.actors + Snapshot.State.actor_archives",
			"active_total":       activeTotal,
			"active_included":    len(activeActors),
			"archived_total":     len(archiveIDs),
			"archived_included":  len(archived),
			"active_truncated":   activeTotal > len(activeActors),
			"archived_truncated": len(archiveIDs) > len(archived),
		},
	}
}

func sortedActorLifecycleIntents(intents map[string]actorLifecycleIntent, op string) []actorLifecycleIntent {
	result := make([]actorLifecycleIntent, 0, len(intents))
	for _, intent := range intents {
		if intent.Op == op {
			result = append(result, intent)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result
}
