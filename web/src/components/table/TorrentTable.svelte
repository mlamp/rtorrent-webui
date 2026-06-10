<script lang="ts">
  import TorrentRow from './TorrentRow.svelte'
  import { view, type ColumnKey } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import type { TorrentRow as TRow } from '$lib/stores/torrents.svelte'

  let { rows }: { rows: TRow[] } = $props()

  // shared grid template: select · name · progress · rate · size · ratio · eta · added
  const COLS = '26px minmax(0,1fr) 150px 86px 58px 50px 50px 70px'
  const ROW_H = 46
  const OVERSCAN = 6

  let scrollTop = $state(0)
  let viewportH = $state(600)
  let viewport: HTMLDivElement

  const total = $derived(rows.length)
  const totalH = $derived(total * ROW_H)

  const start = $derived(Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN))
  const end = $derived(Math.min(total, Math.ceil((scrollTop + viewportH) / ROW_H) + OVERSCAN))
  const slice = $derived(rows.slice(start, end))
  const padTop = $derived(start * ROW_H)
  const padBottom = $derived(Math.max(0, totalH - end * ROW_H))

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
    { key: 'added', label: 'ADDED', right: true },
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
      {#each slice as t (t.hash)}
        <TorrentRow {t} cols={COLS} />
      {/each}
      <div style="height:{padBottom}px"></div>
    {/if}
  </div>
</div>
