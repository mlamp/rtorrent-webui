import { SvelteMap } from 'svelte/reactivity'
import type { Status, TorrentWire, TorrentPatch } from '$lib/types/torrent'

/**
 * One torrent row. Each field is an independent $state signal, so a delta that
 * changes only `downRate` updates exactly that cell — no row re-render, no list
 * diff. This is the lever that keeps 1000+ rows smooth at ~1 update/sec.
 */
export class TorrentRow {
  readonly hash: string
  name = $state('')
  size = $state(0)
  completed = $state(0)
  downRate = $state(0)
  upRate = $state(0)
  upTotal = $state(0)
  ratio = $state(0)
  status = $state<Status>('stopped')
  label = $state('')
  directory = $state('')
  peersConnected = $state(0)
  peersTotal = $state(0)
  seedsConnected = $state(0)
  seedsTotal = $state(0)
  tracker = $state('')
  added = $state(0)
  message = $state('')

  // transient: true for ~1.4s after a download completes (drives the row sweep)
  sweeping = $state(false)

  done = $derived(this.size > 0 ? this.completed / this.size : 0)
  etaSeconds = $derived.by(() => {
    const left = this.size - this.completed
    return left > 0 && this.downRate > 0 ? left / this.downRate : Infinity
  })

  constructor(hash: string) {
    this.hash = hash
  }

  private sweepTimer: ReturnType<typeof setTimeout> | undefined
  private triggerSweep() {
    this.sweeping = true
    clearTimeout(this.sweepTimer)
    this.sweepTimer = setTimeout(() => (this.sweeping = false), 1400)
  }

  apply(p: Partial<TorrentWire>) {
    const prevStatus = this.status
    if (p.name !== undefined) this.name = p.name
    if (p.size !== undefined) this.size = p.size
    if (p.completed !== undefined) this.completed = p.completed
    if (p.downRate !== undefined) this.downRate = p.downRate
    if (p.upRate !== undefined) this.upRate = p.upRate
    if (p.upTotal !== undefined) this.upTotal = p.upTotal
    if (p.ratio !== undefined) this.ratio = p.ratio
    if (p.status !== undefined) this.status = p.status
    if (p.label !== undefined) this.label = p.label
    if (p.directory !== undefined) this.directory = p.directory
    if (p.peersConnected !== undefined) this.peersConnected = p.peersConnected
    if (p.peersTotal !== undefined) this.peersTotal = p.peersTotal
    if (p.seedsConnected !== undefined) this.seedsConnected = p.seedsConnected
    if (p.seedsTotal !== undefined) this.seedsTotal = p.seedsTotal
    if (p.tracker !== undefined) this.tracker = p.tracker
    if (p.added !== undefined) this.added = p.added
    if (p.message !== undefined) this.message = p.message

    // a torrent that just finished (downloading -> seeding) gets a sweep flourish
    if (p.status !== undefined && prevStatus === 'downloading' && p.status === 'seeding') {
      this.triggerSweep()
    }
  }
}

class TorrentStore {
  /** SvelteMap drives MEMBERSHIP reactivity only (#each re-runs on add/remove). */
  map = new SvelteMap<string, TorrentRow>()

  applySnapshot(rows: TorrentWire[]) {
    // Reconcile in place so SvelteMap mutations stay reactive (the `map` field
    // is not $state) and existing rows keep their identity across reconnects.
    const seen = new Set<string>()
    for (const r of rows) {
      seen.add(r.hash)
      const ex = this.map.get(r.hash)
      if (ex) ex.apply(r)
      else {
        const t = new TorrentRow(r.hash)
        t.apply(r)
        this.map.set(r.hash, t)
      }
    }
    for (const h of [...this.map.keys()]) {
      if (!seen.has(h)) this.map.delete(h)
    }
  }

  applyUpserts(upserts: TorrentPatch[]) {
    for (const u of upserts) {
      const ex = this.map.get(u.hash)
      if (ex) ex.apply(u) // field-level update, no map churn
      else {
        const t = new TorrentRow(u.hash)
        t.apply(u)
        this.map.set(u.hash, t)
      }
    }
  }

  remove(hashes: string[]) {
    for (const h of hashes) this.map.delete(h)
  }
}

export const torrents = new TorrentStore()
