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

<Modal bind:open title="Global rate limits">
  <div class="flex flex-col gap-4">
    <p class="text-sm text-muted-foreground">Set to 0 for unlimited.</p>
    <label class="flex items-center justify-between gap-3 text-sm">
      <span>Download</span>
      <span class="flex items-center gap-1">
        <input type="number" min="0" bind:value={down} class="h-9 w-28 rounded-md border bg-background px-2 text-right tabular-nums outline-none focus:ring-2 focus:ring-ring/50" />
        <span class="text-muted-foreground">KiB/s</span>
      </span>
    </label>
    <label class="flex items-center justify-between gap-3 text-sm">
      <span>Upload</span>
      <span class="flex items-center gap-1">
        <input type="number" min="0" bind:value={up} class="h-9 w-28 rounded-md border bg-background px-2 text-right tabular-nums outline-none focus:ring-2 focus:ring-ring/50" />
        <span class="text-muted-foreground">KiB/s</span>
      </span>
    </label>
    <div class="flex justify-end gap-2">
      <button onclick={() => (open = false)} class="rounded-md border px-3 py-1.5 text-sm hover:bg-accent">Cancel</button>
      <button onclick={apply} disabled={busy} class="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90 disabled:opacity-50">
        {busy ? 'Applying…' : 'Apply'}
      </button>
    </div>
  </div>
</Modal>
