import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ComponentProps, ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkbenchShell } from './WorkbenchShell'

vi.mock('@/hooks/useIsMobile', () => ({ useIsMobile: () => false }))
vi.mock('@/components/layout/workspace-layout', () => ({
  WorkspaceLayout: ({ appSidebar, main }: { appSidebar: ReactNode; main: ReactNode }) => <>{appSidebar}{main}</>,
}))
vi.mock('@/components/ui/sidebar', () => ({
  SidebarProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}))
vi.mock('./BookSwitcher', () => ({ BookSwitcher: () => null }))
vi.mock('./WorkbenchAppSidebar', () => ({
  WorkbenchAppSidebar: ({ activityItems }: {
    activityItems: { id: string; active: boolean; onClick: () => void }[]
  }) => <nav>{activityItems.map((item) => (
    <button key={item.id} onClick={item.onClick} aria-pressed={item.active}>{item.id}</button>
  ))}</nav>,
}))

function shellProps(overrides: Partial<ComponentProps<typeof WorkbenchShell>> = {}): ComponentProps<typeof WorkbenchShell> {
  return {
    mode: 'assistant', presentedLayout: 'full', currentBookName: '', workspace: '',
    books: [], summary: null, activityBarExpanded: true, settingsOpen: false, main: <div>Content</div>,
    onSetMode: vi.fn(), onToggleActivityBarExpanded: vi.fn(), onSetInteractiveSubmode: vi.fn(),
    onToggleSettings: vi.fn(), onCloseSettings: vi.fn(), onQuickSwitchBook: vi.fn(async () => true),
    ...overrides,
  }
}

describe('game navigation after workspace hydration', () => {
  beforeEach(() => window.localStorage.clear())

  it('uses the restored adventure workspace without requiring a mode change', async () => {
    const props = shellProps()
    const { rerender } = render(<WorkbenchShell {...props} />)
    rerender(<WorkbenchShell {...props} workspace="/restored-adventure" />)
    fireEvent.click(screen.getByRole('button', { name: 'story' }))
    await waitFor(() => expect(props.onSetMode).toHaveBeenCalledWith('interactive'))
    expect(props.onSetMode).not.toHaveBeenCalledWith('games')
  })

  it('opens the chooser if the adventure workspace was removed on the same route', async () => {
    const props = shellProps({ workspace: '/previous-adventure' })
    const { rerender } = render(<WorkbenchShell {...props} />)
    rerender(<WorkbenchShell {...props} workspace="" />)
    fireEvent.click(screen.getByRole('button', { name: 'story' }))
    await waitFor(() => expect(props.onSetMode).toHaveBeenCalledWith('games'))
  })

  it('retains history when the background adventure workspace changes', async () => {
    const props = shellProps({ mode: 'history' })
    const { rerender } = render(<WorkbenchShell {...props} />)
    rerender(<WorkbenchShell {...props} workspace="/background-adventure" />)
    expect(screen.getAllByRole('button', { pressed: true })).toHaveLength(1)
    fireEvent.click(screen.getByRole('button', { name: 'story' }))
    await waitFor(() => expect(props.onSetMode).toHaveBeenCalledWith('history'))
  })
})
