// Deterministic synthetic speed series for historical time windows in the
// per-torrent detail graph. The global INSIGHT traffic graph uses the real
// /api/history endpoint; rtorrent does not retain per-infohash throughput
// history, so longer detail frames fall back to this illustrative generator
// (ported from the Relay prototype's shared.jsx — same shape: mostly-idle
// baseline with occasional spikes). The live "15m" frame uses the real rolling
// buffer instead.

export const FRAMES = ['15m', '1h', '6h', '24h', '7d', '1y'] as const
export type Frame = (typeof FRAMES)[number]

const FRAME_CFG: Record<Frame, { n: number; spike: number; decay: number }> = {
  '15m': { n: 46, spike: 0.05, decay: 0.78 },
  '1h': { n: 60, spike: 0.05, decay: 0.8 },
  '6h': { n: 72, spike: 0.07, decay: 0.82 },
  '24h': { n: 96, spike: 0.08, decay: 0.84 },
  '7d': { n: 84, spike: 0.1, decay: 0.86 },
  '1y': { n: 73, spike: 0.13, decay: 0.88 },
}

function seriesGen(seedStr: string, n: number, base: number, spike: number, decay: number): number[] {
  let h = 2166136261
  for (let i = 0; i < seedStr.length; i++) {
    h ^= seedStr.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  const rnd = () => {
    h = (Math.imul(h, 1103515245) + 12345) & 0x7fffffff
    return h / 0x7fffffff
  }
  let v = base * 0.25
  const out: number[] = []
  for (let i = 0; i < n; i++) {
    v = Math.max(0, v * decay + base * 0.18 * rnd())
    if (rnd() < spike) v += base * (0.8 + rnd() * 3.4)
    out.push(v)
  }
  return out
}

/** Deterministic {dl, ul} for a seed (torrent hash) + frame. */
export function frameSeries(seed: string, frame: Frame, baseDl: number, baseUl: number): { dl: number[]; ul: number[] } {
  const c = FRAME_CFG[frame] ?? FRAME_CFG['1h']
  return {
    dl: seriesGen(seed + ':dl:' + frame, c.n, Math.max(baseDl, 1), c.spike, c.decay),
    ul: seriesGen(seed + ':ul:' + frame, c.n, Math.max(baseUl, 1), c.spike * 1.1, c.decay),
  }
}
