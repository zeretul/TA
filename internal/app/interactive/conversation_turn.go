package interactiveapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	agent "ta/agent"
	"ta/agent/providers"

	"ta/config"
	agents "ta/internal/agents"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/session"
	"ta/internal/agents/toolresult"
	"ta/internal/character"
	"ta/internal/interactive"
)

func (c *Conversation) PrepareInteractiveTurn(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
	if c == nil || c.store == nil {
		return interactive.RuleResolution{}, fmt.Errorf("互动故事不存在")
	}
	c.turnCheckMu.Lock()
	defer c.turnCheckMu.Unlock()
	if err := c.loadTurnDraft(); err != nil {
		return interactive.RuleResolution{}, err
	}
	c.mu.Lock()
	if c.ruleResolution != nil {
		resolution := *c.ruleResolution
		c.mu.Unlock()
		slog.WarnContext(ctx, fmt.Sprintf("[interactive-agent] reused locked turn check story_id=%s branch_id=%s rule_resolution_id=%s", c.storyID, c.branchID, resolution.ID))
		return resolution, nil
	}
	c.mu.Unlock()
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	// A fight the window owns is settled by its rounds, not by a check that writes
	// the whole exchange in one roll. Refusing here reaches the model before it
	// writes prose, so the run can tell the player to open the window instead of
	// producing a narrative that claims a result the rules never settled.
	if err := c.turnBlockedByBattle(storyCtx); err != nil {
		return interactive.RuleResolution{}, err
	}
	select {
	case <-ctx.Done():
		return interactive.RuleResolution{}, ctx.Err()
	default:
	}
	actorState, currentState, err := c.effectiveTurnState(storyCtx)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	storyDirector := storyDirectorForSnapshot(c.StoryRuntimeForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	storyDirector.ActorState = actorState
	resolution, err := interactive.ResolveTurnRulesWithStorySettings(c.storyID, storyCtx.Snapshot.BranchID, currentState, storyDirector, storyCtx.Meta.CheckSettings, request)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	// A branch already past its confirmed lifespan owes an ending, and this is the
	// only moment the model can be told so: terminal_candidate is the channel that
	// makes it write the ending and submit no next actions.
	//
	// The ordinary crossing is decided at commit, from the instant the turn really
	// leaves the branch on — that is the honest place, because it is the only one
	// that sees the interval. This check exists for the crossings no turn settles:
	// a journey started from the panel writes its own clock record, so the branch
	// can arrive past the edge between two turns. Without this, that branch would
	// keep playing with no years left and the commit would only end it on the next
	// turn that happened to spend time.
	if candidate := interactive.LifespanTerminalCandidate(storyCtx.Meta, currentState, storyClockSeconds(storyCtx.Snapshot.Clock)); candidate != nil {
		resolution.TerminalCandidate = candidate
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] branch is past its confirmed lifespan story_id=%s branch_id=%s reason=%s", c.storyID, storyCtx.Snapshot.BranchID, candidate.Reason))
	}
	c.mu.Lock()
	draft := c.turnDraft
	c.mu.Unlock()
	draft.RuleResolution = &resolution
	result, err := ruleResolutionToolResult(resolution)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	if err := c.commitTurnDraft(ctx, draft, result); err != nil {
		return interactive.RuleResolution{}, err
	}
	c.mu.Lock()
	c.ruleResolution, c.turnDraft = &resolution, draft
	c.mu.Unlock()
	return resolution, nil
}

// storyClockSeconds reads a branch's formal instant, treating a branch that has
// never spent time as the origin.
func storyClockSeconds(clock *interactive.StoryClock) int64 {
	if clock == nil {
		return 0
	}
	return clock.Seconds
}

