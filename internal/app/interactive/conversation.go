package interactiveapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	agent "ta/agent"

	"ta/config"
	agents "ta/internal/agents"
	agentcontext "ta/internal/agents/context"
	"ta/internal/agents/prompts"
	agentrun "ta/internal/agents/run"
	"ta/internal/agents/session"
	novaskills "ta/internal/agents/skills"
	"ta/internal/book/lore"
	"ta/internal/interactive"
	tellerpkg "ta/internal/interactive/teller"
	"ta/internal/storypack"
)

type Conversation struct {
	store            *interactive.Store
	novaDir          string
	workspace        string
	cfg              *config.Config
	storyID          string
	branchID         string
	user             string
	playerLetter     *interactive.PlayerLetterIntent
	inputVisibility  agentrun.InputVisibility
	replyTargetChars int
	// outputLanguage is the locale the player's page is reading in. It decides
	// the language of the turn's prose, which the world material does not.
	// Empty means "follow the material", which is what a request naming no
	// language gets.
	outputLanguage              string
	cycleStoryConfig            *interactive.StoryMeta
	modelContextAppendMu        sync.Mutex
	turnCheckMu                 sync.Mutex
	mu                          sync.Mutex
	lastTurn                    *interactive.TurnEvent
	lastStateReady              bool
	lastSources                 string
	lastContextSources          []interactiveContextSource
	lastContextLedgerParts      []agentcontext.AuditPart
	stableLeadingMessage        string
	assistantMetadata           session.MessageMetadata
	displayEvents               []interactive.DisplayEvent
	modelContextMessages        []interactive.ModelContextMessage
	modelContextBatchSequence   int
	ruleResolution              *interactive.RuleResolution
	turnDraft                   interactive.TurnDraft
	turnDraftLoaded             bool
	turnProtocol                interactiveTurnProtocol
	baseParentID                *string
	replaceTurnID               string
	pinParentAtExecution        bool
	agentCycleCommit            func(context.Context, agentrun.Outcome) error
	agentCycleIdentity          agentrun.CycleIdentity
	acceptedPlayerInputID       string
	pendingDomainCommit         *interactive.DomainCommitIntent
	lastDomainReceipt           *interactive.DomainCommitReceipt
	agentCompaction             *agent.CompactionState
	modelHistoryKey             string
	modelHistory                *interactive.StoryModelHistory
	openingStateSchemaDraft     *interactive.ActorStateSchemaBatchDraft
	openingStateSchemaAudit     interactive.ActorStateSchemaBatchAudit
	requireProtagonistSelection bool
	// offeredPeople is the author key of each person this turn's discovery page
	// printed as being at this place, keyed by normalized name. It is what lets a
	// first appearance be bound to the author's identity without the model
	// supplying a key, and it is per cycle because the page is built from the
	// turn's own location and request.
	offeredPeople map[string]string

	// draftCommit is supplied by a product execution host before admission.
	// Native uses its atomic checkpoint callback; external turns commit only
	// product-owned Story facts and never enter the Native lifecycle.
	draftCommit func(context.Context, interactive.TurnDraft, *agent.ToolResult) error
}

var _ novaskills.ExplicitResolver = (*Conversation)(nil)

func (c *Conversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return agentcontext.ContextBudgetForAgent(c.cfg, config.AgentKindInteractiveStory)
}

func (c *Conversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	if c == nil {
		return nil, nil
	}
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	return novaskills.ResolveConfiguredInvocations(ctx, cfg, config.AgentKindInteractiveStory, message)
}

// BindAgentCycleIdentity receives the coordinator-selected identity before
// the model cycle starts. The turn store persists it with the canonical game
// event, providing an idempotency key across the domain/runtime commit seam.
func (c *Conversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil {
		return
	}
	c.modelContextAppendMu.Lock()
	defer c.modelContextAppendMu.Unlock()
	c.mu.Lock()
	sameCycle := c.agentCycleIdentity == identity
	c.agentCycleIdentity = identity
	if !sameCycle {
		c.acceptedPlayerInputID = ""
		c.turnDraftLoaded = false
		c.turnDraft = interactive.TurnDraft{}
		c.turnProtocol = interactiveTurnProtocol{}
		c.ruleResolution = nil
		// The discovery page is rebuilt for each cycle from that turn's own
		// location and request, so a previous cycle's page must not bind this
		// turn's first appearances.
		c.offeredPeople = nil
	}
	c.modelContextMessages = nil
	c.modelContextBatchSequence = 0
	c.pendingDomainCommit = nil
	c.lastDomainReceipt = nil
	c.mu.Unlock()
}

