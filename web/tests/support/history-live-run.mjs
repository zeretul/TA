// Explicit live-model acceptance. Run only against a disposable native instance.
// TA_HISTORY_TEST_URL, TA_HISTORY_TEST_OUTPUT and phase=play|reload are required.
import assert from 'node:assert/strict'
import { mkdir, writeFile, readFile } from 'node:fs/promises'
import { randomUUID } from 'node:crypto'
import path from 'node:path'

const base = process.env.TA_HISTORY_TEST_URL
const output = process.env.TA_HISTORY_TEST_OUTPUT
const phase = process.argv[2] || 'play'
if (!base || !output || process.env.TA_HISTORY_ALLOW_MODEL_TESTS !== '1') {
  throw new Error('Set a disposable TA_HISTORY_TEST_URL, TA_HISTORY_TEST_OUTPUT, and TA_HISTORY_ALLOW_MODEL_TESTS=1')
}
if (!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base)) throw new Error('Live acceptance requires a loopback test instance')
await mkdir(output, { recursive: true })
const evidence = []
async function save() { await writeFile(path.join(output, `live-${phase}.json`), JSON.stringify(evidence, null, 2)) }
async function api(route, body, expectedStatus = 200) {
  const start = Date.now()
  const response = await fetch(base + route, body === undefined ? undefined : {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  })
  const text = await response.text()
  let result
  try { result = JSON.parse(text) } catch { result = { non_json: text.slice(0, 400) } }
  evidence.push({ route, input: body, status: response.status, seconds: (Date.now() - start) / 1000, result })
  await save()
  if (response.status !== expectedStatus) throw new Error(`${route}: expected ${expectedStatus}, got ${response.status}; see private test evidence`)
  return result
}
const id = (tag) => `live-${tag}-${randomUUID()}`
let projectId, state, campaignPath
function update(next) { state = next; campaignPath = `/api/projects/${projectId}/history/campaigns/${state.id}` }
function command(extra = {}) { return { command_id: id('command'), expected_revision: state.revision, locale: 'zh-CN', ...extra } }
async function act(action, extra) {
  const result = await api(`${campaignPath}/${action}`, command(extra))
  update(result.state)
  return result
}
async function chat(ministerIds, message, locale = 'zh-CN') {
  const result = await act('chat', { minister_ids: ministerIds, message, locale })
  const latest = result.state.chats.at(-1)
  console.log(`Chat ${ministerIds.join(',')}: ${latest.replies.map((r) => r.text).join(' | ').slice(0, 200)}`)
  return latest
}
function region(id) { return state.regions.find((r) => r.id === id) }

