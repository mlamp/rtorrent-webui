<script lang="ts">
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { Status } from '$lib/types/torrent'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import { view } from '$lib/stores/view.svelte'
  import { short, ratio } from '$lib/format'
  import { rollingHistory } from '$lib/history.svelte'
  import SpeedGraph from '../SpeedGraph.svelte'

  let { t }: { t: TorrentRow } = $props()

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
  const selected = $derived(selection.has(t.hash))
  const cursor = $derived(view.cursor === t.hash)

  // Live mini-sparkline. Starts flat-at-zero (no fabricated history) and fills with
  // real samples as poll ticks arrive — matching the global header graph.
  const hist = rollingHistory(() => ({ down: t.downRate, up: t.upRate }), 44)

  function onclick(e: MouseEvent) {
    if (e.metaKey || e.ctrlKey || e.shiftKey) selection.toggle(t.hash)
    else {
      view.cursor = t.hash
      detail.open(t.hash)
    }
  }
</script>

<div
  data-torrent={t.hash}
  class="gcard"
  class:sel={selected}
  class:cursor
  class:sweep={t.sweeping}
  style="--seg-c:{SEGVAR[t.status]}"
  {onclick}
  role="button"
  tabindex="-1"
  onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && detail.open(t.hash)}
>
  <div class="gc-stat" style="color:{SEGVAR[t.status]}">{t.status}</div>
  <div class="gc-top">
    <span
      class="gc-chk"
      onclick={(e) => {
        e.stopPropagation()
        selection.toggle(t.hash)
      }}
      role="presentation"
    >
      <span class="chk" class:on={selected}>✓</span>
    </span>
    <span class="gc-mk" style="color:{SEGVAR[t.status]}">{MARK[t.status]}</span>
    <span class="gc-name">{t.name}</span>
  </div>

  <div class="gc-prog">
    <div class="seg">
      {#each Array(SEG) as _, i (i)}
        <i class="sg" class:on={i < filled} class:lead={isDl && i === filled && t.done < 1}></i>
      {/each}
    </div>
    <span class="gc-pct">{Math.round(t.done * 100)}%</span>
  </div>

  <div class="gc-graph">
    <SpeedGraph dl={hist.dl} ul={hist.ul} h={30} dlColor="var(--status-download)" ulColor="var(--status-seed)" glow={false} grid={false} strokeW={1.4} />
  </div>

  <div class="gc-meta">
    <span class="d">↓{t.downRate ? short(t.downRate) : '·'}</span>
    <span class="u">↑{t.upRate ? short(t.upRate) : '·'}</span>
    <span class="sz">{short(t.size)}B</span>
    <span class="rt">r {ratio(t.ratio)}</span>
  </div>
</div>
