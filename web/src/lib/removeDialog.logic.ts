// Pure, framework-free helpers for the remove/delete confirmation dialog. No
// Svelte runes, no $lib aliases, no svelte-sonner — everything here must run
// under `node --experimental-strip-types` for the unit tests, so any intra-repo
// import would use an explicit `.ts` path (there are none today).

export type RemoveTarget = { hash: string; name: string }

export const headerText = (n: number): string =>
  n === 1 ? 'Remove this torrent?' : `Remove ${n} torrents?`

export const summaryText = (deleteData: boolean): string =>
  deleteData
    ? '→ Removed from rtorrent AND files are PERMANENTLY DELETED from disk.'
    : '→ Removed from rtorrent. Files are KEPT on disk.'

export const primaryLabel = (deleteData: boolean, busy: boolean): string =>
  busy ? 'REMOVING…' : deleteData ? 'DELETE FILES' : 'REMOVE'

// removeURL builds the DELETE endpoint. The URL builder lives here (not in
// client.ts) so it stays unit-testable without dragging in svelte-sonner.
export const removeURL = (hash: string, data: boolean): string =>
  `/api/torrents/${hash}${data ? '?data=true' : ''}`

// reduceRemoveTargets decides the initial dialog state for a remove request: an
// empty target list is ignored (null), and deleteData ALWAYS starts OFF so the
// destructive option is never sticky between opens. The store calls this so the
// empty-guard + reset are the same code these tests pin.
export function reduceRemoveTargets(targets: RemoveTarget[]): { deleteData: false } | null {
  return targets.length ? { deleteData: false } : null
}

// requestDeletesData is the single source of truth for "will this request delete
// files": only when the server allows it AND the user checked the box. Used by
// both the confirm action and the dialog's armed styling so they never diverge.
export function requestDeletesData(capable: boolean, deleteData: boolean): boolean {
  return capable && deleteData
}

// noCheckboxNote is the explanatory line shown in place of the checkbox when the
// server has not advertised the delete capability — distinct copy per config
// state so it never lies about WHY deletion is unavailable.
export function noCheckboxNote(configState: 'idle' | 'loaded' | 'failed'): string {
  switch (configState) {
    case 'loaded':
      return "// file deletion is disabled by this server's config (features.delete_with_data)"
    case 'failed':
      return '// server capabilities unknown (config unavailable)'
    default:
      return '// checking server capabilities…'
  }
}

// namesPreview caps the listed rows so a huge bulk selection can't blow out the
// dialog: the first `max` targets plus a "+K more" remainder. Returns the full
// target objects (not just names) so the view can key the list on the unique
// hash — torrent display names are NOT unique (cross-seed / re-added releases),
// and keying an {#each} on a duplicate name throws at render time.
export function namesPreview(targets: RemoveTarget[], max = 8): { shown: RemoveTarget[]; more: number } {
  return { shown: targets.slice(0, max), more: Math.max(0, targets.length - max) }
}

export type RemoveOutcome =
  | { status: 'fulfilled'; erased: boolean; dataDeleted: boolean }
  | { status: 'rejected' }

// summarizeRemoval turns the per-hash server outcomes into one success toast,
// counting only what the SERVER reported — it never claims a deletion that did
// not happen, never over/under-counts. Returns null when nothing was erased but
// something failed (those calls already raised their own error toasts), and a
// neutral line when nothing happened at all.
export function summarizeRemoval(outcomes: RemoveOutcome[]): string | null {
  const fulfilled = outcomes.filter(
    (o): o is Extract<RemoveOutcome, { status: 'fulfilled' }> => o.status === 'fulfilled',
  )
  const erased = fulfilled.filter((o) => o.erased).length
  const deleted = fulfilled.filter((o) => o.dataDeleted).length
  if (erased === 0) {
    return outcomes.some((o) => o.status === 'rejected') ? null : 'No torrents were removed'
  }
  let msg = `Removed ${erased} torrent${erased > 1 ? 's' : ''}`
  if (deleted > 0) msg += ` · deleted files from ${deleted}`
  return msg
}
