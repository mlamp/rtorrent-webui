export type Status =
  | 'stopped'
  | 'downloading'
  | 'seeding'
  | 'paused'
  | 'hashing'
  | 'error'

export interface TorrentWire {
  hash: string
  name: string
  size: number
  completed: number
  downRate: number
  upRate: number
  upTotal: number
  ratio: number // permille (rtorrent ratio*1000)
  status: Status
  label: string
  directory: string
  peersConnected: number
  peersTotal: number
  seedsConnected: number
  seedsTotal: number
  tracker: string
  added: number
  message: string
}

export type TorrentPatch = Partial<TorrentWire> & { hash: string }

export interface GlobalsWire {
  downRate: number
  upRate: number
  downTotal: number
  upTotal: number
  downLimit: number
  upLimit: number
  torrentCount: number
  activeCount: number
}

export interface SnapshotMsg {
  seq: number
  ts: number
  globals: GlobalsWire
  torrents: TorrentWire[]
}

export interface DeltaMsg {
  seq: number
  ts: number
  globals: GlobalsWire
  upserts: TorrentPatch[]
  removed: string[] | null
}
