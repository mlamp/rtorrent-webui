import { api } from '$lib/api/client'
import type { FileInfo, PeerInfo, TrackerInfo } from '$lib/types/detail'

export type DetailTab = 'general' | 'files' | 'peers' | 'trackers' | 'speed'

class DetailState {
  activeHash = $state<string | null>(null)
  tab = $state<DetailTab>('general')
  files = $state<FileInfo[]>([])
  peers = $state<PeerInfo[]>([])
  trackers = $state<TrackerInfo[]>([])
  loading = $state(false)

  // client-side ring buffer of the active torrent's rates for the Speed tab
  speedDown = $state<number[]>([])
  speedUp = $state<number[]>([])

  open(hash: string) {
    if (this.activeHash === hash) {
      this.close()
      return
    }
    this.activeHash = hash
    this.speedDown = []
    this.speedUp = []
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
      if (this.tab === 'files') this.files = (await api.getFiles(h)) ?? []
      else if (this.tab === 'peers') this.peers = (await api.getPeers(h)) ?? []
      else if (this.tab === 'trackers') this.trackers = (await api.getTrackers(h)) ?? []
    } catch {
      /* toast shown */
    } finally {
      this.loading = false
    }
  }

  pushSpeed(down: number, up: number) {
    this.speedDown = [...this.speedDown.slice(-119), down]
    this.speedUp = [...this.speedUp.slice(-119), up]
  }

  async setFilePriority(index: number, priority: number) {
    if (!this.activeHash) return
    await api.setFilePriority(this.activeHash, index, priority)
    await this.load()
  }
  async toggleTracker(index: number, enabled: boolean) {
    if (!this.activeHash) return
    await api.setTrackerEnabled(this.activeHash, index, enabled)
    await this.load()
  }
}

export const detail = new DetailState()
