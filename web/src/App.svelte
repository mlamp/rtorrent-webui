<script lang="ts">
  import { onMount } from 'svelte'
  import { ModeWatcher, toggleMode } from 'mode-watcher'
  import { Toaster } from 'svelte-sonner'
  import { torrents } from '$lib/stores/torrents.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { view, matches, matchesExcept, isActive, compare, type StatusFilter } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import { detail } from '$lib/stores/detail.svelte'
  import { config } from '$lib/stores/config.svelte'
  import { connectSSE } from '$lib/api/sse'
  import { api, bulk, silentGet } from '$lib/api/client'
  import { short, trackerHost } from '$lib/format'
  import SpeedGraph from './components/SpeedGraph.svelte'
  import TorrentTable from './components/table/TorrentTable.svelte'
  import DetailModal from './components/detail/DetailModal.svelte'
  import AddTorrentDialog from './components/toolbar/AddTorrentDialog.svelte'
  import ThrottleDialog from './components/toolbar/ThrottleDialog.svelte'
  import HelpDialog from './components/ui/HelpDialog.svelte'
  import Brand from './components/ui/Brand.svelte'
  import InsightView from './components/insight/InsightView.svelte'

  let addOpen = $state(false)
  let throttleOpen = $state(false)
  let helpOpen = $state(false)
  let searchActive = $state(false)
  let searchEl = $state<HTMLInputElement>()

  // act on the current selection, or — when nothing is selected — the cursor row
  const targets = (): string[] => (selection.size ? selection.list() : view.cursor ? [view.cursor] : [])
  const startSel = () => bulk(targets(), api.start, 'Started')
  const stopSel = () => bulk(targets(), api.stop, 'Stopped')
  const removeSel = () => {
    const hs = targets()
    bulk(hs, api.remove, 'Removed').then(() => selection.clear())
  }

  let rtVersion = $state('')

  // Sidebar speed sparkline: fed the global /api/history series (same source +
  // time-based logic as the insight/detail charts), seeded on load so it's never
  // empty, refreshed on a slow cadence. The live ↓/↑ numbers stay on the SSE feed
  // (globals); the graph is 15m of context, so a ≤20s lag is invisible.
  type HistPoint = { t: number; down: number; up: number }
  let sidebarHist = $state<{ points: HistPoint[]; start: number; end: number }>({ points: [], start: 0, end: 0 })
  async function loadSidebarHist() {
    const d = await silentGet<{ points: HistPoint[]; start: number; end: number }>('/api/history?range=15m')
    if (d) sidebarHist = { points: d.points ?? [], start: d.start ?? 0, end: d.end ?? 0 }
  }

  onMount(() => {
    const close = connectSSE()
    // Instance name first (fast, rtorrent-independent) so the brand/title never
    // wait on — or get stuck behind — a slow/unreachable daemon.
    fetch('/api/config')
      .then((r) => r.json())
      .then((j) => (config.name = j?.data?.name ?? ''))
      .catch(() => {})
    fetch('/api/version')
      .then((r) => r.json())
      .then((j) => (rtVersion = j?.data?.rtorrent ? `rtorrent ${j.data.rtorrent} · api ${j.data.api}` : ''))
      .catch(() => {})
    loadSidebarHist()
    const histTimer = setInterval(loadSidebarHist, 20000)
    return () => {
      close()
      clearInterval(histTimer)
    }
  })

  // Live up/down speed in the browser tab title (Flood-style). A configured
  // instance name leads the title ("TV · TorUI"); unset keeps "TorUI · rtorrent".
  $effect(() => {
    const d = globals.downRate
    const u = globals.upRate
    const live = globals.connection === 'live'
    const name = config.name
    // Show only the non-zero side(s) — a pure seed shows just ↑, idle shows neither.
    const parts: string[] = []
    if (d > 0) parts.push(`↓ ${short(d)}B/s`)
    if (u > 0) parts.push(`↑ ${short(u)}B/s`)
    if (live && parts.length) {
      document.title = name ? `${parts.join(' ')} · ${name}` : `${parts.join(' ')} · TorUI`
    } else {
      document.title = name ? `${name} · TorUI` : 'TorUI · rtorrent'
    }
  })

  const all = $derived([...torrents.map.values()])
  const visible = $derived.by(() => {
    const arr = all.filter((t) => matches(t, view))
    arr.sort((a, b) => compare(a, b, view.sortKey, view.sortDir))
    return arr
  })

  // ── faceted sidebar counts ───────────────────────────────────────────────
  // Each facet's counts are computed over the set matching every OTHER active
  // filter (status excludes status, etc.), so filtering by tracker narrows the
  // status counts and vice-versa. Option lists still come from the full set so a
  // value never disappears (count just goes to 0).
  const statusBase = $derived(all.filter((t) => matchesExcept(t, view, 'status')))
  const labelBase = $derived(all.filter((t) => matchesExcept(t, view, 'label')))
  const trackerBase = $derived(all.filter((t) => matchesExcept(t, view, 'tracker')))

  const counts = $derived.by(() => {
    const c = { all: statusBase.length, active: 0, downloading: 0, seeding: 0, stopped: 0, error: 0 }
    for (const t of statusBase) {
      if (isActive(t)) c.active++
      if (t.status === 'downloading') c.downloading++
      else if (t.status === 'seeding') c.seeding++
      else if (t.status === 'stopped' || t.status === 'paused') c.stopped++
      else if (t.status === 'error') c.error++
    }
    return c
  })

  const labels = $derived.by(() => {
    const present = [...new Set(all.map((t) => t.label).filter(Boolean))].sort((a, b) => a.localeCompare(b))
    const m = new Map<string, number>()
    for (const t of labelBase) if (t.label) m.set(t.label, (m.get(t.label) ?? 0) + 1)
    return present.map((name) => [name, m.get(name) ?? 0] as [string, number])
  })

  const trackers = $derived.by(() => {
    const present = [...new Set(all.map((t) => trackerHost(t.tracker)).filter(Boolean))].sort((a, b) => a.localeCompare(b))
    const m = new Map<string, number>()
    for (const t of trackerBase) {
      const h = trackerHost(t.tracker)
      if (h) m.set(h, (m.get(h) ?? 0) + 1)
    }
    return present.map((host) => [host, m.get(host) ?? 0] as [string, number])
  })

  const statusFilters: { key: StatusFilter; label: string; mark: string; count: () => number }[] = [
    { key: 'all', label: 'ALL', mark: '✦', count: () => counts.all },
    { key: 'active', label: 'ACTIVE', mark: '⇅', count: () => counts.active },
    { key: 'downloading', label: 'DOWNLOADING', mark: '▶', count: () => counts.downloading },
    { key: 'seeding', label: 'SEEDING', mark: '↑', count: () => counts.seeding },
    { key: 'stopped', label: 'STOPPED', mark: '■', count: () => counts.stopped },
    { key: 'error', label: 'ERROR', mark: '!', count: () => counts.error },
  ]

  const conn = $derived(globals.connection)
  const connDot = $derived(
    conn === 'live' ? 'bg-status-seed' : conn === 'reconnecting' ? 'bg-status-check' : 'bg-status-error',
  )

  // ── global keyboard ─────────────────────────────────────────────────────────
  function selectAllVisible() {
    const allSel = visible.length > 0 && visible.every((t) => selection.has(t.hash))
    if (allSel) selection.clear()
    else selection.replace(visible.map((t) => t.hash))
  }

  function onKey(e: KeyboardEvent) {
    // Let the browser/OS own every modifier combo (Cmd/Ctrl+R reload, Cmd+L, Ctrl+F,
    // Cmd+A, …). All of the app's shortcuts are unmodified keys, so a held
    // meta/ctrl/alt means "not ours".
    if (e.metaKey || e.ctrlKey || e.altKey) return
    const tag = ((e.target as HTMLElement)?.tagName || '').toUpperCase()
    const typing = tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT'

    if (e.key === 'Escape') {
      if (typing) return (e.target as HTMLElement).blur()
      if (helpOpen) return (helpOpen = false)
      if (addOpen) return (addOpen = false)
      if (throttleOpen) return (throttleOpen = false)
      if (detail.activeHash) return detail.close()
      if (selection.size) return selection.clear()
      return
    }
    if (typing || addOpen || throttleOpen) return
    if (e.key === '?') {
      e.preventDefault()
      helpOpen = !helpOpen
      return
    }
    if (helpOpen) return
    // when the detail modal is open, swallow nav (Escape handled above)
    if (detail.activeHash) return

    const rows = visible
    const idx = rows.findIndex((r) => r.hash === view.cursor)
    const move = (i: number) => {
      const c = Math.max(0, Math.min(rows.length - 1, i))
      if (rows[c]) view.cursor = rows[c].hash
    }
    switch (e.key) {
      case '/':
        e.preventDefault()
        searchEl?.focus()
        break
      case 'j':
      case 'ArrowDown':
        e.preventDefault()
        move(idx < 0 ? 0 : idx + 1)
        break
      case 'k':
      case 'ArrowUp':
        e.preventDefault()
        move(idx < 0 ? 0 : idx - 1)
        break
      case 'x':
      case ' ':
        e.preventDefault()
        if (view.cursor) selection.toggle(view.cursor)
        break
      case 'o':
      case 'Enter':
        e.preventDefault()
        if (view.cursor) detail.open(view.cursor)
        break
      case 'a':
        e.preventDefault()
        addOpen = true
        break
      case 'v':
        e.preventDefault()
        view.cycleMode()
        break
      case '*':
        e.preventDefault()
        selectAllVisible()
        break
      case 'p':
        e.preventDefault()
        stopSel()
        break
      case 'r':
        e.preventDefault()
        startSel()
        break
      case 'Backspace':
      case 'Delete':
        e.preventDefault()
        removeSel()
        break
    }
  }

  // keep the keyboard cursor scrolled into view (manual scrollTop, never scrollIntoView)
  $effect(() => {
    const h = view.cursor
    if (!h) return
    const cont = document.querySelector('[data-list]') as HTMLElement | null
    if (!cont) return
    const el = cont.querySelector(`[data-torrent="${CSS.escape(h)}"]`) as HTMLElement | null
    if (!el) return
    const er = el.getBoundingClientRect()
    const cr = cont.getBoundingClientRect()
    if (er.top < cr.top + 4) cont.scrollTop += er.top - cr.top - 10
    else if (er.bottom > cr.bottom - 4) cont.scrollTop += er.bottom - cr.bottom + 10
  })

  const modalTorrent = $derived(detail.activeHash ? torrents.map.get(detail.activeHash) : undefined)
  const hintVisible = $derived(!addOpen && !throttleOpen && !helpOpen && !detail.activeHash)
