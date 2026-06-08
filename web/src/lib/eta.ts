// ETA math, pure so it can be unit-tested. rtorrent exposes no real ETA, so this
// is derived from real rate/size — but smoothed and honest about "unknown".

/** EWMA of the download rate (seeds from the first sample, then blends 70/30). */
export function nextSmoothRate(prev: number, rate: number): number {
  return prev > 0 ? prev * 0.7 + rate * 0.3 : rate
}

/**
 * Seconds remaining, or Infinity when unknown — i.e. done (left<=0) or stalled
 * (downRate<=0), which the formatter renders as '—'. Uses the smoothed rate so
 * jitter doesn't swing it; falls back to the instantaneous rate before the EWMA
 * has a value.
 */
export function etaSecondsFor(left: number, downRate: number, smoothRate: number): number {
  if (left <= 0 || downRate <= 0) return Infinity
  return left / (smoothRate > 0 ? smoothRate : downRate)
}
