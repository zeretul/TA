/**
 * R1 page check: the records overview follows the branch the reader is on.
 *
 * The overview reads four endpoints at once. A branch switch used to clear the
 * screen without retiring the read already in flight, so that read kept the
 * newest sequence number and painted the branch just left over the branch now
 * on screen. The unit suite pins the hook; this script pins what a player sees.
 *
 * The two branches differ in exactly one readable thing: `main` has accepted
 * 分清这一批药材 and the other branch has not. That difference is made through
 * the quest API before this script runs, so the only thing on trial here is
 * whether the overview repaints it after a switch.
 *
 * Run against an isolated stack on TA_R1_BASE whose story has a branch with
 * no accepted task while the main line has one.
 */
import { chromium } from '@playwright/test'
import { writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_R1_BASE || 'http://127.0.0.1:19730'
const STORY = process.env.TA_R1_STORY || ''
const OTHER_BRANCH = process.env.TA_R1_OTHER_BRANCH || ''
const OTHER_TITLE = process.env.TA_R1_OTHER_TITLE || '另一条路'
const QUEST_TITLE = process.env.TA_R1_QUEST || '分清这一批药材'
const OUT = process.env.TA_R1_RESULT || devOutput('r1branch3/result.json')
const ART = process.env.TA_R1_ART || devOutput('r1branch3/shots')
mkdirSync(ART, { recursive: true })

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
  flush()
}
function flush() {
  writeFileSync(OUT, JSON.stringify({ story: STORY, otherBranch: OTHER_BRANCH, failed, steps }, null, 2))
}
function finish(code) {
  flush()
  for (const step of steps) console.log(`${step.ok ? 'PASS' : 'FAIL'}  ${step.name}${step.detail ? ' — ' + step.detail : ''}`)
  console.log(failed === 0 ? '\nALL PASS' : `\n${failed} FAILED`)
  process.exit(code ?? (failed === 0 ? 0 : 1))
}

/** The branch the server says the story is on, so the page and the data agree. */
async function currentBranch(page) {
  const body = await page.evaluate(async (storyId) => {
    const res = await fetch(`/api/interactive/stories/${encodeURIComponent(storyId)}/branches`)
    return res.json()
  }, STORY)
  const rows = Array.isArray(body?.branches) ? body.branches : []
  return rows.find((row) => row.current)?.id || ''
}

async function main() {
  if (!STORY || !OTHER_BRANCH) throw new Error('TA_R1_STORY and TA_R1_OTHER_BRANCH must name the story and its other branch')

  const browser = await chromium.launch()
  const context = await browser.newContext({
    baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce',
    // Wide enough for the prose, the docked overview and the console at once:
    // all three are on screen during the switch, which is the point.
    viewport: { width: 1600, height: 950 },
  })
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', (e) => pageErrors.push(e.message))

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)
  const skip = page.getByRole('button', { name: '跳过引导' }).first()
  if (await skip.isVisible().catch(() => false)) {
    await skip.click()
    await page.waitForTimeout(1200)
  }

  const gameNav = page.getByRole('button', { name: '游戏', exact: true }).first()
  record('从游戏入口进入', await gameNav.isVisible().catch(() => false), '活动栏「游戏」')
  if (!(await gameNav.isVisible().catch(() => false))) return finish()
  await gameNav.click()
  await page.waitForTimeout(4000)

  const panel = page.locator('[data-testid="lore-overview-panel"]').first()
  const mattersTab = page.getByRole('tab', { name: '见闻与事务' }).first()
  const overviewTrigger = page.locator('[data-lore-overview-trigger]').first()

  // --- The branch this story starts on ---
  const startBranch = await currentBranch(page)
  record('页面加载的故事在主线分支上', startBranch === 'main', startBranch || '未读到当前分支')
  if (startBranch !== 'main') return finish()

  await overviewTrigger.click()
  await page.waitForTimeout(2500)
  record('资料总览可打开', await panel.isVisible().catch(() => false), '面板可见')
  await mattersTab.click()
  // The read is four requests; the list arrives after them, not with the click.
  await panel.getByText(QUEST_TITLE, { exact: true }).first().waitFor({ state: 'visible', timeout: 15000 }).catch(() => {})
  const beforeText = await panel.innerText()
  record('主线分支的总览列出已接的事', beforeText.includes(QUEST_TITLE), beforeText.includes(QUEST_TITLE) ? QUEST_TITLE : '未列出')
  await page.screenshot({ path: join(ART, 'r1-01-main-matters.png') })

  // --- Switch branches with the overview on screen ---
  const routesTab = page.getByRole('tab', { name: '路线' }).first()
  record('游戏控制台给出路线分区', await routesTab.isVisible().catch(() => false), '「路线」标签')
  if (!(await routesTab.isVisible().catch(() => false))) return finish()
  await routesTab.click()
  await page.waitForTimeout(1500)

  const otherRow = page.getByRole('button', { name: new RegExp(OTHER_TITLE) }).first()
  record('路线里能看到另一条分支', await otherRow.isVisible().catch(() => false), OTHER_TITLE)
  if (!(await otherRow.isVisible().catch(() => false))) return finish()
  await otherRow.click()
  // The switch writes, reloads the snapshot, then the overview re-reads.
  await page.waitForTimeout(6000)

  const switchedBranch = await currentBranch(page)
  record('点击后故事确实切到另一条分支', switchedBranch === OTHER_BRANCH, switchedBranch || '未读到当前分支')

  const afterText = await panel.innerText()
  record(
    '切换分支后总览不再显示旧分支的事',
    !afterText.includes(QUEST_TITLE),
    afterText.includes(QUEST_TITLE) ? '仍显示旧分支的任务' : '旧分支任务已消失',
  )
  record('切换后总览仍然是打开且可读的', await panel.isVisible().catch(() => false), '面板未关闭、未白屏')
  await page.screenshot({ path: join(ART, 'r1-02-other-branch-matters.png') })

  // A stale read that lands late is the failure this guards against, so give it
  // more than enough time to arrive and check again.
  await page.waitForTimeout(4000)
  const lateText = await panel.innerText()
  record(
    '迟到的旧读取没有回填到屏幕上',
    !lateText.includes(QUEST_TITLE),
    lateText.includes(QUEST_TITLE) ? '四秒后旧分支任务重新出现' : '四秒后仍无旧分支任务',
  )

  // --- And back: the switch must work in both directions ---
  const mainRow = page.getByRole('button', { name: /主线/ }).first()
  const mainVisible = await mainRow.isVisible().catch(() => false)
  record('路线里能切回主线', mainVisible, mainVisible ? '「主线」可见' : '未找到')
  if (mainVisible) {
    await mainRow.click()
    await page.waitForTimeout(6000)
    const backBranch = await currentBranch(page)
    record('切回后故事确实回到主线', backBranch === 'main', backBranch || '未读到当前分支')
    const backText = await panel.innerText()
    record('切回主线后总览重新列出这件事', backText.includes(QUEST_TITLE), backText.includes(QUEST_TITLE) ? QUEST_TITLE : '未重新列出')
    await page.screenshot({ path: join(ART, 'r1-03-back-to-main.png') })
  }

  record('页面无未捕获异常', pageErrors.length === 0, pageErrors.slice(0, 3).join(' | '))
  await browser.close()
  finish()
}

main().catch((error) => {
  record('脚本运行', false, error.message)
  finish(1)
})
