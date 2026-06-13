import { untrack } from 'svelte'
import { lifecycle } from './stores/lifecycle.svelte'

/**
 * Run fn now and every `ms` while the page is visible; tear down when hidden,
 * refresh-then-restart on return. lifecycle.visible must stay the effect's
 * ONLY dependency — fn is untracked because the loaders it wraps read $state
 * synchronously before their first await (range, hashes, detail tab/busy), and
 * tracking those would re-fire the poll and re-phase the interval on every
 * unrelated store change (same untrack pattern as src/lib/history.svelte.ts).
 * Call during component init.
 */
export function pollWhileVisible(fn: () => void, ms: number): void {
  $effect(() => {
    if (!lifecycle.visible) return
    untrack(fn)
    const id = setInterval(fn, ms)
    return () => clearInterval(id)
  })
}
