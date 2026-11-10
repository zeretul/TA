package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"ta/config"
	agentattachment "ta/internal/agents/attachment"
	"ta/internal/agents/conversationconfig"
	agentrun "ta/internal/agents/run"
	interactiveapp "ta/internal/app/interactive"
	appsettings "ta/internal/app/settings"
	apptask "ta/internal/app/task"
	"ta/internal/interactive"
	"ta/internal/storypack"
)

// InteractiveAppService owns stories, branches, Game Planning, and Game Agent tasks.
type InteractiveAppService struct {
	app       *App
	admission sync.RWMutex
	starts    interactiveStartRegistry
}

// InteractiveTurnPersistedEvent is emitted after a game-mode turn is durably
// appended, allowing the UI to merge the new turn without a blocking snapshot
// reload.
type InteractiveTurnPersistedEvent struct {
	StoryID           string                                   `json:"story_id"`
	BranchID          string                                   `json:"branch_id"`
	TurnCount         int                                      `json:"turn_count"`
	Turn              interactive.TurnEvent                    `json:"turn"`
	BranchPlan        *interactive.BranchPlan                  `json:"branch_plan,omitempty"`
	State             map[string]any                           `json:"state"`
	Graph             interactive.StoryGraph                   `json:"graph"`
	Branches          []interactive.BranchSummary              `json:"branches"`
	ContextCompaction *interactive.ContextCompactionProjection `json:"context_compaction"`
	// Clock is the branch's formal time axis at this commit. The history list
	// dates each entry from it instead of inventing a calendar the world pack has
	// not defined.
	Clock *interactive.StoryClock `json:"clock,omitempty"`
}

func (a *App) InteractiveStories() (interactive.Index, error) {
	return a.interactiveService().InteractiveStories()
}

func (s *InteractiveAppService) InteractiveStories() (interactive.Index, error) {
	store := s.store()
	if store == nil {
		return interactive.Index{}, ErrNoWorkspace
	}
	return store.Index()
}

func (a *App) SelectInteractiveStory(storyID string) error {
	return a.interactiveService().SelectInteractiveStory(storyID)
}

func (s *InteractiveAppService) SelectInteractiveStory(storyID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-story] persist current story selection story_id=%s", storyID))
	if err := store.SelectStory(storyID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] persist current story selection failed story_id=%s err=%v", storyID, err))
		return err
	}
	return nil
}

func (a *App) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStoryContext(context.Background(), req)
}

func (s *InteractiveAppService) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return s.CreateInteractiveStoryContext(context.Background(), req)
}

func (a *App) CreateInteractiveStoryContext(ctx context.Context, req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStoryContext(ctx, req)
}

