<script lang="ts">
  // Completion map — a fixed SVG grid of piece cells, `done` fraction filled.
  // rtorrent doesn't expose a real piece bitfield over RPC, so this is a
  // sequential approximation (have / in-flight near the edge while downloading
  // / missing). Ported to the reference's SVG-rect layout (shared.jsx PieceMap)
  // so it reads as a crisp grid and can also serve as the grid-card strip.
  let {
    done = 0,
    count = 280,
    downloading = false,
    cell = 11,
    gap = 2,
    cols = 56,
    radius = 1,
  }: {
    done?: number
    count?: number
    downloading?: boolean
    cell?: number
    gap?: number
    cols?: number
    radius?: number
  } = $props()

  const cells = $derived.by(() => {
    const filled = Math.floor(done * count)
    const out: number[] = []
    for (let i = 0; i < count; i++) {
      if (i < filled) out.push(1)
      else if (downloading && i < filled + 8 && i % 2 === 0) out.push(2)
      else out.push(0)
    }
    return out
  })

  const rows = $derived(Math.ceil(count / cols))
  const w = $derived(cols * (cell + gap) - gap)
  const h = $derived(rows * (cell + gap) - gap)
  const fill = (s: number) =>
    s === 1 ? 'var(--primary)' : s === 2 ? 'var(--warn)' : 'color-mix(in srgb, var(--primary) 12%, transparent)'
</script>

<svg width={w} height={h} viewBox="0 0 {w} {h}" style="display:block;max-width:100%" preserveAspectRatio="xMinYMin meet">
  {#each cells as s, i (i)}
    <rect
      x={(i % cols) * (cell + gap)}
      y={Math.floor(i / cols) * (cell + gap)}
      width={cell}
      height={cell}
      rx={radius}
      fill={fill(s)}
    />
  {/each}
</svg>
