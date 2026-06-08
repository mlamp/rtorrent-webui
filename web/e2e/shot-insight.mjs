import { chromium } from '@playwright/test'

const URL = process.env.INSIGHT_URL || 'http://127.0.0.1:8097'
const OUT = process.env.OUT || '/tmp/demo'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } })
await page.emulateMedia({ colorScheme: 'dark' })
await page.goto(URL)
// SSE on the open page drives the poller -> history accumulates
await page.waitForTimeout(Number(process.env.WAIT || 16000))
await page.getByRole('button', { name: 'INSIGHT' }).click()
await page.waitForTimeout(1500)
await page.screenshot({ path: `${OUT}/insight.png` })

// hover the traffic chart to surface the date/time tooltip
const svg = page.locator('svg[aria-label="traffic history"]')
const box = await svg.boundingBox()
if (box) {
  await page.mouse.move(box.x + box.width * 0.62, box.y + box.height * 0.4)
  await page.waitForTimeout(400)
  await page.screenshot({ path: `${OUT}/insight-hover.png` })
  console.log('chart found, captured hover')
} else {
  console.log('NO CHART SVG FOUND')
}
await browser.close()
