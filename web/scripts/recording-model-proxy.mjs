/**
 * A recording passthrough for the model endpoint.
 *
 * The style acceptance has to show which expression fragments a real generation
 * request actually carried, and the only place that is observable without
 * trusting the product's own reporting is the wire. This proxy sits between the
 * backend and the live gateway, forwards every request and response unchanged,
 * and appends one redacted record per exchange.
 *
 * Deliberately not recorded: the Authorization header, the api key, the full
 * prompt, and any reasoning text. Recorded instead: the request shape (model,
 * stream, thinking-control fields), a SHA-256 and byte count of the system
 * instruction, the headings of the fragments the product injected, and a short
 * bounded excerpt of only the expression sections. That excerpt is the evidence
 * the acceptance needs and nothing else can stand in for it.
 */
import http from 'node:http'
import { appendFileSync, mkdirSync } from 'node:fs'
import { createHash } from 'node:crypto'
import path from 'node:path'

// The product injects each purpose as its own fragment under a stable heading,
// so the heading is what identifies which purpose arrived.
const STYLE_SECTIONS = [
  { purpose: 'main', heading: '# Storyteller System Rules' },
  { purpose: 'dialogue', heading: '## Dialogue Expression' },
  { purpose: 'combat', heading: '## Combat Expression' },
]

// Fragments that can follow the style sections. A section ends at whichever
// comes first: the next style heading, or one of these. Without this the last
// style section would run to the end of the instruction and swallow the
// workflow rules that follow it, which would overstate what the style injected.
//
// The built-in workflow is anchored by its own opening sentence rather than by a
// heading. `# Text Adventure Built-in System Instruction` is prepended to the FIRST
// built-in fragment, so it sits before the styles, not after them; the workflow
// fragment itself is appended with no prefix at all and therefore has no
// separator to match. Measured before this anchor existed: the last style
// section ran on into the workflow and reported 22379 characters where the style
// text was 623 — a 35x overstatement of what the style actually injected.
const SECTION_TERMINATORS = [
  '\n\n## Game Planning Guide\n\n',
  "You are Text Adventure's Game Mode Agent. Generate only the next story-stage turn in response to the user's action.",
]

const EXCERPT_LIMIT = 400
const SYSTEM_EXCERPT_LIMIT = 200
// Assistant prose is the deliverable the acceptance reads, so it is kept longer
// than a framing excerpt: a style verdict drawn from the first 400 characters of
// a multi-paragraph turn is a verdict about the opening sentence, not the turn.
// Still bounded, and still output only — never the prompt or any reasoning text.
const PROSE_LIMIT = 2400

function sha256(value) {
  return createHash('sha256').update(value, 'utf8').digest('hex')
}

function excerpt(value, limit = EXCERPT_LIMIT) {
  const text = String(value ?? '')
  return text.length <= limit ? text : `${text.slice(0, limit)}…[+${text.length - limit}]`
}

/**
 * Pulls the system instruction out of either request shape the product can
 * send: chat-completions carries it as a system message, the responses API as
 * top-level instructions. Both are read so a protocol change shows up as a
 * missing section rather than as a silently empty record.
 */
function systemText(body) {
  const parts = []
  if (typeof body?.instructions === 'string') parts.push(body.instructions)
  for (const message of Array.isArray(body?.messages) ? body.messages : []) {
    const role = message?.role
    if (role !== 'system' && role !== 'developer') continue
    if (typeof message?.content === 'string') parts.push(message.content)
    else if (Array.isArray(message?.content)) {
      for (const part of message.content) {
        if (typeof part?.text === 'string') parts.push(part.text)
      }
    }
  }
  return parts.join('\n')
}

/**
 * Slices the system instruction at the headings above. A section runs to the
 * next heading so the excerpt contains that purpose's own text and the guard
 * assembled ahead of it, and stops there rather than continuing into the
 * workflow rules that follow.
 */
function styleSections(text) {
  const found = []
  const positions = []
  for (const section of STYLE_SECTIONS) {
    const at = text.indexOf(section.heading)
    if (at >= 0) positions.push({ ...section, at })
  }
  positions.sort((a, b) => a.at - b.at)
  for (let i = 0; i < positions.length; i += 1) {
    const start = positions[i].at
    let end = i + 1 < positions.length ? positions[i + 1].at : text.length
    for (const terminator of SECTION_TERMINATORS) {
      const at = text.indexOf(terminator, start)
      if (at >= 0 && at < end) end = at
    }
    found.push({
      purpose: positions[i].purpose,
      heading: positions[i].heading,
      chars: end - start,
      sha256: sha256(text.slice(start, end)),
      excerpt: excerpt(text.slice(start, end)),
    })
  }
  return found
}

