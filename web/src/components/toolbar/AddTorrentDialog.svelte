<script lang="ts">
  import Modal from '../ui/Modal.svelte'
  import { api } from '$lib/api/client'
  import { Magnet, Link2, Upload } from '@lucide/svelte'

  let { open = $bindable(false) }: { open?: boolean } = $props()

  type Tab = 'magnet' | 'url' | 'file'
  let tab = $state<Tab>('magnet')
  let magnet = $state('')
  let url = $state('')
  let file = $state<File | null>(null)
  let label = $state('')
  let start = $state(true)
  let busy = $state(false)
  let dragover = $state(false)

  const tabs: { key: Tab; label: string; icon: typeof Magnet }[] = [
    { key: 'magnet', label: 'Magnet', icon: Magnet },
    { key: 'url', label: 'URL', icon: Link2 },
    { key: 'file', label: 'File', icon: Upload },
  ]

  const canSubmit = $derived(
    (tab === 'magnet' && magnet.trim() !== '') ||
      (tab === 'url' && url.trim() !== '') ||
      (tab === 'file' && file !== null),
  )

  async function submit() {
    if (!canSubmit) return
    busy = true
    try {
      if (tab === 'magnet') await api.addMagnet(magnet.trim(), label || undefined, start)
      else if (tab === 'url') await api.addURL(url.trim(), label || undefined, start)
      else if (file) await api.addFile(file, label || undefined, start)
      open = false
      magnet = ''
      url = ''
      file = null
      label = ''
    } catch {
      /* toast already shown */
    } finally {
      busy = false
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    dragover = false
    const f = e.dataTransfer?.files?.[0]
    if (f) {
      file = f
      tab = 'file'
    }
  }
</script>

<Modal bind:open title="Add torrent">
  <div class="flex flex-col gap-4">
    <div class="flex gap-1 rounded-md bg-secondary p-1">
      {#each tabs as t (t.key)}
        {@const Icon = t.icon}
        <button
          onclick={() => (tab = t.key)}
          class="flex flex-1 items-center justify-center gap-1.5 rounded px-2 py-1.5 text-sm transition
            {tab === t.key ? 'bg-card font-medium text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}"
        >
          <Icon class="size-4" />{t.label}
        </button>
      {/each}
    </div>

    {#if tab === 'magnet'}
      <textarea
        bind:value={magnet}
        rows="3"
        placeholder="magnet:?xt=urn:btih:…"
        class="w-full rounded-md border bg-background p-2 text-sm outline-none focus:ring-2 focus:ring-ring/50"
      ></textarea>
    {:else if tab === 'url'}
      <input
        bind:value={url}
        placeholder="https://example.org/file.torrent"
        class="w-full rounded-md border bg-background p-2 text-sm outline-none focus:ring-2 focus:ring-ring/50"
      />
    {:else}
      <label
        class="flex cursor-pointer flex-col items-center gap-2 rounded-md border-2 border-dashed p-6 text-center text-sm text-muted-foreground transition
          {dragover ? 'border-primary bg-primary/5' : ''}"
        ondragover={(e) => {
          e.preventDefault()
          dragover = true
        }}
        ondragleave={() => (dragover = false)}
        ondrop={onDrop}
      >
        <Upload class="size-6" />
        {#if file}
          <span class="font-medium text-foreground">{file.name}</span>
        {:else}
          <span>Drop a .torrent here, or click to browse</span>
        {/if}
        <input
          type="file"
          accept=".torrent,application/x-bittorrent"
          class="hidden"
          onchange={(e) => (file = e.currentTarget.files?.[0] ?? null)}
        />
      </label>
    {/if}

    <div class="flex items-center gap-3">
      <input
        bind:value={label}
        placeholder="Label (optional)"
        class="h-9 flex-1 rounded-md border bg-background px-2 text-sm outline-none focus:ring-2 focus:ring-ring/50"
      />
      <label class="flex select-none items-center gap-1.5 text-sm">
        <input type="checkbox" bind:checked={start} class="accent-primary" /> Start
      </label>
    </div>

    <div class="flex justify-end gap-2">
      <button onclick={() => (open = false)} class="rounded-md border px-3 py-1.5 text-sm hover:bg-accent">Cancel</button>
      <button
        onclick={submit}
        disabled={!canSubmit || busy}
        class="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90 disabled:opacity-50"
      >
        {busy ? 'Adding…' : 'Add'}
      </button>
    </div>
  </div>
</Modal>
