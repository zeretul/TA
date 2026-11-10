/**
 * T33 更新候选：隔离候选实例上的上传 → 暂存 → 忙时拒绝 → 失败恢复。
 *
 * 《Claude_剩余任务统一收尾》第 8 节要求「T33 在隔离候选走更新/忙时拒绝/失败恢复，
 * 不操作正式安装」。这里全部跑在一个独立的候选实例上：它由 `output/TA`（正式候选，
 * embedweb 构建）启动，数据目录是临时副本，端口空闲，不碰正式安装、不重启任何服务。
 *
 * 覆盖四件事：
 *   1. 上传一份真实的、结构合法的更新包，被正确暂存（staged + apply_ready）。
 *   2. 上传过程中并发请求被拒绝（409 api.update.busy），不会并发写。
 *   3. 校验和错误、版本不新、平台不符都被拒绝，且不破坏已暂存状态。
 *   4. 一次失败的上传之后，再次上传合法包仍能成功（失败恢复）。
 *
 * 故意不做的：`/api/update/apply` 会启动 updater 并让当前进程退出，那等于重启实例，
 * 不属于「隔离验证」的范围，本轮不调用。
 *
 * 用法：
 *   TA_UPDATE_BASE=http://127.0.0.1:19731 \
 *   TA_UPDATE_ARCHIVE=../.ta-dev/update-candidate/TA-v0.6.7-windows-x64.zip \
 *   TA_UPDATE_BIG=../.ta-dev/update-candidate/big-0.6.7.zip \
 *   node scripts/update-candidate-acceptance.mjs
 */
import { writeFileSync, mkdirSync, readFileSync } from 'node:fs'
import { createHash } from 'node:crypto'

const BASE = process.env.TA_UPDATE_BASE || 'http://127.0.0.1:19731'
const ARCHIVE = process.env.TA_UPDATE_ARCHIVE || ''
const BIG_ARCHIVE = process.env.TA_UPDATE_BIG || ARCHIVE
const OUT = process.env.TA_UPDATE_RESULT || '../.ta-dev/update-candidate-result.json'
mkdirSync('../.ta-dev/update-candidate', { recursive: true })

const steps = []
let failed = 0
let finished = false

