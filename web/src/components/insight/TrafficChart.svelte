<script lang="ts">
  // Relay-styled traffic chart: smooth down area + up line, labelled X/Y axes,
  // and a hover crosshair with a date/time + rate tooltip.
  import { short } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  let {
    points = [],
    height = 220,
    range = '15m',
    dlColor = 'var(--status-download)',
    ulColor = 'var(--status-seed)',
  }: { points?: Point[]; height?: number; range?: string; dlColor?: string; ulColor?: string } = $props()

  let w = $state(700)
  let hover = $state<number | null>(null)
  let svgEl = $state<SVGSVGElement>()

  const padL = 56,
    padR = 14,
    padT = 12,
    padB = 26
  const plotW = $derived(Math.max(1, w - padL - padR))
  const plotH = $derived(Math.max(1, height - padT - padB))
  const n = $derived(points.length)

  // ── value (Y) axis ────────────────────────────────────────────────────────
  function niceMax(m: number): number {
    if (m <= 0) return 1024
    const k = Math.floor(Math.log(m) / Math.log(1024))
    const unit = Math.pow(1024, k)
    const f = m / unit
    const steps = [1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024]
    return (steps.find((s) => f <= s) ?? 1024) * unit
  }
  const maxVal = $derived(niceMax(Math.max(...points.map((p) => Math.max(p.down, p.up)), 1)))
  const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => ({ f, v: maxVal * f })))

  // ── time (X) axis ─ derived from the actual data span (handles tier fallback)
  const t0 = $derived(n ? points[0].t : 0)
  const t1 = $derived(n ? points[n - 1].t : 1)
  const span = $derived(Math.max(1, t1 - t0))

  const xOf = (i: number) => padL + (n < 2 ? 0 : (plotW * i) / (n - 1))
  const yOf = (v: number) => padT + plotH - (Math.min(v, maxVal) / maxVal) * plotH

  function pad2(x: number) {
    return String(x).padStart(2, '0')
  }
  function clock(t: number): string {
    const d = new Date(t * 1000)
    return span < 3 * 3600
      ? `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
      : `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
  }
  const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  function fullTime(t: number): string {
    const d = new Date(t * 1000)
    const date = span > 24 * 3600 ? `${MON[d.getMonth()]} ${d.getDate()} ` : ''
    return `${date}${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`
  }
  // ~5 evenly-spaced X ticks by index
  const xTicks = $derived.by(() => {
    if (n < 2) return [] as { x: number; label: string }[]
    const count = Math.min(5, n)
    return Array.from({ length: count }, (_, k) => {
      const i = Math.round((k * (n - 1)) / (count - 1))
      return { x: xOf(i), label: clock(points[i].t) }
    })
  })

  // ── smooth (Catmull-Rom) paths ──────────────────────────────────────────────
  function smooth(key: 'down' | 'up'): string {
    if (n < 2) return ''
    const p = points.map((pt, i) => [xOf(i), yOf(pt[key])] as [number, number])
    let d = `M ${p[0][0]},${p[0][1]}`
    for (let i = 0; i < p.length - 1; i++) {
      const p0 = p[i - 1] || p[i]
      const p1 = p[i]
      const p2 = p[i + 1]
      const p3 = p[i + 2] || p2
      d += ` C ${p1[0] + (p2[0] - p0[0]) / 6},${p1[1] + (p2[1] - p0[1]) / 6} ${
        p2[0] - (p3[0] - p1[0]) / 6
      },${p2[1] - (p3[1] - p1[1]) / 6} ${p2[0]},${p2[1]}`
    }
    return d
  }
  const dlPath = $derived(smooth('down'))
  const ulPath = $derived(smooth('up'))
  const dlArea = $derived(dlPath ? `${dlPath} L ${xOf(n - 1)},${padT + plotH} L ${padL},${padT + plotH} Z` : '')

  const uid = 'tc' + Math.random().toString(36).slice(2, 7)

  // ── hover ─────────────────────────────────────────────────────────────────
  function onMove(e: MouseEvent) {
    if (!svgEl || n < 1) return
    const r = svgEl.getBoundingClientRect()
    const x = ((e.clientX - r.left) * w) / r.width
    const i = Math.round(((x - padL) / plotW) * (n - 1))
    hover = Math.max(0, Math.min(n - 1, i))
  }
  const hp = $derived(hover !== null && points[hover] ? points[hover] : null)
  // keep the tooltip inside the plot
  const tipX = $derived(hover === null ? 0 : Math.max(padL, Math.min(w - padR - 132, xOf(hover) + 10)))
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
      aria-label="traffic history"
      onmousemove={onMove}
      onmouseleave={() => (hover = null)}
    >
      <defs>
        <linearGradient id="{uid}-f" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={dlColor} stop-opacity="0.35" />
          <stop offset="100%" stop-color={dlColor} stop-opacity="0" />
        </linearGradient>
        <filter id="{uid}-g" x="-10%" y="-10%" width="120%" height="120%">
          <feGaussianBlur stdDeviation="2.4" result="b" />
          <feMerge><feMergeNode in="b" /><feMergeNode in="SourceGraphic" /></feMerge>
        </filter>
      </defs>

      <!-- Y grid + labels -->
      {#each yTicks as yt (yt.f)}
        {@const y = padT + plotH - yt.f * plotH}
        <line x1={padL} y1={y} x2={w - padR} y2={y} stroke="currentColor" stroke-opacity="0.08" />
        <text x={padL - 8} y={y + 3.5} text-anchor="end" font-size="9.5" fill="var(--muted-foreground)" font-family="var(--font-mono, monospace)">
          {short(yt.v)}{yt.f === 1 ? '/s' : ''}
        </text>
      {/each}

      <!-- X labels -->
      {#each xTicks as xt, i (i)}
        <text x={xt.x} y={height - 8} text-anchor={i === 0 ? 'start' : i === xTicks.length - 1 ? 'end' : 'middle'} font-size="9.5" fill="var(--muted-foreground)" font-family="var(--font-mono, monospace)">
          {xt.label}
        </text>
      {/each}

      <!-- series -->
      {#if dlArea}<path d={dlArea} fill="url(#{uid}-f)" />{/if}
      {#if ulPath}<path d={ulPath} fill="none" stroke={ulColor} stroke-width="1.6" stroke-opacity="0.85" stroke-linecap="round" />{/if}
      {#if dlPath}<path d={dlPath} fill="none" stroke={dlColor} stroke-width="1.7" stroke-linecap="round" filter="url(#{uid}-g)" />{/if}

      <!-- hover crosshair + markers -->
      {#if hp && hover !== null}
        {@const hx = xOf(hover)}
        <line x1={hx} y1={padT} x2={hx} y2={padT + plotH} stroke="currentColor" stroke-opacity="0.35" stroke-dasharray="3 3" />
        <circle cx={hx} cy={yOf(hp.down)} r="3" fill={dlColor} stroke="var(--background)" stroke-width="1.5" />
        <circle cx={hx} cy={yOf(hp.up)} r="3" fill={ulColor} stroke="var(--background)" stroke-width="1.5" />
      {/if}
    </svg>

    {#if hp}
      <div
        class="pointer-events-none absolute z-10 rounded-sm border border-line px-2.5 py-1.5 text-[11px] leading-tight"
        style="left:{tipX}px; top:{padT + 2}px; background:color-mix(in srgb, var(--background) 92%, transparent); backdrop-filter:blur(3px)"
      >
        <div class="mb-1 text-dim">{fullTime(hp.t)}</div>
        <div class="flex items-center gap-1.5"><span style="color:{dlColor}">↓</span> {short(hp.down)}<small class="text-dim">B/s</small></div>
        <div class="flex items-center gap-1.5"><span style="color:{ulColor}">↑</span> {short(hp.up)}<small class="text-dim">B/s</small></div>
      </div>
    {/if}
  {/if}
</div>
