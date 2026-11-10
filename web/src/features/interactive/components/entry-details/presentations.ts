import type { TFunction } from 'i18next'
import i18n from '@/i18n'
import { realmLabel } from '../story-state/realm-label'
import type { AdventureDetailsResponse, AdventureItemDetails, AdventureSkillDetails, CultivationDetailsResponse, CultivationEntry, DetailFact, EntryCategory, EntryItemGroup, EntryMetadata, EntryPresentation } from './types'

const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
const numeric = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value)
const text = (value: unknown) => typeof value === 'string' ? value : ''
const percent = (value: number) => `${Number((value * 100).toFixed(3))}%`
const fact = (t: TFunction, key: string, value: string | number): DetailFact => ({ label: t(`entryDetails.fact.${key}`), value: String(value) })
const translated = (t: TFunction, key: string, fallback: string) => { const value = t(key); return !value || value === key ? t(fallback) : value }
function base(entry: EntryMetadata & { id: string; name: string }, category: EntryCategory): EntryPresentation {
  return { id: entry.id, name: entry.name, quality_grade: entry.quality_grade,
    description: entry.description, background: entry.background, art: entry.art,
    instance_description: entry.instance_description, source: entry.source,
    category, requirements: [], effects: [], costs: [], status: [] }
}

const itemGroups: Partial<Record<string, EntryItemGroup>> = {
  implement: 'weapons', weapon: 'weapons', 武器: 'weapons', 法器: 'weapons',
  armor: 'armor', 护具: 'armor',
  pill: 'pills', 丹药: 'pills',
  quest: 'quest', quest_item: 'quest', 任务品: 'quest', 任务物品: 'quest',
  manual: 'manuals', skill_book: 'manuals', 功法书: 'manuals', 秘籍: 'manuals', 术册: 'manuals',
  tool: 'tools', formation: 'tools', talisman: 'tools',
  attack_treasure: 'tools', defense_treasure: 'tools', support_treasure: 'tools',
  material: 'materials', 材料: 'materials',
}

