import type { SpeechContentSettings } from '@/features/speech/text'
import type { SSEEvent } from '@/lib/api'
import type { ChatAttachment } from '@/lib/api-client/types'

export type InteractiveSubmode = 'story' | 'timeline'

export type StoryPlanningMode = 'enabled' | 'disabled'
export type StoryTitleSource = 'pending' | 'generated' | 'user'

export interface StorySpeechSettings extends SpeechContentSettings {
  auto_read: boolean
}

/**
 * The intimacy window's own model purpose. It names an existing model profile
 * rather than copying one: editing that profile changes this purpose too, and
 * the main story keeps its own route. It carries no credential.
 */
export interface StoryIntimacySettings {
  profile_id?: string
  thinking_level?: string
}

/**
 * What the player decided about one entity this story registered, as opposed to
 * what a turn established about it.
 *
 * These are the only two fields on the 本局新建 page the player owns. Tags are
 * labels a reader would file the entity under; they are added to a turn's own
 * tags, never instead of them. Load mode is a reading budget, not a stage
 * direction — 常驻 hands the record to the model every turn, and nothing about
 * that makes the entity present, awake, or forced into the scene.
 */
export interface LocalEntitySettings {
  tags?: string[]
  /**
   * One of the world book's own load modes, so a player who set 常驻取用 on a
   * world entry meets the same three names here. Empty means the default
   * reading, which is the same treatment as 自动.
   */
  load_mode?: 'auto' | 'resident' | 'manual'
}

export interface StorySummary {
  id: string
  title: string
  title_source?: StoryTitleSource
  origin: string
  protagonist: StoryProtagonist
  story_teller_id: string
  style_purposes?: StoryStylePurposes
  /**
   * This story's own standing creative direction. Empty means it reads the
   * workspace default (CREATOR.md); a value means this story alone reads that
   * text, so two stories in one workspace never share one rule set.
   */
  narrative_requirements?: string
  planning_template_id: string
  planning_mode?: StoryPlanningMode
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  speech_settings?: StorySpeechSettings
  intimacy_settings?: StoryIntimacySettings
  image_settings?: StoryImageSettings
  check_settings?: Partial<StoryCheckSettings>
  opening: StoryOpeningConfig
  state_schema_policy?: StoryStateSchemaPolicy
  /** The script pack this story started from, frozen at creation time. */
  pack_binding?: StoryPackBinding
  /**
   * True when this story's protagonist came out of a confirmed character
   * creation. The stage uses it to skip its setup screen: that screen asks the
   * player to choose a protagonist, and this story already has the one they
   * chose. It is not derivable from a non-empty name, because a pack that
   * declares its own protagonist produces one without asking anyone.
   */
  identity_confirmed?: boolean
  /**
   * What the player decided about this story's own entities — the tags they
   * file them under and how much of each the model reads. Keyed by the entity's
   * own identity, so a person, a place and a faction are addressed the same way
   * they are listed. Absent means the player has decided nothing yet.
   */
  local_entity_settings?: Record<string, LocalEntitySettings>
  created_at: string
  updated_at: string
  branches: number
  events: number
  turn_count: number
}

/**
 * The script pack a story is bound to. It freezes the world book, module set
 * and lore scope the story started with, so editing a pack later never silently
 * rewrites an in-progress game.
 */
export interface WorldPresentationTerms {
  combat?: string
  exploration?: string
  techniques?: string
  partner?: string
  energy?: string
}
export type WorldPresentationOverrides = Partial<Record<'zh-CN' | 'en-US', WorldPresentationTerms>>

export interface StoryPackBinding {
  pack_id: string
  pack_name?: string
  content_version?: number
  content_hash?: string
  scope: string
  module_refs?: StoryDirectorModuleRefs
  gameplay?: {
    status_template: string
    growth: string
    combat: string
    communication?: string
    letter_delivery_seconds?: number
    presentation?: WorldPresentationOverrides
    opening: string
  }
  bound_at?: string
}

export type StoryStateSchemaMode = 'adapt_template' | 'fixed_template' | 'generate'

export interface StoryStateSchemaPolicy {
  mode: StoryStateSchemaMode
}

type StoryImageMode = 'manual' | 'interval'

export interface StoryImageSettings {
  mode: StoryImageMode
  interval_turns: number
  preset_id?: string
}

/**
 * A new story opens either on a scene the model writes or on text the player
 * wrote. `preset` is only ever read: stories created while the world preset
 * library existed still carry their snapshot and still open from it.
 */
type StoryOpeningMode = 'ai' | 'custom' | 'preset'

export interface StoryOpeningConfig {
  mode: StoryOpeningMode
  preset_id?: string
  preset_text?: string
  custom_text?: string
}

export interface StoryIndex {
  current_story_id: string
  stories: StorySummary[]
}

