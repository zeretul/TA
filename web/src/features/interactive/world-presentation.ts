import type { TFunction } from 'i18next'
import type { Snapshot, StorySummary, WorldPresentationTerms } from './types'

type AdventureFamily = 'wuxia' | 'space' | 'fantasy'
type WorldFamily = AdventureFamily | 'cultivation' | 'general'
export interface WorldPresentationSource {
  story?: Pick<StorySummary, 'pack_binding' | 'module_refs'> | null
  snapshot?: Pick<Snapshot, 'actor_state_schema'> | null
  family?: AdventureFamily
}
export interface WorldPresentation {
  family: WorldFamily
  combatMode: 'adventure' | 'cultivation' | 'none'
  intimacyAvailable: boolean
  communication: string
  terms: Required<WorldPresentationTerms>
  t: TFunction
}

const families = new Set<string>(['wuxia', 'space', 'fantasy'])
const templateFamilies: Record<string, WorldFamily> = {
  'xiuxian-state': 'cultivation', 'wuxia-state-v1': 'wuxia',
  'space-state-v1': 'space', 'fantasy-state-v1': 'fantasy',
}
const termNames = ['combat', 'exploration', 'techniques', 'partner', 'energy'] as const

/** Read only the current story's frozen profile/schema. Display words never grant a capability. */
export function resolveWorldPresentation(source: WorldPresentationSource, locale: string, t: TFunction): WorldPresentation {
  const { story, snapshot } = source
  const profile = story?.pack_binding?.gameplay
  const refs = story?.module_refs || story?.pack_binding?.module_refs
  const schema = snapshot?.actor_state_schema?.system
  const hasNativeRules = Boolean(schema?.adventure_rules)
  const suppliedFamily = schema?.adventure_rules?.family
  const nativeFamily = suppliedFamily && families.has(suppliedFamily) ? suppliedFamily : undefined
  const schemaCultivation = !hasNativeRules && schema?.templates?.some(template =>
    template.id === 'protagonist' && template.fields?.some(field => field.value_format === 'stage'))
  const legacyCultivation = !profile && (schema ? Boolean(schemaCultivation) : refs?.actor_state_id === 'xiuxian-state')
  const declaredFamily = profile?.status_template
    ? templateFamilies[profile.status_template] || 'general'
    : nativeFamily || (legacyCultivation ? 'cultivation' : source.family && families.has(source.family) ? source.family : 'general')
  const family: WorldFamily = declaredFamily
  const disabled = Boolean(refs?.actor_state_disabled)
  let combatMode: WorldPresentation['combatMode'] = 'none'
  const declaredCombat = profile?.combat
  if (!disabled && declaredCombat && families.has(declaredCombat)) {
    // A loaded schema must agree. Before it arrives the validated frozen profile is sufficient.
    if (!schema || nativeFamily === declaredCombat) combatMode = 'adventure'
  } else if (!disabled && (declaredCombat === 'cultivation' && family === 'cultivation' || legacyCultivation)) {
    if (!schema || schemaCultivation) combatMode = 'cultivation'
  } else if (!disabled && !profile && nativeFamily) {
    combatMode = 'adventure'
  }
  const intimacyAvailable = !disabled && !hasNativeRules && (!schema || Boolean(schemaCultivation))
    && (legacyCultivation || profile?.status_template === 'xiuxian-state' && profile.growth === 'cultivation')
  const language = locale.toLowerCase().startsWith('en') ? 'en-US' : 'zh-CN'
  const authored = profile?.presentation?.[language]
  const terms = Object.fromEntries(termNames.map((key) => {
    const label = authored?.[key]?.trim()
    return [key, label || t('worldPresentation.' + family + '.' + key)]
  })) as Required<WorldPresentationTerms>
  const translated = authored?.energy || authored?.techniques ? worldTermsTranslation(t, terms, authored) : t
  return { family, combatMode, intimacyAvailable, communication: profile?.communication || '', terms, t: translated }
}

/** Translate known semantic labels, never replace words inside saved names or prose. */
function worldTermsTranslation(t: TFunction, terms: Required<WorldPresentationTerms>, authored: WorldPresentationTerms): TFunction {
  const energyNames = new Set(['storyStage.adventure.energy', 'storyStage.adventure.innerEnergy', 'storyStage.adventure.effort', 'combatWindow.mana', 'combatWindow.adventure.energy', 'combatWindow.adventure.innerEnergy', 'combatWindow.adventure.effort'])
  const energyCosts = new Set(['entryDetails.fact.mana', 'entryDetails.fact.operatingMana', 'entryDetails.fact.innerEnergy', 'entryDetails.fact.energy', 'entryDetails.fact.effort'])
  const energyPhrases: Record<string, string> = {
    'entryDetails.fact.recovery_min_mana': 'worldPresentation.recoveryEnergyRatio',
    'entryDetails.fact.mp_cost_reduction': 'worldPresentation.energyReduction',
    'entryDetails.fact.max_mana_fraction': 'worldPresentation.energyMaximumRatio',
    'entryDetails.fact.operating_mana_fraction': 'worldPresentation.operatingEnergyRatio',
    'entryDetails.value.mana_max': 'worldPresentation.energyMaximum',
    'entryDetails.value.restore_mana': 'worldPresentation.energyRestoration',
    'combatWindow.skillCost': 'worldPresentation.skillCost',
    'combatWindow.step.manaRecovered': 'worldPresentation.restoreEnergy',
    'combatWindow.adventure.effect.energy': 'worldPresentation.restoreEnergyCount',
    'combatWindow.adventure.spaceUpkeep': 'worldPresentation.spaceUpkeep',
    'combatWindow.adventure.effect.drain': 'worldPresentation.drain',
    'combatWindow.adventure.effect.overclock': 'worldPresentation.overclock',
  }
  return ((key: string | string[], options?: Record<string, unknown>) => {
    if (typeof key === 'string') {
      if (authored.energy) {
        if (energyNames.has(key)) return terms.energy
        if (energyPhrases[key]) return t(energyPhrases[key], { ...options, energy: terms.energy })
        if (energyCosts.has(key)) return t('worldPresentation.energyCost', { ...options, energy: terms.energy })
        if (key === 'combatWindow.adventure.cost') return t('worldPresentation.energyAmountCost', { ...options, energy: terms.energy })
        if (key === 'combatWindow.refusal.insufficientMana') return t('worldPresentation.insufficientEnergy', { ...options, energy: terms.energy })
        if (key === 'combatWindow.item.restoreMana') return t('worldPresentation.restoreEnergy', { ...options, energy: terms.energy })
      }
      if (authored.techniques && key === 'storyStage.state.group.methods') return terms.techniques
      if (authored.techniques && key === 'storyStage.state.method.none') return t('worldPresentation.noTechniques', { techniques: terms.techniques })
    }
    return t(key, options)
  }) as TFunction
}
