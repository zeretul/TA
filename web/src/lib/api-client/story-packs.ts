import type { ActorStateField, WorldPresentationOverrides } from '@/features/interactive/types'
import { jsonHeaders, requestJSON } from './client'
import { projectAPIPath } from './project-scope'
import type { StoryCheckSettings, StoryDirectorModuleRefs, StoryOpeningConfig, StoryPackBinding, StoryProtagonist, StoryStylePurposes, StorySummary } from '../../features/interactive/types'

/** One director module a pack binds, as the catalog resolved it. */
export interface StoryPackModule {
  kind: 'event_package' | 'rule_system' | 'actor_state' | 'image_preset'
  id: string
  name?: string
  version?: number
  missing?: boolean
  disabled?: boolean
}

/** One game-only script pack (剧本包) as the catalog lists it. */
export interface StoryPackSummary {
  id: string
  name: string
  description?: string
  tags?: string[]
  title: string
  protagonist?: string
  world_count: number
  public_world_count?: number
  character_count: number
  builtin: boolean
  invalid?: boolean
  error?: string
  /** The author's own content revision; a story stays on the one it started from. */
  content_version?: number
  modules?: StoryPackModule[]
  /** Bound modules this machine does not have installed. */
  missing?: number
  /** Ready-made characters this pack offers, so a start needs no written sheet. */
  protagonist_templates?: StoryPackTemplateSummary[]
  /** The creation-page options, when this pack offers a creation page. */
  creation?: StoryPackCreation
  /** The resolved new-game defaults, with the application's fallbacks filled in. */
  settings?: StoryPackSettings
  /** The saved-defaults revision, sent back when saving. */
  settings_revision?: string
  /** Whether the player has saved defaults for this world. */
  settings_customized?: boolean
  /**
   * The planning template the world's author suggests. It lives outside
   * `settings` on purpose: a save replaces that whole block, so a suggestion
   * stored there would vanish as soon as the player tuned anything, and the
   * page has to keep showing what the world recommends after the player picks
   * something else.
   */
  recommended_planning_template_id?: string
}

/**
 * One ready-made character, as the pack list projects it.
 *
 * This is only the choice — id, name and the one-line pitch. The profile, the
 * opening and the initial state stay in the pack file and are resolved on the
 * server, so a browser cannot hand the game the numbers a character is worth.
 */
export interface StoryPackTemplateSummary {
  id: string
  name: string
  description?: string
}

/** One offered option on the creation page. */
export interface StoryPackCreationOption {
  id: string
  name: string
  description?: string
  default?: boolean
  country?: string
  kind?: string
  faction_id?: string
}

/** One social origin, with the initial state a start from it really commits. */
export interface StoryPackCreationOrigin extends StoryPackCreationOption {
  opening_tier?: 'ordinary' | 'strong'
  template_id?: string
  default_template_id?: string
  default_location_id?: string
  /**
   * The kind of affiliation this origin can belong to: `none` for a loose
   * cultivator, `sect` for a sect disciple, `family` for a family heir. Absent
   * means the origin does not constrain the affiliation, which is what a
   * custom origin needs. The server enforces the same rule.
   */
  affiliation_kind?: string
  initial_state?: {
    authored_fields?: { name: string; type: string; value: unknown }[]
    /** Adventure values are projected from the same authored actor as the new story. */
    family?: 'wuxia' | 'space' | 'fantasy'
    rank?: number
    currency_label?: string
    realm?: string
    realm_label?: string
    currency: number
    items?: string[]
    techniques?: string[]
    equipped?: string[]
  }
}

/** One real cultivation aptitude tier. Only this group has a rule effect. */
export interface StoryPackCreationAptitude extends StoryPackCreationOption {
  talent: string
}

/**
 * One starting place. `kind` says whether it is an open region or a faction's
 * outer gate; the gate's id names its faction, which is how the page knows a
 * sect disciple cannot be introduced at a different sect's door.
 */
export type StoryPackCreationLocation = StoryPackCreationOption

/**
 * The pack's authored creation-page options.
 *
 * Every option is an id and a label. The values behind them — the starting
 * realm, the purse, the carried things, the aptitude token — are resolved from
 * the same block on the server, so the page cannot offer a choice the start
 * does not honour.
 */
