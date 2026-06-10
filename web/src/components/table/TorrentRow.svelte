<script lang="ts">
  import { untrack } from 'svelte'
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { Status } from '$lib/types/torrent'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import { view } from '$lib/stores/view.svelte'
  import { short, ratio, eta, relativeTime } from '$lib/format'

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
  const cursor = $derived(view.cursor === t.hash)

  // rate-cell morph: how many directions are moving right now
  const dirs = $derived((t.downRate > 0 ? 1 : 0) + (t.upRate > 0 ? 1 : 0))

  // flash newly-filled segments for ~620ms when progress advances
  let lastFilled = untrack(() => filled)
  let flashFrom = $state(-1)
  let flashTo = $state(-1)
  let flashTimer: ReturnType<typeof setTimeout>
  $effect(() => {
    const f = filled
    if (f > lastFilled) {
      flashFrom = lastFilled
      flashTo = f
      clearTimeout(flashTimer)
      flashTimer = setTimeout(() => {
        flashFrom = -1
        flashTo = -1
      }, 620)
    }
    lastFilled = f
    return () => clearTimeout(flashTimer)
  })
</script>

<div
  data-torrent={t.hash}
  class="trow group relative grid cursor-pointer items-center overflow-hidden px-[18px]"
  class:open
  class:selected
  class:cursor
  class:sweep={t.sweeping}
  style="grid-template-columns:{cols}; height:46px; gap:13px; --seg-c:{SEGVAR[t.status]}"
  onclick={() => detail.open(t.hash)}
  role="button"
  tabindex="-1"
  onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && detail.open(t.hash)}
>
  {#if t.sweeping}<span class="rowsweep"></span>{/if}

  <div
    class="selcell"
    onclick={(e) => {
      e.stopPropagation()
      selection.toggle(t.hash)
    }}
    role="presentation"
  >
    <span class="chk" class:on={selected}>✓</span>
  </div>

  <div class="flex min-w-0 items-center gap-2">
    <span class="w-3 shrink-0 text-center text-[11px]" style="color:{SEGVAR[t.status]}">{MARK[t.status]}</span>
    <span class="truncate text-[12.5px]" style="color:var(--foreground)">{t.name}</span>
    {#if t.label}
      <span class="ml-1 shrink-0 rounded-sm border border-line px-1.5 text-[10px] text-acc2">{t.label}</span>
    {/if}
    <!-- tracker messages ("Tracker: […]") come from ANY tracker in the set failing
         and don't error the torrent — show them as an amber warning instead -->
    {#if t.message}
      <span class="ml-1 max-w-[45%] truncate text-[10px] {t.status === 'error' ? 'text-status-error' : 'text-status-check'}" title={t.message}>{t.message}</span>
    {/if}
  </div>

  <div class="flex items-center gap-[9px]">
    <div class="seg">
      {#each Array(SEG) as _, i (i)}
        <i
          class="sg"
          class:on={i < filled}
          class:lead={isDl && i === filled && t.done < 1}
          class:flash={i >= flashFrom && i < flashTo}
        ></i>
      {/each}
    </div>
    <span class="w-8 shrink-0 text-right text-[11.5px] text-acc">{Math.round(t.done * 100)}%</span>
  </div>

  <!-- RATE: morphs between two compact lines / one enlarged solo line / muted idle -->
  <div class="rate rate-{dirs}">
    <span class="ln d {t.downRate > 0 ? 'on' : 'off'} {dirs === 1 && t.downRate > 0 ? 'solo' : ''}">↓{short(t.downRate)}<small>B/s</small></span>
    <span class="ln u {t.upRate > 0 ? 'on' : 'off'} {dirs === 1 && t.upRate > 0 ? 'solo' : ''}">↑{short(t.upRate)}<small>B/s</small></span>
    <span class="ln idle {dirs === 0 ? 'on solo' : 'off'}">idle</span>
  </div>

  <div class="text-right text-[12.5px] text-foreground/70">{short(t.size)}<span class="ml-px text-[10px] text-dim">B</span></div>
  <div class="text-right text-[12px] text-acc2">{ratio(t.ratio)}</div>
  <div class="text-right text-[12px] text-dim">{eta(t.etaSeconds)}</div>
  <div class="text-right text-[12px] text-dim">{relativeTime(t.added)}</div>
</div>