// turnBlockedByBattle refuses a main-story turn while a fight is the combat
// window's to settle.
//
// It is the same judgement the commit makes, asked early. The commit check is the
// one that cannot be bypassed; this one exists so the refusal reaches the model
// before it writes prose, because a turn that discovers the fight only at commit
// time has already spent its output on a narrative that cannot be published.
func (c *Conversation) turnBlockedByBattle(storyCtx interactive.StoryContext) error {
	if c == nil || c.store == nil {
		return nil
	}
	// A regeneration is judged at the parent it starts from, so a replacement
	// turn is not refused by the fight the turn being replaced committed.
	if c.regenerateTargetSnapshot() != "" {
		return nil
	}
	combats, err := c.store.CombatSessions(c.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return err
	}
	if err := interactive.StoryTurnBlockedByBattle(
		storyCtx.Snapshot.State,
		storyDirectorForSnapshot(c.StoryRuntimeForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema).ActorState,
		combats,
	); err != nil {
		return fmt.Errorf("%w：这场交手已经开打，请在战斗窗口里逐回合进行；打完再继续推进主故事", err)
	}
	return nil
}

// SubmitTurnResult durably accepts valid modules without publishing Actor
// State or the final story Turn. Failed modules remain available for repair.
func (c *Conversation) SubmitTurnResult(ctx context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
	if c == nil || c.store == nil {
		return interactive.TurnSubmissionReceipt{}, fmt.Errorf("互动故事不存在")
	}
	c.turnCheckMu.Lock()
	defer c.turnCheckMu.Unlock()
	if err := c.loadTurnDraft(); err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	select {
	case <-ctx.Done():
		return interactive.TurnSubmissionReceipt{}, ctx.Err()
	default:
	}
	if c.InteractiveNarrativeReady() {
		slog.WarnContext(ctx, fmt.Sprintf("[interactive-agent] ignored duplicate turn result before validation story_id=%s branch_id=%s", c.storyID, c.branchID))
		return interactiveTurnResultAlreadyAcceptedReceipt(), nil
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	// The same refusal, for a run that reached submission without preparing a
	// check. Without it a model that narrates the whole fight and submits only
	// state_changes would be caught at commit, after its prose was written.
	if err := c.turnBlockedByBattle(storyCtx); err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	c.mu.Lock()
	requireProtagonistSelection := c.requireProtagonistSelection
	c.mu.Unlock()
	if requireProtagonistSelection && storyCtx.Meta.Protagonist.Mode == interactive.StoryProtagonistModeDefault {
		return interactive.TurnSubmissionReceipt{}, fmt.Errorf("select_story_protagonist must succeed before submitting the opening turn")
	}
	actorState, currentState, err := c.effectiveTurnState(storyCtx)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	director := c.StoryRuntimeForMeta(storyCtx.Meta)
	// The character-facts module is validated against this branch's registered
	// cast and this branch's own scene. Both come from committed state, so the
	// check rejects a turn that speaks for someone the story never introduced,
	// and resolves each line's audience the way the commit will.
	characterState, err := c.store.CharacterState(c.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	// A first appearance the model staged from this turn's discovery page is bound
	// to the author's own key here, before the module is validated or committed.
	// §7.1 requires the mapping to not depend on the model minting an id, and a
	// real run showed why: the model read the page, used the catalog's name, and
	// submitted no key, so the preset person was registered as a new actor.
	c.bindStagedPeople(input.CharacterFacts)
	// A key the model wrote from memory is checked against the pack this story is
	// actually bound to. The pack is the only authority on who its keys name, so a
	// key it does not ship, or ships for someone else, is corrected from the
	// submitted name or dropped rather than allowed to bind a stranger.
	c.sourceKeyAliases(input.CharacterFacts)
	c.mu.Lock()
	current := c.turnProtocol.draft()
	c.mu.Unlock()
	// The world module is judged against the same committed records the commit
	// will read, so a refusal reaches the model while the prose can still be
	// rewritten. A regenerate target is judged at its parent, the moment the
	// replacement turn will start from.
	worldView, err := c.store.TurnWorldView(c.storyID, storyCtx.Snapshot.BranchID, c.regenerateTargetSnapshot())
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	if c.regenerateTargetSnapshot() != "" {
		// A replaced reply cannot supply its own prior qualification or revoke
		// the membership the replacement is actually starting from.
		characterState = character.State{}
		if worldView.Characters != nil {
			characterState = worldView.Characters.Normalize()
		}
	}
	communicationMode := ""
	if gameplay := storyCtx.Meta.PackBinding; gameplay != nil && gameplay.Gameplay != nil {
		communicationMode = gameplay.Gameplay.Communication
	}
	prepared, receipt := interactive.PrepareTurnSubmission(interactive.TurnSubmissionContext{
		ActorState:                  actorState,
		CurrentState:                currentState,
		ChoiceCount:                 storyCtx.Meta.ChoiceCount,
		RuleResolution:              c.ruleResolution,
		RuleStateConsumptionMode:    director.Strategy.RuleStateConsumptionMode,
		RequireCompleteInitialState: c.openingStateSchemaDraft != nil && SnapshotTurnCount(storyCtx.Snapshot) == 0,
		PlanningMode:                storyCtx.Meta.PlanningMode,
		CurrentPlan:                 storyCtx.Snapshot.BranchPlan,
		KnownCharacters:             characterState.CharacterIDs(),
		Scene:                       interactive.StoryScene(storyCtx.Snapshot),
		CharacterAliases:            interactive.TurnCharacterAliases(input.CharacterFacts, characterState),
		// A feeling change is checked against what this branch has already
		// settled, so a repeated fact is recognised as repeated at submission
		// time rather than only at commit time.
		ExistingAffinities:         characterState.Affinities,
		ExistingRelationships:      characterState.Relationships,
		ExistingFactionMemberships: characterState.FactionMemberships,
		ExistingMatters:            characterState.Matters,
		ExistingProfiles:           characterState.Profiles,
		CurrentClockSeconds:        interactive.SnapshotClockSeconds(storyCtx.Snapshot),
		CurrentPlayerInput:         c.user,
		CommunicationMode:          communicationMode,
		Meta:                       storyCtx.Meta,
		BranchID:                   storyCtx.Snapshot.BranchID,
		WorldView:                  &worldView,
		// A craft's loss rate is one draw per command, so the dry run has to read
		// the identity the commit will read or it would judge a batch nobody works.
		CraftIdentity: c.turnDraftIdentity(),
	}, current, input)
	c.mu.Lock()
	draft := c.turnDraft
	c.mu.Unlock()
	// A refused world action means the accepted prose narrates a success the
	// rules just denied. Discarding it here is what lets the model write the real
	// outcome; the refused text is remembered by hash so the run loop cannot
	// re-accept the same success sentence on its way out.
	if receipt.NarrativeRewriteRequired {
		draft.RefusedNarrativeHash = interactive.RefusedNarrativeHash(draft.Narrative)
		draft.Narrative, draft.NarrativeSource = "", ""
	}
	draft.Submission = prepared.Progress()
	result, err := turnSubmissionToolResult(receipt)
	if err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	if err := c.commitTurnDraft(ctx, draft, result); err != nil {
		return interactive.TurnSubmissionReceipt{}, err
	}
	c.mu.Lock()
	staged := c.turnProtocol.update(prepared)
	c.turnDraft = draft
	c.mu.Unlock()
	if !staged {
		receipt = interactiveTurnResultAlreadyAcceptedReceipt()
		slog.WarnContext(ctx, fmt.Sprintf("[interactive-agent] ignored turn result update after protocol lock story_id=%s branch_id=%s", c.storyID, c.branchID))
		return receipt, nil
	}
	stagedResult := prepared.TurnResult()
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] updated turn result draft story_id=%s branch_id=%s ready=%t state_updates=%d choices=%d state_changes_status=%s choices_status=%s plan_update_status=%s diagnostics=%q", c.storyID, c.branchID, receipt.Ready, len(stagedResult.StateUpdates), len(stagedResult.Choices), receipt.ModuleStatus.StateChanges, receipt.ModuleStatus.Choices, receipt.ModuleStatus.PlanUpdate, interactiveTurnSubmissionDiagnosticSummary(receipt.Diagnostics)))
	return receipt, nil
}

func (c *Conversation) InteractiveNarrativeReady() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnProtocol.narrativeReady()
}