function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' — ' + detail : ''}`)
}
function flush() {
  writeFileSync(OUT, JSON.stringify({ passed: steps.length - failed, total: steps.length, steps }, null, 2))
}

async function getStatus() {
  const res = await fetch(`${BASE}/api/update/status`)
  return { status: res.status, body: await res.json().catch(() => null) }
}

/** 上传一份包；checksums 内容由调用方给定，用来区分成功路径和拒绝路径。 */
async function upload(archivePath, checksumsText, archiveName) {
  const form = new FormData()
  form.append('file', new Blob([readFileSync(archivePath)]), archiveName)
  form.append('checksums', new Blob([checksumsText]), 'checksums.txt')
  const res = await fetch(`${BASE}/api/update/upload`, { method: 'POST', body: form })
  const text = await res.text()
  let body
  try { body = JSON.parse(text) } catch { body = text }
  return { status: res.status, body }
}

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex')
}

async function main() {
  const before = await getStatus()
  record('读取更新状态', before.status === 200 && !!before.body?.current_version,
    `status=${before.status} current=${before.body?.current_version} staged=${before.body?.staged}`)
  flush()
  if (before.status !== 200 || !before.body?.current_version) return

  if (!ARCHIVE) { record('更新包就绪', false, '缺少 TA_UPDATE_ARCHIVE'); return }

  const current = before.body.current_version
  const [major, minor, patch] = current.split('.').map(Number)
  const nextVersion = `${major}.${minor}.${patch + 1}`
  const archiveName = `TA-v${nextVersion}-windows-x64.zip`
  const digest = sha256(ARCHIVE)
  const goodChecksums = `${digest}  ${archiveName}\n`

  // 1. 校验和错误：必须拒绝，且不动已暂存状态
  const badSum = await upload(ARCHIVE, `${'0'.repeat(64)}  ${archiveName}\n`, archiveName)
  record('错误校验和被拒绝', badSum.status === 400 && badSum.body?.code === 'api.update.checksumRequired',
    `status=${badSum.status} code=${badSum.body?.code}`)
  const afterBadSum = await getStatus()
  record('错误校验和未改变暂存状态', !!afterBadSum.body?.staged === !!before.body?.staged,
    `staged=${afterBadSum.body?.staged}`)
  flush()

  // 2. 版本不新：必须拒绝
  const sameName = `TA-v${current}-windows-x64.zip`
  const notNewer = await upload(ARCHIVE, `${digest}  ${sameName}\n`, sameName)
  record('同版本包被拒绝', notNewer.status === 400 && notNewer.body?.code === 'api.update.newerRequired',
    `status=${notNewer.status} code=${notNewer.body?.code}`)
  flush()

  // 3. 平台不符：必须拒绝
  const wrongPlatform = await upload(ARCHIVE, `${digest}  TA-v${nextVersion}-linux-x64.zip\n`,
    `TA-v${nextVersion}-linux-x64.zip`)
  record('错误平台包被拒绝', wrongPlatform.status === 400 && wrongPlatform.body?.code === 'api.update.wrongPlatform',
    `status=${wrongPlatform.status} code=${wrongPlatform.body?.code}`)
  flush()

  // 4. 失败恢复：三次拒绝之后，合法包仍能成功暂存
  const recovered = await upload(ARCHIVE, goodChecksums, archiveName)
  record('失败后合法包仍可上传', recovered.status === 200 && recovered.body?.staged === true,
    `status=${recovered.status} staged=${recovered.body?.staged} installed=${recovered.body?.installed_version}`)
  flush()

  const staged = await getStatus()
  record('暂存后状态可读', staged.status === 200 && staged.body?.staged === true && staged.body?.apply_ready === true,
    `staged=${staged.body?.staged} apply_ready=${staged.body?.apply_ready} version=${staged.body?.installed_version}`)
  record('暂存版本是上传的版本', staged.body?.installed_version === nextVersion,
    `${staged.body?.installed_version} vs ${nextVersion}`)
  flush()

  // 5. 忙时拒绝：并发上传同一个包，只允许一个进入，其余 409。
  //    用解压后体积大的包把持锁窗口拉长，否则几个请求会一前一后跑完。
  const bigDigest = sha256(BIG_ARCHIVE)
  const bigChecksums = `${bigDigest}  ${archiveName}\n`
  const results = await Promise.all([
    upload(BIG_ARCHIVE, bigChecksums, archiveName),
    upload(BIG_ARCHIVE, bigChecksums, archiveName),
    upload(BIG_ARCHIVE, bigChecksums, archiveName),
  ])
  const accepted = results.filter((r) => r.status === 200).length
  const busy = results.filter((r) => r.status === 409 && r.body?.code === 'api.update.busy').length
  record('并发上传只有一个被受理', accepted === 1,
    `状态 ${results.map((r) => r.status).join(' / ')}`)
  record('并发上传其余被忙时拒绝', busy === results.length - 1,
    results.map((r) => `${r.status}${r.body?.code ? ':' + r.body.code : ''}`).join(' | '))
  flush()

  const afterRace = await getStatus()
  record('并发后状态仍一致', afterRace.status === 200 && afterRace.body?.staged === true,
    `staged=${afterRace.body?.staged} version=${afterRace.body?.installed_version}`)
  flush()

  // 6. 所有拒绝路径都没触碰当前安装
  const finalStatus = await getStatus()
  record('当前运行版本未被替换', finalStatus.body?.current_version === current,
    `${finalStatus.body?.current_version} vs ${current}`)
  flush()
}

function finish() {
  if (finished) return
  finished = true
  flush()
  console.log(`==== T33 更新候选：${steps.length - failed}/${steps.length} 通过 ====`)
  if (failed) process.exitCode = 1
}

main().catch((e) => { record('脚本异常', false, e.message); console.error(e) }).finally(finish)
