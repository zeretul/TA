package interactive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"ta/internal/character"
	interactivestate "ta/internal/interactive/state"
)

const (
	// TurnSubmissionModuleRecap is the turn's own short recollection of what it
	// just published. It is deliberately not part of the module status machine:
	// a turn whose recap is missing or malformed still publishes, and the history
	// then shows an excerpt of the prose instead. It is display material rebuilt
	// from the committed turn, never a second record of what happened.
	TurnSubmissionModuleRecap = "recap"

	TurnSubmissionModuleStateChanges   = "state_changes"
	TurnSubmissionModuleChoices        = "choices"
	TurnSubmissionModulePlanUpdate     = "plan_update"
	TurnSubmissionModuleCharacterFacts = "character_facts"
	TurnSubmissionModuleCultivation    = "cultivation"
	TurnSubmissionModuleSkills         = "skills"
	TurnSubmissionModuleTechniques     = "techniques"
	TurnSubmissionModuleItems          = "items"
	TurnSubmissionModuleTrade          = "trade"
	TurnSubmissionModuleRest           = "rest"
	TurnSubmissionModuleCraft          = "craft"
	TurnSubmissionModuleWorld          = "world"

	TurnSubmissionModuleAccepted = "accepted"
	TurnSubmissionModuleRejected = "rejected"
	TurnSubmissionModuleMissing  = "missing"

	TurnSubmissionDiagnosticInvalidJSON            = "invalid_json"
	TurnSubmissionDiagnosticInvalidTopLevel        = "invalid_top_level"
	TurnSubmissionDiagnosticInvalidModule          = "invalid_module"
	TurnSubmissionDiagnosticChoiceCountMismatch    = "choice_count_mismatch"
	TurnSubmissionDiagnosticDuplicateChoice        = "duplicate_choice"
	TurnSubmissionDiagnosticEmptyChoice            = "empty_choice"
	TurnSubmissionDiagnosticStoryContextRequired   = "story_context_required"
	TurnSubmissionDiagnosticInitialStateIncomplete = "initial_state_incomplete"
	TurnSubmissionDiagnosticInvalidPlanMode        = "invalid_plan_update_mode"
	// TurnSubmissionDiagnosticWorldRefused means the rules will refuse the world
	// action this turn proposes. It is not a malformed module: the shape is
	// valid, but the road does not start here, the task was never offered, the
	// event is closed or the realm's entrance is not this place. The turn has to
	// rewrite its prose to show the real outcome, so the state_changes that
	// described the refused success are un-accepted with it.
	TurnSubmissionDiagnosticWorldRefused = "world_action_refused"
	// TurnSubmissionDiagnosticCraftRefused means the rules will not work the batch
	// this turn proposes: the method is not registered, the stage is too low, the
	// interval is shorter than one batch, or the materials are not in the pack.
	// Nothing is spent and nothing is produced, so prose that says the work
	// succeeded cannot be published.
	TurnSubmissionDiagnosticCraftRefused = "craft_refused"
	// TurnSubmissionDiagnosticCraftLost means the batch really ran and really
	// failed. The materials are spent and the interval has passed, but the method's
	// loss rate took the product. The module stays accepted — this is a real
	// outcome — and only the narrative has to be rewritten to show it.
	TurnSubmissionDiagnosticCraftLost = "craft_batch_lost"

	turnSubmissionSeverityError = "error"

	maxTurnSubmissionDiagnostics       = 8
	maxTurnSubmissionDiagnosticMessage = 1024
	// The allowed-field list is what the model reads to decide which keys a new
	// Actor may carry. It used to stop at sixteen, which cut the list off in the
	// middle of the cultivation preset's own fields: the diagnostic for a refused
	// key then named a set that ended at 主法器 and never mentioned 已学功法,
	// 重要物品 or 技能与能力. An opponent whose create was refused twice for an
	// unrelated key dropped exactly those fields on the retry, and arrived with a
	// blade missing. The byte bound below is the real limit; this one only has to
	// be larger than any template's field count.
	maxTurnSubmissionAllowedFields  = 64
	maxTurnSubmissionArgumentsBytes = 256 * 1024
	maxTurnSubmissionChoiceBytes    = 512
	// A recap is a short recollection, not a second telling. The reference length
	// is 40–100 characters; the bound below only has to stop a model that pasted
	// the prose back in, and a longer one is trimmed rather than refused.
	maxTurnRecapBytes = 960
	maxTurnRecapRunes = 300
)

