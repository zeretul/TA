import { useWorldPresentation } from '../../world-presentation-context'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { EntryDetails } from '../entry-details/EntryDetails'
import { adventureItemPresentation, adventureSkillPresentation } from '../entry-details/presentations'
import type { AdventureItemDetails, AdventureSkillDetails } from '../entry-details/types'
import { APIError, createAgentCommandID, jsonHeaders, requestJSON } from '@/lib/api-client'

interface Battle { player_scale?: string; personal_combat_skill_id?: string; space_combat_energy_per_round?: number; player_retreat_progress?: number; enemy_retreating?: boolean; enemy_ceasefire_offer?: boolean; player_ceasefire_offer?: boolean; last_enemy_action?: string; last_enemy_skill_name?: string; can_surrender?: boolean; can_disrupt_electronics?: boolean; id: string; opponent_id?: string; scale: string; opponent_name: string; round: number; status: string; asset_id?: string }
interface Skill extends AdventureSkillDetails { mobility?: number; mobility_usable?: boolean; usable?: boolean; id: string; name: string; kind: string; scale: string; cost: number; power?: number; effects?: { kind: string; power: number }[]; backlash?: { health_percent: number; injury: number; foundation_skills?: string[]; foundation_rank?: number; mitigated_health_percent: number; mitigated_injury: number }; required_tool?: string; materials?: Record<string, number>; required_item?: string; rush?: boolean; weapon_style?: boolean; enemy_effects?: { kind: string; power: number }[] }
interface Item extends AdventureItemDetails { id: string; name: string; kind: string; effect?: string; scale: string; max_charges?: number }
interface AdventureWindow {
  progression?: { stage_name: string; rank: number; max_rank: number; experience: number; required_experience: number; next_stage_name?: string; next_stage_rank?: number; foundation_name?: string; required_mastery: number; experience_per_hour: number; currency_per_hour: number }
  head: string; clock_seconds?: number; family: 'wuxia' | 'space' | 'fantasy'; enemy_condition: string
  actor: { health: number; max_health: number; energy: number; max_energy: number; injury: number; poison?: number; control?: number; mental_corruption?: number; mental_shock?: number; mental_exposure_id?: string; item_charges?: Record<string, number>; item_ready_at?: Record<string, number>; equipment?: Record<string, string>; rush_blocked?: number; soft_limbs?: boolean; fog_area_id?: string; fog_treated?: boolean; skills: Record<string, number>; inventory: Record<string, number>; battle?: Battle; assets: Record<string, { health: number; max_health: number; shield: number; fuel: number; permanent_damage?: number; disabled_until?: number }> }
  skills: Skill[]; items: Item[]
}
interface Command { command_id: string; expected_head: string; branch: string; action: { action: 'battle_round'; choice: string; skill_id?: string; item_id?: string; target_id?: string } }
interface Props { storyId: string; branchId: string; active: boolean; onClose: () => void; onContinueStory?: (message: string) => void; windowActions?: ReactNode }

