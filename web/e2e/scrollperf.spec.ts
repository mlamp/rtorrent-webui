import { test } from '@playwright/test'

// Measures frame pacing while programmatically scrolling the 1000-row list.
const URL = process.env.PERF_URL || 'http://127.0.0.1:8098'

test('scroll performance', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(URL)
  await page.waitForSelector('[data-torrent]')
  await page.waitForTimeout(1000)

  const r = await page.evaluate(async () => {
    const list = document.querySelector('[data-list]') as HTMLElement
    if (!list) return { error: 'no list' }
    let longTasks = 0
    const obs = new PerformanceObserver((l) => {
      for (const e of l.getEntries()) if (e.duration > 50) longTasks++
    })
    obs.observe({ entryTypes: ['longtask'] })
    const frames: number[] = []
    let last = performance.now()
    for (let i = 0; i < 80; i++) {
      list.scrollTop += 240
      await new Promise((res) =>
        requestAnimationFrame(() => {
          const n = performance.now()
          frames.push(n - last)
          last = n
          res(null)
        }),
      )
    }
    obs.disconnect()
    frames.sort((a, b) => a - b)
    const p = (q: number) => frames[Math.floor(frames.length * q)]
    const janky = frames.filter((f) => f > 24).length // > ~1.5 frame budget
    return {
      longTasks,
      median: Math.round(p(0.5)),
      p95: Math.round(p(0.95)),
      max: Math.round(frames[frames.length - 1]),
      jankyFrames: janky,
      total: frames.length,
    }
  })
  console.log('SCROLLPERF', JSON.stringify(r))
})
