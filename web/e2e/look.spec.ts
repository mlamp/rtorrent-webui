import { test } from '@playwright/test'

// Redesign visual loop. LOOK_URL points at a running webui (mock or real).
const URL = process.env.LOOK_URL || 'http://127.0.0.1:8099'

test('look', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(URL)
  await page.waitForTimeout(1800)
  await page.screenshot({ path: 'e2e/screenshots/look.png' })

  await page.locator('[data-torrent]').first().click()
  await page.waitForTimeout(900)
  await page.screenshot({ path: 'e2e/screenshots/look-detail.png' })
  await page.locator('[data-torrent]').first().click() // close

  await page.locator('button:has-text("INSIGHT")').click()
  await page.waitForTimeout(1200)
  await page.screenshot({ path: 'e2e/screenshots/look-insight.png' })
  await page.locator('button:has-text("LIST")').click()

  await page.locator('button:has-text("ADD")').click()
  await page.waitForTimeout(500)
  await page.screenshot({ path: 'e2e/screenshots/look-add.png' })
})
