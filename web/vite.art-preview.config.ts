// Lightweight isolated presentation fixture. Never used by production builds.
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, 'src') }, dedupe: ['react', 'react-dom'] },
  cacheDir: 'node_modules/.vite-art-preview',
  optimizeDeps: { noDiscovery: true, include: ['react', 'react-dom/client', 'react-i18next', 'react/jsx-runtime'] },
  server: { host: '127.0.0.1', port: 15191, strictPort: true, proxy: { '/api': 'http://127.0.0.1:8080' } },
})
