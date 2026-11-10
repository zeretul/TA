import { useEffect, useRef, useState } from 'react'
import { MessageCircle, Send, ScrollText, LockKeyhole, Users } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useHistoryLabels } from './presentation'
import { HistoryPortrait } from './HistoryPortrait'
import { useHistoryDraft } from './use-history-draft'
import type { HistoryChat } from './types'
import type { HistoryGameController } from './use-history-game'

export function HistoryCourt({ game, onDraft, seed }: { game: HistoryGameController; seed?: {id:number;ministerId?:string;message:string;reportId?:string}; onDraft: (chat: HistoryChat) => void }) {
  const state = game.state!
  const { t, minister, office } = useHistoryLabels(state)
  const [mode, setMode] = useState<'private' | 'council'>('private')
  const [selected, setSelected] = useState<string[]>([state.ministers[0]?.id].filter(Boolean))
  const [message, setMessage] = useHistoryDraft(`${game.bootstrap!.project_id}:${state.id}`, 'court')
  const [search, setSearch] = useState('')
  const [visibleLimit,setVisibleLimit]=useState(160)
  const [sourceReport, setSourceReport] = useState<string>()
  useEffect(() => { if (seed) { setMode('private'); setSelected([seed.ministerId || state.ministers[0]?.id].filter(Boolean)); setMessage(seed.message); setSourceReport(seed.reportId) } }, [seed?.id])
  useEffect(() => { setSelected(current=>current.filter(id=>state.ministers.some(m=>m.id===id))) }, [state.ministers])
  const bottom = useRef<HTMLDivElement>(null)
  const visibleChats = (state.chats || []).filter((chat) => !chat.discarded && chat.minister_ids.length === selected.length && selected.every((id) => chat.minister_ids.includes(id)))
  useEffect(() => { bottom.current?.scrollIntoView({ block: 'nearest' }) }, [visibleChats.length])
  const filteredMinisters = state.ministers.filter(item => `${minister(item.id)} ${office(item.id)}`.toLowerCase().includes(search.toLowerCase()))
  const choose = (id: string) => setSelected((current) => mode === 'private' ? [id] : current.includes(id) ? current.filter((item) => item !== id) : current.length < 5 ? [...current, id] : current)
  const send = async () => {
    if (!message.trim() || selected.length === 0) return
    if (await game.act('chat', { minister_ids: selected, message: message.trim(), source_report_id: sourceReport })) { setMessage(''); setSourceReport(undefined) }
  }
  return <section className="history-court" aria-labelledby="history-court-heading">
    <div className="history-section-heading"><MessageCircle aria-hidden="true" /><h2 id="history-court-heading">{t('history.court.title')}</h2></div>
    <div className="history-audience-mode" role="group" aria-label={t('history.court.title')}>
      <Button variant={mode === 'private' ? 'secondary' : 'ghost'} disabled={game.busy} aria-pressed={mode === 'private'} onClick={() => { setMode('private'); setSelected((current) => current.slice(0, 1)) }}><LockKeyhole />{t('history.court.private')}</Button>
      <Button variant={mode === 'council' ? 'secondary' : 'ghost'} disabled={game.busy} aria-pressed={mode === 'council'} onClick={() => setMode('council')}><Users />{t('history.court.council')}</Button>
    </div>
    <input className="history-world-select" aria-label={t("history.world.searchMinister")} placeholder={t("history.world.searchMinister")} value={search} onChange={event=>{setSearch(event.target.value);setVisibleLimit(160)}} /><div className="history-ministers" role="group" aria-label={t('history.court.select')}>{filteredMinisters.slice(0,visibleLimit).map((item) => <button type="button" key={item.id} aria-pressed={selected.includes(item.id)} disabled={game.busy} className="history-minister" onClick={() => choose(item.id)}><HistoryPortrait state={state} id={item.id} name={minister(item.id)} /><span><strong>{minister(item.id)}</strong><small>{office(item.id)}</small></span><span className="history-selection-dot" aria-hidden="true" /></button>)}</div>
    {filteredMinisters.length>visibleLimit&&<Button variant="outline" onClick={()=>setVisibleLimit(visibleLimit+160)}>{t('history.bundle.moreMinisters',{shown:visibleLimit,total:filteredMinisters.length})}</Button>}
    <p className="history-muted history-audience-note">{t(mode === 'private' ? 'history.court.privateNote' : 'history.court.councilNote')}</p>
    <div className="history-conversation" role="log" aria-label={t('history.court.title')}>
      {visibleChats.length === 0 && <p className="history-empty-inline">{t('history.court.empty')}</p>}
      {visibleChats.map((chat) => <article className="history-exchange" key={chat.id}>
        <div className="history-chat-heading"><span>{t('history.turn', { turn: chat.turn + 1 })}</span><span>{t('history.court.audience', { names: chat.minister_ids.map(minister).join(' · ') })}</span></div>
        <div className="history-player-message"><strong>{t('history.court.speaker')}</strong><p className="history-prose">{chat.user_message}</p></div>
        {(chat.replies || []).map((reply, index) => <div className="history-minister-message" key={`${reply.minister_id}-${index}`}><HistoryPortrait state={state} id={reply.minister_id} name={minister(reply.minister_id)} /><strong>{minister(reply.minister_id)}</strong><p className="history-prose">{reply.text}</p>{state.world&&<Button size="sm" variant="ghost" disabled={game.busy||(state.advice||[]).some(item=>item.chat_id===chat.id&&item.minister_id===reply.minister_id)} onClick={()=>void game.act('advice',{advice:{action:'save',chat_id:chat.id,minister_id:reply.minister_id}})}>{t((state.advice||[]).some(item=>item.chat_id===chat.id&&item.minister_id===reply.minister_id)?'history.review.savedAdvice':'history.review.saveAdvice')}</Button>}</div>)}
        <Button variant="outline" size="sm" className="mt-3" disabled={game.busy} onClick={() => onDraft(chat)}><ScrollText />{t('history.court.draft')}</Button>
      </article>)}
      <div ref={bottom} />
    </div>
    <div className="history-chat-compose">{sourceReport && <div className="history-actions"><span className="history-muted">{t('history.world.reportAttached',{turn:(state.reports.find(r=>r.id===sourceReport)?.turn || 0)+1})}</span><Button variant="ghost" size="sm" disabled={game.busy} onClick={()=>setSourceReport(undefined)}>{t('history.world.detachReport')}</Button></div>}<label className="sr-only" htmlFor="history-court-message">{t('history.court.message')}</label><Textarea id="history-court-message" value={message} onChange={(event) => setMessage(event.target.value)} disabled={game.busy} placeholder={t('history.court.placeholder')} maxLength={12000} /><div className="history-actions"><Button className="history-primary" disabled={game.busy || !message.trim() || selected.length === 0} onClick={() => void send()}><Send />{t('history.court.send')}</Button>{selected.length === 0 && <span className="history-muted">{t('history.court.noSelection')}</span>}</div></div>
  </section>
}
