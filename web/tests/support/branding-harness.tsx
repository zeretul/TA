// Isolated presentation fixture using actual navigation and About components.
// The test mocks only update-status transport, never game state or model output.
import React from 'react'
import { createRoot } from 'react-dom/client'
import { Gamepad2, Library, Package, Settings2 } from 'lucide-react'
import { SidebarProvider, useSidebar } from '../../src/components/ui/sidebar'
import { WorkbenchAppSidebar } from '../../src/components/workbench/WorkbenchAppSidebar'
import { AboutUpdates } from '../../src/features/settings/AboutUpdates'
import { setConfiguredLocale } from '../../src/i18n'
import '../../src/index.css'
const params = new URLSearchParams(location.search)
const english = params.get('locale') === 'en-US'
setConfiguredLocale(english ? 'en-US' : 'zh-CN')
const theme = params.get('theme') || 'dark'
document.documentElement.dataset.theme = theme
document.documentElement.classList.toggle('dark', theme === 'dark')

function Preview() {
  const { open, toggleSidebar, isMobile, setOpenMobile } = useSidebar()
  const [active, setActive] = React.useState('game')
  const [autoCheck, setAutoCheck] = React.useState(false)
  const items = [
    { id: 'game', label: english ? 'Game' : '游戏', icon: <Gamepad2 /> },
    { id: 'lore', label: english ? 'Lore' : '资料库', icon: <Library /> },
    { id: 'worlds', label: english ? 'World packs' : '世界包', icon: <Package /> },
    { id: 'game-settings', label: english ? 'Game settings' : '游戏设置', icon: <Settings2 /> },
  ].map(item => ({ ...item, active: active === item.id, onClick: () => setActive(item.id) }))
  return <>
    <WorkbenchAppSidebar expanded={open} activityOrderScope="brand-test" activityItems={items} customizationItems={items}
      hiddenActivityIDs={[]} dragDisabled contextSwitcher={null} sidebarLabel="Navigation" settingsLabel={english ? 'Settings' : '系统设置'}
      settingsActive={active === 'settings'} toggleLabel={english ? 'Toggle navigation' : '切换导航'} resizeLabel="Resize navigation"
      minWidth={180} maxWidth={320} currentWidth={232} onOpenSettings={() => setActive('settings')} onToggle={toggleSidebar}
      onReorder={() => {}} onCustomizationReorder={() => {}} onActivityVisibilityChange={() => {}}
      onResizePointerDown={() => {}} onResizeKeyDown={() => {}} />
    <main style={{ minWidth: 0, flex: 1 }}>
      {isMobile && <button onClick={() => setOpenMobile(true)}>Open navigation</button>}
      <AboutUpdates autoCheck={autoCheck} onAutoCheckChange={setAutoCheck} />
    </main>
  </>
}
createRoot(document.getElementById('root')!).render(<SidebarProvider style={{ '--sidebar-width': '232px' } as React.CSSProperties}><Preview /></SidebarProvider>)
