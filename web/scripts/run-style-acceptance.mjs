/**
 * Style acceptance launcher.
 *
 * Starts a recording passthrough in front of the live gateway, writes an
 * isolated data root whose model endpoint points at that passthrough, builds the
 * backend against the freshly built frontend, and runs the driver against it.
 *
 * Nothing here touches the user's own data root, credentials or running
 * services: the data root is under web/test-results, the port is its own, and the
 * proxy only forwards. The process this script starts is the process it stops.
 */
import { spawn, spawnSync } from 'node:child_process'
import { existsSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { startRecordingProxy } from './recording-model-proxy.mjs'

const repositoryRoot = path.resolve(process.cwd(), '..')
const runtimeRoot = path.resolve(process.cwd(), 'test-results', 'style')
const wireDir = path.join(runtimeRoot, 'wire')
const taDir = path.join(runtimeRoot, 'ta')
const binaryPath = path.join(runtimeRoot, 'ta-style.exe')

const backendPort = process.env.TA_STYLE_BACKEND_PORT || '19740'
const proxyPort = process.env.TA_STYLE_PROXY_PORT || '19741'
const upstream = process.env.TA_STYLE_UPSTREAM || 'http://127.0.0.1:8317'
const model = process.env.TA_STYLE_MODEL || 'deepseek-v4.1-flash'
// Matches config.DefaultContextWindowTokens. An earlier revision hardcoded a
// much smaller window here, which made the driver hit the agent's compaction
// ceiling partway through the twelve styles and read that as a style failure.
// The declared window must not be tighter than the product's own default, or
// the acceptance measures the harness instead of the product.
const contextWindowTokens = process.env.TA_STYLE_CONTEXT_WINDOW || '400000'

rmSync(runtimeRoot, { recursive: true, force: true })
mkdirSync(taDir, { recursive: true })
mkdirSync(wireDir, { recursive: true })

const requestLog = path.join(wireDir, 'requests.jsonl')
const responseLog = path.join(wireDir, 'responses.jsonl')

// The backend is built with the embedded frontend so the browser sees the
// current panel. internal/webfs/dist is populated from web/dist by the caller.
const build = spawnSync('go', ['build', '-tags', 'embedweb', '-o', binaryPath, './cmd/ta'], {
  cwd: repositoryRoot,
  env: process.env,
  stdio: 'inherit',
})
if (build.error) throw build.error
if (build.status !== 0) process.exit(build.status ?? 1)

writeFileSync(path.join(taDir, 'config.toml'), `language = "zh-CN"
update_check_enabled = false
model_max_retries = 1

[[model_endpoints]]
id = "accept"
name = "Style acceptance recording passthrough"
provider = "openai-compatible"
protocol = "openai-chat-completions"
api_key = ""
base_url = "http://127.0.0.1:${proxyPort}/v1"

[[model_profiles]]
id = "accept"
name = "acceptance model"
endpoint_id = "accept"
model = "${model}"
context_window_tokens = ${contextWindowTokens}

[agent_models.default]
profile_id = "accept"

# The main narrative is the purpose that is supposed to use thinking, so the
# acceptance exercises the real default rather than a switched-off one.
[agent_models.interactive_story]
profile_id = "accept"
thinking_level = "high"
`, 'utf8')

const proxy = await startRecordingProxy({
  port: proxyPort, upstream, requestLog, responseLog,
  onConnect: (info) => console.log('[proxy] connect', info.id, info.method, info.path, info.unparsedBytes ?? ''),
  onRecord: (event) => console.log('[proxy] record', event.kind, event.record.id,
    event.record.status ?? event.record.path,
    event.kind === 'request' ? `sections=${(event.record.styleSections || []).map((s) => s.purpose).join('+') || 'none'}` : ''),
})

const backend = spawn(binaryPath, ['--no-open', `--port=${backendPort}`], {
  cwd: runtimeRoot,
  env: {
    ...process.env,
    TA_DIR: taDir,
    TA_SKILLS_DIR: path.join(repositoryRoot, 'skills'),
  },
  stdio: 'inherit',
})

let driverExit = 1
async function shutdown() {
  if (!backend.killed) backend.kill()
  await proxy.close().catch(() => {})
}
process.once('SIGINT', () => { void shutdown().then(() => process.exit(130)) })
process.once('SIGTERM', () => { void shutdown().then(() => process.exit(143)) })

async function waitForBackend(timeoutMs = 90000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`http://127.0.0.1:${backendPort}/api/interactive/tellers`)
      if (res.ok) return true
    } catch { /* not up yet */ }
    await new Promise((r) => setTimeout(r, 1500))
  }
  return false
}

try {
  if (!existsSync(path.join(repositoryRoot, 'internal', 'webfs', 'dist', 'index.html'))) {
    throw new Error('internal/webfs/dist is missing; run the frontend build and copy web/dist into it first')
  }
  if (!(await waitForBackend())) throw new Error('backend did not become ready')

  // The driver runs as a child process rather than through spawnSync: this
  // process also owns the recording proxy, and a synchronous spawn would block
  // the event loop the proxy needs to forward the very requests the driver is
  // waiting on. The driver would then sit until the model call timed out.
  driverExit = await new Promise((resolve) => {
    const driver = spawn(process.execPath, [path.join('scripts', 'style-acceptance.mjs')], {
      cwd: process.cwd(),
      env: {
        ...process.env,
        TA_ACCEPT_BASE: `http://127.0.0.1:${backendPort}`,
        TA_ACCEPT_WIRE_DIR: wireDir,
        TA_ACCEPT_RESULT: path.join(runtimeRoot, 'style-acceptance-result.json'),
      },
      stdio: 'inherit',
    })
    driver.once('exit', (code) => resolve(code ?? 1))
  })
} finally {
  await shutdown()
}
process.exit(driverExit)
