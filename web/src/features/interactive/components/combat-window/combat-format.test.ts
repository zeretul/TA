import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import type { CombatWindowItem, CombatWindowSide, CombatWindowSkill } from '../../types'
import {
  itemLabel, poolLabel, realmLabel, reasonLabel, resultLabel, skillCostLabel, skillEffectLabel, skillLabel,
  skillRefusalLabel, standingLabel, techniqueLabel, variantEffectLabel,
} from './combat-format'

/**
 * These cover §七.1's display boundary: what the player is told about an
 * opponent must be something the rules actually established, and anything the
 * rules never settled has to read as unknown rather than as a fact. The backend
 * omits a value it never settled (see combatWindowSides); this is the other half
 * — the interface must render that omission as 未探明 instead of a zero, and
 * must not invent a name, a realm or a technique it was not given.
 */

// A stand-in for i18next's contract: an unknown key comes back as itself, and
// {{name}} interpolates. That is the exact behaviour these functions branch on.
const zh: Record<string, string> = {
  'combatWindow.realm.unknown': '境界未明',
  'combatWindow.unknownValue': '未探明',
  'combatWindow.unknownTechnique': '未识得的招式',
  'combatWindow.unknownItem': '未识得的药物',
  'combatWindow.reasonUnknown': '眼下这一手办不到',
  'combatWindow.health': '生命',
  'combatWindow.realm.qi': '炼气{{layer}}层',
  'combatWindow.realm.foundation': '筑基{{stage}}',
  'combatWindow.realmStage.early': '初期',
  'combatWindow.skillCostFree': '无消耗',
  'combatWindow.skillCost': '耗灵 {{mana}} · 施招负担 {{burden}}',
  'combatWindow.standing.surrendered': '已投降',
  'combatWindow.result.enemy_fled': '敌方撤离',
  'skill.condensed_blade.name': '凝刃术',
  'item.pill.restore_health_lower.name': '回春丹',
  'reason.insufficient_mana': '灵力不足',
  'combatWindow.refusal.insufficientMana': '灵力不够：现有 {{have}} / {{max}}，这一招要 {{need}}。',
  'combatWindow.variantPowerFrom': '威力 {{power}}（原 {{base}}）',
  'combatWindow.variantEffect.burn': '命中后灼烧两轮',
  'combatWindow.effect.prepares': '需先准备一回合',
  'combatWindow.effect.ignoreDefense': '无视对方 {{percent}}% 防御',
  'combatWindow.effect.shield': '生成相当于生命上限 {{percent}}% 的护盾，持续 {{count}} 轮',
  'combatWindow.effect.requiresFocus': '需持法器施展',
  'combatWindow.effect.leavesResidual': '命中会留下未愈的伤',
  'combatWindow.effectSeparator': '；',
}

const t = ((key: string, options?: Record<string, unknown>) => {
  const template = zh[key]
  if (template === undefined) return key
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(options?.[name] ?? ''))
}) as unknown as TFunction

describe('poolLabel reports an unestablished pool as unknown', () => {
  it('says 未探明 rather than 0 when neither the value nor its maximum was settled', () => {
    // The backend sends neither field for an opponent whose numbers the rules
    // never established, and "生命 0 / 0" would read as a corpse.
    expect(poolLabel(undefined, undefined, t)).toBe('未探明')
  })

  it('shows a settled pool as current over maximum', () => {
    expect(poolLabel(40, 60, t)).toBe('40 / 60')
  })

  it('keeps a known current value when only the maximum is unknown', () => {
    expect(poolLabel(40, undefined, t)).toBe('40')
  })

  it('marks an unknown current value without inventing one', () => {
    // A known maximum with an unknown current is still not a number the player
    // may be told, so it is marked rather than filled in.
    expect(poolLabel(undefined, 60, t)).toBe('? / 60')
  })

  it('treats a real zero as a settled zero', () => {
    // An enemy who has spent every point of mana really is at 0, and that is a
    // fact the fight settled — it must not be confused with "not established".
    expect(poolLabel(0, 60, t)).toBe('0 / 60')
  })
})

