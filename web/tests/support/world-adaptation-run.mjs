import { chromium, expect } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
const base = process.env.TA_UI_BASE_URL || 'http://127.0.0.1:19961'
const output = path.resolve(process.env.TA_UI_EVIDENCE_DIR || '../.ta-dev/world-adaptation-070-20261106/browser')
const executablePath = process.env.TA_CHROMIUM_EXECUTABLE_PATH || path.resolve('../.ta-dev/playwright/chromium-1234/chrome-win64/chrome.exe')
const worlds = (process.env.TA_WORLD_CASES || 'fantasy,space,wuxia,cultivation,author,cultivation-author').split(',')
const widths = (process.env.TA_UI_WIDTHS || '375,1280').split(',').map(Number)
const themes = (process.env.TA_UI_THEMES || 'dark,light').split(',')
const labels = {
  cultivation: { 'zh-CN': ['修仙交战','秘境','功法','道侣'], 'en-US': ['Cultivation combat','Secret realms','Cultivation methods','Dao companion'] },
  wuxia: { 'zh-CN': ['江湖交锋','江湖探索','武学','伴侣'], 'en-US': ['Martial encounter','Jianghu exploration','Martial arts','Partner'] },
  space: { 'zh-CN': ['星际交战','设施探索','专长与技术','伴侣'], 'en-US': ['Space encounter','Facility exploration','Expertise and technology','Partner'] },
  fantasy: { 'zh-CN': ['诸境交锋','遗迹探索','技艺与法术','伴侣'], 'en-US': ['Realms encounter','Ruin exploration','Arts and magic','Partner'] },
  author: { 'zh-CN': ['学院决斗','魔法调查','咒语','伴侣'], 'en-US': ['School duels','Magical investigation','Spells','Partner'] },
}
labels['cultivation-author'] = labels.cultivation
await mkdir(output, { recursive: true })
const browser = await chromium.launch({ executablePath, headless: true, args: ['--disable-features=LocalNetworkAccessChecks,LocalNetworkAccessRestrictions'] })
const results = []
try {
  for (const world of worlds) for (const width of widths) for (const theme of themes) {
    const page = await browser.newPage({ viewport: { width, height: 900 }, reducedMotion: 'reduce' })
    page.setDefaultTimeout(12000)
    const errors = [], requests = [], writes = []
    page.on('pageerror', error => errors.push(String(error)))
    await page.route('**/api/**', async route => {
      const request = route.request(), url = new URL(request.url()), pathname = url.pathname
      requests.push(pathname)
      if (request.method() !== 'GET') writes.push({ method: request.method(), pathname })
      const current = new URL(page.url()), fixtureWorld = current.searchParams.get('world') || world
      const family = fixtureWorld === 'author' ? 'fantasy' : fixtureWorld === 'cultivation-author' ? 'cultivation' : fixtureWorld
      const english = current.searchParams.get('locale') === 'en-US'
      const storyId = 'world-ui-' + fixtureWorld, name = english ? 'Mira' : '米拉'
      const character = { id: 'mira', name, known_as: name, present: true, channel: 'scene', unread_messages: 0, open_matters: 0, affinity: 80, affinity_band: 'close', relationship: { kind: 'partner' }, last_exchange: english ? 'I remember our agreement.' : '我记得我们的约定。', last_world_time: '1' }
      const scene = { node_id: 'first', title: english ? 'Crossroads' : '路口', location: 'crossroads', world_time: '1', present: ['mira'] }
      const group = { id: 'radio', name: english ? 'Remote council' : '远程小组', type: 'self', channel: 'message', contact: 'radio', reachable_members: 1, version: 1, members: [{ id: 'mira', name, present: false, reachable: true }, { id: 'away', name: english ? 'Disconnected member' : '失联队员', present: false, reachable: false }], last_exchange: english ? 'Channel established.' : '频道已经建立。' }
      let json = {}
      if (pathname.endsWith('/adventure-window')) json = { story_id: storyId, branch_id: 'main', head: 'fixture-head', family, actor: { health: 80, max_health: 100, energy: 30, max_energy: 50, injury: 0, skills: { basic: 1 }, training: {}, inventory: {}, assets: {}, currency: 80, acquired: {}, defeated: {} }, skills: [{ id: 'basic', name: english ? 'Basic technique' : '基本技艺', kind: 'attack', scale: 'person', cost: 2, power: 3 }], items: [], craft_methods: [] }
      else if (pathname.endsWith('/combat-window')) json = { story_id: storyId, branch_id: 'main', scene, sessions: [], encounter: null }
      else if (pathname.endsWith('/cultivation-entry-details')) json = { techniques: [], skills: [], items: [], craft_methods: [] }
      else if (pathname.endsWith('/characters')) json = { story_id: storyId, branch_id: 'main', character_id: url.searchParams.get('character_id') || '', scene, characters: [character], pending: [], exchanges: [], matters: [], messages: [], has_more: false, communication_mode: family === 'space' ? 'radio' : family === 'cultivation' ? 'cultivation-signal' : 'letter', radio_available: true }
      else if (pathname.endsWith('/groups')) json = { story_id: storyId, branch_id: 'main', group_id: url.searchParams.get('group_id') || '', scene, groups: family === 'space' ? [group] : [], cast: [character], exchanges: [], matters: [], has_more: false }
      else if (pathname.includes('/realm')) json = { story_id: storyId, branch_id: 'main', instances: [], available: [], current_scene: 'crossroads', clock: { seconds: 0 } }
      else if (pathname.endsWith('/player-attention')) json = { v: 1, head: 'fixture-head', characters: [], quests: [] }
      else if (pathname.endsWith('/conversation-config')) json = { agent_kind: 'interactive_story', profile_id: 'default', thinking_level: 'off', approval_mode: 'write', revision: 1, runtime: { kind: 'native' } }
      else if (pathname.endsWith('/chat/active')) json = { active: false }
      else if (pathname.endsWith('/settings')) json = { effective: { speech: { endpoint: '', model: '', voice: '', api_key: '', speed: 1 }, agent_models: {}, theme, locale: english ? 'en-US' : 'zh-CN' }, user: {}, workspace: {}, revisions: {}, resolved_agent_tool_manifests: {} }
      else if (pathname.includes('/skills')) json = { skills: [] }
      else if (pathname.includes('/profiles')) json = { profiles: [], default_profile_id: '' }
      else if (pathname.endsWith('/models')) json = { models: [] }
      else if (pathname.includes('/image-model')) json = { configured: false }
      else if (pathname.includes('/picture')) json = { overridden: false, source: '' }
      else if (pathname.endsWith('/history')) json = { turns: [], has_more: false }
      else if (pathname.endsWith('/agents')) json = { agents: [] }
      await route.fulfill({ json })
    })
    const id = world + '-' + width + '-' + theme
    try {
      await page.goto(base + '/tests/support/world-adaptation-harness.html?world=' + world + '&theme=' + theme + '&locale=zh-CN')
      // This clicks the actual StoryStage toolbar; it would fail under the original fantasy misrouting.
      for (const locale of ['zh-CN', 'en-US']) {
        await page.getByLabel('Fixture language').selectOption(locale)
        const [combat, exploration, techniques, partner] = labels[world][locale]
        const intimacyTitle = locale === 'zh-CN' ? '双修' : 'Dual Cultivation'
        if (world === 'cultivation' || world === 'cultivation-author') await expect(page.getByTitle(intimacyTitle, { exact: true }).first()).toBeVisible()
        else await expect(page.getByTitle(intimacyTitle, { exact: true })).toHaveCount(0)
        await expect(page.getByTitle(combat, { exact: true }).first()).toBeVisible()
        await page.getByTitle(combat, { exact: true }).first().click()
        const native = world === 'cultivation' || world === 'cultivation-author'
        const battle = page.getByTestId(native ? 'combat-window-panel' : 'adventure-combat')
        await expect(battle).toBeVisible()
        if (!native) {
          await expect(battle.getByRole('heading', { name: combat, exact: true })).toBeVisible()
          if (requests.some(pathname => pathname.endsWith('/combat-window'))) throw new Error('Adventure world requested cultivation combat endpoint')
          if (!requests.some(pathname => pathname.endsWith('/adventure-window'))) throw new Error('Adventure window was not requested')
          if (world === 'author') await expect(battle).toContainText(locale === 'zh-CN' ? '魔力' : 'Magic')
        }
        if (world === 'fantasy' || world === 'author') { await page.waitForTimeout(350); await page.screenshot({ path: path.join(output, id + '-' + locale + '-battle.png') }) }
        // Remount only the main scene, not the locale/provider; fixture navigation closes existing windows.
        await page.keyboard.press('Escape'); await page.getByRole('button', { name: 'Owned details', exact: true }).click()
        const owned = page.getByTestId('owned-entry-details')
        await expect(owned).toContainText(techniques)
        await owned.getByRole('button').first().click()
        await expect(owned).not.toContainText(locale === 'zh-CN' ? '载入失败' : 'Failed to load')
        await page.getByRole('button', { name: 'Main game', exact: true }).click()
        await page.getByTitle(exploration, { exact: true }).first().click()
        const realm = page.getByTestId('realm-panel')
        await expect(realm.getByRole('heading', { name: exploration, exact: true })).toBeVisible()
        await expect(realm).toContainText(exploration)
        if (world !== 'cultivation' && world !== 'cultivation-author' && locale === 'zh-CN') await expect(realm).not.toContainText('秘境')
        await page.keyboard.press('Escape'); await page.getByRole('button', { name: 'Owned details', exact: true }).click()
        await page.getByRole('button', { name: 'Main game', exact: true }).click()
        await page.getByTitle(locale === 'zh-CN' ? '聊天' : 'Chat', { exact: true }).first().click()
        await page.getByText(locale === 'zh-CN' ? '米拉' : 'Mira', { exact: true }).first().click()
        await expect(page.getByTestId('character-window-affinity')).toContainText(partner)
        await page.keyboard.press('Escape'); await page.getByRole('button', { name: 'Owned details', exact: true }).click()
        await page.getByRole('button', { name: 'Main game', exact: true }).click()
        if (world === 'space') {
          await page.getByTitle(locale === 'zh-CN' ? '聊天' : 'Chat', { exact: true }).first().click()
          await page.getByText(locale === 'zh-CN' ? '远程小组' : 'Remote council', { exact: true }).first().click()
          await expect(page.locator('body')).toContainText(locale === 'zh-CN' ? '1 人已接通' : '1 connected')
          const body = await page.locator('body').innerText()
          if (locale === 'zh-CN' && (/1 人在场|围坐下来/.test(body))) throw new Error('Radio participants described as physically present')
          await page.waitForTimeout(350); await page.screenshot({ path: path.join(output, id + '-' + locale + '-radio.png') })
          await page.keyboard.press('Escape'); await page.getByRole('button', { name: 'Owned details', exact: true }).click()
          await page.getByRole('button', { name: 'Main game', exact: true }).click()
        }
        if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1)) throw new Error('Horizontal overflow')
        if (errors.length) throw new Error(errors.join('\n'))
      }
      if (writes.length) throw new Error('Unexpected fixture state mutation: ' + JSON.stringify(writes))
      results.push({ world, width, theme, passed: true, locales: ['zh-CN', 'en-US'], checks: ['real-main-battle-route','exploration','owned-ability-title','partner-label','live-language-switch','no-overflow','no-page-errors'], requests: [...new Set(requests)] })
      console.log('PASS ' + id)
    } catch (error) {
      await page.screenshot({ path: path.join(output, id + '-failure.png'), fullPage: true }).catch(() => {})
      await writeFile(path.join(output, id + '-failure.txt'), String(error) + '\n\n' + await page.locator('body').innerText().catch(() => '') + '\n\nPage errors: ' + JSON.stringify(errors) + '\nRequests: ' + JSON.stringify(requests))
      results.push({ world, width, theme, passed: false, error: String(error), errors, requests })
      throw error
    } finally { await page.close(); await writeFile(path.join(output, 'world-adaptation-results.json'), JSON.stringify(results, null, 2)) }
  }
  console.log(JSON.stringify({ passed: results.length, output }))
} finally { await browser.close() }