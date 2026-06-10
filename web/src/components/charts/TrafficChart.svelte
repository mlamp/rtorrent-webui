<script lang="ts">
  // Relay-styled traffic chart: smooth down area + up line, labelled X/Y axes,
  // and a hover crosshair with a date/time + rate tooltip. Shared by the global
  // INSIGHT traffic panel and the per-torrent detail graph — both feed it real
  // {t,down,up} points from /api/history PLUS the server window [start,end].
  // X is mapped by TIME (the server window, not array index / data extrema), so
  // zero-filled idle slots sit at their true position and an idle series renders
  // a flat zero line across the whole window.
  import { short } from '$lib/format'
  import { timeSeriesPath, niceMax } from '$lib/charts'

  type Point = { t: number; down: number; up: number }
  let {
    points = [],
    start = 0,
    end = 0,
    height = 220,
    dlColor = 'var(--status-download)',
    ulColor = 'var(--status-seed)',
  }: {
    points?: Point[]
    start?: number
    end?: number
    height?: number
    dlColor?: string
    ulColor?: string
  } = $props()

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

  // ── value (Y) axis ─ niceMax shared with the sidebar sparkline ──────────────
  const maxVal = $derived(niceMax(Math.max(...points.map((p) => Math.max(p.down, p.up)), 1)))
  const yTicks = $derived([0, 0.25, 0.5, 0.75, 1].map((f) => ({ f, v: maxVal * f })))

  // ── time (X) axis ─ the window is the server-dictated [start, end], NOT the
  // data extrema, so a sparse/idle series still spans the full width.
  const span = $derived(Math.max(1, end - start))
  const xOf = (t: number) => (end > start ? padL + (plotW * (t - start)) / (end - start) : padL)
  const yOf = (v: number) => padT + plotH - (Math.min(v, maxVal) / maxVal) * plotH

  function pad2(x: number) {
    return String(x).padStart(2, '0')
  }
  const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
  // Axis tick label: resolution scales with the visible span so the numbers stay
  // meaningful — seconds when zoomed in, calendar dates when spanning days.
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
  // ~5 evenly-spaced X ticks across the window (by time)
  const xTicks = $derived.by(() => {
    if (end <= start) return [] as { x: number; label: string }[]
    const count = 5
    return Array.from({ length: count }, (_, k) => {
      const t = start + (span * k) / (count - 1)
      return { x: xOf(t), label: axisLabel(t) }
    })
  })

  // ── smooth (monotone-cubic) paths via the shared helper — can't overshoot, so
  // a rate dropping to zero never draws below the axis ─────────────────────────
  const dlPath = $derived(timeSeriesPath(points, 'down', start, end, padL, plotW, padT, plotH, maxVal))
  const ulPath = $derived(timeSeriesPath(points, 'up', start, end, padL, plotW, padT, plotH, maxVal))
  const dlArea = $derived(dlPath ? `${dlPath} L ${xOf(end)},${padT + plotH} L ${padL},${padT + plotH} Z` : '')

  const uid = 'tc' + Math.random().toString(36).slice(2, 7)

  // ── hover ─ invert pointer x → time → nearest sample by time ────────────────
  function onMove(e: MouseEvent) {
    if (!svgEl || n < 1 || end <= start) return
    const r = svgEl.getBoundingClientRect()
    const x = ((e.clientX - r.left) * w) / r.width
    const tt = start + ((x - padL) / plotW) * (end - start)
    let best = 0
    let bd = Infinity
    for (let i = 0; i < n; i++) {
      const d = Math.abs(points[i].t - tt)
      if (d < bd) {
        bd = d
        best = i
      }
    }
    hover = best
  }
  const hp = $derived(hover !== null && points[hover] ? points[hover] : null)
  // keep the tooltip inside the plot
  const tipX = $derived(hp ? Math.max(padL, Math.min(w - padR - 132, xOf(hp.t) + 10)) : 0)
</script>

<div bind:clientWidth={w} class="relative" style="color:var(--primary)">
  {#if n < 2}
    <!-- pre-first-fetch only: the server returns a dense zero-filled grid, so an
         idle torrent renders a flat 0 line rather than landing here -->
    <div style="height:{height}px"></div>
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
        {@const hx = xOf(hp.t)}
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
