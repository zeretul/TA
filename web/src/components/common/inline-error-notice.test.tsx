import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { InlineErrorNotice } from './inline-error-notice'
import i18n from '@/i18n'

it('shows and copies a useful diagnostic without expanding hidden details', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  render(<InlineErrorNotice message={{ error: 'Approval failed', code: 'agent_runtime.definition_mismatch', request_id: 'request-1', details: { detail: 'definition changed' } }} />)
  expect(screen.getByRole('alert')).toHaveTextContent('definition changed')
  expect(screen.getByRole('alert')).toHaveTextContent('agent_runtime.definition_mismatch')
  fireEvent.click(screen.getByRole('button', { name: '复制诊断信息' }))
  await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
  expect(writeText.mock.calls[0][0]).toContain('request-1')
  expect(writeText.mock.calls[0][0]).toContain('界面版本')
})

it.each([
  ['zh-CN', '当前模型服务不可用，请检查连接或切换已配置的模型后重试。', '查看诊断信息', '复制诊断信息'],
  ['en-US', 'The model service is unavailable. Check the connection or choose another configured model, then retry.', 'View diagnostics', 'Copy diagnostics'],
])('keeps a provider outage readable and preserves copyable diagnostics in %s', async (locale, summary, detailsLabel, copyLabel) => {
  const previous = i18n.language
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  try {
    await i18n.changeLanguage(locale)
    const diagnostic = 'provider API error (status 503): POST "http://127.0.0.1:8320/v1/chat/completions": no_available_account'
    render(<InlineErrorNotice message={diagnostic} />)
    expect(screen.getByText(summary)).toBeVisible()
    const disclosure = screen.getByText(detailsLabel).closest('details')
    expect(disclosure).not.toHaveAttribute('open')
    expect(disclosure).toHaveTextContent('no_available_account')
    fireEvent.click(screen.getByRole('button', { name: copyLabel }))
    await waitFor(() => expect(writeText).toHaveBeenCalledOnce())
    expect(writeText.mock.calls[0][0]).toContain('status 503')
    expect(writeText.mock.calls[0][0]).toContain('no_available_account')
  } finally {
    await i18n.changeLanguage(previous)
  }
})
