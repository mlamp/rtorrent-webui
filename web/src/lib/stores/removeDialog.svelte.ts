// Shared state for the remove/delete confirmation dialog.
//
// This store deliberately diverges from the repo's usual `bind:open` dialog
// idiom (AddTorrent/Throttle each own their own `open`): three entry points —
// the detail-modal REMOVE button, the bulk-bar REMOVE button, and the
// Delete/Backspace keys — must all drive ONE shared dialog instance that also
// carries the target list and an on-confirmed callback. A singleton store is the
// clean way to share that.
//
// The capability + config-load state are SNAPSHOTTED at request() time so a late
// /api/config response can never mutate an already-open dialog.
import { config } from './config.svelte'
import { bulkRemove } from '$lib/api/client'
import { reduceRemoveTargets, requestDeletesData, type RemoveTarget } from '$lib/removeDialog.logic'

class RemoveDialogState {
  open = $state(false)
  targets = $state<RemoveTarget[]>([])
  deleteData = $state(false)
  busy = $state(false)
  capable = $state(false)
  configState = $state<'idle' | 'loaded' | 'failed'>('idle')

  #onConfirmed: (() => void) | null = null
  #prevFocus: Element | null = null

  /** Open the dialog for a set of targets. Empty set is a no-op. */
  request(targets: RemoveTarget[], onConfirmed?: () => void) {
    const init = reduceRemoveTargets(targets)
    if (!init) return
    this.targets = targets
    this.deleteData = init.deleteData // never sticky
    this.busy = false
    this.capable = config.deleteWithData // snapshot — a late /api/config can't change an open dialog
    this.configState = config.configState
    this.#onConfirmed = onConfirmed ?? null
    this.#prevFocus = typeof document !== 'undefined' ? document.activeElement : null
    this.open = true
  }

  cancel() {
    if (this.busy) return // dismissal is locked while a removal is in flight
    this.open = false // teardown runs via the component's close effect
  }

  async confirm() {
    if (this.busy) return
    this.busy = true
    const hashes = this.targets.map((t) => t.hash)
    const data = requestDeletesData(this.capable, this.deleteData)
    let erased = 0
    try {
      erased = await bulkRemove(hashes, data)
    } finally {
      this.busy = false
    }
    const onConfirmed = this.#onConfirmed
    this.open = false // always close — the user reads the outcome from the toasts
    // success-only side effect (close the detail modal / clear the selection):
    // run ONLY when at least one torrent was actually removed. erased>0 covers
    // already-gone + empty-base_path + partial success; a total failure (which
    // only raised error toasts) leaves the detail modal / selection intact.
    if (erased > 0) onConfirmed?.()
  }

  /**
   * Invoked by the component whenever the dialog transitions open→closed —
   * covers cancel(), confirm(), AND Modal's own backdrop/✕ which write `open`
   * directly. Resets transient state and restores focus to where the user was,
   * unless that node was destroyed (e.g. the detail modal closed on remove).
   */
  afterClose() {
    this.busy = false
    const prev = this.#prevFocus
    this.#prevFocus = null
    queueMicrotask(() => {
      if (prev instanceof HTMLElement && prev.isConnected) prev.focus()
    })
  }
}

export const removeDialog = new RemoveDialogState()
