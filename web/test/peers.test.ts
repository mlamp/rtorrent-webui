import { peerFlags } from '../src/lib/peers.ts'

let pass = 0, fail = 0
function eq(name: string, got: unknown, want: unknown) {
  if (got === want) { pass++; return }
  fail++
  console.error(`FAIL ${name}\n  got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}

// fixed positions: [encrypted?E:·][incoming?I:O][snubbed?S:·]
eq('all false', peerFlags({ encrypted: false, incoming: false, snubbed: false }), '·O·')
eq('all true', peerFlags({ encrypted: true, incoming: true, snubbed: true }), 'EIS')
eq('incoming only', peerFlags({ encrypted: false, incoming: true, snubbed: false }), '·I·')
eq('outgoing+snubbed', peerFlags({ encrypted: true, incoming: false, snubbed: true }), 'EOS')
// position guard: incoming and snubbed must not be confused
eq('incoming≠snubbed slot', peerFlags({ encrypted: false, incoming: true, snubbed: false }), '·I·')

console.log(`\npeers.ts: ${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