func (c *Conversation) AgentCycleIdentitySnapshot() agentrun.CycleIdentity {
	if c == nil {
		return agentrun.CycleIdentity{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentCycleIdentity
}

func (c *Conversation) WithAgentCycleCommit(commit func(context.Context, agentrun.Outcome) error) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.agentCycleCommit = commit
		c.mu.Unlock()
	}
	return c
}

// CommitAgentCycle implements agentrun.CycleCommitter. The callback is
// immutable once execution begins and bridges the generic durable cycle
// boundary to the game domain's persisted-turn projection.
func (c *Conversation) CommitAgentCycle(ctx context.Context, outcome agentrun.Outcome) error {
	return c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, outcome)
}

func (c *Conversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if stage != agentrun.DomainCommitOutput || c.pendingDomainCommit == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	return agentrun.DomainCommitIntent{
		Identity: c.agentCycleIdentity, Stage: stage, Hash: c.pendingDomainCommit.Hash,
	}, true, nil
}

func (c *Conversation) CommitAgentCycleStage(ctx context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return nil
	}
	c.mu.Lock()
	pending := c.pendingDomainCommit
	commit := c.agentCycleCommit
	if outcome.Status != agentrun.OutcomeCompleted && outcome.Status != agentrun.OutcomePreempted {
		c.pendingDomainCommit = nil
		c.lastDomainReceipt = nil
		c.mu.Unlock()
		if commit != nil {
			return commit(ctx, outcome)
		}
		return nil
	}
	c.mu.Unlock()
	if pending == nil {
		if commit != nil {
			return commit(ctx, outcome)
		}
		return nil
	}
	receipt, err := c.store.CommitDomainTurn(c.storyID, *pending)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.pendingDomainCommit = nil
	c.lastDomainReceipt = &receipt
	turn := receipt.Turn
	c.lastTurn = &turn
	c.lastStateReady = turn.StateStatus == "ready"
	c.turnProtocol.markCommitted()
	c.mu.Unlock()
	if commit != nil {
		return commit(ctx, outcome)
	}
	return nil
}

func (c *Conversation) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitReceipt{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastDomainReceipt == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return agentrun.DomainCommitReceipt{
		Identity: c.agentCycleIdentity, Stage: stage,
		Hash: c.lastDomainReceipt.Hash, Revision: c.lastDomainReceipt.Revision,
	}, true
}

func NewConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *Conversation {
	return &Conversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
}

// WithOutputLanguage binds the locale the player's page is reading in, which is
// what the turn's prose follows. It is a separate call rather than a
// NewConversation parameter because every test that builds a conversation
// without a request behind it wants the zero value.
func (c *Conversation) WithOutputLanguage(locale string) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.outputLanguage = strings.TrimSpace(locale)
		c.mu.Unlock()
	}
	return c
}

// WithCycleStoryConfig freezes mutable story tuning for this model cycle.
// Branch metadata and committed story state remain live so queued execution
// can still pin the actual parent immediately before context assembly.
func (c *Conversation) WithCycleStoryConfig(meta interactive.StoryMeta) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.cycleStoryConfig = &meta
		c.mu.Unlock()
	}
	return c
}

// WithInputVisibility binds the accepted input's player-facing projection.
// Both visible and model-only inputs remain canonical game model context.
func (c *Conversation) WithInputVisibility(visibility agentrun.InputVisibility) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.inputVisibility = visibility
		c.mu.Unlock()
	}
	return c
}

// WithPlayerLetter binds the character-window draft to this one formal story
// turn. The store records it only with the accepted turn commit.
func (c *Conversation) WithPlayerLetter(intent *interactive.PlayerLetterIntent) *Conversation {
	if c == nil || intent == nil {
		return c
	}
	normalized := intent.Normalize()
	c.mu.Lock()
	c.playerLetter = &normalized
	c.mu.Unlock()
	return c
}

// WithRequiredProtagonistSelection guards only an unresolved autonomous
// opening. Ordinary legacy/default stories retain their existing turn flow.
func (c *Conversation) WithRequiredProtagonistSelection(required bool) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.requireProtagonistSelection = required
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithBaseParentID(parentID string) *Conversation {
	if c != nil {
		parentID = strings.TrimSpace(parentID)
		c.mu.Lock()
		c.baseParentID = &parentID
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithRegenerateTarget(turnID string) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.replaceTurnID = strings.TrimSpace(turnID)
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) regenerateTargetSnapshot() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.replaceTurnID
}

