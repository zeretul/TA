package interactive

import (
	"encoding/json"

	agent "ta/agent"
	interactivestate "ta/internal/interactive/state"

	agentcontext "ta/internal/agents/context"
	"ta/internal/agents/conversationconfig"
	"ta/internal/character"
	"ta/internal/cultivation"
	"ta/internal/exploration"
	"ta/internal/npcgrowth"
	"ta/internal/publiquevent"
	"ta/internal/quest"
)

type CreateStoryRequest struct {
	// InitialFactionMembership comes only from server-resolved, player-confirmed creation.
	InitialFactionMembership *character.FactionMembership `json:"-"`
	Title                    string                       `json:"title"`
	CustomAgentID            *string                      `json:"custom_agent_id,omitempty"`
	ProfileID                string                       `json:"profile_id,omitempty"`
	ThinkingLevel            string                       `json:"thinking_level,omitempty"`
	Origin                   string                       `json:"origin"`
	Protagonist              StoryProtagonist             `json:"protagonist,omitempty"`
	StoryTellerID            string                       `json:"story_teller_id"`
	PlanningTemplateID       string                       `json:"planning_template_id,omitempty"`
	PlanningMode             string                       `json:"planning_mode,omitempty"`
	ModuleRefs               *StoryDirectorModuleRefs     `json:"module_refs,omitempty"`
	// FrozenModules is the resolved module graph the caller already resolved for
	// this story. When set, it is stored on the story and every later turn reads
	// it instead of re-resolving from the live preset catalog.
	FrozenModules    *StoryDirectorResolvedSnapshot `json:"-"`
	ReplyTargetChars int                            `json:"reply_target_chars"`
	ChoiceCount      int                            `json:"choice_count"`
	Opening          StoryOpeningConfig             `json:"opening,omitempty"`
	ImageSettings    StoryImageSettings             `json:"image_settings,omitempty"`
	CheckSettings    StoryCheckSettings             `json:"check_settings,omitempty"`
	SpeechSettings   StorySpeechSettings            `json:"speech_settings,omitempty"`
	// IntimacySettings binds the intimacy window's own model purpose. It is a
	// separate binding on purpose: choosing a model here must not change the
	// main story's route, and a story that has never chosen one keeps history
	// readable without inheriting anything.
	IntimacySettings StoryIntimacySettings `json:"intimacy_settings,omitempty"`
	// StylePurposes selects the style used for the main narrative, for dialogue
	// and chat, and for combat description. The latter two may be empty, which
	// means they follow the main narrative.
	StylePurposes StoryStylePurposes `json:"style_purposes,omitempty"`
	// NarrativeRequirements is this story's own standing creative direction:
	// the preferences and constraints the narrator must honour in this game.
	// It is story-scoped on purpose. The workspace CREATOR.md is a shared
	// default, so two stories in one workspace would otherwise read the same
	// text and leak rules between them; a story that carries its own copy reads
	// that copy and nothing else. A pack may seed the value at creation.
	//
	// It governs expression and preference only. It never overrides a settled
	// rule result, a stored number, or anything a character actually knows.
	NarrativeRequirements string                  `json:"narrative_requirements,omitempty"`
	InitialTraitRolls     []InitialActorTraitRoll `json:"initial_trait_rolls,omitempty"`
	PackBinding           *StoryPackBinding       `json:"pack_binding,omitempty"`
	// PackCatalog is the discoverable-people index of the pack revision this
	// story starts on, resolved server-side from the pack the binding names. It is
	// resolved input like ProtagonistInitialState, not client input: a client
	// choosing a world may not hand the story a roster.
	//
	// It is written into the journal once, at creation, so the story keeps
	// locating its own people after the pack file moves on. The lore bodies those
	// people point at are already revision-scoped in the world book; this is the
	// index that says who exists and where they start.
	PackCatalog *PackCatalogSnapshot `json:"-"`
	// Calendar is the year/month/day convention this story freezes at creation.
	// It is resolved server-side like PackCatalog: a client may not hand a story
	// a year length, because that length is what a lifespan is measured with.
	//
	// A nil value means the story never bound one and must not be given one
	// later. Old stories are not migrated: their committed prose was written
	// under no stated year length, and supplying one afterwards would re-read
	// numbers that were never recorded against it.
	Calendar *StoryCalendar `json:"-"`
	// ProtagonistAge is the age the pack's authored body stated for this story's
	// protagonist, read once at creation by the same parser the catalog uses.
	// Zero means the body stated none, which stays unknown rather than guessed.
	ProtagonistAge int `json:"-"`
	// ProtagonistGender is the gender the protagonist's authored body stated,
	// read once at creation by the same explicit-declaration parser the world
	// catalog uses. Empty means the body stated none, and that stays unknown
	// rather than being inferred from a name or a portrait.
	ProtagonistGender string `json:"-"`
	// PackTemplate names a ready-made protagonist a world pack ships. The
	// template's initial state is resolved from the pack file server-side and
	// never travels in this request: a client may choose which template to play,
	// but it cannot hand the story the numbers a template is worth.
	PackTemplate *StoryProtagonistTemplateRef `json:"pack_template,omitempty"`
	// IdentityStateOverlay is the prose half of a confirmed character-creation
	// result, resolved server-side from the pack and written over the template's
	// own persona fields. It is json:"-" because it is resolved input: a client
	// sends its choices to the creation endpoint, never the derived fields.
	//
	// Only narrative fields may appear here. Realm, currency, carried items and
	// techniques are the template's, so a created character starts from the same
	// initial state as the ready-made one and no prose can move a number.
	IdentityStateOverlay map[string]any `json:"-"`
	// IdentityConfirmed records that this story's protagonist came out of a
	// character-creation confirmation, not out of a pack-declared name or a
	// leftover draft. The stage reads it to decide whether the player still owes
	// a choice of protagonist; a name being non-empty is not that fact, because
	// a pack's own protagonist() also fills one in.
	//
	// json:"-" because it is resolved server-side from the creation path: a
	// client that could set it could make an unconformed story skip the only
	// screen that asks who the player is.
	IdentityConfirmed    bool                              `json:"-"`
	StateSchemaPolicy    *StoryStateSchemaPolicy           `json:"state_schema_policy,omitempty"`
	ActorState           *StoryDirectorActorStateSystem    `json:"-"`
	TRPGSystem           *StoryDirectorTRPGSystem          `json:"-"`
	ActorStateAdaptation *ActorStateSchemaAdaptationRecord `json:"-"`
	InitialStateOps      []interactivestate.Op             `json:"-"`
	// ProtagonistInitialState is the chosen template's initial state, filled in
	// by the server after it reads the pack. It is json:"-" for the same reason
	// ActorState is: it is resolved input, not client input.
	ProtagonistInitialState   map[string]any                   `json:"-"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"-"`
	RuntimeConfig             *conversationconfig.Config       `json:"-"`
}

