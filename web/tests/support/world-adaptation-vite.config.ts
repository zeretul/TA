import path from 'node:path'
import { mergeConfig } from 'vite'
import base from '../../vite.config'

// Only the isolated acceptance server may serve the checked-out dependency junction.
// Product build and normal development configuration remain unchanged.
export default mergeConfig(base, {
  server: { fs: { allow: [path.resolve(import.meta.dirname, '../../..')] } },
})
