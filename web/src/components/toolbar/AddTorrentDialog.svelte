<script lang="ts">
  import Modal from '../ui/Modal.svelte'
  import { api, type FsEntry } from '$lib/api/client'
  import { config } from '$lib/stores/config.svelte'
  import { splitDest, filterDirs, cleanSaveTo } from '$lib/dirCombo'
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
    queriedDir = null
    srcEntries = []
    closeList()
  }

  // --- save-to directory combobox (live only when config.browse) -------------
  // A free-text path input augmented, when the server can serve confined
  // directory listings, with typeahead suggestions and one-level click drill-in.
  // dest stays the single source of truth: drill-in/select mutates it directly,
  // so what's shown is exactly what submits.
  let destEl = $state<HTMLInputElement>()
  let listOpen = $state(false)
  let srcEntries = $state<FsEntry[]>([])
  let queriedDir = $state<string | null>(null) // dir last listed (null = none yet)
  let truncated = $state(false)
  let active = $state(-1)
  let popStyle = $state('')
  let seq = 0
  let ac: AbortController | null = null
  let debounceT: ReturnType<typeof setTimeout> | undefined

  // Client-side filter over the loaded entries — instant per keystroke, no
  // network. The (capped) listing for the current dir is the candidate set.
  // splitDest/filterDirs/cleanSaveTo are extracted to $lib/dirCombo (unit-tested).
  const visible = $derived(filterDirs(srcEntries, splitDest(dest).leaf))

  async function runQuery(dir: string) {
    const my = ++seq // monotonic: drop any out-of-order earlier response
    ac?.abort()
    ac = new AbortController()
    const res = await api.browse(dir || undefined, ac.signal)
    if (my !== seq) return // superseded by a newer keystroke
    queriedDir = dir
    // Coerce away a null/absent list (defensive — the server emits []): a null
    // here would make `visible.length` throw and freeze the dropdown.
    srcEntries = (res ? (dir === '' ? res.roots : res.entries) : []) ?? []
    truncated = res?.truncated ?? false
    active = -1
    listOpen = config.browse
  }

  function refresh() {
    const { dir } = splitDest(dest)
    if (dir === queriedDir) return // same dir already loaded; `visible` re-filters live
    void runQuery(dir)
  }
  function onFocus() {
    if (!config.browse) return
    listOpen = true
    refresh()
  }
  function onInput() {
    if (!config.browse) return
    listOpen = true
    active = -1 // the filtered set changed; don't keep a stale highlight index
    clearTimeout(debounceT)
    debounceT = setTimeout(refresh, 140)
  }
  function choose(e: FsEntry) {
    dest = e.path + '/' // trailing slash => next listing shows this dir's children
    void runQuery(e.path)
    destEl?.focus()
  }
  function closeList() {
    listOpen = false
    active = -1
    ac?.abort()
  }
  function onComboKey(e: KeyboardEvent) {
    if (!config.browse) return
    if (e.key === 'Escape') {
      if (listOpen) {
        e.preventDefault()
        e.stopPropagation() // close the dropdown only — don't let App.svelte blur the field
        closeList()
      }
      return // closed: let it bubble (App blurs the input, as today)
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if (listOpen && active >= 0 && visible[active]) choose(visible[active]) // commit highlighted
      else void submit() // nothing highlighted -> submit the typed path verbatim
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (!listOpen) return onFocus()
      if (visible.length) active = (active + 1) % visible.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (listOpen && visible.length) active = active <= 0 ? visible.length - 1 : active - 1
      return
    }
    if (e.key === 'Home' && listOpen && visible.length) {
      e.preventDefault()
      active = 0
    } else if (e.key === 'End' && listOpen && visible.length) {
      e.preventDefault()
      active = visible.length - 1
    }
  }

  // Fixed-position popover anchored to the input: the modal has overflow:hidden,
  // so an absolutely-positioned dropdown from this near-bottom field would clip.
  function positionPop() {
    if (!destEl) return
    const r = destEl.getBoundingClientRect()
    popStyle = `top:${r.bottom + 4}px;left:${r.left}px;width:${r.width}px`
  }
  $effect(() => {
    if (!listOpen) return
    positionPop()
    const on = () => positionPop()
    window.addEventListener('resize', on)
    window.addEventListener('scroll', on, true) // capture: reposition on any ancestor scroll
    return () => {
      window.removeEventListener('resize', on)
      window.removeEventListener('scroll', on, true)
    }
  })
  // Tear the dropdown down whenever the modal closes.
  $effect(() => {
    if (!open) closeList()
  })

  async function submit() {
    if (!canSubmit || busy) return
    busy = true
    // Drill-in leaves a trailing slash ("/data/dl/"); rtorrent wants the bare dir.
    const saveTo = cleanSaveTo(dest)
    try {
      if (tab === 'file') {
        if (file) await api.addFile(file, label || undefined, start, saveTo)
        toast.success('Added 1 torrent')
      } else {
        const fn = tab === 'magnet' ? api.addMagnet : api.addURL
        const res = await Promise.allSettled(lines(tab === 'magnet' ? magnet : url).map((l) => fn(l, label || undefined, start, saveTo)))
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
      <div class="combo">
        <input
          bind:this={destEl}
          class="inp"
          bind:value={dest}
          placeholder="~/downloads (rtorrent default)"
          spellcheck="false"
          autocomplete="off"
          role={config.browse ? 'combobox' : undefined}
          aria-expanded={config.browse ? listOpen : undefined}
          aria-controls={config.browse ? 'dir-listbox' : undefined}
          aria-autocomplete={config.browse ? 'list' : undefined}
          aria-activedescendant={config.browse && listOpen && active >= 0 ? `dir-opt-${active}` : undefined}
          onfocus={onFocus}
          oninput={onInput}
          onkeydown={onComboKey}
          onblur={() => closeList()}
        />
        {#if config.browse && listOpen}
          <!-- pointerdown preventDefault keeps the input focused (fires before blur), so a click selects without the dropdown vanishing first -->
          <ul class="combo-pop" id="dir-listbox" role="listbox" aria-label="directories" style={popStyle} onpointerdown={(e) => e.preventDefault()}>
            {#if visible.length === 0}
              <li class="combo-empty">no matching folders</li>
            {:else}
              {#each visible as e, i (e.path)}
                <!-- svelte-ignore a11y_click_events_have_key_events -- keyboard handling lives on the combobox input (aria-activedescendant); options are not individually focusable per the WAI-ARIA combobox pattern -->
                <li id={`dir-opt-${i}`} role="option" aria-selected={i === active} class="combo-opt {i === active ? 'on' : ''}" onclick={() => choose(e)}>
                  <span class="combo-ico">▸</span>{e.name}
                </li>
              {/each}
              {#if truncated}<li class="combo-trunc">first {visible.length} shown · keep typing to narrow</li>{/if}
            {/if}
          </ul>
        {/if}
      </div>
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
