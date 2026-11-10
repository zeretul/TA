import { act, fireEvent, render, renderHook, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { APIError } from '@/lib/api-client'
import { setConfiguredLocale } from '@/i18n'
import { HistoryGame } from './HistoryGame'
import { useHistoryGame } from './use-history-game'
import { historyAPI } from './api'
import type { HistoryState, HistoryReceipt, PreparedEdict } from './types'

vi.mock('./api', () => ({ historyAPI: { bootstrap: vi.fn(), list: vi.fn(), read: vi.fn(), create: vi.fn(), act: vi.fn(), cancel: vi.fn() } }))
const api = vi.mocked(historyAPI)
const initial = (): HistoryState => ({
  id:'reign-one',name:'Test reign',scenario_id:'ming-1627-slice',revision:1,year:1627,quarter:3,turn:0,
  treasury:1000000,privy_purse:1500000,grain:630000,
  regions:[{id:'shaanxi',name:'陕西',heart:30,tax_base:200000,disaster:40,refugees:10000,irrigation:10,training:15}],
  ministers:[{id:'bi-ziyan',name:'毕自严',office:'户部',profile:'财政',domain:'finance',capacity:100000,knowledge:['finance']},{id:'sun-chengzong',name:'孙承宗',office:'军务',profile:'军务',domain:'military',capacity:100000,knowledge:['military']}],
  projects:[],edicts:[],chats:[],reports:[],checkpoints:[],inspections:[],
})
function receipt(state: HistoryState, kind = 'chat'): HistoryReceipt { return {command_id:'server-command',revision:state.revision,kind,duplicate:false,state,outcomes:[]} }
function bootstrap(state = initial()) {
  api.bootstrap.mockResolvedValue({project_id:'history-project',scenarios:[{id:'ming-1627-slice',name:'Ming',description:'test'}],campaigns:[state]})
  api.read.mockResolvedValue(state)
}
function deferred<T>() { let resolve!: (value:T)=>void; let reject!: (reason:unknown)=>void; const promise = new Promise<T>((res,rej)=>{resolve=res;reject=rej}); return {promise,resolve,reject} }

beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear(); sessionStorage.clear(); setConfiguredLocale('en-US')
  bootstrap()
})

