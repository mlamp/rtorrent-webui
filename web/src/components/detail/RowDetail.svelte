<script lang="ts">
  import { detail, type DetailTab } from '$lib/stores/detail.svelte'
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import { api } from '$lib/api/client'
  import { short, ratio } from '$lib/format'
  import PieceMap from './PieceMap.svelte'
  import CountryFlag from './CountryFlag.svelte'

  // Fixed height so the list's virtualization math stays exact.
  export const DETAIL_H = 340

  let { t }: { t: TorrentRow } = $props()

  const paused = $derived(t.status === 'stopped' || t.status === 'paused')
  const pieceCount = $derived(Math.min(420, Math.max(60, Math.round(t.size / (1 << 20)))))

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
  class="detail-in flex flex-col overflow-hidden border-b border-line"
  style="height:{DETAIL_H}px; background:color-mix(in srgb, var(--primary) 4%, transparent); box-shadow:inset 2px 0 0 var(--acc2)"
>
  <div class="flex shrink-0 items-center justify-between gap-4 px-5 pt-3">
    <div class="flex items-center gap-2 text-[12px] text-dim2">
      <span class="rd-key">hash</span><span class="truncate">{t.hash.slice(0, 28)}…</span>
    </div>
    <div class="flex items-center gap-2">
      <button class="rd-btn" onclick={() => act(paused ? 'resume' : 'pause')}>{paused ? '▶ RESUME' : '⏸ PAUSE'}</button>
      <button class="rd-btn" onclick={() => act('recheck')}>⟳ RECHECK</button>
      <button class="rd-btn danger" onclick={() => act('remove')}>✕ REMOVE</button>
      <button class="rd-btn" onclick={() => detail.close()} aria-label="close">✕</button>
    </div>
  </div>

  <div class="flex min-h-0 flex-1 flex-col px-5 pb-3 pt-3">
    <div class="rd-strip">
      <div class="rd-stat"><div class="rd-stat-l">size</div><div class="rd-stat-v">{short(t.size)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">done</div><div class="rd-stat-v text-primary">{Math.round(t.done * 100)}%</div></div>
      <div class="rd-stat"><div class="rd-stat-l">downloaded</div><div class="rd-stat-v">{short(t.completed)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">uploaded</div><div class="rd-stat-v text-acc2">{short(t.upTotal)}B</div></div>
      <div class="rd-stat"><div class="rd-stat-l">ratio</div><div class="rd-stat-v text-acc2">{ratio(t.ratio)}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">peers</div><div class="rd-stat-v">{t.peersConnected}/{t.peersTotal}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">status</div><div class="rd-stat-v">{t.status}</div></div>
      <div class="rd-stat"><div class="rd-stat-l">tracker</div><div class="rd-stat-v">{t.tracker || '—'}</div></div>
    </div>

    <div class="rd-tabs">
      {#each tabs as tab (tab.key)}
        <button class="rd-tab {detail.tab === tab.key ? 'on' : ''}" onclick={() => detail.setTab(tab.key)}>{tab.label}</button>
      {/each}
    </div>

    <div class="min-h-0 flex-1 overflow-auto">
      {#if detail.tab === 'general'}
        <div class="flex flex-col gap-3">
          <PieceMap done={t.done} count={pieceCount} downloading={t.status === 'downloading'} />
          <div class="flex items-center gap-5 text-[11px] text-dim2">
            <span class="flex items-center gap-1.5"><i class="size-2.5 rounded-sm" style="background:var(--primary)"></i> have</span>
            <span class="flex items-center gap-1.5"><i class="size-2.5 rounded-sm" style="background:var(--warn)"></i> downloading</span>
            <span class="flex items-center gap-1.5"><i class="size-2.5 rounded-sm" style="background:color-mix(in srgb,var(--primary) 15%,transparent)"></i> missing</span>
            <span class="ml-auto text-dim">~{pieceCount} pieces · approx</span>
          </div>
        </div>
      {:else if detail.tab === 'files'}
        {#if detail.loading && detail.files.length === 0}
          <p class="text-dim">loading…</p>
        {:else}
          <div class="flex flex-col gap-px">
            {#each detail.files as f (f.index)}
              <div class="rd-row" style="grid-template-columns:18px 1fr 120px 44px 64px auto">
                <span class="text-center text-primary">{f.path.match(/\.(mkv|mp4|avi)$/i) ? '▦' : f.path.match(/\.(srt|nfo|txt)$/i) ? '✎' : '·'}</span>
                <span class="truncate text-foreground" title={f.path}>{f.path}</span>
                <span class="rd-bar"><i style="width:{f.done * 100}%"></i></span>
                <span class="text-right text-[11px] text-primary">{Math.round(f.done * 100)}%</span>
                <span class="text-right text-[11px] text-dim2">{short(f.size)}B</span>
                <span class="flex gap-0.5">
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
            <div class="rd-row text-[9.5px] uppercase tracking-[0.1em] text-dim" style="grid-template-columns:32px 150px 1fr 70px 70px 56px">
              <span>cc</span><span>address</span><span>client</span><span class="text-right">↓</span><span class="text-right">↑</span><span class="text-right">done</span>
            </div>
            {#each detail.peers as p (p.address + ':' + p.port)}
              <div class="rd-row text-[11.5px]" style="grid-template-columns:32px 150px 1fr 70px 70px 56px">
                <span class="rd-cc"><CountryFlag code={p.country} /></span>
                <span class="truncate">{p.address}</span>
                <span class="truncate text-dim2">{p.client}</span>
                <span class="text-right" style="color:{p.downRate ? 'var(--primary)' : 'var(--dim)'}">{p.downRate ? short(p.downRate) : '—'}</span>
                <span class="text-right" style="color:{p.upRate ? 'var(--acc2)' : 'var(--dim)'}">{p.upRate ? short(p.upRate) : '—'}</span>
                <span class="text-right">{p.progress}%</span>
              </div>
            {/each}
          </div>
        {/if}
      {:else if detail.tab === 'trackers'}
        {#if detail.trackers.length === 0}
          <p class="text-dim">{detail.loading ? 'loading…' : 'no trackers'}</p>
        {:else}
          <div class="flex flex-col gap-px">
            {#each detail.trackers as tr (tr.index)}
              <div class="flex items-center gap-3.5 rounded-sm p-2 text-[12px] hover:bg-[color-mix(in_srgb,var(--primary)_4%,transparent)]">
                <span class="rd-dot {tr.enabled ? 'ok' : 'idle'}"></span>
                <span class="min-w-0 flex-1 truncate text-foreground" title={tr.url}>{tr.url}</span>
                <span class="text-dim2">{tr.enabled ? 'enabled' : 'disabled'}</span>
                <span class="text-dim">success {tr.success}</span>
                <button class="rd-btn" onclick={() => detail.toggleTracker(tr.index, !tr.enabled)}>{tr.enabled ? 'disable' : 'enable'}</button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </div>
</section>