func interactiveContextMessageFromSchema(msg *agents.Message) (interactive.ModelContextMessage, bool) {
	if msg == nil {
		return interactive.ModelContextMessage{}, false
	}
	cloned := msg.Clone()
	continuation := providers.ContinuationExtra(cloned.Extra)
	if cloned.Extra != nil {
		delete(cloned.Extra, providers.ExtraKeyContinuation)
		if len(cloned.Extra) == 0 {
			cloned.Extra = nil
		}
	}
	switch cloned.Role {
	case agents.RoleAssistant:
		if len(cloned.ToolCalls) == 0 {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessageFromAgent(cloned, continuation), true
	case agents.RoleTool:
		if strings.TrimSpace(cloned.ToolCallID) == "" && strings.TrimSpace(cloned.ToolName) == "" {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessageFromAgent(cloned, continuation), true
	case agents.RoleUser:
		if cloned.TaskCompletion == nil && !agent.IsContextStateMessage(cloned) {
			return interactive.ModelContextMessage{}, false
		}
		return interactive.ModelContextMessageFromAgent(cloned, continuation), true
	default:
		return interactive.ModelContextMessage{}, false
	}
}

func interactiveToolCallsFromSchema(calls []agents.ToolCall) []interactive.ModelContextToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]interactive.ModelContextToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, interactive.ModelContextToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: interactive.ModelContextFunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func schemaMessagesFromInteractiveContext(messages []interactive.ModelContextMessage) []*agents.Message {
	if len(messages) == 0 {
		return nil
	}
	result := make([]*agents.Message, 0, len(messages))
	for _, msg := range messages {
		message := interactive.AgentMessageFromModelContext(msg)
		if message == nil {
			continue
		}
		continuation := providers.ContinuationExtra(msg.ProviderContinuation)
		if len(continuation) > 0 {
			if message.Extra == nil {
				message.Extra = make(map[string]any)
			}
			for key, value := range continuation {
				message.Extra[key] = value
			}
		}
		result = append(result, message)
	}
	return result
}

func schemaToolCallsFromInteractive(calls []interactive.ModelContextToolCall) []agents.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	result := make([]agents.ToolCall, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		result = append(result, agents.ToolCall{
			Index: call.Index,
			ID:    call.ID,
			Type:  call.Type,
			Function: agents.FunctionCall{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: call.Extra,
		})
	}
	return result
}

func (c *Conversation) AppendAssistant(content string) error {
	return c.AppendAssistantWithThinking(content, "")
}

func (c *Conversation) AppendContextMessage(msg *agents.Message) error {
	return c.AppendContextMessages(msg)
}

func (c *Conversation) AppendContextMessages(messages ...*agents.Message) error {
	if c == nil || len(messages) == 0 {
		return nil
	}
	converted := make([]interactive.ModelContextMessage, 0, len(messages))
	for _, message := range messages {
		if next, ok := interactiveContextMessageFromSchema(message); ok {
			converted = append(converted, next)
		}
	}
	if len(converted) == 0 {
		return nil
	}
	c.modelContextAppendMu.Lock()
	defer c.modelContextAppendMu.Unlock()

	c.mu.Lock()
	identity := c.agentCycleIdentity
	sequence := c.modelContextBatchSequence
	store := c.store
	storyID := c.storyID
	branchID := c.branchID
	c.mu.Unlock()
	commandID := strings.TrimSpace(string(identity.CommandID))
	operationID := strings.TrimSpace(string(identity.OperationID))
	hasIdentity := commandID != "" || operationID != "" || identity.Cycle != 0
	if hasIdentity && (commandID == "" || operationID == "" || identity.Cycle <= 0) {
		return fmt.Errorf("model context batch requires a complete durable cycle identity")
	}
	if !hasIdentity || store == nil {
		// Legacy/non-runtime callers have no durable cycle to attach to. Keep the
		// historical in-memory behavior for those isolated paths only.
		c.mu.Lock()
		c.modelContextMessages = append(c.modelContextMessages, interactive.CloneModelContextMessages(converted)...)
		c.mu.Unlock()
		return nil
	}
	if strings.TrimSpace(branchID) == "" {
		storyContext, err := store.StoryContext(storyID, "")
		if err != nil {
			return err
		}
		branchID = storyContext.Snapshot.BranchID
	}
	intents, err := interactive.NewModelContextBatchIntents(interactive.DomainCommitIdentity{
		CommandID: commandID, OperationID: operationID, Cycle: identity.Cycle,
	}, branchID, sequence, converted)
	if err != nil {
		return err
	}
	for _, intent := range intents {
		receipt, err := store.AppendModelContextBatch(storyID, intent)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.modelContextBatchSequence = receipt.Event.Sequence + 1
		c.mu.Unlock()
	}
	return nil
}

// CommitAgentCanonicalContext persists an exact Agent-owned hidden batch in
// the Story journal. The batch remains side evidence until the final Turn
// absorbs it; memory tracks only the next sequence and never duplicates the
// journal's canonical message payload.
func (c *Conversation) CommitAgentCanonicalContext(ctx context.Context, request agent.ContextCommitRequest) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("game conversation is unavailable")
	}
	converted := make([]interactive.ModelContextMessage, 0, len(request.Messages))
	for index := range request.Messages {
		message := request.Messages[index].Clone()
		stored, ok := interactiveContextMessageFromSchema(message)
		if !ok {
			return "", fmt.Errorf("Agent context message %d cannot be stored by Game", index)
		}
		converted = append(converted, stored)
	}
	c.modelContextAppendMu.Lock()
	defer c.modelContextAppendMu.Unlock()
	c.mu.Lock()
	identity := c.agentCycleIdentity
	branchID := c.branchID
	currentSequence := c.modelContextBatchSequence
	c.mu.Unlock()
	if request.Identity.CommandID != string(identity.CommandID) || request.Identity.RunID != string(identity.OperationID) ||
		request.Identity.Cycle != identity.Cycle {
		return "", fmt.Errorf("Agent context batch does not match the active Game cycle")
	}
	if strings.TrimSpace(branchID) == "" {
		storyContext, err := c.store.StoryContext(c.storyID, "")
		if err != nil {
			return "", err
		}
		branchID = storyContext.Snapshot.BranchID
	}
	intent, err := interactive.NewAgentContextBatchIntent(
		interactive.DomainCommitIdentity{
			CommandID: request.Identity.CommandID, OperationID: request.Identity.RunID, Cycle: request.Identity.Cycle,
		},
		branchID, request.Sequence, converted,
	)
	if err != nil {
		return "", err
	}
	intent.Checkpoint = request.Checkpoint
	receipt, err := c.store.AppendModelContextBatch(c.storyID, intent)
	if err != nil {
		return "", err
	}
	if request.Sequence >= currentSequence {
		c.mu.Lock()
		c.modelContextBatchSequence = request.Sequence + 1
		c.mu.Unlock()
	}
	return receipt.Revision, nil
}