// This surface settles only a battle the main story has actually opened.
// Unknown requests retain their identity across closing/reopening the window.
export function AdventureCombatPanel({ storyId, branchId, active, onClose, onContinueStory, windowActions }: Props) {
  const { i18n } = useTranslation()
  const [data, setData] = useState<AdventureWindow | null>(null)
  const { t, terms } = useWorldPresentation({ family: data?.family })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [choice, setChoice] = useState('attack')
  const key = `ta:adventure-command:${storyId}:${branchId}`
  const pending = useRef<Command | null>(null)
  const scope = useRef(0)
  const [uncertain, setUncertain] = useState(false)
  const path = `/api/interactive/stories/${encodeURIComponent(storyId)}`
  const reload = useCallback(async () => {
    const epoch = scope.current
    try {
      const next = await requestJSON<AdventureWindow>(`${path}/adventure-window?branch=${encodeURIComponent(branchId)}`)
      if (epoch === scope.current) setData(next)
    } catch (err) {
      console.error('[adventure-window] Read failed', err)
      if (epoch === scope.current) setError('combatWindow.adventure.loadFailed')
    }
  }, [path, branchId, i18n.language])
  useEffect(() => {
    scope.current += 1
    setData(null); setError(''); setChoice('attack'); setBusy(false)
    try { pending.current = JSON.parse(localStorage.getItem(key) || 'null') as Command | null } catch { pending.current = null }
    setUncertain(Boolean(pending.current))
    if (active) void reload()
    return () => { scope.current += 1 }
  }, [key, active, reload])

  const play = async () => {
    if (!data || busy) return
    const epoch = scope.current
    const [kind, id, targetID] = choice.split(':')
    const command = pending.current || { command_id: createAgentCommandID(), expected_head: data.head, branch: branchId, action: { action: 'battle_round' as const, choice: kind, ...(['skill', 'escape', 'pursue'].includes(kind) && id ? { skill_id: id, ...(targetID ? { target_id: targetID } : {}) } : kind === 'item' ? { item_id: id } : {}) } }
    pending.current = command
    localStorage.setItem(key, JSON.stringify(command))
    setBusy(true); setError('')
    try {
      await requestJSON(`${path}/adventure-action`, { method: 'POST', headers: jsonHeaders, body: JSON.stringify(command) })
      localStorage.removeItem(key)
      if (epoch !== scope.current) return
      pending.current = null; setUncertain(false)
      setChoice('attack')
      await reload()
    } catch (err) {
      console.error('[adventure-window] Round failed', err)
      if (epoch !== scope.current) return
      if (err instanceof APIError && err.status >= 400 && err.status < 500) {
        pending.current = null; localStorage.removeItem(key); setUncertain(false); setError('combatWindow.adventure.rejected')
        await reload()
      } else { setUncertain(true); setError('combatWindow.adventure.uncertain') }
    } finally { if (epoch === scope.current) setBusy(false) }
  }
  const battle = data?.actor.battle
  const live = battle?.status === 'active'
  const asset = battle?.asset_id ? data?.actor.assets[battle.asset_id] : undefined
  const skills = data?.skills.filter((skill) => data.actor.skills[skill.id] > 0 && (skill.scale === (battle?.player_scale || battle?.scale) || (battle?.can_disrupt_electronics && skill.effects?.some(effect => effect.kind === 'electronic_disrupt'))) && ['attack', 'heal', 'energy', 'guard', 'control', 'poison', 'antidote', 'repair', 'pilot'].includes(skill.kind)) || []
  const items = data?.items.filter((item) => data.actor.inventory[item.id] > 0 && (item.kind === 'consumable' || (item.kind === 'tool' && item.effect === 'mental_restore'))) || []
  const selectedSkill = ['skill', 'escape', 'pursue'].includes(choice.split(':')[0]) && choice.includes(':') ? skills.find((skill) => choice.split(':')[1] === skill.id) : undefined
  const selectedItem = choice.startsWith('item:') ? items.find(item => choice.split(':')[1] === item.id) : undefined
  const rushBlocked = Boolean(data?.actor.rush_blocked || (data?.actor.fog_area_id && !data.actor.fog_treated))
  const soft = Boolean(data?.actor.soft_limbs)
  const mentalShock = Boolean(data?.actor.mental_shock)
  const clock = data?.clock_seconds || 0
  const shieldState = (item: Item) => {
    const ready = data?.actor.item_ready_at?.[item.id] || 0
    const stored = data?.actor.item_charges?.[item.id] ?? item.max_charges ?? 0
    return { seconds: Math.max(0, ready - clock), count: ready > 0 && ready <= clock && stored === 0 ? item.max_charges || 0 : stored }
  }
  const assetUnavailable = Boolean(asset && ((asset.disabled_until || 0) > clock || asset.fuel < 1 || asset.health < 1))
  const hasRequiredTool = (id: string) => (data?.actor.inventory[id] || 0) > 0
    || Boolean(data?.items.some(item => item.generated_from === id && data.actor.inventory[item.id] > 0))
  const skillUnavailable = (skill: Skill) => {
    if (skill.cost > (data?.actor.energy || 0)) return true
    if (skill.effects?.some(effect => effect.kind === 'electronic_disrupt') && !battle?.can_disrupt_electronics) return true
    if (skill.required_tool && !hasRequiredTool(skill.required_tool)) return true
    if (Object.entries(skill.materials || {}).some(([id, count]) => (data?.actor.inventory[id] || 0) < count)) return true
    if (skill.effects?.some(effect => effect.kind === 'shield_boost')) {
      const shield = data?.items.find(item => (item.id === skill.required_tool || item.generated_from === skill.required_tool) && item.effect === 'shield' && data.actor.equipment?.['person:armor'] === item.id)
      if (!shield || shieldState(shield).seconds > 0 || shieldState(shield).count < 1) return true
    }
    const recoveryEffects = skill.effects?.length ? skill.effects : [{ kind: skill.kind, power: skill.power || 0 }]
    const softRecovery = !skill.required_tool && !skill.weapon_style && !skill.rush && recoveryEffects.every(effect => ['heal', 'energy', 'antidote'].includes(effect.kind))
    const offensive = ['attack', 'control', 'poison'].includes(skill.kind) || skill.effects?.some(effect => ['attack', 'mental_attack', 'control', 'poison', 'seal', 'reflect', 'drain', 'electronic_disrupt'].includes(effect.kind))
    return (Boolean(skill.rush) && rushBlocked) || (mentalShock && offensive) || (soft && !softRecovery)
  }
  const movementChoice = ['escape', 'pursue'].includes(choice.split(':')[0])
  const selectedUnavailable = assetUnavailable ? !['escape', 'accept_ceasefire'].includes(choice) : (choice === 'surrender' ? !battle?.can_surrender : choice === 'accept_ceasefire' ? !battle?.enemy_ceasefire_offer : (soft && ['attack', 'guard', 'escape', 'pursue'].includes(choice.split(':')[0])) || (mentalShock && choice === 'attack') || Boolean(selectedSkill && (movementChoice ? selectedSkill.mobility_usable === false : selectedSkill.usable === false || skillUnavailable(selectedSkill))))
  const backlash = selectedSkill?.backlash
  const foundation = backlash?.foundation_skills?.some(id => (data?.actor.skills[id] || 0) >= (backlash.foundation_rank || 1))
  return <div className="flex h-full min-h-0 flex-col bg-background text-foreground" data-testid="adventure-combat">
    <header className="flex items-center gap-2 border-b px-3 py-2">
      <h2 className="min-w-0 flex-1 text-sm font-semibold">{terms.combat}</h2>
      {windowActions}
      <Button variant="ghost" size="icon-sm" onClick={onClose} aria-label={t('worldPresentation.closeCombat', terms)}><X className="size-4" /></Button>
    </header>
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-3">
      {error ? <div role="alert" className="space-y-2 text-xs"><p>{t(error)}</p><Button variant="outline" size="sm" onClick={() => { setError(''); void reload() }}>{t('combatWindow.retry')}</Button></div> : null}
      {!data ? <p className="text-sm text-muted-foreground">{t('combatWindow.loading')}</p> : <>
        {data.progression ? <section className="space-y-2 rounded-lg border p-3 text-xs" data-testid="adventure-progression">
          <p className="break-words text-sm font-semibold">{data.progression.stage_name} · {t('combatWindow.adventure.progression.rank', { rank: data.progression.rank, max: data.progression.max_rank })}</p>
          <p>{data.progression.required_experience > 0 ? t('combatWindow.adventure.progression.experience', { current: data.progression.experience, required: data.progression.required_experience }) : t('combatWindow.adventure.progression.ceiling')}</p>
          {data.progression.next_stage_name ? <p>{t('combatWindow.adventure.progression.next', { name: data.progression.next_stage_name, rank: data.progression.next_stage_rank })}</p> : null}
          {data.progression.required_experience > 0 ? <p className="break-words">{t('combatWindow.adventure.progression.refine', { name: data.progression.foundation_name || t('combatWindow.adventure.progression.anyFoundation'), mastery: data.progression.required_mastery, experience: data.progression.experience_per_hour, cost: data.progression.currency_per_hour })}</p> : null}
          <p className="text-muted-foreground">{t('combatWindow.adventure.progression.adaptation')}</p>
        </section> : null}
        <dl className="grid grid-cols-2 gap-3 text-xs">
          <div><dt>{t('combatWindow.adventure.health')}</dt><dd className="text-base tabular-nums">{data.actor.health} / {data.actor.max_health}</dd></div>
          <div><dt>{terms.energy}</dt><dd className="text-base tabular-nums">{data.actor.energy} / {data.actor.max_energy}</dd></div>
        </dl>
        {data.items.filter(item => item.effect === 'shield' && data.actor.equipment?.['person:armor'] === item.id).map(item => {
          const status = shieldState(item)
          return <p key={item.id} className="text-xs">{item.name} · {t(status.seconds > 0 ? 'combatWindow.adventure.shieldCooling' : 'combatWindow.adventure.shieldReady', { seconds: status.seconds, count: status.count })}</p>
        })}
        {asset ? <p className="text-xs">{t('combatWindow.adventure.asset', { health: asset.health, max: asset.max_health, shield: asset.shield, fuel: asset.fuel })}</p> : null}
        {data.actor.injury > 0 ? <p className="text-xs">{t('combatWindow.adventure.injury', { count: data.actor.injury })}</p> : null}
        {asset?.permanent_damage ? <p className="text-xs">{t('combatWindow.adventure.permanentWear', { count: asset.permanent_damage })}</p> : null}
        {live && assetUnavailable ? <p role="status" className="text-xs text-destructive">{t('combatWindow.adventure.assetUnavailable')}</p> : null}
        {asset && (asset.disabled_until || 0) > clock ? <p className="text-xs text-destructive">{t('combatWindow.adventure.powerLocked', { count: (asset.disabled_until || 0) - clock })}</p> : null}
        {data.actor.poison ? <p className="text-xs">{t('combatWindow.adventure.poison', { count: data.actor.poison })}</p> : null}
        {data.actor.mental_corruption ? <p className="text-xs">{t('combatWindow.adventure.mentalCorruption', { count: data.actor.mental_corruption })}</p> : null}
        {mentalShock ? <p className="text-xs">{t('combatWindow.adventure.mentalShock')}</p> : null}
        {soft ? <p className="text-xs">{t('combatWindow.adventure.softLimbs')}</p> : null}
        {rushBlocked ? <p className="text-xs">{t(data.actor.fog_area_id && !data.actor.fog_treated ? 'combatWindow.adventure.fogBlocked' : 'combatWindow.adventure.rushBlocked')}</p> : null}
        {data.actor.control ? <p className="text-xs">{t('combatWindow.adventure.control', { count: data.actor.control })}</p> : null}
        {!battle ? <p className="py-8 text-center text-sm text-muted-foreground">{t('combatWindow.adventure.empty')}</p> : <>
          <div className="rounded-lg border p-3">
            <p className="break-words text-sm font-semibold">{battle.opponent_name}</p>
            <p className="mt-1 text-xs text-muted-foreground">{t(`combatWindow.adventure.scale.${battle.scale}`)} · {t('combatWindow.adventure.round', { count: battle.round })}</p>
            <p className="mt-2 text-sm">{live ? t(`combatWindow.adventure.condition.${data.enemy_condition}`) : t(`combatWindow.adventure.status.${battle.status}`)}</p>
            {battle.player_scale && battle.player_scale !== battle.scale ? <p className="mt-2 text-xs">{t('combatWindow.adventure.mixedScale', { player: t(`combatWindow.adventure.scale.${battle.player_scale}`), opponent: t(`combatWindow.adventure.scale.${battle.scale}`) })}</p> : null}
            {battle.space_combat_energy_per_round ? <p className="mt-2 text-xs">{t('combatWindow.adventure.spaceUpkeep', { count: battle.space_combat_energy_per_round })}</p> : null}
            {battle.player_retreat_progress ? <p className="mt-2 text-xs">{t('combatWindow.adventure.retreatProgress', { count: battle.player_retreat_progress })}</p> : null}
            {battle.enemy_retreating ? <p className="mt-2 text-xs">{t('combatWindow.adventure.enemyRetreating')}</p> : null}
            {battle.enemy_ceasefire_offer ? <p className="mt-2 text-xs">{t('combatWindow.adventure.enemyCeasefire')}</p> : null}
            {battle.player_ceasefire_offer && !battle.enemy_ceasefire_offer ? <p className="mt-2 text-xs">{t('combatWindow.adventure.awaitCeasefire')}</p> : null}
            {battle.last_enemy_skill_name ? <p className="mt-2 break-words text-xs text-muted-foreground" data-testid="enemy-last-skill">{t('combatWindow.adventure.lastEnemySkill', { name: battle.last_enemy_skill_name })}</p> : null}
          </div>
          {live ? <fieldset disabled={busy || uncertain} className="space-y-2">
            <legend className="mb-2 text-xs font-medium">{t('combatWindow.adventure.choose')}</legend>
            <label className="block text-xs" htmlFor={`action-${storyId}`}>{t('combatWindow.adventure.action')}</label>
            <select id={`action-${storyId}`} value={choice} onChange={(event) => setChoice(event.target.value)} className="w-full min-w-0 rounded-md border bg-background px-2 py-2 text-sm">
              {['attack', 'guard', 'escape'].map((action) => <option disabled={assetUnavailable ? action !== 'escape' : soft || (mentalShock && action === 'attack')} value={action} key={action}>{t(assetUnavailable && action === 'escape' ? 'combatWindow.adventure.action.abandon' : `combatWindow.adventure.action.${action}`)}</option>)}
              <option value="pursue" disabled={assetUnavailable || soft}>{t('combatWindow.adventure.action.pursue')}</option>
              <option value="ceasefire" disabled={assetUnavailable}>{t('combatWindow.adventure.action.ceasefire')}</option>
              {battle.enemy_ceasefire_offer ? <option value="accept_ceasefire">{t('combatWindow.adventure.action.accept_ceasefire')}</option> : null}
              {skills.filter(skill => (skill.mobility || 0) > 0).flatMap(skill => ['escape', 'pursue'].map(action => <option key={`${action}:${skill.id}`} value={`${action}:${skill.id}`} disabled={assetUnavailable || soft || skill.mobility_usable === false}>{t(`combatWindow.adventure.action.${action}`)} · {skill.name} · {t('combatWindow.adventure.cost', { count: skill.cost })}</option>))}
              {battle.can_surrender ? <option value="surrender">{t('combatWindow.adventure.action.surrender')}</option> : null}
              {soft || data.enemy_condition === 'softened' ? <option value="talk" disabled={assetUnavailable}>{t('combatWindow.adventure.action.talk')}</option> : null}
              {data.actor.mental_corruption && data.actor.mental_exposure_id === battle.opponent_id ? <option value="listen" disabled={assetUnavailable}>{t('combatWindow.adventure.action.listen')}</option> : null}
              {skills.map((skill) => <option key={skill.id} value={`skill:${skill.id}`} disabled={assetUnavailable || skill.usable === false || skillUnavailable(skill)}>{skill.name} · {t('combatWindow.adventure.cost', { count: skill.cost })}</option>)}
              {skills.filter(skill => skill.enemy_effects?.length).map(skill => <option key={`${skill.id}-opponent`} value={`skill:${skill.id}:opponent`} disabled={assetUnavailable || soft || mentalShock || skill.usable === false || skillUnavailable(skill)}>{skill.name} · {t('combatWindow.adventure.target.opponent')}</option>)}
              {items.map((item) => <option key={item.id} value={`item:${item.id}`} disabled={assetUnavailable || ((soft || mentalShock) && ['attack', 'poison', 'soften'].includes(item.effect || '')) || (item.effect === 'mental_restore' && (!data.actor.mental_corruption || (data.actor.item_charges?.[item.id] ?? item.max_charges ?? 0) < 1))}>{item.name} × {data.actor.inventory[item.id]}{item.max_charges ? ` · ${t('combatWindow.adventure.charges', { count: data.actor.item_charges?.[item.id] ?? item.max_charges })}` : ''}</option>)}
            </select>
            {selectedSkill ? <EntryDetails key={`${data.head}:${selectedSkill.id}`} entry={adventureSkillPresentation(selectedSkill, data, t)} compact combat /> : null}
            {selectedItem ? <EntryDetails key={`${data.head}:${selectedItem.id}`} entry={adventureItemPresentation(selectedItem, data, t)} compact combat /> : null}
            {selectedSkill && movementChoice ? <p className="text-xs text-muted-foreground">{t('combatWindow.adventure.movementOnly')}</p> : null}
            {selectedSkill && !movementChoice ? <p className="text-xs text-muted-foreground">{(choice.endsWith(':opponent') && selectedSkill.enemy_effects?.length ? selectedSkill.enemy_effects : selectedSkill.effects?.length ? selectedSkill.effects : [{ kind: selectedSkill.kind, power: selectedSkill.power || 0 }]).map((effect) => t(`combatWindow.adventure.effect.${effect.kind}`, { count: effect.power })).join(' · ')}</p> : null}
            {backlash ? <p className="text-xs text-destructive">{t('combatWindow.adventure.backlash', { health: Math.max(1, Math.floor(data.actor.max_health * (foundation ? backlash.mitigated_health_percent : backlash.health_percent) / 100)), injury: foundation ? backlash.mitigated_injury : backlash.injury })}</p> : null}
          </fieldset> : null}
        </>}
      </>}
    </div>
    <footer className="space-y-2 border-t p-3">
      <p className="text-xs text-muted-foreground">{t('combatWindow.adventure.closeHint')}</p>
      {live || uncertain ? <Button className="w-full" disabled={busy || !data || (!uncertain && selectedUnavailable)} onClick={() => void play()}>{t(busy ? 'combatWindow.adventure.settling' : uncertain ? 'combatWindow.adventure.retrySame' : 'combatWindow.adventure.confirm')}</Button> : battle && onContinueStory ? <Button className="w-full" onClick={() => onContinueStory(t('combatWindow.adventure.continuePrompt'))}>{t('combatWindow.adventure.continue')}</Button> : null}
    </footer>
  </div>
}
