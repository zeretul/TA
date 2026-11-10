import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useHistoryLabels } from './presentation'
import type { HistoryGameController } from './use-history-game'
import type { SavedAdvice } from './types'

export function HistoryAdviceLibrary({game,draft}:{game:HistoryGameController;draft:string}) {
 const {t}=useHistoryLabels(game.state)
 const [query,setQuery]=useState('')
 const items=game.state!.advice||[]
 const matches=items.filter(item=>`${item.title} ${item.text} ${item.notes}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
 return <section className="history-advice-library" aria-label={t('history.review.library')}>
 <h4>{t('history.review.library')} · {items.length}</h4><p className="history-muted">{t('history.review.libraryHint')}</p>
 {!!items.length&&<Input aria-label={t('history.review.searchAdvice')} placeholder={t('history.review.searchAdvice')} value={query} onChange={event=>setQuery(event.target.value)} />}
 {!matches.length&&<p className="history-muted">{t(items.length?'history.world.noMatches':'history.review.emptyAdvice')}</p>}
 {[...matches].reverse().map(item=><AdviceCard key={item.id} item={item} game={game} draft={draft} />)}
 </section>
}
function AdviceCard({item,game,draft}:{item:SavedAdvice;game:HistoryGameController;draft:string}) {
 const {t,minister}=useHistoryLabels(game.state)
 const [editing,setEditing]=useState(false),[title,setTitle]=useState(item.title),[notes,setNotes]=useState(item.notes)
 return <details className="history-details" data-advice-id={item.id}><summary>{item.title&&`${item.title} · `}{minister(item.minister_id)} · {t('history.turn',{turn:item.turn+1})}</summary>
 <p className="history-prose">{item.text}</p>{item.notes&&<p className="history-condition">{item.notes}</p>}
 {editing?<div className="space-y-2"><label>{t('history.review.adviceTitle')}<Input value={title} maxLength={160} onChange={e=>setTitle(e.target.value)} /></label><label>{t('history.review.adviceNotes')}<Textarea value={notes} maxLength={2000} onChange={e=>setNotes(e.target.value)} /></label><Button disabled={game.busy} onClick={()=>void game.act('advice',{advice:{action:'update',id:item.id,title,notes}}).then(ok=>{if(ok)setEditing(false)})}>{t('history.review.saveNotes')}</Button></div>:<div className="history-actions">
 <Button size="sm" variant="outline" disabled={game.busy} onClick={()=>void game.act('prepare-edict',{source_advice_id:item.id,text:draft.trim()||t('history.court.draftInstruction')})}>{t('history.review.useAdvice')}</Button>
 <Button size="sm" variant="ghost" disabled={game.busy} onClick={()=>{setTitle(item.title);setNotes(item.notes);setEditing(true)}}>{t('history.review.organize')}</Button>
 <Button size="sm" variant="ghost" disabled={game.busy} onClick={()=>void game.act('advice',{advice:{action:'remove',id:item.id}})}>{t('history.review.removeAdvice')}</Button></div>}
 </details>
}