</script>

<svelte:window onkeydown={onKey} />
<ModeWatcher defaultMode="dark" />

<div class="flex h-svh flex-col">
  <div class="flex min-h-0 flex-1">
    <!-- ───────── sidebar (full-height left rail; list only) ───────── -->
    {#if view.mode !== 'insight'}
      <aside class="hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-r border-line px-4 pb-[18px] md:flex">
        <!-- brand sits in a 54px-tall slot matching the header height, so it doesn't
             shift vertically when switching list ↔ insight (where it lives in the
             header). -mx-4 lets the divider span the full sidebar width. -->
        <div class="-mx-4 flex h-[54px] shrink-0 items-center border-b border-line px-4"><Brand /></div>

        <div class="cap-box px-[13px] pb-[11px] pt-[13px]">
          <div class="cap">transfer</div>
          <div class="flex gap-2.5">
            <div class="rate-box flex-1">
              <span class="text-[15px] leading-none">↓</span>
              <span class="glow-acc text-[18px] font-semibold tracking-[-0.01em]">{short(globals.downRate)}<small>B/s</small></span>
            </div>
            <div class="rate-box up flex-1">
              <span class="text-[15px] leading-none">↑</span>
              <span class="glow-acc2 text-[18px] font-semibold tracking-[-0.01em]">{short(globals.upRate)}<small>B/s</small></span>
            </div>
          </div>
          <div class="mt-[11px]"><SpeedGraph points={sidebarHist.points} start={sidebarHist.start} end={sidebarHist.end} /></div>
          <div class="mt-2 text-[10.5px] tracking-[0.04em] text-dim">Σ ↓{short(globals.downTotal)} &nbsp; ↑{short(globals.upTotal)}</div>
        </div>

        <div class="flex flex-col gap-px">
          <div class="mb-[7px] text-[10px] uppercase tracking-[0.16em] text-dim">// status</div>
          {#each statusFilters as f (f.key)}
            <div class="frow" class:on={view.status === f.key} onclick={() => (view.status = f.key)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.status = f.key)}>
              <span class="mk">{f.mark}</span>{f.label}<span class="ct">{f.count()}</span>
            </div>
          {/each}
        </div>

        {#if labels.length}
          <div class="flex flex-col gap-px">
            <div class="mb-[7px] text-[10px] uppercase tracking-[0.16em] text-dim">// labels</div>
            <div class="frow" class:on={view.label === null} onclick={() => (view.label = null)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.label = null)}>
              <span class="mk">·</span>all<span class="ct">{labelBase.length}</span>
            </div>
            {#each labels as [name, count] (name)}
              <div class="frow" class:on={view.label === name} onclick={() => (view.label = name)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.label = name)}>
                <span class="mk">·</span>{name}<span class="ct">{count}</span>
              </div>
            {/each}
          </div>
        {/if}

        {#if trackers.length}
          <div class="flex flex-col gap-px">
            <div class="mb-[7px] text-[10px] uppercase tracking-[0.16em] text-dim">// tracker</div>
            <div class="frow" class:on={view.tracker === null} onclick={() => (view.tracker = null)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.tracker = null)}>
              <span class="mk">·</span>all<span class="ct">{trackerBase.length}</span>
            </div>
            {#each trackers as [host, count] (host)}
              <div class="frow" class:on={view.tracker === host} onclick={() => (view.tracker = host)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.tracker = host)}>
                <span class="mk">·</span>{host}<span class="ct">{count}</span>
              </div>
            {/each}
          </div>
        {/if}
      </aside>
    {/if}

    <!-- ───────── main column (top bar lives here; inset for list, full-width for insight) ───────── -->
    <main class="flex min-w-0 flex-1 flex-col">
      <header class="flex h-[54px] shrink-0 items-center gap-[14px] border-b border-line px-4">
        {#if view.mode === 'insight'}
          <!-- insight has no torrent list to filter; show the brand where search would be
               (the sidebar — and its brand — is hidden in this mode) -->
          <Brand class="min-w-0 flex-1" />
        {:else}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            class="searchbar flex h-9 min-w-0 flex-1 cursor-text items-center gap-2 rounded-md border border-line px-[13px] {searchActive || view.search ? 'active' : ''}"
            style="background:color-mix(in srgb, var(--primary) 3%, transparent)"
            onclick={() => searchEl?.focus()}
            role="searchbox"
            aria-label="filter torrents"
            tabindex="-1"
          >
            <span class="text-[12.5px] text-dim">~/torrents</span>
            <span class="text-primary">$</span>
            <span class="text-acc2">grep</span>
            <input
              bind:this={searchEl}
              bind:value={view.search}
              onfocus={() => (searchActive = true)}
              onblur={() => (searchActive = false)}
              class="min-w-[40px] flex-1 border-0 bg-transparent text-[13px] text-foreground outline-none"
              style="caret-color:transparent"
              spellcheck="false"
            />
            <span class="caret">▋</span>
            {#if view.search}
              <span class="whitespace-nowrap text-[11.5px] text-dim">{visible.length} match{visible.length === 1 ? '' : 'es'}</span>
            {/if}
          </div>
        {/if}

        <div class="flex shrink-0 gap-2">
          <button class="tbtn {view.mode === 'list' ? 'solid' : ''}" onclick={() => (view.mode = 'list')}><span>≡</span> LIST</button>
          <button class="tbtn {view.mode === 'insight' ? 'solid' : ''}" onclick={() => (view.mode = 'insight')}><span>▤</span> INSIGHT</button>
          <button class="tbtn acc" onclick={() => (addOpen = true)}><span>+</span> ADD</button>
          <button class="tbtn" onclick={() => (throttleOpen = true)} title="rate limits"><span>⇅</span></button>
          <button class="tbtn" onclick={toggleMode} title="theme">
            <span class="hidden dark:inline">☀</span><span class="inline dark:hidden">☾</span>
          </button>
        </div>
      </header>

      {#if selection.size > 0 && view.mode !== 'insight'}
        <div class="bulkbar">
          <span class="bulk-count"><b>{selection.size}</b> selected</span>
          <span class="bulk-clear" onclick={() => selection.clear()} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && selection.clear()}>✕ clear</span>
          <div class="bulk-actions">
            <button class="bbtn" onclick={startSel}>▶ RESUME</button>
            <button class="bbtn" onclick={stopSel}>⏸ PAUSE</button>
            <button class="bbtn danger" onclick={removeSel}>✕ REMOVE</button>
          </div>
        </div>
      {/if}

      <!-- content fills the space below the header; gives the views a definite
           height so TorrentTable's h-full / InsightView flex:1 resolve correctly -->
      <div class="flex min-h-0 flex-1 flex-col">
        {#if view.mode === 'list'}
          <TorrentTable rows={visible} />
        {:else}
          <InsightView />
        {/if}
      </div>
    </main>
  </div>

  <!-- ───────── status line ───────── -->
  <footer class="flex h-7 shrink-0 items-center justify-between border-t border-line px-4 text-[11px] text-dim">
    <span class="flex items-center gap-2">
      <span class="inline-block size-1.5 rounded-full {connDot}" style="box-shadow:0 0 6px currentColor"></span>
      {conn} · {globals.torrentCount} torrents · {counts.seeding} seeding · {counts.downloading} downloading
    </span>
    <span>{rtVersion}</span>
  </footer>
</div>

{#if hintVisible}
  <button class="kbd-hint" type="button" title="keyboard shortcuts" aria-label="keyboard shortcuts" onclick={() => (helpOpen = true)}><span class="kbd">?</span></button>
{/if}

<Toaster theme="dark" position="bottom-right" />
<AddTorrentDialog bind:open={addOpen} />
<ThrottleDialog bind:open={throttleOpen} />
<HelpDialog bind:open={helpOpen} />
{#if modalTorrent}
  <DetailModal t={modalTorrent} />
{/if}
