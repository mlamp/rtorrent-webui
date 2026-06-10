<script lang="ts">
  import Modal from '../ui/Modal.svelte'
  import { api } from '$lib/api/client'
  import { globals } from '$lib/stores/globals.svelte'

  let { open = $bindable(false) }: { open?: boolean } = $props()
  let down = $state(0) // KiB/s; 0 = unlimited
  let up = $state(0)
  let busy = $state(false)

  // quick-set chips (value in KiB/s); clicking only fills the input — APPLY commits.
  const PRESETS = [
    { label: '∞', kib: 0 },
    { label: '1M', kib: 1024 },
    { label: '5M', kib: 5 * 1024 },
    { label: '10M', kib: 10 * 1024 },
    { label: '25M', kib: 25 * 1024 },
  ]

  $effect(() => {
    if (open) {
      down = Math.round(globals.downLimit / 1024)
      up = Math.round(globals.upLimit / 1024)
    }
  })

  async function apply() {
    busy = true
    try {
      await api.setThrottle(down * 1024, up * 1024)
      open = false
    } catch {
      /* toast shown */
    } finally {
      busy = false
    }
  }
</script>

<Modal bind:open title="rtorrent.throttle" sd="$" width={460}>
  <p class="mb-3 text-[11px] text-dim">// limit in KiB/s · 0 = unlimited</p>

  <div class="fld">
    <div class="fld-l">↓ download</div>
    <label class="opt-row">
      <input type="number" min="0" bind:value={down} class="inp tabular-nums text-right" />
      <span class="text-[11px] text-dim">KiB/s</span>
    </label>
    <div class="mt-2 flex gap-1.5">
      {#each PRESETS as p (p.kib)}
        <button type="button" class="tbtn {down === p.kib ? 'solid' : ''}" onclick={() => (down = p.kib)}>{p.label}</button>
      {/each}
    </div>
  </div>

  <div class="fld" style="margin-bottom:0">
    <div class="fld-l">↑ upload</div>
    <label class="opt-row">
      <input type="number" min="0" bind:value={up} class="inp tabular-nums text-right" />
      <span class="text-[11px] text-dim">KiB/s</span>
    </label>
    <div class="mt-2 flex gap-1.5">
      {#each PRESETS as p (p.kib)}
        <button type="button" class="tbtn {up === p.kib ? 'solid' : ''}" onclick={() => (up = p.kib)}>{p.label}</button>
      {/each}
    </div>
  </div>

  {#snippet footer()}
    <button class="rd-btn sp" onclick={() => (open = false)}>CANCEL</button>
    <button class="tbtn acc" style={busy ? 'opacity:.4;pointer-events:none' : ''} onclick={apply}>{busy ? 'APPLYING…' : 'APPLY'}</button>
  {/snippet}
</Modal>
