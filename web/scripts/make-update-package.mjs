/**
 * 为 T33 更新验收造一份结构合法的 Windows 更新包。
 *
 * 包内结构必须与 `internal/update/install.go:validateReleasePackage` 的要求一致：
 * 根目录叫 `TA`，里面有 TA.exe、TA-updater.exe、web/、skills/、tools/rg.exe
 * 以及 licenses/ripgrep 的两份许可。缺任何一项都会在 stageUpdate 阶段被拒。
 *
 * 这台机器没有 zip 命令，web/node_modules 里也没有打包库，所以这里用 zlib 手写
 * 一个最小 ZIP writer。好处是条目顺序、路径分隔符、压缩方式都完全可控。
 *
 * 用法：node scripts/make-update-package.mjs <版本> <输出zip路径> [解压后大文件字节数]
 *
 * 第三个参数用来把「持有更新锁」的窗口拉长：`web/assets/big.bin` 填的是高压缩率的
 * 重复数据，压缩包只有几 KB，但解压和计算摘要都要真读写那么多字节。忙时拒绝的
 * 并发测试需要这个窗口，否则两个请求会一前一后跑完，测不到 409。
 *
 * 可执行文件名必须和正在运行的候选一致：`stageUpdate` 校验的是
 * `filepath.Base(s.executablePath)`。Windows 候选产出的主程序就叫 `TA`（没有
 * `.exe`），所以这里默认用 `TA`，可用 TA_PKG_EXE 覆盖。
 */
import { deflateRawSync } from 'node:zlib'
import { writeFileSync, mkdirSync } from 'node:fs'
import { dirname } from 'node:path'

const version = process.argv[2]
const outPath = process.argv[3]
const bigBytes = Number(process.argv[4] || 0)
const exeName = process.env.TA_PKG_EXE || 'TA'
if (!version || !outPath) {
  console.error('用法：node scripts/make-update-package.mjs <版本> <输出zip路径> [解压后大文件字节数]')
  process.exit(2)
}

const FILES = {
  [`TA/${exeName}`]: Buffer.from('TA candidate executable placeholder for update acceptance\n'),
  'TA/TA-updater.exe': Buffer.from('TA-updater placeholder for update acceptance\n'),
  'TA/web/index.html': Buffer.from('<!doctype html><html><body>staged release</body></html>\n'),
  'TA/skills/example/SKILL.md': Buffer.from('# example skill\n'),
  'TA/tools/rg.exe': Buffer.from('ripgrep placeholder\n'),
  'TA/licenses/ripgrep/LICENSE-MIT': Buffer.from('MIT License\n\nPermission is hereby granted, free of charge...\n'),
  'TA/licenses/ripgrep/UNLICENSE': Buffer.from('This is free and unencumbered software released into the public domain.\n'),
}

if (bigBytes > 0) {
  FILES['TA/web/assets/big.bin'] = Buffer.alloc(bigBytes, 0x41)
}

// ---------------------------------------------------------------
// 最小 ZIP writer（store + deflate，UTF-8 文件名，DOS 时间戳固定）
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

const DOS_TIME = 0 // 00:00:00
const DOS_DATE = (1 << 5) | 1 // 1980-01-01

const localParts = []
const centralParts = []
let offset = 0

for (const [name, content] of Object.entries(FILES)) {
  const nameBuf = Buffer.from(name, 'utf8')
  const compressed = deflateRawSync(content, { level: 9 })
  const crc = crc32(content)

  const local = Buffer.alloc(30)
  local.writeUInt32LE(0x04034b50, 0)
  local.writeUInt16LE(20, 4) // version needed
  local.writeUInt16LE(0x0800, 6) // UTF-8 名称
  local.writeUInt16LE(8, 8) // deflate
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
  central.writeUInt16LE(20, 4) // version made by
  central.writeUInt16LE(20, 6) // version needed
  central.writeUInt16LE(0x0800, 8)
  central.writeUInt16LE(8, 10)
  central.writeUInt16LE(DOS_TIME, 12)
  central.writeUInt16LE(DOS_DATE, 14)
  central.writeUInt32LE(crc, 16)
  central.writeUInt32LE(compressed.length, 20)
  central.writeUInt32LE(content.length, 24)
  central.writeUInt16LE(nameBuf.length, 28)
  central.writeUInt16LE(0, 30) // extra
  central.writeUInt16LE(0, 32) // comment
  central.writeUInt16LE(0, 34) // disk
  central.writeUInt16LE(0, 36) // internal attrs
  central.writeUInt32LE((0o100644 << 16) >>> 0, 38) // external attrs：普通文件 rw-r--r--
  central.writeUInt32LE(offset, 42)
  centralParts.push(central, nameBuf)

  offset += local.length + nameBuf.length + compressed.length
}

const centralDir = Buffer.concat(centralParts)
const end = Buffer.alloc(22)
end.writeUInt32LE(0x06054b50, 0)
end.writeUInt16LE(0, 4)
end.writeUInt16LE(0, 6)
end.writeUInt16LE(Object.keys(FILES).length, 8)
end.writeUInt16LE(Object.keys(FILES).length, 10)
end.writeUInt32LE(centralDir.length, 12)
end.writeUInt32LE(offset, 16)
end.writeUInt16LE(0, 20)

const archive = Buffer.concat([...localParts, centralDir, end])
mkdirSync(dirname(outPath), { recursive: true })
writeFileSync(outPath, archive)
console.log(`已生成 ${outPath}（版本 ${version}，${Object.keys(FILES).length} 个文件，${archive.length} 字节）`)
