<script lang="ts">
  import Modal from '../ui/Modal.svelte'
  import { api } from '$lib/api/client'
  import { globals } from '$lib/stores/globals.svelte'

  let { open = $bindable(false) }: { open?: boolean } = $props()
  let down = $state(0) // KiB/s; 0 = unlimited
  let up = $state(0)
  let busy = $state(false)

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

<Modal bind:open title="rtorrent.throttle" sd="$" width={440}>
  <p class="mb-3 text-[11px] text-dim">// set to 0 for unlimited</p>
  <label class="opt-row mb-3">
    <span class="flex-1 text-[10px] uppercase tracking-[0.14em] text-dim">↓ download</span>
    <input type="number" min="0" bind:value={down} class="inp w-28 text-right tabular-nums" style="flex:none" />
    <span class="text-[11px] text-dim">KiB/s</span>
  </label>
  <label class="opt-row">
    <span class="flex-1 text-[10px] uppercase tracking-[0.14em] text-dim">↑ upload</span>
    <input type="number" min="0" bind:value={up} class="inp w-28 text-right tabular-nums" style="flex:none" />
    <span class="text-[11px] text-dim">KiB/s</span>
  </label>

  {#snippet footer()}
    <button class="rd-btn sp" onclick={() => (open = false)}>CANCEL</button>
    <button class="tbtn acc" style={busy ? 'opacity:.4;pointer-events:none' : ''} onclick={apply}>{busy ? 'APPLYING…' : 'APPLY'}</button>
  {/snippet}
</Modal>