export interface StoryPackCreation {
  adventure_classes?: StoryPackCreationOption[]
  adventure_weapons?: StoryPackCreationOption[]
  adventure_methods?: StoryPackCreationOption[]
  age_range: [number, number]
  genders?: StoryPackCreationOption[]
  origins?: StoryPackCreationOrigin[]
  state_basis_templates?: string[]
  visible_origin_ids?: string[]
  custom_state_basis_templates?: string[]
  custom_location_policy?: 'authored_only'
  custom_background_rule?: string
  affiliations?: StoryPackCreationOption[]
  locations?: StoryPackCreationOption[]
  root_categories?: StoryPackCreationOption[]
  root_attributes?: StoryPackCreationOption[]
  /**
   * The mutated roots (风／雷／冰). A character has one of these or an ordinary
   * root, never both; the server rejects a request that carries both.
   */
  variant_roots?: StoryPackCreationOption[]
  aptitudes?: StoryPackCreationAptitude[]
  weapon_directions?: StoryPackCreationOption[]
  /** The six temperaments. Narrative only: they never move a number. */
  traits?: StoryPackCreationOption[]
  /**
   * The optional 天赋特性: at most one, and the only creation choice besides the
   * aptitude that moves a real attribute. The size is the rules', not the pack's.
   */
  talents?: StoryPackCreationOption[]
  /** Per group: real_cultivation_talent, narrative or content_tendency. */
  effect_notes?: Record<string, string>
}

/** The story-creation request a pack contributes, in the original request shape. */
export interface StoryPackRequest {
  title: string
  origin: string
  protagonist: StoryProtagonist
  module_refs: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  opening: StoryOpeningConfig
  planning_mode: string
  /** The style a new game of this world is told in. */
  story_teller_id?: string
  planning_template_id?: string
  style_purposes?: StoryStylePurposes
  check_settings?: StoryCheckSettings
  narrative_requirements?: string
  state_schema_policy: { mode: string }
  /** Frozen pack identity, content revision and lore scope for this story. */
  pack_binding?: StoryPackBinding
}

/**
 * A world's defaults for a new game, as the page edits them.
 *
 * These are defaults, not locks: a start copies them into the story, so a player
 * who changes their own game keeps that change and editing the world later
 * leaves a running story on what it started with.
 *
 * The check block is spelled out here rather than reused from a story's own
 * `StoryCheckSettings` because a pack may carry any stored string: the page
 * normalizes it before offering a choice, while a story's own block is already
 * one of the known modes.
 */
export interface StoryPackSettings {
  writing_preferences?: string
  rule_system_id?: string
  planning_mode?: string
  planning_template_id?: string
  style_purposes: StoryStylePurposes
  reply_target_chars?: number
  /** The world's default creative direction, seeded into each new story. */
  narrative_requirements?: string
  check_settings: {
    difficulty_shift?: number
    roll_modifier?: number
    rule_state_consumption_mode?: string
    rule_visibility_mode?: string
  }
}

/** The saved defaults and the revision to send back on the next save. */
export interface StoryPackSettingsSaveResult {
  settings: StoryPackSettings
  revision: string
}

/**
 * One pack in full, as the server resolves it.
 *
 * The world page fetches this when the player loads a world: it is the same
 * read the start path performs, so a pack whose content no longer parses or
 * whose modules are not installed fails here rather than at the first turn.
 */
export interface StoryPackDetail {
  version: number
  id: string
  name: string
  description?: string
  tags?: string[]
  content_version?: number
  settings?: StoryPackSettings
  protagonist_templates?: StoryPackTemplateSummary[]
  creation?: StoryPackCreation
  world?: unknown[]
  characters?: unknown[]
}

export interface StoryPackDraft {
  pack_id: string
  pack_name: string
  world_added: number
  world_exists: number
  request: StoryPackRequest
}

/** A world just started from a pack: its Project and its first story. */
export interface StoryPackStart {
  project_id: string
  workspace: string
  pack_id: string
  pack_name: string
  story: StorySummary
}