func (s *InteractiveAppService) CreateInteractiveStoryContext(ctx context.Context, req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	var err error
	req.Protagonist, err = s.resolveStoryProtagonist(ctx, req.Protagonist, interactive.StoryPackBindingScope(req.PackBinding))
	if err != nil {
		return interactive.StorySummary{}, err
	}
	// The intimacy purpose names a profile the local configuration holds. A
	// binding that points at nothing is refused here rather than stored and
	// discovered at the first generation: a story that saved a dead profile
	// would have to either fail silently or fall back to the main model, and
	// §4.3 of the window design forbids the latter.
	if err := s.validateStoryIntimacyBinding(req.IntimacySettings); err != nil {
		return interactive.StorySummary{}, err
	}
	req, err = s.withStoryCreationDefaults(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	// A ready-made character resolves after the defaults, because only then is
	// the bound actor-state module's own template in hand to seed, and before
	// CreateStory, which is where the initial state becomes the first delta.
	req, err = s.withPackTemplate(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	req.StoryTellerID = s.gameTellerID(req.StoryTellerID)
	if req.RuntimeConfig == nil {
		a := s.app
		a.mu.RLock()
		runtimeCfg := config.Config{}
		workspace := strings.TrimSpace(a.workspace)
		if a.cfg != nil {
			runtimeCfg = *a.cfg
		}
		a.mu.RUnlock()
		if workspace == "" {
			return interactive.StorySummary{}, ErrNoWorkspace
		}
		runtimeCfg, err = refreshConversationRuntimeConfig(runtimeCfg, workspace, runtimeCfg.ProjectStoreDir)
		if err != nil {
			return interactive.StorySummary{}, err
		}
		var seed conversationconfig.Config
		var seedErr error
		if req.CustomAgentID == nil {
			seed, seedErr = interactiveapp.RecentConversationSeed(store, &runtimeCfg, "")
		} else {
			seed, seedErr = conversationconfig.DefaultWithCustomAgent(&runtimeCfg, config.AgentKindInteractiveStory, *req.CustomAgentID)
		}
		if seedErr != nil {
			return interactive.StorySummary{}, seedErr
		}
		profileID := strings.TrimSpace(req.ProfileID)
		thinkingLevel := strings.TrimSpace(req.ThinkingLevel)
		if profileID != "" || thinkingLevel != "" {
			patch := conversationconfig.Patch{}
			if profileID != "" {
				patch.ProfileID = &profileID
			}
			if thinkingLevel != "" {
				patch.ThinkingLevel = &thinkingLevel
			}
			seed, seedErr = conversationconfig.Merge(&runtimeCfg, seed, patch)
			if seedErr != nil {
				return interactive.StorySummary{}, seedErr
			}
		}
		req.RuntimeConfig = &seed
	}
	story, err := store.CreateStory(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	return story, nil
}

func (a *App) RollInteractiveActorTraits(req interactive.ActorTraitRollRequest) (interactive.ActorTraitRollResult, error) {
	return a.interactiveService().RollInteractiveActorTraits(req)
}

func (s *InteractiveAppService) RollInteractiveActorTraits(req interactive.ActorTraitRollRequest) (interactive.ActorTraitRollResult, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorTraitRollResult{}, ErrNoWorkspace
	}
	actorStateID := strings.TrimSpace(req.ActorStateID)
	if actorStateID == "" {
		actorStateID = interactive.DefaultActorStateModuleID
	}
	actorState, err := interactive.NewActorStateLibrary(cfg.DataDir()).Get(actorStateID)
	if err != nil {
		return interactive.ActorTraitRollResult{}, err
	}
	req.ActorStateID = actorStateID
	return interactive.RollActorTraits(actorState.ActorState, req)
}

// freezePackCatalog takes the discoverable-people index of the pack revision a
// story is starting on.
//
// It runs once, at creation. A story with no pack binding has no catalog, and a
// pack whose content cannot be read yields no snapshot: the story still starts,
// and the discovery path then reports the missing catalog rather than silently
// pretending the world holds nobody. Binding by identity rather than by name is
// what makes this the story's own revision even if the pack file is edited
// between the binding being computed and the story being written.
func (s *InteractiveAppService) freezePackCatalog(binding *interactive.StoryPackBinding) *interactive.PackCatalogSnapshot {
	if binding == nil || strings.TrimSpace(binding.PackID) == "" {
		return nil
	}
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil
	}
	pack, err := storypack.NewLibrary(cfg.DataDir()).Get(binding.PackID)
	if err != nil {
		slog.Warn("[interactive] story started without a catalog snapshot", "pack_id", binding.PackID, "error", err)
		return nil
	}
	if binding.ContentHash != "" && pack.ContentHash() != binding.ContentHash {
		// The binding names a revision the file no longer holds. Taking the current
		// file's catalog would start the story on people it never bound to, so no
		// snapshot is written and the discovery path reports the mismatch instead.
		slog.Warn("[interactive] story started on a catalog revision it did not bind",
			"pack_id", binding.PackID,
			"binding_hash", binding.ContentHash, "pack_hash", pack.ContentHash())
		return nil
	}
	if len(pack.Characters) == 0 {
		return nil
	}
	return catalogSnapshot(pack.FreezeCatalog())
}

func (s *InteractiveAppService) withStoryCreationDefaults(req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	if strings.TrimSpace(req.PlanningMode) == "" {
		req.PlanningMode = interactive.StoryPlanningModeEnabled
	}
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return req, nil
	}
	templateID := interactive.NormalizeGamePlanningTemplateID(req.PlanningTemplateID)
	if templateID == "" {
		templateID = interactive.DefaultGamePlanningTemplateID
	}
	req.PlanningTemplateID = templateID
	if _, err := interactive.NewGamePlanningTemplateLibrary(cfg.DataDir()).Get(templateID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[game-planning] load template failed planning_template_id=%s err=%v", templateID, err))
		req.PlanningTemplateID = interactive.DefaultGamePlanningTemplateID
	}
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if req.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
	}
	// Refuse a forged pairing before resolving it. ResolveStoryDirectorModules
	// adopts the rule system's own actor-state link, so a request asking for two
	// systems that do not work together would come back quietly repaired: the
	// player would be handed a different state system than the one they chose and
	// nothing would say so. The store refuses the same pairing too, but by then
	// this layer has already rewritten the request.
	if err := interactive.ValidateStoryModuleCompatibility(cfg.DataDir(), refs); err != nil {
		return req, err
	}
	runtime := interactive.DefaultStoryDirector()
	runtime.ModuleRefs = refs
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	runtime = interactive.ResolveStoryDirectorModules(cfg.DataDir(), runtime)
	normalized := interactive.NormalizeStoryDirectorModuleRefs(runtime.ModuleRefs)
	req.ModuleRefs = &normalized
	// The narrative style is not taken from the module graph. It is the player's
	// own selection, and a story created from a pack must be told in the style
	// that is currently chosen rather than one the pack or a preset happens to
	// name. An explicit selection is honoured below by the story store's own
	// default; nothing here overrides it.
	if interactive.StoryDirectorImagePresetEnabled(runtime) && strings.TrimSpace(req.ImageSettings.PresetID) == "" && strings.TrimSpace(runtime.ModuleRefs.ImagePresetID) != "" {
		req.ImageSettings.PresetID = strings.TrimSpace(runtime.ModuleRefs.ImagePresetID)
	}
	policy := interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeAdaptTemplate}
	if req.StateSchemaPolicy != nil {
		policy = interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	}
	req.StateSchemaPolicy = &policy
	actorState := runtime.ActorState
	if policy.Mode == interactive.StoryStateSchemaModeGenerate {
		actorState = interactive.GeneratedStoryActorStateCore()
	}
	if err := s.attachPackDefinitionDetails(&actorState, req.PackBinding); err != nil {
		return req, err
	}
	if len(actorState.Templates) == 0 && interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(&policy) {
		return req, fmt.Errorf("故事状态模板不可用 / Story state template is unavailable")
	}
	if len(actorState.Templates) > 0 {
		req.ActorState = &actorState
	} else {
		req.ActorState = nil
	}
	req.TRPGSystem = &runtime.TRPGSystem
	// Freeze the module graph this story starts with. Without it every later
	// turn re-resolves event packages and rules from the live preset catalog, so
	// editing a preset file would silently rewrite a story already in progress.
	// The frozen graph is what later turns read; a preset edit then affects the
	// next game instead of this one.
	frozen := runtime.ResolvedSnapshot
	frozen.ActorState = actorState
	req.FrozenModules = &frozen
	// Freeze the world's discoverable-people index for the same reason and at the
	// same moment. The world book this story reads is already scoped to the pack
	// revision that wrote it, but the catalog was only ever in the pack file, so
	// an edit used to make an in-progress story lose every preset person. The
	// snapshot is taken from the bound revision, which is the pack the request's
	// binding names, and it is the revision this story will keep reading.
	req.PackCatalog = s.freezePackCatalog(req.PackBinding)
	// Freeze the calendar beside the catalog, and for the same reason: a year
	// length is what a lifespan is counted with, so it must belong to the story
	// rather than to whichever pack revision is on disk later.
	//
	// Every new story gets the confirmed first-version calendar. The design says
	// 没有时，本轮提出的最小首版自制历法，and 落入世界包配置并按故事冻结 — a pack
	// that ships no calendar is the ordinary case, and refusing to start a game
	// without one would leave the lifespan table permanently unreachable. What
	// the design forbids is applying it to a story already under way, and that is
	// why this is written at creation only: a story whose meta has no calendar
	// keeps none.
	if req.Calendar == nil {
		calendar := interactive.DefaultStoryCalendar()
		req.Calendar = &calendar
	}
	// The protagonist's own age is read here rather than in the store because the
	// parser lives with the pack, and internal/interactive deliberately does not
	// depend on it. Zero stays zero: a body that states no age is an unknown
	// lifespan, never an assumed adult.
	if req.ProtagonistAge <= 0 {
		req.ProtagonistAge = storypack.ParseAge(req.Protagonist.Profile)
	}
	// The protagonist's gender is read the same way and at the same moment: only
	// an explicit statement in the authored body counts, and a body that states
	// none leaves it empty. It is never inferred from a name or a portrait,
	// because a rule that reads it (the intimacy window's own eligibility) must
	// be able to refuse rather than guess.
	if req.ProtagonistGender == "" {
		req.ProtagonistGender = storypack.ParseGender(req.Protagonist.Profile)
	}
	status := interactive.StateSchemaInitializationWaitingOpening
	outcome := ""
	if policy.Mode == interactive.StoryStateSchemaModeFixedTemplate {
		status = interactive.StateSchemaInitializationReady
		outcome = "fixed"
	}
	req.StateSchemaInitialization = &interactive.StateSchemaInitializationStatus{
		Mode:         policy.Mode,
		Status:       status,
		Outcome:      outcome,
		BaseRevision: 1,
	}
	if status == interactive.StateSchemaInitializationReady {
		req.StateSchemaInitialization.TargetRevision = 1
	}
	return req, nil
}

