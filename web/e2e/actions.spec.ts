import { test, expect } from '@playwright/test'

test('add magnet via dialog and select rows', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('header')).toContainText('grep')
  await expect(page.locator('footer')).toContainText('torrents')

  const before = parseInt((await page.locator('footer').innerText()).match(/(\d+) torrents/)?.[1] || '0', 10)

  // Open the Add dialog and add a magnet with a unique infohash.
  await page.getByRole('button', { name: 'Add' }).first().click()
  await expect(page.getByText('Add torrent')).toBeVisible()
  const ih = Date.now().toString(16).padStart(40, '0').slice(-40)
  await page.locator('textarea').fill(`magnet:?xt=urn:btih:${ih}&dn=e2e-add-test`)
  await page.screenshot({ path: 'e2e/screenshots/add-dialog.png' })
  await page.getByRole('button', { name: 'Add', exact: true }).last().click()

  // Dialog closes and the live count grows within a couple of ticks.
  await expect(page.getByText('Add torrent')).toBeHidden()
  await expect(page.locator('footer')).toContainText(`${before + 1} torrents`, { timeout: 5000 })

  // Select all visible rows -> the action bar appears.
  await page.locator('input[type=checkbox]').first().check()
  await expect(page.locator('header')).toContainText('sel')
  await page.screenshot({ path: 'e2e/screenshots/selection.png' })
})
