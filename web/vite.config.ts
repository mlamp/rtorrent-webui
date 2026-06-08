import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

// Single-origin SPA: the Go server serves both the built assets and the API,
// so `base: './'` keeps asset URLs relative regardless of the mount path.
export default defineConfig({
  base: './',
  plugins: [tailwindcss(), svelte()],
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
