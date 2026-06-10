<script lang="ts">
  // Generalized multi-series line chart for the Insight gauge panels (CPU, load,
  // memory, peers, session totals). Shares the monotone-cubic smoothing with
  // TrafficChart but takes N named series, a Y-axis value formatter, and an
  // optional fixed yMax (percentage charts pin 0–100).
  import { monotonePath } from '$lib/charts'

  type MPoint = { t: number; v: number }
  type Series = { points: MPoint[]; color: string; label: string }
  let {
    series = [],
    height = 180,
    yMax = 0,
    format = (v: number) => String(Math.round(v)),
  }: { series?: Series[]; height?: number; yMax?: number; format?: (v: number) => string } = $props()

  let w = $state(700)
  let hover = $state<number | null>(null) // hovered time (epoch secs)
  let svgEl = $state<SVGSVGElement>()

  const padL = 52,
    padR = 14,
    padT = 12,
    padB = 26
  const plotW = $derived(Math.max(1, w - padL - padR))
  const plotH = $derived(Math.max(1, height - padT - padB))

  const allPts = $derived(series.flatMap((s) => s.points))
  const n = $derived(allPts.length)

  const t0 = $derived(n ? Math.min(...allPts.map((p) => p.t)) : 0)
  const t1 = $derived(n ? Math.max(...allPts.map((p) => p.t)) : 1)
  const span = $derived(Math.max(1, t1 - t0))

  // Round up to a "nice" base-10 max (1/2/5 × 10ⁿ) for readable gridlines.
  function niceMax(m: number): number {
    if (m <= 0) return 1
    const pow = Math.pow(10, Math.floor(Math.log10(m)))
    const f = m / pow
    const step = f <= 1 ? 1 : f <= 2 ? 2 : f <= 5 ? 5 : 10
    return step * pow
  }
  const maxVal = $derived(yMax > 0 ? yMax : niceMax(Math.max(...allPts.map((p) => p.v), 1)))
  const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => ({ f, v: maxVal * f })))

  const xOf = (t: number) => padL + ((t - t0) / span) * plotW
  const yOf = (v: number) => padT + plotH - (Math.min(v, maxVal) / maxVal) * plotH

  function path(pts: MPoint[]): string {
    if (pts.length < 2) return ''
    return monotonePath(pts.map((p) => [xOf(p.t), yOf(p.v)] as [number, number]))
  }

  function pad2(x: number) {
    return String(x).padStart(2, '0')
  }
  const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  function axisLabel(t: number): string {
    const d = new Date(t * 1000)
    const hm = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
    if (span <= 3 * 3600) return `${hm}:${pad2(d.getSeconds())}`
    if (span <= 26 * 3600) return hm
    if (span <= 4 * 86400) return `${MON[d.getMonth()]} ${d.getDate()} ${hm}`
    return `${MON[d.getMonth()]} ${d.getDate()}`
  }
  function fullTime(t: number): string {
    const d = new Date(t * 1000)
    const date = span > 24 * 3600 ? `${MON[d.getMonth()]} ${d.getDate()} ` : ''
    return `${date}${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
  }
  const xTicks = $derived.by(() => {
    if (n < 2) return [] as { x: number; label: string }[]
    const count = 5
    return Array.from({ length: count }, (_, k) => {
      const t = t0 + (span * k) / (count - 1)
      return { x: xOf(t), label: axisLabel(t) }
    })
  })

  const uid = 'mc' + Math.random().toString(36).slice(2, 7)

  function onMove(e: MouseEvent) {
    if (!svgEl || n < 1) return
    const r = svgEl.getBoundingClientRect()
    const x = ((e.clientX - r.left) * w) / r.width
    const frac = Math.max(0, Math.min(1, (x - padL) / plotW))
    hover = t0 + frac * span
  }
  // nearest point in a series to the hovered time
  function nearest(pts: MPoint[], t: number): MPoint | null {
    if (!pts.length) return null
    let best = pts[0]
    let bd = Math.abs(pts[0].t - t)
    for (const p of pts) {
      const d = Math.abs(p.t - t)
      if (d < bd) {
        bd = d
        best = p
      }
    }
    return best
  }
  const tipX = $derived(hover === null ? 0 : Math.max(padL, Math.min(w - padR - 120, xOf(hover) + 10)))
</script>

<div bind:clientWidth={w} class="relative" style="color:var(--primary)">
  {#if n < 2}
    <div class="grid place-items-center text-dim" style="height:{height}px">// collecting data…</div>
  {:else}
    <svg
      bind:this={svgEl}
      width="100%"
      height={height}
      viewBox="0 0 {w} {height}"
      preserveAspectRatio="none"
      role="img"
      aria-label="metric history"
      onmousemove={onMove}
      onmouseleave={() => (hover = null)}
    >
      <defs>
        <filter id="{uid}-g" x="-10%" y="-10%" width="120%" height="120%">
          <feGaussianBlur stdDeviation="2" result="b" />
          <feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge>
        </filter>
      </defs>

      {#each yTicks as yt (yt.f)}
        {@const y = padT + plotH - yt.f * plotH}
        <line x1={padL} y1={y} x2={w - padR} y2={y} stroke="currentColor" stroke-opacity="0.08" />
        <text x={padL - 8} y={y + 3.5} text-anchor="end" font-size="9.5" fill="var(--muted-foreground)" font-family="var(--font-mono, monospace)">
          {format(yt.v)}
        </text>
      {/each}

      {#each xTicks as xt, i (i)}
        <text x={xt.x} y={height - 8} text-anchor={i === 0 ? 'start' : i === xTicks.length - 1 ? 'end' : 'middle'} font-size="9.5" fill="var(--muted-foreground)" font-family="var(--font-mono, monospace)">
          {xt.label}
        </text>
      {/each}

      {#each series as s (s.label)}
        {@const d = path(s.points)}
        {#if d}<path d={d} fill="none" stroke={s.color} stroke-width="1.6" stroke-linecap="round" filter="url(#{uid}-g)" />{/if}
      {/each}

      {#if hover !== null}
        {@const hx = xOf(hover)}
        <line x1={hx} y1={padT} x2={hx} y2={padT + plotH} stroke="currentColor" stroke-opacity="0.35" stroke-dasharray="3 3" />
        {#each series as s (s.label)}
          {@const p = nearest(s.points, hover)}
          {#if p}<circle cx={xOf(p.t)} cy={yOf(p.v)} r="3" fill={s.color} stroke="var(--background)" stroke-width="1.5" />{/if}
        {/each}
      {/if}
    </svg>

    {#if hover !== null}
      {@const ht = nearest(series[0]?.points ?? [], hover)}
      <div
        class="pointer-events-none absolute z-10 rounded-sm border border-line px-2.5 py-1.5 text-[11px] leading-tight"
        style="left:{tipX}px; top:{padT + 2}px; background:color-mix(in srgb, var(--background) 92%, transparent); backdrop-filter:blur(3px)"
      >
        <div class="mb-1 text-dim">{ht ? fullTime(ht.t) : ''}</div>
        {#each series as s (s.label)}
          {@const p = nearest(s.points, hover)}
          {#if p}<div class="flex items-center gap-1.5"><span style="color:{s.color}">●</span> {s.label} {format(p.v)}</div>{/if}
        {/each}
      </div>
    {/if}
  {/if}
</div>
