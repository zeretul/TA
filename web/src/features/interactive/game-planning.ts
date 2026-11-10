import type { TFunction } from 'i18next'
import type { GamePlanningSection, GamePlanningTemplate } from './types'

export const DEFAULT_GAME_PLANNING_TEMPLATE_ID = 'default'

/**
 * Splits the planning templates into the world's own suggestion and the rest.
 *
 * The suggestion comes from the world (`recommended_planning_template_id`), not
 * from a local list: which shape fits a story depends on how that world is
 * actually built, so a fixed whitelist would recommend by genre label instead.
 * A world that names nothing, or names a template this machine does not have,
 * suggests nothing and the caller shows one plain list.
 */
export function splitRecommendedPlanningTemplates(
  templates: GamePlanningTemplate[],
  recommendedID: string | undefined,
): { recommended: GamePlanningTemplate[]; others: GamePlanningTemplate[]; recommendedID: string } {
  const wanted = (recommendedID || '').trim()
  const effectiveID = templates.some((template) => template.id === wanted) ? wanted : ''
  if (!effectiveID) return { recommended: [], others: templates, recommendedID: '' }
  return {
    recommended: templates.filter((template) => template.id === effectiveID),
    others: templates.filter((template) => template.id !== effectiveID),
    recommendedID: effectiveID,
  }
}

/** Localize built-in templates while preserving custom creator content. */
export function gamePlanningTemplateName(template: GamePlanningTemplate, t: TFunction): string {
  return localizedBuiltinValue(template, t, 'name', template.name || template.id)
}

export function gamePlanningTemplateDescription(template: GamePlanningTemplate, t: TFunction): string {
  return localizedBuiltinValue(template, t, 'description', template.description || '')
}

export function gamePlanningSectionTitle(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
): string {
  return localizedBuiltinSectionValue(template, section, t, 'title', section.title)
}

export function gamePlanningSectionDescription(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
): string {
  return localizedBuiltinSectionValue(template, section, t, 'description', section.description || '')
}

function localizedBuiltinValue(
  template: GamePlanningTemplate,
  t: TFunction,
  field: 'name' | 'description',
  fallback: string,
): string {
  if (template.custom || template.builtin_overridden) return fallback
  return t(`storyPicker.planningTemplates.${template.id}.${field}`, { defaultValue: fallback })
}

function localizedBuiltinSectionValue(
  template: GamePlanningTemplate,
  section: GamePlanningSection,
  t: TFunction,
  field: 'title' | 'description',
  fallback: string,
): string {
  if (template.custom || template.builtin_overridden) return fallback
  return t(`storyPicker.planningTemplates.${template.id}.sections.${section.id}.${field}`, { defaultValue: fallback })
}
