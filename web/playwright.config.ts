import { defineConfig, devices } from '@playwright/test'

// Drives the real built Go binary (which embeds the SPA) so screenshots reflect
// exactly what ships. Rebuild with `mise run build` before capturing.
export default defineConfig({
  testDir: './e2e',
  outputDir: './e2e/.results',
  timeout: 30_000,
  use: {
    baseURL: 'http://localhost:8099',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
  ],
  webServer: {
    command: '../bin/rtorrent-webui -addr :8099',
    url: 'http://localhost:8099/healthz',
    reuseExistingServer: true,
    timeout: 30_000,
  },
})
