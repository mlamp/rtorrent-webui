import { defineConfig, devices } from '@playwright/test'

// Drives the real built Go binary (which embeds the SPA) so screenshots reflect
// exactly what ships. Rebuild with `mise run build` before capturing.
//
// Backend: defaults to the built-in -mock source (50 synthetic torrents incl. a
// failing-tracker fixture), so the read-only specs run with no live daemon and
// /healthz goes green. Set E2E_LIVE=1 to drive a real rtorrent at :5000 — needed
// for the mutating action specs (add/start/stop), which a stateless mock can't
// satisfy. Perf specs (perf/scrollperf/insight) target their own dedicated
// servers via PERF_URL/INSIGHT_URL and ignore this webServer.
const backend = process.env.E2E_LIVE ? '-rtorrent 127.0.0.1:5000' : '-mock 50'

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
    command: `../bin/rtorrent-webui -addr :8099 ${backend}`,
    url: 'http://localhost:8099/healthz',
    reuseExistingServer: true,
    timeout: 30_000,
  },
})