func (c *Conversation) storyContextForCycle() (interactive.StoryContext, error) {
	if c == nil || c.store == nil {
		return interactive.StoryContext{}, fmt.Errorf("互动故事不存在")
	}
	var (
		storyCtx interactive.StoryContext
		err      error
	)
	if target := c.regenerateTargetSnapshot(); target != "" {
		storyCtx, err = c.store.StoryContextAtTurnParent(c.storyID, c.branchID, target)
	} else {
		storyCtx, err = c.store.StoryContext(c.storyID, c.branchID)
	}
	if err != nil {
		return interactive.StoryContext{}, err
	}
	c.mu.Lock()
	cycleConfig := c.cycleStoryConfig
	c.mu.Unlock()
	if cycleConfig != nil {
		applyCycleStoryConfig(&storyCtx.Meta, *cycleConfig)
	}
	return storyCtx, nil
}

// pendingCombatHandoffFragment renders the battles this branch has fought but
// the main story has not yet absorbed.
//
// It is derived from the canonical journal on every cycle rather than cached,
// because the derivation is what makes a battle settle exactly once: the moment
// a Turn absorbs the fight, that Turn becomes the branch's latest Turn and the
// battle stops being pending. A cached "already told the model" flag could
// disagree with the journal after a restart; this cannot.
//
// A regeneration projects the prefix before the replaced Turn, so a battle
// fought in the turn being regenerated is deliberately not handed over again —
// the replacement turn writes that fight's aftermath itself.
func (c *Conversation) pendingCombatHandoffFragment() (string, error) {
	if c == nil || c.store == nil || c.regenerateTargetSnapshot() != "" {
		return "", nil
	}
	handoffs, err := c.store.PendingCombatHandoffs(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	return interactive.CombatHandoffFragment(handoffs), nil
}

func applyCycleStoryConfig(target *interactive.StoryMeta, source interactive.StoryMeta) {
	if target == nil {
		return
	}
	target.Title = source.Title
	target.TitleSource = source.TitleSource
	target.Origin = source.Origin
	target.StoryTellerID = source.StoryTellerID
	target.PlanningTemplateID = source.PlanningTemplateID
	target.StoryDirectorID = source.StoryDirectorID
	target.PlanningMode = source.PlanningMode
	target.ModuleRefs = source.ModuleRefs
	target.ReplyTargetChars = source.ReplyTargetChars
	target.ChoiceCount = source.ChoiceCount
	target.Opening = source.Opening
	target.ImageSettings = source.ImageSettings
	target.CheckSettings = source.CheckSettings
}

func (c *Conversation) WithExecutionParentPinning() *Conversation {
	if c != nil {
		c.mu.Lock()
		c.pinParentAtExecution = true
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithOpeningStateSchema(storyCtx interactive.StoryContext) *Conversation {
	if c == nil || !interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) || storyCtx.Meta.StateSchemaInitialization == nil || storyCtx.Meta.StateSchemaInitialization.Status != interactive.StateSchemaInitializationWaitingOpening || storyCtx.Meta.ActorStateSchema == nil || SnapshotTurnCount(storyCtx.Snapshot) > 0 {
		return c
	}
	trpgSourceIDs := make([]string, 0, len(storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates))
	for _, rule := range storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates {
		if id := strings.TrimSpace(rule.ID); id != "" {
			trpgSourceIDs = append(trpgSourceIDs, id)
		}
	}
	c.mu.Lock()
	c.openingStateSchemaDraft = interactive.NewOpeningActorStateSchemaBatchDraft(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem)
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{
		OpeningSourceIDs: []string{"opening-draft"},
		TRPGSourceIDs:    trpgSourceIDs,
		CurrentState:     storyCtx.Snapshot.State,
	}
	c.mu.Unlock()
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] enabled opening state schema draft story_id=%s branch_id=%s mode=%s base_revision=%d", c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Meta.StateSchemaPolicy.Mode, storyCtx.Meta.ActorStateSchema.Revision))
	return c
}

func (c *Conversation) refreshOpeningStateSchema(storyCtx interactive.StoryContext) {
	if c == nil || (interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) &&
		storyCtx.Meta.StateSchemaInitialization != nil &&
		storyCtx.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening &&
		SnapshotTurnCount(storyCtx.Snapshot) == 0) {
		return
	}
	c.mu.Lock()
	hadDraft := c.openingStateSchemaDraft != nil
	c.openingStateSchemaDraft = nil
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{}
	c.mu.Unlock()
	if hadDraft {
		slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] cleared stale opening state schema draft story_id=%s branch_id=%s turns=%d", c.storyID, storyCtx.Snapshot.BranchID, len(storyCtx.Snapshot.Turns)))
	}
}

