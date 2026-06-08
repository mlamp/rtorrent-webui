import { torrents } from '$lib/stores/torrents.svelte'
import { globals } from '$lib/stores/globals.svelte'
import type { SnapshotMsg, DeltaMsg } from '$lib/types/torrent'

/**
 * Connect to the SSE stream and feed the stores. Native EventSource does not
 * reconnect on HTTP-error responses and offers no backoff, so we manage it:
 * close on error and reconnect with exponential backoff + jitter.
 */
export function connectSSE(url = '/api/events'): () => void {
  let es: EventSource | null = null
  let backoff = 1000
  let stopped = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const open = () => {
    es = new EventSource(url)

    es.addEventListener('open', () => {
      backoff = 1000
      globals.connection = 'live'
    })

    es.addEventListener('snapshot', (e) => {
      const d: SnapshotMsg = JSON.parse((e as MessageEvent).data)
      torrents.applySnapshot(d.torrents)
      globals.apply(d.globals)
      globals.connection = 'live'
    })

    es.addEventListener('delta', (e) => {
      const d: DeltaMsg = JSON.parse((e as MessageEvent).data)
      if (d.upserts?.length) torrents.applyUpserts(d.upserts)
      if (d.removed?.length) torrents.remove(d.removed)
      globals.apply(d.globals)
      globals.connection = 'live'
    })

    es.addEventListener('error', () => {
      es?.close()
      if (stopped) return
      globals.connection = 'reconnecting'
      const wait = backoff * (0.8 + Math.random() * 0.4)
      backoff = Math.min(backoff * 2, 30000)
      timer = setTimeout(open, wait)
    })
  }

  open()

  return () => {
    stopped = true
    if (timer) clearTimeout(timer)
    es?.close()
    globals.connection = 'offline'
  }
}
