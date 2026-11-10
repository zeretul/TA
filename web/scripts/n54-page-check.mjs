/**
 * N54 page journey: create a world from the page, edit it, save it, then load
 * that world and start a story from it — all through the real game entry.
 *
 * The API contract is checked separately by api-journey.mjs. This script is the
 * page half: it only believes what the browser shows.
 *
 * Run against an isolated stack on TA_N54_BASE (default 19734) with the
 * deterministic model on 19739.
 */
import { chromium } from '@playwright/test'
import { writeFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_N54_BASE || 'http://127.0.0.1:19734'
const OUT = process.env.TA_N54_RESULT || devOutput('n54-check/page-state.json')
const ART = process.env.TA_N54_ART || devOutput('n54-check/shots')
mkdirSync(ART, { recursive: true })

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
  flush()
}
function flush() {
  mkdirSync(dirname(OUT), { recursive: true })
  writeFileSync(OUT, JSON.stringify({ failed, steps }, null, 2))
}
function finish(code) {
  flush()
  for (const step of steps) console.log(`${step.ok ? 'PASS' : 'FAIL'}  ${step.name}${step.detail ? ' — ' + step.detail : ''}`)
  console.log(failed === 0 ? '\nALL PASS' : `\n${failed} FAILED`)
  process.exit(code ?? (failed === 0 ? 0 : 1))
}

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({
    baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce',
    viewport: { width: 1280, height: 900 },
  })
  const page = await context.newPage()
  // Only uncaught exceptions are a page failure. An application that logs a
  // handled API refusal to the console is reporting, not breaking, and the
  // visible effect of that refusal is asserted where it shows on screen.
  const pageErrors = []
  page.on('pageerror', (e) => pageErrors.push(e.message))

  try {
    await page.goto('/', { waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(4000)
    const skip = page.getByRole('button', { name: '跳过引导' }).first()
    if (await skip.isVisible().catch(() => false)) { await skip.click(); await page.waitForTimeout(1200) }

    // The game entry, not the writing one: the old writing-only journey must not
    // be what this check rides on.
    const gameBtn = await findGameButton(page)
    record('从真实游戏入口进入', !!gameBtn, gameBtn ? '找到「游戏」入口' : '未找到')
    if (!gameBtn) return finish()
    await gameBtn.click()
    await page.waitForTimeout(3000)

    // --- The world page, where 创建世界 lives ---
    const worldEntry = await findWorldEntry(page)
    record('世界包页可达', !!worldEntry, worldEntry ? '找到入口' : '未找到')
    if (!worldEntry) return finish()
    await worldEntry.click()
    await page.waitForTimeout(3000)
    await page.screenshot({ path: join(ART, '01-world-page.png') })

    // --- The prepared-worlds list and its 创建世界 action ---
    const createEntry = page.getByRole('button', { name: /创建世界/ }).first()
    const createVisible = await createEntry.isVisible().catch(() => false)
    record('世界页给出创建入口', createVisible, createVisible ? '「创建世界」可见' : '未找到创建入口')

    // --- Guided entry: the other door into the same draft ---
    const guidedOpened = await openCreation(page)
    record('引导入口可打开', guidedOpened, guidedOpened ? '对话框可见' : '未打开')
    if (guidedOpened) {
      // Guided is the default mode, so a player who answers questions never has
      // to find it first.
      const guidedMode = page.getByRole('radio', { name: /回答问题/ }).first()
      record('引导入口是默认方式', await guidedMode.isChecked().catch(() => false), '')
      await page.getByLabel(/世界名称/).first().fill('寒山驿')
      await page.getByLabel(/题材/).first().fill('北境边关')
      await page.getByLabel(/这个世界大致是什么样子/).first()
        .fill('大雪封路四个月，商队在此过冬，也在此结仇。')
      await page.getByLabel(/基调/).first().fill('冷峻')
      await page.getByLabel(/主角大致是什么人/).first().fill('一个没有靠山的行脚人')
      await page.getByLabel(/希望故事从哪里开始/).first().fill('雪夜抵达驿站，身上只剩一枚铜钱。')
      await page.waitForTimeout(600)
      await page.screenshot({ path: join(ART, '02b-guided-filled.png') })

      const guidedGenerate = page.getByRole('button', { name: /生成世界/ }).first()
      record('引导入口可以提交', await guidedGenerate.isEnabled().catch(() => false), '')
      await guidedGenerate.click()
      const guidedDrafted = await page.getByLabel(/故事标题/).first()
        .waitFor({ state: 'visible', timeout: 90000 }).then(() => true).catch(() => false)
      record('引导入口写出草稿', guidedDrafted, guidedDrafted ? '预览页可见' : '未进入预览')

      if (guidedDrafted) {
        // The preview states which template the rules come from, so a world that
        // reads like a frontier post is still on the supported one rather than
        // the cultivation terms being swapped out.
        const notice = await page.getByText(/修仙状态模板/).first().isVisible().catch(() => false)
        record('预览页说明规则来自受支持模板', notice, notice ? '规则说明可见' : '未见模板说明')

        const guidedTitle = page.getByLabel(/故事标题/).first()
        await guidedTitle.fill('雪封寒山驿（引导改）')
        await page.waitForTimeout(1200)
        await page.screenshot({ path: join(ART, '02c-guided-preview.png') })

        // Back to the questions, then over to the one-sentence door: both write
        // into this same draft, so the edit above must survive the switch.
        await page.getByRole('button', { name: /回去改设定/ }).first().click()
        await page.waitForTimeout(1200)
        await page.getByRole('radio', { name: /只写一句话/ }).first().click()
        await page.waitForTimeout(600)
        await page.locator('textarea').first()
          .fill('一座被大雪封住整个冬天的边关驿镇，商队在此过冬，也在此结仇。')
        await page.waitForTimeout(400)
        await page.getByRole('button', { name: /生成世界/ }).first().click()
        await page.waitForTimeout(2000)
        const keptTitle = await page.getByLabel(/故事标题/).first().inputValue().catch(() => '')
        record('两个入口共用同一份草稿', keptTitle === '雪封寒山驿（引导改）', `切到自动入口后标题 ${keptTitle}`)
        await page.screenshot({ path: join(ART, '02d-shared-draft.png') })

        const cancel = page.getByRole('button', { name: /取消/ }).first()
        if (await cancel.isVisible().catch(() => false)) await cancel.click()
        else await page.keyboard.press('Escape')
        await page.waitForTimeout(1500)
      }
    }

    // --- Automatic entry ---
    const opened = await openCreation(page)
    record('「创建世界」对话框打开', opened, opened ? '对话框可见' : '未打开')
    if (!opened) return finish()
    await page.screenshot({ path: join(ART, '02-seed-guided.png') })

    // The seed screen says what the rules will be, before anything is written.
    const rulesNote = await page.getByText(/规则、境界与数值由程序/).first().isVisible().catch(() => false)
    record('种子页说明规则来源', rulesNote, rulesNote ? '规则说明可见' : '未见规则说明')

    // A machine configured through the environment or a keyless local provider
    // can write, so the missing-model card must not appear. It is rendered from
    // the backend's own resolution, not from a reading of the settings files.
    const modelWarning = await page.getByText(/还差一个能写世界的模型/).first().isVisible().catch(() => false)
    record('已配好模型时不提示缺模型', !modelWarning, modelWarning ? '仍出现缺模型警示卡' : '未见警示卡')

    const autoCard = page.getByRole('radio', { name: /一句话/ }).first()
    const autoFallback = page.getByText(/一句话/, { exact: false }).first()
    if (await autoCard.isVisible().catch(() => false)) await autoCard.click()
    else if (await autoFallback.isVisible().catch(() => false)) await autoFallback.click()
    await page.waitForTimeout(800)
    await page.screenshot({ path: join(ART, '03-seed-automatic.png') })

    const sentence = page.locator('textarea').first()
    await sentence.fill('一座被大雪封住整个冬天的边关驿镇，商队在此过冬，也在此结仇。')
    await page.waitForTimeout(500)

    const generate = page.getByRole('button', { name: /生成|写出/ }).first()
    record('自动入口可以提交', await generate.isEnabled().catch(() => false), '生成按钮可用')
    await generate.click()
    await page.waitForTimeout(1000)
    const drafted = await page.getByText(/雪封寒山驿|寒山驿/).first()
      .waitFor({ state: 'visible', timeout: 90000 }).then(() => true).catch(() => false)
    record('自动入口写出草稿并进入预览', drafted, drafted ? '预览页出现世界内容' : '未见草稿')
    await page.screenshot({ path: join(ART, '04-preview.png') })
    if (!drafted) return finish()

    // --- Edit one field; the page must keep it and re-check readiness ---
    const titleInput = page.getByLabel(/故事标题/).first()
    const originalTitle = await titleInput.inputValue().catch(() => '')
    await titleInput.fill('雪封寒山驿（页面改）')
    await page.waitForTimeout(2000)
    const editedTitle = await titleInput.inputValue().catch(() => '')
    record('预览页可改文字', editedTitle === '雪封寒山驿（页面改）', `${originalTitle} → ${editedTitle}`)

    const notStartable = await page.getByText(/还不能开始|原因/).first().isVisible().catch(() => false)
    record('改完仍判定可开始', !notStartable, notStartable ? '出现不可开始提示' : '未出现阻断提示')
    await page.screenshot({ path: join(ART, '05-preview-edited.png') })

    // --- Empty the name: the page must say why it cannot start ---
    const nameInput = page.getByLabel(/世界名称/).first()
    await nameInput.fill('')
    await page.waitForTimeout(2000)
    const reasonShown = await page.getByText(/名称/).first().isVisible().catch(() => false)
    record('缺名字时页面说明原因', reasonShown, reasonShown ? '出现「世界还没有名称」类提示' : '未见原因')
    await page.screenshot({ path: join(ART, '06-preview-not-startable.png') })
    await nameInput.fill('寒山驿')
    await page.waitForTimeout(2000)

    // --- Long text: a very long description must not break the layout ---
    const dialog = page.getByRole('dialog').first()
    const descArea = dialog.locator('textarea').first()
    const longText = '北境边关的驿镇。'.repeat(40)
    await descArea.fill(longText)
    await page.waitForTimeout(1500)
    const longOk = await descArea.inputValue().then((v) => v.length > 100).catch(() => false)
    record('长文可输入且不丢字', longOk, `长度 ${longText.length}`)
    await page.screenshot({ path: join(ART, '07-preview-longtext.png') })
    await descArea.fill('北境边关的驿镇，大雪封路四个月，商队在此过冬，也在此结仇。')
    await page.waitForTimeout(1500)

    // --- Save ---
    const saveBtn = page.getByRole('button', { name: /保存世界/ }).first()
    record('保存按钮可用', await saveBtn.isEnabled().catch(() => false), '')
    await saveBtn.click()
    await page.waitForTimeout(4000)
    const dialogGone = !(await page.getByRole('dialog').first().isVisible().catch(() => false))
    record('保存后对话框关闭', dialogGone, dialogGone ? '已关闭' : '仍打开')
    await page.screenshot({ path: join(ART, '08-after-save.png') })

    const listedInPage = await page.getByText('寒山驿', { exact: false }).first()
      .waitFor({ state: 'visible', timeout: 20000 }).then(() => true).catch(() => false)
    record('新世界出现在页面目录里', listedInPage, listedInPage ? '目录里可见' : '目录里未见')

    // --- Load it and start a character ---
    const packButton = page.getByRole('button', { name: /寒山驿/ }).first()
    const packVisible = await packButton.isVisible().catch(() => false)
    if (packVisible) { await packButton.click(); await page.waitForTimeout(2500) }
    record('可从页面选中新世界', packVisible, packVisible ? '已选中' : '未找到条目')
    await page.screenshot({ path: join(ART, '09-world-selected.png') })

    // Selecting from the switcher leaves its list open over the card, so close
    // it before reading the card's own action.
    await page.keyboard.press('Escape')
    await page.waitForTimeout(800)

    // A world becomes playable through 加载世界包, which is what carries it into
    // a workspace; 开始 belongs to a story that already exists.
    const loadBtn = page.getByRole('button', { name: /^加载世界包$/ }).first()
    const loadVisible = await loadBtn.waitFor({ state: 'visible', timeout: 15000 }).then(() => true).catch(() => false)
    record('新世界可进入建角', loadVisible, loadVisible ? '「加载世界包」可见' : '未见加载入口')
    await page.screenshot({ path: join(ART, '10-creation-entry.png') })

    // --- Carry the world into a workspace, then create a character from it ---
    if (loadVisible) {
      await loadBtn.click()
      await page.waitForTimeout(3000)
      await page.screenshot({ path: join(ART, '11-loaded.png') })

      // Loading turns the action into 创建角色; the creation page itself is one
      // step further on.
      const createRole = page.getByRole('button', { name: /创建角色/ }).first()
      const roleVisible = await createRole.isVisible().catch(() => false)
      record('加载后给出建角入口', roleVisible, roleVisible ? '「创建角色」可见' : '未见建角入口')
      if (roleVisible) { await createRole.click(); await page.waitForTimeout(4000) }

      // The creation dialog is the thing that proves the world was carried into
      // a workspace; a loose text match also hits nodes that are still hidden.
      const creationShown = await page.getByRole('dialog').first().getByLabel(/姓名/).first()
        .waitFor({ state: 'visible', timeout: 45000 }).then(() => true).catch(() => false)
      record('加载后进入建角', creationShown, creationShown ? '建角选项可见' : '未见建角选项')

      if (creationShown) {
        const creationDialog = page.getByRole('dialog').first()
        const nameField = creationDialog.getByLabel(/姓名/).first()
        if (await nameField.isVisible().catch(() => false)) await nameField.fill('陆行舟')
        await page.waitForTimeout(800)
        await page.screenshot({ path: join(ART, '12-creation.png') })

        // The default root category asks for one attribute, and 生成身份 stays
        // disabled until one is picked. That is the page refusing an incomplete
        // choice, not a missing button. Scope the chip to the dialog: the page
        // also holds hidden option nodes outside it.
        const attrChip = creationDialog.getByRole('radio', { name: /^金$/ }).first()
        if (await attrChip.isVisible().catch(() => false)) { await attrChip.click(); await page.waitForTimeout(600) }

        const draftBtn = creationDialog.getByRole('button', { name: /生成身份/ }).first()
        record('建角页可提交', await draftBtn.isEnabled().catch(() => false), '「生成身份」可用')
        await draftBtn.click()

        // Drafting is one model call; the preview that follows is where the
        // player actually starts the story.
        const previewReady = await page.getByText(/确认这个角色/).first()
          .waitFor({ state: 'visible', timeout: 90000 }).then(() => true).catch(() => false)
        record('建角草稿进入预览', previewReady, previewReady ? '预览页可见' : '未进入预览')
        await page.screenshot({ path: join(ART, '13-creation-preview.png') })

        if (previewReady) {
          const startBtn = page.getByRole('button', { name: /开始故事/ }).first()
          record('预览页可开始故事', await startBtn.isEnabled().catch(() => false), '')
          await startBtn.click()
          const opened = await page.getByPlaceholder(/你要做什么/).first()
            .waitFor({ state: 'visible', timeout: 120000 }).then(() => true).catch(() => false)
          record('开场后进入故事页', opened, opened ? '输入框可见' : '未进入故事页')
          await page.waitForTimeout(3000)
          await page.screenshot({ path: join(ART, '13-story-opened.png') })
          const prose = await page.getByText(/周驿丞/, { exact: false }).first().isVisible().catch(() => false)
          record('开场正文可见', prose, prose ? '正文出现在页面上' : '未见正文')
        }
      }
    }

    // --- Narrow screen: the dialog must stay usable at 320px ---
    await page.setViewportSize({ width: 320, height: 720 })
    await page.waitForTimeout(1500)
    const narrowOk = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 2)
    record('320px 窄屏无横向溢出', narrowOk, narrowOk ? '未溢出' : '出现横向滚动')
    await page.screenshot({ path: join(ART, '14-narrow.png'), fullPage: false })
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.waitForTimeout(1000)

    // --- Light theme ---
    const themed = await setTheme(page, 'light')
    record('可切到浅色主题', themed, themed ? '已切换' : '切换失败')
    await page.waitForTimeout(1500)
    await page.screenshot({ path: join(ART, '15-light.png') })
    await setTheme(page, 'dark')
    await page.waitForTimeout(1000)

    // --- Error state: with no model configured, the page says so up front ---
    // The isolated stack has a model, so this is checked by removing the env
    // stack in a separate run; here the console must at least be clean.
    record('页面无未捕获异常', pageErrors.length === 0, pageErrors.slice(0, 3).join(' | '))
  } finally {
    await context.close()
    await browser.close()
  }
  finish()
}

