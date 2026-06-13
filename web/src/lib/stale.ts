export const STALE_LAG_S = 15

/**
 * Per-connection staleness gauge for SSE messages, used to detect a resume
 * backlog (a frozen tab draining queued events) without trusting wall clocks.
 *
 * skew := min over this connection's messages of (rxSec − msgTs); a message is
 * stale when (rxSec − msgTs) − skew > staleLagS (strict >). Min-tracking is
 * immune to absolute client/server clock skew and to the hub's possibly-stale
 * cached snapshot: the first sample only sets the baseline (it can never read
 * stale itself), and later live deltas correct the baseline downward — lag is
 * only ever over-, never under-corrected into a false positive.
 *
 * Clock behavior: a client clock step backward gives negative lag → never a
 * false positive. A forward jump reads stale and STAYS stale on this instance —
 * the driver creates a fresh gauge per connection, which is where the "one
 * cheap resync" property comes from. A server-clock back-step > staleLagS also
 * reads stale once (acceptable: one spurious resync, then the new connection's
 * gauge re-baselines on the post-step timestamps).
 */
export function makeStalenessGauge(staleLagS = STALE_LAG_S): (msgTs: number, rxSec: number) => 'fresh' | 'stale' {
  let skew: number | undefined
  return (msgTs, rxSec) => {
    const lag = rxSec - msgTs
    if (skew === undefined || lag < skew) {
      skew = lag
      return 'fresh'
    }
    return lag - skew > staleLagS ? 'stale' : 'fresh'
  }
}