// ObserveLoreRead receives only successful, visibility-filtered body reads.
// It cannot grant evidence to another run or to a story without an opening draft.
func (c *Conversation) ObserveLoreRead(ids []string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openingStateSchemaDraft == nil {
		return
	}
	seen := make(map[string]bool, len(c.openingStateSchemaAudit.ReviewedLoreIDs)+len(ids))
	for _, id := range c.openingStateSchemaAudit.ReviewedLoreIDs {
		seen[id] = true
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			c.openingStateSchemaAudit.ReviewedLoreIDs = append(c.openingStateSchemaAudit.ReviewedLoreIDs, id)
			seen[id] = true
		}
	}
}

func (c *Conversation) SubmitOpeningStateSchemaBatch(ctx context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
	if c == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.ActorStateSchemaBatchResult{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openingStateSchemaDraft == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("当前故事不需要 Game Agent 初始化状态结构")
	}
	result := c.openingStateSchemaDraft.SubmitStructureOnly(batch, c.openingStateSchemaAudit)
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] staged opening state schema story_id=%s branch_id=%s accepted=%d rejected=%d blocked=%d finalized=%t draft_items=%d", c.storyID, c.branchID, len(result.Accepted), len(result.Rejected), len(result.Blocked), result.Finalized, result.DraftAcceptedItems))
	return result, nil
}

func (c *Conversation) openingStateSchemaProposalSnapshot() *interactive.ActorStateSchemaProposal {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	proposal, ok := c.openingStateSchemaDraft.FinalProposal()
	if !ok {
		return nil
	}
	return &proposal
}

func (c *Conversation) effectiveTurnState(storyCtx interactive.StoryContext) (interactive.StoryDirectorActorStateSystem, map[string]any, error) {
	actorState := interactive.StoryDirectorActorStateSystem{}
	if storyCtx.Meta.ActorStateSchema != nil {
		actorState = storyCtx.Meta.ActorStateSchema.System
	} else {
		actorState = c.StoryRuntimeForMeta(storyCtx.Meta).ActorState
	}
	state := storyCtx.Snapshot.State
	c.mu.Lock()
	proposal, hasProposal := c.openingStateSchemaDraft.FinalProposal()
	draftRequired := c.openingStateSchemaDraft != nil
	c.mu.Unlock()
	if draftRequired && !hasProposal {
		return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("请先调用 initialize_story_state_schema 并完成 finalize，再提交开局状态")
	}
	if hasProposal {
		if storyCtx.Meta.ActorStateSchema == nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("故事缺少开局状态结构基线")
		}
		target, _, err := interactive.ApplyActorStateSchemaAdaptation(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem, proposal.Adaptation)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		initialState, err := interactive.BuildActorStateInitialSnapshot(target, storyCtx.Meta.InitialTraitRolls)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		return target, initialState, nil
	}
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) {
		rawActors, _ := state["actors"].(map[string]any)
		if len(rawActors) == 0 {
			initialState, err := interactive.BuildActorStateInitialSnapshot(actorState, storyCtx.Meta.InitialTraitRolls)
			if err != nil {
				return interactive.StoryDirectorActorStateSystem{}, nil, err
			}
			state = initialState
		}
	}
	return actorState, state, nil
}

type interactiveModelContextCommitState struct {
	storyContext         interactive.StoryContext
	baseParentID         *string
	sourceSummary        string
	contextSources       []interactiveContextSource
	contextLedgerParts   []agentcontext.AuditPart
	stableLeadingMessage string
}

// StableLeadingMessage returns the stable prefix captured by AssembleModelContext.
// Callers use it only when validating a manually prepared compaction candidate.
func StableLeadingMessage(commitState any) (string, bool) {
	state, ok := commitState.(interactiveModelContextCommitState)
	if !ok {
		return "", false
	}
	return state.stableLeadingMessage, true
}

