import type { TFunction } from 'i18next'
import type { CombatWindowItem, CombatWindowSide, CombatWindowSkill } from '../../types'

export { realmLabel } from '../story-state/realm-label'

/**
 * What the window says when it holds no wording for a definition.
 *
 * A definition ID is storage's identity for a technique or a dose, not
 * something the player can act on, so an unworded one is reported in plain
 * language. The ID is logged once per definition instead of drawn: it is
 * what a bug report needs, and it is not what a player needs.
 */
const reportedUnworded = new Set<string>()

function reportUnworded(kind: string, id: string) {
  const key = `${kind}:${id}`
  if (reportedUnworded.has(key)) return
  reportedUnworded.add(key)
  console.warn(`[combat-window] no wording for ${kind} ${id}`)
}

function unwordedTechnique(id: string, t: TFunction): string {
  reportUnworded('technique', id)
  return t('combatWindow.unknownTechnique')
}

function unwordedItem(id: string, t: TFunction): string {
  reportUnworded('item', id)
  return t('combatWindow.unknownItem')
}

/**
 * The technique's name from its own localisation key. The rules publish the key
 * with the definition, so the client never invents a name; an unknown key falls
 * back to the ID rather than to a made-up technique.
 *
 * A technique this story generated has no localisation key at all: its name is
 * the story's own wording and is the only name that exists for it, so it is
 * shown as written. The ID is deliberately not used as a fallback there — a
 * generated ID is `gen.<template>.<name>` and reading it out would show the
 * player the rule layer's own bookkeeping.
 */
export function skillLabel(skill: CombatWindowSkill, t: TFunction): string {
  const name = (skill.name || '').trim()
  if (name) return name
  const key = (skill.name_key || '').trim()
  if (!key) return unwordedTechnique(skill.id, t)
  const translated = t(key)
  return translated === key ? unwordedTechnique(skill.id, t) : translated
}

/**
 * The name of the technique a step performed, in the player's language.
 *
 * The step carries the definition's stable ID and the wording lives in this
 * interface's own locale file, so a technique the locale does not word falls
 * back to the ID rather than to a blank. A story-generated technique carries its
 * own name beside the ID, and that name wins because it is the only wording the
 * technique has.
 */
export function techniqueLabel(
  skillId: string | undefined,
  t: TFunction,
  name?: string,
): string {
  const generated = (name || '').trim()
  if (generated) return generated
  const id = (skillId || '').trim()
  if (!id) return ''
  const key = `skill.${id}.name`
  const translated = t(key)
  return translated === key ? unwordedTechnique(id, t) : translated
}

/**
 * One carried consumable's name from its own localisation key, falling back to
 * the definition's stable ID rather than to a made-up name.
 */
export function itemLabel(item: CombatWindowItem, t: TFunction): string {
  const key = (item.name_key || '').trim()
  if (!key) return unwordedItem(item.def_id, t)
  const translated = t(key)
  return translated === key ? unwordedItem(item.def_id, t) : translated
}

/**
 * What one dose of this consumable actually does, in the player's language.
 *
 * The figures are the ones the rules priced, not ones this interface derived, so
 * a 下品 restoring pill reads as its own registered amount and never as a
 * percentage of whoever happens to be holding it.
 */
export function itemEffectLabel(item: CombatWindowItem, t: TFunction): string {
  switch (item.purpose) {
    case 'shield':
      return t('combatWindow.item.shield', { amount: item.shield_amount ?? 0, count: item.duration_exchanges ?? 0 })
    case 'restore_health':
      return t('combatWindow.item.restoreHealth', { amount: item.restore_amount ?? 0 })
    case 'restore_mana':
      return t('combatWindow.item.restoreMana', { amount: item.restore_amount ?? 0 })
    case 'boost_attack':
      return t('combatWindow.item.boostAttack', {
        percent: Math.round((item.attack_bonus ?? 0) * 100),
        count: item.duration_exchanges ?? 0,
      })
    default:
      return t('combatWindow.item.unknownEffect')
  }
}

/** Why a technique cannot be performed right now, in the player's language. */
export function reasonLabel(reasonKey: string | undefined, t: TFunction): string {
  const key = (reasonKey || '').trim()
  if (!key) return ''
  const translated = t(key)
  return translated === key ? '' : translated
}

/**
 * Why this technique cannot be performed, with the numbers the player needs to
 * act on it.
 *
 * §T21 asks a refusal to state what is missing, not just that something is. The
 * spirit cost is the one reason that is arithmetic the player can see both
 * halves of: the window already carries the pool and the price, so the sentence
 * carries them too. The others are facts with nothing to count — the burden
 * capacity behind a refused heavy move is derived from health and never sent,
 * so that reason keeps its own wording rather than inventing a denominator.
 */
export function skillRefusalLabel(skill: CombatWindowSkill, actor: CombatWindowSide, t: TFunction): string {
  const key = (skill.reason_key || '').trim()
  if (key === 'reason.insufficient_mana' && actor.mana !== undefined) {
    return t('combatWindow.refusal.insufficientMana', {
      need: skill.mana_cost || 0,
      have: actor.mana,
      max: actor.mana_max ?? actor.mana,
    })
  }
  return reasonLabel(key, t)
}

/**
 * The cost line for a technique. A technique with no cost says so rather than
 * showing a zero, because a plain attack's zero is a real property of it and not
 * a missing number.
 */
