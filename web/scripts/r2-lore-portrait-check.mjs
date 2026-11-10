/**
 * R2 page check: the author's lore library shows each person's shipped face.
 *
 * The shipped world book carries no image field — 101 people, not one picture —
 * so nothing in the stored entries can answer "what does this person look
 * like". The server derives the face on every read from the world's own
 * catalog, and this script only believes what the page draws.
 *
 * Run against an isolated stack on TA_R2_BASE with a workspace that has had
 * the fanren-jie pack applied.
 */
import { chromium } from '@playwright/test'
import { writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { devOutput } from '../../scripts/dev-output.mjs'

const BASE = process.env.TA_R2_BASE || 'http://127.0.0.1:19734'
const PROJECT = process.env.TA_R2_PROJECT || ''
const OUT = process.env.TA_R2_RESULT || devOutput('r2/result.json')
const ART = process.env.TA_R2_ART || devOutput('r2/shots')
mkdirSync(ART, { recursive: true })

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
  flush()
}
function flush() {
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
    viewport: { width: 1440, height: 950 },
  })
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', (e) => pageErrors.push(e.message))

  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)
  const skip = page.getByRole('button', { name: '跳过引导' }).first()
  if (await skip.isVisible().catch(() => false)) {
    await skip.click()
    await page.waitForTimeout(1000)
  }

  // --- The author's lore library, reached the way the author reaches it ---
  const loreNav = page.getByRole('button', { name: '资料库', exact: true }).first()
  record('从工作台进入资料库', await loreNav.isVisible().catch(() => false), '活动栏「资料库」可见')
  if (!(await loreNav.isVisible().catch(() => false))) return finish()
  await loreNav.click()
  await page.waitForTimeout(4500)

  // The lore directory is the pane that owns the library search box; the
  // activity bar and other panes share the same class.
  const sidebar = page.getByPlaceholder('搜索资料').locator('xpath=ancestor::div[contains(@class,"nova-sidebar")][1]')
  const rows = sidebar.locator('img')
  // The library is read after the panel mounts, so the rows arrive late.
  await rows.first().waitFor({ state: 'attached', timeout: 30000 }).catch(() => {})
  const rowCount = await rows.count()
  record('资料库目录行画出人物头像', rowCount >= 90, `${rowCount} 行带头像`)

  const rowSrcs = await rows.evaluateAll((els) => els.map((e) => e.getAttribute('src')))
  const allPortraitRoute = rowSrcs.length > 0 && rowSrcs.every((src) => src && src.startsWith('/api/portraits/'))
  record('目录头像都走默认头像路由', allPortraitRoute, rowSrcs[0] || '无 src')

  // A 20px row that silently failed to load would still count as an element.
  const loaded = await rows.evaluateAll((els) => els.filter((e) => e.naturalWidth > 0).length)
  record('目录头像确实解码成功', loaded === rowCount && rowCount > 0, `${loaded}/${rowCount} 张 naturalWidth > 0`)

  const box = await sidebar.boundingBox()
  if (box) {
    await page.screenshot({ path: join(ART, 'r2-01-directory.png'), clip: { x: box.x, y: box.y + 90, width: box.width, height: 460 } })
  }

  // --- The entry's own page ---
  await rows.first().click({ force: true })
  await page.waitForTimeout(1500)
  // The detail picture is the display-sized derivative; the roster rows use the
  // 128px one, so this selector names the entry page and nothing else.
  const bigFace = page.locator('img[src*="size=display"]').first()
  const bigVisible = await bigFace.isVisible().catch(() => false)
  record('人物详情画出大图头像', bigVisible, bigVisible ? '详情顶部有大图' : '未找到大图')
  if (bigVisible) {
    const src = await bigFace.getAttribute('src')
    const natural = await bigFace.evaluate((e) => ({ w: e.naturalWidth, h: e.naturalHeight }))
    record('详情头像用显示尺寸而不是缩略图', String(src).includes('size=display'), String(src))
    record('详情头像实际解码成功', natural.w >= 192 && natural.h >= 192, `${natural.w}x${natural.h}`)
    await page.screenshot({ path: join(ART, 'r2-02-detail.png') })
  }

  const defaultCaption = await page.getByText('世界自带的默认头像').first().isVisible().catch(() => false)
  record('页面说明这是世界自带的默认头像', defaultCaption, defaultCaption ? '说明可见' : '未见说明')
  const emptyLabel = await page.getByText('暂无图片').first().isVisible().catch(() => false)
  record('有默认头像时不显示「暂无图片」', !emptyLabel, emptyLabel ? '仍显示占位文案' : '未显示')

  // --- The clear control must not promise something it cannot do ---
  const openImage = page.getByRole('button', { name: '打开图片生成' }).first()
  const openVisible = await openImage.isVisible().catch(() => false)
  record('详情给出图片生成入口', openVisible, openVisible ? '入口可见' : '未找到')
  if (openVisible) {
    await openImage.click()
    await page.waitForTimeout(900)
    const clearBtn = page.getByRole('button', { name: '清除图片' }).first()
    const clearVisible = await clearBtn.isVisible().catch(() => false)
    const clearDisabled = clearVisible ? await clearBtn.isDisabled() : false
    record('默认头像不可被「清除图片」删除', clearVisible && clearDisabled, clearVisible ? (clearDisabled ? '按钮禁用' : '按钮可点') : '未找到按钮')
    await page.screenshot({ path: join(ART, 'r2-03-image-dialog.png') })
    await page.keyboard.press('Escape')
    await page.waitForTimeout(600)
  }

  // --- A person the world never authored keeps the placeholder ---
  const createButton = sidebar.getByRole('button', { name: /新建.*角色/ }).first()
  const createVisible = await createButton.isVisible().catch(() => false)
  record('资料库给出新建人物入口', createVisible, createVisible ? '入口可见' : '未找到')
  // The page's own 新建 button names every new entry 新角色, which collides on a
  // second run. The entry is created through the same API the button calls, with
  // a name of this run's own, so what is asserted below is only how the page
  // draws a person no world authored.
  const invented = `自写人物-${Date.now().toString(36)}`
  const created = await page.evaluate(async ({ projectId, name }) => {
    const res = await fetch(`/api/projects/${encodeURIComponent(projectId)}/book/lore/items`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        enabled: true, type: 'character', name, importance: 'important', load_mode: 'auto',
        tags: [], brief_description: '这一条由作者自己写，不属于任何世界包。', content: '## ' + name + '\n\n',
      }),
    })
    const body = await res.json()
    return { ok: res.ok, id: body.id, portrait: body.portrait_url || '' }
  }, { projectId: PROJECT, name: invented })
  record('新建自写人物', created.ok, created.ok ? created.id : '创建失败')
  if (created.ok) {
    record('自写人物没有被配上默认头像', !created.portrait, created.portrait || '服务端未给默认图')
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(4500)
    const again = page.getByPlaceholder('搜索资料').locator('xpath=ancestor::div[contains(@class,"nova-sidebar")][1]')
    const row = again.getByText(invented, { exact: true }).first()
    await row.click()
    await page.waitForTimeout(1800)
    const placeholder = await page.getByText('暂无图片').first().isVisible().catch(() => false)
    const noFace = (await page.locator('img[src*="size=display"]').count()) === 0
    record('自写人物保留「暂无图片」占位', placeholder && noFace, `占位=${placeholder} 无默认图=${noFace}`)
    await page.screenshot({ path: join(ART, 'r2-04-invented-person.png') })
  }

  record('页面无未捕获异常', pageErrors.length === 0, pageErrors.slice(0, 3).join(' | '))
  await browser.close()
  finish()
}

main().catch((error) => {
  record('脚本运行', false, error.message)
  finish(1)
})
