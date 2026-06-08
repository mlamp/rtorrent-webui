<script lang="ts">
  import { onMount } from 'svelte'
  import TrafficChart from './TrafficChart.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { short } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  let range = $state('15m')
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

  const pct = (d: { total: number; used: number }) => (d.total > 0 ? (d.used / d.total) * 100 : 0)
  const dcolor = (p: number) =>
    p > 90 ? 'var(--status-error)' : p > 75 ? 'var(--status-check)' : 'var(--status-seed)'
</script>

<div class="h-full overflow-auto p-6">
  <div class="mx-auto grid max-w-5xl gap-7">
    <div class="cap-box p-4 pt-5">
      <div class="cap">traffic</div>
      <div class="mb-3 flex items-center justify-between">
        <div class="flex gap-5 text-[13px]">
          <span class="glow-acc text-status-download">↓ {short(globals.downRate)}B/s</span>
          <span class="glow-acc2 text-status-seed">↑ {short(globals.upRate)}B/s</span>
        </div>
        <div class="flex gap-1.5">
          {#each ranges as r (r)}
            <button class="tbtn {range === r ? 'solid' : ''}" onclick={() => setRange(r)}>{r}</button>
          {/each}
        </div>
      </div>
      <TrafficChart {points} {range} height={220} />
    </div>

    <div class="cap-box p-4 pt-5">
      <div class="cap">disk</div>
      <div class="flex flex-col gap-4">
        {#each disks as d (d.path)}
          {@const p = pct(d)}
          <div>
            <div class="mb-1.5 flex items-center justify-between text-[12.5px]">
              <span class="text-foreground">{d.path}</span>
              <span class="text-dim">{short(d.free)}B free · {short(d.total)}B total</span>
            </div>
            <div class="h-2.5 overflow-hidden rounded-sm" style="background:color-mix(in srgb, var(--primary) 12%, transparent)">
              <div class="h-full transition-all" style="width:{p}%; background:{dcolor(p)}; box-shadow:0 0 8px {dcolor(p)}"></div>
            </div>
          </div>
        {/each}
        {#if disks.length === 0}<p class="text-dim">// no disk paths configured</p>{/if}
      </div>
    </div>

    <div class="cap-box p-4 pt-5">
      <div class="cap">search</div>
      <p class="text-[12.5px] text-dim">// no search adapters configured — v1 seam, site adapters land later</p>
    </div>

    <p class="text-center text-[11px] text-dim">
      peer country data by
      <a href="https://db-ip.com" target="_blank" rel="noreferrer" class="underline hover:text-foreground">DB-IP</a>
      (CC BY 4.0)
    </p>
  </div>
</div>
