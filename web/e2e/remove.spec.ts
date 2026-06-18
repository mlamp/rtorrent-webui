import { test, expect, type Page } from '@playwright/test'

// The remove confirm dialog is frontend-only for the paths that matter here
// (open/cancel/focus/keyboard/capability copy), so these run against the default
// -mock backend. Where a server round-trip is needed (capability ON, busy
// state, truthful toast) we intercept the request rather than mutate the mock —
// the real on-disk delete stays an E2E_LIVE concern.

async function selectFirstRow(page: Page) {
  await page.goto('/')
  await expect(page.locator('footer')).toContainText('torrents')
  await page.locator('[data-torrent] .selcell').first().click()
  await expect(page.locator('.bulk-count')).toContainText('1 selected')
}

const dialog = (page: Page) => page.getByRole('dialog', { name: 'rtorrent.remove' })

test('bulk REMOVE opens a confirm dialog; CANCEL is focused and Escape dismisses', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)

  await page.locator('.bulkbar .bbtn.danger').click()
  await expect(dialog(page)).toBeVisible()
  await expect(page.getByText('Remove this torrent?')).toBeVisible()

  // CANCEL is the safe default focus — Enter must never hit the destructive action.
  await expect(page.getByRole('button', { name: 'CANCEL' })).toBeFocused()
  // Capability is OFF on the mock server: no checkbox, an explanatory note instead.
  await expect(page.getByRole('checkbox')).toHaveCount(0)
  await expect(page.getByText("file deletion is disabled by this server's config")).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/remove-confirm.png' })

  // One Escape closes the dialog; the selection is untouched (nothing removed).
  await page.keyboard.press('Escape')
  await expect(dialog(page)).toBeHidden()
  await expect(page.locator('.bulk-count')).toContainText('1 selected')
})

test('Delete key opens the dialog for the selection; search + Delete does not', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)

  // Typing in the search box and pressing Delete edits text — it must NOT open
  // the remove dialog (the typing guard).
  await page.locator('.searchbar input').fill('ubuntu')
  await page.locator('.searchbar input').press('Delete')
  await page.locator('.searchbar input').press('Backspace')
  await expect(dialog(page)).toBeHidden()

  // With focus off the input, Delete acts on the selection.
  await page.locator('body').click({ position: { x: 5, y: 5 } })
  await page.keyboard.press('Delete')
  await expect(dialog(page)).toBeVisible()
})

test('confirm dialog stacks above the detail modal; one Escape closes only it', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')
  await expect(page.locator('footer')).toContainText('torrents')

  await page.locator('[data-torrent]').first().click() // open detail
  await expect(page.locator('.modal-bd')).toBeVisible()
  await page.locator('.rd-btn.danger', { hasText: 'REMOVE' }).click()

  // The confirm backdrop (.rd-bd) is present AND strictly above the detail
  // backdrop; the detail backdrop (.modal-bd without .rd-bd) is still there.
  await expect(page.locator('.rd-bd')).toBeVisible()
  await expect(page.locator('.modal-bd:not(.rd-bd)')).toBeVisible()
  const z = await page.evaluate(() => getComputedStyle(document.querySelector('.rd-bd')!).zIndex)
  expect(z).toBe('200')

  // One Escape closes ONLY the confirm; the detail modal survives behind it.
  await page.keyboard.press('Escape')
  await expect(page.locator('.rd-bd')).toHaveCount(0)
  await expect(page.locator('.modal-bd:not(.rd-bd)')).toBeVisible()
})

test('focus restores to the trigger after cancel', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)

  const trigger = page.locator('.bulkbar .bbtn.danger')
  await trigger.click()
  await expect(dialog(page)).toBeVisible()
  await page.getByRole('button', { name: 'CANCEL' }).click()
  await expect(dialog(page)).toBeHidden()
  await expect(trigger).toBeFocused()
})

test('capability ON: checkbox arms the destructive action and the toast is truthful', async ({ page }) => {
  // Pretend the server enabled deletion (UI only — the mock still refuses the
  // real unlink). We also stub the DELETE so we can assert the request shape and
  // the server-reported outcome drives the toast.
  await page.route('**/api/config', (route) =>
    route.fulfill({ json: { ok: true, data: { name: '', deleteWithData: true } } }),
  )
  let deletedURL = ''
  await page.route(/\/api\/torrents\/[^/]+(\?.*)?$/, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback()
    deletedURL = route.request().url()
    await route.fulfill({ json: { ok: true, data: { erased: true, dataDeleted: true } } })
  })

  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await expect(dialog(page)).toBeVisible()

  const box = page.getByRole('checkbox')
  await expect(box).toBeVisible()
  await box.check()

  // Armed: the primary button re-labels + turns destructive, the summary warns.
  const primary = page.getByRole('button', { name: 'DELETE FILES' })
  await expect(primary).toBeVisible()
  await expect(primary).toHaveClass(/armed/)
  await expect(page.getByText('PERMANENTLY DELETED from disk')).toBeVisible()
  await page.screenshot({ path: 'e2e/screenshots/remove-armed.png' })

  await primary.click()
  await expect(dialog(page)).toBeHidden()
  expect(deletedURL).toContain('?data=true')
  await expect(page.getByText('deleted files from 1')).toBeVisible()
})

