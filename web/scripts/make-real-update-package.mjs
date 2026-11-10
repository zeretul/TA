/**
 * 用真实候选产物打一份 Windows 更新包。
 *
 * 与 `make-update-package.mjs` 的区别：那个脚本塞的是占位文本，只能证明结构校验
 * 通过，不能证明更新后能启动。这里直接从候选目录读取真实可执行文件、前端产物、
 * skills、ripgrep 与许可文件，压缩进包，因此 apply 之后跑起来的是真的 TA。
 *
 * 包内根目录名必须是 `TA`（`internal/update/paths.go:releasePackageRootName`）。
 * 归档名必须匹配 `internal/update/local.go:archiveNamePattern`：
 * `TA-v<major>.<minor>.<patch>-windows-x64.zip`。
 *
 * 用法：
 *   node scripts/make-real-update-package.mjs \
 *     --from ../output \
 *     --version 0.6.7 \
 *     --out ../.ta-dev/update-real/TA-v0.6.7-windows-x64.zip
 *
 * `--exe` 可覆盖主程序文件名（默认 `TA`，与开发候选一致）。
 * `--config-from <路径>` 把一份 config.toml 放进包里；不传则沿用候选目录里的那份。
 */
import { deflateRawSync } from 'node:zlib'
import { writeFileSync, mkdirSync, readFileSync, statSync, readdirSync, existsSync } from 'node:fs'
import { dirname, join, relative, sep } from 'node:path'
import { createHash } from 'node:crypto'

function argOf(name, fallback = '') {
  const i = process.argv.indexOf(`--${name}`)
  return i >= 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback
}

const from = argOf('from', '../output')
const version = argOf('version')
const outPath = argOf('out')
const exeName = argOf('exe', 'TA')
if (!version || !outPath) {
  console.error('用法：node scripts/make-real-update-package.mjs --from <候选目录> --version <版本> --out <zip路径>')
  process.exit(2)
}

// 只打发布包该有的东西。`.denova-updates` 这类运行期目录、日志和用户数据一律不进包。
const INCLUDE = ['web', 'skills', 'tools', 'licenses']
const REQUIRED = [exeName, 'TA-updater.exe', 'web', 'skills', 'tools/rg.exe',
  'licenses/ripgrep/LICENSE-MIT', 'licenses/ripgrep/UNLICENSE']

function walk(dir, base) {
  const out = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name)
    if (entry.isDirectory()) out.push(...walk(full, base))
    else if (entry.isFile()) out.push(relative(base, full).split(sep).join('/'))
  }
  return out
}

const entries = {}
function add(relPath) {
  entries[`TA/${relPath}`] = readFileSync(join(from, relPath))
}

for (const name of [exeName, 'TA-updater.exe']) add(name)
const configOverride = argOf('config-from')
if (configOverride) entries['TA/config.toml'] = readFileSync(configOverride)
else if (existsSync(join(from, 'config.toml'))) add('config.toml')

for (const dir of INCLUDE) {
  if (!existsSync(join(from, dir))) continue
  const base = join(from, dir)
  for (const rel of walk(base, base)) add(`${dir}/${rel}`)
}

for (const required of REQUIRED) {
  // Directories are implied by the files inside them, so `web` and `skills`
  // only need at least one entry rather than an explicit directory record.
  const present = entries[`TA/${required}`] ||
    Object.keys(entries).some((name) => name.startsWith(`TA/${required}/`))
  if (!present) {
    console.error(`候选目录缺少必需条目：${required}`)
    process.exit(1)
  }
}
// `validateReleasePackage` 用 hostruntime 在包内找 ripgrep，包内必须是 `tools/rg.exe`。
if (statSync(join(from, 'tools', 'rg.exe')).size === 0) {
  console.error('候选目录里的 tools/rg.exe 是空文件')
  process.exit(1)
}

// ---------------------------------------------------------------
// 最小 ZIP writer（deflate，UTF-8 文件名，DOS 时间戳固定）
// ---------------------------------------------------------------
const CRC_TABLE = (() => {
  const table = new Int32Array(256)
  for (let i = 0; i < 256; i += 1) {
    let c = i
    for (let k = 0; k < 8; k += 1) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    table[i] = c
  }
  return table
})()

function crc32(buf) {
  let c = -1
  for (let i = 0; i < buf.length; i += 1) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ -1) >>> 0
}

const DOS_TIME = 0
const DOS_DATE = (1 << 5) | 1

const localParts = []
const centralParts = []
let offset = 0
const names = Object.keys(entries).sort()

for (const name of names) {
  const content = entries[name]
  const nameBuf = Buffer.from(name, 'utf8')
  const compressed = deflateRawSync(content, { level: 9 })
  const crc = crc32(content)

  const local = Buffer.alloc(30)
  local.writeUInt32LE(0x04034b50, 0)
  local.writeUInt16LE(20, 4)
  local.writeUInt16LE(0x0800, 6)
  local.writeUInt16LE(8, 8)
  local.writeUInt16LE(DOS_TIME, 10)
  local.writeUInt16LE(DOS_DATE, 12)
  local.writeUInt32LE(crc, 14)
  local.writeUInt32LE(compressed.length, 18)
  local.writeUInt32LE(content.length, 22)
  local.writeUInt16LE(nameBuf.length, 26)
  local.writeUInt16LE(0, 28)
  localParts.push(local, nameBuf, compressed)

  const central = Buffer.alloc(46)
  central.writeUInt32LE(0x02014b50, 0)
  central.writeUInt16LE(20, 4)
  central.writeUInt16LE(20, 6)
  central.writeUInt16LE(0x0800, 8)
  central.writeUInt16LE(8, 10)
  central.writeUInt16LE(DOS_TIME, 12)
  central.writeUInt16LE(DOS_DATE, 14)
  central.writeUInt32LE(crc, 16)
  central.writeUInt32LE(compressed.length, 20)
  central.writeUInt32LE(content.length, 24)
  central.writeUInt16LE(nameBuf.length, 28)
  central.writeUInt16LE(0, 30)
  central.writeUInt16LE(0, 32)
  central.writeUInt16LE(0, 34)
  central.writeUInt16LE(0, 36)
  central.writeUInt32LE((0o100644 << 16) >>> 0, 38)
  central.writeUInt32LE(offset, 42)
  centralParts.push(central, nameBuf)

  offset += local.length + nameBuf.length + compressed.length
}

const centralDir = Buffer.concat(centralParts)
const end = Buffer.alloc(22)
end.writeUInt32LE(0x06054b50, 0)
end.writeUInt16LE(0, 4)
end.writeUInt16LE(0, 6)
end.writeUInt16LE(names.length, 8)
end.writeUInt16LE(names.length, 10)
end.writeUInt32LE(centralDir.length, 12)
end.writeUInt32LE(offset, 16)
end.writeUInt16LE(0, 20)

const archive = Buffer.concat([...localParts, centralDir, end])
mkdirSync(dirname(outPath), { recursive: true })
writeFileSync(outPath, archive)

const digest = createHash('sha256').update(archive).digest('hex')
const archiveName = `TA-v${version}-windows-x64.zip`
writeFileSync(`${outPath}.checksums`, `${digest}  ${archiveName}\n`)
console.log(`已生成 ${outPath}`)
console.log(`版本 ${version}，${names.length} 个文件，${archive.length} 字节`)
console.log(`sha256 ${digest}`)
