<script lang="ts" module>
  // Fixed height so the list's virtualization math stays exact — TorrentTable
  // imports this. In a modal we fill the host instead.
  export const DETAIL_H = 560
</script>

<script lang="ts">
  import { detail, type DetailTab } from '$lib/stores/detail.svelte'
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import { api, silentGet } from '$lib/api/client'
  import { short, ratio, relativeTime } from '$lib/format'
  import { onMount } from 'svelte'
  import PieceMap from './PieceMap.svelte'
  import CountryFlag from './CountryFlag.svelte'
  import TrafficChart from '../charts/TrafficChart.svelte'
  import { decodePieces } from '$lib/pieces'
  import { peerFlags } from '$lib/peers'

  let { t, inModal = false }: { t: TorrentRow; inModal?: boolean } = $props()

  const paused = $derived(t.status === 'stopped' || t.status === 'paused')
  // Piece counts: prefer the /pieces fetch's own counts so the map and the legend
  // always agree (same snapshot); fall back to the live SSE chunk fields until that
  // fetch lands.
  const pcs = $derived({
    size: detail.pieces?.sizeChunks ?? t.sizeChunks,
    completed: detail.pieces?.completedChunks ?? t.completedChunks,
    chunk: detail.pieces?.chunkSize ?? t.chunkSize,
  })
  // Real piece view from the fetched bitfield; falls back to a % bar when the
  // bitfield isn't available (no metadata yet / partial without a bitfield).
  const pieceView = $derived(decodePieces(detail.pieces?.bitfield ?? '', pcs.size, pcs.completed))

  // ── activity graph (real per-torrent history) ───────────────────────────────
  // Throughput history comes from /api/history?hash=… — cumulative byte counters
  // persisted per torrent in SQLite and derived to rates server-side. (Earlier
  // builds drew a synthetic series here because the store kept no per-infohash
  // data; it does now, so this is the real thing.)
  type Point = { t: number; down: number; up: number }
  const RANGES = [
    { key: '15m', secs: 900 },
    { key: '1h', secs: 3600 },
    { key: '6h', secs: 21600 },
    { key: '24h', secs: 86400 },
    { key: '7d', secs: 604800 },
    { key: '1y', secs: 31536000 },
  ] as const

  let range = $state('1h')
  let points = $state<Point[]>([])
  let firstTS = $state(0) // earliest sample we hold for this torrent (0 = unknown)
  let now = $state(Date.now()) // re-stamped each poll so `enabled` re-evaluates as the torrent ages

  // Offer only the ranges that hold data: every range shorter than the available
  // span, plus the first one that fully covers it. A day-old torrent thus shows up
  // to 24h (with 7d as "all") and hides 1y, instead of plotting a year of nothing.
  // While availability is unknown (firstTS 0) keep every range clickable, so the
  // default selection never renders as a disabled-but-active button.
  const enabled = $derived.by(() => {
    if (firstTS <= 0) return new Set(RANGES.map((r) => r.key))
    const set = new Set<string>()
    const span = now / 1000 - firstTS
    for (const r of RANGES) {
      set.add(r.key)
      if (r.secs > span) break // covers the whole span; longer ranges add nothing
    }
    return set
  })

  // Guard against out-of-order responses: a slow fetch for a range/torrent the user
  // has since left must not overwrite the current one — only the latest wins.
  let reqSeq = 0
  async function loadHistory() {
    const seq = ++reqSeq
    const wantRange = range,
      wantHash = t.hash
    now = Date.now()
    const d = await silentGet<{ points: Point[]; first: number }>(
      `/api/history?range=${wantRange}&hash=${wantHash}`,
    )
    if (seq !== reqSeq || wantRange !== range || wantHash !== t.hash || !d) return // stale
    points = d.points ?? []
    if (d.first) firstTS = d.first
  }

  // refetch on open, on range change, and when the modal is reused for another torrent;
  // clear stale state on a torrent switch so the chart/buttons don't flash old data.
  let lastHash = ''
  $effect(() => {
    range
    if (t.hash !== lastHash) {
      lastHash = t.hash
      firstTS = 0
      points = []
    }
    loadHistory()
  })
  // once availability is known, clamp the selection to a range that actually has data
  $effect(() => {
    if (firstTS > 0 && !enabled.has(range)) {
      const last = RANGES.filter((r) => enabled.has(r.key)).at(-1)
      if (last && last.key !== range) range = last.key
    }
  })
  onMount(() => {
    const id = setInterval(() => {
      loadHistory()
      detail.loadPieces() // keep the PIECES map live (no-op unless that tab is open)
    }, 3000)
    return () => clearInterval(id)
  })

  const tabs: { key: DetailTab; label: string }[] = [
    { key: 'general', label: 'PIECES' },
    { key: 'files', label: 'FILES' },
    { key: 'peers', label: 'PEERS' },
    { key: 'trackers', label: 'TRACKERS' },
  ]
  const PRIOS = [
    { v: 0, label: 'skip' },
    { v: 1, label: 'norm' },
    { v: 2, label: 'high' },
  ]


  async function act(a: 'pause' | 'resume' | 'recheck' | 'remove') {
    try {
      if (a === 'pause') await api.stop(t.hash)
      else if (a === 'resume') await api.start(t.hash)
      else if (a === 'recheck') await api.recheck(t.hash)
      else if (a === 'remove') {
        await api.remove(t.hash)
        detail.close()
      }
    } catch {
      /* toast shown */
    }
  }