describe('realmLabel never guesses a realm', () => {
  it('words a known stage key', () => {
    expect(realmLabel('qi_3', t)).toBe('炼气3层')
    expect(realmLabel('foundation_early', t)).toBe('筑基初期')
  })

  it('reports an unrecognised key as unknown without exposing it or guessing a nearby realm', () => {
    // Guessing here would place an opponent at a stage the rules never put them.
    expect(realmLabel('nascent_peak', t)).toBe('境界未明')
  })

  it('is empty when no realm was reported', () => {
    expect(realmLabel(undefined, t)).toBe('')
    expect(realmLabel('   ', t)).toBe('')
  })
})

describe('technique naming falls back to the id, never to a guess', () => {
  const skill: CombatWindowSkill = { id: 'condensed_blade', version: 1, castable: true, name_key: 'skill.condensed_blade.name' }

  it('uses the definition’s own localisation key', () => {
    expect(skillLabel(skill, t)).toBe('凝刃术')
  })

  it('says a technique is unfamiliar when the locale does not word the key', () => {
    // A definition ID is the rule layer's identity for the art, not something
    // the player can read as a technique, so the fallback is a sentence and
    // the ID goes to the log where a bug report can still find it.
    const unworlded: CombatWindowSkill = { ...skill, name_key: 'skill.no_such_technique.name' }
    const label = skillLabel(unworlded, t)
    expect(label).toBe('未识得的招式')
    expect(label).not.toContain('condensed_blade')
  })

  it('says a technique is unfamiliar when the definition carries no key at all', () => {
    expect(skillLabel({ ...skill, name_key: undefined }, t)).toBe('未识得的招式')
  })

  it('words a step’s technique from the step’s own id', () => {
    expect(techniqueLabel('condensed_blade', t)).toBe('凝刃术')
    // A technique the locale does not know is named in plain language, so the
    // window never shows a blank and never shows `unknown_art` either.
    const unknown = techniqueLabel('unknown_art', t)
    expect(unknown).toBe('未识得的招式')
    expect(unknown).not.toContain('unknown_art')
    expect(techniqueLabel(undefined, t)).toBe('')
  })

  it('keeps a story-given technique name over the catalogue and the fallback', () => {
    // A generated art exists only under the story's own wording, so that
    // wording is the name — not the catalogue's, and never the fallback.
    expect(techniqueLabel('gen.fire_bolt.赤炎诀', t, '赤炎诀')).toBe('赤炎诀')
  })
})

describe('cost and reason labels', () => {
  it('says a free technique is free rather than pricing it at zero', () => {
    // An ordinary attack's zero cost is a property of it, not a missing number,
    // so it reads as 无消耗 instead of 耗灵 0.
    expect(skillCostLabel({ id: 'strike', version: 1, castable: true }, t)).toBe('无消耗')
  })

  it('prices a paid technique at the caster’s own numbers', () => {
    expect(skillCostLabel({ id: 'fire_bolt', version: 1, castable: true, mana_cost: 8, burden_cost: 2 }, t)).toBe('耗灵 8 · 施招负担 2')
  })

  it('words a known refusal reason and stays blank for an unknown one', () => {
    expect(reasonLabel('reason.insufficient_mana', t)).toBe('灵力不足')
    // Blank rather than the raw key: the panel then shows its own generic
    // "现在不能施展" instead of leaking an internal key to the player.
    expect(reasonLabel('reason.some_new_rule', t)).toBe('')
    expect(reasonLabel(undefined, t)).toBe('')
  })

  it('states the pool and the price when the spirit is what is short', () => {
    // §T21: a refusal says what is missing, and for this one both halves are
    // numbers the player already sees. The generic sentence alone would leave
    // them to work out the gap themselves.
    const skill: CombatWindowSkill = { id: 'fire_bolt', version: 1, castable: false, reason_key: 'reason.insufficient_mana', mana_cost: 12 }
    const actor: CombatWindowSide = { actor_id: 'hero', name: '主角', side: 'player', exact: true, mana: 5, mana_max: 40, has_implement: true, standing: 'standing', route_exists: false }
    expect(skillRefusalLabel(skill, actor, t)).toBe('灵力不够：现有 5 / 40，这一招要 12。')
  })

  it('leaves a refusal with nothing to count to its own wording', () => {
    // The burden capacity behind a refused heavy move is derived from health and
    // never sent, so there is no denominator to print; the sentence stays as it
    // was rather than showing an invented one.
    const skill: CombatWindowSkill = { id: 'heavy', version: 1, castable: false, reason_key: 'reason.burden_capacity', burden_cost: 30 }
    const actor: CombatWindowSide = { actor_id: 'hero', name: '主角', side: 'player', exact: true, burden: 80, has_implement: true, standing: 'standing', route_exists: false }
    expect(skillRefusalLabel(skill, actor, t)).toBe('')
  })

  it('names a carried remedy from its own key, and says so plainly when it cannot', () => {
    const pill: CombatWindowItem = { instance_id: '回春丹', def_id: 'pill.restore.health.lower', name_key: 'item.pill.restore_health_lower.name', quantity: 1, purpose: 'restore_health', usable: true }
    expect(itemLabel(pill, t)).toBe('回春丹')
    const unworlded = itemLabel({ ...pill, name_key: 'item.no_such_pill.name' }, t)
    expect(unworlded).toBe('未识得的药物')
    expect(unworlded).not.toContain('pill.restore.health.lower')
    expect(itemLabel({ ...pill, name_key: undefined }, t)).toBe('未识得的药物')
  })
})

