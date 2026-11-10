import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { beforeEach, expect, it, vi } from 'vitest'
import { HistoryGame } from './HistoryGame'
import { historyAPI } from './api'
import { setConfiguredLocale } from '@/i18n'
import type { HistoryState } from './types'

vi.mock('./api', () => ({ historyAPI: { bootstrap:vi.fn(),read:vi.fn(),create:vi.fn(),act:vi.fn(),cancel:vi.fn(),importScenario:vi.fn(),exportScenario:vi.fn() } }))
const api=vi.mocked(historyAPI)
function state():HistoryState{return {id:'world-test',name:'Independent campaign',scenario_id:'other-world',revision:3,year:1127,quarter:2,turn:0,treasury:0,privy_purse:0,grain:0,regions:[],ministers:[{id:'adviser',name:'李纲',name_en:'Li Gang',office:'宰相',office_en:'Chancellor',profile:'',domain:'court',capacity:0,knowledge:[]}],projects:[],edicts:[],chats:[],reports:[],checkpoints:[],inspections:[],scenario:{schema:1,id:'other-world',version:'1.0.0',name:{zh:'南宋',en:'Southern Song'},description:{zh:'南渡求存',en:'Survive and rebuild'},calendar:{name:{zh:'建炎',en:'Jianyan'},year:1127,quarter:2,era_offset:1126},metrics:[{id:'heart',name:{zh:'民心',en:'Support'},unit:{zh:'',en:''},min:0,max:100}],resources:[{id:'cash',name:{zh:'钱帛',en:'Cash'},unit:{zh:'贯',en:'guan'},max_transaction:1000}],objectives:[]},world:{entities:[{id:'zone',kind:'region',name:{zh:'两淮',en:'Huai frontier'},status:'active',metrics:{heart:45},attributes:{},audience:['public']}],accounts:[{id:'cash',name:{zh:'行朝钱库',en:'Court treasury'},resource:'cash',owner:'zone',balance:1500000,audience:['player']}],orders:[{id:'policy',version:1,edict_id:'decree',kind:'policy',text:'Maintain defenses within the authorized cap',minister_id:'adviser',targets:['zone'],depends_on:[],not_before_turn:0,limits:{cash:1000},spent:{cash:400},status:'active',progress:20,last_result:'Work continues',created_turn:0}],events:[],achieved:[]}}}

beforeEach(()=>{vi.resetAllMocks();localStorage.clear();sessionStorage.clear();setConfiguredLocale('en-US');const s=state();api.bootstrap.mockResolvedValue({project_id:'history-project',scenarios:[{id:'other-world',name:'Southern Song',description:'Test scenario'}],campaigns:[s]});api.read.mockResolvedValue(s)})

it('uses the bound scenario currency and routes a policy pause by its stable ID',async()=>{
  render(<HistoryGame active />)
  await screen.findByText('Court treasury')
  expect(screen.getByText('guan')).toBeVisible()
  expect(screen.queryByText('taels')).not.toBeInTheDocument()
  fireEvent.click(screen.getByRole('tab',{name:'Standing orders'}))
  const s=state();s.world!.orders[0].status='paused';s.revision++
  api.act.mockResolvedValue({command_id:'pause',revision:s.revision,kind:'edict',duplicate:false,state:s,outcomes:[]})
  fireEvent.click(screen.getByRole('button',{name:/^Pause$/}))
  await waitFor(()=>expect(api.act).toHaveBeenCalled())
  expect(api.act.mock.calls[0][3]).toMatchObject({expected_revision:3,plan:{orders:[{action:'pause',order_id:'policy'}]}})
  expect(await screen.findByRole('button',{name:/^Resume$/})).toBeVisible()
})

it('opens the saved report for a targeted follow-up without turning the report into an order',async()=>{
  const s=state();s.reports=[{id:'report-one',turn:0,year:1127,quarter:2,opening_treasury:0,income:0,upkeep:0,upkeep_arrears:0,reserved:0,spent:0,grain_reserved:0,grain_spent:0,closing_treasury:0,outcomes:[],events:[],narrative:'The frontier has held.'}];api.read.mockResolvedValue(s)
  render(<HistoryGame active />);await screen.findByText('Court treasury')
  fireEvent.click(screen.getByRole('tab',{name:'Seasonal reports'}));fireEvent.click(screen.getByRole('button',{name:'Discuss this report'}))
  expect(screen.getByRole('textbox',{name:'Address the court'})).toHaveValue('Discuss this report in light of what you now know: causes, risks, and next steps.')
  expect(api.act).not.toHaveBeenCalled()
  api.act.mockResolvedValue({command_id:'chat',revision:4,kind:'chat',duplicate:false,state:{...s,revision:4},outcomes:[]})
  fireEvent.click(screen.getByRole('button',{name:'Address ministers'}))
  await waitFor(()=>expect(api.act).toHaveBeenCalledTimes(1))
  expect(api.act.mock.calls[0][3]).toMatchObject({source_report_id:'report-one'})
  await waitFor(()=>expect(screen.queryByText('Season 1 report attached')).not.toBeInTheDocument())
})

it('keeps cancellation reachable inside the modal during a slow quarterly simulation', async () => {
  let requestSignal: AbortSignal | undefined
  api.act.mockImplementation((_project, _campaign, _action, _body, signal) => {
    requestSignal = signal
    return new Promise((_resolve, reject) => signal?.addEventListener('abort', () => reject(new DOMException('Cancelled', 'AbortError'))))
  })
  api.cancel.mockResolvedValue({ cancelled: true })
  render(<HistoryGame active />)
  await screen.findByText('Court treasury')
  fireEvent.click(screen.getByRole('button', { name: 'Advance a season' }))
  const dialog = screen.getByRole('dialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Conclude & simulate' }))
  const cancel = await within(dialog).findByRole('button', { name: 'Cancel simulation' })
  expect(cancel).toBeEnabled()
  fireEvent.click(cancel)
  await waitFor(() => expect(api.cancel).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  expect(requestSignal?.aborted).toBe(true)
  expect(api.read).toHaveBeenCalledTimes(2)
  expect(screen.getByRole('heading', { name: 'Independent campaign' })).toBeVisible()
  expect(screen.getByText('1,500,000')).toBeVisible()
})
