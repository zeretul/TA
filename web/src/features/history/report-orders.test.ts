import { expect,it } from 'vitest'
import { reportOrderText } from './report-orders'
import type {HistoryState} from './types'
it('uses the wording in effect in the report season, including same-season revisions',()=>{
 const state={edicts:[
 {turn:0,plan:{orders:[{action:'create',text:'Shaanxi 100'}]},outcomes:[{project_id:'a'}]},
 {turn:1,plan:{orders:[{action:'revise',order_id:'a',text:'Henan 1000'}]},outcomes:[]},
 {turn:1,plan:{orders:[{action:'revise',order_id:'a',text:'Henan 800'}]},outcomes:[]},
 {turn:2,plan:{orders:[{action:'revoke',order_id:'a'}]},outcomes:[]},
 ]} as unknown as HistoryState
 expect(reportOrderText(state,0,'a')).toBe('Shaanxi 100')
 expect(reportOrderText(state,1,'a')).toBe('Henan 800')
 expect(reportOrderText(state,2,'a')).toBe('Henan 800')
 expect(reportOrderText(state,0,'missing')).toBeUndefined()
})