describe('standing and result labels', () => {
  it('keeps a yielded enemy distinct from a defeated one', () => {
    // §7 requires the aftermath to differ by how someone left the fight, so the
    // wording must not collapse them.
    expect(standingLabel('surrendered', t)).toBe('已投降')
    expect(standingLabel('standing', t)).toBe('standing')
  })

  it('words a settled result and passes an unknown one through', () => {
    expect(resultLabel('enemy_fled', t)).toBe('敌方撤离')
    expect(resultLabel('some_future_result', t)).toBe('some_future_result')
    expect(resultLabel(undefined, t)).toBe('')
  })
})

describe('technique and variant effects come from the published definition', () => {
  it('words each published property of a technique', () => {
    const skill: CombatWindowSkill = {
      id: 'meridian_secret_blade', version: 1, castable: true,
      effects: { execution_actions: 2, ignore_defense: 0.2, requires_focus: true, leaves_residual: true },
    }
    expect(skillEffectLabel(skill, t)).toBe('需先准备一回合；无视对方 20% 防御；需持法器施展；命中会留下未愈的伤')
  })

  it('reports a shield in the caster’s own terms', () => {
    const skill: CombatWindowSkill = {
      id: 'shield', version: 1, castable: true,
      effects: { shield_fraction: 0.2, duration_exchanges: 2 },
    }
    expect(skillEffectLabel(skill, t)).toBe('生成相当于生命上限 20% 的护盾，持续 2 轮')
  })

  it('says nothing for a technique whose only property is its power', () => {
    // A plain attack does nothing beyond damage, and a line saying "does nothing"
    // would be noise the player has to read past.
    expect(skillEffectLabel({ id: 'strike', version: 1, castable: true, effects: {} }, t)).toBe('')
    expect(skillEffectLabel({ id: 'strike', version: 1, castable: true }, t)).toBe('')
  })

  it('shows what a form trades and what it adds, not the power alone', () => {
    // A weaker form with no lasting effect would read as strictly worse, so the
    // effect has to be visible beside the power (§3 forbids hover-only text).
    expect(variantEffectLabel({ power: 1, effect: 'burn' }, 1.2, t)).toBe('威力 1.00（原 1.20） · 命中后灼烧两轮')
    expect(variantEffectLabel({ power: 1.45 }, 1.85, t)).toBe('威力 1.45（原 1.85）')
  })

  it('does not leak an effect identifier the locale does not word', () => {
    expect(variantEffectLabel({ power: 1, effect: 'some_future_effect' }, 1.2, t)).toBe('威力 1.00（原 1.20）')
  })
})
