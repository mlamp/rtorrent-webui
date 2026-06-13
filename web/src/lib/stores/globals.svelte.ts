import type { GlobalsWire } from '$lib/types/torrent'

// 'idle' = deliberately disconnected while the tab is hidden (not an error);
// reconnects with a fresh server snapshot on the next show.
export type Connection = 'connecting' | 'live' | 'reconnecting' | 'idle' | 'offline'

// Sidebar sparkline window: keep the last ~2 minutes of live samples. Short and
// per-tick so the sidebar graph stays snappy/reactive (the longer historical view
// lives in the Insight panel).
const SPEED_WINDOW_S = 120

class GlobalsState {
  downRate = $state(0)
  upRate = $state(0)
  downTotal = $state(0)
  upTotal = $state(0)
  downLimit = $state(0)
  upLimit = $state(0)
  torrentCount = $state(0)
  activeCount = $state(0)
  connection = $state<Connection>('connecting')

  // increments once per poll snapshot/delta. Components that keep their OWN
  // rolling per-torrent buffers (grid cards, the open detail graph) depend on
  // this so their sparklines advance every tick without the store carrying an
  // O(n) history array for every torrent.
  tick = $state(0)

  // Live, time-stamped speed samples for the sidebar sparkline — appended every
  // poll tick and trimmed to the last SPEED_WINDOW_S seconds. Reactive (updates
  // each tick) and rendered time-based, the same shape the charts use.
  speed = $state<{ t: number; down: number; up: number }[]>([])

  apply(g: GlobalsWire) {
    this.downRate = g.downRate
    this.upRate = g.upRate
    this.downTotal = g.downTotal
    this.upTotal = g.upTotal
    this.downLimit = g.downLimit
    this.upLimit = g.upLimit
    this.torrentCount = g.torrentCount
    this.activeCount = g.activeCount
    const t = Math.floor(Date.now() / 1000)
    this.speed = [...this.speed, { t, down: g.downRate, up: g.upRate }].filter((p) => p.t >= t - SPEED_WINDOW_S)
    this.tick++
  }
}

export const globals = new GlobalsState()
