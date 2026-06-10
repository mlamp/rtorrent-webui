<script lang="ts">
  // Smooth (monotone-cubic) area+line speed sparkline. Time-based and fed the global
  // /api/history series ({t,down,up} + window [start,end]) — the SAME data, shape and
  // path/Y-scale logic as the full TrafficChart (via the shared timeSeriesPath +
  // niceMax helpers), just compact and axis-less. Idle/missing slots are zero-filled
  // server-side, so an idle period reads as a flat line on the baseline.
  import { timeSeriesPath } from '$lib/charts'

  type Point = { t: number; down: number; up: number }
  let {
    points = [],
    start = 0,
    end = 0,
    h = 58,
    dlColor = 'var(--status-download)',
    ulColor = 'var(--status-seed)',
    glow = true,
    grid = true,
    strokeW = 1.6,
  }: {
    points?: Point[]
    start?: number
    end?: number
    h?: number
    dlColor?: string
    ulColor?: string
    glow?: boolean
    grid?: boolean
    strokeW?: number
  } = $props()

  let w = $state(246)
  const uid = 'sg' + Math.random().toString(36).slice(2, 8)

  // Tight scaling (peak + 15% headroom) so the compact, axis-less sparkline fills
  // its height and reads as reactive — unlike the full chart it has no round-number
  // axis to honour, so it doesn't use niceMax.
  const maxVal = $derived(Math.max(1, ...points.map((p) => Math.max(p.down, p.up))) * 1.15)

  const dlPath = $derived(timeSeriesPath(points, 'down', start, end, 0, w, 0, h, maxVal))
  const ulPath = $derived(timeSeriesPath(points, 'up', start, end, 0, w, 0, h, maxVal))
  const dlArea = $derived(dlPath ? `${dlPath} L ${w},${h} L 0,${h} Z` : '')
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
