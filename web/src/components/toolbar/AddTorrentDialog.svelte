<script lang="ts">
  import Modal from '../ui/Modal.svelte'
  import { api } from '$lib/api/client'
  import { toast } from 'svelte-sonner'

  let { open = $bindable(false) }: { open?: boolean } = $props()

  type Tab = 'magnet' | 'file' | 'url'
  let tab = $state<Tab>('magnet')
  let magnet = $state('')
  let url = $state('')
  let file = $state<File | null>(null)
  let dest = $state('')
  let label = $state('')
  let start = $state(true)
  let busy = $state(false)
  let dragover = $state(false)
  let fileInput = $state<HTMLInputElement>()
  let taEl = $state<HTMLTextAreaElement>()

  const tabs: { key: Tab; glyph: string; label: string }[] = [
    { key: 'magnet', glyph: '◈', label: 'MAGNET' },
    { key: 'file', glyph: '▦', label: '.TORRENT' },
    { key: 'url', glyph: '↗', label: 'URL' },
  ]

  const lines = (s: string) =>
    s
      .split('\n')
      .map((x) => x.trim())
      .filter(Boolean)
  const count = $derived(tab === 'file' ? (file ? 1 : 0) : lines(tab === 'magnet' ? magnet : url).length)
  const canSubmit = $derived(count > 0)

  // auto-focus the active text field shortly after open / tab change
  $effect(() => {
    open
    tab
    if (open && tab !== 'file') setTimeout(() => taEl?.focus(), 60)
  })

  function reset() {
    magnet = ''
    url = ''
    file = null
    dest = ''
    label = ''
  }

  async function submit() {
    if (!canSubmit || busy) return
    busy = true
    try {
      if (tab === 'file') {
        if (file) await api.addFile(file, label || undefined, start, dest || undefined)
        toast.success('Added 1 torrent')
      } else {
        const fn = tab === 'magnet' ? api.addMagnet : api.addURL
        const res = await Promise.allSettled(lines(tab === 'magnet' ? magnet : url).map((l) => fn(l, label || undefined, start, dest || undefined)))
        const ok = res.filter((r) => r.status === 'fulfilled').length
        if (ok > 0) toast.success(`Added ${ok} torrent${ok > 1 ? 's' : ''}`)
      }
      open = false
      reset()
    } catch {
      /* per-request toast already shown */
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

<Modal bind:open title="rtorrent.add" sd="$" width={580}>
  <div class="add-tabs">
    {#each tabs as t (t.key)}
      <button class="add-tab {tab === t.key ? 'on' : ''}" onclick={() => (tab = t.key)}><span>{t.glyph}</span> {t.label}</button>
    {/each}
  </div>

  {#if tab === 'magnet'}
    <div class="fld">
      <div class="fld-l">magnet uri<span class="text-dim2">· one per line</span></div>
      <textarea bind:this={taEl} bind:value={magnet} class="ta" placeholder="magnet:?xt=urn:btih:…" spellcheck="false"></textarea>
    </div>
  {:else if tab === 'url'}
    <div class="fld">
      <div class="fld-l">torrent url<span class="text-dim2">· one per line</span></div>
      <textarea bind:this={taEl} bind:value={url} class="ta" placeholder="https://example.org/file.torrent" spellcheck="false"></textarea>
    </div>
  {:else}
    <div class="fld">
      <div class="fld-l">torrent file</div>
      {#if !file}
        <div
          class="drop {dragover ? 'over' : ''}"
          onclick={() => fileInput?.click()}
          ondragover={(e) => {
            e.preventDefault()
            dragover = true
          }}
          ondragleave={() => (dragover = false)}
          ondrop={onDrop}
          role="button"
          tabindex="0"
          onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && fileInput?.click()}
        >
          <div class="drop-ico">▦</div>
          <div class="drop-t">drop .torrent here, or click to browse</div>
          <div class="drop-h">files are parsed locally · never sent to a tracker</div>
        </div>
      {:else}
        <div class="filechip">
          <span class="fc-ico">▦</span>{file.name}
          <span class="fc-x" onclick={() => (file = null)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (file = null)}>✕</span>
        </div>
      {/if}
      <input bind:this={fileInput} type="file" accept=".torrent,application/x-bittorrent" class="hidden" onchange={(e) => (file = e.currentTarget.files?.[0] ?? null)} />
    </div>
  {/if}

  <div class="add-opts">
    <label class="opt-row">
      <span class="w-[58px] text-[10px] uppercase tracking-[0.14em] text-dim">save to</span>
      <input class="inp" bind:value={dest} placeholder="~/downloads (rtorrent default)" spellcheck="false" />
    </label>
    <label class="opt-row">
      <span class="w-[58px] text-[10px] uppercase tracking-[0.14em] text-dim">label</span>
      <input class="inp" bind:value={label} placeholder="optional" spellcheck="false" />
    </label>
    <div
      class="toggle {start ? 'on' : ''}"
      onclick={() => (start = !start)}
      role="switch"
      aria-checked={start}
      tabindex="0"
      onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && (start = !start)}
    >
      <span class="tg"><i></i></span> start immediately
    </div>
  </div>

  {#snippet footer()}
    <span class="add-foot-hint" class:text-acc2={count > 0}>
      {count > 0 ? `${count} torrent${count > 1 ? 's' : ''} ready` : 'paste a magnet, drop a file, or enter a url'}
    </span>
    <button class="rd-btn sp" onclick={() => (open = false)}>CANCEL</button>
    <button class="tbtn acc" style={canSubmit && !busy ? '' : 'opacity:.4;pointer-events:none'} onclick={submit}>
      {busy ? 'ADDING…' : `ADD ${count || ''}`.trim()}
    </button>
  {/snippet}
</Modal>
