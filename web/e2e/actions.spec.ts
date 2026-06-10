import { test, expect } from '@playwright/test'

// The Add flow opens a dialog (frontend-only) and then POSTs to rtorrent. The
// dialog + selection UI works against either backend; the actual add MUTATES
// rtorrent state, so the "count grows" assertion only holds against a live
// rtorrent (E2E_LIVE=1) — the default -mock backend is stateless.
const LIVE = !!process.env.E2E_LIVE

test('add dialog opens and selection works', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('header')).toContainText('grep')
  await expect(page.locator('footer')).toContainText('torrents')

  const before = parseInt((await page.locator('footer').innerText()).match(/(\d+) torrents/)?.[1] || '0', 10)

  // Open the Add dialog (title is the Modal's "$rtorrent.add", not the old
  // "Add torrent" heading the Relay redesign removed) and enter a magnet.
  await page.getByRole('button', { name: 'Add' }).first().click()
  await expect(page.getByText('rtorrent.add')).toBeVisible()
  const ih = Date.now().toString(16).padStart(40, '0').slice(-40)
  await page.locator('textarea').fill(`magnet:?xt=urn:btih:${ih}&dn=e2e-add-test`)
  await page.screenshot({ path: 'e2e/screenshots/add-dialog.png' })

  if (LIVE) {
    // Submit and confirm the live count grows within a couple of ticks.
    await page.getByRole('button', { name: 'Add', exact: true }).last().click()
    await expect(page.getByText('rtorrent.add')).toBeHidden()
    await expect(page.locator('footer')).toContainText(`${before + 1} torrents`, { timeout: 5000 })
  } else {
    // Don't mutate the mock; just dismiss the dialog.
    await page.getByRole('button', { name: 'Close' }).click()
    await expect(page.getByText('rtorrent.add')).toBeHidden()
  }

  // Select one row via its custom select cell (the Relay redesign replaced the
  // native checkbox with a .selcell ✓ toggle; scope to a [data-torrent] row so we
  // don't hit the header select-all) -> the .bulkbar action bar appears below the
  // header.
  await page.locator('[data-torrent] .selcell').first().click()
  await expect(page.locator('.bulk-count')).toContainText('1 selected')
  await page.screenshot({ path: 'e2e/screenshots/selection.png' })
})
