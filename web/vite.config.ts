import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

// Drop the legacy WOFF font fallback from the build. Every browser we target
// supports WOFF2 (which @fontsource lists first in every @font-face), so the
// WOFF copies are dead weight inside the embedded Go binary. We strip the
// `, url(...) format('woff')` clause from each @font-face in a `pre` transform,
// i.e. before Vite resolves url()s — so the WOFF files are never emitted, never
// inlined as base64, and the CSS content hash stays correct.
function dropWoffFallback(): Plugin {
  const WOFF_FALLBACK = /,\s*url\([^)]+\)\s*format\(\s*["']?woff["']?\s*\)/g
  return {
    name: 'drop-woff-fallback',
    enforce: 'pre',
    transform(code, id) {
      if (!id.includes('.css') || !code.includes('woff')) return null
      return { code: code.replace(WOFF_FALLBACK, ''), map: null }
    },
  }
}

// Single-origin SPA: the Go server serves both the built assets and the API,
// so `base: './'` keeps asset URLs relative regardless of the mount path.
export default defineConfig({
  base: './',
  plugins: [tailwindcss(), svelte(), dropWoffFallback()],
  resolve: {
    alias: {
      $lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
  },
  server: {
    // Dev proxy to the Go backend (:8080) so dev mirrors prod single-origin.
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      '/events': { target: 'http://localhost:8080', changeOrigin: true },
      '/rpc': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
})
