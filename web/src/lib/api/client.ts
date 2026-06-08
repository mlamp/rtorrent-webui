import { toast } from 'svelte-sonner'

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

export const api = {
  start: (h: string) => req('POST', `/api/torrents/${h}/start`),
  stop: (h: string) => req('POST', `/api/torrents/${h}/stop`),
  pause: (h: string) => req('POST', `/api/torrents/${h}/pause`),
  recheck: (h: string) => req('POST', `/api/torrents/${h}/recheck`),
  announce: (h: string) => req('POST', `/api/torrents/${h}/announce`),
  remove: (h: string) => req('DELETE', `/api/torrents/${h}`),
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
