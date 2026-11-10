package interactive

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (s *Store) StoryContext(storyID, branchID string) (StoryContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return StoryContext{}, err
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return StoryContext{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	snapshot.ActorStateSchema = actorStateSchemaWithPresetFloor(snapshot.ActorStateSchema, meta)
	return StoryContext{Meta: meta, Snapshot: snapshot}, nil
}

func (s *Store) Snapshot(storyID, branchID string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, snapshot, err := s.boundedStorySnapshotLocked(storyID, branchID)
	if err != nil {
		return Snapshot{}, err
	}
	usageEvents, err := s.readTokenUsageEventsLocked(storyID, snapshot.BranchID)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.TokenUsageEvents = usageEvents
	// The player-facing boundary is declared by the preset, and a story that
	// started before a field was declared private carries no mark for it. The
	// floor is applied here, on the read, so the boundary a reader gets is the
	// one the preset states today without the save being rewritten.
	snapshot.ActorStateSchema = actorStateSchemaWithPresetFloor(snapshot.ActorStateSchema, meta)
	return snapshot, nil
}

// ActorStateSchema returns the story's frozen state contract, which is what
// says a field is private.
//
// It exists so a payload that carries turns but is not the snapshot — the older
// history page — can withhold exactly what the snapshot withholds. The legacy
// resolution is the same one the snapshot read performs, so a story frozen
// before the schema was embedded in its meta still answers with the contract it
// is really being played under.
func (s *Store) ActorStateSchema(storyID, branchID string) (*ActorStateSchemaSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return nil, err
	}
	defer release()
	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return nil, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	meta, err := cloneStoryMeta(handle.projection.Meta)
	if err != nil {
		return nil, err
	}
	state := map[string]any{}
	if projection := handle.projection.Branches[branchID]; projection != nil && projection.State != nil {
		state = projection.State
	}
	if err := s.freezeLegacyActorStateSchemaFromStateLocked(storyID, &meta, state); err != nil {
		return nil, err
	}
	if meta.ActorStateSchema != nil {
		handle.projection.Meta.ActorStateSchema = meta.ActorStateSchema
	}
	return actorStateSchemaWithPresetFloor(meta.ActorStateSchema, meta), nil
}

func (s *Store) boundedStorySnapshotLocked(storyID, branchID string) (StoryMeta, Snapshot, error) {
	return s.boundedStorySnapshotWithLimitLocked(storyID, branchID, defaultStoryHistoryPageTurns)
}