test('dismissal is blocked while a removal is in flight', async ({ page }) => {
  await page.route(/\/api\/torrents\/[^/]+(\?.*)?$/, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback()
    await new Promise((r) => setTimeout(r, 700)) // hold the request to observe the busy state
    await route.fulfill({ json: { ok: true, data: { erased: true, dataDeleted: false } } })
  })

  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await expect(dialog(page)).toBeVisible()

  await page.getByRole('button', { name: 'REMOVE', exact: true }).click()
  // Busy: label flips, primary AND cancel disabled, backdrop click is a no-op.
  await expect(page.getByRole('button', { name: 'REMOVING…' })).toBeDisabled()
  await expect(page.getByRole('button', { name: 'CANCEL' })).toBeDisabled()
  await page.locator('.rd-bd').click({ position: { x: 8, y: 8 } })
  await expect(dialog(page)).toBeVisible()
  // Escape is locked mid-flight too (every dismissal path is consistent).
  await page.keyboard.press('Escape')
  await expect(dialog(page)).toBeVisible()

  // Once it resolves, the dialog closes on its own.
  await expect(dialog(page)).toBeHidden({ timeout: 5000 })
})

test('delete-files checkbox is locked while a removal is in flight', async ({ page }) => {
  await page.route('**/api/config', (route) =>
    route.fulfill({ json: { ok: true, data: { name: '', deleteWithData: true } } }),
  )
  await page.route(/\/api\/torrents\/[^/]+(\?.*)?$/, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback()
    await new Promise((r) => setTimeout(r, 700))
    await route.fulfill({ json: { ok: true, data: { erased: true, dataDeleted: true } } })
  })

  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await page.getByRole('checkbox').check()
  await page.getByRole('button', { name: 'DELETE FILES' }).click()
  // mid-flight the box can't be toggled, so the armed consequence copy can't lie.
  await expect(page.getByRole('checkbox')).toBeDisabled()
  await expect(dialog(page)).toBeHidden({ timeout: 5000 })
})

test('capability ON but box unchecked sends no ?data=true', async ({ page }) => {
  await page.route('**/api/config', (route) =>
    route.fulfill({ json: { ok: true, data: { name: '', deleteWithData: true } } }),
  )
  let deletedURL = ''
  await page.route(/\/api\/torrents\/[^/]+(\?.*)?$/, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback()
    deletedURL = route.request().url()
    await route.fulfill({ json: { ok: true, data: { erased: true, dataDeleted: false } } })
  })

  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await expect(page.getByRole('checkbox')).not.toBeChecked() // box starts OFF (never sticky)
  await page.getByRole('button', { name: 'REMOVE', exact: true }).click()
  await expect(dialog(page)).toBeHidden()
  expect(deletedURL).not.toContain('?data=true')
})

test('config fetch failure shows a distinct "capabilities unknown" note (not "disabled by config")', async ({ page }) => {
  await page.route('**/api/config', (route) => route.abort())
  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await expect(dialog(page)).toBeVisible()
  await expect(page.getByRole('checkbox')).toHaveCount(0)
  await expect(page.getByText('server capabilities unknown')).toBeVisible()
  await expect(page.getByText("disabled by this server's config")).toHaveCount(0)
})

test('a total removal failure keeps the selection and does not falsely signal success', async ({ page }) => {
  await page.route(/\/api\/torrents\/[^/]+(\?.*)?$/, async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback()
    await route.fulfill({ status: 503, json: { ok: false, error: { code: 'rtorrent_unreachable', message: 'down' } } })
  })
  await page.emulateMedia({ colorScheme: 'dark' })
  await selectFirstRow(page)
  await page.locator('.bulkbar .bbtn.danger').click()
  await page.getByRole('button', { name: 'REMOVE', exact: true }).click()
  // Dialog closes (the user reads the error toast), but the selection survives —
  // nothing was removed, so the success-only side effect must not have run.
  await expect(dialog(page)).toBeHidden()
  await expect(page.locator('.bulk-count')).toContainText('1 selected')
})
