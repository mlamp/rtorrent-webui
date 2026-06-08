// Peer flag glyphs — real protocol state only (no fabricated activity bits).
// E = encrypted, I/O = incoming/outgoing, S = snubbed. Pure so it's unit-tested.
export function peerFlags(p: { encrypted: boolean; incoming: boolean; snubbed: boolean }): string {
  return `${p.encrypted ? 'E' : '·'}${p.incoming ? 'I' : 'O'}${p.snubbed ? 'S' : '·'}`
}