func (c *Conversation) ToolResultContextPolicy() toolresult.ContextPolicy {
	return toolresult.ResolveContextPolicy(c.cfg, config.AgentKindInteractiveStory)
}

func (c *Conversation) AppendAssistantWithThinking(content, thinking string) error {
	return c.AppendAssistantWithMetadata(content, thinking, session.MessageMetadata{})
}

func (c *Conversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	return c.stageAssistantOutput(content, thinking, metadata)
}

func (c *Conversation) stageAssistantOutput(content, thinking string, metadata session.MessageMetadata) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("互动故事不存在")
	}
	retained, err := c.LoadNarrativeCandidate(context.Background())
	if err != nil {
		return err
	}
	if rewrite, err := c.NarrativeRewritePending(); err != nil {
		return err
	} else if rewrite {
		// The prose this run produced was discarded because the world refused
		// what it narrated. Publishing it would resurrect the refused success;
		// the turn has to finish with a replacement narrative instead.
		return fmt.Errorf("本回合正文需要按规则回执重写后才能提交")
	}
	if retained != "" {
		content = retained
	}
	if strings.TrimSpace(metadata.RunID) != "" || len(metadata.ProviderContinuation) != 0 {
		c.mu.Lock()
		c.assistantMetadata = metadata
		c.mu.Unlock()
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output content story_id=%s branch_id=%s content=%q", c.storyID, c.branchID, content))
	narrative, parseErr := ParseAssistantOutput(content)
	if parseErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output failed story_id=%s branch_id=%s err=%v content=%q", c.storyID, c.branchID, parseErr, content))
		return parseErr
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] parse assistant output result story_id=%s branch_id=%s narrative=%q", c.storyID, c.branchID, narrative))
	assistantMetadata := c.assistantMetadataSnapshot()
	cycleIdentity := c.AgentCycleIdentitySnapshot()
	turnResult := c.turnResultSnapshot()
	if turnResult == nil {
		return fmt.Errorf("互动回合的 state_changes 或 choices 尚未完整提交，已拒绝写入不完整状态")
	}
	if strings.TrimSpace(string(cycleIdentity.CommandID)) == "" || strings.TrimSpace(string(cycleIdentity.OperationID)) == "" || cycleIdentity.Cycle <= 0 {
		return fmt.Errorf("互动回合输出缺少 durable cycle identity，已拒绝绕过 Agent 提交屏障")
	}
	request := interactive.AppendTurnWithStateRequest{
		BranchID:             c.branchID,
		ExpectedParentID:     c.baseParentIDSnapshot(),
		ReplaceTurnID:        c.regenerateTargetSnapshot(),
		User:                 c.user,
		Narrative:            narrative,
		Thinking:             thinking,
		RunID:                assistantMetadata.RunID,
		AgentKind:            assistantMetadata.AgentKind,
		AgentCommandID:       string(cycleIdentity.CommandID),
		AgentOperationID:     string(cycleIdentity.OperationID),
		AgentCycle:           cycleIdentity.Cycle,
		ProviderContinuation: providers.ContinuationExtra(assistantMetadata.ProviderContinuation),
		DisplayEvents:        withInteractiveNarrativeAnchor(c.DisplayEventsSnapshot()),
		ModelContextMessages: c.modelContextMessagesSnapshot(),
		RuleResolution:       c.ruleResolutionSnapshot(),
		TurnResult:           turnResult,
		Recap:                c.turnRecapSnapshot(),
		TerminalOutcome:      c.terminalOutcomeSnapshot(narrative),
		StateSchemaProposal:  c.openingStateSchemaProposalSnapshot(),
	}
	c.mu.Lock()
	if c.playerLetter != nil {
		letter := *c.playerLetter
		request.PlayerLetter = &letter
	}
	c.mu.Unlock()
	intent, err := interactive.NewDomainCommitIntent(request)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingDomainCommit != nil && c.pendingDomainCommit.Hash != intent.Hash {
		return fmt.Errorf("互动回合同一 Agent cycle 生成了不同的提交内容")
	}
	c.pendingDomainCommit = &intent
	// The structured protocol is now immutable, but the story branch does not
	// advance until the durable actor authorizes the output stage.
	c.turnProtocol.markCommitted()
	return nil
}

