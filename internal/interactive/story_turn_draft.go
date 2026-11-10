package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ta/internal/agents/conversationjournal"
	"ta/internal/agents/sessionjournal"

	agent "ta/agent"
)

const maxTurnDraftBytes = 16 << 20

// TurnResult omits settlement inputs from published turns. Unfinished drafts
// must retain them until the final transaction writes their authoritative records.
type turnDraftActionModules struct {
	CharacterFacts *TurnCharacterFactsInput `json:"character_facts,omitempty"`
	Cultivation    *TurnCultivationInput    `json:"cultivation,omitempty"`
	Skills         *TurnSkillLearningInput  `json:"skills,omitempty"`
	Techniques     *TurnTechniqueInput      `json:"techniques,omitempty"`
	Items          *TurnItemsInput          `json:"items,omitempty"`
	Trade          *TurnTradeInput          `json:"trade,omitempty"`
	Rest           *TurnRestInput           `json:"rest,omitempty"`
	Craft          *TurnCraftInput          `json:"craft,omitempty"`
	World          *TurnWorldActionInput    `json:"world,omitempty"`
}

// TurnSubmissionProgress is the product-owned acceptance state, including a
// partially accepted plan document. It contains no validator or runtime object.
type TurnSubmissionProgress struct {
	Modules              *turnDraftActionModules `json:"modules,omitempty"`
	Result               TurnResult              `json:"result"`
	PlanUpdate           *string                 `json:"plan_update,omitempty"`
	StateChangesAccepted bool                    `json:"state_changes_accepted"`
	ChoicesAccepted      bool                    `json:"choices_accepted"`
	PlanUpdateAccepted   bool                    `json:"plan_update_accepted"`
	PlanUpdateStarted    bool                    `json:"plan_update_started"`
	// CharacterFactsAccepted is part of the durable draft because the module is
	// optional: an absent module and a rejected one are different states, and a
	// draft saved between tool calls has to remember which it was.
	CharacterFactsAccepted bool `json:"character_facts_accepted"`
	// CultivationAccepted is durable for the same reason as character facts: the
	// module is optional, so an absent one and a rejected one must stay
	// distinguishable across a reload mid-submission.
	CultivationAccepted bool `json:"cultivation_accepted"`
	// SkillsAccepted is durable for the same reason again: a turn that taught
	// nothing and a turn whose teaching was rejected must stay distinguishable
	// across a reload mid-submission.
	SkillsAccepted bool `json:"skills_accepted"`
	// TechniquesAccepted is durable for the same reason: a turn that studied or
	// switched no method and a turn whose method module was rejected must stay
	// distinguishable across a reload mid-submission.
	TechniquesAccepted bool `json:"techniques_accepted"`
	// ItemsAccepted is durable for the same reason: a turn that obtained and
	// equipped nothing and a turn whose item module was rejected must stay
	// distinguishable across a reload mid-submission.
	ItemsAccepted bool `json:"items_accepted"`
	// TradeAccepted is durable for the same reason: a turn that bought and sold
	// nothing and a turn whose trade module was rejected must stay
	// distinguishable across a reload mid-submission.
	TradeAccepted bool `json:"trade_accepted"`
	// RestAccepted is durable for the same reason: a turn that spent no time
	// resting and a turn whose rest module was rejected must stay
	// distinguishable across a reload mid-submission.
	RestAccepted bool `json:"rest_accepted"`
	// CraftAccepted is durable for the same reason: a turn that worked no craft
	// and a turn whose craft module was rejected must stay distinguishable across
	// a reload mid-submission. Without it a reloaded draft would come back with
	// the craft module still marked missing and the turn would never become ready.
	CraftAccepted bool `json:"craft_accepted"`
	// WorldAccepted is durable for the same reason: a turn that settled no
	// journey, task, event or realm and a turn whose world module was rejected
	// must stay distinguishable across a reload mid-submission. Without it a
	// draft saved between tool calls would come back with the world module still
	// marked missing and the turn would never become ready.
	WorldAccepted bool `json:"world_accepted"`
	// Recap is the turn's own short recollection. It has no accepted flag: an
	// absent recap is not a module awaiting repair, it is display material the
	// history falls back to a prose excerpt for.
	Recap *string `json:"recap,omitempty"`
}