export interface Teller {
  version: number
  id: string
  name: string
  description: string
  /**
   * The preset's own English display strings. The migrated narrative styles
   * carry their name and description in both languages on the definition rather
   * than as locale keys, so the picker can show English without a second set of
   * translations to keep in step. Absent on the pre-migration styles, which use
   * the `narrativeStyle.builtin.*` locale keys instead.
   */
  name_en?: string
  description_en?: string
  /** Picker grouping for the migrated styles: five groups in library order. */
  group?: string
  group_order?: number
  order?: number
  style_refs?: string[] | null
  style_rules?: StyleRule[] | null
  context_policy: TellerContextPolicy
  slots: TellerPromptSlot[]
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export interface ImagePreset {
  version: number
  id: string
  name: string
  description: string
  prompt?: string
  slots?: ImagePresetSlot[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export interface GamePlanningSection {
  id: string
  title: string
  description: string
}

export interface GamePlanningTemplate {
  version: number
  id: string
  name: string
  description: string
  sections: GamePlanningSection[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export interface StoryDirectorModuleRefs {
  narrative_style_id?: string
  narrative_style_disabled?: boolean
  event_package_ids?: string[]
  event_packages_disabled?: boolean
  rule_system_id?: string
  rule_system_disabled?: boolean
  actor_state_id?: string
  actor_state_disabled?: boolean
  image_preset_id?: string
  image_preset_disabled?: boolean
}

export interface EventPackageModule {
  version: number
  id: string
  name: string
  description: string
  events?: TellerEventCard[]
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export interface RuleSystemModule {
  version: number
  id: string
  name: string
  description: string
  actor_state_id?: string
  trpg_system: StoryDirectorTRPGSystem
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export interface ActorStateModule {
  version: number
  id: string
  name: string
  description: string
  actor_state: StoryDirectorActorStateSystem
  path?: string
  custom: boolean
  builtin_overridden?: boolean
  invalid?: boolean
  error?: string
  created_at?: string
  updated_at?: string
  revision?: string
}

export type StoryProtagonistMode = 'default' | 'custom' | 'lore'

export interface StoryProtagonist {
  mode: StoryProtagonistMode
  name?: string
  profile?: string
  source_lore_item_id?: string
  source_lore_updated_at?: string
}

export interface StoryCheckSettings {
  difficulty_shift: number
  roll_modifier: number
  rule_state_consumption_mode?: 'hybrid_auto' | 'suggestions_only'
  rule_visibility_mode?: 'audit_only' | 'public_roll'
}

export interface InteractiveStoryUpdateInput {
  title?: string
  origin?: string
  protagonist?: StoryProtagonist
  story_teller_id?: string
  style_purposes?: StoryStylePurposesUpdate
  /**
   * Omitted leaves the stored direction alone; an empty string clears it so the
   * story falls back to the workspace default. The distinction is what lets the
   * settings surface drop an override without erasing it by accident.
   */
  narrative_requirements?: string
  planning_template_id?: string
  planning_mode?: StoryPlanningMode
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars?: number
  choice_count?: number
  speech_settings?: StorySpeechSettings
  intimacy_settings?: StoryIntimacySettings
  image_settings?: StoryImageSettings
  check_settings?: StoryCheckSettings
  opening?: StoryOpeningConfig
  state_schema_policy?: StoryStateSchemaPolicy
  /**
   * Replaces the whole per-entity settings map. The page always sends what it is
   * showing, so a whole-map replace is one write instead of a merge that could
   * resurrect a record the player just cleared. Omitting it leaves the stored
   * settings alone; an empty object clears them.
   */
  local_entity_settings?: Record<string, LocalEntitySettings>
}

/**
 * One expression configuration per use. The main narrative is the base voice;
 * dialogue covers character speech in the narrative and in a character window
 * alike, and combat covers the description of an exchange that has already been
 * settled.
 */
export interface StoryStylePurposes {
  main?: string
  dialogue?: string
  combat?: string
}

/**
 * A partial change to the three purposes. An omitted purpose is left as it is;
 * a purpose set to the empty string goes back to following the main narrative.
 * The two are different choices, so the caller must send `''` rather than omit
 * the field to clear a purpose.
 */
export interface StoryStylePurposesUpdate {
  main?: string
  dialogue?: string
  combat?: string
}

export interface StoryDirectorTRPGSystem {
  rule_templates?: RuleCheck[]
}

export interface StoryDirectorActorStateSystem {
  /** Read-only naming metadata from this Story’s frozen crafting catalog. */
  craft_methods?: { id: string; version: number; name_key: string }[]
  adventure_rules?: { family: 'wuxia' | 'space' | 'fantasy'; progression?: { max_rank: number; stages: { id: string; name: string; english_name: string; min_rank: number }[] }; skills: { id: string; name: string }[]; items: { id: string; name: string; max_charges?: number }[] }
  templates?: ActorStateTemplate[]
  initial_actors?: ActorStateInitialActor[]
  trait_pools?: ActorTraitPool[]
}

export interface ActorStateTemplate {
  id: string
  name: string
  description?: string
  fields?: ActorStateField[]
  trait_rules?: ActorTraitRule[]
  /** Legacy Beta input retained for API compatibility; normalized schemas ignore it. */
  display_groups?: string[]
}

export interface ActorTraitRule {
  pool_id: string
  draw_count: number
}

export interface ActorTraitPool {
  id: string
  name: string
  description?: string
  traits?: ActorTraitDefinition[]
}

export interface ActorTraitDefinition {
  id: string
  name: string
  summary?: string
  weight?: number
}

export interface ActorTraitInstance {
  pool_id: string
  pool_name?: string
  trait_id: string
  name: string
  summary?: string
  source_kind?: string
  source_id?: string
  source_turn_id?: string
}

export interface ActorTraitSelection {
  pool_id: string
  trait_ids?: string[]
}

export interface InitialActorTraitRoll {
  actor_id: string
  selections?: ActorTraitSelection[]
  seed?: number
}

export interface ActorStateField {
  id?: string
  path?: string
  name: string
  type: 'number' | 'string' | 'bool' | 'enum' | 'object' | 'list' | string
  default?: unknown
  min?: number
  max?: number
  options?: string[]
  description?: string
  update_instruction?: string
  /** Legacy Beta input retained for API compatibility; field array order is the fallback. */
  order?: number
  /** Optional presentation hint: cluster fields under one named ledger section. */
  group?: string
  /** Optional presentation hint: pin the field renderer; falls back to heuristics when empty. */
  display?: 'stat' | 'inline' | 'block' | 'list'
  /**
   * Name of a sibling field whose current value is this field's maximum.
   *
   * The two cultivation pools use it: 生命 is bounded by 生命上限, which is
   * itself a settled number that moves with the character's stage, so neither
   * can carry a fixed max. The ledger resolves the sibling to show a truthful
   * "current / maximum" and draw a real meter.
   */
  max_field?: string
  /**
   * Optional player-visibility declaration from the world pack.
   *
   * The backend withholds a private field for actors the player is not playing,
   * so the interface normally never sees it. The declaration is still on the
   * schema because the panel must tell "not found out yet" apart from "has no
   * value": a private field with no value reads as 未知, not as an empty slot.
   */
  visibility?: '' | 'private'
  /**
   * Optional vocabulary of the stored value.
   *
   * `stage` means the stored string is a cultivation stage key (e.g. `mortal`,
   * `qi_6`) and the panel renders the wording a player reads rather than the
   * key. The journal keeps the key; this only changes how it is displayed.
   */
  value_format?: '' | 'stage'
}

export interface ActorStateInitialActor {
  id: string
  name: string
  template_id: string
  role?: string
  description?: string
  state?: Record<string, unknown>
}

export interface ImagePresetSlot {
  id: string
  name: string
  target: 'agent_system' | 'tool_request'
  enabled: boolean
  content: string
}

export interface StyleRule {
  scene: string
  style_refs?: string[]
  style_contents?: string[]
}

export interface StyleReference {
  name: string
  description: string
  path: string
  display_path: string
  size?: number
  updated_at?: string
  missing?: boolean
  error?: string
}

export interface StyleReferenceFileDocument {
  reference: StyleReference
  content: string
  revision: string
}

export interface TellerEventPackage {
  id?: string
  name?: string
  enabled: boolean
  events?: TellerEventCard[]
}

export interface TellerEventCard {
  id?: string
  type_name?: string
  description_markdown?: string
  enabled: boolean
  category?: string
  tags?: string[]
  intensity?: string
}

interface TellerContextPolicy {
  creator: string
  lore: string
  runtime_state: string
}

export interface TellerPromptSlot {
  id: string
  name: string
  target: 'system' | 'turn_context'
  enabled: boolean
  content: string
}

/**
 * A branch's formal time axis: elapsed seconds from a per-branch origin.
 *
 * `anchor` is the story's own words for that origin (三月初七清晨). It is a
 * label, never an input to arithmetic: the world pack has no bound calendar, so
 * inventing a date from it would be claiming a fact nobody defined.
 */
export interface StoryClock {
  version: number
  anchor?: string
  seconds: number
}

/** The committed interval of one explicit wait, with the source that earned it. */
export interface TurnClockAdvance {
  action: string
  source_text: string
  duration_seconds: number
  before: StoryClock
  after: StoryClock
}

export interface TurnEvent {
  id: string
  parent_id: string | null
  branch_id: string
  ts: string
  user: string
  attachments?: ChatAttachment[]
  user_context_only?: boolean
  narrative: string
  thinking?: string
  // The turn's own short recollection for the history list. Absent on turns
  // published before the field existed and on any turn whose model omitted it;
  // the history then shows an excerpt of the prose instead.
  recap?: string
  run_id?: string
  agent_kind?: string
  display_events?: TurnDisplayEvent[]
  state_delta?: StateDelta
  hot_state?: HotState
  rule_resolution?: RuleResolution
  turn_result?: TurnResult
  /** Backend-authored evidence for an explicit wait: what the player asked for
   * and the interval it really committed. Absent on every turn that spent no
   * in-fiction time. */
  clock_advance?: TurnClockAdvance
  terminal_outcome?: TerminalOutcome
  state_status?: 'pending' | 'ready' | 'failed'
  state_error?: string
  versions?: TurnVersion[]
  version_idx?: number
}

export interface UpdateTurnNarrativeResult {
  turn: TurnEvent
}

export interface UpdateBranchPlanResult {
  branch_plan: BranchPlan
  context_revision: number
}

export interface StoryAssistantSuggestion {
  label: string
  detail: string
  /** The wording this suggestion would put in the player's input. Never sent automatically. */
  action: string
}

/**
 * One read-only advisory answer about the current scene.
 *
 * `turn_id` and `context_revision` bind the answer to the situation it was
 * written against. An answer whose binding no longer matches the stage is stale:
 * it may describe a scene the player has already left.
 */
export interface StoryAssistantResult {
  story_id: string
  branch_id: string
  turn_id: string
  action: 'analyze' | 'inspire'
  read: string
  suggestions?: StoryAssistantSuggestion[] | null
  context_revision: number
}

export interface StoryAssistantAdoptResult {
  story_id: string
  branch_id: string
  turn_id: string
  scope: 'branch'
  branch_plan: BranchPlan
  context_revision: number
  /** False when the direction was already recorded, so a retry is a read. */
  applied: boolean
}

export interface TurnResult {
  state_updates: Array<{ op: 'replace' | 'delta' | 'create' | string; path: string; value: unknown }>
  choices: string[]
}

export interface TurnDisplayEvent {
  phase?: string
  runtime_managed?: boolean
  agent_cycle?: number
  id?: string
  role: 'assistant' | 'thinking' | 'tool_call' | 'tool_result' | 'narrative' | 'context_compaction' | 'todo_updated'
  content?: string
  name?: string
  args?: string
  status?: 'running' | 'success' | 'error' | 'discarded'
  result?: string
  tool_presentation?: import('@/lib/api').ToolPresentation
  created_at?: string
  run_id?: string
  agent_kind?: string
  agent_name?: string
  root_agent_name?: string
  run_path?: string[]
  subagent?: boolean
  subagent_session_id?: string
  subagent_type?: string
  parent_call_id?: string
}

export interface TokenUsageEvent {
  id?: string
  type?: 'token_usage'
  story_id?: string
  branch_id?: string
  created_at?: string
  run_id?: string
  agent_kind?: string
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  completion_tokens?: number
  reasoning_tokens?: number
  total_tokens?: number
  model_calls?: number
  generated_bytes?: number
  usage_calls?: TokenUsageCall[]
}

interface TokenUsageCall {
  index?: number
  created_at?: string
  finish_reason?: string
  requested_tools?: string[]
  after_tools?: string[]
  prompt_tokens?: number
  cached_prompt_tokens?: number
  uncached_prompt_tokens?: number
  cache_hit_rate?: number
  completion_tokens?: number
  reasoning_tokens?: number
  total_tokens?: number
}

interface TurnVersion {
  turn_id: string
  ts: string
  current?: boolean
}

interface StateDelta {
	schema_version?: number
  ops?: StateOp[]
  actor_ops?: ActorStateOp[]
}

export interface ActorStateOp {
  op: string
  actor_id: string
  field_id: string
  value?: unknown
  reason?: string
  source_turn_id?: string
  source_kind?: string
  source_id?: string
  /**
   * What the field held immediately before this write.
   *
   * The write itself carries an absolute value, so it cannot say what changed:
   * 灵石=148 is either two stones spent or none. The backend records the
   * pre-turn value on the same op so the change summary can subtract, and a
   * summary without it could only repeat the new total. Absent on a save
   * written before the field existed, and on a write whose field was empty.
   */
  previous_value?: unknown
}

export interface StateOp {
  op: string
  path: string
  value?: unknown
  reason?: string
  source_turn_id?: string
  source_kind?: string
  source_id?: string
}

interface HotState {
  choices: string[]
}

export interface BranchPlan {
  markdown: string
  updated_turn_id?: string
  updated_at?: string
  revision?: string
}

export interface RuleCheck {
  id?: string
  label?: string
  dice?: '1d20' | string
  modifier?: number
  failure_policy?: 'fail_forward' | 'success_at_cost' | 'blocked' | 'hard_failure' | string
  difficulty_guidance?: string
  state_effect_guidance?: string
  trigger?: string
  must_check_examples?: string[]
  skip_check_examples?: string[]
  success_hint?: string
  failure_hint?: string
  state_bindings?: RuleStateBinding[]
}

export interface RuleStateBinding {
  id?: string
  label?: string
  trigger?: string
  actor_template_id?: string
  target_template_id?: string
  modifiers?: RuleStateBindingModifier[]
  narrative_state_refs?: RuleNarrativeStateRef[]
  outcome_state_changes?: RuleOutcomeStateChangeBinding[]
}

export interface RuleStateBindingModifier {
  source?: 'actor' | 'target' | string
  field_id?: string
  value_path?: string[]
  effect?: 'advantage' | 'resistance' | string
  scale?: number
  offset?: number
  min?: number
  max?: number
  rounding?: 'none' | 'floor' | 'ceil' | 'nearest' | string
  required?: boolean
}

export interface RuleNarrativeStateRef {
  source?: 'actor' | 'target' | 'scene' | string
  field_id?: string
  usage?: 'check_decision' | 'difficulty' | 'outcome_design' | 'prose' | string
  guidance?: string
}

export interface RuleOutcomeStateChangeBinding {
  outcome?: 'critical_success' | 'success' | 'failure' | 'critical_failure' | string
  state_changes?: RuleComputedStateChange[]
}

export interface RuleComputedStateChange {
  source?: 'actor' | 'target' | string
  field_id?: string
  change_formula?: RuleStateChangeFormula
  reason?: string
}

export interface RuleStateChangeFormula {
  base?: number
  terms?: RuleStateFormulaTerm[]
  min?: number
  max?: number
  rounding?: 'none' | 'floor' | 'ceil' | 'nearest' | string
}

export interface RuleStateFormulaTerm {
  source?: 'actor' | 'target' | string
  field_id?: string
  value_path?: string[]
  scale?: number
  offset?: number
}

export interface RuleResolution {
  id?: string
  /** Safe source identity from player projection; never private check input. */
  display_template_id?: string
  request: TurnCheckRequest
  result: RuleResult
  state_consumption?: RuleStateConsumption
  terminal_candidate?: TerminalCandidate
  rule_constraints?: string[]
  created_at?: string
  seed?: number
}

interface TurnCheckRequest {
  action: string
  intent: string
  challenge: string
  cost: string
  state: string
  adjudication?: TurnCheckAdjudication
  rule?: TurnCheckRule
  bonuses?: TurnCheckBonus[]
  difficulty: 'very_easy' | 'easy' | 'normal' | 'hard' | 'very_hard' | string
  outcomes: TurnCheckOutcomes
}

interface TurnCheckRule {
  template?: string
  template_id?: string
  label?: string
  failure_policy?: string
  roll_mode?: 'normal' | 'advantage' | 'disadvantage' | string
  binding_id?: string
  actor_id?: string
  target_actor_id?: string
}

interface TurnCheckBonus {
  kind?: string
  actor_id?: string
  field_id?: string
  reason: string
  value: number
}

interface TurnCheckOutcomes {
  critical_success: TurnCheckOutcome
  success: TurnCheckOutcome
  failure: TurnCheckOutcome
  critical_failure: TurnCheckOutcome
}

interface TurnCheckOutcome {
  result: string
  state_changes?: TurnStateChange[]
}

interface TurnStateChange {
  actor_id: string
  field_id: string
  change: number
  reason?: string
}

interface TurnCheckAdjudication {
  reason?: string
  stakes?: string
  difficulty_reason?: string
  roll_mode_reason?: string
  state_refs?: Array<{ actor_id: string; field_id: string }>
}

interface RuleResult {
  id?: string
  label?: string
  kind?: string
  mode?: string
  dice?: string
  rolls?: number[]
  roll_total?: number
  modifier?: number
  difficulty?: number
  total?: number
  outcome: string
  seed?: number
  constraints?: string[]
  error?: string
  roll_mode?: string
  kept_roll?: number
  bonus_total?: number
  bonus_details?: TurnCheckBonus[]
  base_target?: number
  target?: number
  requested_difficulty?: string
  effective_difficulty?: string
  difficulty_shift?: number
  story_roll_modifier?: number
  result?: string
  state_changes?: TurnStateChange[]
}

interface RuleStateConsumption {
  status: 'none' | 'disabled' | 'applied' | 'partial' | 'skipped' | string
  mode?: 'hybrid_auto' | 'suggestions_only' | string
  applied_ops?: StateOp[]
  applied_actor_ops?: ActorStateOp[]
  warnings?: RuleStateConsumptionWarning[]
}

interface RuleStateConsumptionWarning {
  actor_id?: string
  field_id?: string
  reason: string
}

interface TerminalCandidate {
  type?: string
  reason?: string
  check_id?: string
}

export interface TerminalOutcome {
  terminal: boolean
  type?: string
  reason?: string
  reason_key?: string
  final_narrative_summary?: string
  caused_by_turn_id?: string
  rule_resolution_id?: string
  restart_suggestions?: string[]
}

export interface ActorTraitRollRequest {
  actor_state_id?: string
  actor_id: string
  template_id: string
  selections?: ActorTraitSelection[]
  seed?: number
}

export interface ActorTraitRollResult {
  actor_state_id?: string
  actor_id: string
  template_id: string
  seed: number
  traits: ActorTraitInstance[]
}

export interface RuleResolutionRerollInput {
  branch_id?: string
  turn_id?: string
}

export interface Snapshot {
  pending_display_events?: TurnDisplayEvent[]
  story_id: string
  branch_id: string
  context_revision?: number
  turns: TurnEvent[]
  pending_player_inputs?: PlayerInputAcceptedEvent[]
  pending_model_context_batches?: ModelContextBatchEvent[]
  current_turn?: TurnEvent
  token_usage_events?: TokenUsageEvent[]
  context_compaction?: ContextCompactionProjection | null
  branch_plan?: BranchPlan
  /** The branch's formal time axis; absent until the story spends in-fiction time. */
  clock?: StoryClock
  state: Record<string, unknown>
  actor_state_schema?: ActorStateSchemaSnapshot
	state_schema_initialization?: StateSchemaInitializationStatus
  graph?: StoryGraph
  turn_count?: number
  turn_start?: number
  history_before_cursor?: string
  has_earlier_turns?: boolean
}

// InteractiveSnapshotResponse mirrors internal/interactive.Snapshot exactly.
// Keep this wire DTO separate from Snapshot because the UI also merges SSE
// deltas into its local projection.
export interface InteractiveSnapshotResponse {
  pending_display_events?: TurnDisplayEvent[]
  story_id: string
  branch_id: string
  context_revision?: number
  turns: TurnEvent[]
  pending_player_inputs?: PlayerInputAcceptedEvent[]
  pending_model_context_batches?: ModelContextBatchEvent[]
  current_turn?: TurnEvent
  token_usage_events?: TokenUsageEvent[]
  context_compaction?: ContextCompactionProjection
  branch_plan?: BranchPlan
  clock?: StoryClock
  state: Record<string, unknown>
  actor_state_schema?: ActorStateSchemaSnapshot
  state_schema_initialization?: StateSchemaInitializationStatus
  graph: StoryGraph
  turn_count: number
  turn_start: number
  history_before_cursor?: string
  has_earlier_turns: boolean
}

export interface PlayerInputAcceptedEvent {
  v: number
  type: string
  id: string
  parent_id?: string
  branch_id: string
  ts: string
  text: string
  attachments?: ChatAttachment[]
  context_only?: boolean
  accepted_turn_count: number
  agent_command_id: string
  agent_operation_id: string
  agent_cycle: number
  agent_commit_hash: string
  agent_canonical_hash?: string
}

export interface ModelContextBatchEvent {
  v: number
  type: string
  id: string
  parent_id?: string
  branch_id: string
  ts: string
  player_input_id: string
  agent_command_id: string
  agent_operation_id: string
  agent_cycle: number
  sequence: number
  messages: ModelContextMessage[]
}

export interface ModelContextMessage {
  role: string
  content?: string
  name?: string
  tool_calls?: ModelContextToolCall[]
  tool_call_id?: string
  tool_name?: string
  tool_result?: ToolResultSummary
}

export interface ModelContextToolCall {
  index?: number
  id: string
  type: string
  function: ModelContextFunctionCall
  extra?: Record<string, unknown>
}

export interface ModelContextFunctionCall {
  name?: string
  arguments?: string
}

export interface ToolResultSummary {
  status: string
  synthetic_reason?: string
  model_truncated?: boolean
  display_truncated?: boolean
  result_retention: string
  context_hints?: unknown
  artifact_persistence?: unknown
  protected_receipt?: unknown
  artifacts?: unknown[]
}

export interface StoryHistoryPage {
  story_id: string
  branch_id: string
  turns: TurnEvent[]
  before_cursor?: string
  has_more: boolean
}

/**
 * One person the player can talk to in the character window. The list comes from
 * the story's own actor registry, so it is not a fixed pair of characters.
 */
export interface CharacterWindowCharacter {
  id: string
  name: string
  source_key?: string
  identity_source?: 'author_catalog' | 'story'
  lifeform?: 'human' | 'animal' | 'spirit_beast'
  cultivation_method_id?: string
  known_as?: string
  aliases?: string[]
  avatar?: string
  /**
   * The world book entry's own picture for this person, relative to the project.
   * It is the picture the world gave them when no avatar was chosen for the
   * actor, and it is drawn before the shipped default. Relative because an
   * absolute path would be one machine's layout; the window addresses it through
   * the project it is already reading.
   */
  avatar_lore_path?: string
  /**
   * The same person's larger portrait, for the open conversation. It is absent
   * when the person has no shipped picture, or when the player chose their own
   * avatar — in both cases the small avatar is what the window draws.
   */
  avatar_display?: string
  image_caption?: string
  role?: string
  description?: string
  /**
   * What the story settled about this person: 'male', 'female', or absent when
   * nobody decided. Absent is not a third state to display — the window shows
   * nothing rather than "unknown", because an unsettled gender is not something
   * the player failed to find out.
   */
  gender?: string
  attitude?: string
  /**
   * Short labels the story attached to this person — the household or group they
   * belong to, what they do, where they are from. The overview's search reads
   * them, so remembering "药铺" finds the clerk whose name the player forgot.
   * Only ever what a turn established; absent means nothing was named yet.
   */
  tags?: string[]
  /** Whether the character is in the scene the player is reading. */
  present: boolean
  /** 'scene' when present, 'message' when reached remotely. */
  channel: string
  unread_messages: number
  open_matters: number
  first_seen_world_time?: string
  /**
   * A one-line preview of the newest line in this private conversation, and the
   * story time it was said. Both come from the same transcript the open
   * conversation draws, so the list never promises a line the window lacks.
   */
  last_exchange?: string
  last_world_time?: string
  /** Where that line sits in the branch journal, counting from one. */
  last_sequence?: number
  /** How this character feels about the protagonist, from -100 to 100. */
  affinity: number
  /** The range that number falls in: hostile … deep. */
  affinity_band: string
  /** The formal bond standing between them, if any. Separate from affinity. */
  relationship?: CharacterRelationship
  /**
   * The reviewed clips this surface may play for this person, the one chosen for
   * the current scene first. Absent when this build has no reviewed clip of the
   * role the surface is entitled to, and the window then keeps the still picture
   * rather than drawing an empty player.
   */
  clips?: CharacterMediaClip[]
}

/**
 * One reviewed clip of a person. It carries only what the surface needs to lay
 * the picture out before it has loaded; the file is fetched from the media route
 * by the URL, which is the clip's own content hash.
 */
export interface CharacterMediaClip {
  url: string
  /** 'general' for an ordinary day, 'intimate' for the private set. */
  role: string
  duration_ms?: number
  width?: number
  height?: number
}

/** A formal bond, as the story recorded it. */
export interface CharacterRelationship {
  id: string
  character_id: string
  kind: string
  statement: string
  world_time: string
  /** Canonical exchanges establishing this bond; not invented evidence. */
  expressed_by?: string
  answered_by?: string
  turn_id?: string
  source_event_id?: string
}

/**
 * One person's picture as the window would draw it, and which layer answered.
 *
 * `source` is 'chosen' for the player's own avatar, 'external' for a picture the
 * author imported, 'lore' for the world book's picture, 'shipped' for the default
 * that lives inside the program, and '' when the person has no picture at all —
 * in which case the window draws the placeholder initial rather than a blank
 * frame. `overridden` is what lets the editor offer "restore the default": it is
 * true only when an imported picture is the one answering.
 */
export interface CharacterPictureState {
  caption?: string
  url?: string
  source: 'chosen' | 'external' | 'lore' | 'shipped' | 'story' | ''
  overridden: boolean
}

/** One imported picture: the entry that now stands, and where to fetch it. */
export interface CharacterPictureImportResult {
  character_id: string
  asset: {
    id: string
    world?: string
    subject?: string
    source_key?: string
    scope?: string
    actor?: string
    purpose?: string
    file: string
    sha256?: string
    bytes?: number
    width?: number
    height?: number
  }
  url?: string
  picture: CharacterPictureState
}

export interface CharacterWindowScene {
  node_id?: string
  title?: string
  location?: string
  world_time?: string
  present?: string[]
  /**
   * The registered place this scene stands in, when the story's own frozen
   * catalog has one the location text names. `location` is prose the story
   * wrote; this is the identity a picture may bind to, so it is what an
   * illustration control is handed. Absent means the location names no
   * registered place, and then nothing can be attached to it.
   */
  place?: string
  /**
   * The place's own picture, when the author supplied one for this world and
   * this registered place. Absent for a place with no picture, and the window
   * then shows the place as text alone rather than a blank frame. A picture
   * existing does not mean the player has been told anything about the place:
   * it only illustrates where the story already put them.
   */
  image?: string
}

/**
 * One place's picture as the window would draw it.
 *
 * It is narrower than a person's picture on purpose: a place has no chosen
 * avatar, no world-book picture and no shipped default, so the only question is
 * whether an external picture answers. An empty `url` means the place is drawn
 * as text alone, which is what it already does.
 */
export interface PlacePictureState {
  url?: string
  overridden: boolean
  /**
   * The registered place this picture belongs to, in the catalog's own spelling.
   * Absent means the name asked about resolves to no registered place, so no
   * picture can be attached to it.
   */
  place?: string
}

/** One imported place picture: the entry that now stands, and where to fetch it. */
export interface PlacePictureImportResult {
  place: string
  world?: string
  asset: {
    id: string
    world?: string
    subject?: string
    place?: string
    purpose?: string
    file: string
    sha256?: string
    bytes?: number
    width?: number
    height?: number
  }
  url?: string
  picture: PlacePictureState
}

/** One line in a character conversation, as committed to the journal. */
export interface CharacterExchange {
  id: string
  story_id: string
  branch_id: string
  character_id: string
  speaker: 'player' | 'character'
  channel: 'scene' | 'private' | 'message' | 'group'
  text: string
  world_time: string
  turn_id?: string
  scene_node?: string
  audience?: string[]
  source_event_id?: string
  /** Set on a group line: the conversation space it was spoken in. */
  group_id?: string
  /**
   * Set on a group line: the person who actually said it. The backend stamps
   * this from the membership, so it is not the reader and cannot be chosen by a
   * model.
   */
  author_id?: string
}

/** One unresolved thread with a character: a permission, a promise, a debt. */
export interface CharacterMatter {
  id: string
  character_id: string
  kind: string
  status: string
  statement: string
  counterparty?: string
  deadline?: string
  world_time: string
  turn_id?: string
  settled_by?: string
  letter?: {
    body: string
    sent_at_seconds: number
    earliest_delivery_seconds: number
  }
  letter_reply?: {
    body: string
  }
}

/**
 * One incoming message a character sent on their own initiative, as the window
 * and the stage's hint both present it. It carries the original send time and
 * the reason they wrote, so an old message still reads as old rather than as
 * today's news.
 */
export interface CharacterWindowMessage {
  id: string
  character_id: string
  name: string
  text: string
  /** Why they wrote. A message without one would be contact for no reason. */
  basis: string
  world_time: string
  deadline?: string
  status: string
  /** Whether the message's deadline has passed at the story's current time. */
  lapsed: boolean
}

/** The whole character window read: the scene, the cast, and one conversation. */
/** The protagonist's latest known standing, from committed membership history. */
export interface FactionMembership {
  id: string
  faction_id: string
  faction_name: string
  status: 'pending' | 'formal' | 'temporary' | 'left'
  role?: string
  statement?: string
  authority_id?: string
  source_exchange_id?: string
  world_time?: string
  clock_seconds?: number
  turn_id?: string
  source_event_id?: string
  source_kind?: 'identity' | 'scene' | 'player'
}

export interface CharacterWindow {
  faction_memberships?: FactionMembership[]
  story_id: string
  branch_id: string
  clock_seconds?: number
  communication_mode?: string
  radio_available?: boolean
  /**
   * What this story settled for the protagonist's own gender. Empty means it
   * never settled one, so the surface offers no intimacy view rather than one
   * that would refuse every invitation.
   */
  protagonist_gender?: string
  scene: CharacterWindowScene
  characters: CharacterWindowCharacter[]
  character_id?: string
  exchanges?: CharacterExchange[]
  matters?: CharacterMatter[]
  messages?: CharacterWindowMessage[]
  /** Every message the player has not dealt with, across the whole cast. */
  pending?: CharacterWindowMessage[]
  has_more: boolean
  cursor?: string
}

export interface CharacterChatResult {
  story_id: string
  branch_id: string
  character_id: string
  reply: string
  world_time: string
  exchange_ids: string[]
  /** The character's feeling after this exchange. */
  affinity: number
  affinity_band: string
  /** How far this exchange moved it. 0 when nothing was settled. */
  affinity_delta: number
  /** The formal bond now standing, if any. */
  relationship?: CharacterRelationship
}

/** One person on a group's roster, as the group window shows them. */
export interface GroupWindowMember {
  id: string
  name: string
  avatar?: string
  /** The world book's own picture for this member, relative to the project. */
  avatar_lore_path?: string
  role?: string
  /** What this member was authored as; empty means the story never settled it. */
  gender?: string
  /** Whether they stand in the scene the player is reading. */
  present: boolean
  /**
   * Whether this member can hear the group right now. A face-to-face group only
   * reaches people who are present, so a member can be in the group and out of
   * earshot at the same time.
   */
  reachable: boolean
}

/** One group the player has on this branch. */
export interface GroupWindowGroup {
  id: string
  name: string
  /** 'self' | 'party' | 'sect' | 'other'. It is a purpose, not an authority. */
  type: string
  channel: string
  /** How the group reaches its members, in player-facing terms. */
  contact: string
  members: GroupWindowMember[]
  reachable_members: number
  last_exchange?: string
  last_world_time?: string
  /** Where that line sits in the branch journal, counting from one. */
  last_sequence?: number
  version: number
}

/**
 * The whole group surface: the scene, every group on this branch, and one
 * group's conversation. Reading it costs no world time and runs no model.
 */
export interface GroupWindow {
  story_id: string
  branch_id: string
  scene: CharacterWindowScene
  groups: GroupWindowGroup[]
  /** What this story settled for the protagonist's own gender; see CharacterWindow. */
  protagonist_gender?: string
  /** The story's known people, so the create form offers real characters. */
  cast?: CharacterWindowCharacter[]
  group_id?: string
  exchanges?: CharacterExchange[]
  /** Every author in the loaded page, including people who have since left. */
  participants?: Record<string, string>
  matters?: CharacterMatter[]
  has_more: boolean
  cursor?: string
}

/** One committed group round: the player's line and whoever answered. */
export interface GroupRoundResult {
  story_id: string
  branch_id: string
  group_id: string
  lines: CharacterExchange[]
  world_time: string
  /** Members who were in the group but could not hear this round. */
  silenced?: string[]
}

/** Where one intimacy activity stands. It is the design's own phase set. */
export type IntimacyPhase = 'selecting' | 'inviting' | 'ready' | 'active' | 'ended'

/**
 * One invited person's actual answer. A technical failure is never one of
 * these: a timeout is reported as an error, not as a refusal.
 */
export type IntimacyResponseStatus = 'pending' | 'accepted' | 'declined' | 'conditional'

/** One person on an activity, as they answered for themselves. */
export interface IntimacyParticipant {
  character_id: string
  status: IntimacyResponseStatus
  /** A conditional answer's condition, kept verbatim so the player can answer it. */
  condition?: string
  /** Their own authored data established an adult age when they were invited. */
  adult: boolean
  responded_world_time?: string
}

/** One activity's durable record, as the journal holds it. */
export interface IntimacyActivity {
  id: string
  story_id: string
  branch_id: string
  phase: IntimacyPhase
  participants?: IntimacyParticipant[]
  group_id?: string
  version: number
  scene?: CharacterWindowScene
  opened_world_time?: string
  ended_world_time?: string
  source_event_id?: string
}

/** One activity as the window lists it, with the names and roster it shows. */
export interface IntimacyWindowActivity extends IntimacyActivity {
  /** Each invited person as the player knows them, so the roster shows a real name. */
  participant_names?: Record<string, string>
  /**
   * The roster the window shows: the confirmed roster while running, and the
   * consented roster while waiting for the player's confirmation. A pending or
   * declined person is never in it.
   */
  roster?: string[]
}

/**
 * One person's outstanding invitation into a running activity: their answer so
 * far and what was said between them and the protagonist. It is listed apart
 * from the room because they have not agreed to join it yet.
 */
export interface IntimacyInvitationThread {
  character_id: string
  name?: string
  status: IntimacyResponseStatus
  condition?: string
  exchanges?: CharacterExchange[]
}

/**
 * What the window may show about the dedicated model. It never carries a key:
 * the credentials stay in the existing local configuration.
 */
export interface IntimacyModelState {
  configured: boolean
  profile_id?: string
  model?: string
  provider?: string
  thinking_level?: string
  /** Why an unconfigured model is unconfigured, in the player's words. */
  reason?: string
}

/**
 * The whole intimacy surface: the scene, this branch's activities, one
 * activity's conversation and the dedicated model's display state. Reading it
 * costs no world time and runs no model.
 */
export interface IntimacyWindow {
  story_id: string
  branch_id: string
  scene: CharacterWindowScene
  /**
   * What this story settled for the protagonist's own gender. Empty means it
   * never settled one, and every invitation below would be refused, so the
   * surface does not offer the view at all.
   */
  protagonist_gender?: string
  /** Every activity this branch holds, oldest first, so a finished one stays readable. */
  activities?: IntimacyWindowActivity[]
  activity_id?: string
  /** The activity's own conversation, oldest first. */
  exchanges?: CharacterExchange[]
  /** Invitation threads of people asked into a running activity; not the room's conversation. */
  invitations?: IntimacyInvitationThread[]
  /** Every author in the loaded page, including people who declined or left. */
  participants?: Record<string, string>
  /** The story's known people, so the invitation form offers real characters. */
  cast?: CharacterWindowCharacter[]
  /** Who may be invited right now: present, adult by their own data, and known. */
  eligible?: string[]
  /**
   * The reviewed private clips of the people actually in the open activity, by
   * character id. Present only while the activity is running and only for people
   * whose own answer put them in the room, so the clip appears under exactly the
   * conditions the activity already required. Everywhere else it is absent and
   * the still picture stays.
   */
  roster_clips?: Record<string, CharacterMediaClip[]>
  model: IntimacyModelState
}

/** One committed answer: the activity after the change, plus what the person said. */
export interface IntimacyResponse {
  story_id: string
  branch_id: string
  activity_id: string
  activity: IntimacyActivity
  reply?: string
  exchange_ids?: string[]
}

/** One committed exchange round in an open activity. */
export interface IntimacyRound {
  story_id: string
  branch_id: string
  activity_id: string
  lines: CharacterExchange[]
  world_time: string
}

/**
 * One technique a combatant has actually learned, at the version they learned.
 * The cost is the rules' own price at that combatant's stage, and castable
 * reports whether they could perform it right now — the window never decides
 * that for itself.
 */
export type QualityGrade = 'white' | 'green' | 'blue' | 'purple' | 'orange' | 'red'

/** Player-safe, locale-resolved definition prose. Instance facts stay separate. */
export interface GameEntryMetadata {
  /** Optional presentation-only art; never affects grade, eligibility or settlement. */
  art?: { source: 'preset' | 'generated' | 'catalog'; catalog_id?: string; preset_id?: string; image_path?: string; project_id?: string }
  quality_grade?: QualityGrade
  description?: string
  background?: string
  instance_description?: string
  source?: string
}

export interface CombatWindowSkill extends GameEntryMetadata {
  id: string
  version: number
  /** A localisation key; the interface supplies the wording. */
  name_key?: string
  /**
   * The technique's own wording, for a definition this story generated. A
   * generated technique has no localisation key, so its name is the only thing
   * there is to show.
   */
  name?: string
  power?: number
  mana_cost?: number
  burden_cost?: number
  castable: boolean
  /** The single reason this cannot be performed now, when it cannot. */
  reason_key?: string
  /**
   * How many opponents the technique reaches. §9 settles a multi-target
   * technique from the player's actual selection up to this cap, never by
   * defaulting to the whole enemy line, so the window needs the number to know
   * how many further choices to offer.
   */
  targets?: number
  /**
   * The published alternative forms of this technique. §5 fixes exactly two and
   * attaches each to one base technique, and §9's scope requires them to be
   * castable — a form the rules settle but the window never offers is a
   * capability the player cannot reach.
   */
  variants?: CombatWindowVariant[]
  /**
   * What the technique does beyond its power and cost, as the rules record it.
   * The window words a sentence from these properties rather than keeping a
   * description of its own, so a balance revision cannot leave an old sentence
   * behind claiming numbers the engine no longer uses.
   */
  effects?: CombatWindowSkillEffects
}

/** What one technique does beyond its power and cost. */
export interface CombatWindowSkillEffects {
  /** 1 completes this exchange; 2 prepares first and completes next. */
  execution_actions?: number
  /** The fraction of the target's defense the technique legally bypasses. */
  ignore_defense?: number
  /** The technique needs its implement in hand. */
  requires_focus?: boolean
  /** Shield size as a fraction of the caster's health maximum. */
  shield_fraction?: number
  /** How many exchanges a lasting effect survives. */
  duration_exchanges?: number
  /** The damage element the rules record. */
  element?: string
  /** The technique leaves a meridian residual injury. */
  leaves_residual?: boolean
}

/** One published alternative form of a technique. */
export interface CombatWindowVariant {
  /** The value a technique action carries as `variant`. */
  id: string
  name_key: string
  /** The power the form casts at. The cost is the base technique's, unchanged. */
  power: number
  /**
   * The engine's own identifier for the lasting effect a real hit applies, so
   * the window words it from its locale. A form the engine stops applying
   * reports nothing and cannot keep an old sentence here.
   */
  effect?: string
}

/**
 * One combatant as the battle window shows them.
 *
 * The pools are optional because a value the rules never settled is omitted
 * rather than sent as a zero the player would read as fact. A battle fought at
 * numbers the rules have not established has nothing to report yet.
 *
 * `exact` is the audience boundary: the protagonist's own side is reported with
 * its real figures, and everyone else is reported only through `observed` —
 * what the fight actually put in front of the player. An opponent's realm,
 * pools, attributes, learned set and carried stock are not sent at all, so the
 * browser never holds a value it is only meant to hide.
 */
export interface CombatWindowSide {
  actor_id: string
  name: string
  side: 'player' | 'enemy'
  /** Whether these are the actor's real figures or the player's observation. */
  exact: boolean
  /** The stage key the rules place this combatant at. Player side only. */
  realm?: string
  health?: number
  health_max?: number
  mana?: number
  mana_max?: number
  attack?: number
  defense?: number
  agility?: number
  perception?: number
  burden?: number
  burden_floor?: number
  shield?: number
  burn_ticks?: number
  slow_actions?: number
  skills?: CombatWindowSkill[]
  /**
   * The consumable stock this combatant really holds, priced from the rules'
   * catalogue. Two copies of one pill are two entries, because §7 keeps a
   * definition apart from the copies made from it.
   */
  items?: CombatWindowItem[]
  /**
   * What really occupies each of the five positions. Every position is present,
   * an empty one included, so the sheet shows all five rows the rules allow
   * rather than only the ones that happen to be filled.
   */
  equipment?: CombatWindowEquipment[]
  /**
   * The pack copies a mid-fight change could take up. A spare adds nothing while
   * it sits in the pack, so nothing here is priced.
   */
  spares?: CombatWindowSpare[]
  has_implement: boolean
  /** How the combatant currently stands. */
  standing: string
  /** An outstanding offer to yield the other side has not answered. */
  offer_text?: string
  /** The paid technique awaiting an explicit next-round continue or cancel. */
  preparation?: {
    skill_id: string
    name_key?: string
    /** The technique's own wording, for a story-generated definition. */
    name?: string
    target_id?: string
    started_round: number
  }
  retreat?: string
  route_exists: boolean
  /** What the player has seen of an opponent. Present only for the other side. */
  observed?: CombatWindowObservation
}

/**
 * What the player has actually seen of an opponent.
 *
 * The condition is a coarse visible band rather than a number, and the
 * techniques are only the ones this fight really put on show. An unrecognised
 * technique carries no identifier of any kind: a skill_id is the definition's
 * name in another form, so sending it would name what the player cannot name.
 */
export interface CombatWindowObservation {
  /** An i18n key: combatWindow.condition.* */
  condition: string
  techniques?: CombatWindowTechnique[]
  /** How many doses the player watched this opponent take. Not a stock count. */
  doses?: number
  /**
   * A lasting effect plainly in sight. Presence only: the size of the shield and
   * how many exchanges it has left are the opponent's own figures and the server
   * does not send them.
   */
  shield?: boolean
  burn_ticks?: boolean
  slow_actions?: boolean
}

export interface CombatWindowTechnique {
  /** The round it was first seen in. */
  round: number
  recognized: boolean
  /** Present only when the player could name it. */
  skill_id?: string
  /** Present only when the player could name it. */
  name_key?: string
  /** The technique's own wording, for a story-generated definition. */
  name?: string
}

/**
 * One of the five positions the rules give a character.
 *
 * The slot is the engine's own name rather than the story's wording, so the
 * interface supplies the label from `combatWindow.slot.*`. An empty position
 * carries no instance, definition or name: absent is not an item called nothing.
 */
export interface CombatWindowEquipment extends GameEntryMetadata {
  /** weapon | garment | attack_treasure | defense_treasure | support_treasure */
  slot: string
  instance_id?: string
  def_id?: string
  name?: string
  /** What driving this copy costs for one exchange, priced from its definition. */
  operating_mana_cost?: number
}

/**
 * One pack copy a mid-fight change could take up.
 *
 * `slot` is read from the copy's registered definition, not chosen by the
 * client: §4 maps an item's real use to its row, so a blade is never offered for
 * a treasure position. Nothing is priced — a spare in the pack adds nothing.
 */
export interface CombatWindowSpare extends GameEntryMetadata {
  instance_id: string
  def_id?: string
  name?: string
  slot: string
}

/**
 * One carried consumable as the window offers it.
 *
 * The numbers are the catalogue's rather than the story's: §4.2 fixes a
 * restoring pill's amount to the stage it was brewed at and a boosting pill's
 * bonus to its grade, so the window shows what the rules will actually pay.
 * `instance_id` names the real copy and is what a use carries, so two copies of
 * one pill stay two separate stocks all the way to the wire.
 */
export interface CombatWindowItem extends GameEntryMetadata {
  instance_id: string
  def_id: string
  /** The definition's localisation key; the wording lives in this interface. */
  name_key?: string
  quantity: number
  /** The rules' own purpose: restore_health, restore_mana or boost_attack. */
  purpose: string
  grade?: string
  rarity?: string
  /** The absolute amount one dose restores. Absent for a boosting pill. */
  restore_amount?: number
  shield_amount?: number
  /** A boosting pill's bonus, as a fraction. */
  attack_bonus?: number
  duration_exchanges?: number
  /**
   * Whether this copy could be taken right now, and the one reason it could not
   * when it cannot. §8 requires a refused use to cost nothing, so a blocked copy
   * is shown with its reason rather than hidden.
   */
  usable: boolean
  reason_key?: string
}

/** A cast's precondition check. A refused cast spent nothing. */
export interface CombatCastCheck {
  allowed: boolean
  reason_key?: string
  mana_cost?: number
  burden_cost?: number
}

/** The settled attack of one technique that resolved. */
export interface CombatAttackResolution {
  hit: boolean
  damage?: number
  shield_absorbed?: number
  health_lost?: number
}

/** One further combatant a multi-target technique struck, and its own check. */
export interface CombatExtraTargetStrike {
  target_id: string
  resolution: CombatAttackResolution
  burn_applied?: boolean
  slow_applied?: boolean
  defeated?: boolean
}

/** One combatant's action within an exchange. */
export interface CombatExchangeStep {
  actor_id: string
  target_id?: string
  kind: string
  /**
   * The technique this step performed, at the definition's stable ID. A refused
   * cast still names it, so a round says what was attempted and not only what
   * came of it. The wording lives in the interface's own locale file.
   */
  skill_id?: string
  /**
   * The published form the technique was cast as, if any. §5 makes a variant a
   * way of casting rather than a second technique, so the step keeps the base
   * technique in `skill_id` and the form here.
   */
  variant?: string
  /** An alternate selected action abandoned an already-paid preparation. */
  preparation_abandoned?: boolean
  cast: CombatCastCheck
  resolution?: CombatAttackResolution
  /**
   * The further combatants a multi-target technique was aimed at, each with its
   * own check. §9 settles them from the player's actual selection, so a target
   * the player did not choose is never among them, and a miss on the primary
   * does not cancel them.
   */
  extra_targets?: CombatExtraTargetStrike[]
  mana_recovered?: number
  /**
   * What a restoring dose restored, reported apart from a rest because the two
   * are different acts that move the same pool.
   */
  health_recovered?: number
  /** True when a dose took effect. A refused one leaves this false and spends nothing. */
  item_applied?: boolean
  burn_applied?: boolean
  slow_applied?: boolean
  escaped?: boolean
  surrendered?: boolean
  defeated?: boolean
  /** Why nothing happened, when nothing did. */
  reason_key?: string
}

/** One exchange's settled result, as the rules committed it. */
export interface CombatExchangeOutcome {
  round: number
  /** The resolved action order, so the prose follows what actually happened. */
  order?: string[]
  steps?: CombatExchangeStep[]
  ended?: boolean
  result?: string
}

/** Durable provenance. Absent on historical records with unknown source. */
export interface EnemyDecisionReceipt {
  v: 1
  status: 'ai' | 'ai_partial_fallback' | 'fallback_unavailable' | 'fallback_context_limit' | 'fallback_invalid' | 'rules' | 'unknown'
  ai_actors?: string[]
}

/** One committed exchange as the history lists it. */
export interface CombatWindowRound {
  enemy_decision?: EnemyDecisionReceipt
  event_id: string
  world_time?: string
  outcome: CombatExchangeOutcome
  fatalities?: CombatFatality[]
  /**
   * The prose written from this exchange. Absent when no passage was written,
   * which §8 allows: the round stands whether or not its expression did.
   */
  narration?: string
  /** The combat style the passage was written in. */
  narration_style_id?: string
  /** The committed narration record, so a missing passage is distinguishable. */
  narration_event_id?: string
}

export interface CombatFatality {
  actor_id: string
  killer_actor_id: string
  source_turn_id: string
  source_event_id: string
  round: number
  step_index: number
}

export interface CombatWindowScene {
  node_id?: string
  title?: string
  location?: string
  world_time?: string
}

/**
 * The whole battle window read: the scene, every live battle on the branch, and
 * one battle with both sides and its committed rounds.
 *
 * With no session_id this reports the scene and the list of live battles, which
 * is what the entry point needs before the player picks one.
 */
export interface CombatWindow {
  story_id: string
  branch_id: string
  scene: CombatWindowScene
  /**
   * The fight the scene already settles, reported on the entry read.
   *
   * The scene states who is present and which actors are hostile by
   * construction, so the panel opens from this rather than asking the player to
   * side the whole cast. Absent once a battle is open, because then sides is the
   * fight.
   */
  encounter?: CombatWindowEncounter
  sessions?: string[]
  session_id?: string
  /** The exchange about to be resolved, 1-based. */
  round?: number
  ended: boolean
  result?: string
  lethal_risk?: boolean
  /**
   * The actor whose actions the player composes. The rules choose it rather
   * than the interface guessing from the side list, because a side can hold
   * more than one combatant.
   */
  player_actor_id?: string
  sides?: CombatWindowSide[]
  /** The exchange just settled, with its own prose. */
  last_round?: CombatWindowRound
  history?: CombatWindowRound[]
  has_more: boolean
  cursor?: string
  /**
   * Set when the last round's prose could not be written. The round itself is
   * committed and stands; only its description failed, and the panel offers to
   * write it rather than replaying the round.
   */
  narration_error?: string
  /**
   * Which of the two missing-prose cases this is. `pending` means the round is
   * committed and no passage was attempted against this read — a process that
   * stopped between settlement and prose leaves exactly that. `failed` means an
   * attempt was made and produced nothing, so there is a reason to show.
   */
  narration_state?: 'pending' | 'failed'
}

/** One committed passage of battle prose. */
export interface CombatNarration {
  story_id: string
  branch_id: string
  session_id: string
  round_event_id: string
  round: number
  narrative: string
  style_id?: string
  event_id?: string
  /** True when this came from a passage already committed under the same command. */
  replayed: boolean
}

/** One participant named when opening a battle. */
export interface CombatantSpec {
  actor_id: string
  side: 'player' | 'enemy'
  route_exists?: boolean
  retreat?: string
}

/** One participant of the encounter the scene settles. */
export interface CombatWindowCombatant {
  actor_id: string
  name?: string
  side: 'player' | 'enemy'
  /**
   * Whether this combatant really has a way out of the fight.
   *
   * The scene settles it, so the panel sends it back when it opens the battle.
   * Reported for the player's own side only: whether an opponent can be run from
   * is not the player's to know in advance.
   */
  route_exists?: boolean
  /** The player's own evaluation template, reported for the same reason. */
  retreat?: string
}

/**
 * The fight the scene already settles, as the entry read reports it.
 *
 * `reason_key` is a localisation key naming why no battle can be opened here:
 * nobody hostile is present, presence itself is unsettled, or there is no
 * rules-tracked protagonist to fight as.
 */
export interface CombatWindowEncounter {
  combatants?: CombatWindowCombatant[]
  /** Present people the scene does not put in the fight. */
  bystanders?: string[]
  reason_key?: string
}

/** One declared intent for the round, by actor. */
export interface CombatActionRequest {
  actor_id: string
  kind: 'technique' | 'rest' | 'escape' | 'surrender' | 'accept_surrender' | 'use_item' | 'swap_equipment' | 'continue_preparation' | 'cancel_preparation'
  skill_id?: string
  variant?: string
  target_id?: string
  /**
   * The further opponents a technique that reaches more than one target is
   * aimed at. §9 settles the reach from the actual choice, so these are the
   * player's own picks and never an inference from who is standing.
   */
  target_ids?: string[]
  offer_text?: string
  /**
   * Which carried copy a use_item action spends. It is an instance and not a
   * definition, and the effect is deliberately not sent: the rules price the
   * dose from the catalogue, so a client cannot hand itself a stronger pill.
   */
  item_instance_id?: string
  /**
   * Which pack copy a swap_equipment action takes up. Only the copy is sent: the
   * position is read by the server from the copy's registered definition, so a
   * client cannot put a blade in a treasure row by asking to.
   */
  swap_instance_id?: string
}

export type PlayerCombatIntent = 'spare' | 'take_life'

export interface ActorStateSchemaSnapshot {
  version: number
	revision: number
  system: StoryDirectorActorStateSystem
	trpg_system?: StoryDirectorTRPGSystem
	adaptation?: ActorStateSchemaAdaptationRecord
  legacy_field_paths?: Record<string, Record<string, string>>
  legacy_actor_templates?: Record<string, string>
}

export interface ActorStateSchemaAdaptationRecord {
	source: string
	summary?: string
	source_turn_id?: string
	lore_revision?: string
	template_ops?: number
	field_ops?: number
	initial_actor_ops?: number
	actor_ops?: number
	reviewed_lore_ids?: string[]
	requirements?: ActorStateSchemaRequirementReview[]
	changes?: ActorStateSchemaAdaptationChange[]
	warnings?: string[]
}

export interface ActorStateSchemaRequirementSource {
	kind: 'lore' | 'opening' | 'turn_result' | 'trpg' | string
	id: string
}

export interface ActorStateSchemaRequirementReview {
	source: ActorStateSchemaRequirementSource
	requirement: string
	value_policy?: 'schema_only' | 'preserve' | 'initialize' | 'defer' | string
	actor_id?: string
	expected_type?: string
	min?: number
	max?: number
	decision: 'covered' | 'add' | 'replace' | 'remove' | 'ignored' | string
	template_id?: string
	field_id?: string
	reason?: string
}

export interface ActorStateSchemaAdaptationChange {
	kind: 'template' | 'field' | 'actor' | 'actor_field' | string
	op: 'add' | 'replace' | 'remove' | 'set' | string
	template_id?: string
	field_id?: string
	target_id?: string
	actor_id?: string
	reason?: string
	value_source?: {
		source_id: string
		item_id: string
		source: ActorStateSchemaRequirementSource
	}
}

export interface StateSchemaInitializationStatus {
  mode: StoryStateSchemaMode
  status: 'waiting_opening' | 'ready'
  outcome?: 'changed' | 'unchanged' | 'fixed'
  source_turn_id?: string
  base_revision?: number
  target_revision?: number
  summary?: string
  lore_revision?: string
  reviewed_lore_ids?: string[]
  requirements?: ActorStateSchemaRequirementReview[]
  changes?: ActorStateSchemaAdaptationChange[]
  warnings?: string[]
  started_at?: string
  completed_at?: string
  updated_at?: string
}

// Read-only projection of the public Agent checkpoint bound to this branch.
// Story persistence never owns or mutates this state.
export interface ContextCompactionProjection {
  id: string
  branch_id: string
  revision: number
  summary: string
  tokens_before: number
  tokens_after: number
  source_message_count: number
}

export interface BranchSummary {
  id: string
  head: string
  from?: string
  from_event?: string
  title?: string
  created_at: string
  current: boolean
}

export interface PlotNode {
  id: string
  parent_id?: string
  branch_id: string
  title: string
  summary: string
  ts: string
  current: boolean
  head: boolean
  terminal?: boolean
  terminal_type?: string
}

export interface StoryGraph {
  nodes: PlotNode[]
  branches: BranchSummary[]
}

export interface InteractiveTurnPersistedEvent {
  /** Transport replay restores history without automatic playback. */
  replayed?: boolean
  story_id: string
  branch_id: string
  turn_count: number
  turn: TurnEvent
  branch_plan?: BranchPlan
  state: Record<string, unknown>
  graph: StoryGraph
  branches: BranchSummary[]
  context_compaction: ContextCompactionProjection | null
  clock?: StoryClock
}

export type InteractiveSSEEvent = SSEEvent