export async function listStoryPacks(): Promise<StoryPackSummary[]> {
  const data = await requestJSON<{ packs: StoryPackSummary[] }>('/api/story-packs')
  return data.packs || []
}

/**
 * Reads one pack in full.
 *
 * This is the "load" action of the world page. It resolves the pack and its
 * modules on the server but creates nothing: no Project, no story, no journal
 * entry, no time advance. The creation page and the start request do the rest
 * when the player explicitly asks for them.
 */
export async function getStoryPack(packId: string): Promise<StoryPackDetail> {
  return requestJSON(`/api/story-packs/${encodeURIComponent(packId)}`)
}

/**
 * Saves this world's defaults for future new games.
 *
 * `revision` is the one the page loaded, so a second window that saved first is
 * reported as a conflict instead of being overwritten. The author's pack file is
 * never touched; the defaults live beside it.
 */
export async function saveStoryPackSettings(
  packId: string,
  settings: StoryPackSettings,
  revision: string,
): Promise<StoryPackSettingsSaveResult> {
  return requestJSON(`/api/story-packs/${encodeURIComponent(packId)}/settings`, {
    method: 'PUT',
    headers: jsonHeaders,
    body: JSON.stringify({ settings, revision }),
  })
}

/**
 * Creates a Project for the pack and the story it starts, in one request.
 *
 * The pack binding is only accepted at story creation, and the protagonist and
 * modules resolve against the Project that is already open, so the two halves
 * are one operation rather than a create-then-repoint sequence.
 *
 * `templateId` picks one of the pack's ready-made characters. It is a choice
 * and only a choice: the character's profile, opening and initial state are
 * read from the pack file on the server, so this request cannot supply them.
 *
 * `identity` is the opposite path: a character the player created and confirmed
 * on the creation page. It is resolved from the pack again server-side, so it
 * carries choices and prose, never state.
 */
export async function startStoryPack(
  packId: string,
  templateId?: string,
  identity?: StoryPackIdentityChoice,
): Promise<StoryPackStart> {
  const body: Record<string, unknown> = {}
  if (templateId) body.template_id = templateId
  if (identity) body.identity = identity
  return requestJSON(`/api/story-packs/${encodeURIComponent(packId)}/start`, {
    method: 'POST',
    headers: jsonHeaders,
    body: Object.keys(body).length > 0 ? JSON.stringify(body) : undefined,
  })
}

/**
 * The choices a creation page sends. All ids and text: nothing numeric, no
 * realm, no purse, no carried thing. The server resolves them against the pack
 * again, so a modified request cannot make the character richer.
 */
export interface StoryPackIdentityChoice {
  adventure_class_id?: string
  adventure_weapon_id?: string
  adventure_method_id?: string
  name: string
  gender?: string
  /** The roll the page showed. Zero asks the server to roll one. */
  age?: number
  /** The one explicit way to ask for a new roll. */
  reroll_age?: boolean
  origin_id: string
  origin_text?: string
  /** The ready-made initial state a custom origin starts from. */
  state_basis_template_id?: string
  affiliation_id: string
  affiliation_text?: string
  location_text?: string
  opening_mode?: 'ai' | 'custom'
  location_id: string
  root_category_id: string
  root_attributes?: string[]
  /**
   * The mutated root (风／雷／冰) picked instead of an ordinary one. Sending it
   * with `root_attributes` is refused: a character has one root, not two.
   */
  variant_root_id?: string
  aptitude_id: string
  weapon_direction_id?: string
  trait_ids?: string[]
  /** At most one. Omitting it means the player took no 天赋特性. */
  talent_id?: string
  identity?: string
  appearance?: string
  background?: string
  situation?: string
  predicament?: string
  contacts?: string[]
  opening?: string
}