/** Thinking controls, recorded by name so an ineffective switch is visible. */
function thinkingFields(body) {
  const fields = {}
  for (const key of ['reasoning_effort', 'reasoning', 'thinking', 'enable_thinking', 'thinking_budget', 'thinking_level', 'include_reasoning']) {
    if (body != null && Object.hasOwn(body, key)) fields[key] = body[key]
  }
  return fields
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = []
    req.on('data', (chunk) => chunks.push(chunk))
    req.on('end', () => resolve(Buffer.concat(chunks)))
    req.on('error', reject)
  })
}

/** Accumulates streamed assistant content without keeping the whole stream. */
function createStreamCollector() {
  let content = ''
  let reasoning = 0
  let finishReason = null
  let usage = null
  let pending = ''
  return {
    feed(chunk) {
      pending += chunk
      const lines = pending.split('\n')
      pending = lines.pop() ?? ''
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed.startsWith('data:')) continue
        const payload = trimmed.slice(5).trim()
        if (!payload || payload === '[DONE]') continue
        let event
        try { event = JSON.parse(payload) } catch { continue }
        if (event?.usage) usage = event.usage
        const choice = event?.choices?.[0]
        if (!choice) continue
        if (choice.finish_reason) finishReason = choice.finish_reason
        const delta = choice.delta ?? choice.message
        if (typeof delta?.content === 'string') content += delta.content
        if (typeof delta?.reasoning_content === 'string') reasoning += delta.reasoning_content.length
        if (typeof delta?.reasoning === 'string') reasoning += delta.reasoning.length
      }
    },
    result() {
      return { content, reasoningChars: reasoning, finishReason, usage }
    },
  }
}

export function startRecordingProxy({ port, upstream, requestLog, responseLog, onRecord, onConnect }) {
  mkdirSync(path.dirname(requestLog), { recursive: true })
  mkdirSync(path.dirname(responseLog), { recursive: true })
  const target = new URL(upstream)
  let sequence = 0

  const server = http.createServer(handleRequest)

  async function handleRequest(req, res) {
    const id = `req-${++sequence}`
    onConnect?.({ id, method: req.method, path: req.url, headers: Object.keys(req.headers) })
    const raw = await readBody(req).catch(() => Buffer.alloc(0))
    let body = null
    try { body = JSON.parse(raw.toString('utf8')) } catch { body = null }
    if (!body) onConnect?.({ id, method: req.method, path: req.url, unparsedBytes: raw.length })

    if (body) {
      const system = systemText(body)
      const record = {
        id,
        ts: new Date().toISOString(),
        path: req.url,
        model: body.model ?? null,
        stream: body.stream === true,
        messageCount: Array.isArray(body.messages) ? body.messages.length : null,
        systemChars: system.length,
        systemSha256: system.length ? sha256(system) : null,
        systemExcerpt: excerpt(system, SYSTEM_EXCERPT_LIMIT),
        styleSections: styleSections(system),
        thinking: thinkingFields(body),
        bodyKeys: Object.keys(body).sort(),
      }
      appendFileSync(requestLog, `${JSON.stringify(record)}\n`, 'utf8')
      onRecord?.({ kind: 'request', record })
    }

    const headers = { ...req.headers }
    delete headers.host
    delete headers['content-length']
    const proxied = http.request({
      hostname: target.hostname,
      port: target.port,
      path: `${target.pathname.replace(/\/$/, '')}${req.url}`,
      method: req.method,
      headers,
    }, (upstreamRes) => {
      res.writeHead(upstreamRes.statusCode ?? 502, upstreamRes.headers)
      const isStream = String(upstreamRes.headers['content-type'] ?? '').includes('text/event-stream')
      const collector = createStreamCollector()
      upstreamRes.on('data', (chunk) => {
        if (isStream) collector.feed(chunk.toString('utf8'))
        res.write(chunk)
      })
      upstreamRes.on('end', () => {
        res.end()
        const collected = isStream
          ? collector.result()
          : { content: '', reasoningChars: 0, finishReason: null, usage: null }
        const responseRecord = {
          id,
          ts: new Date().toISOString(),
          status: upstreamRes.statusCode ?? null,
          stream: isStream,
          contentChars: collected.content.length,
          contentExcerpt: excerpt(collected.content, PROSE_LIMIT),
          reasoningChars: collected.reasoningChars,
          finishReason: collected.finishReason,
          usage: collected.usage,
        }
        appendFileSync(responseLog, `${JSON.stringify(responseRecord)}\n`, 'utf8')
        onRecord?.({ kind: 'response', record: responseRecord })
      })
      upstreamRes.on('error', () => res.end())
    })
    proxied.on('error', (error) => {
      if (!res.headersSent) res.writeHead(502, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ error: { message: `recording proxy upstream failed: ${error.message}` } }))
    })
    proxied.end(raw)
  }

  return new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(port, '127.0.0.1', () => resolve({
      port,
      close: () => new Promise((done) => server.close(() => done())),
    }))
  })
}
