<script lang="ts">
  import { ChevronUp, ChevronDown } from '@lucide/svelte'
  import TorrentRow from './TorrentRow.svelte'
  import { view, type ColumnKey } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import type { TorrentRow as TRow } from '$lib/stores/torrents.svelte'

  let { rows }: { rows: TRow[] } = $props()

  const allSelected = $derived(rows.length > 0 && rows.every((t) => selection.has(t.hash)))
  function toggleAll() {
    const on = !allSelected
    for (const t of rows) selection.set(t.hash, on)
  }

  // Shared grid template for header + rows.
  const COLS = '28px minmax(180px,1fr) 90px 200px 96px 96px 64px 110px 120px 120px'
  const ROW_H = 36
  const OVERSCAN = 8

  let scrollTop = $state(0)
  let viewportH = $state(600)
  let viewport: HTMLDivElement

  const total = $derived(rows.length)
  const start = $derived(Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN))
  const visibleCount = $derived(Math.ceil(viewportH / ROW_H) + OVERSCAN * 2)
  const end = $derived(Math.min(total, start + visibleCount))
  const slice = $derived(rows.slice(start, end))
  const padTop = $derived(start * ROW_H)
  const padBottom = $derived(Math.max(0, (total - end) * ROW_H))

  const headers: { key: ColumnKey | null; label: string; right?: boolean }[] = [
    { key: null, label: '' },
    { key: 'name', label: 'Name' },
    { key: 'size', label: 'Size', right: true },
    { key: 'done', label: 'Progress' },
    { key: 'downRate', label: 'Down', right: true },
    { key: 'upRate', label: 'Up', right: true },
    { key: 'ratio', label: 'Ratio', right: true },
    { key: null, label: 'Seeds/Peers', right: true },
    { key: 'status', label: 'Status' },
    { key: 'label', label: 'Label' },
  ]
</script>

<div class="flex h-full flex-col">
  <div
    class="grid shrink-0 border-b bg-card text-xs uppercase tracking-wide text-muted-foreground"
    style="grid-template-columns:{COLS}"
  >
    {#each headers as h, i (i)}
      {#if i === 0}
        <div class="grid place-items-center">
          <input type="checkbox" class="accent-primary" checked={allSelected} onchange={toggleAll} />
        </div>
      {:else}
        <button
          class="flex items-center gap-1 px-3 py-2 font-medium {h.right ? 'justify-end' : ''} {h.key ? 'hover:text-foreground' : 'cursor-default'}"
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
    class="min-h-0 flex-1 overflow-auto"
  >
    {#if total === 0}
      <div class="grid h-40 place-items-center text-sm text-muted-foreground">No torrents</div>
    {:else}
      <div style="height:{padTop}px"></div>
      {#each slice as t (t.hash)}
        <TorrentRow {t} cols={COLS} />
      {/each}
      <div style="height:{padBottom}px"></div>
    {/if}
  </div>
</div>