if (phase === 'play') {
  const currentWorkspace = await api('/api/workspace/current')
  const boot = await api('/api/history/bootstrap', { locale: 'zh-CN' })
  projectId = boot.project_id
  update(await api(`/api/projects/${projectId}/history/campaigns`, { name: '历史推演隔离验收', command_id: id('create') }))
  const initial = structuredClone(state)
  const afterWorkspace = await api('/api/workspace/current')
  assert.equal(afterWorkspace.project_id, currentWorkspace.project_id, 'History must not select the adventure project')
  assert.equal(afterWorkspace.workspace, currentWorkspace.workspace)
  console.log(`Created isolated campaign ${state.id}, revision ${state.revision}`)

  const secret = await chat(['bi-ziyan'], '毕卿，朕先与你私下约定：赈陕西的账目，下一年仍由你回来复核。此事的密谈记号是“青瓷灯”。请记住复核承诺与记号，暂不向其他大臣转述。先告诉朕，你当前掌握的国库可用银是多少？')
  assert(secret.replies[0].text.trim().length > 0)

  const prepared = await api(`${campaignPath}/prepare-edict`, command({ text: '命毕自严拨银四万两、粮八千石赈济陕西，用一季完成；待该项赈济完成后，再由范景文拨银十二万两修陕西水利，分三季施工；孙承宗拨银六万两整训辽东，用两季，自下一季度开始。各项经费都从国库支出，不准挪用内帑。' }))
  const relief = prepared.plan.instructions.find((o) => o.kind === 'relief')
  const construction = prepared.plan.instructions.find((o) => o.kind === 'construction')
  const training = prepared.plan.instructions.find((o) => o.kind === 'training')
  assert(relief && construction && training, 'The whole decree must retain all three requested actions')
  assert.equal(relief.budget, 40000)
  assert.equal(relief.grain, 8000)
  assert.equal(construction.budget, 120000)
  assert.equal(training.budget, 60000)
  assert(construction.depends_on.includes(relief.id), 'Construction must wait for the requested relief')
  assert(training.not_before_turn >= state.turn + 1, 'Next-quarter training must not begin now')
  assert.equal(prepared.outcomes.filter((o) => o.status === 'ready').length, 3)
  const request = command({ plan: prepared.plan })
  const committed = await api(`${campaignPath}/edict`, request)
  update(committed.state)
  const duplicate = await api(`${campaignPath}/edict`, request)
  assert.equal(duplicate.duplicate, true)
  assert.equal(duplicate.state.revision, state.revision)
  assert.equal(duplicate.state.projects.length, 3)
  const checkpointReceipt = await act('checkpoints', { label: '推进前核验点' })
  const checkpoint = checkpointReceipt.state.checkpoints.at(-1)
  const beforeAdvance = structuredClone(state)
  console.log('Natural-language three-order plan, dependencies, checkpoint and duplicate submission passed')

  for (let quarter = 0; quarter < 4; quarter++) {
    const before = structuredClone(state)
    const advanceRequest = command()
    const result = await api(`${campaignPath}/advance`, advanceRequest)
    update(result.state)
    assert.equal(state.turn, before.turn + 1)
    assert.equal(state.privy_purse, initial.privy_purse, 'Treasury expenditure must not silently draw from privy funds')
    const report = state.reports.at(-1)
    assert.equal(report.opening_treasury + report.income - report.upkeep - report.reserved, report.closing_treasury)
    assert.equal(state.treasury, report.closing_treasury)
    assert(state.treasury >= 0 && state.grain >= 0)
    for (const p of state.projects) assert(p.spent >= 0 && p.spent <= p.budget && p.grain_spent >= 0 && p.grain_spent <= p.grain)
    if (quarter === 0) {
      assert.equal(state.projects.find((p) => p.kind === 'construction').progress, 0)
      assert.equal(state.projects.find((p) => p.kind === 'training').progress, 0)
      const repeated = await api(`${campaignPath}/advance`, advanceRequest)
      assert.equal(repeated.duplicate, true)
      assert.equal(repeated.state.turn, state.turn)
      assert.equal(repeated.state.treasury, state.treasury)
    }
    console.log(`Quarter ${quarter + 1}: turn=${state.turn}, treasury=${state.treasury}, relief=${region('shaanxi').disaster}, projects=${state.projects.map((p) => `${p.kind}:${p.status}:${p.progress}/${p.required}`).join(',')}`)
  }
  assert(state.projects.some((p) => p.kind === 'relief' && p.status === 'completed'), 'Relief must have an actual outcome')
  assert(state.projects.some((p) => p.kind === 'construction' && p.progress > 0), 'A dependent multi-quarter project must start after its predecessor')
  assert.equal(state.projects.find((p) => p.kind === 'relief').grain_spent, 8000, 'Relief must consume its actual grain allocation')
  assert.notDeepEqual(region('shaanxi'), initial.regions.find((r) => r.id === 'shaanxi'), 'The crisis must evolve in saved facts')
  await chat(['fan-jingwen'], '范卿，只谈陕西水利目前的工期、进度和经费，不必涉及其他事。')
  const notWitnessed = await chat(['sun-chengzong'], '孙卿，朕先前与毕自严单独约定的密谈记号具体是什么？你若未在场也未收到转告，就直言不知，不要猜。')
  assert(!notWitnessed.replies.some((r) => r.text.includes('青瓷灯')), 'Another minister must not learn a private code from hidden context')
  await chat(['bi-ziyan', 'fan-jingwen'], '召二卿商议：各自只按已掌握的奏报，说说陕西赈务与水利接下来应注意什么。不要转述私下谈话。')
  await writeFile(path.join(output, 'resume.json'), JSON.stringify({ projectId, state, initial, checkpoint, beforeAdvance }, null, 2))
  console.log('Play phase passed; restart the native server before running reload phase')
} else if (phase === 'reload') {
  const saved = JSON.parse(await readFile(path.join(output, 'resume.json'), 'utf8'))
  projectId = saved.projectId
  update(await api(`/api/projects/${projectId}/history/campaigns/${saved.state.id}`))
  const withAddedViewFields = (original) => {
    const value = structuredClone(original)
    for (const item of [...value.chats, ...value.reports, ...value.inspections]) item.discarded ??= false
    for (const report of value.reports) report.applied_adjustments ??= null
    return value
  }
  assert.deepEqual(withAddedViewFields(state), withAddedViewFields(saved.state), 'Cold-open canonical facts must be identical; additive view fields default only')
  console.log('Cold-open exact state passed')
  const treasury = state.treasury
  const recall = await chat(['bi-ziyan'], '毕卿，时隔多季又谈过其他政事，朕最初单独托你来年复核的是哪件事？那次密谈的记号是什么？另外请按当前这一季的新账报出国库可用银，不要沿用旧账数字。')
  assert(recall.replies[0].text.includes('青瓷灯'), 'Witness must recall the dated private promise after cold reopen')
  evidence.push({ check: 'current_treasury_in_recall_requires_human_read', expected_treasury: treasury, reply: recall.replies[0].text })
  await save()
  const english = await chat(['fan-jingwen'], 'Please describe the present condition of the Shaanxi waterworks in two short English sentences. Do not invent completed work.', 'en')
  assert(/[a-zA-Z]{3}/.test(english.replies[0].text))
  const inspect = await act('inspect', { target: 'treasury' })
  assert(inspect.state.inspections.length > 0)
  const restored = await act('restore', { checkpoint_id: saved.checkpoint.id })
  assert.equal(restored.state.turn, saved.beforeAdvance.turn)
  assert.equal(restored.state.treasury, saved.beforeAdvance.treasury)
  assert.equal(restored.state.projects.length, saved.beforeAdvance.projects.length)
  assert(restored.state.revision > saved.state.revision, 'Restoration is a new journal record, never a revision rewind')
  console.log('Recall, English response, inspection and checkpoint restoration passed')
  const finalQuarter = await act('advance')
  const report = finalQuarter.state.reports.at(-1)
  assert.equal(report.closing_treasury, state.treasury)
  assert(report.narrative.replace(/[,\s]/g, '').includes(String(report.closing_treasury)), 'Final report must state its authoritative closing balance accurately using digits')
  console.log(`Final-rule quarter report balance=${report.closing_treasury}; persisted causes=${report.applied_adjustments?.length || 0}: ${report.narrative.slice(0, 220)}`)
  await writeFile(path.join(output, 'final-state.json'), JSON.stringify(state, null, 2))
} else if (phase === 'verify-report') {
  const saved = JSON.parse(await readFile(path.join(output, 'resume.json'), 'utf8'))
  projectId = saved.projectId
  update(await api(`/api/projects/${projectId}/history/campaigns/${saved.state.id}`))
  await act('restore', { checkpoint_id: saved.checkpoint.id })
  await act('advance')
  const report = state.reports.at(-1)
  const refugees = region('shaanxi').refugees
  assert(report.narrative.replace(/[,\s]/g, '').includes(String(report.closing_treasury)))
  assert(!new RegExp(`${refugees}\\s*人(?:得食|获救|受助|得到赈济)`).test(report.narrative), 'Remaining refugees must not be relabeled as helped people')
  evidence.push({ check: 'refugee_semantics_readback', remaining_refugees: refugees, treasury: report.closing_treasury, narrative: report.narrative })
  await save()
  await writeFile(path.join(output, 'final-state.json'), JSON.stringify(state, null, 2))
  console.log(`Final-report verified: available silver=${report.closing_treasury}, remaining refugees=${refugees}; ${report.narrative}`)
} else throw new Error('Unknown phase')
