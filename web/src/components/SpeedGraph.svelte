<script lang="ts">
  // Smooth (Catmull-Rom) area+line speed sparkline, ported from the prototype.
  let {
    dl = [],
    ul = [],
    h = 58,
    dlColor = 'var(--status-download)',
    ulColor = 'var(--status-seed)',
    glow = true,
    grid = true,
    strokeW = 1.6,
  }: {
    dl?: number[]
    ul?: number[]
    h?: number
    dlColor?: string
    ulColor?: string
    glow?: boolean
    grid?: boolean
    strokeW?: number
  } = $props()

  let w = $state(246)
  const uid = 'sg' + Math.random().toString(36).slice(2, 8)

  const max = $derived(Math.max(1, ...dl, ...(ul ?? [])) * 1.15)

  function pts(arr: number[]): [number, number][] {
    return arr.map((v, i) => [(i / Math.max(1, arr.length - 1)) * w, h - (v / max) * h])
  }
  function smooth(p: [number, number][]): string {
    if (p.length < 2) return ''
    let d = `M ${p[0][0]},${p[0][1]}`
    for (let i = 0; i < p.length - 1; i++) {
      const p0 = p[i - 1] || p[i]
      const p1 = p[i]
      const p2 = p[i + 1]
      const p3 = p[i + 2] || p2
      const c1x = p1[0] + (p2[0] - p0[0]) / 6
      const c1y = p1[1] + (p2[1] - p0[1]) / 6
      const c2x = p2[0] - (p3[0] - p1[0]) / 6
      const c2y = p2[1] - (p3[1] - p1[1]) / 6
      d += ` C ${c1x},${c1y} ${c2x},${c2y} ${p2[0]},${p2[1]}`
    }
    return d
  }

  const dlPath = $derived(smooth(pts(dl)))
  const dlArea = $derived(dlPath ? `${dlPath} L ${w},${h} L 0,${h} Z` : '')
  const ulPath = $derived(ul && ul.length > 1 ? smooth(pts(ul)) : '')
</script>

<div bind:clientWidth={w} style="color:var(--primary)">
  <svg width="100%" height={h} viewBox="0 0 {w} {h}" preserveAspectRatio="none" style="display:block;overflow:visible">
    <defs>
      <linearGradient id="{uid}-f" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stop-color={dlColor} stop-opacity={glow ? 0.45 : 0.28} />
        <stop offset="100%" stop-color={dlColor} stop-opacity="0" />
      </linearGradient>
      {#if glow}
        <filter id="{uid}-g" x="-20%" y="-20%" width="140%" height="140%">
          <feGaussianBlur stdDeviation="3" result="b" />
          <feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge>
        </filter>
      {/if}
    </defs>
    {#if grid}
      {#each [0.25, 0.5, 0.75] as g (g)}
        <line x1="0" y1={h * g} x2={w} y2={h * g} stroke="currentColor" stroke-opacity="0.07" stroke-width="1" />
      {/each}
    {/if}
    {#if dlArea}<path d={dlArea} fill="url(#{uid}-f)" />{/if}
    {#if ulPath}<path d={ulPath} fill="none" stroke={ulColor} stroke-width={strokeW} stroke-opacity="0.85" stroke-linecap="round" />{/if}
    {#if dlPath}<path d={dlPath} fill="none" stroke={dlColor} stroke-width={strokeW} stroke-linecap="round" filter={glow ? `url(#${uid}-g)` : undefined} />{/if}
  </svg>
</div>
