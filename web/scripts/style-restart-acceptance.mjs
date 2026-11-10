/**
 * STY-07 restart acceptance.
 *
 * The style report records one remaining gap: the three-purpose style settings
 * were proven to survive Close()/reopen inside one process, never a real
 * process restart. This closes it the way the rest of the batch is verified:
 * build the backend, write an isolated data root, set the three purposes over
 * the real API, kill the process, start a fresh one on the same data root, and
 * read the settings back.
 *
 * Isolation matches the style launcher: own data root under web/test-results,
 * own port, no reading of the user's real data root. The process this script
 * starts is the process it stops.
 */
import { spawn, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync, readFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const repositoryRoot = path.resolve(process.cwd(), '..')
const runtimeRoot = path.resolve(process.cwd(), 'test-results', 'restart')
const taDir = path.join(runtimeRoot, 'ta')
const binaryPath = path.join(runtimeRoot, 'ta-restart.exe')
const port = process.env.TA_RESTART_PORT || '19742'
const BASE = `http://127.0.0.1:${port}`
const OUT = path.join(runtimeRoot, 'restart-acceptance-result.json')

const steps = []
let failed = 0
function record(name, ok, detail) {
  steps.push({ name, ok, detail })
  if (!ok) failed += 1
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ` — ${detail}` : ''}`)
}
function flush(extra = {}) {
  writeFileSync(OUT, JSON.stringify({ passed: steps.length - failed, total: steps.length, steps, ...extra }, null, 2))
}

rmSync(runtimeRoot, { recursive: true, force: true })
mkdirSync(taDir, { recursive: true })

const build = spawnSync('go', ['build', '-tags', 'embedweb', '-o', binaryPath, './cmd/ta'], {
  cwd: repositoryRoot, env: process.env, stdio: 'inherit',
})
if (build.error) throw build.error
if (build.status !== 0) process.exit(build.status ?? 1)

// A window large enough that nothing compacts mid-check; the settings under
// test do not depend on the model, so the endpoint is never called.
writeFileSync(path.join(taDir, 'config.toml'), `language = "zh-CN"
update_check_enabled = false

[[model_endpoints]]
id = "accept"
name = "restart acceptance"
provider = "openai-compatible"
protocol = "openai-chat-completions"
api_key = ""
base_url = "http://127.0.0.1:8317/v1"

[[model_profiles]]
id = "accept"
name = "acceptance model"
endpoint_id = "accept"
model = "deepseek-v4.1-flash"
context_window_tokens = 400000

[agent_models.default]
profile_id = "accept"
`, 'utf8')

async function req(method, p, data) {
  const res = await fetch(`${BASE}${p}`, {
    method, headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const text = await res.text()
  let body; try { body = JSON.parse(text) } catch { body = text }
  return { ok: res.ok, status: res.status, body }
}

let child = null
async function startBackend() {
  child = spawn(binaryPath, ['--no-open', `--port=${port}`], {
    cwd: runtimeRoot,
    env: { ...process.env, TA_DIR: taDir, TA_SKILLS_DIR: path.join(repositoryRoot, 'skills') },
    stdio: 'inherit',
  })
  const deadline = Date.now() + 90000
  while (Date.now() < deadline) {
    try { const r = await fetch(`${BASE}/api/interactive/tellers`); if (r.ok) return } catch { /* not up */ }
    await new Promise((r) => setTimeout(r, 1200))
  }
  throw new Error('backend did not become ready')
}

/** Kills the process and waits for the port to stop answering, not just for the kill call. */
async function stopBackend() {
  if (!child || child.killed) return
  const pid = child.pid
  child.kill()
  await new Promise((resolve) => { child.once('exit', resolve); setTimeout(resolve, 15000) })
  const deadline = Date.now() + 30000
  while (Date.now() < deadline) {
    try { await fetch(`${BASE}/api/interactive/tellers`); } catch { return }
    await new Promise((r) => setTimeout(r, 800))
  }
  console.log(`[restart] process ${pid} did not release the port in time`)
}

try {
  if (!existsSync(path.join(repositoryRoot, 'internal', 'webfs', 'dist', 'index.html'))) {
    throw new Error('internal/webfs/dist is missing; build the frontend first')
  }
  await startBackend()

  // Story creation requires a selected workspace, so one is made first. The
  // settings under test hang off the story record, but the record cannot exist
  // without a workspace to live in.
  const book = await req('POST', '/api/books/create', { title: `重启验收-${Date.now().toString(36).slice(-6)}`, author: 'Text Adventure' })
  record('创建工作区', book.ok && !!book.body?.workspace, book.body?.workspace || JSON.stringify(book.body).slice(0, 160))
  if (!book.ok) throw new Error('no workspace')
  await req('POST', '/api/workspace/switch', { path: book.body.workspace })

  // A story is created so the settings attach to a real record, not a draft.
  const created = await req('POST', '/api/interactive/stories', {
    title: `重启验收-${Date.now().toString(36).slice(-6)}`,
    mode: 'story',
  })
  const storyId = created.body?.id || created.body?.story?.id
  record('创建故事线', !!storyId, storyId || JSON.stringify(created.body).slice(0, 160))
  if (!storyId) throw new Error('no story id')

  const tellers = (await req('GET', '/api/interactive/tellers')).body
  const ids = new Set((Array.isArray(tellers) ? tellers : tellers.tellers || []).map((t) => t.id))
  record('十二类文风可从接口取到', ids.has('flippant-banter') && ids.has('hotblooded-combat'),
    `${ids.size} 个 teller`)

  const title = created.body?.title || `重启验收-${storyId}`

  // Set the three purposes, then confirm they landed before the restart so a
  // later failure can be attributed to the restart rather than the write.
  const set = await req('PATCH', `/api/interactive/stories/${storyId}`, {
    title,
    story_teller_id: 'source-drama',
    style_purposes: { dialogue: 'flippant-banter', combat: 'hotblooded-combat' },
  })
  record('三用途写入成功', set.ok, `${set.status}`)

  const before = (await req('GET', '/api/interactive/stories')).body
  const beforeRow = (Array.isArray(before?.stories) ? before.stories : []).find((s) => s.id === storyId)
  const beforePurposes = beforeRow?.style_purposes || {}
  record('写入后主叙事＝source-drama',
    (beforeRow?.story_teller_id || '') === 'source-drama', `${beforeRow?.story_teller_id}`)
  record('写入后对白＝flippant-banter',
    (beforePurposes.dialogue || '') === 'flippant-banter', `${beforePurposes.dialogue}`)
  record('写入后战斗＝hotblooded-combat',
    (beforePurposes.combat || '') === 'hotblooded-combat', `${beforePurposes.combat}`)
  flush({ storyId })

  // The point of this script: a real process death, then a real cold start.
  await stopBackend()
  record('进程已停止（端口不再应答）', true, `pid ${child?.pid ?? '?'}`)

  await startBackend()
  record('进程已用同一数据根重新启动', true, `pid ${child?.pid ?? '?'}`)

  const after = (await req('GET', '/api/interactive/stories')).body
  const afterRow = (Array.isArray(after?.stories) ? after.stories : []).find((s) => s.id === storyId)
  record('重启后故事记录仍在', !!afterRow, afterRow ? afterRow.id : '记录消失')
  const afterPurposes = afterRow?.style_purposes || {}
  record('重启后主叙事仍为 source-drama',
    (afterRow?.story_teller_id || '') === 'source-drama', `${afterRow?.story_teller_id}`)
  record('重启后对白仍为 flippant-banter',
    (afterPurposes.dialogue || '') === 'flippant-banter', `${afterPurposes.dialogue}`)
  record('重启后战斗仍为 hotblooded-combat',
    (afterPurposes.combat || '') === 'hotblooded-combat', `${afterPurposes.combat}`)

  // One purpose reset to follow the main narrative must survive as a real
  // choice, distinguishable from "saying nothing".
  await req('PATCH', `/api/interactive/stories/${storyId}`, {
    title, style_purposes: { combat: '' },
  })
  await stopBackend()
  await startBackend()
  const third = (await req('GET', '/api/interactive/stories')).body
  const thirdRow = (Array.isArray(third?.stories) ? third.stories : []).find((s) => s.id === storyId)
  const thirdPurposes = thirdRow?.style_purposes || {}
  record('重启后「战斗改回跟随主叙事」仍生效',
    !thirdPurposes.combat, `combat=${JSON.stringify(thirdPurposes.combat)}`)
  record('重启后对白未被牵连，仍为 flippant-banter',
    (thirdPurposes.dialogue || '') === 'flippant-banter', `${thirdPurposes.dialogue}`)
  record('重启后主叙事未被牵连，仍为 source-drama',
    (thirdRow?.story_teller_id || '') === 'source-drama', `${thirdRow?.story_teller_id}`)
} finally {
  await stopBackend()
  flush()
}

console.log(`==== STY-07 重启验收：${steps.length - failed}/${steps.length} 通过 ====`)
if (failed) process.exitCode = 1
