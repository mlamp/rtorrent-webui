import { chromium } from '@playwright/test'

const URL = process.env.URL || 'http://127.0.0.1:8097'
const OUT = process.env.OUT || '/tmp/ref'

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, deviceScaleFactor: 2 })
await page.emulateMedia({ colorScheme: 'dark' })
await page.goto(URL)
await page.waitForTimeout(1800)

// capture the header + first ~7 rows (the list region)
const list = page.locator('[data-list]').first()
const hdr = page.locator('.grid.shrink-0.border-b').first()
const hb = await hdr.boundingBox().catch(() => null)
const top = hb ? hb.y : (await list.boundingBox()).y
await page.screenshot({ path: `${OUT}/ours-rows.png`, clip: { x: 300, y: top, width: 980, height: 340 } })
console.log('captured rows from y=', top)
await browser.close()
