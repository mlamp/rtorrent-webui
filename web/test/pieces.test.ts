import { decodePieces } from '../src/lib/pieces.ts'

let pass = 0
let fail = 0
function eq(name: string, got: unknown, want: unknown) {
  const g = JSON.stringify(got)
  const w = JSON.stringify(want)
  if (g === w) { pass++; return }
  fail++
  console.error(`FAIL ${name}\n  got  ${g}\n  want ${w}`)
}

// 8 chunks, byte 0xC0 = 11000000 → chunks 0,1 set (MSB-first)
const r1 = decodePieces('c0', 8, 2, 600)
eq('binary map per chunk', r1.mode === 'cells' && r1.cells, [1, 1, 0, 0, 0, 0, 0, 0])

// complete sentinel "0" → all ones regardless of bitfield content
const r2 = decodePieces('0', 4, 4, 600)
eq('complete sentinel', r2.mode === 'cells' && r2.cells, [1, 1, 1, 1])

// completed==0 → all empty even if bitfield is junk
const r3 = decodePieces('', 4, 0, 600)
eq('empty', r3.mode === 'cells' && r3.cells, [0, 0, 0, 0])

// partial but no usable bitfield → bar fallback (never fabricate)
eq('partial w/o bitfield → bar', decodePieces('', 100, 50, 600).mode, 'bar')
eq('short bitfield → bar', decodePieces('ff', 100, 50, 600).mode, 'bar') // 1 byte = 8 bits < 100

// sizeChunks 0 / negative → bar
eq('no chunks → bar', decodePieces('ff', 0, 0, 600).mode, 'bar')

// aggregation: 16 chunks, first 8 set (0xff00), budget 4 → 4 cells each covering 4 chunks
const r4 = decodePieces('ff00', 16, 8, 4)
eq('aggregated fractions', r4.mode === 'cells' && r4.cells, [1, 1, 0, 0])
// aggregated completed count must equal the real popcount
eq('aggregated completed total', r4.mode === 'cells' && r4.completed, 8)

// odd bucketing still tiles all chunks exactly (completed == popcount)
const bits = 'aa55' // 10101010 01010101 → 8 of 16 set
const r5 = decodePieces(bits, 16, 8, 5)
eq('odd bucket completed total', r5.mode === 'cells' && r5.completed, 8)

// malformed hex → bar
eq('malformed hex → bar', decodePieces('zz', 4, 2, 600).mode, 'bar')

console.log(`\npieces.ts: ${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