func (a *App) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().UpdateInteractiveStory(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	a := s.app
	// Runtime tuning is frozen by the active cycle and can be saved immediately.
	// Opening structure changes affect live tools, so they require an idle story.
	updatesOpeningStructure := req.Protagonist != nil || req.StateSchemaPolicy != nil
	if req.ModuleRefs != nil {
		current, err := store.StoryContext(storyID, "")
		if err != nil {
			return interactive.StorySummary{}, err
		}
		updatesOpeningStructure = updatesOpeningStructure || current.Meta.ModuleRefs == nil || current.Meta.ModuleRefs.RuleSystemID != req.ModuleRefs.RuleSystemID
	}
	if updatesOpeningStructure {
		a.mu.RLock()
		active := interactiveTaskForScopeLocked(a, a.workspace, storyID, "")
		a.mu.RUnlock()
		if active != nil {
			return interactive.StorySummary{}, ErrAgentOperationActive
		}
	}
	if req.Protagonist != nil {
		// An update re-resolves the protagonist against the story's existing
		// binding, so editing a story never repoints it at another world.
		scope := ""
		if current, err := s.store().StoryContext(storyID, ""); err == nil {
			scope = current.Meta.LoreScope()
		}
		protagonist, err := s.resolveStoryProtagonist(context.Background(), *req.Protagonist, scope)
		if err != nil {
			return interactive.StorySummary{}, err
		}
		req.Protagonist = &protagonist
	}
	if req.IntimacySettings != nil {
		if err := s.validateStoryIntimacyBinding(*req.IntimacySettings); err != nil {
			return interactive.StorySummary{}, err
		}
	}
	if req.StateSchemaPolicy != nil {
		var err error
		req, err = s.withStoryStateSchemaUpdateDefaults(req)
		if err != nil {
			return interactive.StorySummary{}, err
		}
	}
	if strings.TrimSpace(req.StoryTellerID) != "" {
		req.StoryTellerID = s.gameTellerID(req.StoryTellerID)
	}
	if !updatesOpeningStructure {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.interactive != store {
			return interactive.StorySummary{}, ErrAgentContextChanged
		}
		return store.UpdateStory(storyID, req)
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return interactive.StorySummary{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.StorySummary{}, err
	}
	return store.UpdateStory(storyID, req)
}

func (s *InteractiveAppService) gameTellerID(tellerID string) string {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return strings.TrimSpace(tellerID)
	}
	if teller := interactiveapp.LoadGameTeller(cfg.DataDir(), strings.TrimSpace(tellerID)); teller.ID != "" {
		return teller.ID
	}
	return strings.TrimSpace(tellerID)
}

