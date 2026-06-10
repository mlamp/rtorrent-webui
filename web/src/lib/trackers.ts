import type { TrackerInfo } from '$lib/types/detail'

/**
 * A tracker is "failing" when it is enabled, has recorded at least one announce
 * failure, and its most recent attempt failed (last-failed is no older than
 * last-succeeded). rtorrent exposes per-tracker state only as counters +
 * timestamps — the failure TEXT lives in the torrent-wide d.message — so this
 * predicate is what drives the red dot / "failing" label in the detail view.
 *
 * failedAt >= successAt (not strictly >) so a tracker that has only ever failed
 * (successAt == 0) and one whose fail/ok landed in the same second both read as
 * failing; a later success (successAt > failedAt) clears it.
 *
 * Pure, so it is unit-tested (see web/test/trackers.test.ts).
 */
export function trackerFailing(
  tr: Pick<TrackerInfo, 'enabled' | 'failed' | 'failedAt' | 'successAt'>,
): boolean {
  return tr.enabled && tr.failed > 0 && tr.failedAt >= tr.successAt
}