func (c *Conversation) AssembleModelContext(ctx context.Context, originalMessage string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	_ = originalMessage
	if c == nil || c.store == nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	branch, ok := storyCtx.Meta.Branches[storyCtx.Snapshot.BranchID]
	if !ok {
		return agentcontext.ModelContextResult{}, fmt.Errorf("互动故事分支元数据不存在: %s", storyCtx.Snapshot.BranchID)
	}
	// Commands may wait behind an active cycle. Pin the compare-and-swap parent
	// at actual model-context assembly time, not HTTP acceptance time, so a
	// queued FollowUp sees the turn committed by the preceding cycle while still
	// rejecting unrelated branch writes that race the model run.
	c.mu.Lock()
	pinParent := c.pinParentAtExecution
	c.mu.Unlock()
	var baseParentID *string
	if pinParent && c.regenerateTargetSnapshot() == "" {
		parent := strings.TrimSpace(branch.Head)
		baseParentID = &parent
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	storyDirector := storyDirectorForSnapshot(c.StoryRuntimeForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	// The per-turn fragment comes from the story's resolved main purpose, so a
	// main style changed through the purpose config governs this turn's movement
	// as well as its voice. The teller definition above is the display identity.
	tellerTurnContextPrompt := c.styleFragments(storyCtx.Meta).MainTurn
	if strings.TrimSpace(tellerTurnContextPrompt) == "" {
		tellerTurnContextPrompt = teller.PromptForTargets(tellerpkg.TargetTurnContext)
	}
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	turnHistory := interactiveTurnHistory{Turns: append([]interactive.StoryModelTurn(nil), modelHistory.Turns...)}
	checkpointSummary := ""
	if activeCompaction != nil {
		checkpointSummary = strings.TrimSpace(activeCompaction.Summary)
	}
	branchPlan := ""
	if storyCtx.Meta.PlanningMode == interactive.StoryPlanningModeEnabled && storyCtx.Snapshot.BranchPlan != nil {
		branchPlan = storyCtx.Snapshot.BranchPlan.Markdown
	}
	// The established cast, the scene block and the lore working set all read the
	// same projection. Reading it once keeps the three blocks describing one
	// instant: a profile written between two reads would otherwise let the cast
	// name someone the scene block says is absent.
	characters, charactersOK := c.storyCharacterState(storyCtx)
	// The branch's world projection is read once for the same reason: the cast
	// block's ages and the scene block's formal interval are both readings of one
	// clock, and two reads could describe two instants.
	world := c.storyWorldState(storyCtx)
	ages := castAges(storyCtx, characters, world)
	protagonistAge := protagonistAgeFact(storyCtx, world)
	scene := interactive.StoryScene(storyCtx.Snapshot)
	loreRuntime, err := buildInteractiveStoryLoreContext(loreSelectionInput{
		Visibility:    interactive.StoryLoreVisibility(storyCtx.Meta, storyCtx.Snapshot.State),
		Workspace:     c.workspace,
		Scope:         storyCtx.Meta.LoreScope(),
		PlanMarkdown:  branchPlan,
		UserAction:    input.UserMessage,
		Location:      scene.Location,
		ContentHash:   packContentHash(storyCtx.Meta.PackBinding),
		Catalog:       loreSelectionCatalog(c, storyCtx),
		Scene:         scene,
		Characters:    characters,
		HasCharacters: charactersOK,
	})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	// Resident lore, its budget check and the revision are all read through the
	// story's own scope, so a second world sharing this workspace can never
	// appear in this story's context and its edits never invalidate this one.
	loreStore := lore.NewScopedStore(c.workspace, storyCtx.Meta.LoreScope()).WithVisibility(interactive.StoryLoreVisibility(storyCtx.Meta, storyCtx.Snapshot.State))
	residentSnapshot, err := assembleResidentLore(loreStore)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	if err := validateResidentLoreSnapshot(residentSnapshot, "interactive opening", interactiveResidentLoreMessageMaxBytes); err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	residentLore, loreRevision := residentSnapshot.Content, residentSnapshot.Revision
	ruleSummary := interactive.StoryRuleSummary(storyDirector, StoryRuntimeContextMaxBytes)
	actorStateRuntime := interactive.ActorStateRuntimeContextWithSettings(storyDirector.ActorState, storyCtx.Snapshot.State, StoryRuntimeContextMaxBytes, interactive.SnapshotClockSeconds(storyCtx.Snapshot), storyCtx.Meta.Calendar, storyCtx.Meta.LocalEntityLoadModes(), storyCtx.Meta.ChoiceCount)
	stateSchemaInitialization := interactive.OpeningGameStateSchemaInstruction(storyCtx.Meta)
	// Event material is injected whether or not planning is enabled: the pack's
	// event cards are the story's own reusable content, and the planning switch
	// governs only the branch plan. It arrives as a bounded directory, so no
	// card's body can be truncated, and read_event_card serves the full text.
	eventCatalog := interactive.EventCatalogMarkdown(storyDirector.EventPackages, StoryRuntimeContextMaxBytes)
	// The established cast is read from this branch's character projection, the
	// same record the character window writes. An answer the player gave in the
	// window is therefore already here when the story resumes, and a line the
	// story writes is recorded against a character who actually exists.
	castContext := c.characterCastContext(storyCtx, characters, charactersOK, ages)
	// The discoverable page is the cast block's counterpart: the cast is who this
	// branch has already turned into actors, this is who else the world's
	// verified catalog holds and how far away they are. It is read from the
	// story's own frozen catalog snapshot, so a story never sees people from a
	// catalog revision it did not start on.
	discovery := c.storyDiscoveryContext(storyCtx, input.UserMessage)
	if discovery.Failure != nil {
		slog.Warn("[interactive] discoverable people page withheld", "error", discovery.Failure)
	}
	discoveryContext := discovery.Block
	// The world page is what lets a natural input become a settlement rather than
	// only prose. It is read from the same registries the commit validates
	// against, so an id the narrator copies is one the rules accept.
	worldActions := c.storyWorldActionsContext(storyCtx, input.UserMessage)
	// The craft page is the world page's counterpart for the furnace: the craft
	// module names a method by id, and the narrator cannot invent one the rules
	// accept. It is empty when no method is registered, so a story without craft
	// content pays nothing for it.
	craftMethods := interactive.CraftMethodsContext(storyDirector.ActorState, storyCtx.Snapshot.State, interactive.DefaultActorID, interactive.StoryContextMaxBytes)
	// The scene block is short and is rebuilt every turn from committed state, so
	// it lives in the dynamic part of the context. Only the fixed rules and style
	// fragments are stable-prefix material.
	sceneContext := c.storySceneContext(storyCtx, characters, charactersOK, protagonistAge)
	communicationMode := ""
	if binding := storyCtx.Meta.PackBinding; binding != nil && binding.Gameplay != nil {
		communicationMode = binding.Gameplay.Communication
	}
	runtimeContext := prompts.InteractiveStoryRuntimeContext(prompts.InteractiveStoryPromptInput{
		Title:                     storyCtx.Meta.Title,
		Origin:                    storyCtx.Meta.Origin,
		CommunicationMode:         communicationMode,
		StoryTellerID:             storyCtx.Meta.StoryTellerID,
		BranchID:                  storyCtx.Snapshot.BranchID,
		ReplyTargetChars:          c.replyTargetChars,
		ChoiceCount:               storyCtx.Meta.ChoiceCount,
		BranchPlan:                branchPlan,
		PlanningEnabled:           storyCtx.Meta.PlanningMode == interactive.StoryPlanningModeEnabled,
		RuleChecksEnabled:         !storyDirector.ModuleRefs.RuleSystemDisabled && len(storyDirector.TRPGSystem.RuleTemplates) > 0,
		StoryRuleCatalog:          ruleSummary,
		EventCatalog:              eventCatalog,
		ActorState:                actorStateRuntime,
		StateSchemaInitialization: stateSchemaInitialization,
		LoreContext:               loreRuntime.Context,
		CastContext:               castContext,
		DiscoveryContext:          discoveryContext,
		WorldActions:              worldActions,
		CraftMethods:              craftMethods,
		SceneContext:              sceneContext,
		OutputLanguage:            c.outputLanguage,
	})
	cycleIdentity := c.AgentCycleIdentitySnapshot()
	modelProjection, err := BuildModelContextProjection(
		modelHistory, activeCompaction, storyCtx.Snapshot, c.ToolResultContextPolicy(), cycleIdentity,
	)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	history := modelProjection.Messages
	pendingInputMessages := modelProjection.PendingInputMessages
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	// A battle is committed as a side record and is deliberately absent from
	// model-visible turn history, so the branch head already knows about a fight
	// the narrator would otherwise be told nothing about. §2 of the handoff
	// design is explicit about the consequence: the story must not put a fled
	// enemy back on the road at full health. The fragment is derived from the
	// branch's own records here, server-side, and never from anything a client
	// supplied — §7 forbids trusting a frontend's account of a win or a loss.
	combatHandoff, err := c.pendingCombatHandoffFragment()
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	if combatHandoff != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_combat_handoff", Source: "interactive.combat_handoff", Title: "Battles Already Settled",
			Purpose: "carry the formal result of battles this branch has already fought into the story that must continue from them",
			Content: combatHandoff, Placement: agentcontext.PlacementFinalUserPrefix, Limit: interactive.CombatHandoffFragmentLimit, Included: true,
			Note: "source=branch combat_record events after the latest Turn; derived per cycle, never cached; turn-scoped because a Turn that absorbs the fight stops it being pending",
		})
	}
	protagonistContext := interactive.StoryProtagonistContext(storyCtx.Meta.Protagonist)
	if strings.TrimSpace(protagonistContext) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_story_protagonist", Source: "story.protagonist", Title: "Story Protagonist Profile",
			Purpose: "provide the immutable story-owned protagonist identity and backstory",
			Content: protagonistContext, Placement: agentcontext.PlacementLeadingMessage, Limit: StoryRuntimeContextMaxBytes, Included: true,
			Stability: agent.ContextStablePrefix,
			Note:      "source=StoryMeta.protagonist; lifecycle=immutable after first turn; actor_id=protagonist",
		})
	}
	// A story's own narrative_requirements used to be injected here as a third
	// writing-direction layer above the shared CREATOR.md. That layer is retired:
	// writing direction now comes from one shared document plus the chosen style,
	// so the stored text stays readable for anyone who wrote it but never reaches
	// the model again.
	if binding := storyCtx.Meta.PackBinding; binding != nil && c.novaDir != "" {
		writing, err := storypack.NewLibrary(c.novaDir).WritingPreferences(binding.PackID)
		if err != nil {
			slog.WarnContext(ctx, "Could not read world writing preferences", "pack_id", binding.PackID, "error", err)
		}
		if writing != "" {
			fragments = append(fragments, agentcontext.Fragment{
				ID: "interactive_world_writing", Source: "world.settings.writing_preferences", Title: "剧本包写作偏好",
				Purpose: "按本世界的视角和用词写正文，遵循共享创作红线和已确认的游戏事实",
				Content: writing, Placement: agentcontext.PlacementLeadingMessage,
				Limit: interactive.MaxStoryNarrativeRequirementsBytes, Included: true, Stability: agent.ContextStablePrefix,
			})
		}
	}
	if strings.TrimSpace(residentLore) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_resident_lore", Source: "interactive.resident_lore", Title: "Resident Lore",
			Purpose: "provide revisioned enabled resident lore for the current agent session",
			Content: residentLore, Placement: agentcontext.PlacementLeadingMessage, Limit: interactiveResidentLoreMessageMaxBytes, Included: true,
			Stability: agent.ContextStablePrefix,
			Note:      "source=enabled resident lore; lifecycle=replaceable stable prefix; revision=" + strings.TrimSpace(loreRevision),
		})
	}
	if strings.TrimSpace(tellerTurnContextPrompt) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_turn_rules", Source: "interactive.turn_rules", Title: "Storyteller Rules for This Turn",
			Purpose: "apply the selected storyteller rules to this game turn",
			Content: prompts.InteractiveStoryTurnContextRule(tellerTurnContextPrompt), Placement: agentcontext.PlacementFinalUserPrefix, Limit: StoryRuntimeContextMaxBytes, Included: true,
		})
	}
	if strings.TrimSpace(runtimeContext) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_runtime", Source: "interactive.runtime", Title: "Interactive Runtime Context for This Turn",
			Purpose: "provide bounded story state, branch plan, active lore, actor state, and turn policy",
			Content: runtimeContext, Placement: agentcontext.PlacementFinalUserPrefix, Limit: StoryRuntimeContextMaxBytes, Included: true,
		})
	}
	baseInstruction := prompts.InteractiveStoryTurnInstruction(input.UserMessage, "", "")
	history = append(history, agent.UserMessageWithAttachments(baseInstruction, input.Attachments))
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{Messages: history, Fragments: fragments})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	history = assembled.Messages
	stableLeadingMessage := ""
	residentVisible := residentLore
	for _, fragment := range assembled.Fragments {
		if fragment.Source == "interactive.resident_lore" {
			residentVisible = fragment.Content
			if fragment.Included && fragment.Content != "" {
				if fragment.Content == residentLore {
					c.ObserveLoreRead(residentSnapshot.IDs)
					c.mu.Lock()
					if c.openingStateSchemaDraft != nil {
						c.openingStateSchemaAudit.SourceLoreRevision = loreRevision
					}
					c.mu.Unlock()
				}
				stableLeadingMessage = agentcontext.StandaloneMessage(fragment.Title, fragment.Content, "")
			}
			break
		}
	}
	sourceParts := interactiveStoryContextSources(storyCtx.Meta.Title, storyCtx.Meta.Origin, protagonistContext, teller, checkpointSummary, branchPlan, residentVisible, loreRevision, loreRuntime.Context, loreRuntime.Note, ruleSummary, eventCatalog, actorStateRuntime, stateSchemaInitialization, sceneContext, turnHistory, input.UserMessage)
	if combatHandoff != "" {
		sourceParts = append(sourceParts, interactiveContextSource{
			Source: "CombatHandoff", Title: "Battles Already Settled",
			Purpose: "carry the formal result of battles this branch has already fought into the story that must continue from them",
			Content: combatHandoff, Note: "source=branch combat_record events after the latest Turn; derived per cycle, never cached", Limit: interactive.CombatHandoffFragmentLimit,
		})
	}
	for index, message := range pendingInputMessages {
		sourceParts = append(sourceParts, interactiveContextSource{
			Source: "InterruptedPlayerInput", Title: fmt.Sprintf("Accepted Player Input Without Narrative Output %d", index+1),
			Purpose: "retain accepted player intent after an interrupted cycle",
			Content: message, ExactMessage: true,
		})
	}
	sourceParts = resolveInteractiveContextSources(sourceParts, history)
	sourceSummary := interactiveContextSourceListSummary(sourceParts, assembled.Fragments)
	contextLedgerParts := interactiveContextLedgerParts(sourceParts, history, c.ToolResultContextPolicy())
	slog.InfoContext(ctx, fmt.Sprintf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s planning_template_id=%s teller_slots=%s teller_turn_context=%s history_checkpoint=%s branch_plan=%s turns=%d model_turns=%d history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		PartSummary(storyCtx.Meta.Title),
		PartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.PlanningTemplateID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		PartSummary(tellerTurnContextPrompt),
		PartSummary(checkpointSummary),
		PartSummary(branchPlan),
		modelHistory.TotalTurns,
		len(turnHistory.Turns),
		interactiveMessageListSummary(history),
		PartSummary(history[len(history)-1].Content),
		sourceSummary,
	))
	return agentcontext.ModelContextResult{
		Messages: history,
		Context:  assembled,
		CommitState: interactiveModelContextCommitState{
			storyContext: storyCtx, baseParentID: baseParentID, sourceSummary: sourceSummary,
			contextSources: cloneInteractiveContextSources(sourceParts), contextLedgerParts: contextLedgerParts,
			stableLeadingMessage: stableLeadingMessage,
		},
	}, nil
}

