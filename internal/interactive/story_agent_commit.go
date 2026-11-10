package interactive

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agent "ta/agent"

	"ta/internal/agents/sessionjournal"
	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

// ErrAgentTurnIdentityConflict means one durable Agent command attempted to
// commit two different game turns. Returning the original turn for an exact
// retry is safe; accepting a different payload under the same command ID is
// not.
var ErrAgentTurnIdentityConflict = errors.New("agent turn identity conflict")

// DomainCommitIdentity is shared with the durable Agent coordinator without
// coupling story storage to the runtime package.
type DomainCommitIdentity struct {
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
	Cycle       int    `json:"cycle"`
}

type DomainCommitIntent struct {
	Identity DomainCommitIdentity
	Hash     string
	Request  AppendTurnWithStateRequest
}

type DomainCommitReceipt struct {
	Identity DomainCommitIdentity `json:"identity"`
	Hash     string               `json:"hash"`
	Revision string               `json:"revision"`
	Turn     TurnEvent            `json:"turn"`
	Delta    *StateDeltaEvent     `json:"delta,omitempty"`
}

func NewDomainCommitIntent(req AppendTurnWithStateRequest) (DomainCommitIntent, error) {
	providerContinuation, err := normalizeProviderContinuation(req.ProviderContinuation)
	if err != nil {
		return DomainCommitIntent{}, err
	}
	req.ProviderContinuation = providerContinuation
	identity := DomainCommitIdentity{
		CommandID:   strings.TrimSpace(req.AgentCommandID),
		OperationID: strings.TrimSpace(req.AgentOperationID),
		Cycle:       req.AgentCycle,
	}
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 {
		return DomainCommitIntent{}, fmt.Errorf("%w: command_id, operation_id, and positive cycle are required together", ErrAgentTurnIdentityConflict)
	}
	hash, err := agentTurnRequestHash(req)
	if err != nil {
		return DomainCommitIntent{}, err
	}
	return DomainCommitIntent{Identity: identity, Hash: hash, Request: req}, nil
}

// CommitDomainTurn publishes the staged game turn through the canonical story
// store. Turn ID is the native append-only revision returned to the actor.
func (s *Store) CommitDomainTurn(storyID string, intent DomainCommitIntent) (DomainCommitReceipt, error) {
	canonical, err := NewDomainCommitIntent(intent.Request)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	if canonical.Identity != intent.Identity || canonical.Hash != strings.TrimSpace(intent.Hash) {
		return DomainCommitReceipt{}, fmt.Errorf("%w: staged intent identity or hash changed", ErrAgentTurnIdentityConflict)
	}
	turn, delta, err := s.AppendTurnWithState(storyID, intent.Request)
	if err != nil {
		return DomainCommitReceipt{}, err
	}
	return DomainCommitReceipt{
		Identity: canonical.Identity, Hash: canonical.Hash,
		Revision: turn.ID, Turn: turn, Delta: delta,
	}, nil
}