// StoryProtagonistTemplateRef points at one ready-made protagonist in a world
// pack. It is a reference, not the content: the pack file stays the source of
// the profile, the opening and the initial state.
type StoryProtagonistTemplateRef struct {
	PackID     string `json:"pack_id"`
	TemplateID string `json:"template_id"`
}

type AppendTurnRequest struct {
	BranchID             string                `json:"branch_id"`
	User                 string                `json:"user"`
	Narrative            string                `json:"narrative"`
	Thinking             string                `json:"thinking,omitempty"`
	DisplayEvents        []DisplayEvent        `json:"display_events,omitempty"`
	ModelContextMessages []ModelContextMessage `json:"model_context_messages,omitempty"`
}

type AppendTurnWithStateRequest struct {
	Checkpoint       agent.CanonicalCheckpoint `json:"-"`
	BranchID         string                    `json:"branch_id"`
	ExpectedParentID *string                   `json:"expected_parent_id,omitempty"`
	ReplaceTurnID    string                    `json:"replace_turn_id,omitempty"`
	User             string                    `json:"user"`
	Narrative        string                    `json:"narrative"`
	Thinking         string                    `json:"thinking,omitempty"`
	// Recap is the turn's own short recollection of what it published, written
	// in the story's language. It is display material for the history list and is
	// never a source of truth: an absent one falls back to an excerpt of
	// Narrative, and nothing in settlement or recovery reads it.
	Recap            string `json:"recap,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	AgentKind        string `json:"agent_kind,omitempty"`
	AgentCommandID   string `json:"agent_command_id,omitempty"`
	AgentOperationID string `json:"agent_operation_id,omitempty"`
	AgentCycle       int    `json:"agent_cycle,omitempty"`
	// ProviderContinuation is opaque model-visible state from the exact final
	// assistant output. Story persistence retains it without exposing it in UI
	// projections or interpreting provider-owned payloads.
	ProviderContinuation map[string]any            `json:"-"`
	DisplayEvents        []DisplayEvent            `json:"display_events,omitempty"`
	ModelContextMessages []ModelContextMessage     `json:"model_context_messages,omitempty"`
	Ops                  []interactivestate.Op     `json:"ops,omitempty"`
	ActorOps             []ActorStateOp            `json:"actor_ops,omitempty"`
	RuleResolution       *RuleResolution           `json:"rule_resolution,omitempty"`
	TurnResult           *TurnResult               `json:"turn_result,omitempty"`
	TerminalOutcome      *TerminalOutcome          `json:"terminal_outcome,omitempty"`
	StateSchemaProposal  *ActorStateSchemaProposal `json:"-"`
	// PlayerLetter is trusted UI intent bound to this exact formal turn. The
	// story-store transaction turns it into a Character Matter; it is never
	// accepted from the model's tool schema.
	PlayerLetter *PlayerLetterIntent `json:"player_letter,omitempty"`
}

type RuleResolutionRerollRequest struct {
	BranchID string `json:"branch_id,omitempty"`
	TurnID   string `json:"turn_id,omitempty"`
}

type RewindTurnRequest struct {
	BranchID string `json:"branch_id"`
	TurnID   string `json:"turn_id"`
}

type SwitchTurnVersionRequest struct {
	BranchID      string `json:"branch_id"`
	TurnID        string `json:"turn_id"`
	VersionTurnID string `json:"version_turn_id"`
}

// UpdateTurnNarrativeRequest updates only the creator-visible narrative of an
// existing turn. ExpectedNarrative provides compare-and-swap protection for
// editors opened from an older snapshot.
type UpdateTurnNarrativeRequest struct {
	BranchID          string  `json:"branch_id"`
	TurnID            string  `json:"-"`
	Narrative         string  `json:"narrative"`
	ExpectedNarrative *string `json:"expected_narrative,omitempty"`
}

// UpdateTurnNarrativeResult reports the durable edited turn. Public Agent
// reloads the canonical Story history before the next turn and invalidates
// only projections whose source prefix changed.
type UpdateTurnNarrativeResult struct {
	Turn TurnEvent `json:"turn"`
}

type InteractiveImageGenerateRequest struct {
	CommandID string `json:"command_id"`
	BranchID  string `json:"branch_id,omitempty"`
	TurnID    string `json:"turn_id"`
	Source    string `json:"source,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

type AppendStateDeltaRequest struct {
	ParentID string                `json:"parent_id"`
	BranchID string                `json:"branch_id"`
	Ops      []interactivestate.Op `json:"ops"`
	ActorOps []ActorStateOp        `json:"actor_ops,omitempty"`
}

type MarkStateFailedRequest struct {
	ParentID string `json:"parent_id"`
	BranchID string `json:"branch_id"`
	Error    string `json:"error"`
}

type UpdateStoryRequest struct {
	Title              string                   `json:"title"`
	Origin             *string                  `json:"origin,omitempty"`
	Protagonist        *StoryProtagonist        `json:"protagonist,omitempty"`
	StoryTellerID      string                   `json:"story_teller_id"`
	PlanningTemplateID string                   `json:"planning_template_id,omitempty"`
	PlanningMode       *string                  `json:"planning_mode,omitempty"`
	ModuleRefs         *StoryDirectorModuleRefs `json:"module_refs,omitempty"`
	ReplyTargetChars   *int                     `json:"reply_target_chars,omitempty"`
	ChoiceCount        *int                     `json:"choice_count,omitempty"`
	Opening            *StoryOpeningConfig      `json:"opening,omitempty"`
	ImageSettings      *StoryImageSettings      `json:"image_settings,omitempty"`
	CheckSettings      *StoryCheckSettings      `json:"check_settings,omitempty"`
	SpeechSettings     *StorySpeechSettings     `json:"speech_settings,omitempty"`
	// IntimacySettings rebinds the intimacy window's model purpose. A pointer
	// distinguishes "leave it alone" (nil) from "clear the binding" (pointer to
	// an empty profile ID), so a player can drop a dedicated profile without
	// the request being read as "no change".
	IntimacySettings *StoryIntimacySettings    `json:"intimacy_settings,omitempty"`
	StylePurposes    *StoryStylePurposesUpdate `json:"style_purposes,omitempty"`
	// NarrativeRequirements replaces this story's own creative direction. A
	// pointer distinguishes "leave it alone" (nil) from "clear it" (pointer to
	// the empty string), which is what lets a player drop an override and fall
	// back to the workspace default without sending an ambiguous empty field.
	NarrativeRequirements     *string                          `json:"narrative_requirements,omitempty"`
	StateSchemaPolicy         *StoryStateSchemaPolicy          `json:"state_schema_policy,omitempty"`
	ActorState                *StoryDirectorActorStateSystem   `json:"-"`
	TRPGSystem                *StoryDirectorTRPGSystem         `json:"-"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"-"`
	// LocalEntitySettings replaces the whole per-entity settings map. The map is
	// small and the page always sends what it is showing, so a whole-map replace
	// is one write instead of a merge that could resurrect a record the player
	// just cleared. A nil map leaves the stored settings untouched; an empty map
	// clears them, which is how the page drops every override at once.
	LocalEntitySettings map[string]LocalEntitySettings `json:"local_entity_settings,omitempty"`
}

// StoryStylePurposesUpdate is a partial style-purpose change. Each field is
// optional, and the three are independently meaningful:
//
//   - nil means "leave this purpose as it is", so changing dialogue does not
//     disturb the main narrative or combat.
//   - a pointer to "" means "follow the main narrative again", which is a real
//     choice a player can make and must be distinguishable from saying nothing.
//   - a pointer to an id selects that style for the purpose.
type StoryStylePurposesUpdate struct {
	Main     *string `json:"main,omitempty"`
	Dialogue *string `json:"dialogue,omitempty"`
	Combat   *string `json:"combat,omitempty"`
}

type CreateBranchRequest struct {
	ParentEventID string                     `json:"parent_event_id"`
	Title         string                     `json:"title"`
	CustomAgentID *string                    `json:"custom_agent_id,omitempty"`
	RuntimeConfig *conversationconfig.Config `json:"-"`
}

type Index struct {
	Version        int            `json:"version"`
	CurrentStoryID string         `json:"current_story_id"`
	Stories        []StorySummary `json:"stories"`
}

type StorySummary struct {
	ID                    string                   `json:"id"`
	Title                 string                   `json:"title"`
	TitleSource           string                   `json:"title_source"`
	Origin                string                   `json:"origin"`
	Protagonist           StoryProtagonist         `json:"protagonist"`
	StoryTellerID         string                   `json:"story_teller_id"`
	PlanningTemplateID    string                   `json:"planning_template_id"`
	LegacyStoryDirectorID string                   `json:"story_director_id,omitempty"`
	PlanningMode          string                   `json:"planning_mode"`
	ModuleRefs            *StoryDirectorModuleRefs `json:"module_refs,omitempty"`
	ReplyTargetChars      int                      `json:"reply_target_chars"`
	ChoiceCount           int                      `json:"choice_count"`
	Opening               StoryOpeningConfig       `json:"opening"`
	ImageSettings         StoryImageSettings       `json:"image_settings"`
	CheckSettings         StoryCheckSettings       `json:"check_settings"`
	SpeechSettings        StorySpeechSettings      `json:"speech_settings,omitempty"`
	IntimacySettings      StoryIntimacySettings    `json:"intimacy_settings,omitempty"`
	StylePurposes         StoryStylePurposes       `json:"style_purposes,omitempty"`
	// NarrativeRequirements is this story's own creative direction. Empty means
	// the story reads the workspace default (CREATOR.md) instead, which is the
	// scope the settings surface reports to the player.
	NarrativeRequirements string                  `json:"narrative_requirements,omitempty"`
	StateSchemaPolicy     *StoryStateSchemaPolicy `json:"state_schema_policy,omitempty"`
	PackBinding           *StoryPackBinding       `json:"pack_binding,omitempty"`
	// IdentityConfirmed is the durable answer to "does this story already know
	// who the player is". It is written once, on the character-creation path, and
	// never derived from a name: a pack that names its own protagonist produces
	// the same non-empty name without ever asking the player anything.
	IdentityConfirmed bool `json:"identity_confirmed,omitempty"`
	// LocalEntitySettings is what the player decided about this story's own
	// entities — the tags they file them under and how much of each the model
	// reads. It is published on the summary because the 本局新建 page is a
	// projection of the story rather than a second record of it: the page reads
	// these settings from the same place the turn reads them.
	LocalEntitySettings map[string]LocalEntitySettings `json:"local_entity_settings,omitempty"`
	CreatedAt           string                         `json:"created_at"`
	UpdatedAt           string                         `json:"updated_at"`
	Branches            int                            `json:"branches"`
	Events              int                            `json:"events"`
	// TurnCount is the canonical depth of the story's current branch. Journal
	// side events and turns that only exist on another branch are excluded.
	TurnCount int `json:"turn_count"`
}

type StoryOpeningConfig struct {
	Mode       string `json:"mode"`
	PresetID   string `json:"preset_id,omitempty"`
	PresetText string `json:"preset_text,omitempty"`
	CustomText string `json:"custom_text,omitempty"`
}

// StoryProtagonist is the story-owned snapshot of the player character.
// Actor identity remains the reserved "protagonist" ID; Lore identity is
// provenance only and must never replace the runtime Actor ID.
type StoryProtagonist struct {
	Mode                string `json:"mode"`
	Name                string `json:"name,omitempty"`
	Profile             string `json:"profile,omitempty"`
	SourceLoreItemID    string `json:"source_lore_item_id,omitempty"`
	SourceLoreUpdatedAt string `json:"source_lore_updated_at,omitempty"`
}

type StoryImageSettings struct {
	Mode          string `json:"mode"`
	IntervalTurns int    `json:"interval_turns,omitempty"`
	PresetID      string `json:"preset_id,omitempty"`
}

type BranchMeta struct {
	Head                  string                     `json:"head"`
	CreatedAt             string                     `json:"created_at"`
	From                  string                     `json:"from,omitempty"`
	FromEvent             string                     `json:"from_event,omitempty"`
	Title                 string                     `json:"title,omitempty"`
	RuntimeConfig         *conversationconfig.Config `json:"runtime_config,omitempty"`
	RuntimeConfigRevision uint64                     `json:"runtime_config_revision,omitempty"`
}

type BranchSummary struct {
	ID        string `json:"id"`
	Head      string `json:"head"`
	From      string `json:"from,omitempty"`
	FromEvent string `json:"from_event,omitempty"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at"`
	Current   bool   `json:"current"`
}

type StoryMeta struct {
	V                  int                      `json:"v"`
	Type               string                   `json:"type"`
	StoryID            string                   `json:"story_id"`
	Title              string                   `json:"title"`
	TitleSource        string                   `json:"title_source"`
	Origin             string                   `json:"origin"`
	Protagonist        StoryProtagonist         `json:"protagonist"`
	StoryTellerID      string                   `json:"story_teller_id"`
	PlanningTemplateID string                   `json:"planning_template_id,omitempty"`
	StoryDirectorID    string                   `json:"story_director_id,omitempty"`
	PlanningMode       string                   `json:"planning_mode"`
	ModuleRefs         *StoryDirectorModuleRefs `json:"module_refs,omitempty"`
	// FrozenModules is the resolved module graph this story started with. It is
	// written once, at creation, and read back on every later turn so editing a
	// preset file changes the next game rather than a story in progress. It is
	// separate from ModuleRefs, which stays the user-editable selection shown in
	// the settings UI.
	FrozenModules    *StoryDirectorResolvedSnapshot `json:"frozen_modules,omitempty"`
	ReplyTargetChars int                            `json:"reply_target_chars"`
	ChoiceCount      int                            `json:"choice_count"`
	Opening          StoryOpeningConfig             `json:"opening"`
	ImageSettings    StoryImageSettings             `json:"image_settings"`
	CheckSettings    StoryCheckSettings             `json:"check_settings,omitempty"`
	SpeechSettings   StorySpeechSettings            `json:"speech_settings,omitempty"`
	IntimacySettings StoryIntimacySettings          `json:"intimacy_settings,omitempty"`
	StylePurposes    StoryStylePurposes             `json:"style_purposes,omitempty"`
	// NarrativeRequirements is the story's own creative direction, stored in the
	// journal so it is frozen with the story rather than read from a shared
	// workspace file on every turn.
	NarrativeRequirements string                  `json:"narrative_requirements,omitempty"`
	StateSchemaPolicy     *StoryStateSchemaPolicy `json:"state_schema_policy,omitempty"`
	PackBinding           *StoryPackBinding       `json:"pack_binding,omitempty"`
	// PackCatalog is the discoverable-people index frozen when the story started.
	// It is read back on every turn instead of the live pack's catalog, so editing
	// a pack changes the next game rather than a story in progress.
	PackCatalog *PackCatalogSnapshot `json:"pack_catalog,omitempty"`
	// Calendar is the year/month/day convention frozen when the story started.
	// It is stored rather than read from the pack on every turn for the same
	// reason PackCatalog is: editing a pack must change the next game rather
	// than one already under way.
	//
	// Nil means this story never froze one, and every lifespan question about it
	// stays unanswerable. It is never back-filled: an in-progress story whose
	// prose was written under no stated year length would otherwise be re-read
	// against a convention it never had.
	Calendar *StoryCalendar `json:"calendar,omitempty"`
	// ProtagonistAge is the age the pack stated for this story's protagonist at
	// creation. Zero means it stated none. It is frozen rather than re-read
	// because the pack may move on, and an age that changes under a running
	// story is a lifespan that moves with it.
	ProtagonistAge int `json:"protagonist_age,omitempty"`
	// ProtagonistGender is the gender the protagonist's body stated at creation,
	// frozen for the same reason the age is. Empty means the story never settled
	// it, which is a real state: a rule that needs it must refuse rather than
	// assume, because assuming is how a person ends up with a gender nobody
	// chose.
	ProtagonistGender string `json:"protagonist_gender,omitempty"`
	// IdentityConfirmed is written once, at creation, when the protagonist came
	// from a confirmed character creation. A story without it keeps the stage's
	// setup screen, because nothing else records that the player was ever asked.
	IdentityConfirmed bool                    `json:"identity_confirmed,omitempty"`
	InitialTraitRolls []InitialActorTraitRoll `json:"initial_trait_rolls,omitempty"`
	// LocalEntitySettings is what the player decided about the entities this
	// story registered — the tags they file them under and how much of each the
	// model reads. It is keyed by the entity's own identity, so a person, a
	// place, and a faction are addressed the same way they are listed. It is
	// absent until someone sets one, and an entity with no record reads with
	// the defaults.
	LocalEntitySettings       map[string]LocalEntitySettings   `json:"local_entity_settings,omitempty"`
	ActorStateSchema          *ActorStateSchemaSnapshot        `json:"actor_state_schema,omitempty"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"state_schema_initialization,omitempty"`
	CurrentBranch             string                           `json:"current_branch"`
	Branches                  map[string]BranchMeta            `json:"branches"`
	CreatedAt                 string                           `json:"created_at"`
	UpdatedAt                 string                           `json:"updated_at"`
}

type TurnEvent struct {
	V           int                `json:"v"`
	Type        string             `json:"type"`
	ID          string             `json:"id"`
	ParentID    any                `json:"parent_id"`
	BranchID    string             `json:"branch_id"`
	Ts          string             `json:"ts"`
	User        string             `json:"user"`
	Attachments []agent.Attachment `json:"attachments,omitempty"`
	// UserContextOnly keeps host-owned autonomous instructions available to
	// future model turns while hiding them from the player-authored timeline.
	UserContextOnly bool   `json:"user_context_only,omitempty"`
	Narrative       string `json:"narrative"`
	Thinking        string `json:"thinking,omitempty"`
	// Recap is this turn's own one-paragraph recollection of what it published.
	// The history list shows it; a turn without one shows a prose excerpt. It is
	// rebuilt display material, never a second record of what happened.
	Recap            string `json:"recap,omitempty"`
	RunID            string `json:"run_id,omitempty"`
	AgentKind        string `json:"agent_kind,omitempty"`
	AgentCommandID   string `json:"agent_command_id,omitempty"`
	AgentOperationID string `json:"agent_operation_id,omitempty"`
	AgentCycle       int    `json:"agent_cycle,omitempty"`
	AgentCommitHash  string `json:"agent_commit_hash,omitempty"`
	// ProviderContinuation is hydrated from a private side event for model
	// history only. It is deliberately absent from public Game JSON.
	ProviderContinuation map[string]any `json:"-"`
	PlayerInputID        string         `json:"player_input_id,omitempty"`
	PlayerInputHash      string         `json:"player_input_hash,omitempty"`
	// ConsumedPlayerInputIDs closes every reachable accepted input whose intent
	// was visible to this successful cycle. PlayerInputID remains the exact
	// current command identity; older interrupted inputs are resolved without
	// deleting their append-only audit events.
	ConsumedPlayerInputIDs []string `json:"consumed_player_input_ids,omitempty"`
	// ResolvedPlayerInputContexts preserves interrupted inputs at their original
	// acceptance boundary after this Turn closes them. The current Turn's input
	// remains represented by User and ModelContextMessages; only older pending
	// inputs belong here, so their model evidence cannot be reordered into this
	// Turn during a cold model-history projection.
	ResolvedPlayerInputContexts []ResolvedPlayerInputContext `json:"resolved_player_input_contexts,omitempty"`
	DisplayEvents               []DisplayEvent               `json:"display_events,omitempty"`
	ModelContextMessages        []ModelContextMessage        `json:"model_context_messages,omitempty"`
	StateDelta                  *StateDelta                  `json:"state_delta,omitempty"`
	HotState                    *HotState                    `json:"hot_state,omitempty"`
	RuleResolution              *RuleResolution              `json:"rule_resolution,omitempty"`
	TurnResult                  *TurnResult                  `json:"turn_result,omitempty"`
	// ClockAdvance is backend-authored evidence for an explicit wait action.
	// Domain modules are deliberately not persisted in TurnResult, so this keeps
	// the accepted source and resulting interval on the canonical journal turn.
	ClockAdvance    *TurnClockAdvance `json:"clock_advance,omitempty"`
	TerminalOutcome *TerminalOutcome  `json:"terminal_outcome,omitempty"`
	StateStatus     string            `json:"state_status,omitempty"`
	StateError      string            `json:"state_error,omitempty"`
	Alts            []TurnAlt         `json:"alts,omitempty"`
	AltIdx          int               `json:"alt_idx,omitempty"`
	Versions        []TurnVersion     `json:"versions,omitempty"`
	VersionIdx      int               `json:"version_idx,omitempty"`
	Flags           map[string]bool   `json:"flags,omitempty"`
}

type TurnClockAdvance struct {
	Action          string     `json:"action"`
	SourceText      string     `json:"source_text"`
	DurationSeconds int64      `json:"duration_seconds"`
	Before          StoryClock `json:"before"`
	After           StoryClock `json:"after"`
}

const TokenUsageEventType = "token_usage"

// DisplayEventRoleNarrative marks the position where the turn narrative was
// streamed relative to thinking/tool events. It carries no content; the UI
// renders turn.narrative at this anchor so submission tool cards stay after
// the prose instead of being folded into the thinking trace group.
const DisplayEventRoleNarrative = "narrative"

// DisplayEvent 表示互动回合中只用于前端展示的事件，例如思考过程和工具调用卡片。
// 它不进入下一轮 Agent 上下文；Args/Result 仅用于追溯当时的工具调用过程。
// Role 为 narrative 的事件是正文位置锚点：正文本身不进入 DisplayEvents，
// 锚点只标记正文在事件流中的相对位置，供前端按真实顺序穿插渲染。
type DisplayEvent struct {
	Phase             string                  `json:"phase,omitempty"`
	RuntimeManaged    bool                    `json:"runtime_managed,omitempty"`
	AgentCycle        int                     `json:"agent_cycle,omitempty"`
	ID                string                  `json:"id,omitempty"`
	Role              string                  `json:"role"`
	Content           string                  `json:"content,omitempty"`
	Name              string                  `json:"name,omitempty"`
	Args              string                  `json:"args,omitempty"`
	Status            string                  `json:"status,omitempty"`
	Result            string                  `json:"result,omitempty"`
	ToolPresentation  *agent.ToolPresentation `json:"tool_presentation,omitempty"`
	CreatedAt         string                  `json:"created_at,omitempty"`
	AgentKind         string                  `json:"agent_kind,omitempty"`
	AgentName         string                  `json:"agent_name,omitempty"`
	RootAgentName     string                  `json:"root_agent_name,omitempty"`
	RunPath           []string                `json:"run_path,omitempty"`
	SubAgent          bool                    `json:"subagent,omitempty"`
	RunID             string                  `json:"run_id,omitempty"`
	SubAgentSessionID string                  `json:"subagent_session_id,omitempty"`
	SubAgentType      string                  `json:"subagent_type,omitempty"`
}

// ModelContextMessage is model-visible turn evidence hidden from the chat UI.
// It losslessly stores every Agent Message field needed to reconstruct future
// model input. Provider continuation remains in its private side event so it
// is not exposed by ordinary Game API projections.
type ModelContextMessage struct {
	Role                     string                           `json:"role"`
	Content                  string                           `json:"content,omitempty"`
	Attachments              []agent.Attachment               `json:"attachments,omitempty"`
	MultiContent             []json.RawMessage                `json:"multi_content,omitempty"`
	UserInputMultiContent    []json.RawMessage                `json:"user_input_multi_content,omitempty"`
	AssistantGenMultiContent []json.RawMessage                `json:"assistant_output_multi_content,omitempty"`
	Name                     string                           `json:"name,omitempty"`
	ToolCalls                []ModelContextToolCall           `json:"tool_calls,omitempty"`
	ToolCallID               string                           `json:"tool_call_id,omitempty"`
	ToolName                 string                           `json:"tool_name,omitempty"`
	ToolResult               *agent.ToolResultSummary         `json:"tool_result,omitempty"`
	ResponseMeta             *agent.ResponseMeta              `json:"response_meta,omitempty"`
	AgentMeta                *agent.AgentMessageMeta          `json:"agent_meta,omitempty"`
	TaskCompletion           *agent.TaskCompletionMessageMeta `json:"task_completion,omitempty"`
	ReasoningContent         string                           `json:"reasoning_content,omitempty"`
	Extra                    map[string]any                   `json:"extra,omitempty"`
	// ProviderContinuation is persisted through a private side event and is
	// hydrated only for model-history projections.
	ProviderContinuation map[string]any `json:"-"`
}

type ModelContextToolCall struct {
	Index    *int                     `json:"index,omitempty"`
	ID       string                   `json:"id"`
	Type     string                   `json:"type"`
	Function ModelContextFunctionCall `json:"function"`
	Extra    map[string]any           `json:"extra,omitempty"`
}

type ModelContextFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type TokenUsageEvent struct {
	V                    int              `json:"v"`
	Type                 string           `json:"type"`
	ID                   string           `json:"id"`
	StoryID              string           `json:"story_id,omitempty"`
	BranchID             string           `json:"branch_id"`
	CreatedAt            string           `json:"created_at"`
	RunID                string           `json:"run_id,omitempty"`
	AgentKind            string           `json:"agent_kind,omitempty"`
	PromptTokens         int              `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int              `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int              `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64          `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int              `json:"completion_tokens,omitempty"`
	ReasoningTokens      int              `json:"reasoning_tokens,omitempty"`
	TotalTokens          int              `json:"total_tokens,omitempty"`
	ModelCalls           int              `json:"model_calls,omitempty"`
	GeneratedBytes       int              `json:"generated_bytes,omitempty"`
	UsageCalls           []TokenUsageCall `json:"usage_calls,omitempty"`
}

type TokenUsageCall struct {
	Index                int      `json:"index,omitempty"`
	CreatedAt            string   `json:"created_at,omitempty"`
	FinishReason         string   `json:"finish_reason,omitempty"`
	RequestedTools       []string `json:"requested_tools,omitempty"`
	AfterTools           []string `json:"after_tools,omitempty"`
	PromptTokens         int      `json:"prompt_tokens,omitempty"`
	CachedPromptTokens   int      `json:"cached_prompt_tokens,omitempty"`
	UncachedPromptTokens int      `json:"uncached_prompt_tokens,omitempty"`
	CacheHitRate         float64  `json:"cache_hit_rate,omitempty"`
	CompletionTokens     int      `json:"completion_tokens,omitempty"`
	ReasoningTokens      int      `json:"reasoning_tokens,omitempty"`
	TotalTokens          int      `json:"total_tokens,omitempty"`
}

type TurnAlt struct {
	Narrative string `json:"narrative"`
	Ts        string `json:"ts"`
}

type TurnVersion struct {
	TurnID  string `json:"turn_id"`
	Ts      string `json:"ts"`
	Current bool   `json:"current"`
}

type StateDelta struct {
	SchemaVersion int                   `json:"schema_version,omitempty"`
	Ops           []interactivestate.Op `json:"ops"`
	ActorOps      []ActorStateOp        `json:"actor_ops,omitempty"`
	// WrittenAtSeconds is the branch's formal instant once this record was done
	// with it, and WrittenAtEvent is the record that wrote these values. They are
	// stamped onto each Actor this delta touches, so a record written before a
	// long interval can be read as old instead of as a description of the world
	// now. Both are absent on every delta written before this field existed, and
	// on a delta whose writing event could not be named.
	WrittenAtSeconds int64  `json:"written_at_seconds,omitempty"`
	WrittenAtEvent   string `json:"written_at_event,omitempty"`
}

type HotState struct {
	Choices []string `json:"choices"`
}

type HotChoicesEvent struct {
	V        int      `json:"v"`
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	ParentID string   `json:"parent_id"`
	BranchID string   `json:"branch_id"`
	Ts       string   `json:"ts"`
	Choices  []string `json:"choices"`
}

type StateDeltaEvent struct {
	TerminalOutcome  *TerminalOutcome        `json:"terminal_outcome,omitempty"`
	ClockAdvance     *TurnClockAdvance       `json:"clock_advance,omitempty"`
	AdventureReceipt *AdventureActionReceipt `json:"adventure_receipt,omitempty"`
	V                int                     `json:"v"`
	Type             string                  `json:"type"`
	ID               string                  `json:"id"`
	ParentID         string                  `json:"parent_id"`
	BranchID         string                  `json:"branch_id"`
	Ts               string                  `json:"ts"`
	SchemaVersion    int                     `json:"schema_version,omitempty"`
	Ops              []interactivestate.Op   `json:"ops"`
	ActorOps         []ActorStateOp          `json:"actor_ops,omitempty"`
	// WrittenAtSeconds and WrittenAtEvent mirror the fields of the same name on
	// StateDelta, for the side record a later revision or a rule settlement
	// writes on its own.
	WrittenAtSeconds int64  `json:"written_at_seconds,omitempty"`
	WrittenAtEvent   string `json:"written_at_event,omitempty"`
}

// ContextCompactionProjection is the read-only product projection of the
// public Agent checkpoint currently bound to a Story branch. Story Store never
// persists this value and therefore cannot become a second maintenance
// authority.
type ContextCompactionProjection struct {
	ID       string `json:"id"`
	BranchID string `json:"branch_id"`
	agentcontext.CompactionCheckpoint
}

// TurnVersionProjection records one immutable event copied from the previous
// canonical suffix. The source event remains byte-for-byte auditable while the
// projected event receives a new ID and parent on the selected version path.
type TurnVersionProjection struct {
	SourceID    string `json:"source_id"`
	ProjectedID string `json:"projected_id"`
	EventType   string `json:"event_type"`
}

// TurnVersionSelectionEvent is an append-only audit record for a canonical
// version choice. It deliberately stays off the active ancestry: ProjectedHeadID
// is the branch head, while ParentID only links the audit record to that result.
type TurnVersionSelectionEvent struct {
	V               int                     `json:"v"`
	Type            string                  `json:"type"`
	ID              string                  `json:"id"`
	ParentID        string                  `json:"parent_id,omitempty"`
	BranchID        string                  `json:"branch_id"`
	Ts              string                  `json:"ts"`
	ReplacedTurnID  string                  `json:"replaced_turn_id"`
	SelectedTurnID  string                  `json:"selected_turn_id"`
	PreviousHeadID  string                  `json:"previous_head_id,omitempty"`
	ProjectedHeadID string                  `json:"projected_head_id,omitempty"`
	ProjectedEvents []TurnVersionProjection `json:"projected_events,omitempty"`
	CurrentState    map[string]any          `json:"current_state,omitempty"`
	CurrentTurnID   string                  `json:"current_turn_id,omitempty"`
	CurrentDepth    int                     `json:"current_depth,omitempty"`
	CurrentPlan     *BranchPlan             `json:"current_plan,omitempty"`
	// CurrentCharacters is the character memory at the selected version, so
	// switching versions switches memory with it.
	CurrentCharacters *character.State `json:"current_characters,omitempty"`
	// CurrentCombats is the live battles at the selected version, so switching
	// versions switches the fights with the wounds they produced.
	CurrentCombats map[string]cultivation.CombatSession `json:"current_combats,omitempty"`
	// CurrentNPCGrowth is the growth watermark at the selected version, so
	// switching versions also switches which intervals the rules have been paid
	// for. A selection that omits it leaves the watermark where it is.
	CurrentNPCGrowth map[string]npcgrowth.Snapshot `json:"current_npc_growth,omitempty"`
}

type BranchEvent struct {
	V               int            `json:"v"`
	Type            string         `json:"type"`
	ID              string         `json:"id"`
	ParentID        string         `json:"parent_id"`
	BranchID        string         `json:"branch_id"`
	From            string         `json:"from"`
	Ts              string         `json:"ts"`
	Title           string         `json:"title"`
	StateCheckpoint map[string]any `json:"state_checkpoint,omitempty"`
	LatestTurnID    string         `json:"latest_turn_id,omitempty"`
	Depth           int            `json:"depth,omitempty"`
	PlanCheckpoint  *BranchPlan    `json:"plan_checkpoint,omitempty"`
	// CharacterCheckpoint carries the parent branch's character memory into the
	// fork. A fork inherits everything the parent knew at that point, and from
	// then on the two branches remember separately.
	CharacterCheckpoint *character.State `json:"character_checkpoint,omitempty"`
	// CombatCheckpoint carries the parent branch's live battles into the fork.
	// It travels with the state checkpoint: a fork inherits the wounds the parent
	// had taken, so it must inherit the fight that dealt them, or the window
	// would show a wound with no battle behind it.
	CombatCheckpoint map[string]cultivation.CombatSession `json:"combat_checkpoint,omitempty"`
	// ClockCheckpoint carries the parent branch's formal time axis into the fork.
	// It travels with the state checkpoint for the same reason the battles do: a
	// fork inherits the pools the parent had reached, so it must inherit the
	// instant those pools were reached at.
	ClockCheckpoint *StoryClock `json:"clock_checkpoint,omitempty"`
	// RecoveryCheckpoint carries the parent branch's safe recovery activities into
	// the fork. A fork that inherited a half-finished rest without its activity
	// would let the remainder be earned twice.
	RecoveryCheckpoint map[string]RecoveryActivity `json:"recovery_checkpoint,omitempty"`
	// CultivationCheckpoint carries the parent branch's player-named cultivation
	// intervals into the fork. A fork that inherited a half-finished interval
	// without its activity would let the remainder be trained twice.
	CultivationCheckpoint map[string]CultivationActivity `json:"cultivation_checkpoint,omitempty"`
	// JourneyCheckpoint carries the parent branch's journeys into the fork. A
	// fork that inherited half a walked road without its progress would let the
	// remainder be charged twice.
	JourneyCheckpoint     map[string]exploration.JourneySnapshot `json:"journey_checkpoint,omitempty"`
	LocalAffairCheckpoint []LocalAffairState                     `json:"local_affair_checkpoint,omitempty"`
	// RealmCheckpoint carries the parent branch's realm instances into the fork.
	// A fork that inherited an explored realm without its rooms and taken
	// rewards would let the same treasure be found twice.
	RealmCheckpoint map[string]RealmInstanceState `json:"realm_checkpoint,omitempty"`
	// QuestCheckpoint carries the parent branch's task instances into the fork. A
	// fork that inherited an agreement without its state would let the same task
	// be accepted, completed or paid a second time.
	QuestCheckpoint map[string]quest.Instance `json:"quest_checkpoint,omitempty"`
	// PublicEventCheckpoint carries the parent branch's public event participation
	// into the fork. A fork that inherited a participation without its stage would
	// claim the character was present at something they never attended.
	PublicEventCheckpoint map[string]publiquevent.Instance `json:"public_event_checkpoint,omitempty"`
	// NPCGrowthCheckpoint carries the growth watermark into the fork. A fork that
	// inherited an actor's stage without the instant the rules were paid for
	// would either settle the same stretch of life again or refuse the next one
	// as overlapping.
	NPCGrowthCheckpoint map[string]npcgrowth.Snapshot `json:"npc_growth_checkpoint,omitempty"`
}

type Snapshot struct {
	StoryID                    string                       `json:"story_id"`
	BranchID                   string                       `json:"branch_id"`
	ContextRevision            uint64                       `json:"context_revision,omitempty"`
	Turns                      []TurnEvent                  `json:"turns"`
	PendingPlayerInputs        []PlayerInputAcceptedEvent   `json:"pending_player_inputs,omitempty"`
	PendingDisplayEvents       []DisplayEvent               `json:"pending_display_events,omitempty"`
	PendingModelContextBatches []ModelContextBatchEvent     `json:"pending_model_context_batches,omitempty"`
	CurrentTurn                *TurnEvent                   `json:"current_turn,omitempty"`
	TokenUsageEvents           []TokenUsageEvent            `json:"token_usage_events,omitempty"`
	ContextCompaction          *ContextCompactionProjection `json:"context_compaction,omitempty"`
	BranchPlan                 *BranchPlan                  `json:"branch_plan,omitempty"`
	State                      map[string]any               `json:"state"`
	// Clock is the branch's formal time axis. It is attached so a reader that
	// decides whether a thread still binds can read the same instant the rules
	// stamp deadlines from, instead of ordering two pieces of prose.
	Clock                     *StoryClock                      `json:"clock,omitempty"`
	ActorStateSchema          *ActorStateSchemaSnapshot        `json:"actor_state_schema,omitempty"`
	StateSchemaInitialization *StateSchemaInitializationStatus `json:"state_schema_initialization,omitempty"`
	Graph                     StoryGraph                       `json:"graph"`
	TurnCount                 int                              `json:"turn_count"`
	TurnStart                 int                              `json:"turn_start"`
	HistoryBeforeCursor       string                           `json:"history_before_cursor,omitempty"`
	HasEarlierTurns           bool                             `json:"has_earlier_turns"`
}

// StoryHistoryPage is the bounded UI history projection. BeforeCursor is
// opaque to callers and remains tied to one story generation and branch.
type StoryHistoryPage struct {
	StoryID      string      `json:"story_id"`
	BranchID     string      `json:"branch_id"`
	Turns        []TurnEvent `json:"turns"`
	BeforeCursor string      `json:"before_cursor,omitempty"`
	HasMore      bool        `json:"has_more"`
}

type StoryGraph struct {
	Nodes    []PlotNode      `json:"nodes"`
	Branches []BranchSummary `json:"branches"`
}

type PlotNode struct {
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	BranchID     string `json:"branch_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Ts           string `json:"ts"`
	Current      bool   `json:"current"`
	Head         bool   `json:"head"`
	Terminal     bool   `json:"terminal,omitempty"`
	TerminalType string `json:"terminal_type,omitempty"`
}

type StoryContext struct {
	Meta     StoryMeta `json:"meta"`
	Snapshot Snapshot  `json:"snapshot"`
}
