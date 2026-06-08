export interface FileInfo {
  index: number
  path: string
  size: number
  completedChunks: number
  sizeChunks: number
  priority: number // 0 off, 1 normal, 2 high
  done: number // 0..1
}

export interface PeerInfo {
  address: string
  port: number
  client: string
  downRate: number
  upRate: number
  progress: number // 0..100
  encrypted: boolean
  incoming: boolean
  snubbed: boolean // real rtorrent protocol state (p.is_snubbed)
  country: string // ISO alpha-2 ("" if unknown)
}

export interface PiecesInfo {
  bitfield: string // hex (MSB-first per byte); "0" sentinel = complete
  sizeChunks: number
  completedChunks: number
  chunkSize: number
}

export interface TrackerInfo {
  index: number
  url: string
  enabled: boolean
  type: number
  latestEvent: string
  success: number
}
