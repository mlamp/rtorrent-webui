<script lang="ts">
  import { ChevronUp, ChevronDown } from '@lucide/svelte'
  import TorrentRow from './TorrentRow.svelte'
  import RowDetail from '../detail/RowDetail.svelte'
  import { view, type ColumnKey } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import type { TorrentRow as TRow } from '$lib/stores/torrents.svelte'

  let { rows }: { rows: TRow[] } = $props()

  // shared grid template: select · name · progress · rate · size · ratio · eta
  const COLS = '24px minmax(0,1fr) 200px 92px 64px 52px 60px'
  const ROW_H = 40
  const DETAIL_H = 340 // matches RowDetail's fixed height — keeps windowing exact
  const OVERSCAN = 6

  let scrollTop = $state(0)
  let viewportH = $state(600)
  let viewport: HTMLDivElement

  const total = $derived(rows.length)
  // index of the expanded torrent within the current (filtered/sorted) rows
  const openIdx = $derived(
    detail.activeHash ? rows.findIndex((t) => t.hash === detail.activeHash) : -1,
  )
  const totalH = $derived(total * ROW_H + (openIdx >= 0 ? DETAIL_H : 0))

  // y -> row index, accounting for the one expanded row's extra height
  function rowIndexAtY(y: number): number {
    if (openIdx < 0) return Math.floor(y / ROW_H)
    const detailStart = (openIdx + 1) * ROW_H
    if (y < detailStart) return Math.floor(y / ROW_H)
    if (y < detailStart + DETAIL_H) return openIdx
    return openIdx + 1 + Math.floor((y - detailStart - DETAIL_H) / ROW_H)
  }
  const rowTop = (i: number) => i * ROW_H + (openIdx >= 0 && i > openIdx ? DETAIL_H : 0)

  const start = $derived(Math.max(0, rowIndexAtY(scrollTop) - OVERSCAN))
  const end = $derived(Math.min(total, rowIndexAtY(scrollTop + viewportH) + OVERSCAN + 1))
  const slice = $derived(rows.slice(start, end))
  const padTop = $derived(rowTop(start))
  const detailInWindow = $derived(openIdx >= start && openIdx < end)
  const renderedH = $derived((end - start) * ROW_H + (detailInWindow ? DETAIL_H : 0))
  const padBottom = $derived(Math.max(0, totalH - padTop - renderedH))

  const allSelected = $derived(rows.length > 0 && rows.every((t) => selection.has(t.hash)))
  function toggleAll() {
    const on = !allSelected
    for (const t of rows) selection.set(t.hash, on)
  }

  const headers: { key: ColumnKey | null; label: string; right?: boolean }[] = [
    { key: null, label: '' },
    { key: 'name', label: 'NAME' },
    { key: 'done', label: 'PROGRESS' },
    { key: 'downRate', label: 'RATE' },
    { key: 'size', label: 'SIZE', right: true },
    { key: 'ratio', label: 'RATIO', right: true },
    { key: null, label: 'ETA', right: true },
  ]
</script>

<div class="flex h-full flex-col">
  <div
    class="grid shrink-0 border-b border-line text-[10.5px] uppercase tracking-[0.13em] text-dim"
    style="grid-template-columns:{COLS}"
  >
    {#each headers as h, i (i)}
      {#if i === 0}
        <div class="grid place-items-center">
          <input type="checkbox" class="accent-[var(--primary)]" checked={allSelected} onchange={toggleAll} />
        </div>
      {:else}
        <button
          class="flex items-center gap-1 px-3 py-2.5 {h.right ? 'justify-end' : ''} {h.key ? 'hover:text-foreground' : 'cursor-default'}"
          onclick={() => h.key && view.toggleSort(h.key)}
        >
          {h.label}
          {#if h.key && view.sortKey === h.key}
            {#if view.sortDir === 1}<ChevronUp class="size-3" />{:else}<ChevronDown class="size-3" />{/if}
          {/if}
        </button>
      {/if}
    {/each}
  </div>

  <div
    bind:this={viewport}
    bind:clientHeight={viewportH}
    onscroll={() => (scrollTop = viewport.scrollTop)}
    data-list
    class="min-h-0 flex-1 overflow-auto"
  >
    {#if total === 0}
      <div class="grid h-40 place-items-center text-sm text-dim">// no torrents</div>
    {:else}
      <div style="height:{padTop}px"></div>
      {#each slice as t, k (t.hash)}
        <TorrentRow {t} cols={COLS} />
        {#if start + k === openIdx}
          <RowDetail {t} />
        {/if}
      {/each}
      <div style="height:{padBottom}px"></div>
    {/if}
  </div>
</div>
