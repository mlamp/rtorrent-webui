import { test, expect } from '@playwright/test'

// Points at a mock server (1000 synthetic torrents, ~1/3 changing per second).
const URL = process.env.PERF_URL || 'http://127.0.0.1:8098'

test('1000-row live table stays windowed and responsive', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(URL)
  await expect(page.locator('footer')).toContainText('1000 torrents', { timeout: 10_000 })

  // 1) Virtualization: only a small window of rows is ever in the DOM.
  const rowCount = await page.locator('[data-torrent]').count()
  console.log('DOM rows for 1000 torrents:', rowCount)
  expect(rowCount).toBeLessThan(80)

  // 2) Live updates: the visible rows' content changes across ticks (~1/3 of
  //    rows change each second, so the visible window's text differs).
  const a = await page.locator('main').innerText()
  await page.waitForTimeout(1500)
  const b = await page.locator('main').innerText()
  console.log('table text changed:', a !== b)
  expect(a).not.toEqual(b)

  // 3) Jank proxy: count main-thread long tasks (>50ms) over 3s of live updates.
  const longTasks = await page.evaluate(
    () =>
      new Promise<number>((resolve) => {
        let count = 0
        const obs = new PerformanceObserver((list) => {
          for (const e of list.getEntries()) if (e.duration > 50) count++
        })
        obs.observe({ entryTypes: ['longtask'] })
        setTimeout(() => {
          obs.disconnect()
          resolve(count)
        }, 3000)
      }),
  )
  console.log('long tasks (>50ms) during 3s of live updates:', longTasks)
  expect(longTasks).toBeLessThan(10)

  await page.screenshot({ path: 'e2e/screenshots/perf-1000.png' })
})