</script>

<section
  class="detail-in flex flex-col overflow-hidden"
  style="height:{inModal ? '100%' : DETAIL_H + 'px'}; border-top:1px solid var(--line); background:linear-gradient(180deg, color-mix(in srgb, var(--primary) 4%, transparent), transparent 120px); box-shadow:inset 2px 0 0 var(--acc2)"
>
  <div class="shrink-0 px-5 pt-4">
    <div class="rd-head">
      <div class="flex min-w-0 items-center gap-2 text-[12px] text-dim2">
        <span class="rd-key">hash</span><span class="shrink-0 font-mono">{t.hash.slice(0, 16)}…</span>
        {#if t.directory}
          <span class="rd-key ml-2">path</span><span class="truncate font-mono" title={t.directory}>{t.directory}</span>
        {/if}
      </div>
      <div class="flex items-center gap-2">
        <button class="rd-btn" onclick={() => act(paused ? 'resume' : 'pause')}>{paused ? '▶ RESUME' : '⏸ PAUSE'}</button>
        <button class="rd-btn" onclick={() => act('recheck')}>⟳ RECHECK</button>
        <button class="rd-btn danger" onclick={() => act('remove')}>✕ REMOVE</button>
        <button class="rd-btn" onclick={() => detail.close()} aria-label="close">✕</button>
      </div>
    </div>

    <div class="rd-strip">
      <div class="rd-stat"><div class="rd-stat-l">size</div><div class="rd-stat-v">{short(t.size)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">done</div><div class="rd-stat-v text-primary">{Math.round(t.done * 100)}%</div></div>
      <div class="rd-stat"><div class="rd-stat-l">downloaded</div><div class="rd-stat-v">{short(t.completed)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">uploaded</div><div class="rd-stat-v text-acc2">{short(t.upTotal)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">ratio</div><div class="rd-stat-v text-acc2">{ratio(t.ratio)}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">peers</div><div class="rd-stat-v">{t.peersConnected}/{t.seedsConnected}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">status</div><div class="rd-stat-v">{t.status}</div></div>
      <!-- "created": d.creation_date is the metainfo creation stamp, not the add
           time (rtorrent's d.load_date re-stamps on every restart, so there is no
           stable added-at) — label it for what it is -->
      <div class="rd-stat"><div class="rd-stat-l">created</div><div class="rd-stat-v">{relativeTime(t.added)}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">tracker</div><div class="rd-stat-v" title={t.tracker}>{t.tracker || '—'}</div></div>
    </div>

    <!-- activity graph + timeframe selector — real per-torrent history -->
    <div class="rd-activity">
      <div class="rd-act-head">
        <span class="rd-act-rates">
          <span class="d">↓ {short(t.downRate)}<small>B/s</small></span>
          <span class="u">↑ {short(t.upRate)}<small>B/s</small></span>
        </span>
        <div class="rd-frames">
          {#each RANGES as r (r.key)}
            <button
              class="rd-frame {range === r.key ? 'on' : ''}"
              disabled={!enabled.has(r.key)}
              title={enabled.has(r.key) ? '' : 'no history for this range yet'}
              onclick={() => (range = r.key)}
            >{r.key}</button>
          {/each}
        </div>
      </div>
      <TrafficChart {points} height={150} dlColor="var(--status-download)" ulColor="var(--status-seed)" />
    </div>

    <div class="rd-tabs">
      {#each tabs as tab (tab.key)}
        <button class="rd-tab {detail.tab === tab.key ? 'on' : ''}" onclick={() => detail.setTab(tab.key)}>{tab.label}</button>
      {/each}
    </div>
  </div>

  <div class="min-h-0 flex-1 overflow-auto px-5 pb-4">
    {#if detail.tab === 'general'}
      <div class="flex flex-col gap-3">
        {#if pieceView.mode === 'cells'}
          <PieceMap cells={pieceView.cells} />
        {:else}
          <!-- no per-piece bitfield available — show the real done% bar, not a fake grid -->
          <div class="h-2.5 w-full overflow-hidden rounded-sm" style="background:color-mix(in srgb,var(--primary) 12%,transparent)">
            <div class="h-full" style="width:{Math.round(t.done * 100)}%; background:var(--primary)"></div>
          </div>
        {/if}
        {#if pcs.size > 0}
          <div class="rd-legend">
            <span><i style="background:var(--primary)"></i> have · {pcs.completed}</span>
            <span><i style="background:color-mix(in srgb,var(--primary) 14%,transparent)"></i> missing · {pcs.size - pcs.completed}</span>
            <span class="ml-auto font-mono text-dim">{pcs.size} pieces · {short(pcs.chunk)}B each</span>
          </div>
        {:else}
          <div class="rd-legend"><span class="font-mono text-dim">// piece data unavailable (no metadata yet)</span></div>
        {/if}
      </div>
    {:else if detail.tab === 'files'}
      {#if detail.loading && detail.files.length === 0}
        <p class="text-dim">loading…</p>
      {:else}
        <div class="flex flex-col gap-px">
          {#each detail.files as f (f.index)}
            {@const base = f.path.split('/').pop() ?? f.path}
            {@const dir = f.path.includes('/') ? f.path.slice(0, f.path.length - base.length) : ''}
            <div class="rd-file grid items-center gap-3 rounded-sm px-2 py-1.5 text-[12px] hover:bg-[color-mix(in_srgb,var(--primary)_4%,transparent)]" style="grid-template-columns:18px 1fr 110px 38px 58px auto">
              <span class="text-center text-primary">{f.path.match(/\.(mkv|mp4|avi)$/i) ? '▦' : f.path.match(/\.(srt|nfo|txt)$/i) ? '✎' : '·'}</span>
              <span class="truncate" title={f.path}><span class="text-dim">{dir}</span><span class="text-foreground">{base}</span></span>
              <span class="rd-fbar"><i style="width:{f.done * 100}%"></i></span>
              <span class="text-right text-[11px] text-primary">{Math.round(f.done * 100)}%</span>
              <span class="text-right text-[11px] text-dim2">{short(f.size)}B</span>
              <span class="flex justify-end gap-0.5">
                {#each PRIOS as p (p.v)}
                  <button class="rd-prio {f.priority === p.v ? (p.v === 0 ? 'skip' : 'on') : ''}" onclick={() => detail.setFilePriority(f.index, p.v)}>{p.label}</button>
                {/each}
              </span>
            </div>
          {/each}
          {#if detail.files.length === 0}<p class="text-dim">no files</p>{/if}
        </div>
      {/if}
    {:else if detail.tab === 'peers'}
      {#if detail.peers.length === 0}
        <p class="text-dim">{detail.loading ? 'loading…' : 'no connected peers'}</p>
      {:else}
        <div class="flex flex-col gap-px">
          <div class="grid items-center gap-3 px-2 pb-1.5 text-[9.5px] uppercase tracking-[0.1em] text-dim" style="grid-template-columns:32px 130px 1fr 70px 64px 64px 52px">
            <span>cc</span><span>address</span><span>client</span><span>flags</span><span class="text-right">↓</span><span class="text-right">↑</span><span class="text-right">done</span>
          </div>
          {#each detail.peers as p (p.address + ':' + p.port)}
            <div class="grid items-center gap-3 rounded-sm px-2 py-1 text-[11.5px] hover:bg-[color-mix(in_srgb,var(--primary)_4%,transparent)]" style="grid-template-columns:32px 130px 1fr 70px 64px 64px 52px">
              <span class="rd-cc"><CountryFlag code={p.country} /></span>
              <span class="truncate font-mono" title={p.address}>{p.address}</span>
              <span class="truncate text-dim2" title={p.client}>{p.client}</span>
              <span class="rd-flags font-mono">{peerFlags(p)}</span>
              <span class="text-right font-mono" style="color:{p.downRate ? 'var(--primary)' : 'var(--dim)'}">{p.downRate ? short(p.downRate) : '—'}</span>
              <span class="text-right font-mono" style="color:{p.upRate ? 'var(--acc2)' : 'var(--dim)'}">{p.upRate ? short(p.upRate) : '—'}</span>
              <span class="text-right font-mono">{p.progress}%</span>
            </div>
          {/each}
        </div>
      {/if}
    {:else if detail.tab === 'trackers'}
      <!-- the torrent-wide d.message is the only place rtorrent keeps the failure
           TEXT (per-tracker state is counters only) — surface it here next to the
           per-tracker rows that say which tracker it came from -->
      {#if t.message}
        <div class="mb-2 truncate font-mono text-[11px]" style="color:{t.status === 'error' ? 'var(--status-error)' : 'var(--status-check)'}" title={t.message}>{t.message}</div>
      {/if}
      {#if detail.trackers.length === 0}
        <p class="text-dim">{detail.loading ? 'loading…' : 'no trackers'}</p>
      {:else}
        <div class="rd-trk">
          {#each detail.trackers as tr (tr.index)}
            <!-- failing = the last announce attempt failed (rtorrent has no per-tracker
                 error text, only counters/timestamps — the message itself lives in the
                 torrent-wide d.message) -->
            {@const failing = tr.enabled && tr.failed > 0 && tr.failedAt >= tr.successAt}
            <div class="rd-trkrow">
              <span class="rd-dot {tr.enabled ? (failing ? 'err' : 'ok') : 'idle'}"></span>
              <span class="rd-trkname truncate font-mono" title={tr.url}>{tr.url}</span>
              {#if failing}
                <span class="text-status-error" title="last failure {relativeTime(tr.failedAt)}{tr.successAt ? ` · last ok ${relativeTime(tr.successAt)}` : ' · never succeeded'}">failing</span>
              {:else}
                <span class="text-dim2">{tr.latestEvent || (tr.enabled ? 'working' : 'disabled')}</span>
              {/if}
              <span class="font-mono {failing ? 'text-status-error' : 'text-dim'}">
                ok {tr.success}{tr.failed > 0 ? ` · fail ${tr.failed}` : ''}
              </span>
              <button class="rd-btn ml-auto" onclick={() => detail.toggleTracker(tr.index, !tr.enabled)}>{tr.enabled ? 'disable' : 'enable'}</button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</section>