function itemGroup(kind: string, definition: string): EntryItemGroup {
  if (/^manual[.]|(?:^|-)manual-|(?:^|-)record-/.test(definition)) return 'manuals'
  return Object.hasOwn(itemGroups, kind) ? itemGroups[kind]! : 'other'
}
function eligibility(entry: { usable?: boolean; castable?: boolean; reason_key?: string }, out: EntryPresentation, t: TFunction) {
  const allowed = entry.castable ?? entry.usable
  if (typeof allowed === 'boolean') out.status.push(fact(t, 'useEligibility', t(allowed ? 'entryDetails.available' : 'entryDetails.unavailable')))
  if (entry.reason_key) out.status.push(fact(t, 'reason', translated(t, entry.reason_key, 'entryDetails.unavailable')))
}
function adventureNames(data: AdventureDetailsResponse, t: TFunction) {
  return (id: string, kind: 'skills' | 'items') => data[kind]?.find(entry => entry.id === id)?.name || t(`entryDetails.unknown${kind === 'skills' ? 'Skill' : 'Item'}`)
}
// i18next interpolation must use the real projected power, not lore or quality.
function effectText(effect: { kind: string; power: number }, t: TFunction) {
  const key = `combatWindow.adventure.effect.${effect.kind}`
  return t(key) === key ? t('entryDetails.unreportedEffect') : t(key, { count: effect.power })
}
export function adventureSkillPresentation(entry: AdventureSkillDetails, data: AdventureDetailsResponse, t: TFunction): EntryPresentation {
  const out = base(entry, 'skills'), name = adventureNames(data, t)
  out.visual_theme = data.family
  const mastery = data.actor.skills?.[entry.id]
  if (numeric(mastery) && mastery > 0) out.visual_level = { kind: 'mastery', value: mastery, maximum: data.family === 'wuxia' ? 4 : 10, scale: entry.scale }
  if (entry.required_rank !== undefined) out.requirements.push(fact(t, 'requiredRank', entry.required_rank))
  if (entry.prerequisite) out.requirements.push(fact(t, 'prerequisite', name(entry.prerequisite, 'skills')))
  if (entry.required_item) out.requirements.push(fact(t, 'learningSource', name(entry.required_item, 'items')))
  if (entry.required_tool) out.requirements.push(fact(t, 'requiredTool', name(entry.required_tool, 'items')))
  if (entry.required_weapon) out.requirements.push(fact(t, 'requiredWeapon', name(entry.required_weapon, 'items')))
  if (entry.required_weapon_class) out.requirements.push(fact(t, 'requiredWeaponClass', translated(t, `entryDetails.weaponClass.${entry.required_weapon_class}`, 'entryDetails.unreported')))
  if (entry.scale) out.requirements.push(fact(t, 'scale', translated(t, `combatWindow.adventure.scale.${entry.scale}`, 'entryDetails.unreported')))
  for (const key of ['weapon_style', 'rush', 'once_per_target'] as const) if (entry[key]) out.requirements.push(fact(t, key, t('entryDetails.required')))
  if (entry.training_hours !== undefined) out.costs.push(fact(t, 'trainingHours', entry.training_hours))
  if (entry.cost !== undefined) out.costs.push(fact(t, data.family === 'wuxia' ? 'innerEnergy' : data.family === 'fantasy' ? 'effort' : 'energy', entry.cost))
  for (const [id, amount] of Object.entries(entry.materials || {})) out.costs.push({ label: name(id, 'items'), value: String(amount) })
  const effects = entry.effects?.length ? entry.effects : entry.kind && entry.power !== undefined ? [{ kind: entry.kind, power: entry.power }] : []
  for (const effect of effects) out.effects.push(fact(t, 'effect', effectText(effect, t)))
  for (const effect of entry.enemy_effects || []) out.effects.push(fact(t, 'opponentEffect', effectText(effect, t)))
  for (const conditional of entry.conditional_effects || []) out.effects.push({ label: t('entryDetails.conditional', { item: name(conditional.required_item, 'items') }), value: conditional.effects.map(effect => effectText(effect, t)).join(' · ') })
  if (entry.backlash) {
    const b = entry.backlash
    out.costs.push(fact(t, 'backlashHealth', `${b.health_percent}%`), fact(t, 'backlashInjury', b.injury))
    if (b.foundation_skills?.length) out.costs.push({ label: t('entryDetails.mitigatedBacklash', { skills: (b.foundation_skills || []).map(id => name(id, 'skills')).join(' / '), rank: b.foundation_rank ?? 1 }), value: t('entryDetails.backlashValues', { health: b.mitigated_health_percent, injury: b.mitigated_injury }) })
  }
  if (typeof entry.learned === 'boolean') out.status.push(fact(t, 'learned', t(entry.learned ? 'entryDetails.yes' : 'entryDetails.no')))
  if (typeof entry.trainable === 'boolean') out.status.push(fact(t, 'trainable', t(entry.trainable ? 'entryDetails.yes' : 'entryDetails.no')))
  const level = data.actor.skills?.[entry.id]
  if (numeric(level)) out.status.push(fact(t, 'mastery', level))
  const hours = data.actor.training?.[entry.id]
  if (numeric(hours)) out.status.push(fact(t, 'trainingHoursCompleted', hours))
  eligibility(entry, out, t)
  return out
}
export function adventureItemPresentation(entry: AdventureItemDetails, data: AdventureDetailsResponse, t: TFunction): EntryPresentation {
  const asset = data.actor.assets?.[entry.id]
  const slots = Object.entries(data.actor.equipment || {}).filter(([, id]) => id === entry.id).map(([slot]) => slot)
  const out = base(entry, asset ? 'assets' : slots.length ? 'equipment' : 'inventory'), name = adventureNames(data, t)
  out.item_group = asset ? 'assets' : itemGroup(entry.kind || '', entry.id)
  out.visual_theme = data.family
  if (entry.required_skill) out.requirements.push(fact(t, 'requiredSkill', name(entry.required_skill, 'skills')))
  if (entry.weapon_class) out.requirements.push(fact(t, 'weaponClass', translated(t, `entryDetails.weaponClass.${entry.weapon_class}`, 'entryDetails.unreported')))
  if (entry.scale) out.requirements.push(fact(t, 'scale', translated(t, `combatWindow.adventure.scale.${entry.scale}`, 'entryDetails.unreported')))
  if (entry.support_only) out.requirements.push(fact(t, 'supportOnly', t('entryDetails.supportOnly')))
  for (const key of ['equippable', 'deployable'] as const) if (typeof entry[key] === 'boolean') out.status.push(fact(t, key, t(entry[key] ? 'entryDetails.available' : 'entryDetails.unavailable')))
  for (const [id, amount] of Object.entries(entry.materials || {})) out.costs.push({ label: name(id, 'items'), value: String(amount) })
  if (entry.effect && entry.power !== undefined) out.effects.push(fact(t, 'effect', effectText({ kind: entry.effect, power: entry.power }, t)))
  else if (entry.power !== undefined && entry.power !== 0) out.effects.push(fact(t, 'power', entry.power))
  const quantity = data.actor.inventory?.[entry.id]
  if (numeric(quantity)) { out.summary = t('entryDetails.quantity', { count: quantity }); out.status.push(fact(t, 'quantity', quantity)) }
  if (entry.max_charges) out.status.push(fact(t, 'charges', `${data.actor.item_charges?.[entry.id] ?? entry.max_charges} / ${entry.max_charges}`))
  for (const slot of slots) {
    const [scale, kind] = slot.split(':')
    out.status.push(fact(t, 'slot', `${translated(t, `combatWindow.adventure.scale.${scale}`, 'entryDetails.unreported')} · ${translated(t, `storyStage.adventure.slot.${kind}`, 'entryDetails.unreported')}`))
  }
  if (asset) out.status.push(fact(t, 'health', `${asset.health} / ${asset.max_health}`), fact(t, 'shield', asset.shield), fact(t, 'fuel', asset.fuel))
  eligibility(entry, out, t)
  return out
}
export function adventurePresentations(data: AdventureDetailsResponse, t: TFunction): EntryPresentation[] {
  // Defense in depth: even a malformed response never turns a catalog into possessions.
  return [...(data.craft_methods || []).map(entry => {
    const out = base(entry, 'craft')
    out.visual_theme = data.family
    const itemName = (id: string) => entry.item_names?.[id] || adventureNames(data, t)(id, 'items')
    out.requirements.push(fact(t, 'requiredSkill', entry.skill_name || adventureNames(data, t)(entry.skill_id, 'skills')))
    out.requirements.push(fact(t, 'requiredSkillRank', entry.skill_rank))
    if (entry.required_asset) out.requirements.push(fact(t, 'requiredAsset', itemName(entry.required_asset)))
    if (entry.required_tool) out.requirements.push(fact(t, 'requiredTool', itemName(entry.required_tool)))
    out.costs.push(fact(t, 'craftingHours', entry.hours))
    if (entry.fuel_cost !== undefined && entry.fuel_cost > 0) out.costs.push(fact(t, 'fuelCost', entry.fuel_cost))
    out.costs.push(fact(t, 'materials', Object.entries(entry.materials).map(([id, count]) => itemName(id) + ' × ' + count).join('；')))
    out.effects.push(fact(t, 'products', itemName(entry.output_id) + ' × ' + entry.output_count))
    return out
  }), 
    ...(data.skills || []).filter(entry => entry.id !== 'strike' && (data.actor.skills?.[entry.id] || 0) > 0).map(entry => adventureSkillPresentation(entry, data, t)),
    ...(data.items || []).filter(entry => (data.actor.inventory?.[entry.id] || 0) > 0 || Object.values(data.actor.equipment || {}).includes(entry.id) || Boolean(data.actor.assets?.[entry.id])).map(entry => adventureItemPresentation(entry, data, t)),
  ]
}