func (s *InteractiveAppService) withStoryStateSchemaUpdateDefaults(req interactive.UpdateStoryRequest) (interactive.UpdateStoryRequest, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" || req.StateSchemaPolicy == nil {
		return req, nil
	}
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if req.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
	}
	// Same hazard as at creation: resolving adopts the rule system's actor-state
	// link, so a forged pairing has to be refused while it is still the one the
	// request named.
	if err := interactive.ValidateStoryModuleCompatibility(cfg.DataDir(), refs); err != nil {
		return req, err
	}
	runtime := interactive.DefaultStoryDirector()
	runtime.ModuleRefs = refs
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	runtime = interactive.ResolveStoryDirectorModules(cfg.DataDir(), runtime)
	normalized := interactive.NormalizeStoryDirectorModuleRefs(runtime.ModuleRefs)
	req.ModuleRefs = &normalized
	policy := interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	actorState := runtime.ActorState
	if policy.Mode == interactive.StoryStateSchemaModeGenerate {
		actorState = interactive.GeneratedStoryActorStateCore()
	}
	if len(actorState.Templates) == 0 && interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(&policy) {
		return req, fmt.Errorf("故事状态模板不可用 / Story state template is unavailable")
	}
	req.StateSchemaPolicy = &policy
	if len(actorState.Templates) > 0 {
		req.ActorState = &actorState
	} else {
		req.ActorState = nil
	}
	req.TRPGSystem = &runtime.TRPGSystem
	status := interactive.StateSchemaInitializationWaitingOpening
	outcome := ""
	if policy.Mode == interactive.StoryStateSchemaModeFixedTemplate {
		status = interactive.StateSchemaInitializationReady
		outcome = "fixed"
	}
	req.StateSchemaInitialization = &interactive.StateSchemaInitializationStatus{Mode: policy.Mode, Status: status, Outcome: outcome, BaseRevision: 1}
	if status == interactive.StateSchemaInitializationReady {
		req.StateSchemaInitialization.TargetRevision = 1
	}
	return req, nil
}

