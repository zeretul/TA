// Technical export only: retain the approved C artwork and transparency.
// Run with: pnpm --dir web generate-icons
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import sharp from 'sharp'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const brandDir = path.join(root, 'assets/branding')
const publicDir = path.join(root, 'web/public')
const publicBrandDir = path.join(publicDir, 'brand')
const original = path.join(brandDir, 'TA-C-Cosmic.original.png')
const background = { r: 26, g: 26, b: 26, alpha: 1 }
const transparent = { r: 0, g: 0, b: 0, alpha: 0 }

// The supplied 1254px transparent canvas has a 1/255-alpha fringe. Crop only
// the invisible outer margin; retain all of the visible approved artwork.
const artwork = await sharp(original).extract({ left: 91, top: 301, width: 1091, height: 679 }).png().toBuffer()
await mkdir(publicBrandDir, { recursive: true })
await sharp(artwork).resize({ width: 512 }).png().toFile(path.join(publicBrandDir, 'ta-cosmic-mark.png'))
await sharp(artwork).resize({ width: 1024 }).png().toFile(path.join(brandDir, 'ta-cosmic-mark-1024.png'))

async function icon(size, scale = 0.9, opaque = false) {
  const limit = Math.floor(size * scale)
  const mark = await sharp(artwork).resize({ width: limit, height: limit, fit: 'inside' }).png().toBuffer()
  return sharp({ create: { width: size, height: size, channels: 4, background: opaque ? background : transparent } })
    .composite([{ input: mark, gravity: 'centre' }]).png().toBuffer()
}

for (const [size, filename] of [[16, 'favicon-16.png'], [32, 'favicon-32.png'], [192, 'pwa-192.png'], [512, 'pwa-512.png']]) {
  await writeFile(path.join(publicDir, filename), await icon(size))
}
await writeFile(path.join(publicDir, 'apple-touch-icon.png'), await icon(180, 0.88, true))
// A 66%-wide 1.607:1 mark fits fully within the maskable 80%-diameter safe circle.
await writeFile(path.join(publicDir, 'pwa-maskable-512.png'), await icon(512, 0.66, true))

// ICO supports embedded PNG frames on supported Windows releases. Every
// frame is independently generated at its intended physical pixel size.
const sizes = [16, 24, 32, 48, 64, 128, 256]
const images = await Promise.all(sizes.map(size => icon(size)))
const header = Buffer.alloc(6 + 16 * images.length)
header.writeUInt16LE(1, 2)
header.writeUInt16LE(images.length, 4)
let offset = header.length
for (const [i, data] of images.entries()) {
  const at = 6 + i * 16
  header[at] = sizes[i] === 256 ? 0 : sizes[i]
  header[at + 1] = header[at]
  header.writeUInt16LE(1, at + 4)
  header.writeUInt16LE(32, at + 6)
  header.writeUInt32LE(data.length, at + 8)
  header.writeUInt32LE(offset, at + 12)
  offset += data.length
}
const ico = Buffer.concat([header, ...images])
await writeFile(path.join(publicDir, 'favicon.ico'), ico)
await writeFile(path.join(brandDir, 'ta-cosmic.ico'), ico)
// Keep legacy favicon.svg URLs on the same new brand, with no external href.
const image = (await icon(256)).toString('base64')
await writeFile(path.join(publicDir, 'favicon.svg'), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><image width="256" height="256" href="data:image/png;base64,${image}"/></svg>\n`)
await readFile(path.join(publicBrandDir, 'ta-cosmic-mark.png'))
console.log('Generated approved TA Cosmic transparent brand, browser, PWA and Windows ICO assets.')
