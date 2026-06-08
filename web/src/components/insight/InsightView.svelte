<script lang="ts">
  import { onMount } from 'svelte'
  import TrafficChart from './TrafficChart.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { short } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  let range = $state('1h')
  let points = $state<Point[]>([])
  let disks = $state<{ path: string; total: number; free: number; used: number }[]>([])
  const ranges = ['15m', '1h', '6h', '24h', '7d']

  async function loadHistory() {
    try {
      const j = await (await fetch(`/api/history?range=${range}`)).json()
      points = j?.data?.points ?? []
    } catch {
      /* ignore */
    }
  }
  async function loadDisk() {
    try {
      const j = await (await fetch('/api/diskspace')).json()
      disks = j?.data ?? []
    } catch {
      /* ignore */
    }
  }
  function setRange(r: string) {
    range = r
    loadHistory()
  }
  onMount(() => {
    loadHistory()
    loadDisk()
    const id = setInterval(() => {
      loadHistory()
      loadDisk()
    }, 3000)
    return () => clearInterval(id)
  })

  const usedPct = (d: { total: number; used: number }) => (d.total > 0 ? (d.used / d.total) * 100 : 0)
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
    <TrafficChart {points} {range} height={300} />
  </section>

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

  <section class="ip">
    <div class="ip-label">search</div>
    <div class="ip-search font-mono">// no search adapters configured — v1 seam, site adapters land later</div>
  </section>

  <div class="ip-foot font-mono">
    peer country data by <a href="https://db-ip.com" target="_blank" rel="noreferrer">DB-IP</a> (CC BY 4.0)
  </div>
</div>
