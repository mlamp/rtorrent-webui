import type { GlobalsWire } from '$lib/types/torrent'

export type Connection = 'connecting' | 'live' | 'reconnecting' | 'offline'

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

  apply(g: GlobalsWire) {
    this.downRate = g.downRate
    this.upRate = g.upRate
    this.downTotal = g.downTotal
    this.upTotal = g.upTotal
    this.downLimit = g.downLimit
    this.upLimit = g.upLimit
    this.torrentCount = g.torrentCount
    this.activeCount = g.activeCount
    this.tick++
  }
}

export const globals = new GlobalsState()
