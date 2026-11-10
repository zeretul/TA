/**
 * Character interaction acceptance (real model, real browser).
 *
 * Covers the four deliverables end to end against a live backend:
 *   1. main-story conversation stop points
 *   2. the character window (cast, conversation, reply, cancel, retry)
 *   3. same-scene continuity and acting on a negotiated arrangement
 *   4. proactive incoming messages
 *
 * Writes results after every step so output survives pipe buffering.
 * Run with the backend already serving on TA_ACCEPT_BASE.
 */
import { chromium } from '@playwright/test'
import { randomUUID } from 'node:crypto'
import { writeFileSync, mkdirSync } from 'node:fs'

const BASE = process.env.TA_ACCEPT_BASE || 'http://127.0.0.1:19730'
const OUT = process.env.TA_ACCEPT_RESULT || './character-acceptance-result.json'
const ART = './test-results/character'
mkdirSync(ART, { recursive: true })

const steps = []
let failed = 0
function record(name, ok, detail = '') {
  steps.push({ name, ok: !!ok, detail: String(detail) })
  if (!ok) failed += 1
}

// NAR-18 asks for the real cost of the preparation this batch added, so the
// driver times every model-backed request rather than reporting only a total.
// A "model call" here is one request that the backend answers by running the
// model; reads of the character window and snapshot are served from the journal
// and are timed separately so they cannot inflate the model figure.
const MODEL_PATHS = ['/api/interactive/chat', '/characters/messages']
const metrics = { modelCalls: 0, modelMs: 0, firstWaitMs: null, reads: 0, readMs: 0, longest: [] }
function isModelCall(path) {
  return MODEL_PATHS.some((p) => path.includes(p))
}
function flush() {
  writeFileSync(OUT, JSON.stringify({
    passed: steps.length - failed, total: steps.length,
    modelCalls: metrics.modelCalls,
    modelSeconds: Math.round(metrics.modelMs / 1000),
    firstModelWaitSeconds: metrics.firstWaitMs === null ? null : Math.round(metrics.firstWaitMs / 1000),
    journalReads: metrics.reads,
    steps,
  }, null, 2))
}

