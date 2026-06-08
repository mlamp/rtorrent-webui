import type { Status } from '$lib/types/torrent'
import type { TorrentRow } from './torrents.svelte'

export type StatusFilter = 'all' | 'downloading' | 'seeding' | 'stopped' | 'error'
export type ColumnKey =
  | 'name'
  | 'size'
  | 'done'
  | 'downRate'
  | 'upRate'
  | 'ratio'
  | 'status'
  | 'label'
  | 'added'

class ViewState {
  status = $state<StatusFilter>('all')
  label = $state<string | null>(null)
  search = $state('')
  sortKey = $state<ColumnKey>('name')
  sortDir = $state<1 | -1>(1)

  toggleSort(key: ColumnKey) {
    if (this.sortKey === key) this.sortDir = this.sortDir === 1 ? -1 : 1
    else {
      this.sortKey = key
      this.sortDir = 1
    }
  }
}

export const view = new ViewState()

const statusMatch: Record<StatusFilter, (s: Status) => boolean> = {
  all: () => true,
  downloading: (s) => s === 'downloading',
  seeding: (s) => s === 'seeding',
  stopped: (s) => s === 'stopped' || s === 'paused',
  error: (s) => s === 'error',
}

export function matches(t: TorrentRow, v: ViewState): boolean {
  if (!statusMatch[v.status](t.status)) return false
  if (v.label !== null && t.label !== v.label) return false
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
