import { api, silentGet } from '$lib/api/client'
import type { FileInfo, PeerInfo, TrackerInfo, PiecesInfo } from '$lib/types/detail'

export type DetailTab = 'general' | 'files' | 'peers' | 'trackers' | 'speed'

class DetailState {
  activeHash = $state<string | null>(null)
  tab = $state<DetailTab>('general')
  files = $state<FileInfo[]>([])
  peers = $state<PeerInfo[]>([])
  trackers = $state<TrackerInfo[]>([])
  pieces = $state<PiecesInfo | null>(null)
  loading = $state(false)
  // set true around mutating calls (file priority / tracker toggle) so a silent
  // background refresh can't clobber the optimistic result mid-flight.
  busy = $state(false)
  // monotonic guard: only the latest refreshActive() response may apply.
  refreshSeq = 0

  // client-side ring buffer of the active torrent's rates for the Speed tab
  speedDown = $state<number[]>([])
  speedUp = $state<number[]>([])

  open(hash: string) {
    if (this.activeHash === hash) {
      this.close()
      return
    }
    this.activeHash = hash
    this.tab = 'general' // every newly-opened torrent starts on PIECES, not a stale tab
    this.speedDown = []
    this.speedUp = []
    this.pieces = null
    this.load()
  }
  close() {
    this.activeHash = null
  }
  setTab(t: DetailTab) {
    this.tab = t
    this.load()
  }

  async load() {
    const h = this.activeHash
    if (!h) return
    this.loading = true
    try {
      if (this.tab === 'general') this.pieces = (await api.getPieces(h)) ?? null
      else if (this.tab === 'files') this.files = (await api.getFiles(h)) ?? []
      else if (this.tab === 'peers') this.peers = (await api.getPeers(h)) ?? []
      else if (this.tab === 'trackers') this.trackers = (await api.getTrackers(h)) ?? []
    } catch {
      /* toast shown */
    } finally {
      this.loading = false
    }
  }

  // Silent refresh of whichever tab is open (no loading flag), so the detail panel
  // stays live while open without flickering a spinner — "subscribed when opened".
  // Guards: a monotonic seq + activeHash/tab recheck discard stale responses, and
  // `busy` skips refreshing while a user mutation is in flight.
  async refreshActive() {
    const h = this.activeHash
    const tab = this.tab
    if (!h || this.busy) return
    const seq = ++this.refreshSeq
    const path =
      tab === 'general'
        ? 'pieces'
        : tab === 'files'
          ? 'files'
          : tab === 'peers'
            ? 'peers'
            : tab === 'trackers'
              ? 'trackers'
              : ''
    if (!path) return
    const d = await silentGet<unknown>(`/api/torrents/${h}/${path}`)
    // drop if stale (hash/tab moved on, a newer refresh started) or a mutation began
    if (d == null || seq !== this.refreshSeq || this.activeHash !== h || this.tab !== tab || this.busy) return
    if (tab === 'general') this.pieces = d as PiecesInfo
    else if (tab === 'files') this.files = d as FileInfo[]
    else if (tab === 'peers') this.peers = d as PeerInfo[]
    else if (tab === 'trackers') this.trackers = d as TrackerInfo[]
  }

  pushSpeed(down: number, up: number) {
    this.speedDown = [...this.speedDown.slice(-119), down]
    this.speedUp = [...this.speedUp.slice(-119), up]
  }

  async setFilePriority(index: number, priority: number) {
    if (!this.activeHash) return
    this.busy = true
    try {
      await api.setFilePriority(this.activeHash, index, priority)
      await this.load()
    } finally {
      this.busy = false
    }
  }
  async toggleTracker(index: number, enabled: boolean) {
    if (!this.activeHash) return
    this.busy = true
    try {
      await api.setTrackerEnabled(this.activeHash, index, enabled)
      await this.load()
    } finally {
      this.busy = false
    }
  }
}

export const detail = new DetailState()
