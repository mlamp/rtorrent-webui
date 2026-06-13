import { test, expect, type Page } from '@playwright/test'

// Integration seam for the SSE visibility lifecycle: the driver's decision
// logic is unit-tested in test/sse-driver.test.ts; these specs cover what plain
// node cannot see — the DOM event mapping (lifecycle store), the footer
// classes, and pollWhileVisible under the real Svelte runtime. Visibility is
// stubbed (Playwright pages are always 'visible' natively) and timers run on
// page.clock, so nothing here sleeps real seconds.

// Wrap EventSource in a counting proxy and make document.visibilityState
// scriptable. Must run before the app boots (addInitScript).
const INIT = `
  const Orig = window.EventSource
  window.__es = []
  window.EventSource = class extends Orig {
    constructor(url, init) {
      super(url, init)
      window.__es.push(this)
    }
  }
  let vis = 'visible'
  Object.defineProperty(document, 'visibilityState', { get: () => vis, configurable: true })
  Object.defineProperty(document, 'hidden', { get: () => vis === 'hidden', configurable: true })
  window.__setVis = (v) => {
    vis = v
    document.dispatchEvent(new Event('visibilitychange'))
  }
`

declare global {
  interface Window {
    __es: EventSource[]
    __setVis: (v: 'visible' | 'hidden') => void
  }
}

const esCount = (page: Page) => page.evaluate(() => window.__es.length)
const lastEsState = (page: Page) => page.evaluate(() => window.__es[window.__es.length - 1]?.readyState)
const setVis = (page: Page, v: 'visible' | 'hidden') => page.evaluate((s) => window.__setVis(s), v)

async function boot(page: Page) {
  await page.clock.install()
  await page.addInitScript(INIT)
  await page.goto('/')
  await expect(page.locator('footer')).toContainText('live') // SSE snapshot landed
}

const dot = (page: Page) => page.locator('footer .rounded-full').first()

test('hidden 60s closes the stream and shows a neutral idle state (E1)', async ({ page }) => {
  await boot(page)
  await setVis(page, 'hidden')
  await page.clock.runFor(60_500) // grace expires on the fake clock
  expect(await lastEsState(page)).toBe(2) // CLOSED
  await expect(page.locator('footer')).toContainText('idle')
  await expect(dot(page)).not.toHaveClass(/bg-status-error/) // idle is not an error state
})

test('show reconnects once and returns to live (E2)', async ({ page }) => {
  await boot(page)
  const before = await esCount(page)
  await setVis(page, 'hidden')
  await page.clock.runFor(60_500)
  await setVis(page, 'visible')
  expect(await esCount(page)).toBe(before + 1) // exactly one new connection
  await expect(page.locator('footer')).toContainText('live') // snapshot resync
})

test('insight polling pauses while hidden and refreshes on return (E3)', async ({ page }) => {
  await boot(page)
  let history = 0
  page.on('request', (r) => {
    if (r.url().includes('/api/history')) history++
  })
  await page.getByRole('button', { name: 'INSIGHT' }).click()
  await expect.poll(() => history).toBeGreaterThanOrEqual(1) // immediate load on mount
  const base = history
  await page.clock.runFor(9_000)
  await expect.poll(() => history).toBe(base + 3) // 3s cadence while visible
  await setVis(page, 'hidden')
  await page.clock.runFor(9_000)
  expect(history).toBe(base + 3) // zero polls while hidden
  await setVis(page, 'visible')
  await expect.poll(() => history).toBe(base + 4) // instant refresh on return
})

test('range click fires exactly one fetch — untrack guard (E4)', async ({ page }) => {
  await boot(page)
  await page.getByRole('button', { name: 'INSIGHT' }).click()
  let history = 0
  page.on('request', (r) => {
    if (r.url().includes('/api/history')) history++
  })
  await page.getByRole('button', { name: '6h', exact: true }).click()
  await page.waitForTimeout(200) // real wall-clock beat; fake timers stay frozen
  // setRange calls loadHistory once. Without untrack(fn) in pollWhileVisible the
  // poll effect would track `range`, re-run on the click, and fetch a second time.
  expect(history).toBe(1)
})

test('freeze closes the stream synchronously (E5)', async ({ page }) => {
  await boot(page)
  const state = await page.evaluate(() => {
    document.dispatchEvent(new Event('freeze'))
    return window.__es[window.__es.length - 1].readyState // same task, right after
  })
  expect(state).toBe(2) // CLOSED before the handler returned
})
