<script lang="ts">
  import { HardDrive } from '@lucide/svelte'
  import { bytes } from '$lib/format'

  type Disk = { path: string; total: number; free: number; used: number }
  let { disks = [] }: { disks?: Disk[] } = $props()

  function pct(d: Disk) {
    return d.total > 0 ? (d.used / d.total) * 100 : 0
  }
  function color(p: number) {
    return p > 90 ? 'var(--status-error)' : p > 75 ? 'var(--status-check)' : 'var(--status-seed)'
  }
</script>

<div class="flex flex-col gap-4">
  {#each disks as d (d.path)}
    {@const p = pct(d)}
    <div>
      <div class="mb-1 flex items-center justify-between text-sm">
        <span class="flex items-center gap-1.5 font-medium"><HardDrive class="size-4 text-muted-foreground" />{d.path}</span>
        <span class="tabular-nums text-muted-foreground">{bytes(d.free)} free of {bytes(d.total)}</span>
      </div>
      <div class="h-2.5 overflow-hidden rounded-full bg-secondary">
        <div class="h-full rounded-full transition-all" style="width:{p}%; background:{color(p)}"></div>
      </div>
    </div>
  {/each}
  {#if disks.length === 0}
    <p class="text-sm text-muted-foreground">No disk paths configured.</p>
  {/if}
</div>
