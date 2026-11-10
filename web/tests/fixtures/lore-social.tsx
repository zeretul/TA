import { useState } from 'react'
import { createRoot } from 'react-dom/client'
import { setConfiguredLocale } from '../../src/i18n'
import { LoreOverviewPanel } from '../../src/features/interactive/components/lore-overview/LoreOverviewPanel'
import { snapshot, cast } from './lore-social-data'
import '../../src/index.css'

const params = new URLSearchParams(location.search)
if (params.get('standalone') === '1') {
  const realFetch = window.fetch.bind(window)
  window.fetch = async (input, init) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    if (!url.includes('/api/interactive/stories/social-fixture/')) return realFetch(input, init)
    const path = new URL(url, location.origin).pathname
    const body = path.endsWith('/characters') ? cast : path.endsWith('/journey') ? { story_id: 'social-fixture', branch_id: 'main', routes: [] } : path.endsWith('/quests') ? { story_id: 'social-fixture', branch_id: 'main', tasks: [], available: [] } : { story_id: 'social-fixture', branch_id: 'main', events: [] }
    return new Response(JSON.stringify(body), { headers: { 'Content-Type': 'application/json' } })
  }
}
setConfiguredLocale(params.get('language') || 'zh-CN')
document.documentElement.dataset.theme = params.get('theme') || 'dark'
function Fixture() {
  const [open, setOpen] = useState(true)
  return <main style={{ width: '100%', maxWidth: 580, height: '100dvh', margin: 'auto' }}>
    {open ? <LoreOverviewPanel storyId="social-fixture" branchId="main" active snapshot={snapshot} resizable={false} onClose={() => setOpen(false)} /> : <button onClick={() => setOpen(true)}>Reopen</button>}
  </main>
}
createRoot(document.getElementById('root')!).render(<Fixture />)
