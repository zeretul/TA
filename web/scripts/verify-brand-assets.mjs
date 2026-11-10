// Validate exported brand assets and optional icons extracted from built EXEs.
// Usage: node web/scripts/verify-brand-assets.mjs [extracted-main.ico ...]
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import sharp from 'sharp'
const root = path.resolve(import.meta.dirname, '../..')
const original = await readFile(path.join(root, 'assets/branding/TA-C-Cosmic.original.png'))
assert.equal(createHash('sha256').update(original).digest('hex'), '8284b6907eb07e65176571c717d5e9f0565f22ff1c8740fb33eb93dd5e49792c')
for (const [file, size, opaque] of [
  ['favicon-16.png', 16, false], ['favicon-32.png', 32, false],
  ['pwa-192.png', 192, false], ['pwa-512.png', 512, false],
  ['apple-touch-icon.png', 180, true], ['pwa-maskable-512.png', 512, true],
]) {
  const img = sharp(path.join(root, 'web/public', file))
  const meta = await img.metadata()
  assert.equal(meta.width, size, file)
  assert.equal(meta.height, size, file)
  const stats = await img.ensureAlpha().stats()
  assert.equal(stats.channels[3].max, 255, file)
  assert.equal(stats.channels[3].min, opaque ? 255 : 0, file)
}
const mask = await sharp(path.join(root, 'web/public/pwa-maskable-512.png')).ensureAlpha().raw().toBuffer()
for (let y = 0; y < 512; y++) for (let x = 0; x < 512; x++) {
  if (Math.hypot(x + 0.5 - 256, y + 0.5 - 256) <= 204.8) continue
  const p = (y * 512 + x) * 4
  assert.deepEqual([...mask.subarray(p, p + 4)], [26, 26, 26, 255], 'Maskable artwork must fit in the safe circle')
}
function frames(data) {
  assert.equal(data.readUInt16LE(0), 0)
  assert.equal(data.readUInt16LE(2), 1)
  assert.equal(data.readUInt16LE(4), 7)
  return Array.from({ length: 7 }, (_, i) => {
    const p = 6 + i * 16, size = data[p] || 256, length = data.readUInt32LE(p + 8), offset = data.readUInt32LE(p + 12)
    assert.equal(data[p + 1] || 256, size)
    assert.ok(offset + length <= data.length)
    return { size, data: data.subarray(offset, offset + length) }
  })
}
const expectedICO = await readFile(path.join(root, 'assets/branding/ta-cosmic.ico'))
assert.deepEqual(await readFile(path.join(root, 'web/public/favicon.ico')), expectedICO)
const expected = frames(expectedICO)
assert.deepEqual(expected.map(f => f.size), [16, 24, 32, 48, 64, 128, 256])
for (const frame of expected) {
  const meta = await sharp(frame.data).metadata()
  assert.equal(meta.width, frame.size)
  assert.equal(meta.height, frame.size)
}
for (const file of process.argv.slice(2)) {
  const actual = frames(await readFile(file))
  for (const frame of expected) {
    const candidate = actual.find(f => f.size === frame.size)
    assert.ok(candidate, `Missing ${frame.size}px frame in ${file}`)
    assert.deepEqual(await sharp(candidate.data).ensureAlpha().raw().toBuffer(), await sharp(frame.data).ensureAlpha().raw().toBuffer(), `${file}: ${frame.size}px pixels differ`)
  }
}
console.log(`PASS: unchanged original; transparent favicon / PWA; opaque Apple / safe-circle maskable; 7 ICO sizes; ${process.argv.length - 2} extracted EXE icon(s) match pixel-for-pixel.`)
