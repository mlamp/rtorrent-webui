// Decode rtorrent's d.bitfield into a renderable cell array — the source of truth
// for the detail PIECES map (replacing the old size/done% guess).
//
// rtorrent's bitfield is a hex string, MSB-first per byte (chunk 0 = bit 7 of byte
// 0). The literal "0" sentinel means "complete, bitfield freed". Each returned cell
// is a completion fraction in [0,1]; when a torrent has more chunks than the cell
// budget, cells aggregate so the map stays a faithful density rather than a guess.

export type PieceView =
  | { mode: 'cells'; cells: number[]; chunks: number; completed: number }
  | { mode: 'bar' } // no usable per-piece data — caller should show the % bar instead

export function decodePieces(
  bitfield: string,
  sizeChunks: number,
  completedChunks: number,
  budget = 600,
): PieceView {
  if (!Number.isFinite(sizeChunks) || sizeChunks <= 0) return { mode: 'bar' }

  // Resolve a per-chunk have[] from the most reliable signal available.
  let have: Uint8Array | null
  if (completedChunks >= sizeChunks) {
    have = new Uint8Array(sizeChunks).fill(1) // complete (also covers the "0" sentinel)
  } else if (completedChunks <= 0) {
    have = new Uint8Array(sizeChunks) // nothing yet (all zero)
  } else {
    have = decodeHex(bitfield, sizeChunks)
    if (!have) return { mode: 'bar' } // partial but no valid bitfield — never fabricate a layout
  }

  // Bucket into at most `budget` cells; exactly one cell per chunk when they fit.
  const cellCount = Math.min(sizeChunks, Math.max(1, Math.floor(budget)))
  const cells = new Array<number>(cellCount)
  let completed = 0
  for (let k = 0; k < cellCount; k++) {
    const lo = Math.floor((k * sizeChunks) / cellCount)
    const hi = k === cellCount - 1 ? sizeChunks : Math.floor(((k + 1) * sizeChunks) / cellCount)
    let set = 0
    for (let c = lo; c < hi; c++) if (have[c]) set++
    cells[k] = set / (hi - lo)
    completed += set
  }
  return { mode: 'cells', cells, chunks: sizeChunks, completed }
}

// MSB-first per byte. Returns null if the hex is malformed or too short for sizeChunks.
function decodeHex(bitfield: string, sizeChunks: number): Uint8Array | null {
  const hex = bitfield.trim()
  if (!/^[0-9a-fA-F]+$/.test(hex) || hex.length % 2 !== 0) return null
  if ((hex.length >> 1) * 8 < sizeChunks) return null
  const out = new Uint8Array(sizeChunks)
  for (let c = 0; c < sizeChunks; c++) {
    const byte = parseInt(hex.slice((c >> 3) * 2, (c >> 3) * 2 + 2), 16)
    out[c] = (byte >> (7 - (c & 7))) & 1
  }
  return out
}
