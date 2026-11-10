import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import i18n from '@/i18n'
import { EntryArtwork, artworkIntensity, artworkSource } from './EntryArtwork'
import { adventurePresentations, cultivationPresentation } from './presentations'
import { QUALITY_GRADES, type EntryPresentation } from './types'
const entry: EntryPresentation = { id: 'test', name: 'Blade', category: 'equipment', requirements: [], effects: [], costs: [], status: [] }
afterEach(async () => { cleanup(); await i18n.changeLanguage('zh-CN') })

describe('collectible art and independent display effects', () => {
  it('renders all nine genuine shared raster presets with an accessible image name', async () => {
    await i18n.changeLanguage('en-US')
    for (const theme of ['cultivation', 'wuxia', 'space']) for (const category of ['technique', 'skill', 'equipment']) {
      const view = render(<EntryArtwork entry={{ ...entry, art: { source: 'preset', preset_id: `${theme}-${category}` } }} />)
      expect(screen.getByRole('img', { name: 'Illustration of Blade' })).toHaveStyle({ backgroundSize: '300% 300%' })
      expect(view.container.querySelector('figcaption')).toBeNull()
      view.unmount()
    }
  })
  it.each(QUALITY_GRADES)('preserves %s hue at higher actual mastery and works without art', grade => {
    const row = { ...entry, quality_grade: grade, visual_theme: 'space' as const, visual_level: { kind: 'mastery' as const, value: 1, maximum: 10, scale: 'ship' } }
    const view = render(<EntryArtwork entry={row} />)
    const frame = view.container.querySelector('[data-art-grade]')!
    const hue = (frame as HTMLElement).style.getPropertyValue('--entry-aura')
    expect(frame).toHaveAttribute('data-art-intensity', '0.000')
    expect(screen.queryByRole('img')).toBeNull()
    view.rerender(<EntryArtwork entry={{ ...row, visual_level: { ...row.visual_level, value: 10 } }} />)
    expect(frame).toHaveAttribute('data-art-grade', grade)
    expect(frame).toHaveAttribute('data-art-intensity', '1.000')
    expect((frame as HTMLElement).style.getPropertyValue('--entry-aura')).toBe(hue)
  })
  it('has no invented grade, stage, image or intensity for unknown data', () => {
    const view = render(<EntryArtwork entry={entry} />)
    expect(view.container.querySelector('[data-art-grade]')).toBeNull()
    view.rerender(<EntryArtwork entry={{ ...entry, quality_grade: 'constructor' as EntryPresentation['quality_grade'] }} />)
    expect(view.container.querySelector('[data-art-grade]')).toBeNull()
    expect(artworkIntensity(undefined)).toBe(0)
    expect(artworkIntensity({ kind: 'stage', value: 'foundation_early' })).toBeGreaterThan(artworkIntensity({ kind: 'stage', value: 'qi_13' }))
    expect(artworkIntensity({ kind: 'stage', value: 'mahayana_late' })).toBe(1)
    expect(artworkIntensity({ kind: 'stage', value: 'qi_99' })).toBe(0)
    expect(artworkSource({ source: 'preset', preset_id: 'unknown' })).toBeNull()
    expect(artworkSource({ source: 'generated', project_id: 'p', image_path: 'https://evil.test/a.png' })).toBeNull()
    expect(artworkSource({ source: 'generated', project_id: 'p', image_path: 'assets/../secret' })).toBeNull()
  })
  it('keeps every supported cultivation stage continuous and normalized through Mahayana', () => {
    const stages = [
      ...Array.from({ length: 13 }, (_, i) => `qi_${i + 1}`),
      ...['foundation', 'core', 'nascent', 'spirit', 'void', 'integration', 'mahayana']
        .flatMap(realm => ['early', 'middle', 'late'].map(stage => `${realm}_${stage}`)),
    ]
    const intensities = stages.map(value => artworkIntensity({ kind: 'stage', value }))
    expect(intensities[0]).toBe(0)
    expect(intensities.at(-1)).toBe(1)
    for (let i = 1; i < intensities.length; i++) {
      expect(intensities[i]).toBeGreaterThan(intensities[i - 1])
      expect(intensities[i] - intensities[i - 1]).toBeCloseTo(1 / 33)
    }
  })
  it.each([
    ['zh-CN', 'integration_middle', '合体中期'],
    ['en-US', 'integration_middle', 'Body Integration, middle'],
  ])('renders a localized Spirit Realm art caption in %s', async (language, value, label) => {
    await i18n.changeLanguage(language)
    const view = render(<EntryArtwork entry={{ ...entry, visual_level: { kind: 'stage', value } }} />)
    expect(view.container.textContent).toContain(label)
    expect(view.container.textContent).not.toContain(value)
  })
  it('uses safe authenticated project assets, and failed art leaves a usable empty frame', async () => {
    await i18n.changeLanguage('en-US')
    const art = { source: 'generated' as const, image_path: 'assets/image/generated/blade.png', project_id: 'project 1' }
    expect(artworkSource(art)?.url).toContain('/api/projects/project%201/files/asset?path=')
    const view = render(<EntryArtwork entry={{ ...entry, art }} />)
    fireEvent.error(view.container.querySelector('img')!)
    expect(screen.queryByRole('img')).toBeNull()
    expect(screen.getByText('No illustration yet')).toBeVisible()
    expect(view.container.querySelector('figcaption')).toBeNull()
  })
  it('uses real mastery with the world cap and scale; equipment does not inherit actor rank', () => {
    const [skill, item] = adventurePresentations({ head: 'h', family: 'space', actor: { skills: { pilot: 7 }, inventory: { ship: 1 } }, skills: [{ id: 'pilot', name: 'Pilot', scale: 'ship' }], items: [{ id: 'ship', name: 'Ship', scale: 'ship', power: 99 }] }, i18n.t)
    expect(skill.visual_level).toEqual({ kind: 'mastery', value: 7, maximum: 10, scale: 'ship' })
    expect(item.visual_level).toBeUndefined()
    expect(cultivationPresentation({ id: 't', name: 'Method', definition: { minimum_stage: 'foundation_late', power: 999 } }, 'techniques', i18n.t).visual_level).toEqual({ kind: 'stage', value: 'foundation_late' })
    expect(cultivationPresentation({ id: 't', name: 'Method', definition: { grade: 'top' } }, 'techniques', i18n.t).visual_level).toBeUndefined()
  })
})


