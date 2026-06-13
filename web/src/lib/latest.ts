// latestOnly wraps an async loader (fetch + apply-to-state) used by polling UI
// like InsightView: range clicks and the 3s interval poll race the same endpoint,
// so a slow response for a previously-selected range must not overwrite the
// current one. Same out-of-order guard as RowDetail.svelte's reqSeq.
export function latestOnly<T>(fetch: () => Promise<T | null>, apply: (data: T) => void): () => Promise<void> {
  let seq = 0
  return async () => {
    const token = ++seq
    const d = await fetch()
    if (token !== seq || !d) return // stale or failed — a newer request owns the state
    apply(d)
  }
}
