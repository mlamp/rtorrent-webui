import { toast } from 'svelte-sonner'
import { removeURL, summarizeRemoval, type RemoveOutcome } from '$lib/removeDialog.logic'

async function req(method: string, url: string, opts: RequestInit = {}): Promise<any> {
  let res: Response
  try {
    res = await fetch(url, { method, ...opts })
  } catch {
    toast.error('Network error')
    throw new Error('network')
  }
  const j = await res.json().catch(() => ({}))
  if (!res.ok || j?.ok === false) {
    const msg = j?.error?.message || res.statusText || 'Request failed'
    toast.error(msg)
    throw new Error(msg)
  }
  return j?.data
}

function jsonBody(body: unknown): RequestInit {
  return { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

/**
 * Silent GET for pollers (history/diskspace): returns the unwrapped `data`
 * payload, or null on any network/parse error. Unlike `req`, it never raises a
 * toast — a transient blip on a 3s poll must not spam the user.
 */
export async function silentGet<T>(url: string): Promise<T | null> {
  try {
    const j = await (await fetch(url)).json()
    return (j?.data ?? null) as T | null
  } catch {
    return null
  }
}

export type FsEntry = { name: string; path: string }
export type FsListing = { path: string; roots: FsEntry[]; entries: FsEntry[]; truncated: boolean }

/**
 * Browse a server directory listing (GET /api/fs), confined server-side to the
 * configured download roots. SILENT: returns null on any error or abort — the
 * save-to combobox treats "no listing" as "no suggestions", never an error
 * toast (a 404 while mid-typing a path is normal). Omit `path` for the roots.
 * Pass an AbortSignal so a superseded keystroke's request can be cancelled.
 */
export async function browseDir(path?: string, signal?: AbortSignal): Promise<FsListing | null> {
  const qs = path ? '?' + new URLSearchParams({ path }).toString() : ''
  try {
    const res = await fetch('/api/fs' + qs, { signal })
    const j = await res.json().catch(() => ({}))
    if (!res.ok || j?.ok === false) return null
    return (j?.data ?? null) as FsListing | null
  } catch {
    return null // network error or AbortError — both mean "no suggestions"
  }
}

export const api = {
  browse: browseDir,
  start: (h: string) => req('POST', `/api/torrents/${h}/start`),
  stop: (h: string) => req('POST', `/api/torrents/${h}/stop`),
  pause: (h: string) => req('POST', `/api/torrents/${h}/pause`),
  recheck: (h: string) => req('POST', `/api/torrents/${h}/recheck`),
  announce: (h: string) => req('POST', `/api/torrents/${h}/announce`),
  remove: (h: string, data = false): Promise<{ erased: boolean; dataDeleted: boolean }> =>
    req('DELETE', removeURL(h, data)),
  setLabel: (h: string, label: string) => req('PUT', `/api/torrents/${h}/label`, jsonBody({ label })),
  setPriority: (h: string, priority: number) =>
    req('PUT', `/api/torrents/${h}/priority`, jsonBody({ priority })),
  setThrottle: (down: number, up: number) => req('PUT', '/api/throttle', jsonBody({ down, up })),
  addMagnet: (magnet: string, label?: string, start?: boolean, dir?: string) =>
    req('POST', '/api/torrents', jsonBody({ magnet, label, start, directory: dir || undefined })),
  addURL: (url: string, label?: string, start?: boolean, dir?: string) =>
    req('POST', '/api/torrents', jsonBody({ url, label, start, directory: dir || undefined })),
  addFile: (file: File, label?: string, start?: boolean, dir?: string) => {
    const fd = new FormData()
    fd.append('torrent', file)
    if (label) fd.append('label', label)
    if (start) fd.append('start', 'true')
    if (dir) fd.append('directory', dir)
    return req('POST', '/api/torrents', { body: fd })
  },
  getFiles: (h: string) => req('GET', `/api/torrents/${h}/files`),
  getPeers: (h: string) => req('GET', `/api/torrents/${h}/peers`),
  getTrackers: (h: string) => req('GET', `/api/torrents/${h}/trackers`),
  getPieces: (h: string) => req('GET', `/api/torrents/${h}/pieces`),
  setFilePriority: (h: string, index: number, priority: number) =>
    req('PUT', `/api/torrents/${h}/files/${index}/priority`, jsonBody({ priority })),
  setTrackerEnabled: (h: string, index: number, enabled: boolean) =>
    req('PUT', `/api/torrents/${h}/trackers/${index}/enabled`, jsonBody({ enabled })),
}

/** Run an action across many hashes, surfacing one toast on success. */
export async function bulk(hashes: string[], fn: (h: string) => Promise<unknown>, verb: string) {
  if (hashes.length === 0) return
  const results = await Promise.allSettled(hashes.map(fn))
  const ok = results.filter((r) => r.status === 'fulfilled').length
  if (ok > 0) toast.success(`${verb} ${ok} torrent${ok > 1 ? 's' : ''}`)
}

/**
 * Remove many torrents (optionally deleting their data), surfacing one truthful
 * toast built from each hash's SERVER-reported {erased, dataDeleted} — so the
 * summary never claims a deletion that did not happen. Per-hash failures already
 * raised their own error toast via req(); they are excluded from the count.
 * Returns how many torrents were actually erased so the caller can gate its
 * success-only side effects (a total failure returns 0).
 */
export async function bulkRemove(hashes: string[], data: boolean): Promise<number> {
  if (hashes.length === 0) return 0
  const settled = await Promise.allSettled(hashes.map((h) => api.remove(h, data)))
  const outcomes: RemoveOutcome[] = settled.map((r) =>
    r.status === 'fulfilled'
      ? { status: 'fulfilled', erased: r.value?.erased ?? false, dataDeleted: r.value?.dataDeleted ?? false }
      : { status: 'rejected' },
  )
  const erased = outcomes.filter((o) => o.status === 'fulfilled' && o.erased).length
  const msg = summarizeRemoval(outcomes)
  // success toast only when something was actually removed; the neutral
  // "nothing happened" line must not wear a success checkmark.
  if (msg) {
    if (erased > 0) toast.success(msg)
    else toast(msg)
  }
  return erased
}
