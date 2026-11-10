package interactive

import (
	"encoding/json"
	"fmt"
	"strings"

	"ta/internal/agents/conversationjournal"
	"ta/internal/agents/sessionjournal"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/npcgrowth"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

const (
	// Version 19 adds the per-branch formal time axis and its safe recovery
	// activities: one comparable second-level instant every in-fiction interval
	// advances, plus the rest and meditation activities that spend it.
	// Version 20 adds the branch's journeys: the road each branch is standing on
	// and how far it has really walked.
	// Version 21 adds how each character feels about the protagonist and the
	// formal relationships that stand between them.
	// Version 22 adds each non-protagonist actor's growth watermark: how much
	// world time the growth rules have already been paid for.
	// Version 24 rebuilds verified cooperation and realm action source references.
	// Version 25 restores character facts when the latest main turn is replaced.
	storyProjectionVersion      = 25
	storyRecentTransactionLimit = 200
	storyRecentCommitLimit      = 200
	storyTurnAnchorEvery        = 256
)

type storyTurnAnchor struct {
	Before int                        `json:"before"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

type storyCommitLocator struct {
	CommandID   string                     `json:"command_id"`
	OperationID string                     `json:"operation_id"`
	Cycle       int                        `json:"cycle"`
	BranchID    string                     `json:"branch_id"`
	Hash        string                     `json:"hash"`
	EventID     string                     `json:"event_id"`
	EventType   string                     `json:"event_type"`
	Cursor      conversationjournal.Cursor `json:"cursor"`
}

type storyBranchProjection struct {
	CultivationRules   *cultivation.Rules `json:"-"`
	WorldContent       *WorldContent      `json:"-"`
	WorldPackID        string             `json:"-"`
	LocalAffairs       []LocalAffairState `json:"local_affairs,omitempty"`
	Head               string             `json:"head"`
	ContextRevision    uint64             `json:"context_revision"`
	HistoryRevision    string             `json:"history_revision,omitempty"`
	HistoryEpoch       string             `json:"history_epoch,omitempty"`
	LatestTurnID       string             `json:"latest_turn_id,omitempty"`
	LatestTurnParentID string             `json:"latest_turn_parent_id,omitempty"`
	Depth              int                `json:"depth"`
	State              map[string]any     `json:"state"`
	// PackCatalog is read-only author metadata projected from StoryMeta for
	// source-qualified rules. It is not a second persisted branch state.
	PackCatalog       *PackCatalogSnapshot `json:"-"`
	StateBeforeLatest map[string]any       `json:"state_before_latest,omitempty"`
	Plan              *BranchPlan          `json:"plan,omitempty"`
	PlanBeforeLatest  *BranchPlan          `json:"plan_before_latest,omitempty"`
	// TimeBeforeLatest is the formal time axis as it stood before the latest
	// Turn, saved for exactly the reason StateBeforeLatest is. A Turn carries
	// no clock of its own, so a training, resting or crafting turn publishes
	// its interval as a side record after itself; that record is part of the
	// position the Turn left the branch on. Replacing the Turn has to put the
	// clock, the activities and the growth watermark back where the
	// replacement starts from, or the branch would keep the replaced
	// interval's time and its own.
	TimeBeforeLatest      *storyTimeCheckpoint       `json:"time_before_latest,omitempty"`
	PendingPlayerInputIDs []string                   `json:"pending_player_input_ids,omitempty"`
	PendingInterruption   *TurnInterruptedEvent      `json:"pending_interruption,omitempty"`
	TailCursor            conversationjournal.Cursor `json:"tail_cursor,omitempty"`
	// Characters is the branch's character projection: who exists, what was
	// said, what each of them knows, what is still owed, and what they sent on
	// their own initiative. It lives on the branch because memory is
	// branch-scoped: two branches of one story must not share conversations.
	Characters *character.State `json:"characters,omitempty"`
	// Like state and time, character facts established by a replaced main turn
	// must leave the active path. This is a rebuildable latest-turn checkpoint,
	// not a second journal or a rewrite of historical character records.
	CharactersBeforeLatest *character.State `json:"characters_before_latest,omitempty"`
	// Combats is the branch's live battles, keyed by session id. A battle is
	// rebuilt from its own side records on replay, so this is a cache of the
	// canonical journal rather than a second place battles are stored: dropping
	// it and replaying the branch produces the same map.
	//
	// It lives on the branch for the same reason character memory does. A battle
	// is fought against a particular set of facts, and a branch that forked
	// before the fight must not inherit its wounds.
	Combats map[string]cultivation.CombatSession `json:"combats,omitempty"`
	// Clock is the branch's formal time axis: the single comparable second-level
	// instant every activity that spends in-fiction time advances. It lives on
	// the branch because time is branch-scoped: a fork inherits the instant its
	// parent had at the fork point and the two advance separately afterwards.
	//
	// It is nil for a branch that has never spent formal time. That is not an
	// error and not a zero clock: §三.2 forbids back-filling history for stories
	// played before the formal axis existed, so the origin is established the
	// first time an activity actually spends time.
	Clock *StoryClock `json:"clock,omitempty"`
	// Recoveries is the branch's safe rest and meditation activities, keyed by
	// activity id. A recovery is rebuilt from its own side records on replay, so
	// this is a cache of the canonical journal rather than a second place rest is
	// stored: dropping it and replaying the branch produces the same map.
	//
	// It lives on the branch for the same reason the clock does. A rest is taken
	// against a particular set of wounds, and a branch that forked before the
	// rest must not inherit its recovery.
	Recoveries map[string]RecoveryActivity `json:"recoveries,omitempty"`
	// Cultivations is the branch's player-named cultivation intervals, keyed by
	// activity id. Like the recoveries it is rebuilt from its own side records on
	// replay, so this is a cache of the canonical journal rather than a second
	// place training is stored.
	//
	// It lives on the branch for the same reason the clock does. An interval is
	// spent against a particular position and a particular set of conditions, and
	// a branch that forked before the interval must not inherit its progress.
	Cultivations map[string]CultivationActivity `json:"cultivations,omitempty"`
	// Journeys is the branch's travel, keyed by journey id. Like the recoveries
	// it is rebuilt from its own side records on replay, so this is a cache of
	// the canonical journal rather than a second place a road is stored.
	//
	// It lives on the branch for the same reason the clock does. A fork inherits
	// the journey its parent was standing on at the fork point, and from then on
	// the two walk separately; a branch that forked before a road must not
	// inherit its distance.
	Journeys map[string]exploration.JourneySnapshot `json:"journeys,omitempty"`
	// Realms is the branch's realm instances, keyed by instance id. Like the
	// journeys it is rebuilt from its own side records on replay, so this is a
	// cache of the canonical journal rather than a second place a realm is
	// stored.
	//
	// It lives on the branch for the same reason the clock does. A realm keeps
	// the rooms it has revealed and the rewards it has handed over, and a branch
	// that forked before an exploration must not inherit what that branch found.
	Realms map[string]RealmInstanceState `json:"realms,omitempty"`
	// Quests is the branch's task instances, keyed by task id. Like the realms
	// it is rebuilt from its own side records on replay, so this is a cache of
	// the canonical journal rather than a second place a task is stored.
	//
	// It lives on the branch for the same reason the clock does. A task is an
	// agreement between this character and this branch's world, and a branch that
	// forked before the agreement must not inherit what it promised.
	Quests map[string]quest.Instance `json:"quests,omitempty"`
	// PublicEvents is this character's participation in the public events the
	// branch knows about, keyed by event id. The events' own stages are
	// deliberately not stored: a stage is a reading of the committed clock, so
	// storing it would create a second time axis that could disagree with the
	// branch's.
	//
	// It lives on the branch for the same reason the tasks do. A fork that
	// happened before the character took part must not inherit the participation,
	// and a fork cut after it must keep the stage they were actually present at.
	PublicEvents map[string]publiquevent.Instance `json:"public_events,omitempty"`
	// NPCGrowth is the growth rules' settlement watermark for each
	// non-protagonist actor, keyed by actor id: the revision and the instant the
	// rules have already been paid for. It is rebuilt from its own side records
	// on replay, so it is a cache of the canonical journal rather than a second
	// ledger.
	//
	// It lives on the branch for the same reason the clock does. A branch that
	// forked before an interval must not inherit the growth that interval paid
	// for, or the same stretch of life would be granted twice — once on each
	// branch.
	NPCGrowth map[string]npcgrowth.Snapshot `json:"npc_growth,omitempty"`
}

// clockSeconds reports the branch's formal instant, treating a branch that has
// never spent formal time as being at its own origin.
func (branch *storyBranchProjection) clockSeconds() int64 {
	if branch == nil || branch.Clock == nil {
		return 0
	}
	return branch.Clock.Seconds
}

// storyTimeSnapshot copies the formal time axis and everything measured in it.
//
// It exists so a Turn can be replaced from the position it started at. The
// clock, the activities that spent it and the growth watermark are one
// position; saving some of them and not the others would leave a regenerated
// turn either carrying time it no longer spent or having spent time twice.
func (branch *storyBranchProjection) storyTimeSnapshot() *storyTimeCheckpoint {
	if branch == nil {
		return nil
	}
	return &storyTimeCheckpoint{
		LocalAffairs: cloneLocalAffairs(branch.LocalAffairs),
		Clock:        cloneStoryClock(branch.Clock),
		Recoveries:   cloneRecoveryActivities(branch.Recoveries),
		Cultivations: cloneCultivationActivities(branch.Cultivations),
		Journeys:     cloneJourneySnapshots(branch.Journeys),
		Realms:       cloneRealmInstances(branch.Realms),
		Quests:       cloneQuestInstances(branch.Quests),
		PublicEvents: clonePublicEventInstances(branch.PublicEvents),
		NPCGrowth:    cloneNPCGrowthSnapshots(branch.NPCGrowth),
	}
}

// restoreStoryTimeSnapshot puts the branch back on a saved time axis. A nil
// checkpoint means the branch had no formal time to describe and is left
// alone rather than being reset to an invented origin.
func (branch *storyBranchProjection) restoreStoryTimeSnapshot(checkpoint *storyTimeCheckpoint) {
	if branch == nil || checkpoint == nil {
		return
	}
	branch.LocalAffairs = cloneLocalAffairs(checkpoint.LocalAffairs)
	branch.Clock = cloneStoryClock(checkpoint.Clock)
	branch.Recoveries = cloneRecoveryActivities(checkpoint.Recoveries)
	branch.Cultivations = cloneCultivationActivities(checkpoint.Cultivations)
	branch.Journeys = cloneJourneySnapshots(checkpoint.Journeys)
	branch.Realms = cloneRealmInstances(checkpoint.Realms)
	branch.Quests = cloneQuestInstances(checkpoint.Quests)
	branch.PublicEvents = clonePublicEventInstances(checkpoint.PublicEvents)
	branch.NPCGrowth = cloneNPCGrowthSnapshots(checkpoint.NPCGrowth)
}

// storyJournalProjection is the bounded game reducer checkpoint. It stores
// current branch state and sparse locators, never historical narrative,
// thinking, rich tool results, maintenance state, or prior state snapshots.
type storyJournalProjection struct {
	Version       int                               `json:"version"`
	StoryID       string                            `json:"story_id"`
	Generation    string                            `json:"generation"`
	Meta          StoryMeta                         `json:"meta"`
	EventCount    int                               `json:"event_count"`
	TurnCount     int                               `json:"turn_count"`
	RecentCursors []conversationjournal.Cursor      `json:"recent_cursors,omitempty"`
	TurnAnchors   []storyTurnAnchor                 `json:"turn_anchors,omitempty"`
	RecentCommits []storyCommitLocator              `json:"recent_commits,omitempty"`
	Branches      map[string]*storyBranchProjection `json:"branches"`
	AgentSessions sessionjournal.Projection         `json:"agent_sessions,omitempty"`
	TurnDrafts    map[string]storyDraftLocator      `json:"turn_drafts,omitempty"`

	expectedID         string
	expectedGeneration string
	lastCursor         conversationjournal.Cursor
}

func newStoryJournalProjection(storyID, generation string) *storyJournalProjection {
	projection := &storyJournalProjection{expectedID: strings.TrimSpace(storyID), expectedGeneration: strings.TrimSpace(generation)}
	_ = projection.Reset()
	return projection
}

func (projection *storyJournalProjection) Reset() error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	*projection = storyJournalProjection{
		Version: storyProjectionVersion, StoryID: expectedID, Generation: expectedGeneration,
		Branches: make(map[string]*storyBranchProjection), TurnDrafts: make(map[string]storyDraftLocator), expectedID: expectedID, expectedGeneration: expectedGeneration,
	}
	projection.AgentSessions.Reset()
	return nil
}

func (projection *storyJournalProjection) Restore(data json.RawMessage) error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	var restored storyJournalProjection
	if err := json.Unmarshal(data, &restored); err != nil {
		return err
	}
	if restored.Version != storyProjectionVersion {
		return fmt.Errorf("%w: unsupported story projection version %d", conversationjournal.ErrProjectionCheckpointIncompatible, restored.Version)
	}
	if restored.StoryID != expectedID || restored.Generation != expectedGeneration {
		return fmt.Errorf("story projection identity mismatch")
	}
	if restored.Branches == nil {
		restored.Branches = make(map[string]*storyBranchProjection)
	}
	if restored.TurnDrafts == nil {
		restored.TurnDrafts = make(map[string]storyDraftLocator)
	}
	restored.expectedID = expectedID
	restored.expectedGeneration = expectedGeneration
	if err := restored.AgentSessions.Normalize(); err != nil {
		return err
	}
	if len(restored.RecentCursors) > 0 {
		restored.lastCursor = restored.RecentCursors[len(restored.RecentCursors)-1]
	}
	*projection = restored
	return nil
}

func (projection *storyJournalProjection) Checkpoint() (json.RawMessage, error) {
	return json.Marshal(projection)
}

func (projection *storyJournalProjection) Apply(record conversationjournal.Record) error {
	projection.rememberCursor(record.Location.Cursor)
	if handled, err := projection.AgentSessions.Apply(record); handled || err != nil {
		return err
	}
	meta, events, err := decodeStoryProjectionPayload(record.Payload)
	if err != nil {
		return err
	}
	if meta.StoryID != "" {
		return projection.applyMeta(record.Location.Cursor, meta)
	}
	if len(events) != 1 {
		return fmt.Errorf("story projection payload is empty")
	}
	return projection.applyEvent(record.Location.Cursor, events[0])
}

func decodeStoryProjectionPayload(payload json.RawMessage) (StoryMeta, []StoryEventRecord, error) {
	var meta StoryMeta
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		return StoryMeta{}, nil, err
	}
	if typed.Type == StoryEventTypeMeta {
		if err := json.Unmarshal(payload, &meta); err != nil {
			return StoryMeta{}, nil, err
		}
		meta = normalizeStoryMeta(meta)
		if err := validateStoryMeta(meta); err != nil {
			return StoryMeta{}, nil, err
		}
		return meta, nil, nil
	}
	if typed.Type == sessionjournal.RecordType {
		return StoryMeta{}, nil, nil
	}
	event, err := decodeStoryEventRecord(payload)
	if err != nil {
		return StoryMeta{}, nil, err
	}
	return StoryMeta{}, []StoryEventRecord{event}, nil
}

func (projection *storyJournalProjection) applyMeta(cursor conversationjournal.Cursor, meta StoryMeta) error {
	meta = normalizeStoryMeta(meta)
	if err := validateStoryMeta(meta); err != nil {
		return err
	}
	if meta.StoryID != projection.expectedID {
		return fmt.Errorf("story projection id mismatch: have=%q want=%q", meta.StoryID, projection.expectedID)
	}
	projection.Meta = meta
	projection.StoryID = meta.StoryID
	projection.Generation = projection.expectedGeneration
	for branchID, branchMeta := range meta.Branches {
		branch := projection.branch(branchID)
		// The immutable first metadata row describes the final legacy head even
		// though its flat events follow. Rebuild that path from the root.
		if cursor != 1 || projection.EventCount > 0 {
			branch.Head = branchMeta.Head
		}
	}
	for branchID := range projection.Branches {
		if _, ok := meta.Branches[branchID]; !ok {
			delete(projection.Branches, branchID)
		}
	}
	return nil
}

func (projection *storyJournalProjection) applyEvent(cursor conversationjournal.Cursor, record StoryEventRecord) error {
	changesModelContext, err := storyEventChangesModelContext(record.Envelope.Type)
	if err != nil {
		return err
	}
	projection.EventCount++
	branch := projection.branch(record.Envelope.BranchID)
	branch.TailCursor = cursor
	parentID := parentIDFromRaw(record.Raw)
	if changesModelContext {
		// These three publications append to the canonical lane. A replacement
		// Turn or a creator edit invalidates checkpoints even when only archived
		// bodies changed. Both markers are derived from existing event IDs.
		switch record.Envelope.Type {
		case StoryEventTypePlayerInput, StoryEventTypeModelContextBatch:
			branch.HistoryRevision = record.Envelope.ID
		case StoryEventTypeTurn:
			branch.HistoryRevision = record.Envelope.ID
			if parentID != branch.Head {
				branch.HistoryEpoch = record.Envelope.ID
			}
		case StoryEventTypeStateDelta, StoryEventTypeTurnInterrupted, StoryEventTypeTurnStateRevised, StoryEventTypeStoryConfigUpdated, StoryEventTypeBranchPlanRevised, StoryEventTypeCharacterRecord, StoryEventTypeCombatRecord, StoryEventTypeRecoveryRecord, StoryEventTypeCultivationRecord, StoryEventTypeJourneyRecord, StoryEventTypeRealmRecord, StoryEventTypeQuestRecord, StoryEventTypePublicEventRecord, StoryEventTypeNPCGrowthRecord, StoryEventTypeCraftRecord, StoryEventTypeCooperationRecord, StoryEventTypeLocalAffairRecord:
			// These affect next-turn preparation, not canonical raw messages.
		case StoryEventTypeBranch, StoryEventTypeBranchHeadMoved, StoryEventTypeTurnVersionSelected, StoryEventTypeTurnNarrativeRevised:
			branch.HistoryEpoch, branch.HistoryRevision = record.Envelope.ID, record.Envelope.ID
		default:
			return fmt.Errorf("canonical history does not handle model context event %q", record.Envelope.Type)
		}
	}
	switch record.Envelope.Type {
	case StoryEventTypeTurn:
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return err
		}
		delete(projection.TurnDrafts, turnDraftKey(turn.BranchID, DomainCommitIdentity{CommandID: turn.AgentCommandID, OperationID: turn.AgentOperationID, Cycle: turn.AgentCycle}))
		branch.consumePlayerInputs(turn.PlayerInputID, turn.ConsumedPlayerInputIDs)
		if branch.PendingInterruption != nil && !branch.hasPendingPlayerInput(branch.PendingInterruption.PlayerInputID) {
			branch.PendingInterruption = nil
		}
		if projection.TurnCount%storyTurnAnchorEvery == 0 {
			projection.TurnAnchors = append(projection.TurnAnchors, storyTurnAnchor{Before: projection.TurnCount, Cursor: cursor})
		}
		projection.TurnCount++
		if parentID == branch.Head || (branch.Head == "" && parentID == "") {
			before := cloneStoryState(branch.State)
			branch.StateBeforeLatest = cloneStoryState(before)
			branch.PlanBeforeLatest = cloneBranchPlan(branch.Plan)
			branch.TimeBeforeLatest = branch.storyTimeSnapshot()
			branch.CharactersBeforeLatest = cloneCharacterState(branch.Characters)
			branch.State = cloneStoryState(before)
			if branch.Clock == nil && turn.ClockAdvance != nil && turn.ClockAdvance.Before != NewStoryClock(storySceneText(branch.State, storySceneTimeField)) {
				if !normalizeLegacyFirstWaitClockOrigin(branch, &turn) {
					return fmt.Errorf("first wait clock origin does not match the branch scene")
				}
			}
			if err := applyTurnClockAdvance(branch, turn); err != nil {
				return err
			}
			applyTurnState(branch.State, turn)
			branch.Head = turn.ID
			branch.LatestTurnID = turn.ID
			branch.LatestTurnParentID = parentID
			branch.Depth++
		} else if parentID == branch.LatestTurnParentID && branch.LatestTurnID != "" {
			branch.State = cloneStoryState(branch.StateBeforeLatest)
			branch.Plan = cloneBranchPlan(branch.PlanBeforeLatest)
			branch.restoreStoryTimeSnapshot(branch.TimeBeforeLatest)
			if branch.TimeBeforeLatest != nil {
				branch.Characters = cloneCharacterState(branch.CharactersBeforeLatest)
			}
			if branch.Clock == nil && turn.ClockAdvance != nil && turn.ClockAdvance.Before != NewStoryClock(storySceneText(branch.State, storySceneTimeField)) {
				if !normalizeLegacyFirstWaitClockOrigin(branch, &turn) {
					return fmt.Errorf("replacement wait clock origin does not match the branch scene")
				}
			}
			if err := applyTurnClockAdvance(branch, turn); err != nil {
				return err
			}
			applyTurnState(branch.State, turn)
			branch.Head = turn.ID
			branch.LatestTurnID = turn.ID
		}
		projection.rememberCommit(cursor, StoryEventTypeTurn, turn.ID, turn.BranchID, turn.AgentCommandID, turn.AgentOperationID, turn.AgentCycle, turn.AgentCommitHash)
	case StoryEventTypeStateDelta:
		var delta StateDeltaEvent
		if err := mapToStruct(record.Raw, &delta); err != nil {
			return err
		}
		if parentID == branch.Head || record.Envelope.ID == branch.Head {
			if delta.ClockAdvance != nil {
				if err := applyTurnClockAdvance(branch, TurnEvent{User: delta.ClockAdvance.SourceText, ClockAdvance: delta.ClockAdvance}); err != nil {
					return err
				}
			}
			applyStateDeltaToProjection(branch.State, StateDelta{
				SchemaVersion: delta.SchemaVersion, Ops: delta.Ops, ActorOps: delta.ActorOps,
				WrittenAtSeconds: delta.WrittenAtSeconds, WrittenAtEvent: delta.WrittenAtEvent,
			})
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypePlayerInput:
		var event PlayerInputAcceptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.rememberPendingPlayerInput(event.ID)
		projection.rememberCommit(cursor, StoryEventTypePlayerInput, event.ID, event.BranchID, event.AgentCommandID, event.AgentOperationID, event.AgentCycle, event.AgentCommitHash)
	case StoryEventTypeTurnInterrupted:
		var event TurnInterruptedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.PendingInterruption = &event
	case StoryEventTypeModelContextBatch:
		var event ModelContextBatchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		normalized, err := normalizeModelContextBatchEvent(event)
		if err != nil {
			return err
		}
		projection.rememberCommit(cursor, StoryEventTypeModelContextBatch, normalized.ID, normalized.BranchID, normalized.AgentCommandID, normalized.AgentOperationID, normalized.AgentCycle, "")
	case StoryEventTypeProviderContinuation, StoryEventTypeModelContextProviderContinuation:
		// Opaque state is joined to its owner only by bounded model-history reads.
		// It never changes branch state or public projections by itself.
	case StoryEventTypeBranchPlanUpdated:
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.TurnID == branch.LatestTurnID && event.ParentID == event.TurnID {
			branch.Plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts, Revision: event.ID}
		}
	case StoryEventTypeBranchPlanRevised:
		var event BranchPlanUpdatedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.TurnID == branch.LatestTurnID && event.ParentID == branch.Head {
			branch.Plan = &BranchPlan{Markdown: event.Markdown, UpdatedTurnID: event.TurnID, UpdatedAt: event.Ts, Revision: event.ID}
			branch.Head = event.ID
		}
	case StoryEventTypeCharacterRecord:
		var event CharacterRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// A record only counts when it belongs to the path the branch is actually
		// on. A regenerated turn leaves its old character record behind in the
		// journal, and folding that back in would have the cast remember a scene
		// the story replaced.
		if event.ParentID == branch.Head {
			if err := applyCharacterRecord(branch, event); err != nil {
				return err
			}
			// A character record is a side record: it advances the branch head so
			// the next turn is parented after it, without creating a Turn or
			// consuming a turn number.
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeCombatRecord:
		var event CombatRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule as a character record: a battle a regenerated turn
		// left behind must not have its wounds folded into a branch that no longer
		// contains that fight.
		if event.ParentID == branch.Head {
			if err := applyCombatRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeRecoveryRecord:
		var event RecoveryRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again: a rest a regenerated turn left behind must not
		// have its recovery and its spent time folded into a branch that no longer
		// contains that interval.
		if event.ParentID == branch.Head {
			if err := applyRecoveryRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeCultivationRecord:
		var event CultivationRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule a rest follows. An interval a regenerated turn left
		// behind must not have its training and its spent years folded into a
		// branch that no longer contains that interval.
		if event.ParentID == branch.Head {
			if err := applyCultivationRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeJourneyRecord:
		var event JourneyRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. A stretch of road a regenerated turn left
		// behind must not have its distance and its place folded into a branch
		// that no longer contains that stretch.
		if event.ParentID == branch.Head {
			if err := applyJourneyRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeRealmRecord:
		var event RealmRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. A realm action a regenerated turn left behind
		// must not fold its rooms and its taken rewards into a branch that no
		// longer contains that action.
		if event.ParentID == branch.Head {
			if err := applyRealmRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeQuestRecord:
		var event QuestRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. A task action a regenerated turn left behind
		// must not fold its agreement and its payment into a branch that no longer
		// contains that action.
		if event.ParentID == branch.Head {
			if err := applyQuestRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypePublicEventRecord:
		var event PublicEventRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. A participation a regenerated turn left behind
		// must not fold its choice and its payment into a branch that no longer
		// contains that action.
		if event.ParentID == branch.Head {
			if err := applyPublicEventRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeNPCGrowthRecord:
		var event NPCGrowthRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. Growth a regenerated turn left behind must not
		// fold its watermark into a branch that no longer contains the interval
		// that paid for it, or the actor would be paid for that time on a path
		// where the rules were never asked.
		if event.ParentID == branch.Head {
			if err := applyNPCGrowthRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeLocalAffairRecord:
		var event LocalAffairRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.ParentID == branch.Head {
			if err := applyLocalAffairRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeCooperationRecord:
		var event CooperationRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		if event.ParentID == branch.Head {
			if err := applyCooperationRecord(branch, event, projection.StoryID); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeCraftRecord:
		var event CraftRecordEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		// The same path rule again. A batch a regenerated turn left behind must not
		// move the clock into a branch that no longer contains that work. The pack
		// changes are not folded here: they are in the Turn's own StateDelta, and
		// applying them twice would spend the materials twice.
		if event.ParentID == branch.Head {
			if err := applyCraftRecord(branch, event); err != nil {
				return err
			}
			branch.Head = record.Envelope.ID
		}
	case StoryEventTypeTurnStateRevised:
		var revision TurnStateRevisedEvent
		if err := mapToStruct(record.Raw, &revision); err != nil {
			return err
		}
		if revision.TurnID == branch.LatestTurnID && (revision.ClearStateDelta || revision.StateDelta != nil) {
			branch.State = cloneStoryState(branch.StateBeforeLatest)
			if !revision.ClearStateDelta && revision.StateDelta != nil {
				applyStateDeltaToProjection(branch.State, *revision.StateDelta)
			}
		}
	case StoryEventTypeBranch:
		var event BranchEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.ParentID
		branch.LatestTurnID = event.LatestTurnID
		branch.Depth = event.Depth
		// A fork inherits the complete model-visible prefix at its parent. Its
		// revision therefore starts at that canonical depth rather than at the
		// number of branch-creation records observed for the new branch.
		branch.ContextRevision = uint64(event.Depth)
		if event.StateCheckpoint != nil {
			branch.State = cloneStoryState(event.StateCheckpoint)
		}
		branch.Plan = cloneBranchPlan(event.PlanCheckpoint)
		branch.PlanBeforeLatest = nil
		branch.TimeBeforeLatest = nil
		branch.CharactersBeforeLatest = nil
		branch.Characters = cloneCharacterState(event.CharacterCheckpoint)
		branch.Combats = cloneCombatSessions(event.CombatCheckpoint)
		branch.Clock = cloneStoryClock(event.ClockCheckpoint)
		branch.Recoveries = cloneRecoveryActivities(event.RecoveryCheckpoint)
		branch.Cultivations = cloneCultivationActivities(event.CultivationCheckpoint)
		branch.Journeys = cloneJourneySnapshots(event.JourneyCheckpoint)
		branch.LocalAffairs = cloneLocalAffairs(event.LocalAffairCheckpoint)
		branch.Realms = cloneRealmInstances(event.RealmCheckpoint)
		branch.Quests = cloneQuestInstances(event.QuestCheckpoint)
		branch.PublicEvents = clonePublicEventInstances(event.PublicEventCheckpoint)
		branch.NPCGrowth = cloneNPCGrowthSnapshots(event.NPCGrowthCheckpoint)
	case StoryEventTypeBranchHeadMoved:
		var event BranchHeadMovedEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.NextHead
		branch.LatestTurnID = event.NextLatestTurnID
		branch.Depth = event.NextDepth
		branch.State = cloneStoryState(event.StateCheckpoint)
		branch.StateBeforeLatest = nil
		branch.Plan = cloneBranchPlan(event.PlanCheckpoint)
		branch.PlanBeforeLatest = nil
		branch.TimeBeforeLatest = nil
		branch.CharactersBeforeLatest = nil
		branch.Characters = cloneCharacterState(event.CharacterCheckpoint)
		branch.Combats = cloneCombatSessions(event.CombatCheckpoint)
		branch.Clock = cloneStoryClock(event.ClockCheckpoint)
		branch.Recoveries = cloneRecoveryActivities(event.RecoveryCheckpoint)
		branch.Cultivations = cloneCultivationActivities(event.CultivationCheckpoint)
		branch.Journeys = cloneJourneySnapshots(event.JourneyCheckpoint)
		branch.LocalAffairs = cloneLocalAffairs(event.LocalAffairCheckpoint)
		branch.Realms = cloneRealmInstances(event.RealmCheckpoint)
		branch.Quests = cloneQuestInstances(event.QuestCheckpoint)
		branch.PublicEvents = clonePublicEventInstances(event.PublicEventCheckpoint)
		if event.NPCGrowthCheckpoint != nil || event.Reason == "regenerate_parent_checkpoint" {
			branch.NPCGrowth = cloneNPCGrowthSnapshots(event.NPCGrowthCheckpoint)
		}
	case StoryEventTypeTurnVersionSelected:
		var event TurnVersionSelectionEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		branch.Head = event.ProjectedHeadID
		branch.LatestTurnID = event.CurrentTurnID
		branch.Depth = event.CurrentDepth
		if event.CurrentState != nil {
			branch.State = cloneStoryState(event.CurrentState)
		}
		branch.StateBeforeLatest = nil
		branch.Plan = cloneBranchPlan(event.CurrentPlan)
		branch.PlanBeforeLatest = nil
		branch.TimeBeforeLatest = nil
		branch.CharactersBeforeLatest = nil
		// A version selection that carries no character checkpoint leaves memory
		// alone rather than clearing it: the absence of a checkpoint means the
		// selection did not move memory, not that the branch has no characters.
		if event.CurrentCharacters != nil {
			branch.Characters = cloneCharacterState(event.CurrentCharacters)
		}
		// Battles follow the same rule: a selection that says nothing about them
		// leaves the live fights where they are.
		if event.CurrentCombats != nil {
			branch.Combats = cloneCombatSessions(event.CurrentCombats)
		}
		// The growth watermark follows the same rule: a selection that says
		// nothing about it leaves the settled intervals where they are.
		if event.CurrentNPCGrowth != nil {
			branch.NPCGrowth = cloneNPCGrowthSnapshots(event.CurrentNPCGrowth)
		}
	case StoryEventTypeTurnDraft:
		var event TurnDraftEvent
		if err := mapToStruct(record.Raw, &event); err != nil {
			return err
		}
		projection.TurnDrafts[turnDraftKey(event.BranchID, event.Draft.Identity)] = storyDraftLocator{ID: event.ID, Cursor: cursor}
	case StoryEventTypeHotChoices,
		StoryEventTypeTurnNarrativeRevised, StoryEventTypeTurnDisplayAppended,
		StoryEventTypeStoryConfigUpdated, StoryEventTypeBranchSwitched, StoryEventTypeBranchArchived,
		StoryEventTypeCombatNarration:
		// Side/audit records do not independently advance branch state. A combat
		// narration is one of these: it is prose about an exchange whose settled
		// facts the paired combat record already carries, so it advances neither
		// the head nor the branch's state.
	default:
		return fmt.Errorf("story projection does not handle persisted event type %q", record.Envelope.Type)
	}
	if changesModelContext && record.Envelope.Type != StoryEventTypeBranch {
		branch.ContextRevision++
	}
	return nil
}

func (branch *storyBranchProjection) rememberPendingPlayerInput(playerInputID string) {
	playerInputID = strings.TrimSpace(playerInputID)
	if playerInputID == "" {
		return
	}
	for _, existing := range branch.PendingPlayerInputIDs {
		if existing == playerInputID {
			return
		}
	}
	branch.PendingPlayerInputIDs = append(branch.PendingPlayerInputIDs, playerInputID)
}

func (branch *storyBranchProjection) hasPendingPlayerInput(playerInputID string) bool {
	if branch == nil {
		return false
	}
	playerInputID = strings.TrimSpace(playerInputID)
	for _, pendingID := range branch.PendingPlayerInputIDs {
		if pendingID == playerInputID {
			return true
		}
	}
	return false
}

func (branch *storyBranchProjection) consumePlayerInputs(currentPlayerInputID string, consumedPlayerInputIDs []string) {
	if branch == nil || len(branch.PendingPlayerInputIDs) == 0 {
		return
	}
	consumed := make(map[string]bool, len(consumedPlayerInputIDs)+1)
	if currentPlayerInputID = strings.TrimSpace(currentPlayerInputID); currentPlayerInputID != "" {
		consumed[currentPlayerInputID] = true
	}
	for _, playerInputID := range consumedPlayerInputIDs {
		if playerInputID = strings.TrimSpace(playerInputID); playerInputID != "" {
			consumed[playerInputID] = true
		}
	}
	if len(consumed) == 0 {
		return
	}
	pending := branch.PendingPlayerInputIDs[:0]
	for _, playerInputID := range branch.PendingPlayerInputIDs {
		if !consumed[playerInputID] {
			pending = append(pending, playerInputID)
		}
	}
	if len(pending) == 0 {
		branch.PendingPlayerInputIDs = nil
		return
	}
	branch.PendingPlayerInputIDs = pending
}

func (projection *storyJournalProjection) branch(branchID string) *storyBranchProjection {
	branchID = strings.TrimSpace(branchID)
	branch := projection.Branches[branchID]
	if branch == nil {
		branch = &storyBranchProjection{State: initialStoryState()}
		projection.Branches[branchID] = branch
	}
	if branch.State == nil {
		branch.State = initialStoryState()
	}
	branch.bindWorld(projection.Meta)
	return branch
}

func (projection *storyJournalProjection) rememberCursor(cursor conversationjournal.Cursor) {
	if cursor == 0 || cursor == projection.lastCursor {
		return
	}
	projection.lastCursor = cursor
	projection.RecentCursors = append(projection.RecentCursors, cursor)
	if overflow := len(projection.RecentCursors) - storyRecentTransactionLimit; overflow > 0 {
		projection.RecentCursors = append([]conversationjournal.Cursor(nil), projection.RecentCursors[overflow:]...)
	}
}

func (projection *storyJournalProjection) rememberCommit(cursor conversationjournal.Cursor, eventType, eventID, branchID, commandID, operationID string, cycle int, hash string) {
	if strings.TrimSpace(commandID) == "" {
		return
	}
	projection.RecentCommits = append(projection.RecentCommits, storyCommitLocator{
		CommandID: strings.TrimSpace(commandID), OperationID: strings.TrimSpace(operationID), Cycle: cycle,
		BranchID: branchID, Hash: strings.TrimSpace(hash),
		EventID: eventID, EventType: eventType, Cursor: cursor,
	})
	if overflow := len(projection.RecentCommits) - storyRecentCommitLimit; overflow > 0 {
		projection.RecentCommits = append([]storyCommitLocator(nil), projection.RecentCommits[overflow:]...)
	}
}

func applyTurnState(state map[string]any, turn TurnEvent) {
	if turn.StateDelta != nil {
		applyStateDeltaToProjection(state, *turn.StateDelta)
	}
}

// normalizeLegacyFirstWaitClockOrigin corrects one historical projection defect
// without changing its canonical Turn record. Older wait commits could take
// their first clock anchor from the scene value written by that same Turn, even
// though replay applies the clock before the Turn's state delta. Accept that
// record only when both persisted clock anchors exactly match the Turn's own
// post-state scene; the in-memory projection then uses the scene at turn start.
func normalizeLegacyFirstWaitClockOrigin(branch *storyBranchProjection, turn *TurnEvent) bool {
	if branch == nil || branch.Clock != nil || turn == nil || turn.ClockAdvance == nil || turn.StateDelta == nil {
		return false
	}
	advance := turn.ClockAdvance
	postTurnState := cloneStoryState(branch.State)
	applyTurnState(postTurnState, *turn)
	postTurnClock := NewStoryClock(storySceneText(postTurnState, storySceneTimeField))
	if advance.Before != postTurnClock || advance.After.Version != postTurnClock.Version || advance.After.Anchor != postTurnClock.Anchor {
		return false
	}
	startClock := NewStoryClock(storySceneText(branch.State, storySceneTimeField))
	advance.Before.Anchor = startClock.Anchor
	advance.After.Anchor = startClock.Anchor
	return true
}

// applyStateDeltaToProjection is the single fold every replay path goes through.
//
// Besides the operations it carries, a delta records which turn wrote it and how
// far into the story the branch stood when it did. Those two values are stamped
// onto every Actor the delta touched, so a record written before a decade of
// seclusion can be read as a decade old rather than as a description of the
// world now. A delta from before those fields existed carries neither, and its
// records stay undated — which is the truth about them.
func applyStateDeltaToProjection(state map[string]any, delta StateDelta) {
	for _, op := range delta.Ops {
		applyStateOp(state, op)
	}
	for _, op := range delta.ActorOps {
		applyActorStateOp(state, op)
	}
	stampActorStateWrites(state, delta.ActorOps, delta.WrittenAtSeconds, delta.WrittenAtEvent)
}

// stampActorStateWrites records when each touched Actor was last written.
//
// It is keyed by Actor rather than by field: a field that this turn did not
// touch keeps the story's older text, but the record as a whole was last
// examined now, and that is what a narrator needs in order to decide whether
// the whole thing still describes the world.
func stampActorStateWrites(state map[string]any, actorOps []ActorStateOp, atSeconds int64, eventID string) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" || len(actorOps) == 0 {
		return
	}
	actors, _ := state[actorStateRoot].(map[string]any)
	if actors == nil {
		return
	}
	stamped := make(map[string]bool, len(actorOps))
	for _, op := range actorOps {
		actorID := normalizeStatePanelActorID(op.ActorID)
		if actorID == "" || stamped[actorID] {
			continue
		}
		stamped[actorID] = true
		record, _ := actors[actorID].(map[string]any)
		if record == nil {
			continue
		}
		record[actorWrittenAtSecondsField] = atSeconds
		record[actorWrittenAtEventField] = eventID
	}
}

func cloneStoryState(state map[string]any) map[string]any {
	if state == nil {
		return initialStoryState()
	}
	data, err := json.Marshal(state)
	if err != nil {
		return initialStoryState()
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil || cloned == nil {
		return initialStoryState()
	}
	return cloned
}

// cloneCombatSessions deep-copies the live battles for a checkpoint.
//
// A fork or a rewind must get its own copy for the same reason character memory
// does, but there is a sharper one here: a battle carries mutable combatants,
// so two branches sharing one session value would resolve their exchanges
// against each other's wounds. The copy is also what keeps a fork consistent —
// its state checkpoint already carries the settled health, so it must carry the
// battle that produced it, or the window would show a wounded character with no
// fight behind the wound.
func cloneCombatSessions(sessions map[string]cultivation.CombatSession) map[string]cultivation.CombatSession {
	if len(sessions) == 0 {
		return nil
	}
	cloned := make(map[string]cultivation.CombatSession, len(sessions))
	for id, session := range sessions {
		cloned[id] = cloneCombatSession(session)
	}
	return cloned
}

// cloneCombatSession deep-copies one battle, including the combatant slices and
// the preparation map, so a resolved exchange on one branch cannot write through
// into another.
func cloneCombatSession(session cultivation.CombatSession) cultivation.CombatSession {
	cloned := session
	cloned.Combatants = append([]cultivation.Combatant(nil), session.Combatants...)
	for index := range cloned.Combatants {
		cloned.Combatants[index].Skills = append([]cultivation.SkillDef(nil), session.Combatants[index].Skills...)
	}
	if len(session.Preparations) > 0 {
		cloned.Preparations = make(map[string]cultivation.Preparation, len(session.Preparations))
		for actorID, preparation := range session.Preparations {
			cloned.Preparations[actorID] = preparation
		}
	}
	return cloned
}

// cloneCharacterState deep-copies a character projection for a checkpoint. A
// branch that forks or rewinds must get its own copy: two branches that shared
// one backing array would silently rewrite each other's memory.
func cloneCharacterState(state *character.State) *character.State {
	if state == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return nil
	}
	var cloned character.State
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	normalized := cloned.Normalize()
	return &normalized
}

func storyJournalGeneration(meta StoryMeta) string {
	return "story:" + strings.TrimSpace(meta.StoryID) + ":" + strings.TrimSpace(meta.CreatedAt)
}
