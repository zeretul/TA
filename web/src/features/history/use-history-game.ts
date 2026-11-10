import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError } from '@/lib/api-client'
import { historyAPI } from './api'
import type { HistoryAction, HistoryBootstrap, HistoryReceipt, HistoryState, PreparedEdict } from './types'

type PendingKind = HistoryAction | 'create' | 'load'
interface Operation { kind: PendingKind; id: string; projectId: string; campaignId?: string; body: Record<string, unknown> }
interface Failure { key: string; retry?: Operation; requestId?: string }
const campaignKey = (projectId: string) => `ta:history:${projectId}:campaign`
const commandId = () => `history-${crypto.randomUUID()}`

/** Owns one history destination, never the currently selected adventure project. */
export function useHistoryGame(active: boolean) {
  const { i18n } = useTranslation()
  const [bootstrap, setBootstrap] = useState<HistoryBootstrap | null>(null)
  const [state, setState] = useState<HistoryState | null>(null)
  const [prepared, setPrepared] = useState<PreparedEdict | null>(null)
  const [pending, setPending] = useState<PendingKind | null>('load')
  const [failure, setFailure] = useState<Failure | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const mounted = useRef(true)
  const flight = useRef<{ operation: Operation; controller: AbortController; cancelling: boolean } | null>(null)
  const stateRef = useRef(state)
  stateRef.current = state
  const locale = i18n.resolvedLanguage || i18n.language
  const catalogLocale = useRef(locale)
  const historyProjectId = bootstrap?.project_id

  const accept = useCallback((next: HistoryState) => {
    setState(next)
    stateRef.current = next
    setPrepared(null)
    setBootstrap((current) => current ? {
      ...current, campaigns: [next, ...current.campaigns.filter((item) => item.id !== next.id)],
    } : current)
  }, [])

  const execute = useCallback(async (operation: Operation): Promise<boolean> => {
    if (flight.current) return false
    const controller = new AbortController()
    const current = { operation, controller, cancelling: false }
    flight.current = current
    setPending(operation.kind)
    setFailure(null)
    setNotice(null)
    try {
      if (operation.kind === 'create') {
        const next = await historyAPI.create(operation.projectId, operation.body, controller.signal)
        if (!mounted.current || flight.current !== current || current.cancelling) return false
        accept(next)
        localStorage.setItem(campaignKey(operation.projectId), next.id)
      } else if (operation.kind === 'load') {
        const next = await historyAPI.read(operation.projectId, operation.campaignId!, controller.signal)
        if (!mounted.current || flight.current !== current || current.cancelling) return false
        accept(next)
        localStorage.setItem(campaignKey(operation.projectId), next.id)
      } else {
        const result = await historyAPI.act(operation.projectId, operation.campaignId!, operation.kind, operation.body, controller.signal)
        if (!mounted.current || flight.current !== current || current.cancelling) return false
        if (operation.kind === 'prepare-edict') setPrepared(result as PreparedEdict)
        else { accept((result as HistoryReceipt).state);if(operation.kind==='archive'){setState(null);stateRef.current=null;localStorage.removeItem(campaignKey(operation.projectId))}else if(operation.kind==='unarchive'){localStorage.setItem(campaignKey(operation.projectId),operation.campaignId!)} }
        setNotice(`history.done.${operation.kind}`)
      }
      return true
    } catch (error) {
      if (!mounted.current || flight.current !== current || current.cancelling) return false
      if (controller.signal.aborted) return false
      const code = error instanceof APIError ? error.code : undefined
      const conflict = code === 'api.history.conflict' || code === 'api.history.commandConflict'
      console.error('[history] Operation failed', { kind: operation.kind, code, requestId: error instanceof APIError ? error.requestID : undefined })
      setFailure({
        key: code?.startsWith('api.history.') ? code : 'history.error.request',
        retry: conflict ? undefined : operation,
        requestId: error instanceof APIError ? error.requestID : undefined,
      })
      if (conflict && operation.campaignId) {
        try { accept(await historyAPI.read(operation.projectId, operation.campaignId)) }
        catch { /* Keep the visible state and explicit refresh action. */ }
      }
      return false
    } finally {
      if (flight.current === current && !current.cancelling) {
        flight.current = null
        if (mounted.current) setPending(null)
      }
    }
  }, [accept])

  const load = useCallback(async () => {
    if (flight.current) return
    setPending('load')
    setFailure(null)
    try {
      const next = await historyAPI.bootstrap()
      if (!mounted.current) return
      setBootstrap({ ...next, campaigns: next.campaigns || [] })
      const previous = localStorage.getItem(campaignKey(next.project_id))
      const selected = next.campaigns?.find((item) => item.id === previous && !item.unavailable && !item.archived) || next.campaigns?.find((item) => !item.unavailable && !item.archived)
      if (selected) await execute({ kind: 'load', id: commandId(), projectId: next.project_id, campaignId: selected.id, body: {} })
      else {setState(null);stateRef.current=null}
    } catch (error) {
      console.error('[history] Bootstrap failed', error)
      if (mounted.current) setFailure({ key: 'history.error.load' })
    } finally { if (mounted.current && !flight.current) setPending(null) }
  }, [execute])

  useEffect(() => {
    mounted.current = true
    void load()
    return () => {
      mounted.current = false
      const current = flight.current
      if (current) {
        if (current.operation.campaignId && ['chat', 'prepare-edict', 'direct-edict', 'advance'].includes(current.operation.kind)) {
          void historyAPI.cancel(current.operation.projectId, current.operation.campaignId, current.operation.id).catch((error) => console.warn('[history] Unmount cancellation failed', error)).finally(() => current.controller.abort())
        } else current.controller.abort()
      }
    }
  }, [load])

  // Scenario summaries are translated by the server. Refresh only the catalog:
  // reopening the campaign would discard a reviewed plan or interrupt an operation.
  useEffect(() => {
    if (!historyProjectId || catalogLocale.current === locale) return
    catalogLocale.current = locale
    const controller = new AbortController()
    void historyAPI.bootstrap(controller.signal).then((next) => {
      if (!mounted.current || controller.signal.aborted) return
      setBootstrap((current) => current?.project_id === next.project_id
        ? { ...current, scenarios: next.scenarios }
        : current)
    }).catch((error) => {
      if (controller.signal.aborted || !mounted.current) return
      console.warn('[history] Scenario catalog localization failed', error)
      setFailure((current) => current || { key: 'history.error.load' })
    })
    return () => controller.abort()
  }, [historyProjectId, locale])

  const cancel = useCallback(async () => {
    const current = flight.current
    if (!current?.operation.campaignId || current.cancelling || !['chat', 'prepare-edict', 'direct-edict', 'advance'].includes(current.operation.kind)) return
    current.cancelling = true
    try {
      await historyAPI.cancel(current.operation.projectId, current.operation.campaignId, current.operation.id)
      current.controller.abort()
      // A response can be lost after commit: always read the canonical result.
      const latest = await historyAPI.read(current.operation.projectId, current.operation.campaignId)
      if (mounted.current && flight.current === current) {
        accept(latest)
        setNotice('history.cancelled')
      }
    } catch (error) {
      current.controller.abort()
      console.warn('[history] Cancellation or state refresh failed', error)
      if (mounted.current) setFailure({ key: 'history.error.cancel' })
    } finally {
      if (flight.current === current) {
        flight.current = null
        if (mounted.current) setPending(null)
      }
    }
  }, [accept])

  useEffect(() => { if (!active) void cancel() }, [active, cancel])

  // Set IDs on the stable operation so a retry cannot duplicate a committed action.
  const runWithCommand = useCallback((kind: HistoryAction, fields: Record<string, unknown> = {}) => {
    const current = stateRef.current
    if (!bootstrap || !current) return Promise.resolve(false)
    const body = { expected_revision: current.revision, locale, ...fields }
    const retry = failure?.retry
    if (retry?.kind === kind && retry.campaignId === current.id) {
      const { command_id: _commandId, ...previous } = retry.body
      if (JSON.stringify(previous) === JSON.stringify(body)) return execute(retry)
    }
    const id = commandId()
    return execute({ kind, id, projectId: bootstrap.project_id, campaignId: current.id,
      body: { ...body, command_id: id },
    })
  }, [bootstrap, execute, failure, locale])

  return {
    bootstrap, state, prepared, pending, failure, notice,
    busy: pending !== null,
    canCancel: ['chat', 'prepare-edict', 'direct-edict', 'advance'].includes(pending || ''),
    discardPrepared: () => setPrepared(null),
    reload: load,
    clearNotice: () => setNotice(null),
    retry: () => failure?.retry ? execute(failure.retry) : load(),
    refresh: () => bootstrap && state ? execute({ kind: 'load', id: commandId(), projectId: bootstrap.project_id, campaignId: state.id, body: {} }) : load(),
    cancel,
    create: (name: string, scenarioId?: string) => {
      if (!bootstrap) return Promise.resolve(false)
      if (failure?.retry?.kind === 'create' && failure.retry.body.name === name && failure.retry.body.scenario_id === scenarioId) return execute(failure.retry)
      const id = commandId()
      return execute({ kind: 'create', id, projectId: bootstrap.project_id, body: { command_id: id, name, locale, scenario_id: scenarioId || bootstrap.scenarios[0]?.id } })
    },
    select: (id: string) => bootstrap ? execute({ kind: 'load', id: commandId(), projectId: bootstrap.project_id, campaignId: id, body: {} }) : Promise.resolve(false),
    recover: (campaignId:string)=>{const saved=bootstrap?.campaigns.find(item=>item.id===campaignId);if(!bootstrap||!saved)return Promise.resolve(false);const id=commandId();return execute({kind:'unarchive',id,projectId:bootstrap.project_id,campaignId,body:{command_id:id,expected_revision:saved.revision,locale}})},
    act: runWithCommand,
  }
}
export type HistoryGameController = ReturnType<typeof useHistoryGame>