func interruptedPlayerInputModelMessage(input interactive.PlayerInputAcceptedEvent) string {
	return "[Accepted player input from an interrupted turn]\n" +
		"No narrative was produced for this input. Treat it as player intent, not as a completed Turn.\n\n" +
		strings.TrimSpace(input.Text)
}

func (c *Conversation) CommitModelInput(ctx context.Context, _ string, assembled agentcontext.ModelContextResult) error {
	if c == nil {
		return fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok := assembled.CommitState.(interactiveModelContextCommitState)
	if !ok {
		return fmt.Errorf("互动故事模型上下文缺少提交状态")
	}
	if state.baseParentID != nil {
		c.WithBaseParentID(*state.baseParentID)
	}
	c.refreshOpeningStateSchema(state.storyContext)
	c.mu.Lock()
	c.lastSources = state.sourceSummary
	c.lastContextSources = cloneInteractiveContextSources(state.contextSources)
	c.lastContextLedgerParts = append([]agentcontext.AuditPart(nil), state.contextLedgerParts...)
	c.stableLeadingMessage = state.stableLeadingMessage
	c.mu.Unlock()
	return nil
}

// MaterializeAgentCanonicalInput records the accepted player input before
// model-context assembly. The matching live cycle is excluded from interrupted
// input projection when its context is assembled immediately afterwards.
func (c *Conversation) MaterializeAgentCanonicalInput(
	ctx context.Context,
	message string,
	attachments []agent.Attachment,
	checkpoint agent.CanonicalCheckpoint,
) (interactive.PlayerInputReceipt, error) {
	if c == nil || c.store == nil {
		return interactive.PlayerInputReceipt{}, fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	identity := c.AgentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return interactive.PlayerInputReceipt{}, fmt.Errorf("canonical game input requires an exact Agent cycle identity")
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, c.branchID, message)
	if err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	intent, err = intent.WithAttachments(attachments)
	if err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	c.mu.Lock()
	inputVisibility := c.inputVisibility
	c.mu.Unlock()
	if inputVisibility == agentrun.InputModelOnly {
		intent, err = intent.WithContextOnly()
		if err != nil {
			return interactive.PlayerInputReceipt{}, err
		}
	}
	intent.Checkpoint = checkpoint
	receipt, err := c.store.CommitPlayerInput(c.storyID, intent)
	if err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	c.mu.Lock()
	c.acceptedPlayerInputID = receipt.Event.ID
	c.mu.Unlock()
	return receipt, nil
}

func (c *Conversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSources
}

func (c *Conversation) ContextLedgerParts() []agentcontext.AuditPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agentcontext.AuditPart(nil), c.lastContextLedgerParts...)
}

func (c *Conversation) ContextLedgerPartsForMessages(messages []*agents.Message) []agentcontext.AuditPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	sources := cloneInteractiveContextSources(c.lastContextSources)
	c.mu.Unlock()
	parts := interactiveContextLedgerParts(sources, messages, c.ToolResultContextPolicy())
	c.mu.Lock()
	c.lastContextLedgerParts = append([]agentcontext.AuditPart(nil), parts...)
	c.mu.Unlock()
	return parts
}

func (c *Conversation) RunTraceMetadata() agentrun.TraceMetadata {
	if c == nil {
		return agentrun.TraceMetadata{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := agentrun.TraceMetadata{StoryID: c.storyID, BranchID: c.branchID}
	if c.lastTurn != nil {
		metadata.BranchID = c.lastTurn.BranchID
		metadata.TurnID = c.lastTurn.ID
	}
	return metadata
}
