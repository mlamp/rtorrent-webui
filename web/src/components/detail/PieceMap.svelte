<script lang="ts">
  // Real per-piece completion map. `cells` come from decodePieces($lib/pieces) —
  // each is a completion fraction in [0,1] (1 = have, 0 = missing, in-between when
  // a cell aggregates several chunks on a large torrent). No fabrication: if the
  // bitfield isn't available, the caller shows a % bar instead of this grid.
  let {
    cells = [],
    cols = 56,
    cell = 11,
    gap = 2,
    radius = 1,
  }: { cells?: number[]; cols?: number; cell?: number; gap?: number; radius?: number } = $props()

  const rows = $derived(Math.ceil(cells.length / cols))
  const w = $derived(cols * (cell + gap) - gap)
  const h = $derived(rows * (cell + gap) - gap)

  // 0 → faint "missing"; 1 → solid "have"; partial → blended (aggregated cells).
  function fill(f: number): string {
    if (f >= 0.999) return 'var(--primary)'
    if (f <= 0.001) return 'color-mix(in srgb, var(--primary) 12%, transparent)'
    return `color-mix(in srgb, var(--primary) ${Math.round(12 + f * 88)}%, transparent)`
  }
</script>

<svg width={w} height={h} viewBox="0 0 {w} {h}" style="display:block;max-width:100%" preserveAspectRatio="xMinYMin meet">
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
