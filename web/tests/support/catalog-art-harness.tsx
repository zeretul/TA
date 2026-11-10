// Presentation-only fixture. Catalog transport is mocked by browser tests; no
// game facts, discovered people or model calls are created by this page.
import React from 'react'
import { createRoot } from 'react-dom/client'
import { EntryArtwork } from '../../src/features/interactive/components/entry-details/EntryArtwork'
import { EntryDetails } from '../../src/features/interactive/components/entry-details/EntryDetails'
import { PersonAvatar } from '../../src/features/interactive/components/person-avatar'
import { setConfiguredLocale } from '../../src/i18n'
import type { EntryPresentation } from '../../src/features/interactive/components/entry-details/types'
import '../../src/index.css'
const params = new URLSearchParams(location.search)
setConfiguredLocale(params.get('locale') || 'zh-CN')
document.documentElement.classList.toggle('dark', params.get('theme') !== 'light')
const base: EntryPresentation = { id: 'fixture', name: '独立插画长标题 · Dedicated world illustration with a long title', category: 'equipment', quality_grade: 'blue', requirements: [], effects: [], costs: [], status: [], description: 'Presentation fixture only. 合成视觉验收，不提交游戏事实。' }
const examples: EntryPresentation[] = [
  { ...base, id: 'dedicated', art: { source: 'catalog', catalog_id: 'c74695d9ece8b400b3f9c56a94c957ef5309bb92fcdd908065368019ff9b7f8e' } },
  { ...base, id: 'shared', art: { source: 'preset', preset_id: 'space-equipment' } },
  { ...base, id: 'failed', art: { source: 'catalog', catalog_id: 'cd'.repeat(32) } },
]
createRoot(document.getElementById('root')!).render(<main style={{ padding: 20, maxWidth: 1100, margin: 'auto' }}>
  <h1>Catalog artwork fixture</h1>
  <section><PersonAvatar name="宋老橹" avatar={`/api/catalog-art/${'73cc2b0cd0ac0102b92b09c48f6ca6eca77f021816c9eae57fc1ac3478f3948f'}.png`} /></section>
  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(min(100%,260px),1fr))', gap: 24 }}>
    {examples.map(entry => <section data-testid={entry.id} key={entry.id}><EntryArtwork entry={entry} /><EntryDetails entry={entry} /></section>)}
  </div>
</main>)
