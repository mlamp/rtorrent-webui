<script lang="ts">
  // Real per-piece completion map. `cells` come from decodePieces($lib/pieces) —
  // each is a completion fraction in [0,1] (1 = have, 0 = missing, in-between when
  // a cell aggregates several chunks on a large torrent). No fabrication: if the
  // bitfield isn't available, the caller shows a % bar instead of this grid.
  //
  // Layout is responsive: the map fills the full available width left→right, and the
  // cell size shrinks as the piece count grows (many pieces → small cells), so a
  // 50-piece torrent reads as chunky blocks while a 4000-piece one stays dense.
  let {
    cells = [],
    pieceCount = cells.length,
    gap = 2,
    radius = 1,
  }: { cells?: number[]; pieceCount?: number; gap?: number; radius?: number } = $props()

  let containerW = $state(0)

  // Target cell size keyed off the true piece count (not the budget-capped cell
  // array length), so density — not the aggregation cap — drives how big a cell is.
  const targetCell = $derived(
    pieceCount <= 100 ? 14 : pieceCount <= 400 ? 11 : pieceCount <= 1500 ? 9 : pieceCount <= 4000 ? 7 : 6,
  )

  // Columns that fit the measured width at the target size (N cells need N-1 gaps);
  // fall back to a sane default on the first frame before measurement lands.
  const cols = $derived(
    containerW > 0 ? Math.max(1, Math.floor((containerW + gap) / (targetCell + gap))) : 56,
  )
  // Recompute the real cell size so the row spans the container edge-to-edge
  // (fractional px is fine in SVG) instead of leaving a ragged right margin.
  const cell = $derived(containerW > 0 ? Math.max(1, (containerW - (cols - 1) * gap) / cols) : targetCell)

  const rows = $derived(Math.ceil(cells.length / cols))
  const w = $derived(containerW > 0 ? containerW : cols * (cell + gap) - gap)
  const h = $derived(rows * (cell + gap) - gap)

  // 0 → faint "missing"; 1 → solid "have"; partial → blended (aggregated cells).
  function fill(f: number): string {
    if (f >= 0.999) return 'var(--primary)'
    if (f <= 0.001) return 'color-mix(in srgb, var(--primary) 12%, transparent)'
    return `color-mix(in srgb, var(--primary) ${Math.round(12 + f * 88)}%, transparent)`
  }
</script>

<div bind:clientWidth={containerW} class="w-full">
  <svg width="100%" height={h} viewBox="0 0 {w} {h}" style="display:block" preserveAspectRatio="xMinYMin meet">
    {#each cells as f, i (i)}
      <rect
        x={(i % cols) * (cell + gap)}
        y={Math.floor(i / cols) * (cell + gap)}
        width={cell}
        height={cell}
        rx={radius}
        fill={fill(f)}
      />
    {/each}
  </svg>
</div>