it('reserves the square frame for actual art, not an empty or failed illustration', () => {
  const view = render(<EntryArtwork entry={entry} />)
  expect(view.container.querySelector('figure')).toHaveClass('entry-artwork--empty')
  view.rerender(<EntryArtwork entry={{ ...entry, art: { source: 'preset', preset_id: 'space-equipment' } }} />)
  expect(view.container.querySelector('figure')).not.toHaveClass('entry-artwork--empty')
  fireEvent.error(view.container.querySelector('img')!)
  expect(view.container.querySelector('figure')).toHaveClass('entry-artwork--empty')
})

it('renders a dedicated catalog illustration and degrades without borrowing another image', async () => {
  await i18n.changeLanguage('en-US')
  const hash = 'ab'.repeat(32)
  const art = { source: 'catalog' as const, catalog_id: hash }
  expect(artworkSource(art)).toEqual({ url: `/api/catalog-art/${hash}.png` })
  const view = render(<EntryArtwork entry={{ ...entry, art }} />)
  expect(view.container.querySelector('figcaption')).toBeNull()
  expect(screen.getByRole('img')).not.toHaveStyle({ backgroundSize: '300% 300%' })
  fireEvent.error(view.container.querySelector('img')!)
  expect(screen.getByText('No illustration yet')).toBeVisible()
  view.rerender(<EntryArtwork entry={{ ...entry, art: { ...art, catalog_id: 'cd'.repeat(32) } }} />)
  expect(screen.getByRole('img')).toBeVisible()
})

it.each(['../secret', 'https://example.com/a.png', 'AB'.repeat(32), 'a'.repeat(63), 'a'.repeat(64) + '.png'])('rejects unsafe catalog identity %s', catalog_id => {
  expect(artworkSource({ source: 'catalog', catalog_id })).toBeNull()
})

it('rejects mixed catalog and project image scopes', () => {
  expect(artworkSource({ source: 'catalog', catalog_id: 'a'.repeat(64), project_id: 'other' })).toBeNull()
})
