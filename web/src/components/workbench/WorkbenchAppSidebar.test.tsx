import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { SidebarProvider } from '@/components/ui/sidebar'
import { WorkbenchAppSidebar } from './WorkbenchAppSidebar'

function BrandedNavigation({ onGame, onSettings }: { onGame: () => void; onSettings: () => void }) {
  const [expanded, setExpanded] = useState(true)
  const items = [{ id: 'game', label: 'Game', icon: <span aria-hidden="true">G</span>, active: true, onClick: onGame }]
  return <SidebarProvider open={expanded} onOpenChange={setExpanded}>
    <WorkbenchAppSidebar expanded={expanded} activityOrderScope="brand-test" activityItems={items} customizationItems={items}
      hiddenActivityIDs={[]} dragDisabled contextSwitcher={null} sidebarLabel="Navigation" settingsLabel="Settings"
      settingsActive={false} toggleLabel="Toggle navigation" resizeLabel="Resize navigation"
      minWidth={180} maxWidth={320} currentWidth={232} onOpenSettings={onSettings} onToggle={() => setExpanded(v => !v)}
      onReorder={() => {}} onCustomizationReorder={() => {}} onActivityVisibilityChange={() => {}}
      onResizePointerDown={() => {}} onResizeKeyDown={() => {}} />
  </SidebarProvider>
}

describe('branded primary navigation', () => {
  it('keeps the mark and navigation controls available after repeated collapse and expand', async () => {
    const user = userEvent.setup()
    const onGame = vi.fn(), onSettings = vi.fn()
    const { container } = render(<BrandedNavigation onGame={onGame} onSettings={onSettings} />)
    for (let i = 0; i < 2; i++) {
      expect(screen.getByText('Text Adventure')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Toggle navigation' }))
      expect(screen.queryByText('Text Adventure')).not.toBeInTheDocument()
      expect(container.querySelector('[data-slot="sidebar-header"] img')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Game' }))
      await user.click(screen.getByRole('button', { name: 'Settings' }))
      await user.click(screen.getByRole('button', { name: 'Toggle navigation' }))
    }
    expect(screen.getByText('Text Adventure')).toBeInTheDocument()
    expect(onGame).toHaveBeenCalledTimes(2)
    expect(onSettings).toHaveBeenCalledTimes(2)
  })
})
