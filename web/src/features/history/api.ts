import { fetchAPI, responseAPIError, jsonHeaders, projectAPIPath, requestJSON } from '@/lib/api-client'
import type { HistoryAction, HistoryBootstrap, HistoryReceipt, HistoryState, HistorySummary, PreparedEdict } from './types'

function campaignPath(projectId: string, campaignId: string, suffix = '') {
  return projectAPIPath(projectId, `history/campaigns/${encodeURIComponent(campaignId)}${suffix ? `/${suffix}` : ''}`)
}
function post<T>(url: string, body: object, signal?: AbortSignal) {
  return requestJSON<T>(url, { method: 'POST', headers: jsonHeaders, body: JSON.stringify(body), signal })
}
export const historyAPI = {
  importScenarioPackage: (projectId:string,file:File) => requestJSON<{id:string}>(projectAPIPath(projectId,'history/scenarios'),{method:'POST',headers:{'Content-Type':file.name.toLowerCase().endsWith('.zip')?'application/zip':'application/json'},body:file}),
  exportScenarioBundle: async(projectId:string,id:string) => {const response=await fetchAPI(projectAPIPath(projectId,`history/scenarios/${encodeURIComponent(id)}/bundle`));if(!response.ok)throw await responseAPIError(response);return response.blob()},
  importScenario: (projectId:string, pack:object) => post<unknown>(projectAPIPath(projectId,"history/scenarios"),pack),
  exportScenario: (projectId:string,id:string) => requestJSON<unknown>(projectAPIPath(projectId,`history/scenarios/${encodeURIComponent(id)}`)),
  bootstrap: (signal?: AbortSignal) => post<HistoryBootstrap>('/api/history/bootstrap', {}, signal),
  list: (projectId: string) => requestJSON<{ campaigns: HistorySummary[] }>(projectAPIPath(projectId, 'history/campaigns')),
  read: (projectId: string, id: string, signal?: AbortSignal) => requestJSON<HistoryState>(campaignPath(projectId, id), { signal, cache: 'no-store' }),
  create: (projectId: string, body: object, signal?: AbortSignal) => post<HistoryState>(projectAPIPath(projectId, 'history/campaigns'), body, signal),
  act: (projectId: string, id: string, action: HistoryAction, body: object, signal?: AbortSignal) => post<HistoryReceipt | PreparedEdict>(campaignPath(projectId, id, action), body, signal),
  cancel: (projectId: string, id: string, commandId: string) => post<{ cancelled: boolean }>(campaignPath(projectId, id, 'cancel'), { command_id: commandId }),
}