async function main() {
  const unique = randomUUID().slice(0, 8)
  const started = Date.now()

  // --- Workspace + pack, so the story runs inside a real world binding ---
  const book = await post('/api/books/create', { title: `人物验收-${unique}`, author: 'Text Adventure' })
  record('创建工作区', book.ok && !!book.body.workspace, book.body.workspace || JSON.stringify(book.body).slice(0, 160)); flush()
  if (!book.ok) return finish()
  const projectId = book.body.project_id
  await post('/api/workspace/switch', { path: book.body.workspace })

  const applied = await post(`/api/projects/${encodeURIComponent(projectId)}/story-packs/fanren-jie/apply`, {})
  record('载入凡人人界剧本包', applied.ok && applied.body.world_added > 0, `新增 ${applied.body?.world_added} 条`); flush()
  const request = applied.body?.request
  if (!request) return finish()

  const story = await post('/api/interactive/stories', { ...request, title: `人物验收故事-${unique}` })
  record('按剧本包创建故事线', story.ok && !!story.body.id, story.body?.id || JSON.stringify(story.body).slice(0, 160)); flush()
  if (!story.ok) return finish()
  const storyId = story.body.id

  // --- Deliverable 1: the opening, and a turn that reaches a stop point ---
  await post('/api/interactive/chat', {
    command_id: `ch-opening-${storyId}`, mode: 'story', story_id: storyId, branch: 'main', start_opening: true,
  })
  const snap1 = await pollSnapshot(storyId, 1, 200000)
  const opening = snap1.turns?.[0]?.narrative || ''
  record('开局生成（真实模型）', opening.length > 50, `${opening.length} 字`); flush()

  // Ask a question that puts named people in front of the protagonist, which is
  // what makes a conversation stop point possible at all. Two people are named
  // on purpose: the voice comparison below needs two registered characters in
  // the same scene, and a turn that introduces only one leaves nothing to
  // compare, so the distinctness checks would report not_run rather than pass.
  await post('/api/interactive/chat', {
    command_id: `ch-turn2-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
    message: '我走进青石镇的坊市，先向收散修灵草的铺子掌柜打听最近的行情和规矩，'
      + '又转身问旁边摆摊卖符的年轻女修：这坊市里做买卖还有什么门道。',
  })
  const snap2 = await pollSnapshot(storyId, 2, 240000)
  const turn2 = snap2.turns?.[1]
  const text2 = turn2?.narrative || ''
  record('自由输入推进一回合', text2.length > 50, `第2轮 ${text2.length} 字`); flush()

  // A stop point is a turn that hands the initiative back to the player rather
  // than running on to a word-count floor. The earlier version of this check
  // computed `stopPoint` and then asserted only `text2.length > 50`, so a turn
  // that never paused for the player passed on length alone. The predicate is
  // now the pass condition, and it is checked against the end of the text rather
  // than the whole of it, because what matters is where the turn stops.
  //
  // The test is on the final sentence, not on an anchored suffix of the raw
  // text: Chinese dialogue ends with 。" or 。」, and an anchor that cannot span
  // the closing punctuation reports a hand-back as a miss. What is being asked
  // is whether the last thing on the page is an opening for the player — an
  // addressed line, a question, someone waiting — rather than a completed action
  // of the protagonist's own.
  const trimmed2 = text2.trim()
  const stripped2 = trimmed2.replace(/[”"』」）)】\s]+$/u, '')
  const lastSentence = stripped2.split(/[。！？\n]/u).filter((s) => s.trim()).pop() || ''
  const endsOnQuestion = /[？?]\s*[”」』"]?\s*$/u.test(trimmed2)
  // Who holds the floor at the end of the page. The previous version split the
  // text on 。 and looked for keywords in the final fragment, which reported a
  // hand-back as a miss whenever the closing line was dialogue followed by an
  // attribution: the acceptance turn ended on `“想清楚再应。”她说，“我这摊子上的
  // 符，一张二十文。”`, someone else's line with the protagonist's answer
  // deliberately withheld, and the fragment carried no 你 and no keyword.
  //
  // The question is structural instead. The last quoted span and its
  // attribution are read directly, and the turn counts as a hand-back when
  // someone other than the protagonist holds the floor — unless the protagonist
  // then carries the scene on in the narration after it, which is the opposite
  // of stopping and is what the overran check below exists to catch.
  const lastOpenIdx = Math.max(text2.lastIndexOf('“'), text2.lastIndexOf('「'), text2.lastIndexOf('『'))
  const lastCloseIdx = Math.max(text2.lastIndexOf('”'), text2.lastIndexOf('」'), text2.lastIndexOf('』'))
  const endsInsideQuote = lastOpenIdx > lastCloseIdx
  const finalQuote = lastOpenIdx < 0
    ? ''
    : text2.slice(lastOpenIdx + 1, endsInsideQuote ? text2.length : lastCloseIdx).trim()
  // The sentence the closing line is attributed to. A fixed-width window is not
  // enough: `沈青把木牌接过来，说：“成”` puts the name eleven characters before
  // the verb, and a 20-character slice can also swallow the previous speaker.
  // Reading back to the last sentence break keeps the attribution to the clause
  // that actually introduces the quote.
  const beforeQuote = lastOpenIdx < 0 ? '' : text2.slice(0, lastOpenIdx)
  const attributionSentence = beforeQuote.split(/[。！？\n]/u).filter((s) => s.trim()).pop() || beforeQuote
  const trailingNarration = endsInsideQuote || lastCloseIdx < 0 ? '' : text2.slice(lastCloseIdx + 1).trim()
  // The protagonist is named, not pronoun-referenced, in the story's own prose,
  // and a pronoun in an attribution belongs to whoever else is on the page.
  const protagonistSpeaks = /(沈青|主角)/u.test(attributionSentence)
    && /(说|道|问|答|应|开口|笑|哼|喊|叫|嘟囔|低声|反问)/u.test(attributionSentence)
  const protagonistContinues = /(沈青|主角)/u.test(trailingNarration)
    && /(说|道|问|答|应|走|转身|点头|摇头|接过|伸手|迈|离开|开口|收进|揣|转身)/u.test(trailingNarration)
  const otherHoldsFloor = finalQuote.length > 0 && !protagonistSpeaks && !protagonistContinues
  // A turn may also stop mid-utterance inside a quotation that never closes, as
  // in `他抬起头：“去不去`. That is the strongest hand-back, but it carries no
  // `？` and no closing mark for the checks above to key on — an A-not-A question
  // (`去不去`, `成不成`) is interrogative by construction. The shape is read
  // directly rather than the assertion being relaxed: a completed narration
  // ending on an unclosed quote is not a stop point, only an unfinished
  // utterance aimed at the player is.
  const unclosedTail = endsInsideQuote ? text2.slice(lastOpenIdx + 1).trim() : ''
  const aNotA = /(.)不\1/u.test(unclosedTail)
  const trailingParticle = /[吗么呢]$/u.test(unclosedTail)
  const spokenToPlayer = endsInsideQuote
    && (aNotA || trailingParticle || /[你您]/u.test(unclosedTail))
  // Narration that explicitly leaves the decision with the player: someone
  // waiting on the protagonist, or the protagonist having answered nothing yet.
  // The list is deliberately concrete — a bare 看/等 keyword matched almost any
  // sentence and would have passed a turn that never paused at all.
  const waitsOnPlayer = /(等他|等她|等你|望着他|望着你|看着他|看着你|盯着他|盯着你|等他开口|等他答话|等他回答|等他决定|等他表态|等他拿主意|等他先|没有答话|没答话|没有回答|没回答|没有应声|没应声|没有开口|没开口|没有接话|没接话|沉默着等他)/u.test(text2.slice(-120))
  const stopPoint = otherHoldsFloor || endsOnQuestion || spokenToPlayer || waitsOnPlayer
  // The other half of "stop at the conversation": the turn must not narrate the
  // player doing the thing they have not yet chosen to do. The player only asked
  // about the market, so the turn may not already have them inside it.
  const overran = /(你(就)?(走|进|迈|踏|推门|穿过|离开)了?(城门|坊市|铺子|里间))/.test(text2)
  record('第2轮形成可应答停点', text2.length > 50 && stopPoint,
    `${text2.length} 字，结尾${otherHoldsFloor ? `停在他人台词（${attributionSentence.trim().slice(-8)}：“${finalQuote.slice(0, 24)}”）` : endsOnQuestion ? '为问句' : spokenToPlayer ? `为未收尾的当面发问（${unclosedTail.slice(-20)}）` : waitsOnPlayer ? '为等待主角回应' : '无停点（跑到了收束）'}：${lastSentence.slice(-60)}`); flush()
  record('第2轮未越过玩家尚未选择的行动', !overran,
    overran ? '正文替玩家完成了未选择的行动' : '正文停在玩家可介入处'); flush()

  // --- Deliverable 1 + 3: three consecutive negotiation rounds that react ---
  const negotiationTurns = []
  const negotiationInputs = [
    '我向掌柜提出：想用身上仅剩的两株灵草换一份坊市通行木牌，先赊账，三日内补齐差价。',
    '我说三日期限太紧，改为一旬，并愿意多付一成的利息。',
    '我退一步，接受三日，但要求他先把木牌给我，我留下姓名和住处作保。',
  ]
  for (let i = 0; i < negotiationInputs.length; i += 1) {
    await post('/api/interactive/chat', {
      command_id: `ch-neg${i}-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
      message: negotiationInputs[i],
    })
    const snap = await pollSnapshot(storyId, 3 + i, 240000)
    const turn = snap.turns?.[2 + i]
    const text = turn?.narrative || ''
    negotiationTurns.push(text)
    record(`第${3 + i}轮交涉有实际回应`, text.length > 50, `${text.length} 字`); flush()
  }
  const distinct = new Set(negotiationTurns.map((t) => t.slice(0, 60))).size
  record('三轮交涉不是同一段复用', distinct === negotiationTurns.length, `${distinct}/${negotiationTurns.length} 段开头互不相同`); flush()

  // NAR-11: the choice protocol is kept, but the choices must be genuinely
  // different things to do. Distinct text alone is not enough — three phrasings
  // of one action would pass that. The earlier version compared six-character
  // prefixes, which three rewordings of "accept the offer" would still pass.
  // What is checked now is the action each choice actually proposes: each must
  // fall in a different intent class, and the set must not be one action
  // restated. A choice whose intent cannot be classified is counted as its own
  // class rather than silently passing.
  //
  // The choices ride on the turn's result, not on the turn itself: the committed
  // turn record carries `turn_result`, and `choices` is a field of that object.
  // Reading `turn.choices` yields undefined and would make this assertion pass
  // or fail for the wrong reason.
  const lastTurn = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  const lastRecord = lastTurn?.turns?.[(lastTurn?.turns?.length || 0) - 1]
  const rawChoices = lastRecord?.turn_result?.choices ?? lastRecord?.choices ?? []
  const lastTurnChoices = (Array.isArray(rawChoices) ? rawChoices : [])
    .filter((c) => typeof c === 'string' && c.trim())
    .map((c) => c.trim())
  const choiceHeads = lastTurnChoices.map((c) => c.slice(0, 6))
  const noPrefix = lastTurnChoices.every((a) => lastTurnChoices.every((b) => a === b || !b.startsWith(a)))
  record('选项数量符合剧本包协议', lastTurnChoices.length === 3,
    `${lastTurnChoices.length} 项：${lastTurnChoices.join(' / ').slice(0, 120)}`); flush()
  record('选项不互为前缀', noPrefix && new Set(choiceHeads).size === choiceHeads.length,
    `开头 ${choiceHeads.join(' | ')}`); flush()
  const intents = lastTurnChoices.map(classifyChoiceIntent)
  record('选项意图分属不同类别（NAR-11）',
    lastTurnChoices.length >= 2 && new Set(intents).size === lastTurnChoices.length,
    lastTurnChoices.map((c, i) => `${intents[i]}「${c.slice(0, 14)}」`).join(' / ')); flush()

  // --- Deliverable 2: the character window reads a real cast ---
  const windowBefore = await get(`/api/interactive/stories/${storyId}/characters?branch=main`)
  const cast = windowBefore.body?.characters || []
  record('人物窗口返回本局人物表', windowBefore.ok && cast.length >= 1,
    `${cast.length} 人：${cast.map((c) => `${c.id}/${c.name}`).join('、').slice(0, 120)}`); flush()
  record('人物来自本局演员登记而非固定两人', cast.length >= 1 && cast.every((c) => !!c.id && !!c.name),
    cast.map((c) => c.id).join(',')); flush()
  record('人物表带当前场景与频道', !!windowBefore.body?.scene && cast.every((c) => c.channel === 'scene' || c.channel === 'message'),
    `场景 ${windowBefore.body?.scene?.location || windowBefore.body?.scene?.title || 'n/a'}`); flush()
  if (cast.length === 0) return finish()

  // --- Deliverable 2: a conversation, with a distinguishable voice ---
  const first = cast[0]
  const chat1 = await chatWith(storyId, first.id, '我压低声音问他：这牌子的事，你能做主吗？', `ch-chat1-${unique}`)
  record('人物窗口回复成功', chat1.ok && (chat1.body?.reply || '').length > 0,
    `${(chat1.body?.reply || '').length} 字`); flush()
  const reply1 = chat1.body?.reply || ''
  record('窗口回复是短对话而非长文', reply1.length > 0 && reply1.length <= 800, `${reply1.length} 字`); flush()

  // The retry path: the same command id must return the same words and commit
  // no second exchange. This is the "repeated clicks do not duplicate" case.
  const chat1Retry = await chatWith(storyId, first.id, '我压低声音问他：这牌子的事，你能做主吗？', `ch-chat1-${unique}`)
  record('同一 command_id 重试不产生第二条回复', chat1Retry.ok && chat1Retry.body?.reply === reply1,
    chat1Retry.body?.reply === reply1 ? '回复逐字一致' : '重试回复与首次不同'); flush()

  // A second question to the same person, this time one only that person's own
  // registered profile can answer. "Two replies differ" would pass for two
  // unrelated strangers; what is checked here is that each character answers
  // with what the story registered about them, which is the part a generic
  // voice would get wrong.
  const selfQuestion = '我先自报家门，再问他：你叫什么名字？在这镇上是什么身份？'
  const chatSelf1 = await chatWith(storyId, first.id, selfQuestion, `ch-self1-${unique}`)
  const selfReply1 = chatSelf1.body?.reply || ''
  const grounded1 = characterAnswersAsSelf(selfReply1, first, cast)
  record('人物以自己的登记身份作答', !!grounded1,
    `${first.name}/${first.role || '无角色'}：命中「${grounded1 || '无'}」—— ${selfReply1.slice(0, 40)}…`); flush()

  // The roster is re-read rather than reused. `cast` was taken before the
  // negotiation turns, and those turns legitimately register whoever they put on
  // stage — the story had grown by the time this check runs, and reusing the
  // stale list reported "the story only has one person" as an uncovered check
  // when the story in fact had several. That is a fixture defect wearing the
  // costume of a product gap, so the fixture re-reads the same way the peer and
  // third-character blocks below already do.
  const secondRoster = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
  let second = secondRoster.find((c) => c.id !== first.id)
  if (!second && secondRoster.length === 1) {
    // The story has genuinely kept its stage to one person. The script makes a
    // second one walk in rather than reporting the check unrun: a turn that
    // plainly introduces a new named person is what registers them, and the
    // check is about the window accepting whoever the story registered.
    const beforeSecond = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
    const secondTurns = beforeSecond.turns?.length || 0
    await post('/api/interactive/chat', {
      command_id: `ch-second-in-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
      message: '我从铺子里出来，在街口被一个挎着竹篮的妇人叫住，她自称姓焦，是替镇上几户人家跑腿传话的。'
        + '她问我是不是刚从贺记丹铺出来，又问我认不认得柜上管账的柳先生。',
    })
    await pollSnapshot(storyId, secondTurns + 1, 240000)
    const regrown = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
    second = regrown.find((c) => c.id !== first.id)
  }
  let reply2 = ''
  if (second) {
    const chat2 = await chatWith(storyId, second.id, '我压低声音问他：这牌子的事，你能做主吗？', `ch-chat2-${unique}`)
    reply2 = chat2.body?.reply || ''
    record('第二个人物也能交谈', chat2.ok && reply2.length > 0, `${reply2.length} 字`); flush()
    const chatSelf2 = await chatWith(storyId, second.id, selfQuestion, `ch-self2-${unique}`)
    const selfReply2 = chatSelf2.body?.reply || ''
    const grounded2 = characterAnswersAsSelf(selfReply2, second, cast)
    record('第二个人物也以自己的登记身份作答', !!grounded2,
      `${second.name}/${second.role || '无角色'}：命中「${grounded2 || '无'}」—— ${selfReply2.slice(0, 40)}…`); flush()
    // Distinctness is grounded rather than textual: the two replies must differ,
    // and each must have answered with its own registered name or role, so the
    // two identities are carried by the replies and not by the roster alone. A
    // shared generic voice that answers both questions the same way fails both
    // halves.
    const distinctVoice = reply1 !== reply2 && !!grounded1 && !!grounded2 && grounded1 !== grounded2
    record('两个人物口吻可区分', distinctVoice,
      `回复不同=${reply1 !== reply2}；各自身份依据：${grounded1 || '无'} vs ${grounded2 || '无'}`); flush()
  } else {
    record('第二个人物也能交谈', false, '本局只登记了一人，无法比较口吻'); flush()
  }

  const windowAfter = await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}`)
  const exchanges = windowAfter.body?.exchanges || []
  record('窗口读回已提交的交谈', exchanges.length >= 2, `${exchanges.length} 条（含重试后未重复）`); flush()
  // The window writes on the private channel; the scene channel carries the
  // main story's own lines for the same person. Counting every player line would
  // count the story's speech as if the window had duplicated it, so the retry
  // check is scoped to the channel the window actually writes.
  const windowPlayerLines = exchanges.filter((e) => e.speaker === 'player' && e.channel === 'private').length
  const windowReplyLines = exchanges.filter((e) => e.speaker === 'character' && e.channel === 'private').length
  // The window legitimately holds several player lines by now — the first
  // question, the retry of it, and the later self-introduction question — so
  // counting them all would fail on a correct run. What the retry must not do is
  // commit a second copy of the retried message, so the count is scoped to that
  // one message's text.
  const retriedText = '我压低声音问他：这牌子的事，你能做主吗？'
  const retryCopies = exchanges.filter(
    (e) => e.speaker === 'player' && e.channel === 'private' && e.text === retriedText,
  ).length
  // A retry that committed a second reply as well as a second question would
  // still leave one copy of the question, so the reply side is counted too: the
  // conversation must hold exactly one answer per distinct thing the player said.
  const distinctPlayerTexts = new Set(
    exchanges.filter((e) => e.speaker === 'player' && e.channel === 'private').map((e) => e.text),
  ).size
  record('重试未在记录里留下第二条玩家发言', retryCopies === 1 && windowReplyLines === distinctPlayerTexts,
    `重试那条发言出现 ${retryCopies} 次；私聊玩家发言 ${windowPlayerLines} 条（去重 ${distinctPlayerTexts} 条）、回复 ${windowReplyLines} 条`); flush()
  record('窗口发言与主线在场发言分频道存放', exchanges.some((e) => e.channel === 'scene'),
    `场景 ${exchanges.filter((e) => e.channel === 'scene').length} 条 / 私聊 ${exchanges.filter((e) => e.channel === 'private').length} 条`); flush()
  record('人物表未因交谈重复登记', (windowAfter.body?.characters || []).length === cast.length,
    `${(windowAfter.body?.characters || []).length} vs ${cast.length}`); flush()

  // --- Fault injection: the window under repeated clicks and lost responses ---
  // Every check so far has been a well-behaved request. What the design calls
  // out is the failure side: a repeated click, a response that never arrives, a
  // connection that dies mid-flight. None of these may produce a second
  // exchange, and none may be recorded as something that happened in the story.
  const faultBefore = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body
  const faultExchangesBefore = faultBefore?.exchanges?.length || 0
  const faultCastBefore = (faultBefore?.characters || []).length

  // Two clicks that overlap rather than queue. The window disables its button
  // while a send is in flight, but a second client, a double-submitted form or a
  // recovery re-read can still race the first, so the guarantee has to be in the
  // backend rather than in the disabled attribute.
  const raceText = '我最后再确认一遍：这事你到底做不做得了主？'
  const raceCommand = `ch-race-${unique}`
  const raceResults = await Promise.all([
    chatWith(storyId, first.id, raceText, raceCommand),
    chatWith(storyId, first.id, raceText, raceCommand),
    chatWith(storyId, first.id, raceText, raceCommand),
  ])
  const raceReplies = raceResults.map((r) => r.body?.reply || '')
  const raceOk = raceResults.every((r) => r.ok)
  const raceSingle = new Set(raceReplies.filter(Boolean)).size === 1
  const afterRace = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body
  const raceCopies = (afterRace?.exchanges || [])
    .filter((e) => e.speaker === 'player' && e.channel === 'private' && e.text === raceText).length
  record('并发重复点击只结算一次', raceOk && raceSingle && raceCopies === 1,
    `3 次并发响应一致=${raceSingle}；该发言在记录中出现 ${raceCopies} 次`); flush()

  // A response that never reaches the client. The request is sent and then
  // aborted: the backend has already committed by the time the abort lands, so
  // the retry must read the reply back rather than ask the character again.
  // This is the case the command ID exists for, and it is checked by identity of
  // the reply, not by counting calls.
  const lostText = '那我先回去了，明早再来听你的准信。'
  const lostCommand = `ch-lost-${unique}`
  const controller = new AbortController()
  let lostAborted = false
  try {
    await fetch(`${BASE}/api/interactive/stories/${storyId}/characters/messages`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command_id: lostCommand, branch: 'main', character_id: first.id, message: lostText }),
      signal: controller.signal,
    })
    // The response arrived before the abort could be issued, which is fine: the
    // point of the check is the retry, not the timing of the abort.
  } catch {
    lostAborted = true
  }
  controller.abort()
  const lostRetry = await chatWith(storyId, first.id, lostText, lostCommand)
  const afterLost = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body
  const lostCopies = (afterLost?.exchanges || [])
    .filter((e) => e.speaker === 'player' && e.channel === 'private' && e.text === lostText).length
  const lostReplyCopies = (afterLost?.exchanges || [])
    .filter((e) => e.speaker === 'character' && e.channel === 'private' && e.text === (lostRetry.body?.reply || '\u0000')).length
  record('提交后响应丢失再重试不重复结算',
    lostRetry.ok && lostCopies === 1 && lostReplyCopies === 1,
    `中断=${lostAborted}；重试回复 ${(lostRetry.body?.reply || '').length} 字；发言 ${lostCopies} 条、对应回复 ${lostReplyCopies} 条`); flush()

  // A request the backend must refuse rather than serve with a default. A
  // character ID that is not in this story's cast is the case the design calls
  // out: the window must not invent a conversation partner, and the refusal must
  // be an error the caller can see rather than a silent fallback to someone
  // else. The failed request must leave no record behind either.
  const ghost = await chatWith(storyId, 'nobody-registered-under-this-id', '这句不该落地。', `ch-ghost-${unique}`)
  const afterGhost = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body
  const ghostLanded = (afterGhost?.exchanges || []).some((e) => e.text === '这句不该落地。')
  record('未登记的人物请求被拒绝且不落记录', !ghost.ok && !ghostLanded,
    `状态 ${ghost.status}；记录中出现=${ghostLanded}`); flush()

  // None of the failure traffic may have changed who is in the story or how
  // many turns it has. A fault that registered a character or advanced the plot
  // would be the real damage, and it would be invisible in the checks above.
  const faultSnapshot = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  record('故障注入未改动人物表与回合数',
    (afterGhost?.characters || []).length === faultCastBefore && (faultSnapshot?.turns?.length || 0) === 2 + negotiationInputs.length,
    `人物 ${faultCastBefore} → ${(afterGhost?.characters || []).length}；回合 ${2 + negotiationInputs.length} → ${faultSnapshot?.turns?.length || 0}`); flush()
  // The two deliberate faults above did settle real exchanges, so the count must
  // have grown by exactly those two — not by three, which is what a double
  // settlement of the concurrent click would look like.
  const faultExchangesAfter = (afterGhost?.exchanges?.length || 0)
  record('故障注入只留下应结算的那几条', faultExchangesAfter === faultExchangesBefore + 4,
    `${faultExchangesBefore} → ${faultExchangesAfter}（并发 2 条 + 丢失重试 2 条）`); flush()


  // --- Deliverable 3: a negotiated arrangement, and one-send advancement ---
  // The matters list is `omitempty`, so an empty conversation omits the key
  // entirely, and two empty lists compare equal. The earlier version therefore
  // passed when nothing at all was recorded. What is required now is grounded in
  // the negotiation the run just performed: the arrangement must have landed on
  // one of the characters actually present, as an open thread of a kind
  // negotiation produces, and it must still be open rather than already
  // discharged as if the player had acted on it.
  //
  // The window scopes matters to one character, and the story is free to put the
  // bargaining in front of anyone it registered — the shopkeeper who was asked,
  // or a companion who spoke up. Reading only `cast[0]` would report "no
  // arrangement" whenever the story chose someone else, so every cast member is
  // read and the arrangement may land on any of them.
  const matterWindows = []
  for (const member of (windowAfter.body?.characters || [])) {
    const w = await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(member.id)}`)
    matterWindows.push({ member, body: w.body || {} })
  }
  // Cross-endpoint invariant: each character's own matter list must match the
  // count the roster reports for them. A dropped or mis-scoped list breaks it,
  // and this now holds for the whole cast rather than one row.
  const disagreements = matterWindows.filter(({ member, body }) =>
    (body.matters || []).length !== (member.open_matters ?? 0))
  record('每人的未了事项与人物表计数一致', matterWindows.length > 0 && disagreements.length === 0,
    disagreements.length
      ? disagreements.map(({ member, body }) =>
          `${member.id}: 窗口 ${(body.matters || []).length} vs 表 ${member.open_matters ?? 0}`).join('；')
      : `${matterWindows.length} 人全部一致`); flush()

  const negotiationKinds = new Set(['permission', 'promise', 'action'])
  const allOpen = matterWindows.flatMap(({ member, body }) =>
    (body.matters || []).filter((m) => m.status === 'open').map((m) => ({ ...m, owner: member.name || member.id })))
  const substantive = allOpen.filter((m) => negotiationKinds.has(m.kind))
  record('商议留下了实际未了安排（许可/承诺/待办）', substantive.length >= 1,
    substantive.length
      ? substantive.map((m) => `${m.owner} ${m.kind}：${String(m.statement || '').slice(0, 34)}`).join(' / ')
      : `未记录许可/承诺/待办，本局未了事项：${allOpen.map((m) => `${m.owner} ${m.kind}/${m.status}`).join('、') || '空'}`); flush()

  // A recorded permission is a right, not an accomplished act. The story was
  // told to act on the arrangement only in the next turn, so at this point any
  // permission must still be open rather than already settled as if it had been
  // used. This is the "permission is not the act it permits" split, checked on
  // real recorded state rather than asserted in prose.
  const permissions = allOpen.filter((m) => m.kind === 'permission')
  record('许可未被当作已履行', permissions.every((m) => m.status === 'open'),
    permissions.length
      ? permissions.map((m) => `${m.owner} ${m.status}`).join('、')
      : '本局未产生许可类事项（由下一步的按商议行动覆盖）'); flush()

  // An action the player committed to but has not yet carried out is the third
  // kind of thread, and it must be recorded as unfinished at this point. The run
  // told the story the player acts on the arrangement only in the next turn, so
  // an `action` matter already marked settled here would mean the story performed
  // the player's move for them.
  const actions = allOpen.filter((m) => m.kind === 'action')
  record('未执行动作仍为未了', actions.every((m) => m.status === 'open'),
    actions.length ? actions.map((m) => `${m.owner} ${m.status}`).join('、') : '本局未产生待办动作类事项'); flush()

  // The refusal and leave paths are exercised rather than assumed. A negotiation
  // that can only ever produce agreement is not a negotiation, so the player asks
  // for something a shopkeeper has no standing to grant and the run checks that
  // the refusal is answered as a refusal — the reply must not read as consent —
  // and that no permission was recorded by it. This is the "拒绝/离开路径" clause:
  // the schema admitting `abandoned` proves nothing; a real decline must land.
  const refusalQuestion = '我要求他立刻把铺子的地契过户给我，并当场交出全部存货。'
  const refusal = await chatWith(storyId, first.id, refusalQuestion, `ch-refuse-${unique}`)
  const refusalReply = refusal.body?.reply || ''
  const refusalIsRefusal = refusal.ok && refusalReply.length > 0
    && /(不|没|无|难|恕|抱歉|不能|无法|不成|岂|断然|摇头|拒绝|拒绝)/.test(refusalReply)
  const refusalConsents = /(好|可以|行|成交|依你|听你的|这就给你|拿去吧|如你所愿)/.test(refusalReply.slice(0, 120))
  record('无理要求被回绝而非应允', refusalIsRefusal && !refusalConsents,
    `${refusalReply.length} 字：${refusalReply.slice(0, 60)}…`); flush()

  const afterRefusal = []
  for (const member of (windowAfter.body?.characters || [])) {
    const w = await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(member.id)}`)
    afterRefusal.push({ member, body: w.body || {} })
  }
  const refusalPermissions = afterRefusal.flatMap(({ member, body }) =>
    (body.matters || []).filter((m) => m.kind === 'permission' && m.status === 'open')
      .map((m) => ({ ...m, owner: member.name || member.id })))
  // The decline must not have manufactured a right. The arrangement the player
  // did negotiate may still stand, so the check is that the refusal added no
  // *new* open permission, not that none exists.
  const beforePermissionIDs = new Set(permissions.map((m) => m.id))
  const newPermissions = refusalPermissions.filter((m) => !beforePermissionIDs.has(m.id))
  record('回绝未凭空产生许可', newPermissions.length === 0,
    newPermissions.length ? newPermissions.map((m) => `${m.owner} ${m.statement}`).join(' / ') : '未新增许可'); flush()

  // --- Cognition: secrets, audience, rumour and counter-evidence ---
  // These are the model-backed cases the review asked for, and they are probes
  // through the real window path rather than reads of an internal table: the
  // window API deliberately exposes no knowledge records, so what is checked is
  // what the character actually says. Each probe is two-sided, because an
  // absence check alone passes for a reply that simply changed the subject.
  //
  // The secret is a coined noun, so its presence or absence in a reply is
  // unambiguous and cannot be satisfied by a paraphrase or a lucky guess.
  const secretToken = '鸦青匣'
  // The peer is a character the roster reports as present, because a private
  // conversation must not become shared knowledge merely because the third party
  // was standing in the same room — that is the audience rule the review names.
  //
  // The roster is re-read rather than reused: `cast` predates the negotiation and
  // the story has grown since. And if the story happens to have only one person
  // on stage, the script brings a second one into the scene rather than reporting
  // the check unrun — a turn that plainly puts two named people in the room is
  // what creates the case, and it is the fixture's job to create it.
  let peerRoster = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
  let peer = peerRoster.find((c) => c.id !== first.id && c.present)
  if (!peer) {
    const beforePeer = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
    const peerTurns = beforePeer.turns?.length || 0
    await post('/api/interactive/chat', {
      command_id: `ch-peer-in-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
      message: '我回到铺子里，把行囊搁在门边。这时从里间走出一个瘦高的年轻人，袖口沾着墨，'
        + '自称姓柳，是柜上管账的。他就在条案边站着，离我和掌柜不过两步远。'
        + '我朝他点了点头，仍压低声音与掌柜说话。',
    })
    await pollSnapshot(storyId, peerTurns + 1, 240000)
    peerRoster = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
    peer = peerRoster.find((c) => c.id !== first.id && c.present)
  }

  const toldSecret = await chatWith(storyId, first.id,
    '我压低声音，只对他一个人说：那批货其实藏在鸦青匣里，就搁在我家灶房后头。'
    + '这话只进你一个人的耳朵，别让第三个人知道。',
    `ch-secret-${unique}`)
  record('私密告知被本人接住', toldSecret.ok && (toldSecret.body?.reply || '').length > 0,
    `${(toldSecret.body?.reply || '').length} 字`); flush()

  const secretRecall = await chatWith(storyId, first.id, '那批货我搁在什么地方了？你替我想想。', `ch-secret-recall-${unique}`)
  const secretReply = secretRecall.body?.reply || ''
  const secretHeld = secretReply.includes(secretToken)
  record('自己的秘密本人可复述', secretRecall.ok && secretHeld,
    `${secretReply.length} 字，含「${secretToken}」=${secretHeld}：${secretReply.slice(0, 60)}`); flush()

  if (peer) {
    const leak = await chatWith(storyId, peer.id, '那批货眼下藏在哪儿，你听说过吗？', `ch-leak-${unique}`)
    const leakReply = leak.body?.reply || ''
    const carries = leakReply.includes(secretToken)
    // The check is two-sided, but the second side is about the secret rather
    // than about the wording of a refusal. Demanding an explicit "不知道" fails
    // a character who simply dodges the question, and dodging is a correct
    // answer: the requirement is that the third party does not come to hold the
    // secret, not that they phrase their ignorance a particular way. So the
    // positive side is that they do not claim to know it — an evasion satisfies
    // that as well as a flat denial does, and both are reported.
    const knowsIt = /(我知道|我清楚|我晓得|我了解|就藏在|藏在[^，。；]{0,8}(里|中|后|下|头)|搁在[^，。；]{0,8}(里|中|后|下|头))/.test(leakReply)
    const denies = /(不知道|不清楚|没听说|没听过|不知|未曾|无从|没有听说|并未听说|不曾|没人和我说|问错人|哪知道|我哪|怎么知道|别问我|说不上|说不好|不晓得|不敢说)/.test(leakReply)
    const withheld = !carries && !knowsIt
    record('他人的私密未被第三人获知', leak.ok && withheld,
      `${peer.name}${peer.present ? '（在场）' : ''}：含秘密=${carries}，宣称知道=${knowsIt}，`
      + `明确否认=${denies}（${denies ? '明确不知' : '回避未答'}）：${leakReply.slice(0, 60)}`); flush()
    record('在场不等于共享私密', !!peer.present && withheld,
      peer.present
        ? `${peer.name} 与主角同处一场，仍不知道私下告知的内容`
        : 'not_run：本局没有与主角同场的第三人可比对'); flush()
  } else {
    record('他人的私密未被第三人获知', false, 'not_run：本局只登记了一人，无第三方可比对'); flush()
    record('在场不等于共享私密', false, 'not_run：本局没有与主角同场的第三人可比对'); flush()
  }

  // A rumour must be carried as a rumour, and counter-evidence must move the
  // belief rather than being ignored. The correction is stated as fact the
  // character has no reason to doubt, then the same question is asked again:
  // an unchanging answer would mean the character is reciting its first
  // impression rather than its current one.
  await chatWith(storyId, first.id,
    '我听说柜上的柳先生挪用了账上的银子，你觉着这事是真的吗？', `ch-rumour-${unique}`)
  const counter = await chatWith(storyId, first.id,
    '我亲自核过账了：账目是平的，那笔银子是东家自己支走的，与柳先生无干。', `ch-counter-${unique}`)
  const counterReply = counter.body?.reply || ''
  const evolved = await chatWith(storyId, first.id, '那柳先生到底有没有挪用银子？', `ch-evolved-${unique}`)
  const evolvedReply = evolved.body?.reply || ''
  // The affirmation set is deliberately narrow and the correction set broad, so
  // the check errs towards reporting a miss: a reply that hedges without
  // repeating the accusation is not read as an affirmation.
  const affirmsTheft = /(柳先生|他)[^。！？]{0,8}(确实|的确|果然|就是)[^。！？]{0,8}(挪用|私吞|贪|拿)/
    .test(evolvedReply)
    || /(确实|的确|果然|确有其事)[^。！？]{0,6}(挪用|私吞|贪了)/.test(evolvedReply)
  // The property under test is that the belief moved, not that it moved into a
  // particular vocabulary. A character who refuses to engage, demands a source,
  // or declines to answer is behaving correctly — the prepared context explicitly
  // tells characters that refusing and deflecting in character is correct, and
  // this character's registered bounds make the question a probe to be refused.
  // Requiring the words 账目/清白/误会/原来 would fail exactly those replies, so
  // the positive side is that the character no longer asserts the theft it was
  // told about. Evidence that the reply is about this question rather than an
  // unrelated deflection is required separately.
  const carriesCorrection = /(账目|账是平|平的|支走|错怪|误会|清白|无干|不是他|与他无关|查清|原来)/.test(evolvedReply)
  const stillEngages = /(柳|挪用|银子|账|谁说的|听谁|凭据|证据|第几|哪一)/.test(evolvedReply)
  const beliefMoved = !affirmsTheft && (carriesCorrection || stillEngages)
  record('传闻被当作传闻处理而非既定事实', counter.ok && counterReply.length > 0,
    `${counterReply.length} 字：${counterReply.slice(0, 60)}`); flush()
  record('反证之后认知随之更正', evolved.ok && beliefMoved,
    `仍断言挪用=${affirmsTheft}，带更正措辞=${carriesCorrection}，仍在谈此事=${stillEngages}`
    + `（拒绝作答本身是正确行为）：${evolvedReply.slice(0, 80)}`); flush()

  // The third-character generic entry: a character the story registered but did
  // not put in this negotiation must still be enterable by the same window path,
  // with no per-character special-casing. A window that only works for the two
  // the script happened to touch would pass every check above.
  //
  // The roster is re-read rather than reused: `cast` was taken before the
  // negotiation turns, and those turns legitimately register whoever they put on
  // stage. Reusing the stale list would report "the story only has two people"
  // as an uncovered check when the story had in fact grown — a fixture defect
  // wearing the costume of a product gap.
  let roster = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
  let untouched = roster.find((c) => c.id !== first.id && c.id !== second?.id)
  if (!untouched) {
    // The story chose to keep its stage to two people, so the script makes the
    // third one walk in: a turn that plainly introduces a new named person is
    // what registers them, and the check is about the window accepting whoever
    // the story registered, not about the story's casting choices.
    const beforeThird = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
    const thirdTurns = beforeThird.turns?.length || 0
    await post('/api/interactive/chat', {
      command_id: `ch-third-in-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
      message: '我出了铺子，在街口遇见一个挑着药担的老汉，自称姓葛，是常年在镇上收山货的。'
        + '我与他攀谈起来，问他是哪里人、做这行多少年了。',
    })
    await pollSnapshot(storyId, thirdTurns + 1, 240000)
    roster = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
    untouched = roster.find((c) => c.id !== first.id && c.id !== second?.id)
  }
  if (untouched) {
    const third = await chatWith(storyId, untouched.id, '我向他打听：这镇上最近可有什么异动？', `ch-third-${unique}`)
    const thirdReply = third.body?.reply || ''
    const thirdSelf = await chatWith(storyId, untouched.id, selfQuestion, `ch-third-self-${unique}`)
    const grounded3 = characterAnswersAsSelf(thirdSelf.body?.reply || '', untouched, roster)
    record('第三人可用同一入口交谈', third.ok && thirdReply.length > 0,
      `${untouched.name}：${thirdReply.length} 字`); flush()
    record('第三人同样以自己的登记身份作答', !!grounded3,
      `${untouched.name}/${untouched.role || '无角色'}：命中「${grounded3 || '无'}」`); flush()
  } else {
    record('第三人可用同一入口交谈', false,
      `本局登记了 ${roster.length} 人，且新引入的人物未被登记，未能覆盖第三人通用入口`); flush()
  }

  const beforeAction = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  const beforeCount = beforeAction.turns?.length || 0
  await post('/api/interactive/chat', {
    command_id: `ch-act-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
    message: '我按刚才与他谈定的条件行事：出示凭据，直接进入坊市里间。',
  })
  const afterAction = await pollSnapshot(storyId, beforeCount + 1, 240000)
  record('按商议行动一次发送即推进', (afterAction.turns?.length || 0) === beforeCount + 1,
    `${beforeCount} → ${afterAction.turns?.length} 回合`); flush()

  // --- Deliverable 4: proactive incoming messages ---
  // A message is caused, not scheduled: the only product path that delivers one
  // is a story turn whose own facts include it, and whether a given turn causes
  // one is the model's call. So the script does what a player would — it puts a
  // concrete reason to write into the fiction, with a contact method and an
  // explicit deadline, then advances the story to the moment that reason comes
  // due, which is the causal basis a message needs.
  //
  // The fixture has to make writing the *only* route, not merely a reasonable
  // one. An earlier version said the character should "send word", and the model
  // simply had him walk over and say it in person — a perfectly good scene, and
  // no message. Nothing about that run was a product defect, so the fixture is
  // what has to change: the protagonist leaves town and gives up his lodging, and
  // the character's own established situation (a leg he cannot walk three li on)
  // rules out following. Word sent to a named place is then the only way the
  // character can reach the protagonist at all.
  //
  // The reason is deliberately one the character owes the protagonist, not one
  // the protagonist owes the character. An appointment the protagonist has to
  // keep gives the character nothing to write about — they are the one waiting —
  // while a decision the character must send word about before a deadline is
  // exactly the unfinished business a message exists to carry.
  //
  // It makes a bounded number of such advances, and if none causes a message it
  // records that as a failure rather than letting an empty list stand in for
  // coverage.
  //
  // The must-fire claim is about a message this appointment caused, so the
  // pending set is baselined first: a message left over from an earlier step
  // would otherwise satisfy the check without the appointment having produced
  // anything at all.
  const baselinePending = await get(`/api/interactive/stories/${storyId}/characters?branch=main`)
  const baselineIDs = new Set((baselinePending.body?.pending || []).map((m) => m.id))

  await post('/api/interactive/chat', {
    command_id: `ch-appoint-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
    message: '我把价钱的事与他定下，却不得不连夜出镇：有人递话给我，说我留在西口外那间破棚子里的行囊被人翻过，'
      + '我得亲自去看一眼。我告诉他，我这一走至少两日不回镇上，城东客栈的房钱我也退了，'
      + '他若有准信，只能托人捎到西口外那间破棚子，我每日午前都在那儿等。'
      + '我盯着他那条腿，把话说明白：你走不了三里泥路，我不指望你亲自来，捎个字条、托个顺路的人带句话都算。'
      + '日落之前我都在。过了日落，那瓶样货我就送去别家，他再要也没有了。说完我背起行囊出了镇。',
  })
  let msgTurns = beforeCount + 2
  await pollSnapshot(storyId, msgTurns, 240000)

  // Each advance moves the story a short way towards the deadline the
  // appointment named. They are deliberately small steps that stay on the near
  // side of it: a jump past the deadline would arrive after the reason to write
  // had already expired, which is a different test and would also spend the
  // deadline the lapse check needs to still be ahead of us.
  //
  // They also keep the protagonist at the named place, so a message has an
  // address to arrive at and the fiction has no route by which the character
  // could simply appear.
  const dueAdvances = [
    '我在西口外那间破棚子里把行囊翻检一遍，收拾停当，就在棚下坐着等信。',
    '日头偏西，路上过了两拨人，没有一个是替我捎话来的。',
    '日头压到山梁上，离日落只剩一线，棚外仍旧没有动静。',
  ]
  let windowMsgs = { body: {} }
  let pending = []
  let fresh = []
  for (let attempt = 0; attempt <= dueAdvances.length; attempt += 1) {
    if (attempt > 0) {
      await post('/api/interactive/chat', {
        command_id: `ch-due${attempt}-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
        message: dueAdvances[attempt - 1],
      })
      msgTurns += 1
      await pollSnapshot(storyId, msgTurns, 240000)
    }
    windowMsgs = await get(`/api/interactive/stories/${storyId}/characters?branch=main`)
    pending = windowMsgs.body?.pending || []
    fresh = pending.filter((m) => !baselineIDs.has(m.id))
    if (fresh.length > 0) break
  }

  // `pending` is `omitempty` too, so its absence must be read as "nothing to deal
  // with" rather than taken on faith. The invariant that makes this a real check
  // is that pending is exactly the cast's unread-and-read messages: it can never
  // be smaller than the unread count the roster reports, it can never carry a
  // character the cast does not list, and it can never carry a handled one.
  const pendingCast = windowMsgs.body?.characters || []
  const castIDs = new Set(pendingCast.map((c) => c.id))
  const unreadTotal = pendingCast.reduce((n, c) => n + (c.unread_messages || 0), 0)
  const pendingScoped = pending.every((m) => castIDs.has(m.character_id))
  const pendingStatuses = pending.every((m) => m.status === 'unread' || m.status === 'read')
  record('来讯查询可用（有无均返回结构）',
    windowMsgs.ok && pending.length >= unreadTotal && pendingScoped && pendingStatuses,
    `待处理 ${pending.length} 条，人物表未读合计 ${unreadTotal}，`
    + `归属本局人物=${pendingScoped}，状态合法=${pendingStatuses}`); flush()

  // The block that follows is the point of this deliverable, and it is entered
  // whether or not a message appeared. An empty list is a real failure of the
  // must-fire setup, not a reason to skip the lifecycle checks, so it is recorded
  // as one and the run continues to the checks that can still be made.
  record('约定到期后产生来讯（必然触发）', fresh.length > 0,
    fresh.length
      ? `${fresh.length} 条新来讯：${fresh.map((m) => `${m.name || m.character_id}「${String(m.text || '').slice(0, 24)}」`).join('、')}`
      : `推进 ${1 + dueAdvances.length} 轮未产生新来讯（原有待处理 ${pending.length} 条不计入）`); flush()

  const message = fresh[0]
  if (message) {
    const beforeTime = message.world_time
    const beforeBasis = message.basis
    record('来讯带原始发送时间', !!message.world_time, `${message.world_time}`); flush()
    record('来讯带因果依据', !!message.basis, String(message.basis).slice(0, 60)); flush()
    // Receiving a message is not accepting it. Nothing about the delivery may
    // have moved the message out of unread or created an arrangement.
    record('来讯不自动接受任何安排', !message.status || message.status === 'unread', `状态 ${message.status}`); flush()

    const readURL = `/api/interactive/stories/${storyId}/characters/${encodeURIComponent(message.character_id)}/messages/${encodeURIComponent(message.id)}/status`
    const marked = await post(readURL, { branch: 'main', status: 'read' })
    record('来讯可标记已读', marked.ok, JSON.stringify(marked.body).slice(0, 100)); flush()
    const afterRead = await get(`/api/interactive/stories/${storyId}/characters?branch=main`)
    const readEntry = (afterRead.body?.pending || []).find((m) => m.id === message.id)
    // A read message stays in the list — the player has seen it, not answered it
    // — and reading must not rewrite when the character wrote.
    record('已读来讯仍在列表且保留原时间',
      !!readEntry && readEntry.world_time === beforeTime,
      readEntry ? `状态 ${readEntry.status}，时间 ${readEntry.world_time}（原 ${beforeTime}）` : '已读后来讯从列表消失'); flush()

    // The same status replayed is the same state, not a second transition. This
    // is the retry/recovery half of the idempotency rule.
    const reread = await post(readURL, { branch: 'main', status: 'read' })
    const afterReread = await get(`/api/interactive/stories/${storyId}/characters?branch=main`)
    const rereadEntry = (afterReread.body?.pending || []).find((m) => m.id === message.id)
    record('重复标记同一状态不产生第二条来讯',
      reread.ok && (afterReread.body?.pending || []).filter((m) => m.id === message.id).length === 1
        && rereadEntry?.world_time === beforeTime,
      `重复标记后待处理 ${(afterReread.body?.pending || []).length} 条`); flush()

    // Ignoring dismisses the message without erasing it. The character still
    // sent it, so the record must keep its text, its basis and its time.
    const ignored = await post(readURL, { branch: 'main', status: 'ignored' })
    record('来讯可忽略', ignored.ok, JSON.stringify(ignored.body).slice(0, 100)); flush()
    const kept = (await get(
      `/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(message.character_id)}&limit=200`)).body?.messages || []
    const keptEntry = kept.find((m) => m.id === message.id)
    record('忽略不等于消息没发生过',
      !!keptEntry && keptEntry.text === message.text && keptEntry.basis === beforeBasis && keptEntry.world_time === beforeTime,
      keptEntry
        ? `状态 ${keptEntry.status}，原文与依据保留，时间 ${keptEntry.world_time}`
        : '忽略后来讯从记录中消失'); flush()

    // Lapse is measured against the story's own clock, so the check is only
    // meaningful once that clock has actually passed the deadline. Reading the
    // scene before and after is what lets a failure be attributed: a deadline
    // that is still ahead of us is an unrun check, not a broken product, and the
    // report must not confuse the two.
    if (message.deadline) {
      // Read the character's own message list rather than `pending`: by this
      // point the message has been ignored, so a pending lookup would return
      // nothing and the check would pass on an absent subject.
      const beforeLapse = await get(`/api/interactive/stories/${storyId}/characters?branch=main`
        + `&character_id=${encodeURIComponent(message.character_id)}&limit=200`)
      const clockBefore = beforeLapse.body?.scene?.world_time || ''
      const lapseEntry = (beforeLapse.body?.messages || []).find((m) => m.id === message.id)
      // Only assert "not lapsed" when the clock is unambiguously still before the
      // deadline. A model that wrote a deadline already behind the current scene
      // time has produced a lapsed message on arrival, which is a different
      // situation from the one this check is about, and reporting it as a defect
      // would be blaming the product for the fixture.
      const deadlineNumber = leadingNumber(message.deadline)
      const clockNumber = leadingNumber(clockBefore)
      const stillAhead = deadlineNumber !== null && clockNumber !== null && clockNumber <= deadlineNumber
      if (!lapseEntry) {
        record('未到期来讯不算失效', false, '来讯不在该人物记录中'); flush()
      } else if (!stillAhead) {
        record('未到期来讯不算失效', false,
          `not_run：期限 ${message.deadline} 与场景时间 ${clockBefore || '(未记录)'} 无法判定先后`); flush()
      } else {
        record('未到期来讯不算失效', lapseEntry.lapsed === false,
          `期限 ${message.deadline}，场景时间 ${clockBefore}，失效=${lapseEntry.lapsed}`); flush()
      }

      // Advance the story until its clock is unambiguously past the deadline.
      // The driver does not re-implement the product's world-time comparison: it
      // only claims the deadline passed when the clock's leading number is
      // strictly greater, which every reasonable ordering agrees on. Outside that
      // regime the check is reported unrun rather than guessed, so a tie-breaking
      // difference cannot manufacture a pass.
      let clockAfter = clockBefore
      for (let advance = 0; advance < 3; advance += 1) {
        const clockNumber = leadingNumber(clockAfter)
        if (deadlineNumber !== null && clockNumber !== null && clockNumber > deadlineNumber) break
        await post('/api/interactive/chat', {
          command_id: `ch-lapse${advance}-${storyId}`, mode: 'story', story_id: storyId, branch: 'main',
          message: '此后我在城中盘桓了许久，一晃过了大半个月，早把旧约搁下了。',
        })
        msgTurns += 1
        const snap = await pollSnapshot(storyId, msgTurns, 240000)
        clockAfter = snap?.scene?.world_time || clockAfter
      }
      const afterLapse = (await get(
        `/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(message.character_id)}&limit=200`)).body?.messages || []
      const lapsedEntry = afterLapse.find((m) => m.id === message.id)
      const clockAfterNumber = leadingNumber(clockAfter)
      const past = deadlineNumber !== null && clockAfterNumber !== null && clockAfterNumber > deadlineNumber
      if (!lapsedEntry) {
        record('过期来讯标记失效且保留原时间', false, '过期后来讯从记录中消失'); flush()
      } else if (!past) {
        record('过期来讯标记失效且保留原时间', false,
          `not_run：推进后场景时间 ${clockAfter || '(未记录)'} 未明确越过期限 ${message.deadline}`); flush()
      } else {
        record('过期来讯标记失效且保留原时间',
          lapsedEntry.lapsed === true && lapsedEntry.world_time === beforeTime,
          `场景时间 ${clockBefore || '(未记录)'} → ${clockAfter}，期限 ${message.deadline}，`
          + `失效=${lapsedEntry.lapsed}，原时间保留=${lapsedEntry.world_time === beforeTime}`); flush()
      }
    } else {
      record('未到期来讯不算失效', false, 'not_run：本条来讯未带期限'); flush()
      record('过期来讯标记失效且保留原时间', false, 'not_run：本条来讯未带期限，无法验证失效'); flush()
    }

    // Reply and act: a reply is an ordinary window exchange against the same
    // person, and only then may the message be marked handled. This is the
    // "reply/action" path, and it must not be conflated with reading.
    const replyTo = await chatWith(storyId, message.character_id, '我回他的话：那件事我另作打算，改日再说。', `ch-reply-msg-${unique}`)
    record('可回复来讯', replyTo.ok && (replyTo.body?.reply || '').length > 0,
      `${(replyTo.body?.reply || '').length} 字`); flush()
    const handled = await post(readURL, { branch: 'main', status: 'handled' })
    const afterHandled = (await get(
      `/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(message.character_id)}&limit=200`)).body?.messages || []
    const handledEntry = afterHandled.find((m) => m.id === message.id)
    record('回复后可标记处理完毕且保留原时间',
      handled.ok && handledEntry?.status === 'handled' && handledEntry?.world_time === beforeTime,
      handledEntry ? `状态 ${handledEntry.status}，时间 ${handledEntry.world_time}` : '处理完毕后来讯从记录中消失'); flush()
  } else {
    // The lifecycle checks cannot be made without a message, and they are not
    // silently dropped: each one is recorded as unrun so the report cannot read
    // as coverage it does not have.
    // This list must stay in step with the checks above: a name missing here is a
    // check that disappears from the report entirely, which reads as coverage the
    // run never had. `未到期来讯不算失效` was omitted and would have done exactly
    // that whenever the fixture produced no message.
    for (const name of [
      '来讯带原始发送时间', '来讯带因果依据', '来讯不自动接受任何安排', '来讯可标记已读',
      '已读来讯仍在列表且保留原时间', '重复标记同一状态不产生第二条来讯', '来讯可忽略',
      '忽略不等于消息没发生过', '未到期来讯不算失效', '过期来讯标记失效且保留原时间',
      '可回复来讯', '回复后可标记处理完毕且保留原时间',
    ]) {
      record(name, false, 'not_run：本轮未产生来讯，缺少可验证的对象'); flush()
    }
  }

  // --- Isolation: a second story keeps its own cast ---
  // The baseline is re-read here rather than reused from earlier: the turns that
  // ran since then legitimately grew the main cast, and a stale baseline would
  // report that growth as cross-story contamination.
  const cast1Before = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
  const story2 = await post('/api/interactive/stories', { ...request, title: `隔离人物-${unique}` })
  const cast2 = story2.body?.id
    ? (await get(`/api/interactive/stories/${story2.body.id}/characters?branch=main`)).body?.characters || []
    : []
  record('同工作区新故事人物表为空（不串档）', cast2.length === 0, `${cast2.length} 人`); flush()
  const cast1After = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
  record('新故事未影响原故事人物表', cast1After.length === cast1Before.length,
    `${cast1After.length} vs ${cast1Before.length}`); flush()

  // --- Isolation: two branches keep separate character memory ---
  const mainSnap = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body
  const lastTurnId = mainSnap.turns?.[(mainSnap.turns?.length || 0) - 1]?.id
  const br = await post(`/api/interactive/stories/${storyId}/branches`, { parent_event_id: lastTurnId, title: `人物支线-${unique}` })
  record('创建分支', br.ok && !!br.body?.id, br.body?.id || JSON.stringify(br.body).slice(0, 120)); flush()
  if (br.body?.id) {
    const branchCast = (await get(`/api/interactive/stories/${storyId}/characters?branch=${encodeURIComponent(br.body.id)}`)).body?.characters || []
    const mainCastNow = (await get(`/api/interactive/stories/${storyId}/characters?branch=main`)).body?.characters || []
    record('分支继承人物登记（同一份记忆起点）', branchCast.length === mainCastNow.length,
      `分支 ${branchCast.length} 人 vs 主线 ${mainCastNow.length} 人`); flush()

    // The branch-only line is what proves the two memories are separate. Counts
    // cannot: the window pages at 40, so both branches report the same capped
    // length and a count comparison would pass for the wrong reason.
    const branchOnly = `这条支线里我改口不谈了-${unique}`
    await chatWith(storyId, first.id, branchOnly, `ch-branch-${unique}`, br.body.id)
    const branchLines = (await get(`/api/interactive/stories/${storyId}/characters?branch=${encodeURIComponent(br.body.id)}&character_id=${encodeURIComponent(first.id)}&limit=200`)).body?.exchanges || []
    const mainLines = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body?.exchanges || []
    const inBranch = branchLines.some((e) => e.text.includes(branchOnly))
    const inMain = mainLines.some((e) => e.text.includes(branchOnly))
    record('分支交谈写入分支自身', inBranch, `分支 ${branchLines.length} 条`); flush()
    record('分支交谈不写回主线', inBranch && !inMain,
      `分支含支线发言=${inBranch}，主线含支线发言=${inMain}`); flush()
    await post(`/api/interactive/stories/${storyId}/switch-branch`, { branch_id: 'main' })
  }

  // --- Browser: the real window, on the real stage ---
  const browser = await chromium.launch()
  const context = await browser.newContext({ baseURL: BASE, locale: 'zh-CN', serviceWorkers: 'block', reducedMotion: 'reduce' })
  const page = await context.newPage()
  const consoleErrors = []
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()) })
  page.on('pageerror', (e) => consoleErrors.push(`pageerror: ${e.message}`))

  try {
    await page.goto('/')
    await page.waitForTimeout(5000)
    const skip = page.getByRole('button', { name: '跳过引导' }).first()
    if (await skip.isVisible().catch(() => false)) { await skip.click(); await page.waitForTimeout(1200) }

    const gameBtn = await findGameButton(page)
    if (gameBtn) await gameBtn.click()
    await page.waitForTimeout(2500)

    const picker = page.getByRole('button', { name: '选择故事线' }).first()
    if (await picker.isVisible().catch(() => false)) {
      await picker.click()
      await page.waitForTimeout(1200)
      const option = page.locator(`button[aria-label="${story.body.title || `人物验收故事-${unique}`}"]`).first()
      if (await option.isVisible().catch(() => false)) { await option.click(); await page.waitForTimeout(4000) }
    }
    await page.getByPlaceholder(/你要做什么/).first().waitFor({ state: 'visible', timeout: 40000 })
    record('主舞台可用', true, '输入框可见'); flush()

    // The character entry point lives on the original stage.
    const openBtn = page.getByRole('button', { name: '打开人物窗口' }).first()
    const openVisible = await openBtn.isVisible().catch(() => false)
    record('主舞台含人物窗口入口', openVisible, openVisible ? '找到按钮' : '未找到'); flush()
    if (openVisible) {
      await openBtn.click()
      await page.waitForTimeout(2500)
      await page.screenshot({ path: `${ART}/01-window-open.png` })

      const panel = page.locator('[data-testid="character-window-panel"]').first()
      const panelVisible = await panel.waitFor({ state: 'visible', timeout: 20000 }).then(() => true).catch(() => false)
      record('人物窗口打开', panelVisible, panelVisible ? '面板可见' : '面板未出现'); flush()

      const rows = page.locator('[data-testid="character-window-roster-row"]')
      const rowCount = await rows.count().catch(() => 0)
      record('窗口列出本局人物', rowCount >= 1, `${rowCount} 行`); flush()

      // Clicking the row by its own character id is what makes this a real
      // selection rather than a click on whatever text happened to match.
      const row = page.locator(`[data-testid="character-window-roster-row"][data-character-id="${first.id}"]`).first()
      const rowVisible = await row.isVisible().catch(() => false)
      record('窗口按人物 id 列出该人物', rowVisible, rowVisible ? `找到 ${first.id}` : `未找到 ${first.id}`); flush()
      if (rowVisible) { await row.click(); await page.waitForTimeout(2500) }

      // The property is that the window re-reads committed conversation, so the
      // line to look for is one the default page is guaranteed to hold. The
      // window pages at 40 and `reply1` is the *earliest* window reply; by the
      // time this browser step runs the story has moved well past it, and the
      // target line sits one slot off the newest page. Asserting on the oldest
      // line therefore tested the fixture's assumption, not the product.
      const windowNow = await get(`/api/interactive/stories/${storyId}/characters?branch=main`
        + `&character_id=${encodeURIComponent(first.id)}&limit=200`)
      const committed = (windowNow.body?.exchanges || []).filter((e) => (e.text || '').trim())
      const newestLine = (committed[committed.length - 1]?.text || '').trim()
      const priorLine = (newestLine || reply1).slice(0, 10)
      const historyShown = priorLine
        ? await page.getByText(priorLine, { exact: false }).first().waitFor({ state: 'visible', timeout: 20000 })
          .then(() => true).catch(() => false)
        : false
      record('窗口回看已提交的交谈', historyShown,
        `匹配「${priorLine}…」（窗口共 ${committed.length} 条，默认页显示最近 40 条）`); flush()
      await page.screenshot({ path: `${ART}/02-conversation.png` })

      // Send one line from the real window UI. The placeholder carries the
      // selected character's known name, so matching its prefix is enough.
      const input = page.getByPlaceholder(/^对 .+ 说/).first()
      const inputVisible = await input.isVisible().catch(() => false)
      if (inputVisible) {
        const beforeUi = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(first.id)}&limit=200`)).body?.exchanges?.length || 0
        await input.fill('那你开个价吧。')
        await page.locator('[data-testid="character-window-panel"] button[aria-label="发送"]').first().click()
        const grew = await waitForExchangeCount(storyId, first.id, beforeUi + 2, 150000)
        record('窗口内发送并收到回复', grew >= beforeUi + 2, `${beforeUi} → ${grew} 条`); flush()
        await page.screenshot({ path: `${ART}/03-window-reply.png` })
      } else {
        record('窗口内发送并收到回复', false, '未找到窗口输入框'); flush()
      }

      // Closing returns the player to the reading position.
      await page.getByRole('button', { name: '关闭人物窗口' }).first().click()
      await page.waitForTimeout(1500)
      const composerBack = await page.getByPlaceholder(/你要做什么/).first().isVisible().catch(() => false)
      record('关闭窗口回到阅读位置', composerBack, composerBack ? '主舞台输入框仍在' : '未回到主舞台'); flush()
      await page.screenshot({ path: `${ART}/04-back-to-stage.png` })
    }

    // Narrow screen: the same window must be reachable as an overlay.
    await page.setViewportSize({ width: 420, height: 860 })
    await page.waitForTimeout(1500)
    const narrowBtn = page.getByRole('button', { name: '打开人物窗口' }).first()
    const narrowVisible = await narrowBtn.isVisible().catch(() => false)
    record('窄屏仍有人物入口', narrowVisible, narrowVisible ? '入口可见' : '入口不可见'); flush()
    if (narrowVisible) {
      await narrowBtn.click()
      await page.waitForTimeout(2500)
      // The sheet reopens on whoever was selected, so it shows that
      // conversation rather than the roster. Either is a valid open; what must
      // hold is that the panel itself is on screen and reachable.
      const narrowPanel = await page.locator('[data-testid="character-window-panel"]').first()
        .waitFor({ state: 'visible', timeout: 15000 }).then(() => true).catch(() => false)
      const narrowRows = await page.locator('[data-testid="character-window-roster-row"]').count().catch(() => 0)
      const narrowBack = await page.getByRole('button', { name: '返回人物列表' }).first().isVisible().catch(() => false)
      record('窄屏以浮层打开人物窗口', narrowPanel && (narrowRows >= 1 || narrowBack),
        `面板=${narrowPanel} 列表行=${narrowRows} 会话态=${narrowBack}`); flush()
      await page.screenshot({ path: `${ART}/05-narrow-window.png` })
      const closeNarrow = page.getByRole('button', { name: '关闭人物窗口' }).first()
      if (await closeNarrow.isVisible().catch(() => false)) { await closeNarrow.click(); await page.waitForTimeout(1000) }
    }
    await page.setViewportSize({ width: 1440, height: 900 })

    // Reload is not a restart substitute, but the window must restore.
    await page.reload()
    await page.waitForTimeout(6000)
    const gameBtn2 = await findGameButton(page)
    if (gameBtn2) await gameBtn2.click()
    await page.waitForTimeout(2500)
    const picker2 = page.getByRole('button', { name: '选择故事线' }).first()
    if (await picker2.isVisible().catch(() => false)) {
      await picker2.click()
      await page.waitForTimeout(1200)
      const option2 = page.locator(`button[aria-label="${story.body.title || `人物验收故事-${unique}`}"]`).first()
      if (await option2.isVisible().catch(() => false)) { await option2.click(); await page.waitForTimeout(4000) }
    }
    const openBtn2 = page.getByRole('button', { name: '打开人物窗口' }).first()
    if (await openBtn2.isVisible().catch(() => false)) {
      await openBtn2.click()
      await page.waitForTimeout(2500)
      const restoredRows = page.locator('[data-testid="character-window-roster-row"]')
      const restoredCount = await restoredRows.count().catch(() => 0)
      record('刷新后人物窗口恢复', restoredCount >= 1, `${restoredCount} 行`); flush()
      await page.screenshot({ path: `${ART}/06-after-reload.png` })
    }
  } catch (e) {
    record('浏览器人物链路验收', false, e.message.split('\n')[0]); flush()
    await page.screenshot({ path: `${ART}/99-ui-failure.png` }).catch(() => {})
  }

  const unexpected = consoleErrors.filter((t) => !/404|favicon|Failed to load resource/.test(t))
  record('浏览器无未预期报错', unexpected.length === 0, unexpected.slice(0, 3).join(' | ')); flush()
  await browser.close()
  record('验收总耗时', true, `${Math.round((Date.now() - started) / 1000)} 秒`); flush()
  // NAR-18: the preparation this batch added must be visible as a cost, not a
  // claim. Model-backed calls are counted and timed apart from journal reads.
  const slowest = metrics.longest.slice().sort((a, b) => b.ms - a.ms)[0]
  record('模型调用与等待可计量（NAR-18）', metrics.modelCalls > 0,
    `模型调用 ${metrics.modelCalls} 次 / 合计 ${Math.round(metrics.modelMs / 1000)} 秒；`
    + `首次等待 ${metrics.firstWaitMs === null ? 'n/a' : Math.round(metrics.firstWaitMs / 1000) + ' 秒'}；`
    + `最长 ${slowest ? Math.round(slowest.ms / 1000) + ' 秒 (' + slowest.path + ')' : 'n/a'}；`
    + `日志读取 ${metrics.reads} 次 / 合计 ${Math.round(metrics.readMs / 1000)} 秒`); flush()
  finish()
}

// characterAnswersAsSelf returns the piece of this character's own registered
// identity that their reply actually carried, or '' if the reply could have come
// from anyone. It returns the evidence rather than a boolean so two characters'
// answers can be compared: a shared generic voice would produce the same
// evidence for both, which is what makes "the replies differ" insufficient.
// characterAnswersAsSelf reports the piece of the character's own registered
// profile the reply is grounded in, or '' when the reply could have come from
// any generic voice. Two things are required, because either alone is
// satisfiable for the wrong reason:
//
//   - The reply must not claim to be a different registered character. Talking
//     about someone else by name is normal and must not trip this, so only an
//     explicit identity claim is rejected. A generic voice that recites whoever
//     it was last told about fails here, which is the failure this check exists
//     to catch.
//   - The reply must use a phrase that is registered for this character and for
//     no one else in the cast. Requiring uniqueness is what makes the match
//     evidence: a phrase several characters share says nothing about who is
//     speaking, and a character registered as deliberately anonymous has no name
//     to recite, so their own description is the only thing that can ground them.
//
// The matched term is returned rather than a boolean so the report shows what
// the reply was grounded in and a reader can judge the match.
function characterAnswersAsSelf(reply, character, cast = []) {
  const text = String(reply || '')
  if (!text) return ''
  for (const other of cast) {
    if (!other || other.id === character.id) continue
    for (const name of [other.name, other.known_as].map(cjk).filter((v) => v.length >= 2)) {
      if (new RegExp(`(我是|我叫|我姓|在下|我便是|我就是)${escapeRegExp(name)}`).test(text)) return ''
    }
  }
  const declared = [character.name, character.known_as, character.role, ...(character.aliases || [])]
    .map(cjk)
    .filter((v) => v.length >= 2)
  for (const candidate of declared) {
    if (text.includes(candidate)) return candidate
  }
  for (const term of exclusiveProfileTerms(character, cast)) {
    if (text.includes(term)) return term
  }
  return ''
}

function cjk(value) {
  return String(value || '').trim()
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

// exclusiveProfileTerms returns phrases of three or more CJK characters that
// appear in this character's own registration and in no other cast member's.
// Two-character grams are excluded because they are common enough in ordinary
// prose that a match would prove nothing.
function exclusiveProfileTerms(character, cast = []) {
  const mine = registrationText(character)
  const others = cast
    .filter((c) => c && c.id !== character.id)
    .map(registrationText)
    .join(' ')
  const terms = new Set()
  for (const run of mine.match(/[一-鿿]{3,}/gu) || []) {
    for (let size = 4; size >= 3; size -= 1) {
      for (let i = 0; i + size <= run.length; i += 1) {
        terms.add(run.slice(i, i + size))
      }
    }
  }
  return [...terms]
    .filter((term) => !others.includes(term))
    .sort((a, b) => b.length - a.length)
}

function registrationText(character) {
  return [character.name, character.known_as, character.role, character.description, ...(character.aliases || [])]
    .map(cjk)
    .join(' ')
}

// classifyChoiceIntent reduces a choice to the kind of action it proposes, so
// that three rewordings of one action land in one class and fail the
// distinctness check, while three genuinely different actions do not. A choice
// whose action is not recognisable keeps its own first phrase as its class, so
// an unclassifiable option is never counted as "different" by default.
function classifyChoiceIntent(choice) {
  const text = String(choice || '')
  if (/(拒绝|回绝|不接|婉拒|谢绝|不答应|摇头|退走|离开|告辞|转身)/.test(text)) return '拒绝/离开'
  if (/(接受|应下|答应|同意|点头|成交|收下|签下)/.test(text)) return '接受'
  if (/(讨价|还价|压价|加价|改成|改为|要求|条件|利息|期限|再谈|商量)/.test(text)) return '议价/提条件'
  if (/(询问|打听|问|探|试探|套话)/.test(text)) return '询问/试探'
  if (/(等|观望|按兵|不表态|沉默|静观)/.test(text)) return '观望'
  if (/(动手|攻击|出手|拔|打|抢|强行)/.test(text)) return '动武'
  return `其他:${text.slice(0, 6)}`
}

async function chatWith(storyId, characterId, message, commandId, branchId) {
  return post(`/api/interactive/stories/${storyId}/characters/messages`, {
    command_id: commandId, branch: branchId || 'main', character_id: characterId, message,
  })
}
async function findGameButton(page) {
  const candidates = [
    page.getByRole('button', { name: '游戏', exact: true }),
    page.locator('button', { hasText: /^游戏$/ }),
    page.locator('[data-mode="story"]'),
    page.getByText('游戏', { exact: true }),
  ]
  for (const c of candidates) {
    const first = c.first()
    if (await first.isVisible().catch(() => false)) return first
  }
  return null
}
async function waitForExchangeCount(storyId, characterId, want, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let count = 0
  while (Date.now() < deadline) {
    const body = (await get(`/api/interactive/stories/${storyId}/characters?branch=main&character_id=${encodeURIComponent(characterId)}&limit=200`)).body
    count = (body?.exchanges || []).length
    if (count >= want) return count
    await new Promise((r) => setTimeout(r, 2500))
  }
  return count
}
async function post(path, data) { return req('POST', path, data) }
async function get(path) { return req('GET', path) }
// leadingNumber reads the first run of digits in a world-time label. Stories
// author their own labels ("第5日 辰时", "三年春"), so the driver only uses this to
// decide whether the clock has moved unambiguously past a deadline; the product
// owns the actual comparison, and the Go tests cover its tie-breaking.
function leadingNumber(value) {
  const match = String(value ?? '').match(/\d+/)
  return match ? Number(match[0]) : null
}
async function req(method, path, data) {
  const startedAt = Date.now()
  const res = await fetch(`${BASE}${path}`, {
    method, headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  })
  const text = await res.text()
  let body; try { body = JSON.parse(text) } catch { body = text }
  const elapsed = Date.now() - startedAt
  if (isModelCall(path)) {
    metrics.modelCalls += 1
    metrics.modelMs += elapsed
    if (metrics.firstWaitMs === null) metrics.firstWaitMs = elapsed
    metrics.longest.push({ path, ms: elapsed })
  } else {
    metrics.reads += 1
    metrics.readMs += elapsed
  }
  return { ok: res.ok, status: res.status, body }
}
async function pollSnapshot(storyId, wantTurns, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let last = {}
  while (Date.now() < deadline) {
    last = (await get(`/api/interactive/stories/${storyId}/snapshot?branch=main`)).body || {}
    if ((last.turns?.length || 0) >= wantTurns) return last
    await new Promise((r) => setTimeout(r, 2500))
  }
  return last
}
function finish() {
  flush()
  console.log(`==== 人物交流验收：${steps.length - failed}/${steps.length} 通过 ====`)
  for (const s of steps) console.log(`${s.ok ? 'PASS' : 'FAIL'}  ${s.name}${s.detail ? ' — ' + s.detail : ''}`)
  if (failed) process.exitCode = 1
}

main().catch((e) => { record('脚本异常', false, e.message); flush(); console.error(e); process.exitCode = 1 })