/** A confirmed character as the server resolved it. */
export interface StoryPackIdentity {
  adventure_class_id?: string
  adventure_class_name?: string
  adventure_weapon_id?: string
  adventure_weapon_name?: string
  adventure_method_id?: string
  adventure_method_name?: string
  name: string
  gender: string
  gender_name: string
  age: number
  origin_id: string
  origin_name: string
  origin_text: string
  template_id: string
  affiliation_id: string
  affiliation_name: string
  affiliation_country?: string
  location_id: string
  location_name: string
  location_country?: string
  root_category_id: string
  root_category_name: string
  root_attributes?: string[]
  /** The mutated root, when the player picked one instead of an ordinary root. */
  variant_root_id?: string
  variant_root_name?: string
  aptitude_id: string
  aptitude_name: string
  /** The value the cultivation rules really read. */
  talent: string
  weapon_direction_id?: string
  weapon_direction_name?: string
  trait_ids?: string[]
  trait_names?: string[]
  /** The optional 天赋特性. Its size is the rules', stated in `talent_bonus`. */
  talent_id?: string
  talent_name?: string
  talent_bonus?: string
  /** The composed 灵根与特殊体质 text, built server-side. */
  foundation: string
  identity_text?: string
  appearance?: string
  background?: string
  situation?: string
  predicament?: string
  contacts?: string[]
  opening?: string
  initial_state?: StoryPackCreationOrigin['initial_state']
  profile: string
  premise: string
}

/** One draft: the player's choices plus the prose the model filled in. */
export interface StoryPackIdentityDraft {
  identity: StoryPackIdentity
  contact_candidates?: string[]
  rewritten?: boolean
  /** Set when the model failed: the choices and rolled age are still valid. */
  error?: string
}

/**
 * Resolves the player's choices and has the model write the fields they left
 * blank. Nothing is created: the page may call it again to modify or rewrite.
 */
export async function draftStoryPackIdentity(
  packId: string,
  choice: StoryPackIdentityChoice,
  rewrite = false,
  signal?: AbortSignal,
): Promise<StoryPackIdentityDraft> {
  return requestJSON(`/api/story-packs/${encodeURIComponent(packId)}/identity-draft`, {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ choice, rewrite }),
    signal,
  })
}

/** One ready-made character in full, for showing what the choice actually is. */
export interface StoryPackTemplateDetail {
  id: string
  name: string
  description?: string
  character_name: string
  profile: string
  origin?: string
  opening_text?: string
}

export async function getStoryPackTemplate(packId: string, templateId: string): Promise<StoryPackTemplateDetail> {
  return requestJSON(
    `/api/story-packs/${encodeURIComponent(packId)}/templates/${encodeURIComponent(templateId)}`,
  )
}

/**
 * Writes the pack's world book into the Project and returns the story request.
 * The caller still submits that request through the ordinary creation path, so
 * a pack never becomes a second way to start a game.
 */
export async function applyStoryPack(projectId: string, packId: string): Promise<StoryPackDraft> {
  return requestJSON(projectAPIPath(projectId, `story-packs/${encodeURIComponent(packId)}/apply`), {
    method: 'POST',
    headers: jsonHeaders,
  })
}

/**
 * A world being created. This is the one document the guided and automatic
 * entry points both write into, and the one the player edits before saving.
 *
 * The prose fields are the player's. Nothing here is a rule: the module ids,
 * the aptitude tiers, the starting purse and the state a ready-made character
 * begins with are assembled by the server from a fixed, supported template, so
 * editing this draft cannot make a character start richer than a built-in
 * world's.
 */
export interface StoryPackWorldDraft {
  name: string
  description?: string
  tags?: string[]
  /** The story's own title, which may differ from the world's name. */
  title: string
  /** The premise a first scene opens on. */
  origin: string
  protagonist_name?: string
  protagonist?: string
  narrative_requirements?: string
  opening_text?: string
  world?: StoryPackWorldEntry[]
  characters?: StoryPackWorldCharacter[]
  topic_terms?: Record<string, string[]>
  origins?: StoryPackWorldOrigin[]
  affiliations?: StoryPackWorldOption[]
  locations?: StoryPackWorldOption[]
  age_range?: [number, number]
  /** The sentence stating which rules actually run. Set by the server. */
  notice?: string
  gameplay: StoryPackWorldGameplay
}