// boundedStorySnapshotWithLimitLocked is the shared hot-path projection. UI
// snapshots use the default page, while Director reconciliation may retain its
// full bounded decision horizon without scanning the canonical prefix.
func (s *Store) boundedStorySnapshotWithLimitLocked(storyID, branchID string, limit int) (StoryMeta, Snapshot, error) {
	release, err := s.acquireStoryReadLeaseLocked(storyID)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	defer release()

	handle, err := s.refreshStoryJournalLocked(storyID, true)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	if branchID == "" {
		branchID = handle.projection.Meta.CurrentBranch
	}
	limit = normalizeStoryHistoryPageLimit(limit)
	cacheKey := storySnapshotCacheKey{branchID: branchID, limit: limit}
	head := handle.journal.Head().Cursor
	if cached, ok := handle.snapshots[cacheKey]; ok && cached.cursor == head {
		meta, snapshot, cloneErr := cloneStorySnapshotCache(cached.meta, cached.snapshot)
		if cloneErr != nil {
			return StoryMeta{}, Snapshot{}, cloneErr
		}
		s.rememberStoryReplayStats(storyID, StoryJournalReplayStats{})
		return meta, snapshot, nil
	}

	loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, "", limit, true)
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	snapshot := loaded.snapshot
	if err := s.freezeLegacyActorStateSchemaFromStateLocked(storyID, &loaded.meta, loaded.projection.State); err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	if handle := s.storyJournals[strings.TrimSpace(storyID)]; handle != nil {
		handle.projection.Meta.ActorStateSchema = loaded.meta.ActorStateSchema
		handle.projection.Meta.StateSchemaInitialization = loaded.meta.StateSchemaInitialization
		if err := cacheStoryRecentLoaded(handle, snapshot.BranchID, loaded.meta, loaded.records); err != nil {
			return StoryMeta{}, Snapshot{}, err
		}
	}
	snapshot.ActorStateSchema = loaded.meta.ActorStateSchema
	snapshot.StateSchemaInitialization = loaded.meta.StateSchemaInitialization
	snapshot.ContextRevision = loaded.projection.ContextRevision
	snapshot.State = cloneStoryState(loaded.projection.State)
	snapshot.BranchPlan = cloneBranchPlan(loaded.projection.Plan)
	// The formal clock travels with the state so a reader deciding whether a
	// thread still binds does not have to order two pieces of prose to find out.
	snapshot.Clock = cloneStoryClock(loaded.projection.Clock)
	initializeActors := true
	if storyStateSchemaPolicyRequiresOpeningDraft(loaded.meta.StateSchemaPolicy) && loaded.meta.StateSchemaInitialization != nil && loaded.meta.StateSchemaInitialization.Status == StateSchemaInitializationWaitingOpening {
		initializeActors = false
	}
	if initializeActors {
		if err := applyFrozenMissingInitialActors(snapshot.State, loaded.meta.ActorStateSchema); err != nil {
			return StoryMeta{}, Snapshot{}, fmt.Errorf("补全冻结初始 Actor 失败: %w", err)
		}
	}
	applyLegacyActorStateAliases(snapshot.State, loaded.meta.ActorStateSchema)
	// The settled affinity is the only number the rules computed for how a
	// character feels about the protagonist. Writing it onto the Actor state
	// field here, on the one path every snapshot read goes through, is what
	// keeps the story ledger and the character window from showing two
	// different answers to the same question.
	applySettledFavorability(snapshot.State, loaded.projection.Characters)
	snapshot.TurnCount = loaded.totalTurns
	snapshot.TurnStart = loaded.turnStart
	snapshot.HistoryBeforeCursor = loaded.page.BeforeCursor
	snapshot.HasEarlierTurns = loaded.page.HasMore
	for _, input := range snapshot.PendingPlayerInputs {
		draft, found, err := s.loadTurnDraftLocked(storyID, snapshot.BranchID, DomainCommitIdentity{CommandID: input.AgentCommandID, OperationID: input.AgentOperationID, Cycle: input.AgentCycle})
		if err != nil {
			return StoryMeta{}, Snapshot{}, err
		}
		if found {
			snapshot.PendingDisplayEvents = append(snapshot.PendingDisplayEvents, sanitizeDisplayEvents(draft.DisplayEvents)...)
		}
	}
	if cachedMeta, cachedSnapshot, cloneErr := cloneStorySnapshotCache(loaded.meta, snapshot); cloneErr != nil {
		return StoryMeta{}, Snapshot{}, cloneErr
	} else {
		if handle.snapshots == nil {
			handle.snapshots = make(map[storySnapshotCacheKey]storySnapshotCache)
		}
		handle.snapshots[cacheKey] = storySnapshotCache{
			cursor: handle.journal.Head().Cursor, meta: cachedMeta, snapshot: cachedSnapshot,
		}
	}
	return loaded.meta, snapshot, nil
}

// cloneStorySnapshotCache isolates callers from the hot projection cache. The
// bounded core intentionally excludes Director-plan and token-usage overlays;
// Snapshot and StoryContext attach those independently after this cache lookup.
func cloneStorySnapshotCache(meta StoryMeta, snapshot Snapshot) (StoryMeta, Snapshot, error) {
	payload, err := json.Marshal(struct {
		Meta     StoryMeta `json:"meta"`
		Snapshot Snapshot  `json:"snapshot"`
	}{Meta: meta, Snapshot: snapshot})
	if err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	var cloned struct {
		Meta     StoryMeta `json:"meta"`
		Snapshot Snapshot  `json:"snapshot"`
	}
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return StoryMeta{}, Snapshot{}, err
	}
	// Preserve the established API distinction between an initialized empty
	// pending collection and an absent collection across the JSON deep copy.
	if snapshot.PendingPlayerInputs != nil && cloned.Snapshot.PendingPlayerInputs == nil {
		cloned.Snapshot.PendingPlayerInputs = []PlayerInputAcceptedEvent{}
	}
	if snapshot.PendingModelContextBatches != nil && cloned.Snapshot.PendingModelContextBatches == nil {
		cloned.Snapshot.PendingModelContextBatches = []ModelContextBatchEvent{}
	}
	return cloned.Meta, cloned.Snapshot, nil
}

