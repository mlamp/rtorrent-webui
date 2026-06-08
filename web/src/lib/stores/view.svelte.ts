import type { Status } from '$lib/types/torrent'
import { trackerHost } from '$lib/format'
import type { TorrentRow } from './torrents.svelte'

export type StatusFilter = 'all' | 'active' | 'downloading' | 'seeding' | 'stopped' | 'error'
export type ViewMode = 'list' | 'grid' | 'insight'
export type ColumnKey =
  | 'name'
  | 'size'
  | 'done'
  | 'downRate'
  | 'upRate'
  | 'rate'
  | 'eta'
  | 'ratio'
  | 'status'
  | 'label'
  | 'added'

class ViewState {
  status = $state<StatusFilter>('all')
  label = $state<string | null>(null)
  tracker = $state<string | null>(null)
  search = $state('')
  sortKey = $state<ColumnKey>('name')
  sortDir = $state<1 | -1>(1)
  /** which primary view is showing (list cards/grid cards/insight). */
  mode = $state<ViewMode>('list')
  /** keyboard-navigation cursor (a torrent hash), independent of selection. */
  cursor = $state<string | null>(null)

  toggleSort(key: ColumnKey) {
    if (this.sortKey === key) this.sortDir = this.sortDir === 1 ? -1 : 1
    else {
      this.sortKey = key
      // name reads best ascending; everything else (rates, size, ratio) descending
      this.sortDir = key === 'name' ? 1 : -1
    }
  }
  cycleMode() {
    this.mode = this.mode === 'list' ? 'grid' : this.mode === 'grid' ? 'insight' : 'list'
  }
}

export const view = new ViewState()

const statusMatch: Record<StatusFilter, (s: Status) => boolean> = {
  all: () => true,
  active: () => true, // handled by rate, see matches()
  downloading: (s) => s === 'downloading',
  seeding: (s) => s === 'seeding',
  stopped: (s) => s === 'stopped' || s === 'paused',
  error: (s) => s === 'error',
}

/** ACTIVE = anything currently transferring (down or up). */
export const isActive = (t: TorrentRow): boolean => t.downRate > 0 || t.upRate > 0

export function matches(t: TorrentRow, v: ViewState): boolean {
  if (v.status === 'active') {
    if (!isActive(t)) return false
  } else if (!statusMatch[v.status](t.status)) return false
  if (v.label !== null && t.label !== v.label) return false
  if (v.tracker !== null && trackerHost(t.tracker) !== v.tracker) return false
  if (v.search && !t.name.toLowerCase().includes(v.search.toLowerCase())) return false
  return true
}

export function compare(a: TorrentRow, b: TorrentRow, key: ColumnKey, dir: 1 | -1): number {
  let r = 0
  switch (key) {
    case 'name':
      r = a.name.localeCompare(b.name)
      break
    case 'size':
      r = a.size - b.size
      break
    case 'done':
      r = a.done - b.done
      break
    case 'downRate':
      r = a.downRate - b.downRate
      break
    case 'upRate':
      r = a.upRate - b.upRate
      break
    case 'rate':
      r = a.downRate + a.upRate - (b.downRate + b.upRate)
      break
    case 'eta': {
      const av = isFinite(a.etaSeconds) ? a.etaSeconds : Number.MAX_SAFE_INTEGER
      const bv = isFinite(b.etaSeconds) ? b.etaSeconds : Number.MAX_SAFE_INTEGER
      r = av - bv
      break
    }
    case 'ratio':
      r = a.ratio - b.ratio
      break
    case 'status':
      r = a.status.localeCompare(b.status)
      break
    case 'label':
      r = a.label.localeCompare(b.label)
      break
    case 'added':
      r = a.added - b.added
      break
  }
  return r * dir
}