export function skillCostLabel(skill: CombatWindowSkill, t: TFunction): string {
  const mana = skill.mana_cost || 0
  const burden = skill.burden_cost || 0
  if (mana <= 0 && burden <= 0) return t('combatWindow.skillCostFree')
  return t('combatWindow.skillCost', { mana, burden })
}

/**
 * The name of one published variant of a technique.
 *
 * The rules publish the key with the variant, so the wording is never invented
 * here; a key this interface does not word falls back to the form's own generic
 * wording before it falls back to the variant's ID.
 *
 * The generic step matters for a technique this story generated: its ID is
 * `gen.<template>.<name>`, so the key the rules derive for its burning form can
 * never be translated, and the raw `burn` would show the player the rule layer's
 * own bookkeeping. The form itself is a published fact — §5 fixes exactly these
 * two — so naming it is not inventing anything, and it is deliberately not the
 * base technique's name, which would make two different casts look identical.
 */
export function variantLabel(variant: { id: string; name_key?: string }, t: TFunction): string {
  const key = (variant.name_key || '').trim()
  if (key) {
    const translated = t(key)
    if (translated !== key) return translated
  }
  const genericKey = `combatWindow.variantName.${variant.id}`
  const generic = t(genericKey)
  if (generic !== genericKey) return generic
  return variant.id
}

/**
 * The lasting effect a variant applies, in the player's language.
 *
 * The server sends the engine's own identifier rather than a sentence, so this
 * is the only place the wording lives. An identifier this locale does not word
 * reports nothing rather than showing the player `burn`.
 */
export function variantEffectName(effect: string | undefined, t: TFunction): string {
  const key = (effect || '').trim()
  if (!key) return ''
  const translationKey = `combatWindow.variantEffect.${key}`
  const translated = t(translationKey)
  return translated === translationKey ? '' : translated
}

/**
 * What a variant trades, in the player's language.
 *
 * §5 keeps both forms' 耗灵 and 负担 unchanged, so the one number that differs is
 * the power. Showing it is what lets the player see that 火弹's burning form is
 * weaker up front and 重雷's delaying form gives up direct force for control.
 * The lasting effect is appended because the power alone does not say what the
 * form buys: a form that costs 0.25 power and adds nothing would be a strictly
 * worse choice, and one that adds a burn is not.
 */
export function variantEffectLabel(
  variant: { power: number; effect?: string },
  basePower: number | undefined,
  t: TFunction,
): string {
  const power = Number.isFinite(variant.power) ? variant.power.toFixed(2) : ''
  const trade = Number.isFinite(basePower as number)
    ? t('combatWindow.variantPowerFrom', { power, base: (basePower as number).toFixed(2) })
    : t('combatWindow.variantPower', { power })
  const effect = variantEffectName(variant.effect, t)
  return effect ? `${trade} · ${effect}` : trade
}

/**
 * One sentence of what a technique really does, in the player's language.
 *
 * The properties come from the published definition rather than from a written
 * description stored beside it, so a balance revision changes the sentence and a
 * technique the engine stops giving a shield stops claiming one. Anything the
 * rules did not publish is simply absent: a plain attack yields no sentence, and
 * the window shows none rather than a line saying it does nothing.
 */
export function skillEffectLabel(skill: CombatWindowSkill, t: TFunction): string {
  const effects = skill.effects
  if (!effects) return ''
  const parts: string[] = []
  if (effects.execution_actions === 2) parts.push(t('combatWindow.effect.prepares'))
  if (effects.ignore_defense) {
    parts.push(t('combatWindow.effect.ignoreDefense', { percent: Math.round(effects.ignore_defense * 100) }))
  }
  if (effects.shield_fraction) {
    parts.push(t('combatWindow.effect.shield', {
      percent: Math.round(effects.shield_fraction * 100),
      count: effects.duration_exchanges ?? 0,
    }))
  }
  if (effects.requires_focus) parts.push(t('combatWindow.effect.requiresFocus'))
  if (effects.leaves_residual) parts.push(t('combatWindow.effect.leavesResidual'))
  return parts.join(t('combatWindow.effectSeparator'))
}

/** How a combatant currently stands. */
export function standingLabel(standing: string, t: TFunction): string {
  const key = `combatWindow.standing.${standing}`
  const translated = t(key)
  return translated === key ? standing : translated
}

/** The battle's settled result. */
export function resultLabel(result: string | undefined, t: TFunction): string {
  const key = (result || '').trim()
  if (!key) return ''
  const translated = t(`combatWindow.result.${key}`)
  return translated === `combatWindow.result.${key}` ? key : translated
}

/**
 * A pool as the player may know it. An unestablished value is reported as not yet
 * known rather than as a zero, because a zero would read as a fact the rules
 * never settled.
 */
export function poolLabel(current: number | undefined, max: number | undefined, t: TFunction): string {
  if (current === undefined && max === undefined) return t('combatWindow.unknownValue')
  if (max === undefined) return String(current)
  if (current === undefined) return `? / ${max}`
  return `${current} / ${max}`
}

/** The headcount of one side, so the window can report both sides' numbers. */
export function sideCount(sides: CombatWindowSide[], side: 'player' | 'enemy'): number {
  return sides.filter((combatant) => combatant.side === side).length
}
