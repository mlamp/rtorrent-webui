import { test, expect } from '@playwright/test'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, mkdirSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// The save-to directory combobox lights up only when the server advertises
// `browse` (a unix-socket/override transport AND >=1 resolvable download root).
//
// Part A runs against the default -mock webServer (no downloads.dirs) -> browse
// OFF -> the field must stay a plain text input and the existing locators must be
// untouched. Part B stands up a throwaway server with -disk-dirs pointing at a
// temp tree so the listing reflects a known, sandboxed filesystem, and drives the
// roots -> typeahead -> drill-in flow end to end.

test('save-to stays a plain input when browse is disabled (default mock)', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Add' }).first().click()
  await expect(page.getByText('rtorrent.add')).toBeVisible()

  // No combobox affordance, and the plain free-text save-to input is present.
  await expect(page.getByRole('combobox')).toHaveCount(0)
  const dest = page.getByPlaceholder('~/downloads (rtorrent default)')
  await expect(dest).toBeVisible()
  await dest.fill('/srv/manual/path')
  await expect(dest).toHaveValue('/srv/manual/path') // free-text still works

  // Existing locators preserved: exactly one textarea, the Close button.
  await expect(page.locator('textarea')).toHaveCount(1)
  await page.getByRole('button', { name: 'Close' }).click()
  await expect(page.getByText('rtorrent.add')).toBeHidden()
})

test.describe('browse enabled (server with download roots)', () => {
  const PORT = 8097
  const BASE = `http://localhost:${PORT}`
  let srv: ChildProcess
  let root: string

  test.beforeAll(async () => {
    // A sandboxed tree the webui-side listing will enumerate.
    root = mkdtempSync(join(tmpdir(), 'rtbrowse-'))
    for (const d of ['movies', 'music', 'books', join('music', 'live')]) {
      mkdirSync(join(root, d), { recursive: true })
    }
    // -mock keeps the default unix socket (browse transport gate true); -disk-dirs
    // gives a resolvable root, so /api/config reports browse:true.
    srv = spawn('../bin/rtorrent-webui', ['-addr', `:${PORT}`, '-mock', '5', '-disk-dirs', root], {
      stdio: 'ignore',
    })
    // Wait for /healthz (mock mode goes green without a daemon).
    const deadline = Date.now() + 15_000
    for (;;) {
      try {
        const r = await fetch(`${BASE}/healthz`)
        if (r.ok) break
      } catch {
        /* not up yet */
      }
      if (Date.now() > deadline) throw new Error('browse test server did not start')
      await new Promise((r) => setTimeout(r, 150))
    }
  })

  test.afterAll(() => {
    srv?.kill('SIGKILL')
    if (root) rmSync(root, { recursive: true, force: true })
  })

  test('roots, typeahead filter, and click drill-in', async ({ page }) => {
    await page.goto(`${BASE}/`)
    await page.getByRole('button', { name: 'Add' }).first().click()
    await expect(page.getByText('rtorrent.add')).toBeVisible()

    // The save-to field is now a combobox.
    const combo = page.getByRole('combobox')
    await expect(combo).toHaveCount(1)
    await combo.click()

    // Top level: the single configured root. Drill into it.
    await expect(page.getByRole('listbox')).toBeVisible()
    await page.getByRole('option').first().click()

    // Its children appear.
    await expect(page.getByRole('option', { name: 'movies' })).toBeVisible()
    await expect(page.getByRole('option', { name: 'music' })).toBeVisible()
    await expect(page.getByRole('option', { name: 'books' })).toBeVisible()

    // Typeahead filters client-side as we type.
    await page.keyboard.type('mu')
    await expect(page.getByRole('option', { name: 'music' })).toBeVisible()
    await expect(page.getByRole('option', { name: 'movies' })).toHaveCount(0)

    // Drill into music -> its child "live" shows.
    await page.getByRole('option', { name: 'music' }).click()
    await expect(page.getByRole('option', { name: 'live' })).toBeVisible()

    // The input reflects the drilled path (single source of truth).
    await expect(combo).toHaveValue(/\/music\/$/)
  })

  test('keyboard nav: ArrowDown highlights, Enter commits the active option', async ({ page }) => {
    await page.goto(`${BASE}/`)
    await page.getByRole('button', { name: 'Add' }).first().click()
    const combo = page.getByRole('combobox')
    await combo.click()
    await page.getByRole('option').first().click() // into the root
    await expect(page.getByRole('option', { name: 'books' })).toBeVisible()

    // ArrowDown highlights the first option; Enter commits (drills into) it.
    await page.keyboard.press('ArrowDown')
    await expect(page.locator('.combo-opt.on')).toHaveCount(1)
    await page.keyboard.press('Enter')
    // books has no children -> "no matching folders", proving the commit happened.
    await expect(page.getByText('no matching folders')).toBeVisible()
    await expect(combo).toHaveValue(/\/books\/$/)
  })
})
