import { expect, it } from 'vitest'
import { reportAccounts } from './report-accounts'
import type { HistoryState } from './types'

it('reconstructs an earlier season and counts an internal transfer once without using the latest balance', () => {
  const account = { id: 'granary', resource: 'grain', owner: 'state', balance: 5000, name: { zh: '粮仓', en: 'Granary' }, audience: ['player'] }
  const reserve = { ...account, id: 'reserve', balance: 0 }
  const state = {
    scenario: { initial: { accounts: [account] } },
    world: { accounts: [{ ...account, balance: 3000 }] },
    reports: [
      { id: 'first', world: { new_accounts: [reserve], transfers: [{ from: 'granary', to: 'reserve', amount: 1000 }, { from: 'granary', to: '', amount: 200 }] } },
      { id: 'second', world: { transfers: [{ from: 'granary', to: '', amount: 800 }] } },
    ],
  } as unknown as HistoryState
  const first = reportAccounts(state, 'first')
  expect(first.map(a => [a.id, a.opening, a.change, a.balance])).toEqual([['granary', 5000, -1200, 3800], ['reserve', 0, 1000, 1000]])
  expect(first.reduce((sum, account) => sum + account.balance, 0)).toBe(4800)
  expect(reportAccounts(state, 'second').find(a => a.id === 'granary')?.balance).toBe(3000)
})

it('does not invent zero openings for accounts missing from the visible evidence', () => {
  const state = { scenario: {}, world: { accounts: [] }, reports: [{ id: 'first', world: { transfers: [{ from: '', to: 'unknown', amount: 500 }] } }] } as unknown as HistoryState
  expect(reportAccounts(state, 'first')).toEqual([])
})

it('uses the declared player realm rather than the first visible account for unchanged balances',()=>{
 const account=(id:string,owner:string)=>({id,owner,balance:100,name:{zh:id,en:id},resource:'silver',audience:['player']})
 const state={scenario:{player_entity_id:'realm',initial:{accounts:[account('private','person'),account('treasury','realm')]}},world:{accounts:[account('private','person'),account('treasury','realm')]},reports:[{id:'r',world:{transfers:[]}}]} as unknown as HistoryState
 expect(reportAccounts(state,'r').map(a=>a.id)).toEqual(['treasury'])
})