func (prepared *PreparedTurnSubmission) Progress() *TurnSubmissionProgress {
	if prepared == nil {
		return nil
	}
	result := prepared.TurnResult()
	modules := &turnDraftActionModules{CharacterFacts: result.CharacterFacts, Cultivation: result.Cultivation, Skills: result.Skills, Techniques: result.Techniques, Items: result.Items, Trade: result.Trade, Rest: result.Rest, Craft: result.Craft, World: result.World}
	return &TurnSubmissionProgress{Result: result, Modules: modules, PlanUpdate: cloneStringPointer(prepared.result.PlanUpdate), StateChangesAccepted: prepared.stateUpdatesAccepted,
		ChoicesAccepted: prepared.choicesAccepted, PlanUpdateAccepted: prepared.planUpdateAccepted, PlanUpdateStarted: prepared.planUpdateStarted,
		CharacterFactsAccepted: prepared.characterFactsAccepted, CultivationAccepted: prepared.cultivationAccepted,
		SkillsAccepted: prepared.skillsAccepted, TechniquesAccepted: prepared.techniquesAccepted,
		ItemsAccepted: prepared.itemsAccepted, TradeAccepted: prepared.tradeAccepted,
		RestAccepted: prepared.restAccepted, CraftAccepted: prepared.craftAccepted,
		WorldAccepted: prepared.worldAccepted, Recap: cloneStringPointer(prepared.recap)}
}

func (progress *TurnSubmissionProgress) Prepared() *PreparedTurnSubmission {
	if progress == nil {
		return nil
	}
	result := progress.Result
	if progress.Modules != nil {
		result.CharacterFacts = progress.Modules.CharacterFacts
		result.Cultivation = progress.Modules.Cultivation
		result.Skills = progress.Modules.Skills
		result.Techniques = progress.Modules.Techniques
		result.Items = progress.Modules.Items
		result.Trade = progress.Modules.Trade
		result.Rest = progress.Modules.Rest
		result.Craft = progress.Modules.Craft
		result.World = progress.Modules.World
	}
	result.PlanUpdate = cloneStringPointer(progress.PlanUpdate)
	return clonePreparedTurnSubmission(&PreparedTurnSubmission{result: result, stateUpdatesAccepted: progress.StateChangesAccepted,
		choicesAccepted: progress.ChoicesAccepted, planUpdateAccepted: progress.PlanUpdateAccepted, planUpdateStarted: progress.PlanUpdateStarted,
		characterFactsAccepted: progress.CharacterFactsAccepted, cultivationAccepted: progress.CultivationAccepted,
		skillsAccepted: progress.SkillsAccepted, techniquesAccepted: progress.TechniquesAccepted,
		itemsAccepted: progress.ItemsAccepted, tradeAccepted: progress.TradeAccepted,
		restAccepted: progress.RestAccepted, craftAccepted: progress.CraftAccepted,
		worldAccepted: progress.WorldAccepted, recap: cloneStringPointer(progress.Recap)})
}

// TurnDraft is the unfinished product turn. A final Turn with the same exact
// identity ends it in that Turn's transaction; draft text never becomes Actor
// State or official story history by itself.
type TurnDraft struct {
	Identity        DomainCommitIdentity    `json:"identity"`
	Narrative       string                  `json:"narrative,omitempty"`
	NarrativeSource string                  `json:"narrative_source,omitempty"`
	Submission      *TurnSubmissionProgress `json:"submission,omitempty"`
	RuleResolution  *RuleResolution         `json:"rule_resolution,omitempty"`
	DisplayEvents   []DisplayEvent          `json:"display_events,omitempty"`
	// RefusedNarrativeHash is the digest of prose this turn has already
	// discarded because the world refused the action it narrated. The run loop
	// and the protocol middleware both see the same candidate again after the
	// refusal, and neither may accept it: the turn would then publish the
	// refused success while the world stayed put.
	RefusedNarrativeHash string `json:"refused_narrative_hash,omitempty"`
}

type TurnDraftEvent struct {
	V        int       `json:"v"`
	Type     string    `json:"type"`
	ID       string    `json:"id"`
	ParentID string    `json:"parent_id,omitempty"`
	BranchID string    `json:"branch_id"`
	Ts       string    `json:"ts"`
	Draft    TurnDraft `json:"draft"`
}

