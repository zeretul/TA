import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useHistoryLabels } from './presentation'
import type { HistoryState } from './types'

export function HistoryOpenEvents({state,onDraft}:{state:HistoryState;onDraft:(text:string)=>void}) {
 const {t}=useHistoryLabels(state)
 const [all,setAll]=useState(false),[query,setQuery]=useState('')
 const open=state.world!.events.filter(event=>event.status==='open')
 const matches=open.filter(event=>`${event.title} ${event.text} ${event.follow_up||''}`.toLocaleLowerCase().includes(query.toLocaleLowerCase()))
 const shown=all||query ? matches : matches.slice(-8).reverse()
 return <section className="history-manuscript"><h2>{t('history.world.currentEvents')} · {open.length}</h2>
 <Input aria-label={t('history.review.searchEvents')} placeholder={t('history.review.searchEvents')} value={query} onChange={event=>setQuery(event.target.value)} />
 {shown.map(event=><article className="history-event" key={event.id}><strong>{event.title}</strong><p>{event.text}</p>{event.follow_up&&<p className="history-condition">{event.follow_up}</p>}<small>{t('history.turn',{turn:event.turn+1})}</small><Button variant="ghost" size="sm" onClick={()=>onDraft(t('history.world.eventDraft',{title:event.title,text:event.text}))}>{t('history.world.handle')}</Button></article>)}
 {!matches.length&&<p className="history-muted">{t(open.length?'history.world.noMatches':'history.world.noEvents')}</p>}
 {!query&&open.length>8&&<Button variant="outline" onClick={()=>setAll(!all)}>{t(all?'history.review.collapseEvents':'history.review.allEvents',{count:open.length,hidden:open.length-8})}</Button>}
 </section>
}