func findEventBranch(lines []StoryEventRecord, eventID string) (string, bool) {
	for _, record := range lines {
		if record.Envelope.ID != eventID {
			continue
		}
		return record.Envelope.BranchID, record.Envelope.BranchID != ""
	}
	return "", false
}

func branchIsTerminal(lines []StoryEventRecord, head string) bool {
	if turn := latestTurnForBranchHead(lines, head); turn != nil && turn.TerminalOutcome != nil && turn.TerminalOutcome.Terminal {
		return true
	}
	path, _, err := walkEventPath(head, eventsByID(lines))
	if err != nil {
		return false
	}
	for index := len(path) - 1; index >= 0; index-- {
		if path[index].Envelope.Type == StoryEventTypeStateDelta {
			var delta StateDeltaEvent
			if mapToStruct(path[index].Raw, &delta) == nil && delta.TerminalOutcome != nil && delta.TerminalOutcome.Terminal {
				return true
			}
		}
		if path[index].Envelope.Type != StoryEventTypeCombatRecord {
			continue
		}
		var event CombatRecordEvent
		if mapToStruct(path[index].Raw, &event) == nil && event.TerminalOutcome != nil && event.TerminalOutcome.Terminal {
			return true
		}
	}
	return false
}

func stateFromPath(path []StoryEventRecord) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				applyStateDeltaToProjection(state, StateDelta{
					SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps,
					WrittenAtSeconds: delta.WrittenAtSeconds, WrittenAtEvent: delta.WrittenAtEvent,
				})
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil {
				applyTurnState(state, turn)
			}
		}
	}
	return state
}

func stateBeforeTurn(path []StoryEventRecord, turnID string) map[string]any {
	state := initialStoryState()
	for _, record := range path {
		if record.Envelope.ID == turnID {
			break
		}
		switch record.Envelope.Type {
		case StoryEventTypeStateDelta:
			var delta StateDeltaEvent
			if err := mapToStruct(record.Raw, &delta); err == nil {
				applyStateDeltaToProjection(state, StateDelta{
					SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps,
					WrittenAtSeconds: delta.WrittenAtSeconds, WrittenAtEvent: delta.WrittenAtEvent,
				})
			}
		case StoryEventTypeTurn:
			var turn TurnEvent
			if err := mapToStruct(record.Raw, &turn); err == nil {
				applyTurnState(state, turn)
			}
		}
	}
	return state
}

func (s *Store) storyDirectorForMeta(meta StoryMeta) StoryDirector {
	runtime := DefaultStoryDirector()
	template := DefaultGamePlanningTemplate()
	if strings.TrimSpace(s.novaDir) != "" {
		if selected, err := NewGamePlanningTemplateLibrary(s.novaDir).Get(meta.PlanningTemplateID); err == nil {
			template = selected
		}
	}
	runtime.ID = template.ID
	runtime.Name = template.Name
	runtime.Description = template.Description
	runtime.Strategy.PromptMarkdown = RenderGamePlanningTemplateMarkdown(template)
	runtime.Strategy.RuleStateConsumptionMode = meta.CheckSettings.RuleStateConsumptionMode
	runtime.Strategy.RuleVisibilityMode = meta.CheckSettings.RuleVisibilityMode
	refs := DefaultStoryDirectorModuleRefs()
	if meta.ModuleRefs != nil {
		refs = NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	}
	runtime.ModuleRefs = refs
	if meta.FrozenModules != nil {
		runtime.ResolvedSnapshot = *meta.FrozenModules
		return ResolveFrozenStoryDirectorModules(s.novaDir, runtime)
	}
	runtime.ResolvedSnapshot = StoryDirectorResolvedSnapshot{}
	return ResolveStoryDirectorModules(s.novaDir, runtime)
}

func terminalOutcomeFromRuleResolution(resolution RuleResolution, turnID, narrative string) *TerminalOutcome {
	if resolution.TerminalCandidate == nil {
		return nil
	}
	candidate := resolution.TerminalCandidate
	return normalizeTerminalOutcomePointer(&TerminalOutcome{
		Terminal:              true,
		Type:                  firstNonEmptyString(candidate.Type, "bad_end"),
		Reason:                candidate.Reason,
		FinalNarrativeSummary: trimBytes(narrative, maxInteractiveTextBytes),
		CausedByTurnID:        turnID,
		RuleResolutionID:      resolution.ID,
	})
}