type storyDraftLocator struct {
	ID     string                     `json:"id"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

func turnDraftKey(branchID string, identity DomainCommitIdentity) string {
	return branchID + ":" + deterministicPlayerInputID(identity)
}

// RefusedNarrativeHash is the digest a draft stores when it discards prose.
// The run loop and the protocol middleware both see the candidate again after
// the refusal, and comparing digests is what lets them recognise it without
// keeping a second copy of the text.
func RefusedNarrativeHash(narrative string) string {
	if strings.TrimSpace(narrative) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(narrative))
	return hex.EncodeToString(sum[:])
}

func validateTurnDraft(draft TurnDraft) error {
	if draft.Identity.CommandID == "" || draft.Identity.OperationID == "" || draft.Identity.Cycle <= 0 {
		return errors.New("turn draft requires an exact Agent cycle identity")
	}
	if draft.Narrative != "" && draft.NarrativeSource == "" {
		return errors.New("turn draft narrative requires a source")
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	if len(encoded) > maxTurnDraftBytes {
		return fmt.Errorf("turn draft exceeds the %d-byte product limit", maxTurnDraftBytes)
	}
	return nil
}

// SaveTurnDraft confirms product progress and the Agent acceptance facts in
// one journal transaction, before returning acceptance to a model or tool.
func (s *Store) SaveTurnDraft(storyID, branchID string, draft TurnDraft, checkpoint agent.CanonicalCheckpoint) error {
	if err := validateTurnDraft(draft); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return err
	}
	defer release()
	meta, _, err := s.readStoryRecentLocked(storyID, branchID)
	if err != nil {
		return err
	}
	branch, found := meta.Branches[branchID]
	if !found {
		return errors.New("turn draft branch is unavailable")
	}
	handle := s.storyJournals[storyID]
	projection := handle.projection.branch(branchID)
	if !projection.hasPendingPlayerInput(deterministicPlayerInputID(draft.Identity)) {
		return errors.New("turn draft has no pending canonical player input")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	event := TurnDraftEvent{V: schemaVersion, Type: StoryEventTypeTurnDraft, ID: newID("draft"), ParentID: branch.Head, BranchID: branchID, Ts: now, Draft: draft}
	meta.UpdatedAt = now
	agentRecords, err := sessionjournal.CheckpointRecords(&handle.projection.AgentSessions, checkpoint, event.ID)
	if err != nil {
		return err
	}
	return s.appendStoryTransactionLocked(storyID, meta, append([]any{event}, agentRecords...)...)
}

// LoadTurnDraft uses a rebuildable locator, retaining every unfinished draft
// regardless of the recent-turn window. It never executes a submission tool.
func (s *Store) LoadTurnDraft(storyID, branchID string, identity DomainCommitIdentity) (TurnDraft, bool, error) {
	if strings.TrimSpace(identity.CommandID) == "" {
		return TurnDraft{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadTurnDraftLocked(storyID, branchID, identity)
}

func (s *Store) loadTurnDraftLocked(storyID, branchID string, identity DomainCommitIdentity) (TurnDraft, bool, error) {
	handle, err := s.openStoryJournalLocked(storyID)
	if err != nil {
		return TurnDraft{}, false, err
	}
	if _, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: handle.journal.Head().Cursor}); err != nil {
		return TurnDraft{}, false, err
	}
	locator, found := handle.projection.TurnDrafts[turnDraftKey(branchID, identity)]
	if !found {
		return TurnDraft{}, false, nil
	}
	records, err := handle.journal.ReadRange(context.Background(), conversationjournal.Range{After: locator.Cursor - 1, Through: locator.Cursor})
	if err != nil {
		return TurnDraft{}, false, err
	}
	for _, record := range records {
		var event TurnDraftEvent
		if err := json.Unmarshal(record.Payload, &event); err != nil {
			return TurnDraft{}, false, err
		}
		if event.Type == StoryEventTypeTurnDraft && event.ID == locator.ID {
			if event.Draft.Identity != identity || event.BranchID != branchID {
				return TurnDraft{}, false, errors.New("turn draft identity mismatch")
			}
			return event.Draft, true, validateTurnDraft(event.Draft)
		}
	}
	return TurnDraft{}, false, errors.New("turn draft locator has no canonical record")
}
