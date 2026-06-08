<script lang="ts">
  import { Circle } from '@lucide/svelte'
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { Status } from '$lib/types/torrent'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import { bytes, rate, ratio, percent } from '$lib/format'

  let { t, cols }: { t: TorrentRow; cols: string } = $props()

  const STATUS_LABEL: Record<Status, string> = {
    downloading: 'Downloading',
    seeding: 'Seeding',
    stopped: 'Stopped',
    paused: 'Paused',
    hashing: 'Checking',
    error: 'Error',
  }
  const STATUS_COLOR: Record<Status, string> = {
    downloading: 'text-status-download',
    seeding: 'text-status-seed',
    stopped: 'text-status-stopped',
    paused: 'text-status-stopped',
    hashing: 'text-status-check',
    error: 'text-status-error',
  }
  function barColor(s: Status): string {
    if (s === 'downloading') return 'var(--status-download)'
    if (s === 'error') return 'var(--status-error)'
    if (s === 'hashing') return 'var(--status-check)'
    return 'var(--status-seed)'
  }
</script>

<div
  data-torrent={t.hash}
  class="grid items-center border-b border-border/60 transition-colors hover:bg-accent/50 {selection.has(t.hash) ? 'bg-primary/10' : detail.activeHash === t.hash ? 'bg-accent/40' : ''}"
  style="grid-template-columns:{cols}; height:36px"
>
  <div class="px-3">
    <input
      type="checkbox"
      class="accent-primary"
      checked={selection.has(t.hash)}
      onchange={(e) => selection.set(t.hash, e.currentTarget.checked)}
    />
  </div>
  <button class="truncate px-3 text-left font-medium hover:text-primary" title={t.name} onclick={() => detail.open(t.hash)}>{t.name}</button>
  <div class="px-3 text-right text-sm tabular-nums text-muted-foreground">{bytes(t.size)}</div>
  <div class="px-3">
    <div class="flex items-center gap-2">
      <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-secondary">
        <div class="h-full rounded-full" style="width:{t.done * 100}%; background:{barColor(t.status)}"></div>
      </div>
      <span class="w-9 text-right text-xs tabular-nums text-muted-foreground">{percent(t.done)}</span>
    </div>
  </div>
  <div class="px-3 text-right text-sm tabular-nums {t.downRate > 0 ? 'text-status-download' : 'text-muted-foreground'}">{rate(t.downRate)}</div>
  <div class="px-3 text-right text-sm tabular-nums {t.upRate > 0 ? 'text-status-seed' : 'text-muted-foreground'}">{rate(t.upRate)}</div>
  <div class="px-3 text-right text-sm tabular-nums">{ratio(t.ratio)}</div>
  <div class="px-3 text-right text-sm tabular-nums text-muted-foreground">{t.seedsConnected} / {t.peersConnected}</div>
  <div class="truncate px-3 text-sm">
    <span class="inline-flex items-center gap-1.5 font-medium {STATUS_COLOR[t.status]}">
      <Circle class="size-2 fill-current" />{STATUS_LABEL[t.status]}
    </span>
  </div>
  <div class="truncate px-3">
    {#if t.label}
      <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">{t.label}</span>
    {/if}
  </div>
</div>