func (a *App) DeleteInteractiveStory(storyID string) error {
	return a.interactiveService().DeleteInteractiveStory(storyID)
}

func (s *InteractiveAppService) DeleteInteractiveStory(storyID string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.interactive
	sessionStore := a.sessionStore
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStoreDir
	}
	if store == nil {
		return ErrNoWorkspace
	}
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	if fence.chat == nil {
		return ErrNoWorkspace
	}
	if err := fence.chat.DeleteStoryBindings(context.Background(), fence.projectID, storyID, ""); err != nil {
		return err
	}
	if err := store.DeleteStory(storyID); err != nil {
		return err
	}
	if err := agentattachment.RemoveScope(stateRoot, agentattachment.StoryScope(storyID)); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] remove deleted Story attachments failed story_id=%s error=%v", storyID, err))
	}
	if sessionStore != nil {
		return sessionStore.DeleteByPrefix("interactive-story-" + storyID + "-")
	}
	return nil
}

func (a *App) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	return a.interactiveService().InteractiveSnapshot(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	store := s.store()
	if store == nil {
		return interactive.Snapshot{}, ErrNoWorkspace
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return interactive.Snapshot{}, err
	}
	s.app.mu.RLock()
	workspace := s.app.workspace
	projectID := ""
	if s.app.cfg != nil {
		projectID = s.app.cfg.ProjectID
	}
	executionRuntime := s.app.executionRuntime
	s.app.mu.RUnlock()
	// Story Store owns every user-visible game fact. Agent Session contributes
	// an optional runtime projection from that same journal, so an unavailable
	// projection must never hide already committed turns and state from the user.
	snapshot.ContextCompaction = nil
	if executionRuntime == nil {
		redactInteractiveSnapshotAttachmentPaths(&snapshot)
		projectInteractiveSnapshotForPlayer(&snapshot, loreAudienceForSnapshot(store, snapshot))
		return snapshot, nil
	}
	status, projected := projectAgentRuntime(context.Background(), executionRuntime, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: projectID, Workspace: workspace,
		StoryID: storyID, BranchID: snapshot.BranchID, Mode: "interactive",
	})
	if !projected {
		redactInteractiveSnapshotAttachmentPaths(&snapshot)
		projectInteractiveSnapshotForPlayer(&snapshot, loreAudienceForSnapshot(store, snapshot))
		return snapshot, nil
	}
	snapshot.ContextCompaction, err = interactiveapp.ProjectAgentCompaction(
		status.Compaction, storyID, snapshot.BranchID,
	)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf(
			"[interactive-snapshot] ignore invalid optional Agent Compaction projection workspace=%s story_id=%s branch_id=%s err=%v",
			workspace, storyID, snapshot.BranchID, err,
		))
		snapshot.ContextCompaction = nil
	}
	redactInteractiveSnapshotAttachmentPaths(&snapshot)
	projectInteractiveSnapshotForPlayer(&snapshot, loreAudienceForSnapshot(store, snapshot))
	return snapshot, nil
}

