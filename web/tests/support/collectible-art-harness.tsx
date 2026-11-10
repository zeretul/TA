// Synthetic visual fixture: production components, not earned/model gameplay.
import React from 'react'
import { createRoot } from 'react-dom/client'
import { EntryDetails } from '../../src/features/interactive/components/entry-details/EntryDetails'
import { EntryArtwork } from '../../src/features/interactive/components/entry-details/EntryArtwork'
import { QUALITY_GRADES, type EntryPresentation } from '../../src/features/interactive/components/entry-details/types'
import { setConfiguredLocale } from '../../src/i18n'
import '../../src/index.css'
const params = new URLSearchParams(location.search)
const locale = params.get('locale') || 'zh-CN'
setConfiguredLocale(locale)
document.documentElement.dataset.theme = params.get('theme') || 'dark'
document.documentElement.classList.toggle('dark', params.get('theme') !== 'light')
const theme = (params.get('genre') || 'cultivation') as 'cultivation' | 'wuxia' | 'space'
const categories = ['technique', 'skill', 'equipment']
const entries = QUALITY_GRADES.map((grade, index): EntryPresentation => ({ id: `demo-${grade}`, name: locale === 'en-US' ? `Illustrated ${grade} entry with a long readable title` : `${grade} · 天地长风万象归元演示条目`, quality_grade: grade, category: 'skills', visual_theme: theme, visual_level: theme === 'cultivation' ? { kind: 'stage', value: ['qi_1', 'qi_3', 'qi_7', 'qi_13', 'foundation_middle', 'core_late'][index] } : { kind: 'mastery', value: theme === 'wuxia' ? Math.min(index + 1, 4) : index + 1, maximum: theme === 'wuxia' ? 4 : 10, scale: 'person' }, art: { source: 'preset', preset_id: `${theme}-${categories[index % 3]}` }, description: locale === 'en-US' ? 'Synthetic visual fixture. Actual components, no game mutations.' : '合成视觉验收资料，使用真实组件，不提交游戏变化。', background: 'A long background. '.repeat(120), requirements: [], effects: [], costs: [], status: [] }))
const empty: EntryPresentation = { ...entries[0], id: 'empty', name: locale === 'en-US' ? 'Unillustrated entry' : '暂无配图条目', art: undefined, visual_level: undefined }
createRoot(document.getElementById('root')!).render(<main style={{ padding: 24, maxWidth: params.get('width') === 'narrow' ? 375 : 1100, margin: 'auto' }}><h1 style={{ marginBottom: 20 }}>Collectible art visual fixture</h1><div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(min(100%,240px),1fr))', gap: 24 }}>{entries.map(entry => <section key={entry.id}><EntryArtwork entry={entry} /><div style={{ marginTop: 16 }}><EntryDetails entry={entry} /></div></section>)}<section><EntryArtwork entry={empty} /><EntryDetails entry={empty} /></section></div></main>)