// TurnSubmissionDiagnostic is bounded, English-only, and points to the exact
// independently retryable module and operation.
type TurnSubmissionDiagnostic struct {
	Module    string `json:"module"`
	Index     *int   `json:"index,omitempty"`
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Path      string `json:"path,omitempty"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

type TurnSubmissionModuleStatus struct {
	StateChanges   string `json:"state_changes"`
	Choices        string `json:"choices"`
	PlanUpdate     string `json:"plan_update"`
	CharacterFacts string `json:"character_facts"`
	Cultivation    string `json:"cultivation"`
	Skills         string `json:"skills"`
	Techniques     string `json:"techniques"`
	Items          string `json:"items"`
	Trade          string `json:"trade"`
	Rest           string `json:"rest"`
	Craft          string `json:"craft"`
	World          string `json:"world"`
}

// TurnSubmissionReceipt reports independent module acceptance. Ready becomes
// true only after all required modules have been accepted, possibly across calls.
type TurnSubmissionReceipt struct {
	CooperationOutcome   string                     `json:"cooperation_outcome,omitempty"`
	Ready                bool                       `json:"ready"`
	ModuleStatus         TurnSubmissionModuleStatus `json:"module_status"`
	Diagnostics          []TurnSubmissionDiagnostic `json:"diagnostics,omitempty"`
	RetryModules         []string                   `json:"retry_modules,omitempty"`
	MissingModules       []string                   `json:"missing_modules,omitempty"`
	DiagnosticsTruncated bool                       `json:"diagnostics_truncated,omitempty"`
	PlanUpdateDetail     *TurnPlanUpdateReceipt     `json:"plan_update_detail,omitempty"`
	// NarrativeRewriteRequired means the submitted prose describes an outcome the
	// rules will not produce. The world module can be refused outright, a craft
	// can be refused outright, or a craft's batch can really run and really be
	// lost to the method's failure rate. The caller must discard the narrative and
	// have the model write the real outcome; for a refused world action the
	// accepted state_changes were cleared with it, so the retry starts from the
	// committed world rather than from the refused turn's own claims.
	NarrativeRewriteRequired bool `json:"narrative_rewrite_required,omitempty"`
	// NarrativeRewriteReason is the refusal the model has to narrate. It is
	// English and bounded like a diagnostic message — except for a craft, whose
	// reason is written in the story's own words because it names materials and a
	// method the player reads in prose.
	NarrativeRewriteReason string `json:"narrative_rewrite_reason,omitempty"`
	// TradeRefusals lists trade lines the rules declined to settle this turn.
	// The module stays accepted — an unpaid debt is still a real outcome — but
	// the prose cannot claim the money moved, so a non-empty list also sets
	// NarrativeRewriteRequired.
	TradeRefusals []TradeRefusal `json:"trade_refusals,omitempty"`
}

// TurnPlanUpdateReceipt makes section-level partial success explicit to the
// model. Accepted section edits are retained in the run-local draft even when
// another section must be retried.
type TurnPlanUpdateReceipt struct {
	Mode             string   `json:"mode"`
	AcceptedSections []string `json:"accepted_sections,omitempty"`
	RejectedSections []string `json:"rejected_sections,omitempty"`
	RetrySections    []string `json:"retry_sections,omitempty"`
	RetainedDraft    bool     `json:"retained_draft,omitempty"`
}

// TurnSubmissionInput holds independently retryable modules decoded from one
// submit_interactive_turn call. Any module may be absent on a targeted retry.
type TurnSubmissionInput struct {
	StateUpdates   *[]interactivestate.Update
	Choices        *[]string
	PlanUpdate     *TurnPlanUpdateInput
	CharacterFacts *TurnCharacterFactsInput
	Cultivation    *TurnCultivationInput
	Skills         *TurnSkillLearningInput
	Techniques     *TurnTechniqueInput
	Items          *TurnItemsInput
	Trade          *TurnTradeInput
	Rest           *TurnRestInput
	Craft          *TurnCraftInput
	World          *TurnWorldActionInput
	// Recap is the turn's short recollection of what it published, in the
	// story's language. It is optional and carries no acceptance state: the turn
	// commits with or without it, and an absent recap is not a module awaiting
	// repair.
	Recap       *string
	Diagnostics []TurnSubmissionDiagnostic
}

// TurnSubmissionContext contains all story-scoped validation inputs. IDs and
// current state are backend-bound and never supplied by the model.
type TurnSubmissionContext struct {
	ActorState                  StoryDirectorActorStateSystem
	CurrentState                map[string]any
	ChoiceCount                 int
	RuleResolution              *RuleResolution
	RuleStateConsumptionMode    string
	RequireCompleteInitialState bool
	PlanningMode                string
	CurrentPlan                 *BranchPlan
	// KnownCharacters is this branch's registered cast. A character fact naming
	// anyone else is rejected at submission time rather than published and then
	// repaired, so the player never reads prose about a person the story then
	// denies exists.
	KnownCharacters []string
	// CharacterAliases maps an id the model minted this turn onto the actor that
	// already owns the same author source key. The state_changes module is
	// retargeted through it before compiling, so the numbers and the prose land
	// on one Actor instead of two.
	CharacterAliases map[string]string
	// Scene is the story's own scene. It resolves the audience of each exchange,
	// which is why the model does not choose its own witnesses.
	Scene character.Scene
	// ExistingAffinities and ExistingRelationships are what this branch has
	// already settled about its people. Submission checks a proposed change
	// against them so a fact already counted is not scored again, and so the
	// commit and the check agree about the value a change starts from.
	ExistingAffinities         []character.AffinityChange
	ExistingRelationships      []character.Relationship
	ExistingFactionMemberships []character.FactionMembership
	ExistingMatters            []character.Matter
	ExistingProfiles           []character.Profile
	CurrentClockSeconds        int64
	// CurrentPlayerInput is the accepted player text for source-bound actions
	// such as an explicit wait; model narration cannot authorize its own time.
	CurrentPlayerInput string
	CommunicationMode  string
	// Meta, BranchID and WorldView let submission run the same world planner the
	// commit will run. Without them a world action can be accepted here and
	// refused there, after the prose is locked — the narrator would then have
	// written 已进入洞府 while the world never moved. Callers that have no store
	// (shape-only tests) leave WorldView nil and the dry run is skipped.
	Meta      StoryMeta
	BranchID  string
	WorldView *TurnWorldView
	// CraftIdentity is the command identity the turn will commit under. A craft's
	// loss rate is one draw per command, so the dry run can only reproduce the
	// commit's answer if it reads the same identity. It is empty only in
	// shape-only callers and in a cycle that has not bound one yet; the craft dry
	// run is skipped then rather than run against a different seed, because a
	// verdict from a different die would be worse than no verdict.
	CraftIdentity DomainCommitIdentity
}

// PreparedTurnSubmission holds accepted modules while failed modules are
// retried. It is immutable after construction.
type PreparedTurnSubmission struct {
	result               TurnResult
	stateUpdatesAccepted bool
	choicesAccepted      bool
	planUpdateAccepted   bool
	planUpdateStarted    bool
	// characterFactsAccepted starts true because the module is optional: a turn
	// whose prose introduced nobody and settled nothing omits it. It only ever
	// turns false when a submitted module was rejected, which is a real repair
	// rather than an absent module.
	characterFactsAccepted bool
	// cultivationAccepted follows the same optional-module rule as character
	// facts: most turns do not contain an authorised training interval, and an
	// absent module is not a module awaiting repair.
	cultivationAccepted bool
	// skillsAccepted follows the same rule again. Learning a technique is rarer
	// than training, and a turn that learned nothing is not a turn awaiting a
	// repair.
	skillsAccepted bool
	// techniquesAccepted follows the same optional-module rule. Studying or
	// switching a method is not an every-turn event, and a turn that did neither
	// is not a turn awaiting a repair.
	techniquesAccepted bool
	// itemsAccepted follows the same optional-module rule. Acquiring or equipping
	// something is not an every-turn event, and a turn that did neither is not a
	// turn awaiting a repair.
	itemsAccepted bool
	// tradeAccepted follows the same optional-module rule. A deal is not an
	// every-turn event, and a turn that traded nothing is not a turn awaiting a
	// repair.
	tradeAccepted bool
	// restAccepted follows the same optional-module rule. Most turns are not a
	// rest interval, and a turn that spent no time resting is not a turn awaiting
	// a repair.
	restAccepted bool
	// craftAccepted follows the same optional-module rule. Most turns work no
	// batch, and a turn that crafted nothing is not a turn awaiting a repair.
	craftAccepted bool
	// worldAccepted starts false, unlike the other optional modules. The world
	// module is a decision every turn has to make: either a real action or an
	// explicit action=none. A turn whose prose takes a task and whose submission
	// simply omits the module used to be reported accepted, which told the model
	// its success had settled while the world never moved. An omission is now a
	// missing module the turn waits for.
	worldAccepted bool
	// worldDeclaredNone records that the turn's world decision was action=none.
	// Without it an accepted none and a not-yet-decided world look the same
	// (result.World is nil in both), and a later call could settle an action the
	// turn already decided not to take.
	worldDeclaredNone bool
	// recap is the turn's own one-paragraph recollection, kept beside the
	// accepted modules rather than inside them. It has no acceptance state, so a
	// call that omits it cannot un-accept a recap an earlier call already gave.
	recap *string
}

func (s *PreparedTurnSubmission) TurnResult() TurnResult {
	if s == nil {
		return TurnResult{}
	}
	return TurnResult{
		StateUpdates:   append([]interactivestate.Update(nil), s.result.StateUpdates...),
		Choices:        append([]string(nil), s.result.Choices...),
		PlanUpdate:     cloneStringPointer(s.result.PlanUpdate),
		CharacterFacts: cloneTurnCharacterFactsInput(s.result.CharacterFacts),
		Cultivation:    cloneTurnCultivationInput(s.result.Cultivation),
		Skills:         cloneTurnSkillLearningInput(s.result.Skills),
		Techniques:     cloneTurnTechniqueInput(s.result.Techniques),
		Items:          cloneTurnItemsInput(s.result.Items),
		Trade:          cloneTurnTradeInput(s.result.Trade),
		Rest:           cloneTurnRestInput(s.result.Rest),
		Craft:          cloneTurnCraftInput(s.result.Craft),
		World:          cloneTurnWorldActionInput(s.result.World),
	}
}

func (s *PreparedTurnSubmission) Ready() bool {
	return s != nil && s.stateUpdatesAccepted && s.choicesAccepted && s.planUpdateAccepted &&
		s.characterFactsAccepted && s.cultivationAccepted && s.skillsAccepted && s.techniquesAccepted && s.itemsAccepted && s.tradeAccepted && s.restAccepted && s.craftAccepted && s.worldAccepted
}

// Recap is the turn's own recollection, empty when the model gave none. It is
// read at commit time and is deliberately outside TurnResult: that struct's
// module fields never reach the public event, and the recap has to.
func (s *PreparedTurnSubmission) Recap() string {
	if s == nil || s.recap == nil {
		return ""
	}
	return *s.recap
}

// PrepareTurnSubmission accepts valid modules independently and retains any
// module accepted by an earlier call. state_changes remains atomic internally.
func PrepareTurnSubmission(validation TurnSubmissionContext, current *PreparedTurnSubmission, input TurnSubmissionInput) (*PreparedTurnSubmission, TurnSubmissionReceipt) {
	prepared := clonePreparedTurnSubmission(current)
	// A real story turn always carries the committed world view the dry run
	// reads, and it has to decide the world module: an action or action=none.
	// Shape-only callers without a store keep the old optional behaviour.
	if current == nil && validation.WorldView == nil {
		prepared.worldAccepted = true
	}
	planningEnabled := normalizeStoryPlanningMode(validation.PlanningMode) == StoryPlanningModeEnabled
	diagnostics := make([]TurnSubmissionDiagnostic, 0, len(input.Diagnostics))
	rejected := map[string]bool{}
	planDiagnostics := false
	planFatalDiagnostic := false
	// The turn's own compiled state writes, kept so the trade dry run can price a
	// payment against the purse a create in this same submission just filled. It
	// stays nil unless state_changes was accepted.
	var compiledState CompiledTurnStateUpdates
	for _, diagnostic := range input.Diagnostics {
		if diagnostic.Module == TurnSubmissionModulePlanUpdate && !planningEnabled {
			continue
		}
		if (diagnostic.Module == TurnSubmissionModuleStateChanges && prepared.stateUpdatesAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleChoices && prepared.choicesAccepted) ||
			(diagnostic.Module == TurnSubmissionModulePlanUpdate && prepared.planUpdateAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleCharacterFacts && prepared.characterFactsAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleCultivation && prepared.cultivationAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleSkills && prepared.skillsAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleTechniques && prepared.techniquesAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleItems && prepared.itemsAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleTrade && prepared.tradeAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleRest && prepared.restAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleCraft && prepared.craftAccepted) ||
			(diagnostic.Module == TurnSubmissionModuleWorld && prepared.worldAccepted) {
			continue
		}
		diagnostics = append(diagnostics, diagnostic)
		if diagnostic.Module == TurnSubmissionModulePlanUpdate {
			planDiagnostics = true
			prepared.planUpdateStarted = true
			if diagnostic.Index == nil {
				planFatalDiagnostic = true
			}
			continue
		}
		if diagnostic.Module == TurnSubmissionModuleStateChanges || diagnostic.Module == TurnSubmissionModuleChoices {
			rejected[diagnostic.Module] = true
		}
		if diagnostic.Module == TurnSubmissionModuleCharacterFacts {
			rejected[TurnSubmissionModuleCharacterFacts] = true
			// The module is optional, so it starts accepted. A submitted module
			// that was rejected has to un-accept it, or a turn whose character
			// facts did not decode would publish the prose and drop the people in
			// it.
			prepared.characterFactsAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleCultivation {
			rejected[TurnSubmissionModuleCultivation] = true
			prepared.cultivationAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleSkills {
			rejected[TurnSubmissionModuleSkills] = true
			prepared.skillsAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleTechniques {
			rejected[TurnSubmissionModuleTechniques] = true
			prepared.techniquesAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleItems {
			rejected[TurnSubmissionModuleItems] = true
			prepared.itemsAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleTrade {
			rejected[TurnSubmissionModuleTrade] = true
			prepared.tradeAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleRest {
			rejected[TurnSubmissionModuleRest] = true
			prepared.restAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleCraft {
			rejected[TurnSubmissionModuleCraft] = true
			prepared.craftAccepted = false
		}
		if diagnostic.Module == TurnSubmissionModuleWorld {
			rejected[TurnSubmissionModuleWorld] = true
			prepared.worldAccepted = false
		}
	}
	if input.StateUpdates != nil && !prepared.stateUpdatesAccepted && !rejected[TurnSubmissionModuleStateChanges] {
		updates := normalizeTurnSubmissionStateUpdateTargets(validation.ActorState, validation.CurrentState, *input.StateUpdates)
		updates = rebindTurnStateUpdatesToEstablishedActors(validation.ActorState, validation.CurrentState, updates, validation.CharacterAliases) // The pool a rest module is about to restore is the rules' answer for this
		// turn, so a write to that same pool is refused here rather than accepted
		// and then overwritten. The cultivation guard mirrors the commit path's:
		// when both modules are present the rest is the one that will be dropped,
		// and a pool write that would have been legal without it must not turn a
		// conflict the receipt already reports into a second, misleading failure.
		restActorID, restPoolField := "", ""
		if input.Rest != nil && !input.Rest.Empty() && (input.Cultivation == nil || input.Cultivation.Empty()) {
			restActorID, restPoolField = turnRestPoolOwner(input.Rest)
		}
		// The place a world action is about to write is the rules' answer too, so a
		// write to the same field is refused here rather than accepted and then
		// contradicted by the record.
		compileOptions := TurnStateUpdateCompileOptions{
			RuleResolution:           validation.RuleResolution,
			RuleStateConsumptionMode: validation.RuleStateConsumptionMode,
			RestActorID:              restActorID,
			RestPoolField:            restPoolField,
			WorldOwnsSceneLocation:   turnWorldOwnsSceneLocation(input.World),
			WorldSettlesMovement:     turnWorldSettlesMovement(input.World),
		}
		compiled, err := CompileTurnStateUpdates(validation.ActorState, validation.CurrentState, updates, compileOptions)
		if err != nil {
			validationErrors := flattenStateUpdateValidationErrors(err)
			collected := collectTurnStateUpdateValidationErrors(validation.ActorState, validation.CurrentState, updates, compileOptions)
			validationErrors = mergeStateUpdateValidationErrors(validationErrors, collected)
			if len(validationErrors) > 0 {
				err = &StateUpdateValidationErrors{Items: validationErrors}
			}
			diagnostics = append(diagnostics, diagnosticsForStateUpdateError(err)...)
			rejected[TurnSubmissionModuleStateChanges] = true
		} else {
			moduleDiagnostics := make([]TurnSubmissionDiagnostic, 0, 2)
			if diagnostic := storyContextSubmissionDiagnostic(validation.ActorState, validation.CurrentState, updates); diagnostic != nil {
				moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
			}
			if diagnostic := combatEngagementSubmissionDiagnostic(validation.ActorState, validation.CurrentState, compiled); diagnostic != nil {
				moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
			}
			facts := prepared.result.CharacterFacts
			if facts == nil {
				facts = input.CharacterFacts
			}
			if diagnostic := combatCompanionSubmissionDiagnostic(validation.ActorState, validation.CurrentState, compiled, facts, validation.KnownCharacters); diagnostic != nil {
				moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
			}
			if validation.RequireCompleteInitialState {
				if diagnostic := openingInitialStateSubmissionDiagnostic(validation.ActorState, validation.CurrentState, compiled); diagnostic != nil {
					moduleDiagnostics = append(moduleDiagnostics, *diagnostic)
				}
			}
			if len(moduleDiagnostics) > 0 {
				diagnostics = append(diagnostics, moduleDiagnostics...)
				rejected[TurnSubmissionModuleStateChanges] = true
			} else {
				prepared.result.StateUpdates = compiled.Updates
				prepared.stateUpdatesAccepted = true
				compiledState = compiled
			}
		}
	}

	if input.Choices != nil && !prepared.choicesAccepted && !rejected[TurnSubmissionModuleChoices] {
		choices, diagnostic := validateSubmittedChoices(*input.Choices, validation.ChoiceCount, validation.RuleResolution != nil && validation.RuleResolution.TerminalCandidate != nil)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleChoices] = true
		} else {
			prepared.result.Choices = choices
			prepared.choicesAccepted = true
		}
	}

	if !planningEnabled {
		// The story-level switch is authoritative. A stale or mistaken model
		// field must not make an otherwise valid turn retry, and must never be
		// retained for a story whose planning feature is disabled.
		prepared.result.PlanUpdate = nil
		prepared.planUpdateAccepted = true
		prepared.planUpdateStarted = false
	} else if input.PlanUpdate != nil && !prepared.planUpdateAccepted {
		prepared.planUpdateStarted = true
	}

	// The cultivation module is optional in the same way: most turns are not a
	// training interval. A rejected one un-accepts it so the turn waits for a
	// repair rather than committing prose that claims training the rules never
	// applied.
	selectionState := cloneStoryState(validation.CurrentState)
	for _, op := range compiledState.Ops {
		applyStateOp(selectionState, op)
	}
	for _, op := range compiledState.ActorOps {
		applyActorStateOp(selectionState, op)
	}
	if input.Cultivation != nil && !rejected[TurnSubmissionModuleCultivation] && prepared.result.Cultivation == nil {
		if diagnostic := validateTurnCultivationModule(input.Cultivation); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCultivation] = true
			prepared.cultivationAccepted = false
		} else if diagnostic := cultivationMethodSelectionDiagnostic(validation.ActorState.
			CultivationRules,
			selectionState, cultivationSubmissionSelectionInput(validation, selectionState, input.Cultivation)); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCultivation] = true
			prepared.cultivationAccepted = false
		} else if !input.Cultivation.Empty() {
			prepared.result.Cultivation = cloneTurnCultivationInput(input.Cultivation)
			prepared.cultivationAccepted = true
		}
	}

	// The skills module is optional in the same way. A rejected one un-accepts it
	// so the turn waits for a repair rather than publishing prose in which a
	// character demonstrates a technique the rules never granted them.
	if input.Skills != nil && !rejected[TurnSubmissionModuleSkills] && prepared.result.Skills == nil {
		if diagnostic := validateTurnSkillLearningModule(input.Skills); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleSkills] = true
			prepared.skillsAccepted = false
		} else if !input.Skills.Empty() {
			prepared.result.Skills = cloneTurnSkillLearningInput(input.Skills)
			prepared.skillsAccepted = true
		}
	}

	// The method module is optional in the same way. A rejected one un-accepts
	// it so the turn waits for a repair rather than publishing prose in which a
	// character trains or switches a method the rules never applied.
	if input.Techniques != nil && !rejected[TurnSubmissionModuleTechniques] && prepared.result.Techniques == nil {
		if diagnostic := validateTurnTechniqueModule(input.Techniques); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleTechniques] = true
			prepared.techniquesAccepted = false
		} else if !input.Techniques.Empty() {
			prepared.result.Techniques = cloneTurnTechniqueInput(input.Techniques)
			prepared.techniquesAccepted = true
		}
	}

	// The item module is optional in the same way. A rejected one un-accepts it so
	// the turn waits for a repair rather than publishing prose in which a
	// character obtained or equipped something the rules never applied.
	if input.Items != nil && !rejected[TurnSubmissionModuleItems] && prepared.result.Items == nil {
		if diagnostic := validateTurnItemsModule(input.Items); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleItems] = true
			prepared.itemsAccepted = false
		} else if !input.Items.Empty() {
			prepared.result.Items = cloneTurnItemsInput(input.Items)
			prepared.itemsAccepted = true
		}
	}

	// The trade module is optional in the same way. A rejected one un-accepts it
	// so the turn waits for a repair rather than publishing prose in which goods
	// and coin changed hands and the rules never settled them.
	if input.Trade != nil && !rejected[TurnSubmissionModuleTrade] && prepared.result.Trade == nil {
		if diagnostic := validateTurnTradeModule(input.Trade); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleTrade] = true
			prepared.tradeAccepted = false
		} else if !input.Trade.Empty() {
			prepared.result.Trade = cloneTurnTradeInput(input.Trade)
			prepared.tradeAccepted = true
		}
	}

	// The rest module is optional in the same way. A rejected one un-accepts it so
	// the turn waits for a repair rather than publishing prose in which the
	// character rested a day and the rules never restored anything.
	if input.Rest != nil && !rejected[TurnSubmissionModuleRest] && prepared.result.Rest == nil {
		if diagnostic := validateTurnRestModule(input.Rest); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleRest] = true
			prepared.restAccepted = false
		} else if !input.Rest.Empty() {
			prepared.result.Rest = cloneTurnRestInput(input.Rest)
			prepared.restAccepted = true
		}
	}
	// The craft module is optional in the same way. A rejected one un-accepts it
	// so the turn waits for a repair rather than publishing prose in which the
	// character worked a batch the rules never ran — which is exactly the fake
	// success ART-01 exists to prevent.
	if input.Craft != nil && !rejected[TurnSubmissionModuleCraft] && prepared.result.Craft == nil {
		if diagnostic := validateTurnCraftModule(input.Craft); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCraft] = true
			prepared.craftAccepted = false
		} else if !input.Craft.Empty() {
			prepared.result.Craft = cloneTurnCraftInput(input.Craft)
			prepared.craftAccepted = true
		}
	}
	// The world module is required as a decision, not as an action. A real
	// action settles a journey, a task, an event or a realm; action=none settles
	// nothing and is the declaration that lets an omitted module mean "still
	// missing". A rejected module un-accepts it so the turn waits for a repair
	// rather than publishing prose in which the character walked to the market,
	// took a task, joined an event or entered a realm and the world never moved.
	if input.World != nil && !rejected[TurnSubmissionModuleWorld] && prepared.result.World == nil && !prepared.worldDeclaredNone {
		diagnostic := validateTurnWorldActionModule(input.World)
		if diagnostic == nil && (strings.TrimSpace(input.World.Action) == TurnWorldActionWait || strings.TrimSpace(input.World.Action) == "adventure" || strings.TrimSpace(input.World.Action) == TurnWorldActionCooperation || strings.TrimSpace(input.World.Action) == TurnWorldActionFaction || localAffairRequiresSource(input.World)) &&
			strings.TrimSpace(validation.CurrentPlayerInput) != "" &&
			!strings.Contains(validation.CurrentPlayerInput, strings.TrimSpace(input.World.SourceText)) {
			diagnostic = newTurnSubmissionDiagnostic(TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticInvalidModule,
				"/world/source_text", "an exact quote from the accepted player input", input.World.SourceText,
				"A wait is allowed only when its source text occurs in the player's current request.")
		}
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleWorld] = true
			prepared.worldAccepted = false
		} else if input.World.DeclaresNone() {
			prepared.worldAccepted = true
			prepared.worldDeclaredNone = true
		} else if input.World.Settles() {
			prepared.result.World = cloneTurnWorldActionInput(input.World)
			prepared.worldAccepted = true
		}
	}
	// The shape being valid is not the same as the world allowing it. The commit
	// runs the same planner, and a refusal there would arrive after the prose is
	// locked, leaving a turn whose narrative says 已进入洞府 and whose world never
	// moved. Asking now — while the model can still rewrite — is the only place
	// the answer can still change the turn. This runs after the rest/cultivation
	// exclusions so a module that was going to be dropped anyway cannot un-accept
	// the state writes.
	narrativeRewriteReason := ""
	cooperationOutcome := ""
	worldNarrativeResult := ""
	worldClockSeconds := validation.CurrentClockSeconds
	if prepared.result.World != nil && validation.WorldView != nil {
		draftFacts := prepared.result.CharacterFacts
		if draftFacts == nil && !rejected[TurnSubmissionModuleCharacterFacts] {
			draftFacts = input.CharacterFacts
		}
		planned, planErr := dryRunTurnWorldPlan(validation, prepared.result.World, draftFacts)
		if factionModuleConflict(&prepared.result) {
			planErr = fmt.Errorf("共同制作不能与另一次物品、交易或学习结算混用")
		}
		if planned.cooperation != nil {
			cooperationOutcome = planned.cooperation.Outcome
		}
		plannedClock := planned.clockSeconds()
		if reason := turnWorldPlanError(prepared.result.World, planErr); reason != "" {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticWorldRefused,
				"/world", "a world action the rules accept", reason,
				"The world refuses this action, so the prose that narrates it succeeding cannot be published. Rewrite the narrative to show what really happened, then resubmit state_changes, choices and the world module for the real outcome.",
			))
			prepared.result.World = nil
			prepared.worldAccepted = false
			rejected[TurnSubmissionModuleWorld] = true
			// The state writes were compiled from the same prose. Keeping them
			// would let the refused success — a new location, an acquired item —
			// survive in a turn whose world action was dropped.
			prepared.result.StateUpdates = nil
			prepared.stateUpdatesAccepted = false
			rejected[TurnSubmissionModuleStateChanges] = true
			narrativeRewriteReason = reason
		} else {
			worldClockSeconds = max(worldClockSeconds, plannedClock)
			worldNarrativeResult = planned.narrativeResult
		}
	}
	// The shape being valid is not the same as the rules allowing it. Trade used
	// to be checked only for shape here and settled at commit, which meant a
	// refused payment reached the model after the prose was locked: a live run
	// wrote 已付、已结清 while the rules had moved nothing. Running the same
	// settlement the commit will run — against the state this turn's own accepted
	// writes leave — is what lets the refusal reach the model while it can still
	// rewrite the outcome.
	tradeRefusals := []TradeRefusal(nil)
	if prepared.result.Trade != nil && prepared.tradeAccepted {
		tradeBase := cloneStoryState(validation.CurrentState)
		for _, op := range compiledState.Ops {
			applyStateOp(tradeBase, op)
		}
		for _, op := range compiledState.ActorOps {
			applyActorStateOp(tradeBase, op)
		}
		tradeActorID := normalizeStatePanelActorID(prepared.result.Trade.ActorID)
		if tradeActorID == "" {
			tradeActorID = DefaultActorID
		}
		if _, dryRun, ok := xiuxianTradeOps(tradeBase, validation.ActorState, tradeActorID, *prepared.result.Trade, ""); ok {
			tradeRefusals = dryRun.Refused
		}
	}
	// The shape being valid is not the same as the rules allowing it, and a craft
	// is the fourth module where that gap used to swallow the answer. A live run
	// lost its batch to the method's 15% loss rate, was told only
	// "craft: accepted", wrote 剑成了, and put the blade in the pack itself on the
	// next turn with a source it invented. The materials were really spent and the
	// interval really passed, so the turn is not malformed — the prose is simply
	// false, and only a rewrite can fix it.
	//
	// The dry run reads the same base the commit will read (this turn's accepted
	// writes applied on top of the committed state) and the same command identity,
	// so it cannot reach a different verdict than the commit. Without a command
	// identity the draw would be seeded differently and the verdict would be about
	// a batch nobody is going to work, so it is skipped instead.
	//
	// It runs after the interval exclusions below so a craft that was going to be
	// dropped anyway cannot report a lost batch nobody will ever work.
	craftRewriteReason := ""

	for techniqueIntervalConflict(&prepared.result) {
		module := TurnSubmissionModuleTechniques
		if skillLearningHasStudy(prepared.result.Skills) {
			module = TurnSubmissionModuleSkills
			prepared.result.Skills = nil
			prepared.skillsAccepted = false
		} else {
			prepared.result.Techniques = nil
			prepared.techniquesAccepted = false
		}
		diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(module, nil, TurnSubmissionDiagnosticInvalidModule, "/"+module, "one timed activity per turn", "overlapping learning intervals", "技能或功法研习与其他计时行动分别结算，不能重复使用同一段时间。"))
		rejected[module] = true
	}

	// A turn cannot both train and rest. Each interval advances the formal clock
	// through its own side record, and two records in one turn would each advance
	// from the same starting instant — the second would overwrite the first's
	// clock, so one of the two intervals would be spent twice and the other not at
	// all. The two are also different intents: 闭关半年 and 休养一天 cannot both be
	// what this turn covers.
	if prepared.result.Rest != nil && prepared.result.Cultivation != nil {
		diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
			TurnSubmissionModuleRest, nil, TurnSubmissionDiagnosticInvalidModule,
			"/rest", "omit rest when the turn is a cultivation interval", "both cultivation and rest", "A turn covers either a training interval or a rest interval, never both.",
		))
		prepared.result.Rest = nil
		prepared.restAccepted = false
		rejected[TurnSubmissionModuleRest] = true
	}
	// The same holds for the world module. A stretch of road and a public-event
	// participation each publish a record that advances the formal clock, and
	// rest and training each publish their own; two of them in one turn would
	// both advance from the same starting instant, so one interval would be spent
	// twice and the other not at all. They are also different intents: 去坊市 and
	// 闭关半年 cannot both be what this turn covers.
	if prepared.result.World != nil {
		switch {
		case prepared.result.Rest != nil:
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticInvalidModule,
				"/world", "omit world when the turn is a rest interval", "both rest and world", "A turn covers either a world action or a rest interval, never both.",
			))
			prepared.result.World = nil
			prepared.worldAccepted = false
			rejected[TurnSubmissionModuleWorld] = true
		case prepared.result.Cultivation != nil:
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleWorld, nil, TurnSubmissionDiagnosticInvalidModule,
				"/world", "omit world when the turn is a cultivation interval", "both cultivation and world", "A turn covers either a world action or a training interval, never both.",
			))
			prepared.result.World = nil
			prepared.worldAccepted = false
			rejected[TurnSubmissionModuleWorld] = true
		}
	}

	// A craft is the fourth kind of interval that publishes its own clock record,
	// so it is exclusive with the other three for exactly the same reason: two
	// records in one turn would both advance from the same starting instant, and
	// one of the two intervals would be spent twice while the other was not spent
	// at all. It is also a different intent — 炼一炉丹 and 闭关半年 cannot both be
	// what this turn covers.
	if prepared.result.Craft != nil {
		conflict := ""
		switch {
		case prepared.result.Cultivation != nil:
			conflict = "cultivation"
		case prepared.result.Rest != nil:
			conflict = "rest"
		case prepared.result.World != nil:
			conflict = "world"
		}
		if conflict != "" {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticInvalidModule,
				"/craft", "omit craft when the turn is a "+conflict+" interval", "both craft and "+conflict,
				"A turn covers one interval of in-fiction time, so a craft never shares a turn with training, rest or a world action.",
			))
			prepared.result.Craft = nil
			prepared.craftAccepted = false
			rejected[TurnSubmissionModuleCraft] = true
		}
	}
	if prepared.result.Craft != nil && validation.CraftIdentity.CommandID != "" {
		craftBase := cloneStoryState(validation.CurrentState)
		for _, op := range compiledState.Ops {
			applyStateOp(craftBase, op)
		}
		for _, op := range compiledState.ActorOps {
			applyActorStateOp(craftBase, op)
		}
		// The commit works the furnace after the item and trade modules, because a
		// material bought or handed over earlier in the same turn is a material the
		// furnace can really consume. A dry run that read only the committed pack
		// would refuse 买料并开炉 — the very turn a player naturally writes — while
		// the commit would have accepted it, which is the opposite of the
		// agreement this check exists to have. Both modules are replayed here in
		// the commit's own order.
		craftActorID := normalizeStatePanelActorID(prepared.result.Craft.ActorID)
		if craftActorID == "" {
			craftActorID = DefaultActorID
		}
		// Match the commit's learning-before-inventory order. A method learned
		// earlier in this turn can be used by the craft that follows it.
		if prepared.result.Skills != nil && prepared.skillsAccepted {
			learningActorID := normalizeStatePanelActorID(prepared.result.Skills.ActorID)
			if learningActorID == "" {
				learningActorID = DefaultActorID
			}
			if learningOps, _, ok := xiuxianSkillLearningOps(craftBase, validation.ActorState, learningActorID, *prepared.result.Skills, ""); ok {
				for _, op := range learningOps {
					applyActorStateOp(craftBase, op)
				}
			}
		}
		if prepared.result.Items != nil && prepared.itemsAccepted {
			itemActorID := normalizeStatePanelActorID(prepared.result.Items.ActorID)
			if itemActorID == "" {
				itemActorID = DefaultActorID
			}
			if itemOps, _, ok := xiuxianItemOps(craftBase, validation.ActorState, itemActorID,
				withoutPaidAcquisitions(*prepared.result.Items, prepared.result.Trade), ""); ok {
				for _, op := range itemOps {
					applyActorStateOp(craftBase, op)
				}
			}
		}
		if prepared.result.Trade != nil && prepared.tradeAccepted {
			tradeActorID := normalizeStatePanelActorID(prepared.result.Trade.ActorID)
			if tradeActorID == "" {
				tradeActorID = DefaultActorID
			}
			if tradeOps, _, ok := xiuxianTradeOps(craftBase, validation.ActorState, tradeActorID, *prepared.result.Trade, ""); ok {
				for _, op := range tradeOps {
					applyActorStateOp(craftBase, op)
				}
			}
		}
		outcome, refusal := planTurnCraftCommit(validation.ActorState, craftActorID, craftBase, prepared.result.Craft, validation.CraftIdentity, "")
		switch {
		case refusal != "":
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticCraftRefused,
				"/craft", "a craft the rules accept", refusal,
				"The rules refuse this batch, so no product was made and no material was spent. Rewrite the narrative to show what really happened, then resubmit state_changes, choices and the craft module for the real outcome.",
			))
			prepared.result.Craft = nil
			prepared.craftAccepted = false
			rejected[TurnSubmissionModuleCraft] = true
			craftRewriteReason = refusal
		case outcome.batch.FailedBatches > 0:
			reason := craftLossRewriteReason(validation.ActorState.
				CultivationRules,
				outcome)
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModuleCraft, nil, TurnSubmissionDiagnosticCraftLost,
				"/craft", "a batch that yields the method's product", reason,
				"本次制作包含失败批次；材料与时间已经付出，成功批次的成品按规则结果保留。按实际产出与损失重写正文，再提交 choices 和 craft；不要用 items 重复领取成品。",
			))
			// The module stays accepted and the commit still spends the materials:
			// a lost batch is a real outcome, not a refused action. Only the prose
			// has to change.
			craftRewriteReason = reason
		}
		if outcome.progressMinutes > 0 {
			craftRewriteReason += fmt.Sprintf(" 本次实际制作%d分钟，完成%d批，另有一批进行到%d/%d分钟并已保存；该未完批次材料已投入，不能出售、重复扣除或提前领取成品。暂停本身不会使这批报废，下次同一方法可继续；不得沿用旧剧情里必须一口气完成、中断就废等与当前正式进度矛盾的说法。按实际进度叙述暂停，不提前宣称成功或失败。", outcome.batch.ElapsedMinutes, outcome.batch.Batches, outcome.progressMinutes, outcome.method.MinutesPerBatch)
		}
	}

	techniqueClockSeconds := validation.CurrentClockSeconds
	if prepared.result.Techniques != nil && prepared.techniquesAccepted {
		techniqueBase := cloneStoryState(validation.CurrentState)
		for _, op := range compiledState.Ops {
			applyStateOp(techniqueBase, op)
		}
		for _, op := range compiledState.ActorOps {
			applyActorStateOp(techniqueBase, op)
		}
		var projection *storyBranchProjection
		if view := validation.WorldView; view != nil {
			projection = &storyBranchProjection{State: validation.CurrentState, Clock: cloneStoryClock(view.Clock), Journeys: cloneJourneySnapshots(view.Journeys), Realms: cloneRealmInstances(view.Realms), Quests: cloneQuestInstances(view.Quests), PublicEvents: clonePublicEventInstances(view.PublicEvents), LocalAffairs: cloneLocalAffairs(view.LocalAffairs), Combats: view.Combats, Recoveries: cloneRecoveryActivities(view.Recoveries)}
		}
		outcome, err := planTurnTechniqueCommit(projection, validation.Meta, validation.ActorState, techniqueBase, prepared.result.Techniques, "", validation.CurrentPlayerInput)
		if err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleTechniques, nil, TurnSubmissionDiagnosticInvalidModule, "/techniques", "an available learning interval", err.Error(), "当前无法进行这次功法研习，请按实际情况调整。"))
			prepared.result.Techniques = nil
			prepared.techniquesAccepted = false
			rejected[TurnSubmissionModuleTechniques] = true
		} else {
			if outcome.clock != nil {
				techniqueClockSeconds = outcome.clock.After.Seconds
			}
			if len(outcome.result.Refused) > 0 {
				narrativeRewriteReason = "部分功法操作没有成立：" + outcome.result.Refused[0].Reason + "。请据实际掌握与运转状态叙述，不把未完成的学习写成已经学会。"
			}
		}
	}

	if prepared.result.Skills != nil && prepared.skillsAccepted {
		learningBase := cloneStoryState(validation.CurrentState)
		for _, op := range compiledState.Ops {
			applyStateOp(learningBase, op)
		}
		for _, op := range compiledState.ActorOps {
			applyActorStateOp(learningBase, op)
		}
		var projection *storyBranchProjection
		if view := validation.WorldView; view != nil {
			projection = &storyBranchProjection{State: validation.CurrentState, Clock: cloneStoryClock(view.Clock), Journeys: cloneJourneySnapshots(view.Journeys), Realms: cloneRealmInstances(view.Realms), Quests: cloneQuestInstances(view.Quests), PublicEvents: clonePublicEventInstances(view.PublicEvents), LocalAffairs: cloneLocalAffairs(view.LocalAffairs), Combats: view.Combats, Recoveries: cloneRecoveryActivities(view.Recoveries)}
		}
		outcome, err := planTurnSkillCommit(projection, validation.Meta, validation.ActorState, learningBase, prepared.result.Skills, "", validation.CurrentPlayerInput)
		if err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleSkills, nil, TurnSubmissionDiagnosticInvalidModule, "/skills", "an available learning interval", err.Error(), "当前无法进行这次技能研习，请按实际情况调整。"))
			prepared.result.Skills = nil
			prepared.skillsAccepted = false
			rejected[TurnSubmissionModuleSkills] = true
		} else {
			if outcome.clock != nil {
				techniqueClockSeconds = outcome.clock.After.Seconds
			}
			if len(outcome.result.Refused) > 0 {
				narrativeRewriteReason = "部分技能操作未成立：" + outcome.result.Refused[0].Reason + "。请按实际状态叙述。"
			}
			for _, study := range outcome.result.Studied {
				if !study.Mastered {
					narrativeRewriteReason = "技能仍在研习中，尚不能施展；只记录本次实际进度，不写成已掌握。"
					break
				}
			}
		}
	}

	// Character receipt and reply facts belong to the instant this validated
	// world action leaves behind. Read its planned clock only after interval
	// conflicts have been rejected; an uncommitted wait cannot deliver a letter.
	characterClockSeconds := techniqueClockSeconds
	if prepared.result.World != nil && prepared.worldAccepted {
		characterClockSeconds = worldClockSeconds
	}
	// A module that was already accepted is retained rather than re-validated, so
	// the gate is whether facts are actually stored: the flag starts true for an
	// absent module, which is why it cannot be the gate on its own.
	if input.CharacterFacts != nil && !rejected[TurnSubmissionModuleCharacterFacts] && prepared.result.CharacterFacts == nil {
		if diagnostic := validateTurnCharacterFactsWithVisuals(input.CharacterFacts, validation, prepared, characterClockSeconds); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCharacterFacts] = true
			prepared.characterFactsAccepted = false
			// A rejected character module is not merely absent. Publishing the
			// prose anyway would show the player a line from someone the story
			// then declines to remember, so the turn waits for a repair.
		} else if diagnostic := validateTurnCharacterQualifications(validation.ActorState.
			CultivationRules,
			input.CharacterFacts, validation.ExistingProfiles, validation.Meta.PackCatalog); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCharacterFacts] = true
			prepared.characterFactsAccepted = false
		} else if diagnostic := validateLetterWorldCharacterChannels(input.CharacterFacts, validation.CommunicationMode); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			rejected[TurnSubmissionModuleCharacterFacts] = true
			prepared.characterFactsAccepted = false
		} else if err := validateTurnFactionMemberships(membershipContextForSubmission(validation), input.CharacterFacts); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleCharacterFacts, nil, TurnSubmissionDiagnosticInvalidModule, "/character_facts/memberships", "an actual authorized membership decision", err.Error(), "势力身份尚未成立：请依据玩家原话、本人回应与实际接纳权限修正，不把申请写成已经加入。"))
			rejected[TurnSubmissionModuleCharacterFacts] = true
			prepared.characterFactsAccepted = false
		} else if !input.CharacterFacts.Empty() {
			prepared.result.CharacterFacts = cloneTurnCharacterFactsInput(input.CharacterFacts)
			prepared.characterFactsAccepted = true
		}
	}

	// Retained facts must also remain valid if a later repair removed their
	// timed world action. The commit uses the same final clock.
	if prepared.result.CharacterFacts != nil {
		if diagnostic := validateLetterMatterSettlements(prepared.result.CharacterFacts, validation.ExistingMatters, characterClockSeconds); diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			prepared.result.CharacterFacts = nil
			prepared.characterFactsAccepted = false
			rejected[TurnSubmissionModuleCharacterFacts] = true
		}
	}

	// Facts can be repaired after state writes were accepted. Recheck the final
	// pair, including an explicit refusal that contradicts a retained engagement.
	if prepared.result.CharacterFacts != nil {
		compiled, err := CompileTurnStateUpdates(validation.ActorState, validation.CurrentState, prepared.result.StateUpdates, TurnStateUpdateCompileOptions{
			RuleResolution: validation.RuleResolution, RuleStateConsumptionMode: validation.RuleStateConsumptionMode,
			WorldOwnsSceneLocation: turnWorldOwnsSceneLocation(prepared.result.World), WorldSettlesMovement: turnWorldSettlesMovement(prepared.result.World),
		})
		if err == nil {
			if diagnostic := combatCompanionSubmissionDiagnostic(validation.ActorState, validation.CurrentState, compiled, prepared.result.CharacterFacts, validation.KnownCharacters); diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
				prepared.result.StateUpdates = nil
				prepared.stateUpdatesAccepted = false
				rejected[TurnSubmissionModuleStateChanges] = true
			}
		}
	}

	diagnosticsBeforePlanApply := len(diagnostics)
	planDetail := applyTurnSubmissionPlanUpdate(validation, prepared, input.PlanUpdate, planFatalDiagnostic, &diagnostics)
	if planDiagnostics {
		prepared.planUpdateAccepted = false
		planDetail = addDecodedPlanSectionFailures(planDetail, input.Diagnostics)
	}
	if planDetail != nil && len(planDetail.RejectedSections) > 0 && prepared.result.PlanUpdate != nil {
		planDetail.RetainedDraft = true
	}
	if planDiagnostics || len(diagnostics) > diagnosticsBeforePlanApply || (planDetail != nil && len(planDetail.RejectedSections) > 0) {
		rejected[TurnSubmissionModulePlanUpdate] = true
	}
	if planningEnabled && !prepared.planUpdateAccepted && !prepared.planUpdateStarted && input.PlanUpdate == nil && validation.CurrentPlan != nil {
		prepared.planUpdateAccepted = true
	}

	// The recap has no acceptance state and no diagnostic. A call that gives one
	// replaces the draft's, and a call that omits it leaves an earlier one
	// standing — which is why it is assigned here instead of inside the module
	// loop, where an absent module would be reported missing.
	if input.Recap != nil {
		prepared.recap = cloneStringPointer(input.Recap)
	}

	receipt := buildTurnSubmissionReceipt(prepared, rejected, diagnostics)
	receipt.PlanUpdateDetail = planDetail
	receipt.TradeRefusals = tradeRefusals
	receipt.CooperationOutcome = cooperationOutcome
	if prepared.result.World != nil && prepared.result.World.Cooperation != nil && (prepared.result.World.Cooperation.Action == "work" || prepared.result.World.Cooperation.Action == "work_if_agreed") && cooperationOutcome != "" && cooperationOutcome != "worked" {
		narrativeRewriteReason = "这次协作的真实结果是" + cooperationOutcome + "：未共同工作，未扣物品或精力，也未推进600秒。保留实际回应与明确分享；不能叙述方案已经做成。可补齐资源、以后再议或独行。"
		if cooperationOutcome == "already_worked" {
			narrativeRewriteReason = "这份方案先前已经实际做成或用过。本次没有再次工作、扣物品或精力，也不再推进600秒。保留先前方案及其真实使用状态，不要否认已发生的共同工作。"
		}
	}
	// Retained world actions are immutable. Only the first successful preview
	// discards prose, so retries for unrelated modules cannot erase its rewrite.
	if narrativeRewriteReason == "" && worldNarrativeResult != "" && prepared.worldAccepted &&
		(current == nil || !current.worldAccepted || current.result.World == nil) {
		narrativeRewriteReason = worldNarrativeResult + " 依此重写正文，保留已接受的世界行动。"
	}
	// The world and the craft are both single outcomes that decide the whole
	// interval, so at most one of them can be the reason the prose is false.
	if narrativeRewriteReason != "" {
		receipt.NarrativeRewriteRequired = true
		receipt.NarrativeRewriteReason = narrativeRewriteReason
	}
	if craftRewriteReason != "" {
		receipt.NarrativeRewriteRequired = true
		if receipt.NarrativeRewriteReason == "" {
			receipt.NarrativeRewriteReason = craftRewriteReason
		}
	}
	// A refused payment is the one trade outcome that reaches the model as a
	// rewrite instruction rather than a diagnostic. The module is well-formed and
	// stays accepted — the story may still owe the coin — but the prose that says
	// it was paid is now false, so the narrative has to be written again.
	if len(tradeRefusals) > 0 {
		receipt.NarrativeRewriteRequired = true
		if receipt.NarrativeRewriteReason == "" {
			receipt.NarrativeRewriteReason = tradeRefusalRewriteReason(tradeRefusals)
		}
	}
	return prepared, receipt
}

// tradeRefusalRewriteReason states a refused payment as the outcome the prose
// has to narrate instead.
//
// The live run's failure was not that the rules refused — they are supposed to —
// but that the refusal was dropped and the narrative kept saying 已付、已结清.
// Naming what really happened is what lets the model write 未付、仍欠.
func tradeRefusalRewriteReason(refusals []TradeRefusal) string {
	parts := make([]string, 0, len(refusals))
	for _, refusal := range refusals {
		reason := strings.TrimSpace(refusal.Reason)
		if reason == "" {
			continue
		}
		parts = append(parts, reason)
	}
	if len(parts) == 0 {
		return ""
	}
	return "这笔钱没有付成：" + strings.Join(parts, "；") + "。正文与见闻与事务都要写真实结果（未付、仍欠或改为其他安排），不能写成已付或已结清。"
}

// dryRunTurnWorldAction asks the commit's own planner whether the world allows
// this action, and returns the refusal text when it does not.
//
// Both states are the committed one: submission has no turn writes to add yet,
// which is the case planTurnWorldAction's doc comment covers for callers in
// exactly this position. The planner is pure — it builds side records and
// operations but writes nothing — so running it here costs a few lookups and
// guarantees the commit cannot reach a different verdict than this call did.
func dryRunTurnWorldAction(validation TurnSubmissionContext, input *TurnWorldActionInput) (int64, string) {
	plan, err := dryRunTurnWorldPlan(validation, input, nil)
	return plan.clockSeconds(), turnWorldPlanError(input, err)
}

func turnWorldPlanError(input *TurnWorldActionInput, err error) string {
	if err == nil {
		return ""
	}
	if input.Action == "adventure" {
		return adventureRefusalForModel(err)
	}
	return err.Error()
}

func dryRunTurnWorldPlan(validation TurnSubmissionContext, input *TurnWorldActionInput, facts *TurnCharacterFactsInput) (turnWorldPlan, error) {
	view := validation.WorldView
	if view == nil {
		return turnWorldPlan{}, fmt.Errorf("world view is missing")
	}
	projection := &storyBranchProjection{PackCatalog: validation.Meta.PackCatalog, LatestTurnID: view.LatestTurnID, Characters: cloneCharacterState(view.Characters), State: validation.CurrentState, Clock: cloneStoryClock(view.Clock), Journeys: cloneJourneySnapshots(view.Journeys), Realms: cloneRealmInstances(view.Realms), Quests: cloneQuestInstances(view.Quests), PublicEvents: clonePublicEventInstances(view.PublicEvents), LocalAffairs: cloneLocalAffairs(view.LocalAffairs), Combats: view.Combats, Recoveries: cloneRecoveryActivities(view.Recoveries)}
	return planTurnWorldAction(projection, validation.CurrentState, validation.Meta, validation.ActorState, validation.BranchID, "", input, facts)
}

func addDecodedPlanSectionFailures(detail *TurnPlanUpdateReceipt, diagnostics []TurnSubmissionDiagnostic) *TurnPlanUpdateReceipt {
	for _, diagnostic := range diagnostics {
		if diagnostic.Module != TurnSubmissionModulePlanUpdate || diagnostic.Index == nil {
			continue
		}
		if detail == nil {
			detail = &TurnPlanUpdateReceipt{Mode: TurnPlanUpdateModeReplaceSections}
		}
		label := fmt.Sprintf("sections[%d]", *diagnostic.Index)
		if !stringSliceContains(detail.RejectedSections, label) {
			detail.RejectedSections = append(detail.RejectedSections, label)
			detail.RetrySections = append(detail.RetrySections, label)
		}
	}
	if detail != nil && len(detail.AcceptedSections) > 0 && len(detail.RejectedSections) > 0 {
		detail.RetainedDraft = true
	}
	return detail
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func applyTurnSubmissionPlanUpdate(validation TurnSubmissionContext, prepared *PreparedTurnSubmission, update *TurnPlanUpdateInput, blocked bool, diagnostics *[]TurnSubmissionDiagnostic) *TurnPlanUpdateReceipt {
	if normalizeStoryPlanningMode(validation.PlanningMode) != StoryPlanningModeEnabled || update == nil || prepared.planUpdateAccepted {
		return nil
	}
	detail := &TurnPlanUpdateReceipt{Mode: update.Mode}
	if blocked {
		return detail
	}

	switch update.Mode {
	case TurnPlanUpdateModeReplaceDocument:
		value := normalizeBranchPlanMarkdown(update.Markdown)
		if err := validateBranchPlanMarkdown(value); err != nil {
			*diagnostics = append(*diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/markdown", fmt.Sprintf("non-empty Markdown up to %d bytes", maxBranchPlanBytes),
				fmt.Sprintf("%d bytes", len([]byte(value))), err.Error(),
			))
			return detail
		}
		prepared.result.PlanUpdate = &value
		prepared.planUpdateAccepted = true
		return detail

	case TurnPlanUpdateModeReplaceSections:
		baseline := ""
		if prepared.result.PlanUpdate != nil {
			baseline = *prepared.result.PlanUpdate
		} else if validation.CurrentPlan != nil {
			baseline = validation.CurrentPlan.Markdown
		}
		draft, accepted, sectionErrors, err := applyBranchPlanSectionUpdates(baseline, update.Sections)
		detail.AcceptedSections = accepted
		if len(accepted) > 0 {
			prepared.result.PlanUpdate = &draft
		}
		if err != nil {
			*diagnostics = append(*diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidModule,
				"/plan_update/sections", "existing branch plan with unique ATX H2 sections", "uneditable plan structure", err.Error(),
			))
			return detail
		}
		for _, sectionError := range sectionErrors {
			index := sectionError.Index
			*diagnostics = append(*diagnostics, *newTurnSubmissionDiagnostic(
				TurnSubmissionModulePlanUpdate, &index, sectionError.Code,
				fmt.Sprintf("/plan_update/sections/%d", index), sectionError.Expected, sectionError.Actual, sectionError.Message,
			))
			label := sectionError.Heading
			if label == "" {
				label = fmt.Sprintf("sections[%d]", index)
			}
			if !stringSliceContains(detail.RejectedSections, label) {
				detail.RejectedSections = append(detail.RejectedSections, label)
			}
		}
		detail.RetrySections = append(detail.RetrySections, detail.RejectedSections...)
		detail.RetainedDraft = len(accepted) > 0 && len(detail.RejectedSections) > 0
		if len(sectionErrors) == 0 && len(update.Sections) > 0 {
			prepared.planUpdateAccepted = true
		}
		return detail

	default:
		*diagnostics = append(*diagnostics, *newTurnSubmissionDiagnostic(
			TurnSubmissionModulePlanUpdate, nil, TurnSubmissionDiagnosticInvalidPlanMode,
			"/plan_update/mode", "replace_document or replace_sections", update.Mode, "plan_update.mode is not supported.",
		))
		return detail
	}
}

func clonePreparedTurnSubmission(current *PreparedTurnSubmission) *PreparedTurnSubmission {
	if current == nil {
		return &PreparedTurnSubmission{
			result:                 TurnResult{StateUpdates: []interactivestate.Update{}, Choices: []string{}},
			characterFactsAccepted: true,
			cultivationAccepted:    true,
			skillsAccepted:         true,
			techniquesAccepted:     true,
			itemsAccepted:          true,
			tradeAccepted:          true,
			restAccepted:           true,
			craftAccepted:          true,
			worldAccepted:          false,
		}
	}
	return &PreparedTurnSubmission{
		result: TurnResult{
			StateUpdates:   append([]interactivestate.Update(nil), current.result.StateUpdates...),
			Choices:        append([]string(nil), current.result.Choices...),
			PlanUpdate:     cloneStringPointer(current.result.PlanUpdate),
			CharacterFacts: cloneTurnCharacterFactsInput(current.result.CharacterFacts),
			Cultivation:    cloneTurnCultivationInput(current.result.Cultivation),
			Skills:         cloneTurnSkillLearningInput(current.result.Skills),
			Techniques:     cloneTurnTechniqueInput(current.result.Techniques),
			Items:          cloneTurnItemsInput(current.result.Items),
			Trade:          cloneTurnTradeInput(current.result.Trade),
			Rest:           cloneTurnRestInput(current.result.Rest),
			Craft:          cloneTurnCraftInput(current.result.Craft),
			World:          cloneTurnWorldActionInput(current.result.World),
		},
		stateUpdatesAccepted:   current.stateUpdatesAccepted,
		choicesAccepted:        current.choicesAccepted,
		planUpdateAccepted:     current.planUpdateAccepted,
		planUpdateStarted:      current.planUpdateStarted,
		characterFactsAccepted: current.characterFactsAccepted,
		cultivationAccepted:    current.cultivationAccepted,
		skillsAccepted:         current.skillsAccepted,
		techniquesAccepted:     current.techniquesAccepted,
		itemsAccepted:          current.itemsAccepted,
		tradeAccepted:          current.tradeAccepted,
		restAccepted:           current.restAccepted,
		craftAccepted:          current.craftAccepted,
		worldAccepted:          current.worldAccepted,
		// Derived rather than stored: an accepted world decision with no action
		// is exactly a none declaration, including after a draft reload.
		worldDeclaredNone: current.worldAccepted && current.result.World == nil,
		recap:             cloneStringPointer(current.recap),
	}
}

func buildTurnSubmissionReceipt(prepared *PreparedTurnSubmission, rejected map[string]bool, diagnostics []TurnSubmissionDiagnostic) TurnSubmissionReceipt {
	receipt := TurnSubmissionReceipt{Ready: prepared.Ready()}
	receipt.ModuleStatus.StateChanges = turnSubmissionModuleStatus(prepared.stateUpdatesAccepted, rejected[TurnSubmissionModuleStateChanges])
	receipt.ModuleStatus.Choices = turnSubmissionModuleStatus(prepared.choicesAccepted, rejected[TurnSubmissionModuleChoices])
	receipt.ModuleStatus.PlanUpdate = turnSubmissionModuleStatus(prepared.planUpdateAccepted, rejected[TurnSubmissionModulePlanUpdate])
	receipt.ModuleStatus.CharacterFacts = turnSubmissionModuleStatus(prepared.characterFactsAccepted, rejected[TurnSubmissionModuleCharacterFacts])
	receipt.ModuleStatus.Cultivation = turnSubmissionModuleStatus(prepared.cultivationAccepted, rejected[TurnSubmissionModuleCultivation])
	receipt.ModuleStatus.Skills = turnSubmissionModuleStatus(prepared.skillsAccepted, rejected[TurnSubmissionModuleSkills])
	receipt.ModuleStatus.Techniques = turnSubmissionModuleStatus(prepared.techniquesAccepted, rejected[TurnSubmissionModuleTechniques])
	receipt.ModuleStatus.Items = turnSubmissionModuleStatus(prepared.itemsAccepted, rejected[TurnSubmissionModuleItems])
	receipt.ModuleStatus.Trade = turnSubmissionModuleStatus(prepared.tradeAccepted, rejected[TurnSubmissionModuleTrade])
	receipt.ModuleStatus.Rest = turnSubmissionModuleStatus(prepared.restAccepted, rejected[TurnSubmissionModuleRest])
	receipt.ModuleStatus.Craft = turnSubmissionModuleStatus(prepared.craftAccepted, rejected[TurnSubmissionModuleCraft])
	receipt.ModuleStatus.World = turnSubmissionModuleStatus(prepared.worldAccepted, rejected[TurnSubmissionModuleWorld])
	for _, module := range turnSubmissionModules {
		status := receipt.ModuleStatus.StateChanges
		switch module {
		case TurnSubmissionModuleChoices:
			status = receipt.ModuleStatus.Choices
		case TurnSubmissionModulePlanUpdate:
			status = receipt.ModuleStatus.PlanUpdate
		case TurnSubmissionModuleCharacterFacts:
			status = receipt.ModuleStatus.CharacterFacts
		case TurnSubmissionModuleCultivation:
			status = receipt.ModuleStatus.Cultivation
		case TurnSubmissionModuleSkills:
			status = receipt.ModuleStatus.Skills
		case TurnSubmissionModuleTechniques:
			status = receipt.ModuleStatus.Techniques
		case TurnSubmissionModuleItems:
			status = receipt.ModuleStatus.Items
		case TurnSubmissionModuleTrade:
			status = receipt.ModuleStatus.Trade
		case TurnSubmissionModuleRest:
			status = receipt.ModuleStatus.Rest
		case TurnSubmissionModuleCraft:
			status = receipt.ModuleStatus.Craft
		case TurnSubmissionModuleWorld:
			status = receipt.ModuleStatus.World
		}
		if status != TurnSubmissionModuleAccepted {
			receipt.RetryModules = append(receipt.RetryModules, module)
		}
		if status == TurnSubmissionModuleMissing {
			receipt.MissingModules = append(receipt.MissingModules, module)
		}
	}
	if len(diagnostics) > maxTurnSubmissionDiagnostics {
		receipt.DiagnosticsTruncated = true
		diagnostics = diagnostics[:maxTurnSubmissionDiagnostics]
	}
	receipt.Diagnostics = diagnostics
	return receipt
}

// turnSubmissionModules is the ordered module list every receipt iterates. It is
// one list rather than four parallel literals so a new module cannot be added to
// the status struct and forgotten in the retry list.
var turnSubmissionModules = []string{
	TurnSubmissionModuleStateChanges,
	TurnSubmissionModuleChoices,
	TurnSubmissionModulePlanUpdate,
	TurnSubmissionModuleCharacterFacts,
	TurnSubmissionModuleCultivation,
	TurnSubmissionModuleSkills,
	TurnSubmissionModuleTechniques,
	TurnSubmissionModuleItems,
	TurnSubmissionModuleTrade,
	TurnSubmissionModuleRest,
	TurnSubmissionModuleCraft,
	TurnSubmissionModuleWorld,
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func turnSubmissionModuleStatus(accepted, rejected bool) string {
	if accepted {
		return TurnSubmissionModuleAccepted
	}
	if rejected {
		return TurnSubmissionModuleRejected
	}
	return TurnSubmissionModuleMissing
}

func validateSubmittedChoices(values []string, configured int, terminal bool) ([]string, *TurnSubmissionDiagnostic) {
	configured = normalizeStoryChoiceCount(configured)
	if err := validateStoryChoiceCount(configured); err != nil {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "invalid_choice_count_config", "", fmt.Sprintf("%d-%d", MinStoryChoiceCount, MaxStoryChoiceCount), fmt.Sprint(configured), "The story choice count configuration is invalid.")
	}
	if len(values) == 0 {
		if terminal {
			return []string{}, nil
		}
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), "0 choices", fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for index, value := range values {
		choice := strings.TrimSpace(value)
		if choice == "" {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticEmptyChoice, fmt.Sprintf("/choices/%d", index), "non-empty string", "empty string", "Choices must not be empty.")
		}
		if len([]byte(choice)) > maxTurnSubmissionChoiceBytes {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), "choice_too_large", fmt.Sprintf("/choices/%d", index), fmt.Sprintf("at most %d bytes", maxTurnSubmissionChoiceBytes), fmt.Sprintf("%d bytes", len([]byte(choice))), "The choice text is too long.")
		}
		key := normalizedChoiceKey(choice)
		if seen[key] {
			return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticDuplicateChoice, fmt.Sprintf("/choices/%d", index), "distinct normalized choice", choice, "Choices must remain distinct after text normalization.")
		}
		seen[key] = true
		normalized = append(normalized, choice)
	}
	if terminal {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", "empty array for the declared terminal turn", fmt.Sprintf("%d choices", len(normalized)), "RuleResolution declared a terminal turn, so choices must be an empty array.")
	}
	if len(normalized) != configured {
		return nil, newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticChoiceCountMismatch, "/choices", fmt.Sprintf("exactly %d choices", configured), fmt.Sprintf("%d choices", len(normalized)), fmt.Sprintf("Non-terminal turns must submit exactly %d distinct choices.", configured))
	}
	return normalized, nil
}

func decodeChoicesModule(raw json.RawMessage) ([]string, []TurnSubmissionDiagnostic) {
	var items []json.RawMessage
	if err := decodeStrictJSON(raw, &items, false); err != nil || items == nil {
		if err == nil {
			err = errors.New("choices cannot be null")
		}
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, TurnSubmissionDiagnosticInvalidModule, "/choices", "array of strings", jsonValueKind(raw), fmt.Sprintf("choices must be an array of strings: %v", err))}
	}
	if len(items) > MaxStoryChoiceCount {
		return nil, []TurnSubmissionDiagnostic{*newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, nil, "too_many_choices", "/choices", fmt.Sprintf("at most %d choices", MaxStoryChoiceCount), fmt.Sprintf("%d choices", len(items)), fmt.Sprintf("choices cannot exceed %d items.", MaxStoryChoiceCount))}
	}
	choices := make([]string, 0, len(items))
	diagnostics := make([]TurnSubmissionDiagnostic, 0)
	for index, item := range items {
		var choice string
		if err := decodeStrictJSON(item, &choice, false); err != nil {
			diagnostics = append(diagnostics, *newTurnSubmissionDiagnostic(TurnSubmissionModuleChoices, intPointer(index), TurnSubmissionDiagnosticInvalidModule, fmt.Sprintf("/choices/%d", index), "string", jsonValueKind(item), "Each choice must be a string."))
			continue
		}
		choices = append(choices, choice)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return choices, nil
}

func decodeStrictJSON(data []byte, target any, useNumber bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if useNumber {
		decoder.UseNumber()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func newTurnSubmissionDiagnostic(module string, index *int, code, path, expected, actual, message string) *TurnSubmissionDiagnostic {
	return &TurnSubmissionDiagnostic{
		Module:    module,
		Index:     index,
		Code:      code,
		Severity:  turnSubmissionSeverityError,
		Path:      path,
		Expected:  trimBytes(expected, maxTurnSubmissionDiagnosticMessage),
		Actual:    trimBytes(actual, maxTurnSubmissionDiagnosticMessage),
		Retryable: true,
		Message:   trimBytes(message, maxTurnSubmissionDiagnosticMessage),
	}
}

func intPointer(value int) *int {
	return &value
}

func jsonValueKind(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "empty"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 'n':
		return "null"
	case 't', 'f':
		return "bool"
	default:
		return "number or invalid JSON"
	}
}

func turnSubmissionAllowedFields(template ActorStateTemplate) []string {
	fields := make([]string, 0, len(template.Fields))
	for _, field := range template.Fields {
		fields = append(fields, actorStateFieldID(field))
		if len(fields) >= maxTurnSubmissionAllowedFields {
			break
		}
	}
	return fields
}

// Eligibility sees the same resumed method as commit. The submitted module stays
// unchanged; only the immutable preview is narrowed by the shared planner.
func cultivationSubmissionSelectionInput(validation TurnSubmissionContext, state map[string]any, input *TurnCultivationInput) *TurnCultivationInput {
	if input == nil || validation.WorldView == nil {
		return input
	}
	view := validation.WorldView
	projection := &storyBranchProjection{State: state, Clock: cloneStoryClock(view.Clock), Cultivations: cloneCultivationActivities(view.Cultivations), Journeys: cloneJourneySnapshots(view.Journeys), Realms: cloneRealmInstances(view.Realms), Quests: cloneQuestInstances(view.Quests), PublicEvents: clonePublicEventInstances(view.PublicEvents)}
	actorID := normalizeStatePanelActorID(input.ActorID)
	if actorID == "" {
		actorID = DefaultActorID
	}
	plan := planTurnCultivation(projection, actorID, input, lifespanBoundary(validation.Meta, state, actorID))
	return &plan.input
}
