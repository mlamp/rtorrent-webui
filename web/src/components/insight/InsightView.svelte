<script lang="ts">
  import { onMount } from 'svelte'
  import { Search } from '@lucide/svelte'
  import TrafficChart from './TrafficChart.svelte'
  import DiskWidget from './DiskWidget.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { rate } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  let range = $state('15m')
  let points = $state<Point[]>([])
  let disks = $state<any[]>([])
  const ranges = ['15m', '1h', '6h', '24h']

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
</script>

<div class="h-full overflow-auto p-6">
  <div class="mx-auto grid max-w-5xl gap-6">
    <!-- traffic -->
    <section class="rounded-lg border bg-card p-4">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <h2 class="font-semibold">Traffic</h2>
          <div class="mt-0.5 flex gap-4 text-sm">
            <span class="text-status-download">▼ {rate(globals.downRate)}</span>
            <span class="text-status-seed">▲ {rate(globals.upRate)}</span>
          </div>
        </div>
        <div class="flex gap-1 rounded-md bg-secondary p-1">
          {#each ranges as r (r)}
            <button
              onclick={() => setRange(r)}
              class="rounded px-2.5 py-1 text-xs transition {range === r ? 'bg-card font-medium text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}"
            >{r}</button>
          {/each}
        </div>
      </div>
      <TrafficChart {points} />
    </section>

    <!-- disk -->
    <section class="rounded-lg border bg-card p-4">
      <h2 class="mb-3 font-semibold">Disk space</h2>
      <DiskWidget {disks} />
    </section>

    <!-- search (seam) -->
    <section class="rounded-lg border bg-card p-4">
      <h2 class="mb-2 flex items-center gap-2 font-semibold"><Search class="size-4" /> Tracker search</h2>
      <p class="text-sm text-muted-foreground">
        No search adapters configured. This is a v1 seam — site adapters land later.
      </p>
    </section>

    <p class="text-center text-xs text-muted-foreground">
      Peer country data by
      <a href="https://db-ip.com" target="_blank" rel="noreferrer" class="underline hover:text-foreground">DB-IP</a>
      (CC BY 4.0)
    </p>
  </div>
</div>
