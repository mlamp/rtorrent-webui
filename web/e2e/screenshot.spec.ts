import { test, expect } from '@playwright/test'

const shot = (name: string) => `e2e/screenshots/${name}.png`

test('themed shell renders in light, dark, and mobile', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'light' })
  await page.goto('/') // SSE keeps the connection open, so don't wait for networkidle
  await expect(page.locator('header')).toContainText('rtorrent-webui')
  // wait for the live snapshot to populate the table
  await expect(page.locator('text=/torrents/').first()).toBeVisible()
  await page.waitForTimeout(800)
  await page.screenshot({ path: shot('desktop-light') })

  // mode-watcher follows prefers-color-scheme when mode = system
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.waitForTimeout(250)
  await expect(page.locator('html')).toHaveClass(/dark/)
  await page.screenshot({ path: shot('desktop-dark') })

  await page.setViewportSize({ width: 414, height: 896 })
  await page.waitForTimeout(200)
  await page.screenshot({ path: shot('mobile-dark') })
})