export interface StoryPackWorldGameplay {
  world_content?: Record<string, unknown>
  cultivation_rules?: Record<string, unknown>
  status_template: string
  numeric_scale: string
  growth: string
  combat: string
  communication: string
  letter_delivery_days?: number
  presentation?: WorldPresentationOverrides
  opening: string
  state_fields?: StoryPackWorldStateField[]
  /** Author-declared facts; initial values still come from the selected template. */
  authored_state_fields?: ActorStateField[]
  check_bindings?: StoryPackWorldCheckBinding[]
  confirmed: boolean
}

export interface StoryPackWorldStateField {
  name: string
  description?: string
  minimum: number
  maximum: number
  initial: number
}

export interface StoryPackWorldCheckBinding {
  action: string
  field: string
  scale: number
  offset: number
  minimum: number
  maximum: number
}

export interface StoryPackWorldEntry {
  audience?: 'public' | 'gm' | '' 
  name: string
  type?: string
  importance?: string
  tags?: string[]
  keywords?: string[]
  load_mode?: string
  brief_description?: string
  content: string
}

export interface StoryPackWorldCharacter {
  name: string
  aliases?: string[]
  country?: string
  faction?: string
  role?: string
  topics?: string[]
  place?: string
  place_label?: string
  brief?: string
  initial_stage?: string
  level?: string
}

/** One social starting position. `basis` is one the rules provide. */
export interface StoryPackWorldOrigin {
  name: string
  description?: string
  basis: 'wanderer' | 'disciple' | 'scion' | string
  technique?: string
  item?: string
}

/** One labelled choice on the creation page: an affiliation or a place. */
export interface StoryPackWorldOption {
  name: string
  description?: string
  country?: string
  default?: boolean
}

/**
 * What the page shows before saving. It is built from the pack the draft would
 * become, so the counts and `startable` are what a saved world would really
 * have — a draft too thin to start says why rather than offering a start that
 * would fail.
 */
export interface StoryPackWorldPreview {
  name?: string
  title?: string
  origin?: string
  world_count: number
  character_count: number
  origins?: string[]
  affiliations?: string[]
  locations?: string[]
  startable: boolean
  reasons?: string[]
  reason_keys?: string[]
  notice: string
}

/** One drafted world plus what the page needs to render it. */
export interface StoryPackWorldDraftResult {
  draft: StoryPackWorldDraft
  preview: StoryPackWorldPreview
  rewritten?: boolean
  /** Set when the model failed: the player's own edits are still in `draft`. */
  error?: string
  error_key?: string
}

/** The player's own material, kept apart from the draft a rewrite replaces. */
export interface StoryPackWorldSeed {
  mode?: 'guided' | 'automatic'
  name?: string
  genre?: string
  brief?: string
  tone?: string
  protagonist?: string
  opening_idea?: string
  gameplay?: StoryPackWorldGameplay
}

/**
 * Has the model draft a world's premise, places and people. Nothing is created:
 * the page may call it again to edit, switch entry point, or regenerate.
 */
export async function draftStoryPackWorld(
  seed: StoryPackWorldSeed,
  draft?: StoryPackWorldDraft,
  rewrite = false,
): Promise<StoryPackWorldDraftResult> {
  return requestJSON('/api/story-packs/world-draft', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ ...seed, draft, rewrite }),
  })
}

/**
 * Re-checks an edited draft without saving it and without a model call.
 *
 * The preview page lets the player rewrite prose between drafting and saving.
 * That edit must be judged by the same rules a save applies, or the page would
 * keep offering a start that the save then refuses. Nothing is written.
 */
export async function previewStoryPackWorld(draft: StoryPackWorldDraft): Promise<StoryPackWorldPreview> {
  return requestJSON('/api/story-packs/world-preview', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(draft),
  })
}

/** What a save wrote. */
export interface StoryPackWorldSaveResult {
  pack_id: string
  /** The pack as the world list will now show it. */
  summary: StoryPackSummary
}

/**
 * Writes an edited draft as a new world. Saving is create-only and never
 * replaces an existing world, so pressing save twice produces two worlds rather
 * than destroying the first.
 */
export async function saveStoryPackWorld(draft: StoryPackWorldDraft): Promise<StoryPackWorldSaveResult> {
  return requestJSON('/api/story-packs/world-save', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(draft),
  })
}

