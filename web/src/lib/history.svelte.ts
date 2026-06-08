import { untrack } from 'svelte'
import { globals } from './stores/globals.svelte'

/**
 * A rolling per-component buffer of (down, up) rate samples, advanced exactly
 * once per global poll tick (globals.tick). Sampling the source inside untrack
 * means the buffer advances on the tick cadence only — not on every rate
 * change — so a steady transfer still scrolls and an idle torrent reads flat.
 *
 * Scoped to the mounting component (grid card / open detail), so we never carry
 * an O(n) history array for every torrent in the store. Call during init.
 */
export function rollingHistory(
  source: () => { down: number; up: number },
  n = 44,
  seed?: () => { dl: number[]; ul: number[] },
) {
  const pad = (a: number[]) => {
    const t = a.slice(-n)
    return t.length < n ? [...Array(n - t.length).fill(0), ...t] : t
  }
  const s = seed?.()
  let dl = $state<number[]>(s ? pad(s.dl) : Array(n).fill(0))
  let ul = $state<number[]>(s ? pad(s.ul) : Array(n).fill(0))

  $effect(() => {
    globals.tick // the sole tracked dependency — fires once per poll
    untrack(() => {
      const { down, up } = source()
      dl = [...dl.slice(-(n - 1)), down]
      ul = [...ul.slice(-(n - 1)), up]
    })
  })

  return {
    get dl() {
      return dl
    },
    get ul() {
      return ul
    },
  }
}
