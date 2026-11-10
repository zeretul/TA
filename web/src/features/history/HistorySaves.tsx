import { useState } from 'react'
import { Save, RotateCcw, ArrowRightLeft, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import type { HistoryGameController } from './use-history-game'
import type { HistoryCheckpoint } from './types'
import { historyAPI } from './api'
import { useHistoryLabels } from './presentation'

export function HistoryNewGame({ game, open, onOpenChange }: { game: HistoryGameController; open: boolean; onOpenChange: (value: boolean) => void }) {
  const { t } = useHistoryLabels()
  const [name, setName] = useState('')
  const [scenario, setScenario] = useState('')
  const [packBusy, setPackBusy] = useState(false)
  const [packNotice, setPackNotice] = useState('')
  const selected=game.bootstrap?.scenarios.find(p=>p.id===scenario) || game.bootstrap?.scenarios[0]
  const importPack=async(file?:File)=>{if(!file || !game.bootstrap)return;setPackBusy(true);setPackNotice('');try{if(file.size>64*1024*1024)throw new Error('Scenario too large');const imported=await historyAPI.importScenarioPackage(game.bootstrap.project_id,file);await game.reload();setScenario(imported.id);setPackNotice('history.world.imported')}catch(error){console.warn('[history] Scenario import failed',error);setPackNotice('history.world.importError')}finally{setPackBusy(false)}}
  const exportPack=async()=>{if(!selected || !game.bootstrap)return;setPackBusy(true);try{const pack=await historyAPI.exportScenarioBundle(game.bootstrap.project_id,selected.id);const url=URL.createObjectURL(pack);const link=document.createElement('a');link.href=url;link.download=`${selected.id}.zip`;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}catch(error){console.warn('[history] Scenario export failed',error);setPackNotice('history.world.importError')}finally{setPackBusy(false)}}
  return <Dialog open={open} onOpenChange={value=>{if(!game.busy&&!packBusy)onOpenChange(value)}}><DialogContent className="max-h-[85dvh] overflow-y-auto"><DialogHeader><DialogTitle>{t('history.newGame')}</DialogTitle><DialogDescription>{t('history.newHint')}</DialogDescription></DialogHeader>
    <form onSubmit={event=>{event.preventDefault();void game.create(name.trim()||selected?.name||t('history.defaultName'),selected?.id).then(ok=>{if(ok){setName('');onOpenChange(false)}})}}>
      <label htmlFor="history-new-scenario" className="mb-2 block text-sm">{t('history.world.scenario')}</label><select id="history-new-scenario" className="history-world-select w-full" value={selected?.id||''} disabled={game.busy||packBusy} onChange={event=>setScenario(event.target.value)}>{game.bootstrap?.scenarios.map(pack=><option key={pack.id} value={pack.id}>{pack.name}</option>)}</select><p className="my-3 text-xs text-muted-foreground">{selected?.description}</p>
      <label htmlFor="history-new-name" className="mb-2 block text-sm">{t('history.name')}</label><Input id="history-new-name" value={name} onChange={event=>setName(event.target.value)} placeholder={t('history.namePlaceholder')} maxLength={100} disabled={game.busy}/><p className="mt-3 text-xs text-muted-foreground">{t('history.scenarioNote')}</p>
      <div className="history-actions"><label className="cursor-pointer text-sm underline">{t('history.world.import')}<input className="sr-only" aria-label={t('history.world.import')} type="file" accept=".zip,.json,application/zip,application/json" disabled={game.busy||packBusy} onChange={event=>{void importPack(event.target.files?.[0]);event.target.value=''}}/></label><Button type="button" variant="ghost" size="sm" disabled={game.busy||packBusy||!selected} onClick={()=>void exportPack()}>{t('history.world.export')}</Button></div>
      <p className="history-muted text-xs">{t('history.bundle.hint')}</p>
      {packNotice&&<p role="status" className="text-sm">{t(packNotice)}</p>}{game.failure&&<p role="alert" className="mt-3 text-sm text-destructive">{t(game.failure.key)}</p>}<div className="mt-5 flex justify-end"><Button type="submit" disabled={game.busy||packBusy||!selected}><Plus/>{t('history.create')}</Button></div>
    </form></DialogContent></Dialog>
}

export function HistorySaves({ game, open, onOpenChange }: { game: HistoryGameController; open: boolean; onOpenChange: (value: boolean) => void }) {
  const { t, season } = useHistoryLabels(game.state)
  const [label, setLabel] = useState('')
  const [deleteOpen,setDeleteOpen]=useState(false)
  const [restore, setRestore] = useState<HistoryCheckpoint | null>(null)
  const state = game.state
  const save = async () => {
    if (!state) return
    if (await game.act('checkpoints', { label: label.trim() || t('history.save.default', { season: season(state.year, state.quarter) }) })) setLabel('')
  }
  return <>
    <Dialog open={open} onOpenChange={(value) => { if (!game.busy) onOpenChange(value) }}><DialogContent className="max-h-[85dvh] overflow-y-auto"><DialogHeader><DialogTitle>{t('history.save.title')}</DialogTitle><DialogDescription>{t('history.autoSaved')}</DialogDescription></DialogHeader>
      {game.failure && <p role="alert" className="text-sm text-destructive">{t(game.failure.key)}</p>}<section><h3 className="mb-2 text-sm font-medium">{t('history.save.switch')}</h3><ul className="divide-y">{game.bootstrap?.campaigns.filter(campaign=>!campaign.archived).map((campaign) => <li key={campaign.id} className="flex min-w-0 items-center gap-3 py-3"><div className="min-w-0 flex-1"><p className="break-words font-medium">{campaign.name}</p><p className="mt-1 text-xs text-muted-foreground">{campaign.unavailable ? t('history.save.unavailable') : `${season(campaign.year, campaign.quarter)} · ${t('history.turn', { turn: campaign.turn + 1 })}`}</p></div><Button variant="outline" size="sm" disabled={game.busy || campaign.unavailable || campaign.id === state?.id} onClick={() => void game.select(campaign.id).then((ok) => { if (ok) onOpenChange(false) })}><ArrowRightLeft />{t(campaign.unavailable ? 'history.save.unavailable' : campaign.id === state?.id ? 'history.save.current' : 'history.save.switch')}</Button></li>)}</ul></section>
      {!!game.bootstrap?.campaigns.some(c=>c.archived) && <details className="history-details"><summary>{t('history.world.trash')}</summary>{game.bootstrap.campaigns.filter(c=>c.archived).map(c=><div className="history-actions" key={c.id}><span>{c.name}</span><Button variant="outline" size="sm" disabled={game.busy} onClick={()=>void game.recover(c.id)}>{t('history.world.recover')}</Button></div>)}</details>}
      {state && <Button variant="ghost" disabled={game.busy} onClick={()=>setDeleteOpen(true)}>{t('history.world.deleteCurrent')}</Button>}
      {state && <section className="border-t pt-4"><h3 className="mb-3 text-sm font-medium">{t('history.save.checkpoints')}</h3><form className="flex flex-wrap gap-2" onSubmit={(event) => { event.preventDefault(); void save() }}><label className="sr-only" htmlFor="history-checkpoint-name">{t('history.save.label')}</label><Input id="history-checkpoint-name" className="min-w-0 flex-1" value={label} onChange={(event) => setLabel(event.target.value)} placeholder={t('history.save.label')} maxLength={100} disabled={game.busy} /><Button type="submit" disabled={game.busy}><Save />{t('history.saveCheckpoint')}</Button></form>
        {!state.checkpoints?.length ? <p className="py-4 text-sm text-muted-foreground">{t('history.save.empty')}</p> : <ul className="mt-3 divide-y">{[...state.checkpoints].reverse().map((checkpoint) => <li key={checkpoint.id} className="flex min-w-0 items-center gap-3 py-3"><div className="min-w-0 flex-1"><p className="break-words">{checkpoint.label}</p><p className="text-xs text-muted-foreground">{t('history.turn', { turn: checkpoint.turn + 1 })}</p></div><Button variant="outline" size="sm" disabled={game.busy} onClick={() => setRestore(checkpoint)}><RotateCcw />{t('history.save.restore')}</Button></li>)}</ul>}
      </section>}
    </DialogContent></Dialog>
    <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}><DialogContent><DialogHeader><DialogTitle>{t('history.world.deleteTitle',{name:state?.name})}</DialogTitle><DialogDescription>{t('history.world.deleteHint')}</DialogDescription></DialogHeader><div className="history-actions"><Button variant="ghost" onClick={()=>setDeleteOpen(false)}>{t('history.back')}</Button><Button disabled={game.busy} onClick={()=>void game.act('archive').then(ok=>{if(ok)setDeleteOpen(false)})}>{t('history.world.deleteCurrent')}</Button></div></DialogContent></Dialog>
    <Dialog open={!!restore} onOpenChange={(value) => { if (!value && !game.busy) setRestore(null) }}><DialogContent><DialogHeader><DialogTitle>{t('history.save.restoreTitle', { label: restore?.label })}</DialogTitle><DialogDescription>{t('history.save.restoreHint')}</DialogDescription></DialogHeader>{game.failure && <p role="alert" className="text-sm text-destructive">{t(game.failure.key)}</p>}<div className="flex flex-wrap justify-end gap-2"><Button variant="ghost" disabled={game.busy} onClick={() => setRestore(null)}>{t('history.back')}</Button><Button disabled={game.busy} onClick={() => { if (restore) void game.act('restore', { checkpoint_id: restore.id }).then((ok) => { if (ok) { setRestore(null); onOpenChange(false) } }) }}>{t('history.save.restoreConfirm')}</Button></div></DialogContent></Dialog>
  </>
}