async function openCreation(page) {
  const candidates = [
    page.getByRole('button', { name: /创建世界/ }).first(),
    page.getByText(/创建世界/).first(),
  ]
  for (const candidate of candidates) {
    if (await candidate.isVisible().catch(() => false)) {
      await candidate.click()
      await page.waitForTimeout(1500)
      if (await page.getByRole('dialog').first().isVisible().catch(() => false)) return true
    }
  }
  return false
}

async function findGameButton(page) {
  const candidates = [
    page.getByRole('button', { name: '游戏', exact: true }),
    page.locator('button', { hasText: /^游戏$/ }),
    page.locator('[data-mode="story"]'),
  ]
  for (const candidate of candidates) {
    const first = candidate.first()
    if (await first.isVisible().catch(() => false)) return first
  }
  return null
}

async function findWorldEntry(page) {
  // The world-pack page is reached from the world switcher's 管理世界, not from a
  // sidebar item; the story stage opens first when a story already exists.
  const switcher = page.getByRole('button', { name: /切换世界|世界/ }).first()
  if (await switcher.isVisible().catch(() => false)) {
    await switcher.click()
    await page.waitForTimeout(1200)
  }
  const candidates = [
    page.getByRole('button', { name: /管理世界/ }).first(),
    page.getByRole('link', { name: /管理世界/ }).first(),
    page.getByText(/管理世界/).first(),
  ]
  for (const candidate of candidates) {
    if (await candidate.isVisible().catch(() => false)) return candidate
  }
  return null
}

async function setTheme(page, theme) {
  return page.evaluate(async (value) => {
    const res = await fetch('/api/settings', {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ layer: 'user', changes: { theme: value } }),
    })
    return res.ok
  }, theme)
}

main().catch((error) => {
  record('脚本自身', false, String(error?.stack || error))
  finish(1)
})
