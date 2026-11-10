import type { InitialActorTraitRoll, StoryCheckSettings, StoryDirectorModuleRefs, StoryImageSettings, StoryOpeningConfig, StoryPackBinding, StoryPlanningMode, StoryProtagonist, StoryStateSchemaPolicy, StoryStylePurposes } from './types'
import type { ThinkingLevel } from '@/features/settings/thinking-levels'

export interface StoryCreateInput {
  title: string
  custom_agent_id?: string
  profile_id: string
  thinking_level: ThinkingLevel
  origin: string
  protagonist?: StoryProtagonist
  story_teller_id: string
  /**
   * The style per generation purpose. Omitted means the main narrative style is
   * the story's only voice, which is what a hand-created story has always done.
   */
  style_purposes?: StoryStylePurposes
  planning_template_id: string
  planning_mode: StoryPlanningMode
  module_refs?: StoryDirectorModuleRefs
  reply_target_chars: number
  choice_count: number
  image_settings?: StoryImageSettings
  check_settings?: StoryCheckSettings
  opening?: StoryOpeningConfig
  initial_trait_rolls?: InitialActorTraitRoll[]
  state_schema_policy?: StoryStateSchemaPolicy
  pack_binding?: StoryPackBinding
  /**
   * This story's own standing creative direction, seeded from the world's
   * default when one starts it. It governs expression only and never overrides
   * a rule result or what a character knows.
   */
  narrative_requirements?: string
}

const STORY_OPENING_TEXT_LIMIT = 4000
export const DEFAULT_INTERACTIVE_REPLY_TARGET_CHARS = 2000
export const DEFAULT_INTERACTIVE_CHOICE_COUNT = 5
export const MIN_INTERACTIVE_CHOICE_COUNT = 2
export const MAX_INTERACTIVE_CHOICE_COUNT = 10

export function truncateStoryOpeningText(text: string) {
  const trimmed = text.trim()
  if (trimmed.length <= STORY_OPENING_TEXT_LIMIT) return trimmed
  return trimmed.slice(0, STORY_OPENING_TEXT_LIMIT)
}
