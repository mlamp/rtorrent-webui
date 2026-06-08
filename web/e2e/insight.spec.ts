import { test, expect } from '@playwright/test'

const URL = process.env.INSIGHT_URL || 'http://127.0.0.1:8097'

test('insight view shows traffic + disk', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto(URL)
  await expect(page.locator('footer')).toContainText('torrents')
  // the page's SSE drives the poller -> history accumulates
  await page.waitForTimeout(18000)
  await page.getByRole('button', { name: 'INSIGHT' }).click()
  await page.waitForTimeout(1500)
  await expect(page.getByText('traffic').first()).toBeVisible()
  await expect(page.getByText('disk').first()).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/insight.png' })
})