/** The whitelist is presentation only. Unknown fields are never dumped as JSON. */
const numericEffects = ['power', 'targets', 'execution_actions', 'duration_exchanges', 'residual_burden_floor', 'safe_recovery_minutes', 'support_days', 'growth_coefficient', 'restore_amount', 'shield_amount', 'defense_pierce'] as const
const fractionalEffects = ['ignore_defense', 'hit_offset', 'recovery_min_health', 'recovery_min_mana', 'shield_fraction', 'shield_base_health_fraction', 'breakthrough_bonus', 'quality_attack', 'mp_cost_reduction', 'strain_reduction', 'attack_bonus', 'defense_bonus', 'bonus', 'max_health_fraction', 'max_mana_fraction'] as const
function cultivationDefinition(out: EntryPresentation, definition: Record<string, unknown>, t: TFunction) {
  for (const key of numericEffects) if (numeric(definition[key])) out.effects.push(fact(t, key, definition[key]))
  for (const key of fractionalEffects) if (numeric(definition[key])) out.effects.push(fact(t, key === 'strain_reduction' && definition.special_effect === 'item.effect.armor.rated_stability' ? 'rated_strain_reduction' : key, percent(definition[key])))
  for (const key of ['requires_focus', 'leaves_residual'] as const) if (definition[key] === true) out.requirements.push(fact(t, key, t('entryDetails.required')))
  for (const key of ['minimum_stage', 'supports_through', 'applicable_stage', 'effect_stage'] as const) if (text(definition[key])) out.requirements.push(fact(t, key, realmLabel(text(definition[key]), t) || t('entryDetails.unreported')))
  if (text(definition.required_manual_id)) {
    const realm = text(definition.required_manual_id).replace('manual.spirit.', '')
    out.requirements.push(fact(t, 'required_manual_id', translated(t, `item.spirit.${realm}_manual.name`, 'entryDetails.unreported')))
  }
  if (numeric(definition.standard_learning_minutes)) out.costs.push(fact(t, 'standard_learning_minutes', definition.standard_learning_minutes))
  if (numeric(definition.operating_mana_fraction)) out.costs.push(fact(t, 'operating_mana_fraction', percent(definition.operating_mana_fraction)))
  if (text(definition.special_effect)) out.effects.push(fact(t, 'special_effect', translated(t, text(definition.special_effect), 'entryDetails.unreportedEffect')))
  for (const key of ['circulation', 'element', 'purpose', 'bonus_attribute', 'support_strength'] as const) if (text(definition[key])) out.effects.push(fact(t, key, translated(t, `entryDetails.value.${text(definition[key])}`, 'entryDetails.unreported')))
  for (const [attribute, amount] of Object.entries(record(definition.specialty))) if (numeric(amount)) out.effects.push({ label: translated(t, `entryDetails.value.${attribute}`, 'entryDetails.unreported'), value: percent(amount) })
  // Payloads carry actual mechanics. Grade/rarity/quality are deliberately not
  // rendered as a competing second quality system, and never grant bonuses.
  for (const key of ['implement', 'armor', 'treasure', 'pill', 'talisman', 'formation'] as const) {
    const nested = record(definition[key])
    if (Object.keys(nested).length) cultivationDefinition(out, nested, t)
  }
}
export function cultivationPresentation(entry: CultivationEntry, category: EntryCategory, t: TFunction): EntryPresentation {
  const out = base(entry, category)
  if (entry.record_key) out.id = entry.record_key
  if (category === 'inventory' || category === 'equipment') {
    out.item_group = itemGroup(entry.recorded_kind || text(entry.definition?.kind) || (entry.slot === 'weapon' ? 'weapon' : entry.slot === 'garment' ? 'armor' : entry.slot || ''), entry.def_id || '')
  }
  if (category === 'techniques' && entry.definition?.quality === 'good') {
    out.quality_grade ||= 'blue'
    out.grade_label = t('entryDetails.methodGood')
  }
  out.visual_theme = 'cultivation'
  const declaredStage = text(entry.definition?.applicable_stage) || text(entry.definition?.minimum_stage)
  if (declaredStage) out.visual_level = { kind: 'stage', value: declaredStage }
  if (entry.name_key) out.name = translated(t, entry.name_key, 'entryDetails.unreported')
  if (!entry.uncatalogued && typeof entry.stage_eligible === 'boolean') out.status.push(fact(t, 'stageEligibility', t(entry.stage_eligible ? 'entryDetails.yes' : 'entryDetails.no')))
  if (entry.recorded_grade) out.status.push(fact(t, 'recordedGrade', entry.recorded_grade))
  if (entry.recorded_rarity) out.status.push(fact(t, 'recordedRarity', entry.recorded_rarity))
  if (entry.recorded_effect) out.effects.push(fact(t, 'effect', entry.recorded_effect))
  cultivationDefinition(out, entry.definition || {}, t)
  for (const key of ['restore_amount', 'shield_amount', 'duration_exchanges'] as const) if (numeric(entry[key])) out.effects.push(fact(t, key, entry[key]))
  if (numeric(entry.attack_bonus)) out.effects.push(fact(t, 'attack_bonus', percent(entry.attack_bonus)))
  if (numeric(entry.effective_strain_reduction)) out.effects.push(fact(t, 'effective_strain_reduction', percent(entry.effective_strain_reduction)))
  if (entry.mastered !== false && entry.mana_cost !== undefined) out.costs.push(fact(t, 'mana', entry.mana_cost))
  if (entry.mastered !== false && entry.burden_cost !== undefined) out.costs.push(fact(t, 'burden', entry.burden_cost))
  if (!entry.uncatalogued && entry.operating_mana_cost !== undefined) out.costs.push(fact(t, 'operatingMana', entry.operating_mana_cost))
  if (entry.quantity !== undefined) { out.summary = t('entryDetails.quantity', { count: entry.quantity }); out.status.push(fact(t, 'quantity', entry.quantity)) }
  if (entry.slot) out.status.push(fact(t, 'slot', translated(t, `combatWindow.slot.${entry.slot}`, 'entryDetails.unreported')))
  if (typeof entry.mastered === 'boolean') out.status.push(fact(t, 'mastered', t(entry.mastered ? 'entryDetails.yes' : 'entryDetails.no')))
  if (typeof entry.active === 'boolean') out.status.push(fact(t, 'active', t(entry.active ? 'entryDetails.yes' : 'entryDetails.no')))
  if (entry.learning_minutes !== undefined) out.status.push(fact(t, 'learning_minutes', entry.learning_minutes))
  if (entry.required_learning_minutes !== undefined) out.requirements.push(fact(t, 'requiredLearningMinutes', entry.required_learning_minutes))
  if (category === 'craft') {
    const definition = entry.definition || {}
    if (numeric(definition.minutes_per_batch)) out.costs.push(fact(t, 'minutesPerBatch', definition.minutes_per_batch))
    if (numeric(definition.loss_chance)) out.status.push(fact(t, 'lossChance', percent(definition.loss_chance)))
    const itemName = (id: string) => {
      const name = entry.material_names?.[id]
      // Built-ins use localisation keys; Story-local definitions carry their
      // actual author-given names, which must survive both UI languages.
      return name ? (i18n.exists(name) ? t(name) : name) : t('entryDetails.unknownItem')
    }
    const names = (key: string) => Array.isArray(definition[key]) ? (definition[key] as { def_id: string; quantity: number }[]).map(value => itemName(value.def_id) + ' × ' + value.quantity).join('；') : ''
    if (names('inputs')) out.costs.push(fact(t, 'materials', names('inputs')))
    if (names('outputs')) out.effects.push(fact(t, 'products', names('outputs')))
    if (Array.isArray(definition.required_tools)) for (const tool of definition.required_tools) {
      if (typeof tool === 'string') out.requirements.push(fact(t, 'requiredTool', itemName(tool)))
    }
    if (definition.failure_cost === 'consume_inputs' || (numeric(definition.loss_chance) && definition.loss_chance > 0)) out.costs.push(fact(t, 'failureCost', t('entryDetails.failureConsumesInputs')))
    if (text(definition.required_location)) out.requirements.push(fact(t, 'location', text(definition.required_location)))
  }
  eligibility(entry, out, t)
  return out
}
export function cultivationPresentations(data: CultivationDetailsResponse, t: TFunction): EntryPresentation[] {
  return [...(data.craft_methods || []).map(entry => cultivationPresentation(entry, 'craft', t)), ...(data.techniques || []).map(entry => cultivationPresentation(entry, 'techniques', t)), ...(data.skills || []).filter(entry => entry.id !== 'strike' || (entry.record_key && entry.record_key !== entry.id)).map(entry => cultivationPresentation(entry, 'skills', t)), ...(data.items || []).filter(entry => (entry.quantity ?? 0) > 0).map(entry => cultivationPresentation(entry, entry.slot ? 'equipment' : 'inventory', t))]
}
