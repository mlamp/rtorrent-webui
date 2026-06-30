import { torrents } from '$lib/stores/torrents.svelte'
import { globals } from '$lib/stores/globals.svelte'
import { lifecycle } from '$lib/stores/lifecycle.svelte'
import { createSseDriver } from '$lib/sse-driver'

/**
 * Thin shell around the visibility-aware SSE driver: real EventSource, real
 * timers/clock, and the rune-bearing stores live here; every lifecycle and
 * staleness decision lives in $lib/sse-driver (pure, unit-tested in plain node
 * — see test/sse-driver.test.ts for the invariants).
 */
export function connectSSE(url = '/api/events'): () => void {
  const driver = createSseDriver(url, {
    createES: (u) => new EventSource(u),
    setTimeout: (fn, ms) => setTimeout(fn, ms),
    clearTimeout: (id) => clearTimeout(id as ReturnType<typeof setTimeout>),
    now: Date.now,
    random: Math.random,
    visible: () => lifecycle.visible,
    onConnection: (c) => (globals.connection = c),
    applySnapshot: (d) => {
      torrents.applySnapshot(d.torrents)
      globals.apply(d.globals)
    },
    applyDelta: (d) => {
      if (d.upserts?.length) torrents.applyUpserts(d.upserts)
      if (d.removed?.length) torrents.remove(d.removed)
      globals.apply(d.globals)
    },
    warn: (m) => console.warn(m),
    onHealth: (h) => (globals.rtHealth = h),
  })
  const unsub = lifecycle.subscribe((s) => driver.signal(s))
  driver.start()
  return () => {
    unsub()
    driver.stop()
  }
}