describe('history operations', () => {
  it('refreshes translated scenario summaries without reopening the campaign or discarding a prepared edict', async () => {
    setConfiguredLocale('zh-CN')
    api.bootstrap.mockResolvedValueOnce({ project_id: 'history-project', scenarios: [{ id: 'ming-1627-slice', name: '明末', description: '中文剧本说明' }], campaigns: [initial()] })
    const { result } = renderHook(() => useHistoryGame(true))
    await waitFor(() => expect(result.current.state?.id).toBe('reign-one'))
    const prepared: PreparedEdict = { expected_revision: 1, plan: { text: 'Preserve this reviewed draft', instructions: [] } }
    api.act.mockResolvedValueOnce(prepared)
    await act(async () => { await result.current.act('prepare-edict', { text: prepared.plan.text }) })
    const selectedState = result.current.state
    api.bootstrap.mockResolvedValueOnce({ project_id: 'history-project', scenarios: [{ id: 'ming-1627-slice', name: 'Late Ming', description: 'English scenario description' }], campaigns: [] })
    await act(async () => { setConfiguredLocale('en-US') })
    await waitFor(() => expect(result.current.bootstrap?.scenarios[0].name).toBe('Late Ming'))
    expect(result.current.bootstrap?.scenarios[0].description).toBe('English scenario description')
    expect(result.current.state).toBe(selectedState)
    expect(result.current.prepared).toBe(prepared)
    expect(result.current.bootstrap?.campaigns).toHaveLength(1)
    expect(api.read).toHaveBeenCalledTimes(1)
    expect(api.act).toHaveBeenCalledTimes(1)
  })

  it('ignores an older catalog response after another language change', async () => {
    const { result } = renderHook(() => useHistoryGame(true))
    await waitFor(() => expect(result.current.state?.id).toBe('reign-one'))
    const oldCatalog = deferred<Awaited<ReturnType<typeof historyAPI.bootstrap>>>()
    api.bootstrap.mockReturnValueOnce(oldCatalog.promise)
    await act(async () => { setConfiguredLocale('zh-CN') })
    await waitFor(() => expect(api.bootstrap).toHaveBeenCalledTimes(2))
    api.bootstrap.mockResolvedValueOnce({ project_id: 'history-project', scenarios: [{ id: 'ming-1627-slice', name: 'English latest', description: 'Current' }], campaigns: [] })
    await act(async () => { setConfiguredLocale('en-US') })
    await waitFor(() => expect(result.current.bootstrap?.scenarios[0].name).toBe('English latest'))
    await act(async () => { oldCatalog.resolve({ project_id: 'history-project', scenarios: [{ id: 'ming-1627-slice', name: '迟到中文', description: 'Old' }], campaigns: [] }) })
    expect(result.current.bootstrap?.scenarios[0].name).toBe('English latest')
    expect(api.read).toHaveBeenCalledTimes(1)
  })

  it('keeps the same command and revision when retrying an uncertain write', async () => {
    const {result} = renderHook(()=>useHistoryGame(true))
    await waitFor(()=>expect(result.current.state?.id).toBe('reign-one'))
    api.act.mockRejectedValueOnce(new APIError('lost response',{status:0,code:'client.network_error'}))
    const next={...initial(),revision:2}
    api.act.mockResolvedValueOnce(receipt(next))
    await act(async()=>{ await result.current.act('chat',{message:'Remember the relief promise',minister_ids:['bi-ziyan']}) })
    expect(result.current.failure).not.toBeNull()
    const original=api.act.mock.calls[0][3]
    await act(async()=>{ await result.current.retry() })
    expect(api.act.mock.calls[1][3]).toEqual(original)
    expect(original).toMatchObject({expected_revision:1,message:'Remember the relief promise'})
    expect(original).toHaveProperty('command_id')
    expect(result.current.state?.revision).toBe(2)
  })

  it('prevents a double click from starting a second operation', async()=>{
    const {result}=renderHook(()=>useHistoryGame(true))
    await waitFor(()=>expect(result.current.state).not.toBeNull())
    const pending=deferred<HistoryReceipt>()
    api.act.mockReturnValue(pending.promise)
    let running:Promise<boolean>
    await act(async()=>{ running=result.current.act('advance'); expect(await result.current.act('advance')).toBe(false) })
    expect(api.act).toHaveBeenCalledTimes(1)
    await act(async()=>{ pending.resolve(receipt({...initial(),revision:2,turn:1},'advance')); await running! })
    expect(result.current.state?.turn).toBe(1)
  })

  it('calls server cancellation before abort and rereads a possibly committed result', async()=>{
    const {result}=renderHook(()=>useHistoryGame(true))
    await waitFor(()=>expect(result.current.state).not.toBeNull())
    let signal:AbortSignal|undefined
    api.act.mockImplementation((_p,_c,_k,_body,requestSignal)=>{
      signal=requestSignal
      return new Promise((_resolve,reject)=>requestSignal?.addEventListener('abort',()=>reject(new DOMException('cancelled','AbortError'))))
    })
    api.cancel.mockImplementation(async()=>{expect(signal?.aborted).toBe(false);return {cancelled:false}})
    api.read.mockResolvedValue({...initial(),revision:2,turn:1,quarter:4})
    let running:Promise<boolean>
    await act(async()=>{running=result.current.act('advance')})
    await act(async()=>{await result.current.cancel();await running!})
    expect(api.cancel).toHaveBeenCalledWith('history-project','reign-one',(api.act.mock.calls[0][3] as Record<string,unknown>).command_id)
    expect(signal?.aborted).toBe(true)
    expect(result.current.state?.turn).toBe(1)
    expect(result.current.pending).toBeNull()
  })

  it('refreshes a stale campaign and discards its prepared plan without blindly replaying', async()=>{
    const {result}=renderHook(()=>useHistoryGame(true))
    await waitFor(()=>expect(result.current.state).not.toBeNull())
    const plan:PreparedEdict={expected_revision:1,plan:{text:'Relieve Shaanxi',instructions:[{id:'relief',kind:'relief',target:'shaanxi',budget:100000,grain:20000,quarters:1,minister_id:'bi-ziyan'}]}}
    api.act.mockResolvedValueOnce(plan)
    await act(async()=>{await result.current.act('prepare-edict',{text:plan.plan.text})})
    expect(result.current.prepared).not.toBeNull()
    api.act.mockRejectedValueOnce(new APIError('stale',{status:409,code:'api.history.conflict'}))
    api.read.mockResolvedValue({...initial(),revision:4,treasury:820000})
    await act(async()=>{await result.current.act('edict',{plan:plan.plan,expected_revision:plan.expected_revision})})
    expect(result.current.prepared).toBeNull()
    expect(result.current.state?.treasury).toBe(820000)
    expect(result.current.failure?.retry).toBeUndefined()
  })
})

