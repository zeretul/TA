import { AppBrandMark } from '@/components/common/AppBrandMark'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APP_VERSION } from '@/app-version'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Progress } from '@/components/ui/progress'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { APIError } from '@/lib/api-client'
import { applyUpdate, checkForUpdate, fetchUpdateStatus, installUpdateStream, uploadUpdate } from './api'
import type { UpdateCheckResult, UpdateInstallProgress, UpdateStatusResult } from './types'

type Operation = 'idle' | 'checking' | 'downloading' | 'uploading' | 'applying'

// This surface never decides whether applying is safe. The application must
// atomically refuse busy installs, including requests arriving after this UI.
export function AboutUpdates({ autoCheck, onAutoCheckChange, visible = true }: {
  autoCheck: boolean
  onAutoCheckChange: (value: boolean) => void
  visible?: boolean
}) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<UpdateStatusResult | null>(null)
  const [check, setCheck] = useState<UpdateCheckResult | null>(null)
  const [operation, setOperation] = useState<Operation>('idle')
  const [progress, setProgress] = useState<UpdateInstallProgress | null>(null)
  const [error, setError] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [archive, setArchive] = useState<File | null>(null)
  const [checksums, setChecksums] = useState<File | null>(null)
  const fileID = useId()
  const archiveInput = useRef<HTMLInputElement>(null)
  const checksumInput = useRef<HTMLInputElement>(null)
  const alive = useRef(false)
  const running = useRef(false)
  const busy = operation !== 'idle'
  const channelConfigured = check?.channel_configured ?? status?.channel_configured ?? false

  useEffect(() => {
    alive.current = true
    if (visible) {
      void fetchUpdateStatus().then(value => {
        if (alive.current) setStatus(value)
      }).catch(() => {
        if (alive.current) setError(t('settings.updates.statusFailed'))
      })
    }
    return () => { alive.current = false }
  }, [visible, t])

  const run = async (next: Operation, action: () => Promise<void>) => {
    if (running.current) return
    running.current = true
    setOperation(next)
    setError('')
    try {
      await action()
    } catch (failure) {
      if (alive.current) {
        // Only translate known server keys; never display raw transport,
        // manifest, local-path or process details in the player interface.
        const keys: Record<string, string> = {
          'api.update.busy': 'settings.updates.busy',
          'api.update.checksumRequired': 'settings.updates.checksumRequired',
          'api.update.checksumMismatch': 'settings.updates.checksumMismatch',
          'api.update.wrongPlatform': 'settings.updates.noPlatformAsset',
          'api.update.invalidPackage': 'settings.updates.invalidPackage',
          'api.update.newerRequired': 'settings.updates.notNewer',
        }
        setError(t(keys[failure instanceof APIError ? failure.code || '' : ''] || 'settings.updates.operationFailed'))
      }
    } finally {
      running.current = false
      if (alive.current) setOperation('idle')
    }
  }

  const download = () => run('downloading', async () => {
    setProgress(null)
    const stream = await installUpdateStream()
    const reader = stream.getReader()
    let staged = false
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        if (value.event === 'error') throw new Error('Update download failed')
        if (value.event === 'update_progress' && alive.current) setProgress(JSON.parse(value.data) as UpdateInstallProgress)
        if (value.event === 'update_result') {
          const result = JSON.parse(value.data) as UpdateStatusResult
          staged = result.staged === true && result.apply_ready === true
          if (alive.current) setStatus(current => ({ ...current, ...result }))
        }
      }
      if (!staged) throw new Error('Update stream ended without a staged result')
    } finally {
      reader.releaseLock()
    }
  })

  const upload = () => run('uploading', async () => {
    if (!archive || !checksums) return
    const result = await uploadUpdate(archive, checksums)
    if (!result.staged || !result.apply_ready) throw new Error('Package was not staged')
    if (alive.current) setStatus(current => ({ ...current, ...result }))
  })

  const install = () => run('applying', async () => {
    const result = await applyUpdate()
    if (result.status !== 'restarting') throw new Error('Update was not scheduled')
    // Keep the explicit restart notice, without polling or claiming that the
    // new process is already running. The player reloads once TA has restarted.
    if (alive.current) setStatus(current => ({ ...current, restarting: true }))
  })

  const percent = Math.min(100, Math.max(0, Number.isFinite(progress?.percent) ? progress?.percent ?? 0 : 0))
  const phaseKeys: Record<string, string> = {
    checking: 'settings.updates.phase.checking', downloading: 'settings.updates.phase.downloading',
    verifying: 'settings.updates.phase.verifying', extracting: 'settings.updates.phase.extracting',
    staging: 'settings.updates.phase.staging', staged: 'settings.updates.phase.staged',
  }

  return <div className="min-w-0 space-y-5 p-4 sm:p-5" data-testid="about-updates">
    <div className="flex min-w-0 items-center gap-4">
      <AppBrandMark className="h-12 w-20" />
      <div className="min-w-0">
        <h3 className="font-semibold">Text Adventure</h3>
        <p className="text-sm text-muted-foreground">{t('settings.updates.currentVersion', { version: status?.current_version || APP_VERSION })}</p>
      </div>
    </div>
    {status?.restarting ? <div className="space-y-3" role="status">
      <p>{t('settings.updates.restartNotice')}</p>
      <Button variant="outline" onClick={() => window.location.reload()}>{t('settings.updates.reload')}</Button>
    </div> : <>
      <div className="flex items-center justify-between gap-4">
        <label htmlFor="ta-update-auto" className="min-w-0 text-sm">{t('settings.updates.autoCheck')}</label>
        <Switch id="ta-update-auto" checked={autoCheck} disabled={busy} onCheckedChange={onAutoCheckChange} />
      </div>
      <p className="text-sm text-muted-foreground">{t('settings.updates.checkOnly')}</p>
      {!channelConfigured && status && <p role="status" className="text-sm text-muted-foreground">{t('settings.updates.channelUnset')}</p>}
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={busy || !status || !channelConfigured} onClick={() => void run('checking', async () => {
          const result = await checkForUpdate()
          if (alive.current) setCheck(result)
        })}>{t(operation === 'checking' ? 'settings.updates.checking' : 'settings.updates.check')}</Button>
        {check?.update_available && check.can_install && <Button disabled={busy} onClick={() => void download()}>{t('settings.updates.download')}</Button>}
      </div>
      {check && <div className="min-w-0 space-y-2 text-sm" role="status">
        <p>{t(check.update_available ? 'settings.updates.latestVersion' : 'settings.updates.upToDate', { version: check.latest_version })}</p>
        {check.update_available && !check.can_install && <p>{t('settings.updates.noPlatformAsset')}</p>}
        {check.release_notes && <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border p-3 font-sans">{check.release_notes}</pre>}
      </div>}
      {operation === 'downloading' && <div role="status" className="space-y-2">
        <p className="text-sm">{t(phaseKeys[progress?.phase || 'checking'] || 'settings.updates.installing')}</p>
        <Progress value={percent} aria-label={t('settings.updates.progressAria')} />
      </div>}
      <div className="min-w-0 space-y-3 border-t pt-4">
        <h4 className="text-sm font-medium">{t('settings.updates.manual')}</h4>
        <p className="text-sm text-muted-foreground">{t('settings.updates.offlineHint')}</p>
        <div className="min-w-0 space-y-1 text-sm">
          <label htmlFor={`${fileID}-archive`}>{t('settings.updates.selectPackage')}</label>
          <input id={`${fileID}-archive`} ref={archiveInput} type="file" accept=".zip,.gz" className="sr-only" disabled={busy} onChange={event => {
            const file = event.target.files?.[0] ?? null
            if (file && file.size > 512 * 1024 * 1024) { setError(t('settings.updates.tooLarge')); setArchive(null); return }
            setArchive(file)
          }} />
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Button variant="outline" disabled={busy} onClick={() => archiveInput.current?.click()}>{t('settings.updates.chooseFile')}</Button>
            <span className="min-w-0 break-all text-muted-foreground">{archive?.name || t('settings.updates.noFile')}</span>
          </div>
        </div>
        <div className="min-w-0 space-y-1 text-sm">
          <label htmlFor={`${fileID}-checksums`}>{t('settings.updates.selectChecksums')}</label>
          <input id={`${fileID}-checksums`} ref={checksumInput} type="file" accept=".txt" className="sr-only" disabled={busy} onChange={event => {
            const file = event.target.files?.[0] ?? null
            if (file && file.size > 1024 * 1024) { setError(t('settings.updates.checksumsTooLarge')); setChecksums(null); return }
            setChecksums(file)
          }} />
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Button variant="outline" disabled={busy} onClick={() => checksumInput.current?.click()}>{t('settings.updates.chooseFile')}</Button>
            <span className="min-w-0 break-all text-muted-foreground">{checksums?.name || t('settings.updates.noFile')}</span>
          </div>
        </div>
        <Button variant="outline" disabled={busy || !archive || !checksums} onClick={() => void upload()}>{t(operation === 'uploading' ? 'settings.updates.uploading' : 'settings.updates.verifyPackage')}</Button>
      </div>
      {status?.staged && status.apply_ready && <div role="status" className="space-y-3 rounded-lg border p-3">
        <p className="text-sm">{t('settings.updates.stagedVersion', { version: status.installed_version })}</p>
        <Button disabled={busy} onClick={() => setConfirm(true)}>{t('settings.updates.apply')}</Button>
      </div>}
    </>}
    {error && <p role="alert" className="break-words text-sm text-destructive">{error}</p>}
    <AlertDialog open={confirm} onOpenChange={setConfirm}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>{t('settings.updates.confirmTitle')}</AlertDialogTitle>
          <AlertDialogDescription>{t('settings.updates.confirmHint')}</AlertDialogDescription></AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={() => void install()}>{t('settings.updates.apply')}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
}
