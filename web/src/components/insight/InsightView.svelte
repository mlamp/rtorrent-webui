<script lang="ts">
  import TrafficChart from '../charts/TrafficChart.svelte'
  import MetricChart from '../charts/MetricChart.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { silentGet } from '$lib/api/client'
  import { latestOnly } from '$lib/latest'
  import { pollWhileVisible } from '$lib/poll.svelte'
  import { short } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  type GPoint = { t: number; v: number }
  let range = $state('1h')
  let points = $state<Point[]>([])
  let winStart = $state(0) // server-dictated grid window [start, end] for the X axis
  let winEnd = $state(0)
  let disks = $state<{ path: string; total: number; free: number; used: number }[]>([])
  let metrics = $state<Record<string, GPoint[]>>({})
  const ranges = ['15m', '1h', '6h', '24h', '7d']

  // Guard against out-of-order responses: a slow fetch for a range the user has
  // since left (or a stale interval poll) must not overwrite the current one.
  const loadHistory = latestOnly(
    () => silentGet<{ points: Point[]; start: number; end: number }>(`/api/history?range=${range}`),
    (d) => {
      points = d.points ?? []
      winStart = d.start ?? 0
      winEnd = d.end ?? 0
    },
  )
  const loadMetrics = latestOnly(
    () => silentGet<Record<string, GPoint[]>>(`/api/metrics?range=${range}`),
    (d) => (metrics = d),
  )
  async function loadDisk() {
    const d = await silentGet<typeof disks>('/api/diskspace')
    if (d) disks = d
  }
  function setRange(r: string) {
    range = r
    loadHistory()
    loadMetrics()
  }
  // Immediate load + 3s cadence while the tab is visible; paused when hidden,
  // refreshed instantly on return.
  pollWhileVisible(() => {
    loadHistory()
    loadMetrics()
    loadDisk()
  }, 3000)

  const usedPct = (d: { total: number; used: number }) => (d.total > 0 ? (d.used / d.total) * 100 : 0)

  // Build a chart series from a stored gauge metric, scaling its integer encoding
  // back to a display value (cpu/mem permille→%, load ×100→load, others as-is).
  const ser = (key: string, color: string, label: string, scale = 1) => ({
    points: (metrics[key] ?? []).map((p) => ({ t: p.t, v: p.v / scale })),
    color,
    label,
  })

  const pct = (v: number) => `${Math.round(v)}%`
  const ld = (v: number) => v.toFixed(2)
  const cnt = (v: number) => String(Math.round(v))
  const byt = (v: number) => `${short(v)}B`

  const LOAD_COLORS = { l1: 'var(--status-seed)', l5: 'var(--status-check)', l15: 'var(--status-error)' }
</script>

<div class="insight">
  <section class="ip">
    <div class="ip-label">traffic</div>
    <div class="ip-traf-head">
      <span class="ip-rates">
        <span class="d">↓ {short(globals.downRate)}<small>B/s</small></span>
        <span class="u">↑ {short(globals.upRate)}<small>B/s</small></span>
      </span>
      <div class="rd-frames">
        {#each ranges as r (r)}
          <button class="rd-frame {range === r ? 'on' : ''}" onclick={() => setRange(r)}>{r}</button>
        {/each}
      </div>
    </div>
    <TrafficChart {points} start={winStart} end={winEnd} height={300} />
  </section>

  <div class="ip-metrics">
    <section class="ip">
      <div class="ip-label">cpu</div>
      <MetricChart series={[ser('cpu', 'var(--primary)', 'cpu', 10)]} yMax={100} format={pct} height={170} />
    </section>

    <section class="ip">
      <div class="ip-label">memory</div>
      <MetricChart series={[ser('mem', 'var(--acc2)', 'mem', 10)]} yMax={100} format={pct} height={170} />
    </section>

    <section class="ip">
      <div class="ip-label-row">
        <span class="ip-label">load average</span>
        <span class="ip-legend">
          <span><i style="background:{LOAD_COLORS.l1}"></i>1m</span>
          <span><i style="background:{LOAD_COLORS.l5}"></i>5m</span>
          <span><i style="background:{LOAD_COLORS.l15}"></i>15m</span>
        </span>
      </div>
      <MetricChart
        series={[ser('load1', LOAD_COLORS.l1, '1m', 100), ser('load5', LOAD_COLORS.l5, '5m', 100), ser('load15', LOAD_COLORS.l15, '15m', 100)]}
        format={ld}
        height={170}
      />
    </section>

    <section class="ip">
      <div class="ip-label">connected peers</div>
      <MetricChart series={[ser('peers', 'var(--status-download)', 'peers')]} format={cnt} height={170} />
    </section>

    <section class="ip">
      <div class="ip-label-row">
        <span class="ip-label">session transfer</span>
        <span class="ip-legend">
          <span><i style="background:var(--status-download)"></i>↓ {short(globals.downTotal)}B</span>
          <span><i style="background:var(--status-seed)"></i>↑ {short(globals.upTotal)}B</span>
        </span>
      </div>
      <MetricChart series={[ser('sess_down', 'var(--status-download)', '↓'), ser('sess_up', 'var(--status-seed)', '↑')]} format={byt} height={170} />
    </section>
  </div>

  <section class="ip">
    <div class="ip-label">disk</div>
    <div class="flex flex-col gap-4">
      {#each disks as d (d.path)}
        <div>
          <div class="ip-disk-head">
            <span class="font-mono">{d.path}</span>
            <span class="free font-mono">{short(d.free)}B free · {short(d.total)}B total</span>
          </div>
          <div class="ip-bar"><i style="width:{usedPct(d)}%"></i></div>
        </div>
      {/each}
      {#if disks.length === 0}<p class="text-dim">// no disk paths configured</p>{/if}
    </div>
  </section>

  <div class="ip-foot font-mono">
    peer country data by <a href="https://db-ip.com" target="_blank" rel="noreferrer">DB-IP</a> (CC BY 4.0)
  </div>
</div>