describe('historical game flow',()=>{
  it('keeps the edict unchanged on model failure and separates review from issuing',async()=>{
    render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    const input=screen.getByRole('textbox',{name:'Your edict'})
    fireEvent.change(input,{target:{value:'Relief first, then build a canal; also dispatch an ocean expedition.'}})
    api.act.mockRejectedValueOnce(new APIError('model unavailable',{status:503,code:'api.history.modelFailed'}))
    fireEvent.click(screen.getByRole('button',{name:'Draft execution plan'}))
    await screen.findByRole('alert')
    expect(input).toHaveValue('Relief first, then build a canal; also dispatch an ocean expedition.')
    const prepared:PreparedEdict={expected_revision:1,plan:{text:'Relief first, then build a canal; also dispatch an ocean expedition.',instructions:[{id:'relief',kind:'relief',target:'shaanxi',minister_id:'bi-ziyan',budget:100000,grain:20000,quarters:1},{id:'canal',kind:'construction',target:'shaanxi',minister_id:'bi-ziyan',budget:150000,quarters:2,depends_on:['relief']},{id:'expedition',kind:'unsupported',reason:'An ocean expedition is not yet available.'}]},outcomes:[{instruction_id:'relief',status:'ready',reason:'validated'},{instruction_id:'canal',status:'ready',reason:'validated'},{instruction_id:'expedition',status:'unsupported',reason:'An ocean expedition is not yet available.'}]}
    api.act.mockResolvedValueOnce(prepared)
    fireEvent.click(screen.getByRole('button',{name:'Retry this operation'}))
    await screen.findByRole('heading',{name:'Awaiting your seal'})
    expect(screen.getByText('After: 1. Relief')).toBeVisible()
    expect(screen.getByText('Will not execute')).toBeVisible()
    expect(screen.getByText('An ocean expedition is not yet available.')).toBeVisible()
    expect(api.act.mock.calls.map((call)=>call[2])).toEqual(['prepare-edict','prepare-edict'])
    expect(api.act.mock.calls[1][3]).toEqual(api.act.mock.calls[0][3])
    api.act.mockResolvedValueOnce(receipt({...initial(),revision:2,edicts:[{id:'edict-one',turn:0,plan:prepared.plan,outcomes:[{instruction_id:'expedition',status:'unsupported',reason:'unsupported_instruction_kind'}]}]},'edict'))
    fireEvent.click(screen.getByRole('button',{name:'Issue this edict'}))
    await waitFor(()=>expect(screen.getByRole('textbox',{name:'Your edict'})).toHaveValue(''))
    expect(api.act.mock.calls[2][2]).toBe('edict')
    expect(api.act.mock.calls[2][3]).toMatchObject({expected_revision:1,plan:prepared.plan})
  })

  it('shows validation failures even when the AI used a supported instruction kind',async()=>{
    render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    fireEvent.change(screen.getByRole('textbox',{name:'Your edict'}),{target:{value:'Build in a missing region'}})
    api.act.mockResolvedValueOnce({expected_revision:1,plan:{text:'Build in a missing region',instructions:[{id:'bad',kind:'construction',target:'unknown',budget:100}]},outcomes:[{instruction_id:'bad',status:'unsupported',reason:'unknown_region'}]})
    fireEvent.click(screen.getByRole('button',{name:'Draft execution plan'}))
    expect(await screen.findByText('Needs discussion · This region is not part of the current game.')).toBeVisible()
  })

  it('keeps abandoned branch dialogue and reports out of the current timeline',async()=>{
    const state=initial()
    state.chats=[{id:'old',turn:5,mode:'private',minister_ids:['bi-ziyan'],user_message:'abandoned-future-secret',replies:[{minister_id:'bi-ziyan',text:'abandoned-future-reply'}],revision:8,discarded:true},{id:'current',turn:0,mode:'private',minister_ids:['bi-ziyan'],user_message:'current-private-message',replies:[{minister_id:'bi-ziyan',text:'current-reply'}],revision:2}]
    state.reports=[{id:'old-report',turn:4,year:1628,quarter:3,opening_treasury:1,income:1,upkeep:1,upkeep_arrears:0,reserved:0,spent:0,grain_reserved:0,grain_spent:0,closing_treasury:1,outcomes:[],events:[],narrative:'abandoned-season-report',discarded:true}]
    state.inspections=[{id:'old-inspection',target:'treasury',turn:4,facts:{treasury:123456789},discarded:true}]
    bootstrap(state)
    render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    fireEvent.click(screen.getByRole('tab',{name:'Audience & council'}))
    expect(screen.getByText('current-private-message')).toBeVisible()
    expect(screen.queryByText('abandoned-future-secret')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab',{name:'Chronicle'}))
    expect(screen.queryByText('abandoned-season-report')).not.toBeInTheDocument()
    expect(screen.queryByText('123,456,789')).not.toBeInTheDocument()
  })

  it('keeps an unreadable save visible while opening a healthy game', async()=>{
    const good=initial()
    const bad={...good,id:'broken-reign',name:'Old damaged reign',unavailable:true,error_code:'history.campaign_unreadable'}
    api.bootstrap.mockResolvedValue({project_id:'history-project',scenarios:[],campaigns:[bad,good]})
    localStorage.setItem('ta:history:history-project:campaign','broken-reign')
    render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    expect(api.read).toHaveBeenCalledWith('history-project','reign-one',expect.any(AbortSignal))
    expect(screen.getByText('A save could not be read and remains listed. Other saves and new games are still available.')).toBeVisible()
    fireEvent.click(screen.getAllByRole('button',{name:'Saves & checkpoints'})[0])
    const dialog=screen.getByRole('dialog',{name:'Historical saves'})
    expect(within(dialog).getByText('Old damaged reign')).toBeVisible()
    expect(within(dialog).getByRole('button',{name:'Unreadable'})).toBeDisabled()
  })

  it('drafts from exactly the selected saved discussion and keeps the manual draft on failure and issuance',async()=>{
    const state=initial()
    state.chats=[
      {id:'selected-discussion',turn:0,mode:'private',minister_ids:['bi-ziyan'],user_message:'How should we relieve Shaanxi?',replies:[{minister_id:'bi-ziyan',text:'I propose 40000 taels and 8000 shi over one season.'}],revision:2},
      {id:'other-discussion',turn:0,mode:'private',minister_ids:['bi-ziyan'],user_message:'Discuss a separate training proposal.',replies:[{minister_id:'bi-ziyan',text:'This unrelated proposal must not become the drafting source.'}],revision:3},
    ]
    state.revision=3;bootstrap(state)
    render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    fireEvent.change(screen.getByRole('textbox',{name:'Your edict'}),{target:{value:'My separate handwritten draft remains unfinished.'}})
    fireEvent.click(screen.getByRole('tab',{name:'Audience & council'}))
    const exchange=screen.getByText('How should we relieve Shaanxi?').closest('article')!
    api.act.mockRejectedValueOnce(new APIError('model unavailable',{status:503,code:'api.history.modelFailed'}))
    fireEvent.click(within(exchange).getByRole('button',{name:'Draft from this discussion'}))
    await screen.findByRole('alert')
    expect(api.act.mock.calls[0][2]).toBe('prepare-edict')
    expect(api.act.mock.calls[0][3]).toMatchObject({source_chat_id:'selected-discussion',expected_revision:3})
    expect(api.act.mock.calls[0][3]).not.toHaveProperty('replies')
    expect(screen.getByRole('textbox',{name:'Your edict'})).toHaveValue('My separate handwritten draft remains unfinished.')
    const plan:PreparedEdict={expected_revision:3,plan:{text:'Draft the selected discussion for approval.',source_chat_id:'selected-discussion',instructions:[{id:'relief',kind:'relief',target:'shaanxi',minister_id:'bi-ziyan',budget:40000,grain:8000,quarters:1}]},outcomes:[{instruction_id:'relief',status:'ready',reason:'validated'}]}
    api.act.mockResolvedValueOnce(plan)
    fireEvent.click(screen.getByRole('button',{name:'Retry this operation'}))
    const review=await screen.findByRole('region',{name:'Awaiting your seal'})
    expect(within(review).getByText('Source discussion · Season 1 · Bi Ziyan')).toBeInTheDocument()
    expect(within(review).getByText('I propose 40000 taels and 8000 shi over one season.')).toBeInTheDocument()
    expect(within(review).queryByText('This unrelated proposal must not become the drafting source.')).not.toBeInTheDocument()
    expect(within(review).getByText('Your earlier unissued draft is preserved. You are reviewing a separate plan drafted from this discussion.')).toBeVisible()
    expect(api.act.mock.calls[1][3]).toEqual(api.act.mock.calls[0][3])
    expect(api.act.mock.calls.map(call=>call[2])).toEqual(['prepare-edict','prepare-edict'])
    api.act.mockResolvedValueOnce(receipt({...state,revision:4,edicts:[{id:'source-edict',turn:0,plan:plan.plan,outcomes:[]}]},'edict'))
    fireEvent.click(screen.getByRole('button',{name:'Issue this edict'}))
    await waitFor(()=>expect(screen.getByRole('textbox',{name:'Your edict'})).toHaveValue('My separate handwritten draft remains unfinished.'))
    expect(api.act.mock.calls[2][3]).toMatchObject({plan:{source_chat_id:'selected-discussion'}})
  })

  it('requires explicit checkpoint restoration and passes the selected checkpoint identity',async()=>{
    const state=initial();state.checkpoints=[{id:'checkpoint-one',label:'Before the relief edict',turn:0,revision:1}]
    bootstrap(state);render(<HistoryGame active />)
    await screen.findByRole('heading',{name:'Test reign'})
    fireEvent.click(screen.getByRole('button',{name:'Saves & checkpoints'}))
    fireEvent.click(screen.getByRole('button',{name:'Return here'}))
    const dialog=screen.getByRole('dialog',{name:'Return to “Before the relief edict”?'})
    expect(api.act).not.toHaveBeenCalled()
    api.act.mockResolvedValueOnce(receipt({...state,revision:3},'restore'))
    fireEvent.click(within(dialog).getByRole('button',{name:'Restore checkpoint'}))
    await waitFor(()=>expect(api.act).toHaveBeenCalledTimes(1))
    expect(api.act.mock.calls[0][3]).toMatchObject({checkpoint_id:'checkpoint-one',expected_revision:1})
  })
})
