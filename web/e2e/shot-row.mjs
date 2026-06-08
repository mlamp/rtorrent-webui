import { chromium } from '@playwright/test'

const URL = process.env.URL || 'http://127.0.0.1:8097'
const OUT = process.env.OUT || '/tmp/ref'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 2 })
await page.emulateMedia({ colorScheme: 'dark' })
await page.goto(URL)
await page.waitForTimeout(1500)

// sidebar
const aside = page.locator('aside').first()
if (await aside.count()) await aside.screenshot({ path: `${OUT}/ours-sidebar.png` })

// filter to downloading so the first row is mid-transfer, then grab a row
const dl = page.getByRole('button', { name: 'DOWNLOADING' })
if (await dl.count()) {
  await dl.click()
  await page.waitForTimeout(800)
}
const row = page.locator('[data-torrent]').first()
if (await row.count()) {
  const box = await row.boundingBox()
  // capture header + the row to mirror the reference strip, full table width
  await page.screenshot({
    path: `${OUT}/ours-row.png`,
    clip: { x: box.x, y: box.y - 2, width: Math.min(1092, box.width), height: 44 },
  })
  console.log('row captured at', JSON.stringify(box))
} else {
  console.log('NO ROW FOUND')
}
await browser.close()
