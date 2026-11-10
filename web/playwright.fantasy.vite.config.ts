import { defineConfig, mergeConfig } from 'vite'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import base from './vite.config'

// Test-only binary route. It serves the very same imported PNGs; this is not
// the production HTTP handler and does not expose a manifest or source prose.
export default mergeConfig(base, defineConfig({
  plugins: [{ name: 'fantasy-runtime-art-fixture', configureServer(server) {
    const root = resolve(process.cwd(), '../internal/catalogart/assets')
    const manifest = JSON.parse(readFileSync(resolve(root, 'manifest.json'), 'utf8'))
    const hashes = new Set<string>(manifest.entries.filter((entry: { world: string }) => entry.world === 'sui-guan-realms').flatMap((entry: { variants: Record<string, { sha256: string }> }) => Object.values(entry.variants).map(variant => variant.sha256)))
    server.middlewares.use('/api/catalog-art', (req, res) => {
      const match = /^\/([a-f0-9]{64})\.png$/.exec(req.url || '')
      if (!match || !hashes.has(match[1])) { res.statusCode = 404; res.end(); return }
      res.setHeader('Content-Type', 'image/png')
      res.end(readFileSync(resolve(root, 'images', match[1] + '.png')))
    })
  } }],
}))