// CommitAgentCanonicalOutput publishes the exact public Agent output through
// the existing atomic game-turn transaction and preserves Text Adventure's post-commit
// Director/version callbacks.
func (c *Conversation) CommitAgentCanonicalOutput(
	ctx context.Context,
	message *agents.Message,
	metadata session.MessageMetadata,
	checkpoint agent.CanonicalCheckpoint,
) (interactive.DomainCommitReceipt, error) {
	if message == nil || message.Role != agent.Assistant || len(message.ToolCalls) != 0 {
		return interactive.DomainCommitReceipt{}, fmt.Errorf("canonical game output requires a final assistant message")
	}
	metadata.ProviderContinuation = providers.ContinuationExtra(message.Extra)
	if err := c.stageAssistantOutput(message.Content, message.ReasoningContent, metadata); err != nil {
		return interactive.DomainCommitReceipt{}, err
	}
	c.mu.Lock()
	if c.pendingDomainCommit != nil {
		c.pendingDomainCommit.Request.Checkpoint = checkpoint
	}
	c.mu.Unlock()
	if err := c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		return interactive.DomainCommitReceipt{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastDomainReceipt == nil {
		return interactive.DomainCommitReceipt{}, fmt.Errorf("canonical game output returned no domain receipt")
	}
	return *c.lastDomainReceipt, nil
}

func (c *Conversation) assistantMetadataSnapshot() session.MessageMetadata {
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := c.assistantMetadata
	metadata.RunPath = append([]string(nil), metadata.RunPath...)
	return metadata
}

func (c *Conversation) modelContextMessagesSnapshot() []interactive.ModelContextMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.modelContextMessages) == 0 {
		return nil
	}
	return interactive.CloneModelContextMessages(c.modelContextMessages)
}

