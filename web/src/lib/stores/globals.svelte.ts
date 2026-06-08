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

  apply(g: GlobalsWire) {
    this.downRate = g.downRate
    this.upRate = g.upRate
    this.downTotal = g.downTotal
    this.upTotal = g.upTotal
    this.downLimit = g.downLimit
    this.upLimit = g.upLimit
    this.torrentCount = g.torrentCount
    this.activeCount = g.activeCount
  }
}

export const globals = new GlobalsState()