func (a *App) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	return a.interactiveService().InteractiveHistoryPage(storyID, branchID, beforeCursor, limit)
}

func (s *InteractiveAppService) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryHistoryPage{}, ErrNoWorkspace
	}
	page, err := store.ReadHistoryPage(storyID, branchID, beforeCursor, limit)
	if err != nil {
		return interactive.StoryHistoryPage{}, err
	}
	// The boundary is enforced where the data leaves the process, and an older
	// page carries the same turns the snapshot does. Reading the schema the page
	// was committed under is what lets the projection withhold the same private
	// writes here rather than only on the newest page.
	schema, err := store.ActorStateSchema(storyID, page.BranchID)
	if err != nil {
		return interactive.StoryHistoryPage{}, err
	}
	for index := range page.Turns {
		page.Turns[index] = playerKnownTurn(page.Turns[index], schema)
		page.Turns[index].Attachments = attachmentDescriptors(page.Turns[index].Attachments)
	}
	return page, nil
}

func (a *App) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	return a.interactiveService().RerollInteractiveRuleResolution(storyID, resolutionID, req)
}

func (s *InteractiveAppService) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.RuleResolution{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.RuleResolution{}, err
	}
	return store.RerollRuleResolution(storyID, resolutionID, req)
}

// ActiveInteractiveTask 返回当前游戏模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *apptask.Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *apptask.Task {
	task, _ := s.ActiveInteractiveTaskFor("", "")
	return task
}

func (s *InteractiveAppService) store() *interactive.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.interactive
}

func (s *InteractiveAppService) interactiveRuntimeConfig() (*interactive.Store, config.Config, string, error) {
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.cfg == nil {
		a.mu.RUnlock()
		return nil, config.Config{}, "", ErrNoWorkspace
	}
	store := a.interactive
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.RUnlock()

	if layered, err := config.LoadLayeredWithStartupConfigAt(
		novaDir, workspace, config.ProjectConfigPath(runtimeCfg.ProjectStoreDir),
	); err == nil {
		appsettings.ApplyLayered(&runtimeCfg, layered)
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load layered settings failed workspace=%s err=%v", workspace, err))
	}
	return store, runtimeCfg, workspace, nil
}

func (s *InteractiveAppService) cfg() *config.Config {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// validateStoryIntimacyBinding refuses a binding that names a profile the local
// configuration does not have. An empty profile is legitimate: it means the
// player has not chosen a dedicated model yet, and the window still shows the
// history it already has.
//
// The check reads the same stored profiles the settings UI edits. It never
// copies a credential anywhere; it only asks whether the ID resolves.
func (s *InteractiveAppService) validateStoryIntimacyBinding(settings interactive.StoryIntimacySettings) error {
	profileID := strings.TrimSpace(settings.ProfileID)
	if profileID == "" {
		return nil
	}
	cfg := s.cfg()
	if cfg == nil {
		return interactive.ErrIntimacyModelProfile
	}
	if !config.ModelProfileExists(cfg, profileID) {
		return fmt.Errorf("%w: %s", interactive.ErrIntimacyModelProfile, profileID)
	}
	return nil
}