func (c *Conversation) ruleResolutionSnapshot() *interactive.RuleResolution {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleResolution == nil {
		return nil
	}
	resolution := *c.ruleResolution
	return &resolution
}

func (c *Conversation) turnResultSnapshot() *interactive.TurnResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnProtocol.turnResult()
}

// turnRecapSnapshot reads the turn's recollection from the same protocol the
// commit reads its modules from, so the two can never come from different
// submission calls.
func (c *Conversation) turnRecapSnapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnProtocol.turnRecap()
}

func interactiveTurnSubmissionDiagnosticSummary(diagnostics []interactive.TurnSubmissionDiagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, strings.Join([]string{diagnostic.Module, diagnostic.Code, diagnostic.Path, diagnostic.Message}, ":"))
	}
	return strings.Join(parts, "; ")
}

func interactiveTurnResultAlreadyAcceptedReceipt() interactive.TurnSubmissionReceipt {
	return interactive.TurnSubmissionReceipt{
		Ready: true,
		ModuleStatus: interactive.TurnSubmissionModuleStatus{
			StateChanges:   interactive.TurnSubmissionModuleAccepted,
			Choices:        interactive.TurnSubmissionModuleAccepted,
			PlanUpdate:     interactive.TurnSubmissionModuleAccepted,
			CharacterFacts: interactive.TurnSubmissionModuleAccepted,
		},
		Diagnostics: []interactive.TurnSubmissionDiagnostic{{
			Module:    "submission",
			Code:      "turn_result_already_accepted",
			Severity:  "warning",
			Retryable: false,
			Message:   "This turn already has a complete TurnResult; the first accepted modules were retained.",
		}},
	}
}

func (c *Conversation) baseParentIDSnapshot() *string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baseParentID == nil {
		return nil
	}
	value := *c.baseParentID
	return &value
}

func (c *Conversation) terminalOutcomeSnapshot(narrative string) *interactive.TerminalOutcome {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ruleResolution == nil || c.ruleResolution.TerminalCandidate == nil {
		return nil
	}
	candidate := c.ruleResolution.TerminalCandidate
	return &interactive.TerminalOutcome{
		Terminal:              true,
		Type:                  candidate.Type,
		Reason:                candidate.Reason,
		FinalNarrativeSummary: strings.TrimSpace(narrative),
		RuleResolutionID:      c.ruleResolution.ID,
	}
}

func (c *Conversation) LastTurnForState() (interactive.TurnEvent, bool, bool) {
	if c == nil {
		return interactive.TurnEvent{}, false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastTurn == nil {
		return interactive.TurnEvent{}, false, false
	}
	return *c.lastTurn, c.lastStateReady, true
}
