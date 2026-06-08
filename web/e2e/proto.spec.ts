import { test } from '@playwright/test'

const URL = 'http://127.0.0.1:8077/Torui%20-%20Relay.html'

test('capture relay prototype reference', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto(URL)
  // Babel compiles the JSX in-browser — wait for the app to render.
  await page.waitForSelector('.brand', { timeout: 20000 })
  await page.waitForTimeout(1500)
  await page.screenshot({ path: 'e2e/screenshots/proto-relay.png' })

  // open a torrent detail (click first row)
  await page.locator('.trow').first().click()
  await page.waitForTimeout(900)
  await page.screenshot({ path: 'e2e/screenshots/proto-relay-detail.png' })

  // let the live engine animate, capture a downloading frame
  await page.locator('.trow').first().click()
  await page.waitForTimeout(2500)
  await page.screenshot({ path: 'e2e/screenshots/proto-relay-live.png' })
})
