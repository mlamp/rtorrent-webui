<script lang="ts">
  import TorrentRow from './TorrentRow.svelte'
  import RowDetail from '../detail/RowDetail.svelte'
  import { view, type ColumnKey } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import type { TorrentRow as TRow } from '$lib/stores/torrents.svelte'

  let { rows }: { rows: TRow[] } = $props()

  // shared grid template: select · name · progress · rate · size · ratio · eta
  const COLS = '26px minmax(0,1fr) 150px 86px 58px 50px 50px'
  const ROW_H = 46
  const DETAIL_H = 472 // matches RowDetail's fixed height — keeps windowing exact
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
  const someSelected = $derived(selection.size > 0 && !allSelected)
  function toggleAll() {
    const on = !allSelected
    for (const t of rows) selection.set(t.hash, on)
  }

  const headers: { key: ColumnKey; label: string; right?: boolean }[] = [
    { key: 'name', label: 'NAME' },
    { key: 'done', label: 'PROGRESS' },
    { key: 'rate', label: 'RATE' },
    { key: 'size', label: 'SIZE', right: true },
    { key: 'ratio', label: 'RATIO', right: true },
    { key: 'eta', label: 'ETA', right: true },
  ]
</script>

<div class="flex h-full flex-col">
  <div
    class="grid shrink-0 items-center border-b border-line px-[18px] py-[9px] text-[10.5px] uppercase tracking-[0.13em] text-dim"
    style="grid-template-columns:{COLS}; gap:13px"
  >
    <div class="selcell" onclick={toggleAll} role="presentation">
      <span class="chk" class:on={allSelected} class:part={someSelected}>{someSelected && !allSelected ? '–' : '✓'}</span>
    </div>
    {#each headers as h (h.key)}
      <button
        class="sortable flex items-center gap-[5px] {h.right ? 'justify-end' : ''} {view.sortKey === h.key ? 'act text-acc' : 'hover:text-dim2'}"
        onclick={() => view.toggleSort(h.key)}
      >
        {h.label}
        <span class="sarrow">{view.sortKey === h.key ? (view.sortDir === 1 ? '▲' : '▼') : '▲▼'}</span>
      </button>
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
