import { test, expect } from '@playwright/test'

test('detail panel tabs render', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('footer')).toContainText('torrents')

  // filter to a file-bearing torrent, then open it
  await page.getByPlaceholder('Filter torrents…').fill('ubuntu')
  await page.waitForTimeout(300)
  await page.locator('[data-torrent] button').first().click()

  await expect(page.getByRole('button', { name: 'General' })).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/detail-general.png' })

  await page.getByRole('button', { name: 'Files' }).click()
  await page.waitForTimeout(800)
  await page.screenshot({ path: 'e2e/screenshots/detail-files.png' })

  await page.getByRole('button', { name: 'Speed' }).click()
  await page.waitForTimeout(1500)
  await page.screenshot({ path: 'e2e/screenshots/detail-speed.png' })
})
