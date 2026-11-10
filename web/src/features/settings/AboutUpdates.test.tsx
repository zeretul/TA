import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import { APIError } from '@/lib/api-client'
import { AboutUpdates } from './AboutUpdates'
import { applyUpdate, checkForUpdate, fetchUpdateStatus, installUpdateStream, uploadUpdate } from './api'

vi.mock('./api', () => ({
  applyUpdate: vi.fn(), checkForUpdate: vi.fn(), fetchUpdateStatus: vi.fn(),
  installUpdateStream: vi.fn(), uploadUpdate: vi.fn(),
}))

const ready = { current_version: '0.6.6', channel_configured: true, staged: true, apply_ready: true, installed_version: '0.6.7' }
const latest = { current_version: '0.6.6', channel_configured: true, latest_version: '0.6.7', update_available: true, can_install: true, platform: 'windows-x64', release_url: '', published_at: '', release_notes: 'A better journey.' }
const show = () => render(<AboutUpdates autoCheck onAutoCheckChange={vi.fn()} />)

beforeEach(() => {
  vi.resetAllMocks()
  setConfiguredLocale('zh-CN')
  vi.mocked(fetchUpdateStatus).mockResolvedValue({ ...ready, staged: false, apply_ready: false })
})

describe('AboutUpdates', () => {
  it('shows the unconfigured channel and keeps offline staging available without checking online', async () => {
    vi.mocked(fetchUpdateStatus).mockResolvedValue({ ...ready, channel_configured: false, staged: false })
    show()
    expect(await screen.findByText(/还没有连接在线更新源/)).toBeVisible()
    expect(screen.getByRole('button', { name: '检查更新' })).toBeDisabled()
    expect(screen.getByLabelText('选择安装包')).toBeEnabled()
    expect(screen.getByRole('button', { name: '校验并暂存' })).toBeDisabled()
    expect(checkForUpdate).not.toHaveBeenCalled()
    expect(installUpdateStream).not.toHaveBeenCalled()
  })

  it('checks explicitly and shows release notes without downloading', async () => {
    const user = userEvent.setup()
    vi.mocked(checkForUpdate).mockResolvedValue(latest)
    show()
    await waitFor(() => expect(screen.getByRole('button', { name: '检查更新' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: '检查更新' }))
    expect(await screen.findByText('A better journey.')).toBeVisible()
    expect(screen.getByRole('button', { name: '下载更新包' })).toBeEnabled()
    expect(installUpdateStream).not.toHaveBeenCalled()
    expect(applyUpdate).not.toHaveBeenCalled()
  })

  it('requires both files and only stages an offline update', async () => {
    const user = userEvent.setup()
    vi.mocked(uploadUpdate).mockResolvedValue({ previous_version: '0.6.6', installed_version: '0.6.7', installed: false, staged: true, apply_ready: true, restart_required: true })
    show()
    await user.upload(screen.getByLabelText('选择安装包'), new File(['zip'], 'TA-v0.6.7-windows-x64.zip', { type: 'application/zip' }))
    expect(screen.getByRole('button', { name: '校验并暂存' })).toBeDisabled()
    await user.upload(screen.getByLabelText('选择校验文件（checksums.txt）'), new File(['checksum'], 'checksums.txt', { type: 'text/plain' }))
    await user.click(screen.getByRole('button', { name: '校验并暂存' }))
    expect(await screen.findByText('版本 0.6.7 已就绪。')).toBeVisible()
    expect(uploadUpdate).toHaveBeenCalledOnce()
    expect(applyUpdate).not.toHaveBeenCalled()
  })

  it('restores a pending package and requires a second explicit confirmation', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchUpdateStatus).mockResolvedValue(ready)
    vi.mocked(applyUpdate).mockResolvedValue({ status: 'restarting', version: '0.6.7' })
    show()
    await user.click(await screen.findByRole('button', { name: '重启并安装' }))
    expect(applyUpdate).not.toHaveBeenCalled()
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '重启并安装' }))
    expect(await screen.findByText(/正在重新启动/)).toBeVisible()
    expect(applyUpdate).toHaveBeenCalledOnce()
  })

  it('keeps the staged package after a busy refusal and allows retry', async () => {
    const user = userEvent.setup()
    vi.mocked(fetchUpdateStatus).mockResolvedValue(ready)
    vi.mocked(applyUpdate).mockRejectedValueOnce(new APIError('C:/private/raw-details', { status: 409, code: 'api.update.busy' }))
    vi.mocked(applyUpdate).mockResolvedValueOnce({ status: 'restarting', version: '0.6.7' })
    show()
    await user.click(await screen.findByRole('button', { name: '重启并安装' }))
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '重启并安装' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('还有行动正在进行')
    expect(screen.queryByText(/private\/raw-details/)).not.toBeInTheDocument()
    expect(screen.getByText('版本 0.6.7 已就绪。')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '重启并安装' }))
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: '重启并安装' }))
    expect(await screen.findByText(/正在重新启动/)).toBeVisible()
  })

  it('does not offer installation if the download stream ends without a staged result', async () => {
    const user = userEvent.setup()
    vi.mocked(checkForUpdate).mockResolvedValue(latest)
    vi.mocked(installUpdateStream).mockResolvedValue(new ReadableStream({ start(controller) {
      controller.enqueue({ event: 'update_progress', data: JSON.stringify({ phase: 'verifying', percent: 100 }) })
      controller.close()
    } }))
    show()
    await waitFor(() => expect(screen.getByRole('button', { name: '检查更新' })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: '检查更新' }))
    await user.click(await screen.findByRole('button', { name: '下载更新包' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('更新未能完成')
    expect(screen.queryByRole('button', { name: '重启并安装' })).not.toBeInTheDocument()
  })

  it('shows only English after switching language and reports invalid checksums safely', async () => {
    setConfiguredLocale('en-US')
    vi.mocked(uploadUpdate).mockRejectedValue(new APIError('secret-path', { status: 400, code: 'api.update.checksumMismatch' }))
    const user = userEvent.setup()
    show()
    fireEvent.change(screen.getByLabelText('Select release archive'), { target: { files: [new File(['zip'], 'TA-v0.6.7-windows-x64.zip')] } })
    fireEvent.change(screen.getByLabelText('Select checksum file (checksums.txt)'), { target: { files: [new File(['bad'], 'checksums.txt')] } })
    await user.click(screen.getByRole('button', { name: 'Validate and stage' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('does not match')
    expect(screen.queryByText(/校验|secret-path/)).not.toBeInTheDocument()
  })
})
