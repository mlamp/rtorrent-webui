<script lang="ts">
  import { detail, type DetailTab } from '$lib/stores/detail.svelte'
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { PeerInfo } from '$lib/types/detail'
  import { api } from '$lib/api/client'
  import { short, ratio } from '$lib/format'
  import { FRAMES, frameSeries, type Frame } from '$lib/series'
  import { rollingHistory } from '$lib/history.svelte'
  import PieceMap from './PieceMap.svelte'
  import CountryFlag from './CountryFlag.svelte'
  import SpeedGraph from '../SpeedGraph.svelte'

  // Fixed height so the list's virtualization math stays exact (must match
  // TorrentTable's DETAIL_H). In a modal we fill the host instead.
  export const DETAIL_H = 472

  let { t, inModal = false }: { t: TorrentRow; inModal?: boolean } = $props()

  const paused = $derived(t.status === 'stopped' || t.status === 'paused')
  const pieceCount = $derived(Math.min(420, Math.max(60, Math.round(t.size / (1 << 20)))))
  const have = $derived(Math.round(t.done * pieceCount))

  // ── activity graph ─────────────────────────────────────────────────────────
  let frame = $state<Frame>('15m')
  const baseDl = $derived(Math.max(t.downRate * 1.4, 1.5 * 1024 * 1024))
  const baseUl = $derived(Math.max(t.upRate * 1.4, 400 * 1024))
  // live 15m buffer — seeded from the REAL current rate so it reads full on open
  // (idle torrents stay flat); live samples then scroll in each tick.
  const live = rollingHistory(
    () => ({ down: t.downRate, up: t.upRate }),
    44,
    () => frameSeries(t.hash, '15m', t.downRate, t.upRate),
  )
  const series = $derived(
    frame === '15m' ? { dl: live.dl, ul: live.ul } : frameSeries(t.hash, frame, baseDl, baseUl),
  )

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

  function peerFlags(p: PeerInfo): string {
    return `${p.downRate > 0 ? 'D' : '·'}${p.upRate > 0 ? 'U' : '·'}${p.encrypted ? 'E' : '·'}${p.incoming ? 'I' : 'O'}`
  }

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
        <span class="rd-key">hash</span><span class="truncate font-mono">{t.hash.slice(0, 24)}…</span>
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
      <div class="rd-stat"><div class="rd-stat-l">tracker</div><div class="rd-stat-v">{t.tracker || '—'}</div></div>
    </div>

    <!-- activity graph + timeframe selector (handover §4) -->
    <div class="rd-activity">
      <div class="rd-act-head">
        <span class="rd-act-rates">
          <span class="d">↓ {short(t.downRate)}<small>B/s</small></span>
          <span class="u">↑ {short(t.upRate)}<small>B/s</small></span>
        </span>
        <div class="rd-frames">
          {#each FRAMES as f (f)}
            <button class="rd-frame {frame === f ? 'on' : ''}" onclick={() => (frame = f)}>{f}</button>
          {/each}
        </div>
      </div>
      <SpeedGraph dl={series.dl} ul={series.ul} h={84} dlColor="var(--status-download)" ulColor="var(--status-seed)" glow grid strokeW={1.7} />
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
        <PieceMap done={t.done} count={pieceCount} downloading={t.status === 'downloading'} />
        <div class="rd-legend">
          <span><i style="background:var(--primary)"></i> have · {have}</span>
          <span><i style="background:var(--warn)"></i> downloading</span>
          <span><i style="background:color-mix(in srgb,var(--primary) 14%,transparent)"></i> missing · {pieceCount - have}</span>
          <span class="ml-auto font-mono text-dim">{pieceCount} pieces · {short(t.size / pieceCount)}B each</span>
        </div>
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
              <span class="truncate font-mono">{p.address}</span>
              <span class="truncate text-dim2">{p.client}</span>
              <span class="rd-flags font-mono">{peerFlags(p)}</span>
              <span class="text-right font-mono" style="color:{p.downRate ? 'var(--primary)' : 'var(--dim)'}">{p.downRate ? short(p.downRate) : '—'}</span>
              <span class="text-right font-mono" style="color:{p.upRate ? 'var(--acc2)' : 'var(--dim)'}">{p.upRate ? short(p.upRate) : '—'}</span>
              <span class="text-right font-mono">{p.progress}%</span>
            </div>
          {/each}
        </div>
      {/if}
    {:else if detail.tab === 'trackers'}
      {#if detail.trackers.length === 0}
        <p class="text-dim">{detail.loading ? 'loading…' : 'no trackers'}</p>
      {:else}
        <div class="rd-trk">
          {#each detail.trackers as tr (tr.index)}
            <div class="rd-trkrow">
              <span class="rd-dot {tr.enabled ? 'ok' : 'idle'}"></span>
              <span class="rd-trkname truncate font-mono" title={tr.url}>{tr.url}</span>
              <span class="text-dim2">{tr.latestEvent || (tr.enabled ? 'working' : 'disabled')}</span>
              <span class="font-mono text-dim">announces {tr.success}</span>
              <button class="rd-btn ml-auto" onclick={() => detail.toggleTracker(tr.index, !tr.enabled)}>{tr.enabled ? 'disable' : 'enable'}</button>
            </div>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
</section>
