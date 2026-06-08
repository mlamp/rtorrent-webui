<script lang="ts">
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { Status } from '$lib/types/torrent'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import { short, ratio, eta } from '$lib/format'

  let { t, cols }: { t: TorrentRow; cols: string } = $props()

  const SEG = 26

  const MARK: Record<Status, string> = {
    downloading: '▶',
    seeding: '↑',
    stopped: '■',
    paused: '⏸',
    hashing: '⟳',
    error: '!',
  }
  const SEGVAR: Record<Status, string> = {
    downloading: 'var(--status-download)',
    seeding: 'var(--status-seed)',
    stopped: 'var(--status-stopped)',
    paused: 'var(--status-stopped)',
    hashing: 'var(--status-check)',
    error: 'var(--status-error)',
  }

  const filled = $derived(Math.floor(t.done * SEG))
  const isDl = $derived(t.status === 'downloading')
  const open = $derived(detail.activeHash === t.hash)
  const selected = $derived(selection.has(t.hash))
</script>

<div
  data-torrent={t.hash}
  class="trow group relative grid cursor-pointer items-center overflow-hidden px-3"
  class:open
  class:selected
  style="grid-template-columns:{cols}; height:40px; --seg-c:{SEGVAR[t.status]}"
  onclick={() => detail.open(t.hash)}
>
  {#if t.sweeping}<span class="rowsweep"></span>{/if}

  <div
    class="pr-1 transition-opacity group-hover:opacity-100 {selected ? 'opacity-100' : 'opacity-0'}"
    onclick={(e) => e.stopPropagation()}
    role="presentation"
  >
    <input
      type="checkbox"
      class="align-middle accent-[var(--primary)]"
      checked={selected}
      onchange={(e) => selection.set(t.hash, e.currentTarget.checked)}
    />
  </div>

  <div class="flex min-w-0 items-center gap-2">
    <span class="w-3 shrink-0 text-center text-[11px]" style="color:{SEGVAR[t.status]}">{MARK[t.status]}</span>
    <span class="shrink-0 text-[14px] leading-none transition-transform duration-200 {open ? 'rotate-90 text-primary' : 'text-dim'}">›</span>
    <span class="truncate text-[12.5px] text-foreground">{t.name}</span>
    {#if t.label}
      <span class="ml-1 shrink-0 rounded-sm border border-line px-1.5 text-[10px] text-acc2">{t.label}</span>
    {/if}
    {#if t.message && t.status === 'error'}
      <span class="ml-1 shrink-0 truncate text-[10px] text-status-error" title={t.message}>{t.message}</span>
    {/if}
  </div>

  <div class="flex items-center gap-2">
    <div class="seg">
      {#each Array(SEG) as _, i (i)}
        <i class="sg {i < filled ? 'on' : ''} {isDl && i === filled && t.done < 1 ? 'lead' : ''}"></i>
      {/each}
    </div>
    <span class="w-8 shrink-0 text-right text-[11px] text-primary">{Math.round(t.done * 100)}%</span>
  </div>

  <div class="flex flex-col text-[11.5px] leading-[1.25]">
    <span class="text-status-download {t.downRate > 0 ? 'txt-glow' : ''}">↓{t.downRate > 0 ? short(t.downRate) : '·'}</span>
    <span class="text-status-seed {t.upRate > 0 ? 'txt-glow' : ''}">↑{t.upRate > 0 ? short(t.upRate) : '·'}</span>
  </div>

  <div class="text-right text-[12px] text-dim2">{short(t.size)}<span class="text-dim">B</span></div>
  <div class="text-right text-[12px] text-acc2 txt-glow">{ratio(t.ratio)}</div>
  <div class="text-right text-[12px] text-dim">{isDl ? eta(t.etaSeconds) : '·'}</div>
</div>