// AppendTurnWithState atomically publishes one canonical turn and its state
// delta. Durable Agent identities make the write idempotent and allow an
// ambiguous post-rename filesystem error to reconcile the exact committed
// turn without executing the cycle twice.
func (s *Store) AppendTurnWithState(storyID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, error) {
	providerContinuation, err := normalizeProviderContinuation(req.ProviderContinuation)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	req.ProviderContinuation = providerContinuation
	s.mu.Lock()
	defer s.mu.Unlock()
	releaseStory, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	defer releaseStory()

	meta, lines, err := s.readStoryRecentLocked(storyID, req.BranchID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	lines, err = projectStoryEventOverlays(lines)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	branchID := req.BranchID
	if branchID == "" {
		branchID = meta.CurrentBranch
	}
	branch, ok := meta.Branches[branchID]
	if !ok {
		return TurnEvent{}, nil, fmt.Errorf("分支不存在: %s", branchID)
	}
	branchProjection, err := s.storyBranchProjectionLocked(storyID, branchID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	branchProjection.PackCatalog = ClonePackCatalogSnapshot(meta.PackCatalog)
	branchProjection.bindWorld(meta)
	if req.PlayerLetter != nil {
		intent := req.PlayerLetter.Normalize()
		if intent.CharacterID == "" || intent.Body == "" || len([]rune(intent.Body)) > 4000 {
			return TurnEvent{}, nil, ErrInvalidPlayerLetter
		}
		var gameplay *StoryGameplayProfile
		if meta.PackBinding != nil {
			gameplay = meta.PackBinding.Gameplay
		}
		if gameplay == nil || gameplay.Communication != "letter" || gameplay.LetterDeliverySeconds < 24*60*60 || gameplay.LetterDeliverySeconds > 30*24*60*60 {
			return TurnEvent{}, nil, ErrPlayerLetterUnavailable
		}
		if _, exists := branchProjection.Characters.Profile(intent.CharacterID); !exists {
			return TurnEvent{}, nil, ErrPlayerLetterRecipientUnavailable
		}
		req.PlayerLetter = &intent
	}
	if existing, delta, found, err := committedAgentTurnForRequest(lines, branchID, req); err != nil {
		return TurnEvent{}, nil, err
	} else if found {
		s.syncStoryIndexProjectionLocked(storyID)
		return existing, delta, nil
	}
	if req.TurnResult != nil && req.TurnResult.World != nil && req.TurnResult.World.Action == TurnWorldActionLocalAffair && ((req.TurnResult.Rest != nil && !req.TurnResult.Rest.Empty()) || (req.TurnResult.Cultivation != nil && !req.TurnResult.Cultivation.Empty()) || (req.TurnResult.Craft != nil && !req.TurnResult.Craft.Empty())) {
		return TurnEvent{}, nil, fmt.Errorf("local affair cannot share a turn with rest, cultivation or craft")
	}
	if factionModuleConflict(req.TurnResult) {
		return TurnEvent{}, nil, fmt.Errorf("faction production cannot share a turn with inventory, trade or learning modules")
	}
	if techniqueIntervalConflict(req.TurnResult) {
		return TurnEvent{}, nil, fmt.Errorf("technique study cannot share a turn with another timed activity")
	}
	playerInput, hasPlayerInput, err := playerInputForTurnRequest(lines, branchID, req)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	if req.TurnResult != nil && req.TurnResult.World != nil && (strings.TrimSpace(req.TurnResult.World.Action) == TurnWorldActionWait || strings.TrimSpace(req.TurnResult.World.Action) == "adventure" || strings.TrimSpace(req.TurnResult.World.Action) == TurnWorldActionCooperation || strings.TrimSpace(req.TurnResult.World.Action) == TurnWorldActionFaction || localAffairRequiresSource(req.TurnResult.World)) {
		if (req.TurnResult.Rest != nil && !req.TurnResult.Rest.Empty()) ||
			(req.TurnResult.Cultivation != nil && !req.TurnResult.Cultivation.Empty()) ||
			(req.TurnResult.Craft != nil && !req.TurnResult.Craft.Empty()) {
			return TurnEvent{}, nil, fmt.Errorf("wait action cannot share a turn with rest, cultivation or craft")
		}
		playerText := req.User
		if hasPlayerInput {
			playerText = playerInput.Text
		}
		if source := strings.TrimSpace(req.TurnResult.World.SourceText); source == "" || !strings.Contains(playerText, source) {
			return TurnEvent{}, nil, fmt.Errorf("wait action source must quote the accepted player input")
		}
	}
	if strings.TrimSpace(req.AgentCommandID) != "" && !hasPlayerInput {
		return TurnEvent{}, nil, fmt.Errorf("%w: canonical player input is missing for completed turn", ErrPlayerInputIdentityConflict)
	}
	var consumedPlayerInputIDs []string
	var resolvedPlayerInputContexts []ResolvedPlayerInputContext
	if hasPlayerInput {
		_, activeAncestry, ancestryErr := walkEventPath(branch.Head, eventsByID(lines))
		if ancestryErr != nil {
			return TurnEvent{}, nil, ancestryErr
		}
		pendingInputs, pendingErr := pendingPlayerInputsForBranch(lines, branchID, activeAncestry)
		if pendingErr != nil {
			return TurnEvent{}, nil, pendingErr
		}
		pendingInputs = pendingPlayerInputsFromProjection(pendingInputs, branchProjection.PendingPlayerInputIDs)
		consumedPlayerInputIDs = make([]string, 0, len(pendingInputs))
		currentInputIncluded := false
		for _, pending := range pendingInputs {
			consumedPlayerInputIDs = append(consumedPlayerInputIDs, pending.ID)
			currentInputIncluded = currentInputIncluded || pending.ID == playerInput.ID
		}
		if !currentInputIncluded {
			consumedPlayerInputIDs = append(consumedPlayerInputIDs, playerInput.ID)
		}
		resolvedPlayerInputContexts, err = resolveHistoricalPlayerInputContexts(lines, branchID, pendingInputs, playerInput.ID)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		// Only the current cycle's tool suffix belongs to this Turn. Historical
		// pending cycles retain their own acceptance position and evidence in
		// ResolvedPlayerInputContexts instead of being moved to this later Turn.
		req.ModelContextMessages, err = mergeModelContextBatchesForPlayerInput(lines, branchID, playerInput.ID, req.ModelContextMessages)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if req.ExpectedParentID != nil && branch.Head != strings.TrimSpace(*req.ExpectedParentID) {
		return TurnEvent{}, nil, fmt.Errorf("%w: 当前分支已前进，拒绝提交基于旧版本的回合: expected_parent=%s current_head=%s", ErrStoryContextRevisionConflict, strings.TrimSpace(*req.ExpectedParentID), branch.Head)
	}
	commitParentID := branch.Head
	if replaceTurnID := strings.TrimSpace(req.ReplaceTurnID); replaceTurnID != "" {
		if err := requireLatestLogicalTurn(meta, lines, branchID, replaceTurnID); err != nil {
			return TurnEvent{}, nil, err
		}
		commitParentID, err = regenerationParentOnCurrentPath(lines, branch.Head, replaceTurnID)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if branchIsTerminal(lines, commitParentID) {
		return TurnEvent{}, nil, fmt.Errorf("当前分支已终局，请从历史回合创建新分支后继续")
	}
	parentID := any(nil)
	if commitParentID != "" {
		parentID = commitParentID
	}
	state := cloneStoryState(branchProjection.State)
	// A replacement turn starts from its parent, so everything it reads about
	// the committed story must be the position as it stood there. The live
	// projection has already replayed the very turn being replaced — its walk,
	// its task, its participation, the interval it trained or rested — and
	// starting from that would let the old turn's success be the permission for
	// the new one's, and would spend its time a second time.
	//
	// The clock is the sharp end of this: a training, resting or crafting turn
	// publishes its interval record after itself, so the live clock already
	// includes the interval being replaced. Regenerating one thirty-day
	// seclusion from the live clock would leave the branch sixty days older than
	// the story it tells, and every instant derived from it — a task deadline, a
	// lifespan edge, an NPC's growth watermark — would move with it. The
	// recoveries, cultivations and growth watermarks are measured in that same
	// clock, so they rewind with it or the branch would deny time it kept.
	parentProjection := branchProjection
	var regenerationRestore *BranchHeadMovedEvent
	if strings.TrimSpace(req.ReplaceTurnID) != "" {
		state = cloneStoryState(branchProjection.StateBeforeLatest)
		parentPath, parentAncestors, err := walkEventPath(commitParentID, eventsByID(lines))
		if err != nil {
			return TurnEvent{}, nil, err
		}
		parentTime, err := storyTimeFromRecords(parentPath, meta)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		parentCombats, err := combatsFromPath(parentPath)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		parentProjection = &storyBranchProjection{
			Head:             commitParentID,
			Combats:          parentCombats,
			Characters:       cloneCharacterState(branchProjection.CharactersBeforeLatest),
			State:            state,
			PackCatalog:      ClonePackCatalogSnapshot(meta.PackCatalog),
			WorldContent:     worldContentForMeta(meta),
			CultivationRules: cultivationRulesForMeta(meta),
			WorldPackID:      branchProjection.WorldPackID,
			Clock:            parentTime.Clock,
			Recoveries:       parentTime.Recoveries,
			Cultivations:     parentTime.Cultivations,
			Journeys:         parentTime.Journeys,
			Realms:           parentTime.Realms,
			Quests:           parentTime.Quests,
			PublicEvents:     parentTime.PublicEvents,
			LocalAffairs:     cloneLocalAffairs(parentTime.LocalAffairs),
			NPCGrowth:        parentTime.NPCGrowth,
		}
		if branchProjection.TimeBeforeLatest == nil {
			parentProjection, regenerationRestore, err = s.rebuildRegenerationParentLocked(storyID, branchID, branchProjection.Head, parentProjection, parentPath, parentAncestors)
			if err != nil {
				return TurnEvent{}, nil, err
			}
			state = cloneStoryState(parentProjection.State)
		}
		for _, record := range parentPath {
			if record.Envelope.Type == StoryEventTypeTurn {
				parentProjection.LatestTurnID = record.Envelope.ID
			}
		}
	}
	director := s.storyDirectorForMeta(meta)
	actorState := actorStateSystemFromSnapshot(meta.ActorStateSchema, director.ActorState)
	applyLegacyActorStateAliases(state, meta.ActorStateSchema)
	// A live battle is settled by the combat window's rounds, not by prose, so the
	// main story may not advance through one. The declaration is read from the
	// state this turn starts from, which is the parent state for a replacement
	// turn: the turn that declares a fight is not refused by its own declaration,
	// and a retry of an already committed turn is answered above rather than here.
	if actorState.AdventureRules != nil {
		if a, err := AdventureActorFromState(state, DefaultActorID); err == nil && a.Battle != nil && a.Battle.Status == "active" {
			return TurnEvent{}, nil, fmt.Errorf("adventure battle active: use the battle window before continuing the story")
		}
	}
	if err := StoryTurnBlockedByBattle(state, actorState, branchProjection.Combats); err != nil {
		return TurnEvent{}, nil, fmt.Errorf("%w：这场交手已经开打，请在战斗窗口里逐回合进行；打完再继续推进主故事", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	terminal := (req.TerminalOutcome != nil && req.TerminalOutcome.Terminal) || (req.RuleResolution != nil && req.RuleResolution.TerminalCandidate != nil)
	turnResult := normalizeTurnResultPointer(req.TurnResult, meta.ChoiceCount, terminal)
	if req.TurnResult != nil && turnResult == nil {
		return TurnEvent{}, nil, fmt.Errorf("TurnResult 未通过校验")
	}
	agentCommitHash := ""
	if strings.TrimSpace(req.AgentCommandID) != "" {
		agentCommitHash, err = agentTurnRequestHash(req)
		if err != nil {
			return TurnEvent{}, nil, err
		}
	}
	turn := TurnEvent{
		V:                           schemaVersion,
		Type:                        StoryEventTypeTurn,
		ID:                          newID("ev"),
		ParentID:                    parentID,
		BranchID:                    branchID,
		Ts:                          now,
		User:                        req.User,
		Attachments:                 append([]agent.Attachment(nil), playerInput.Attachments...),
		UserContextOnly:             playerInput.ContextOnly,
		Narrative:                   req.Narrative,
		Thinking:                    strings.TrimSpace(req.Thinking),
		Recap:                       strings.TrimSpace(req.Recap),
		RunID:                       strings.TrimSpace(req.RunID),
		AgentKind:                   strings.TrimSpace(req.AgentKind),
		AgentCommandID:              strings.TrimSpace(req.AgentCommandID),
		AgentOperationID:            strings.TrimSpace(req.AgentOperationID),
		AgentCycle:                  req.AgentCycle,
		AgentCommitHash:             agentCommitHash,
		PlayerInputID:               playerInput.ID,
		PlayerInputHash:             playerInput.AgentCommitHash,
		ConsumedPlayerInputIDs:      append([]string(nil), consumedPlayerInputIDs...),
		ResolvedPlayerInputContexts: cloneResolvedPlayerInputContexts(resolvedPlayerInputContexts),
		DisplayEvents:               sanitizeDisplayEvents(req.DisplayEvents),
		ModelContextMessages:        sanitizeModelContextMessages(req.ModelContextMessages),
		RuleResolution:              normalizeRuleResolutionPointer(req.RuleResolution),
		TurnResult:                  turnResult,
		TerminalOutcome:             normalizeTerminalOutcomePointer(req.TerminalOutcome),
		Flags:                       map[string]bool{"pinned": false, "locked": false},
	}
	// The position this turn starts from, kept until the delta is built. The
	// summary the player reads is the difference between this and the settled
	// result, and that difference cannot be recovered afterwards: the live state
	// has moved by then, so 灵石=148 no longer says whether the turn spent two
	// stones or none. Captured before the opening bootstrap writes, because an
	// opening turn's own seeding is not a change the player made.
	turnStartState := cloneStoryState(state)
	actorState, openingOps, openingActorOps, err := prepareOpeningGameStateSchemaCommit(&meta, lines, state, actorState, branchID, turn.ID, now, req.StateSchemaProposal)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	ops := normalizeStateOps(append(openingOps, req.Ops...))
	actorOps := normalizeActorStateOps(append(openingActorOps, req.ActorOps...))
	if turn.TurnResult != nil && (len(turn.TurnResult.StateUpdates) > 0 || turn.TurnResult.CharacterFacts != nil) {
		// A person the turn names through character_facts may already exist under
		// another actor id. Retargeting the turn's own writes before compiling is
		// what keeps the numbers and the prose on one Actor: the profile rebind
		// below uses the same alias table.
		updates := rebindTurnStateUpdatesToEstablishedActors(actorState, state, turn.TurnResult.StateUpdates, turnCharacterAliases(turn.TurnResult.CharacterFacts, parentProjection))
		restActorID, restPoolField := "", ""
		if turn.TurnResult.Rest != nil && !turn.TurnResult.Rest.Empty() {
			restActorID, restPoolField = turnRestPoolOwner(turn.TurnResult.Rest)
		}
		compiled, err := CompileTurnStateUpdates(actorState, state, updates, TurnStateUpdateCompileOptions{
			SourceTurnID:             turn.ID,
			RuleResolution:           turn.RuleResolution,
			RuleStateConsumptionMode: director.Strategy.RuleStateConsumptionMode,
			RestActorID:              restActorID,
			RestPoolField:            restPoolField,
			WorldOwnsSceneLocation:   turnWorldOwnsSceneLocation(turn.TurnResult.World),
			WorldSettlesMovement:     turnWorldSettlesMovement(turn.TurnResult.World),
		})
		if err != nil {
			return TurnEvent{}, nil, fmt.Errorf("TurnResult state_updates 校验失败: %w", err)
		}
		var knownCharacters []string
		if parentProjection.Characters != nil {
			knownCharacters = parentProjection.Characters.CharacterIDs()
		}
		if diagnostic := combatCompanionSubmissionDiagnostic(actorState, state, compiled, turn.TurnResult.CharacterFacts, knownCharacters); diagnostic != nil {
			return TurnEvent{}, nil, fmt.Errorf("combat companion declaration rejected: %s", diagnostic.Message)
		}
		turn.TurnResult.StateUpdates = compiled.Updates
		for i := range compiled.Ops {
			compiled.Ops[i].SourceKind = interactivestate.SourceTurnResult
			compiled.Ops[i].SourceID = turn.ID
			compiled.Ops[i].SourceTurnID = turn.ID
		}
		ops = append(ops, compiled.Ops...)
		for i := range compiled.ActorOps {
			compiled.ActorOps[i].SourceKind = interactivestate.SourceTurnResult
			compiled.ActorOps[i].SourceID = turn.ID
			compiled.ActorOps[i].SourceTurnID = turn.ID
		}
		actorOps = append(actorOps, compiled.ActorOps...)
	}
	// The settlement runs last, over the state this turn has already produced. It
	// has to see the stage the turn ended at rather than the one it started from:
	// a turn that advances the character's cultivation must show the new stage's
	// numbers, and settling before the rule ops would publish the previous
	// stage's attributes alongside the new stage key.
	//
	// Rule consumption applies its own operations to the live state in place, so
	// the projection base is snapshotted first and the turn's full operation set
	// is replayed onto it. Settling against the live map instead would apply the
	// rule operations a second time.
	settlementBase := cloneStoryState(state)
	if turn.RuleResolution != nil {
		ruleOps, ruleActorOps := applyRuleStateConsumptionV2(state, actorState, turn.ID, turn.RuleResolution, director.Strategy.RuleStateConsumptionMode)
		ops = append(ops, ruleOps...)
		actorOps = append(actorOps, ruleActorOps...)
	}
	// The authorised cultivation interval runs before the settlement, because the
	// settlement derives the displayed numbers from the stage this writes. Running
	// it after would publish the previous stage's attributes alongside the new
	// stage key.
	//
	// It is applied to a projection of the turn's own operations rather than to
	// the live map, so the interval starts from the position the turn actually
	// left the character in without any operation being applied twice.
	//
	// The interval is also the turn's formal time. A Turn carries no clock of its
	// own — its projection cannot move one — so a turn that trains must publish
	// the interval it really spent as a cultivation side record, or the story
	// would grow the character without the world moving. The record is written
	// after the turn below, and it reuses the operations computed here rather
	// than advancing a second time.
	var turnAdvance *CultivationAdvanceResult
	// The interval a continuation owes is decided here rather than by the caller.
	// A further training turn continues the activity a real boundary paused,
	// settling no more than the remainder that activity still holds; the module's
	// own days are the training the player asked for in this turn and are capped
	// by that remainder. A turn with no paused activity is an ordinary new
	// interval, cut at the first committed boundary inside it.
	var turnCultivation turnCultivationPlan
	if turn.TurnResult != nil && turn.TurnResult.Cultivation != nil && !turn.TurnResult.Cultivation.Empty() {
		advanceActorID := normalizeStatePanelActorID(turn.TurnResult.Cultivation.ActorID)
		if advanceActorID == "" {
			advanceActorID = DefaultActorID
		}
		turnCultivation = planTurnCultivation(parentProjection, advanceActorID, turn.TurnResult.Cultivation, lifespanBoundary(meta, settlementBase, advanceActorID))
		input := turnCultivation.input
		advanceBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(advanceBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(advanceBase, op)
		}
		advanceOps, advance, ok := advanceXiuxianCultivation(advanceBase, actorState, advanceActorID, input, xiuxianFormalFactsFromBranch(parentProjection, advanceActorID), turnIdentity(turn), turn.ID)
		if advance.RefusalReason != "" {
			return TurnEvent{}, nil, fmt.Errorf("%s", advance.RefusalReason)
		}
		if ok {
			actorOps = append(actorOps, advanceOps...)
			turnAdvance = &advance
		}
	}
	// Learning runs on the same projection base as the advance and before the
	// settlement, so a technique learned this turn is priced by the settlement
	// that follows and the panel the player sees already accounts for it. Running
	// it after the settlement would leave the turn's own display one step behind
	// what the character actually knows.
	var turnSkill turnSkillOutcome
	if turn.TurnResult != nil && turn.TurnResult.Skills != nil && !turn.TurnResult.Skills.Empty() {
		learningBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(learningBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(learningBase, op)
		}
		outcome, err := planTurnSkillCommit(parentProjection, meta, actorState, learningBase, turn.TurnResult.Skills, turn.ID, turn.User)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		turnSkill = outcome
		actorOps = append(actorOps, outcome.ops...)
		if outcome.clock != nil {
			turn.ClockAdvance = outcome.clock
		}
	}
	// Methods run after skills and before the settlement for the same reason:
	// a method that completed or started circulating this turn must be priced by
	// the settlement that follows, or the panel the player sees would still show
	// the previous method's numbers.
	var turnTechnique turnTechniqueOutcome
	if turn.TurnResult != nil && turn.TurnResult.Techniques != nil && !turn.TurnResult.Techniques.Empty() {
		techniqueBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(techniqueBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(techniqueBase, op)
		}
		outcome, err := planTurnTechniqueCommit(parentProjection, meta, actorState, techniqueBase, turn.TurnResult.Techniques, turn.ID, turn.User)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		turnTechnique = outcome
		actorOps = append(actorOps, outcome.ops...)
		if outcome.clock != nil {
			turn.ClockAdvance = outcome.clock
		}
	}
	// Items run before the settlement for the same reason: a copy equipped this
	// turn must be priced by the settlement that follows, or the panel would show
	// the previous equipment's numbers for one turn.
	if turn.TurnResult != nil && turn.TurnResult.Items != nil && !turn.TurnResult.Items.Empty() {
		itemBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(itemBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(itemBase, op)
		}
		itemActorID := normalizeStatePanelActorID(turn.TurnResult.Items.ActorID)
		if itemActorID == "" {
			itemActorID = DefaultActorID
		}
		if itemOps, _, ok := xiuxianItemOps(itemBase, actorState, itemActorID, withoutPaidAcquisitions(*turn.TurnResult.Items, turn.TurnResult.Trade), turn.ID); ok {
			actorOps = append(actorOps, itemOps...)
		}
	}
	// Trade runs after items and before the settlement. It has to see the pack
	// the item module left, because a copy obtained this turn is a copy the
	// character can sell, and a purchase adds to the same pack. Running it
	// before items would price a sale against stock the turn had already
	// changed, and running it after the settlement would leave the purse the
	// player sees one step behind what really changed hands.
	if turn.TurnResult != nil && turn.TurnResult.Trade != nil && !turn.TurnResult.Trade.Empty() {
		tradeBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(tradeBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(tradeBase, op)
		}
		tradeActorID := normalizeStatePanelActorID(turn.TurnResult.Trade.ActorID)
		if tradeActorID == "" {
			tradeActorID = DefaultActorID
		}
		if tradeOps, _, ok := xiuxianTradeOps(tradeBase, actorState, tradeActorID, *turn.TurnResult.Trade, turn.ID); ok {
			actorOps = append(actorOps, tradeOps...)
		}
	}
	// Rest and regulation run after trade and before the settlement. A rest this
	// turn really spends has to start from the position the turn left the
	// character in — a wound the prose described is already on them — and its
	// restored pool has to be priced by the settlement that follows, or the panel
	// the player sees would show the pool before the rest.
	//
	// It is applied to a projection of the turn's own operations rather than to
	// the live map, so the interval starts from that position without any
	// operation being applied twice.
	//
	// The interval is also the turn's formal time, for the same reason a training
	// interval is: a Turn carries no clock, so a turn that rests must publish the
	// time it really spent as a recovery side record, or the world would stand
	// still while the character healed.
	var turnRest turnRestOutcome
	restCommitted := false
	if turn.TurnResult != nil && turn.TurnResult.Rest != nil && !turn.TurnResult.Rest.Empty() &&
		(turn.TurnResult.Cultivation == nil || turn.TurnResult.Cultivation.Empty()) &&
		(turn.TurnResult.World == nil || turn.TurnResult.World.Empty()) {
		restBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(restBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(restBase, op)
		}
		restActorID := normalizeStatePanelActorID(turn.TurnResult.Rest.ActorID)
		if restActorID == "" {
			restActorID = DefaultActorID
		}
		if outcome, ok := planTurnRestCommit(parentProjection, actorState, restBase, meta, storyID, branchID, restActorID, turn.TurnResult.Rest, turn.ID); ok {
			actorOps = append(actorOps, outcome.ops...)
			turnRest = outcome
			restCommitted = true
		}
	}
	// The world action — a stretch of road, a task decision, a participation or a
	// realm action — runs after rest and before the settlement. Its preconditions
	// read the committed branch, not this turn's prose: a narrator who writes
	// 主角走进洞府 has described the result, not moved the character, and the
	// sentence that describes being inside cannot be the evidence that entering
	// was allowed. The arithmetic still reads what this turn's own operations
	// left, so a reward adds to the balance the turn really ended with.
	//
	// Its side record carries the formal clock for a journey and a public event.
	// A Turn cannot, so a turn that walks or joins must publish the interval it
	// really spent, or the world would stand still while the character travelled.
	//
	// A refusal fails the turn. The prose and the other modules are already in
	// hand, and some of them may describe the refused success — the location the
	// character never reached, the item they never took. Dropping only the world
	// record would publish exactly the fake success this path exists to prevent,
	// so nothing is appended and the model rewrites the turn.
	var turnWorld turnWorldPlan
	worldCommitted := false
	if turn.TurnResult != nil && turn.TurnResult.World != nil && !turn.TurnResult.World.Empty() &&
		(turn.TurnResult.Rest == nil || turn.TurnResult.Rest.Empty()) &&
		(turn.TurnResult.Cultivation == nil || turn.TurnResult.Cultivation.Empty()) {
		worldBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(worldBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(worldBase, op)
		}
		plan, err := planTurnWorldCommit(parentProjection, meta, actorState, worldBase, branchID, storyID, turn.ID, turn.TurnResult.World, turn.TurnResult.CharacterFacts)
		if err != nil {
			return TurnEvent{}, nil, err
		}
		if plan.cooperation != nil && plan.cooperation.Outcome == "worked" && plan.cooperation.Definition.PartnerSkillID != "" && actorState.AdventureRules != nil {
			partner, _ := cooperationPartner(parentProjection, plan.cooperation.Definition)
			if _, initialize, initErr := cooperationNPCActor(storyCommittedState(parentProjection), *actorState.AdventureRules, partner); initErr != nil {
				return TurnEvent{}, nil, initErr
			} else if initialize {
				ops = append(ops, interactivestate.Op{Op: "set", Path: actorStateRoot + "." + partner.ID + ".id", Value: partner.ID, SourceKind: StateOpSourceCooperation}, interactivestate.Op{Op: "set", Path: actorStateRoot + "." + partner.ID + ".name", Value: partner.Name, SourceKind: StateOpSourceCooperation}, interactivestate.Op{Op: "set", Path: actorStateRoot + "." + partner.ID + ".template_id", Value: AdventureNPCTemplateID, SourceKind: StateOpSourceCooperation})
			}
		}
		actorOps = append(actorOps, plan.ops...)
		turnWorld = plan
		if plan.wait != nil && plan.localAffair == nil {
			turn.ClockAdvance = &TurnClockAdvance{
				Action: plan.action, SourceText: strings.TrimSpace(turn.TurnResult.World.SourceText),
				DurationSeconds: plan.wait.After.Seconds - plan.wait.Before.Seconds,
				Before:          plan.wait.Before, After: plan.wait.After,
			}
		}
		if turn.ClockAdvance != nil && plan.action == "adventure" {
			turn.ClockAdvance.SourceText = turn.User
		}
		worldCommitted = true
	}
	// Craft runs after the world action and before the settlement. Its preconditions
	// read the position the turn left, so the herbs this turn's items module
	// acquired are herbs the furnace can really consume, and the product is priced
	// by the settlement that follows.
	//
	// It is exclusive with training, rest and the world module: each of those
	// publishes a clock record of its own, and two records in one turn would both
	// advance from the same starting instant. Submission refuses that combination,
	// and this guard keeps a caller that bypassed submission from producing it.
	//
	// Like a rest, a craft is real in-fiction time, and a Turn carries no clock, so
	// the interval is published as a craft side record below.
	var turnCraft turnCraftOutcome
	craftCommitted := false
	if turn.TurnResult != nil && turn.TurnResult.Craft != nil && !turn.TurnResult.Craft.Empty() &&
		(turn.TurnResult.Cultivation == nil || turn.TurnResult.Cultivation.Empty()) &&
		(turn.TurnResult.Rest == nil || turn.TurnResult.Rest.Empty()) &&
		(turn.TurnResult.World == nil || turn.TurnResult.World.Empty()) {
		craftBase := cloneStoryState(settlementBase)
		for _, op := range ops {
			applyStateOp(craftBase, op)
		}
		for _, op := range actorOps {
			applyActorStateOp(craftBase, op)
		}
		craftActorID := normalizeStatePanelActorID(turn.TurnResult.Craft.ActorID)
		if craftActorID == "" {
			craftActorID = DefaultActorID
		}
		if outcome, refusal := planTurnCraftCommit(actorState, craftActorID, craftBase, turn.TurnResult.Craft, turnIdentity(turn), turn.ID); refusal == "" {
			actorOps = append(actorOps, outcome.ops...)
			turnCraft = outcome
			craftCommitted = true
		} else {
			slog.WarnContext(context.Background(), "craft was refused at commit", "turn", turn.ID, "reason", refusal)
		}
	}
	actorOps = append(actorOps, settleXiuxianTurnState(settlementBase, actorState, ops, actorOps, turn.ID)...)
	branch.Head = turn.ID
	titleGenerated := generatePendingStoryTitle(&meta, turn.Narrative)

	var delta *StateDeltaEvent
	actorOps = normalizeActorStateOps(actorOps)
	// Every Actor write now carries the value the field held before it. The
	// summary the player reads is built from that difference, and it is recorded
	// here because this is the last moment the pre-turn position still exists:
	// the settlement below has already moved the live state, and a write on its
	// own only says where a field landed, not where it came from.
	actorOps = recordActorStatePreviousValues(turnStartState, ops, actorOps)
	if len(ops) > 0 || len(actorOps) > 0 {
		for _, op := range ops {
			if err := validateStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		for _, op := range actorOps {
			if err := validateActorStateOp(op); err != nil {
				return TurnEvent{}, nil, err
			}
		}
		stateDelta := newStateDeltaWithActorOps(ops, actorOps)
		turn.StateDelta = &stateDelta
		turn.StateStatus = "ready"
		stateDeltaEvent := newStateDeltaEventWithActorOps(turn.ID, parentIDString(parentID), branchID, now, ops, actorOps)
		delta = &stateDeltaEvent
	} else if turn.TurnResult != nil {
		turn.StateStatus = "ready"
	} else {
		turn.StateStatus = "pending"
	}

	meta.Branches[branchID] = branch
	meta.UpdatedAt = now
	newEvents := []any{}
	if regenerationRestore != nil {
		regenerationRestore.Ts = now
		newEvents = append(newEvents, *regenerationRestore)
	}
	newEvents = append(newEvents, turn)
	// The formal instant this turn leaves the branch on. Every module that spends
	// real time publishes its own interval record below, and the branch's clock is
	// only moved by those records, so this starts at the committed position and is
	// raised by whichever record this turn really wrote.
	committedClockSeconds := parentProjection.clockSeconds()
	if turnSkill.clock != nil {
		committedClockSeconds = turnSkill.clock.After.Seconds
	}
	if turnTechnique.clock != nil {
		committedClockSeconds = turnTechnique.clock.After.Seconds
	}
	if worldClock := turnWorld.clockSeconds(); worldClock > committedClockSeconds {
		committedClockSeconds = worldClock
	}
	var committedPlanUpdate *string
	if meta.PlanningMode == StoryPlanningModeEnabled && turn.TurnResult != nil {
		committedPlanUpdate = cloneStringPointer(turn.TurnResult.PlanUpdate)
		// Replacing the latest Turn first restores the branch projection to the
		// target's parent. Omission means preserve the current plan, so attach
		// that same content to the replacement Turn instead of losing it.
		if committedPlanUpdate == nil && strings.TrimSpace(req.ReplaceTurnID) != "" && branchProjection.Plan != nil {
			committedPlanUpdate = cloneStringPointer(&branchProjection.Plan.Markdown)
		}
	}
	if committedPlanUpdate != nil {
		newEvents = append(newEvents, BranchPlanUpdatedEvent{
			V: schemaVersion, Type: StoryEventTypeBranchPlanUpdated, ID: newID("bpu"),
			ParentID: turn.ID, BranchID: turn.BranchID, Ts: turn.Ts, TurnID: turn.ID,
			Markdown: normalizeBranchPlanMarkdown(*committedPlanUpdate),
		})
	}
	// Character-side facts are published in this same transaction, as a side
	// record parented after the Turn. They cannot go through AppendCharacterFacts:
	// that takes the store lock this call already holds, and a second transaction
	// would let the prose exist while the people in it did not.
	// The character record's scene is where the turn left the protagonist, not
	// where it found them: the turn's own state delta is what moves the story,
	// and a conversation that starts after this turn reads the same state.
	sceneState := cloneStoryState(state)
	if turn.StateDelta != nil {
		applyStateDeltaToProjection(sceneState, *turn.StateDelta)
	}
	if turn.TurnResult != nil && turn.TurnResult.CharacterFacts != nil {
		communicationMode := ""
		if meta.PackBinding != nil && meta.PackBinding.Gameplay != nil {
			communicationMode = meta.PackBinding.Gameplay.Communication
		}
		if diagnostic := validateLetterWorldCharacterChannels(turn.TurnResult.CharacterFacts, communicationMode); diagnostic != nil {
			return TurnEvent{}, nil, fmt.Errorf("character communication commit refused: %s", diagnostic.Message)
		}
		var existingMatters []character.Matter
		if parentProjection.Characters != nil {
			existingMatters = parentProjection.Characters.Matters
		}
		if diagnostic := validateLetterMatterSettlements(turn.TurnResult.CharacterFacts, existingMatters, committedClockSeconds); diagnostic != nil {
			return TurnEvent{}, nil, fmt.Errorf("letter matter commit refused: %s", diagnostic.Message)
		}
	}
	characterRecord, err := buildTurnCharacterRecord(storyID, branchID, turn, parentProjection, sceneState, meta.Calendar, req.PlayerLetter, meta.PackBinding)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	if turnWorld.faction != nil {
		if err := bindFactionConsent(turnWorld.faction, characterRecord); err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if turnWorld.cooperation != nil {
		if err := bindCooperationConsent(turnWorld.cooperation, characterRecord); err != nil {
			return TurnEvent{}, nil, err
		}
	}
	if characterRecord != nil {
		newEvents = append(newEvents, *characterRecord)
		// The branch head advances to the character record so the next turn is
		// parented after it, exactly as a window conversation does. The Turn's own
		// identity and turn number are untouched.
		branch.Head = characterRecord.ID
		meta.Branches[branchID] = branch
	}
	if turnSkill.clock != nil {
		closed, headID := storyIntervalEvents(intervalProjectionAfterOps(parentProjection, turnSkill.ops), actorState, branchID, branch.Head, now, turn.ID, turnSkill.result.ActorID, turnSkill.clock.Before, turnSkill.clock.After)
		newEvents = append(newEvents, closed...)
		branch.Head = headID
		meta.Branches[branchID] = branch
	}
	if turnTechnique.clock != nil {
		closed, headID := storyIntervalEvents(intervalProjectionAfterOps(parentProjection, turnTechnique.ops), actorState, branchID, branch.Head, now, turn.ID, turnTechnique.result.ActorID, turnTechnique.clock.Before, turnTechnique.clock.After)
		newEvents = append(newEvents, closed...)
		branch.Head = headID
		meta.Branches[branchID] = branch
	}
	// The interval a training turn really spent is published as a cultivation
	// side record, because the formal clock lives only on records that can carry
	// it and a Turn cannot. It is parented after whatever the head now is, so a
	// regenerated turn's old record no longer matches the path and cannot fold
	// its time into a branch that does not contain that training.
	if turnAdvance != nil {
		if clockRecord := turnCultivationClockRecord(parentProjection, branch.Head, storyID, branchID, turn, *turnAdvance, actorState, turnCultivation, now); clockRecord != nil {
			newEvents = append(newEvents, *clockRecord)
			branch.Head = clockRecord.ID
			committedClockSeconds = clockRecord.ClockAfter.Seconds
			// The training interval closed every registered matter whose instant it
			// crossed, not only the activity the turn was about.
			closed, headID := storyIntervalEvents(parentProjection, actorState, branchID, branch.Head, now, turn.ID, clockRecord.Activity.ActorID, clockRecord.ClockBefore, clockRecord.ClockAfter)
			newEvents = append(newEvents, closed...)
			branch.Head = headID
			meta.Branches[branchID] = branch
		}
	}
	// The interval a resting turn really spent is published the same way and for
	// the same reason. It is chained after the cultivation record so both fold in
	// the order the turn produced them, and a regenerated turn's old rest record
	// no longer matches the path.
	if restCommitted {
		if recoveryRecord := turnRecoveryRecord(parentProjection, branch.Head, branchID, turn, turnRest, actorState, now); recoveryRecord != nil {
			newEvents = append(newEvents, *recoveryRecord)
			branch.Head = recoveryRecord.ID
			committedClockSeconds = recoveryRecord.ClockAfter.Seconds
			closed, headID := storyIntervalEvents(parentProjection, actorState, branchID, branch.Head, now, turn.ID, recoveryRecord.Activity.ActorID, recoveryRecord.ClockBefore, recoveryRecord.ClockAfter)
			newEvents = append(newEvents, closed...)
			branch.Head = headID
			meta.Branches[branchID] = branch
		}
	}
	// The world action's own record is published the same way and for the same
	// reason: it is parented after whatever the turn left at the branch head, so
	// a regenerated turn's old record no longer matches the path and cannot fold
	// its travel, task, participation or realm instance into a branch that does
	// not contain it.
	//
	// Its state writes are not repeated here. They are in the turn's own
	// StateDelta, and writing them twice would let a replay apply them twice.
	if worldCommitted {
		worldEvents, worldHead, worldErr := turnWorldSettlementEvents(parentProjection, actorState, branchID, branch.Head, now, turn.ID, turn.TurnResult.World, turnWorld)
		if worldErr != nil {
			return TurnEvent{}, nil, worldErr
		}
		if len(worldEvents) > 0 {
			newEvents = append(newEvents, worldEvents...)
			branch.Head = worldHead
			if worldClock := turnWorld.clockSeconds(); worldClock > committedClockSeconds {
				committedClockSeconds = worldClock
			}
			meta.Branches[branchID] = branch
		}
	}
	// The interval a crafting turn really spent is published the same way. It is
	// parented after whatever the turn left at the branch head, so a regenerated
	// turn's old craft record no longer matches the path and cannot fold its hours
	// into a branch that does not contain that batch.
	//
	// Its pack writes are not repeated here for the same reason the world record's
	// are not: they are in the turn's own StateDelta, and writing them twice would
	// let a replay spend the materials twice.
	if craftCommitted {
		if craftRecord := turnCraftRecord(parentProjection, branch.Head, branchID, turn, turnCraft, actorState, now); craftRecord != nil {
			newEvents = append(newEvents, *craftRecord)
			branch.Head = craftRecord.ID
			committedClockSeconds = craftRecord.ClockAfter.Seconds
			closed, headID := storyIntervalEvents(parentProjection, actorState, branchID, branch.Head, now, turn.ID, "", craftRecord.ClockBefore, craftRecord.ClockAfter)
			newEvents = append(newEvents, closed...)
			branch.Head = headID
			meta.Branches[branchID] = branch
		}
	}
	modelContinuationEvents, err := newModelContextProviderContinuationEvents(turn.ID, turn.BranchID, turn.Ts, turn.ModelContextMessages)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	newEvents = append(newEvents, modelContinuationEvents...)
	if len(req.ProviderContinuation) != 0 {
		newEvents = append(newEvents, newProviderContinuationEvent(turn, req.ProviderContinuation))
	}
	agentRecords, err := sessionjournal.CheckpointRecords(&s.storyJournals[storyID].projection.AgentSessions, req.Checkpoint, turn.ID)
	if err != nil {
		return TurnEvent{}, nil, err
	}
	newEvents = append(newEvents, agentRecords...)
	// A natural 寿尽 is decided here, from the instant this turn really left the
	// branch on, and written onto this turn rather than onto a later one.
	//
	// It is the backend's decision and not the model's. §4 of the time/lifespan
	// design says 生死结果属于正式事实，不由AI为了制造戏剧随机宣布, and the model
	// cannot know the table: it is a pair of committed numbers — the confirmed
	// total for the current realm and the interval the branch has spent. The
	// model is told the edge is coming through the interval's own stop node, and
	// that is what lets its prose arrive at the brink rather than past it.
	//
	// An ending the caller already declared is left alone. Two sources writing
	// TerminalOutcome would let the weaker one overwrite the real cause.
	if turn.TerminalOutcome == nil {
		// The realm is read from the position the turn leaves, not the one it
		// found. A breakthrough this very turn adopts the new realm's total (§3),
		// so a 炼气 character who reached 筑基 at 119 is not killed at 120 by the
		// stage they no longer hold.
		lifespanState := cloneStoryState(state)
		if turn.StateDelta != nil {
			applyStateDeltaToProjection(lifespanState, *turn.StateDelta)
		}
		boundary := lifespanBoundary(meta, lifespanState, DefaultActorID)
		if ending := lifespanEnding(boundary, committedClockSeconds, meta.Calendar, turn.Narrative); ending != nil {
			turn.TerminalOutcome = ending
			// A turn that ends a branch owes no next actions. The choices were
			// validated before this decision, so they are cleared here rather than
			// refused: a narrator who wrote an ordinary turn and then ran the clock
			// out has produced a prose that is still true, and asking it to rewrite
			// would only make it describe an ending it was never told about.
			if turn.TurnResult != nil {
				turn.TurnResult.Choices = nil
			}
			slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-story] branch ended by confirmed lifespan story_id=%s branch=%s turn=%s total_years=%d clock=%d", storyID, branchID, turn.ID, boundary.TotalYears, committedClockSeconds))
		}
	}
	// Every Actor this turn wrote is stamped with the turn that wrote it and the
	// instant the branch stood at once the turn was done.
	//
	// The instant is only known here: it starts at the committed position and is
	// raised by whichever side record this turn really published, so a record
	// written before a decade of seclusion says so rather than describing a world
	// a decade gone. A turn that spent no time leaves the instant at zero, and the
	// record is then dated by its turn alone.
	//
	// Both the Turn's own delta and the side event carry the stamp, because the
	// two are read by different replay paths — the turn by the model-visible
	// history, the side event by a branch that forked after it.
	if turn.StateDelta != nil && len(turn.StateDelta.ActorOps) > 0 {
		atSeconds, atEvent := stateDeltaStamp(committedClockSeconds, turn.ID)
		turn.StateDelta.WrittenAtSeconds = atSeconds
		turn.StateDelta.WrittenAtEvent = atEvent
		if delta != nil {
			delta.WrittenAtSeconds = atSeconds
			delta.WrittenAtEvent = atEvent
		}
	}
	if appendErr := s.appendStoryTransactionLocked(storyID, meta, newEvents...); appendErr != nil {
		return TurnEvent{}, nil, appendErr
	}
	if titleGenerated {
		slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-story] generated story title from first committed narrative story_id=%s", storyID))
	}
	s.syncStoryIndexProjectionLocked(storyID)
	return turn, delta, nil
}

func committedAgentTurnForRequest(lines []StoryEventRecord, branchID string, req AppendTurnWithStateRequest) (TurnEvent, *StateDeltaEvent, bool, error) {
	commandID := strings.TrimSpace(req.AgentCommandID)
	operationID := strings.TrimSpace(req.AgentOperationID)
	hasIdentity := commandID != "" || operationID != "" || req.AgentCycle != 0
	if hasIdentity && (commandID == "" || operationID == "" || req.AgentCycle <= 0) {
		return TurnEvent{}, nil, false, fmt.Errorf("%w: command_id, operation_id, and positive cycle are required together", ErrAgentTurnIdentityConflict)
	}
	if commandID == "" {
		return TurnEvent{}, nil, false, nil
	}
	commitHash, err := agentTurnRequestHash(req)
	if err != nil {
		return TurnEvent{}, nil, false, err
	}
	for _, record := range lines {
		if record.Envelope.Type != StoryEventTypeTurn {
			continue
		}
		var turn TurnEvent
		if err := mapToStruct(record.Raw, &turn); err != nil {
			return TurnEvent{}, nil, false, fmt.Errorf("decode committed agent turn: %w", err)
		}
		if strings.TrimSpace(turn.AgentCommandID) != commandID {
			continue
		}
		if turn.BranchID != branchID || strings.TrimSpace(turn.AgentOperationID) != operationID || turn.AgentCycle != req.AgentCycle || strings.TrimSpace(turn.AgentCommitHash) == "" || turn.AgentCommitHash != commitHash {
			return TurnEvent{}, nil, false, fmt.Errorf("%w: command_id=%q existing_operation=%q requested_operation=%q existing_cycle=%d requested_cycle=%d", ErrAgentTurnIdentityConflict, commandID, turn.AgentOperationID, operationID, turn.AgentCycle, req.AgentCycle)
		}
		return turn, stateDeltaEventForCommittedTurn(turn), true, nil
	}
	return TurnEvent{}, nil, false, nil
}

func agentTurnRequestHash(req AppendTurnWithStateRequest) (string, error) {
	// ModelContextMessages are durable side evidence keyed independently by the
	// accepted player input and batch ordinal. Excluding them keeps a staged Turn
	// identity stable when the store atomically folds those batches into it.
	//
	// Recap is excluded for a different reason: it is rebuildable display
	// material, so a cycle that changes only its recollection has not changed what
	// it publishes. Including it would also rewrite the identity of every turn
	// committed before the field existed.
	payload := struct {
		BranchID             string
		ExpectedParentID     *string
		ReplaceTurnID        string
		User                 string
		Narrative            string
		Thinking             string
		RunID                string
		AgentKind            string
		ProviderContinuation map[string]any
		DisplayEvents        []DisplayEvent
		Ops                  []interactivestate.Op
		ActorOps             []ActorStateOp
		RuleResolution       *RuleResolution
		TurnResult           *TurnResult
		PlanUpdate           *string
		TerminalOutcome      *TerminalOutcome
		StateSchemaProposal  *ActorStateSchemaProposal
		PlayerLetter         *PlayerLetterIntent
	}{
		BranchID: strings.TrimSpace(req.BranchID), ExpectedParentID: req.ExpectedParentID,
		ReplaceTurnID: strings.TrimSpace(req.ReplaceTurnID),
		User:          req.User, Narrative: req.Narrative, Thinking: req.Thinking,
		RunID: req.RunID, AgentKind: req.AgentKind, ProviderContinuation: req.ProviderContinuation,
		DisplayEvents: req.DisplayEvents,
		Ops:           req.Ops, ActorOps: req.ActorOps, RuleResolution: req.RuleResolution,
		TurnResult: req.TurnResult, PlanUpdate: planUpdateFromTurnResult(req.TurnResult), TerminalOutcome: req.TerminalOutcome,
		StateSchemaProposal: req.StateSchemaProposal, PlayerLetter: req.PlayerLetter,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("hash agent turn commit payload: %w", err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

func planUpdateFromTurnResult(result *TurnResult) *string {
	if result == nil {
		return nil
	}
	return cloneStringPointer(result.PlanUpdate)
}

func stateDeltaEventForCommittedTurn(turn TurnEvent) *StateDeltaEvent {
	if turn.StateDelta == nil {
		return nil
	}
	event := newStateDeltaEventWithActorOps(
		turn.ID, parentIDString(turn.ParentID), turn.BranchID, turn.Ts,
		append([]interactivestate.Op(nil), turn.StateDelta.Ops...), append([]ActorStateOp(nil), turn.StateDelta.ActorOps...),
	)
	event.SchemaVersion = turn.StateDelta.SchemaVersion
	event.WrittenAtSeconds = turn.StateDelta.WrittenAtSeconds
	event.WrittenAtEvent = turn.StateDelta.WrittenAtEvent
	return &event
}

// syncStoryIndexProjectionLocked updates the rebuildable story catalog from
// canonical JSONL state. A projection failure is logged but never turns an
// already-durable event into a false-negative operation result.
func (s *Store) syncStoryIndexProjectionLocked(storyID string) {
	if _, err := s.publishStorySummaryLocked(storyID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] index projection write failed after canonical commit story_id=%s err=%v", storyID, err))
	}
}
