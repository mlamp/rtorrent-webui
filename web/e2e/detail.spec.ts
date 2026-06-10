import { test, expect } from '@playwright/test'

test('detail panel tabs render', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('footer')).toContainText('torrents')

  // filter to a file-bearing torrent, then open it (whole row is clickable)
  await page.locator('.searchbar input').fill('ubuntu')
  await page.waitForTimeout(300)
  await page.locator('[data-torrent]').first().click()

  await expect(page.getByRole('button', { name: 'PIECES' })).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/detail-general.png' })

  await page.getByRole('button', { name: 'FILES' }).click()
  await page.waitForTimeout(800)
  await page.screenshot({ path: 'e2e/screenshots/detail-files.png' })

  await page.getByRole('button', { name: 'PEERS' }).click()
  await page.waitForTimeout(600)
  await page.screenshot({ path: 'e2e/screenshots/detail-peers.png' })

  // TRACKERS renders per-tracker health rows (or the empty state) — never a
  // stuck spinner. Against the mock backend this includes the red "failing"
  // row for torrents with a dead backup tracker.
  await page.getByRole('button', { name: 'TRACKERS' }).click()
  await page.waitForTimeout(600)
  await expect(page.locator('.rd-trkrow').first().or(page.getByText('no trackers'))).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/detail-trackers.png' })
})
