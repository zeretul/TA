import type { TFunction } from 'i18next'
import type { CombatWindowEquipment, CombatWindowItem, CombatWindowSkill, CombatWindowSpare } from '../../types'
import { itemLabel, skillLabel } from '../combat-window/combat-format'
import { cultivationPresentation } from './presentations'

/** These adapt only the server's player-side battle projection, never enemy
 * observation or a definition looked up by the browser. */
export function combatSkillPresentation(skill: CombatWindowSkill, t: TFunction) {
  return cultivationPresentation({ ...skill, name: skillLabel(skill, t), definition: {
    power: skill.power, targets: skill.targets, ...skill.effects,
    shield_base_health_fraction: skill.effects?.shield_fraction,
  } }, 'skills', t)
}
export function combatEquipmentPresentation(item: CombatWindowEquipment | CombatWindowSpare, t: TFunction) {
  return cultivationPresentation({ ...item, id: item.instance_id || '', name: item.name || t('entryDetails.unknownItem'), quantity: 1 }, 'equipment', t)
}
export function combatItemPresentation(item: CombatWindowItem, t: TFunction) {
  return cultivationPresentation({ ...item, id: item.instance_id, name: itemLabel(item, t), definition: {
    purpose: item.purpose, restore_amount: item.restore_amount, shield_amount: item.shield_amount,
    attack_bonus: item.attack_bonus, duration_exchanges: item.duration_exchanges,
  } }, 'inventory', t)
}
