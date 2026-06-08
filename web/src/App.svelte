<script lang="ts">
  import { onMount } from 'svelte'
  import { ModeWatcher, toggleMode } from 'mode-watcher'
  import { Toaster } from 'svelte-sonner'
  import { torrents } from '$lib/stores/torrents.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { view, matches, compare, type StatusFilter } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import { connectSSE } from '$lib/api/sse'
  import { api, bulk } from '$lib/api/client'
  import { short } from '$lib/format'
  import SpeedGraph from './components/SpeedGraph.svelte'
  import TorrentTable from './components/table/TorrentTable.svelte'
  import AddTorrentDialog from './components/toolbar/AddTorrentDialog.svelte'
  import ThrottleDialog from './components/toolbar/ThrottleDialog.svelte'
  import DetailPanel from './components/detail/DetailPanel.svelte'
  import InsightView from './components/insight/InsightView.svelte'

  let addOpen = $state(false)
  let throttleOpen = $state(false)
  let mainView = $state<'torrents' | 'insight'>('torrents')
  let searchActive = $state(false)
  let searchEl = $state<HTMLInputElement>()

  const startSel = () => bulk(selection.list(), api.start, 'Started')
  const stopSel = () => bulk(selection.list(), api.stop, 'Stopped')
  const removeSel = () => {
    const hs = selection.list()
    bulk(hs, api.remove, 'Removed').then(() => selection.clear())
  }

  let rtVersion = $state('')
  onMount(() => {
    const close = connectSSE()
    fetch('/api/version')
      .then((r) => r.json())
      .then((j) => (rtVersion = j?.data?.rtorrent ? `rtorrent ${j.data.rtorrent} · api ${j.data.api}` : ''))
      .catch(() => {})
    return close
  })

  const all = $derived([...torrents.map.values()])
  const visible = $derived.by(() => {
    const arr = all.filter((t) => matches(t, view))
    arr.sort((a, b) => compare(a, b, view.sortKey, view.sortDir))
    return arr
  })

  const counts = $derived.by(() => {
    const c = { all: all.length, downloading: 0, seeding: 0, stopped: 0, error: 0 }
    for (const t of all) {
      if (t.status === 'downloading') c.downloading++
      else if (t.status === 'seeding') c.seeding++
      else if (t.status === 'stopped' || t.status === 'paused') c.stopped++
      else if (t.status === 'error') c.error++
    }
    return c
  })

  const labels = $derived.by(() => {
    const m = new Map<string, number>()
    for (const t of all) if (t.label) m.set(t.label, (m.get(t.label) ?? 0) + 1)
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  })

  const statusFilters: { key: StatusFilter; label: string; mark: string; count: () => number }[] = [
    { key: 'all', label: 'ALL', mark: '✦', count: () => counts.all },
    { key: 'downloading', label: 'DOWNLOADING', mark: '▶', count: () => counts.downloading },
    { key: 'seeding', label: 'SEEDING', mark: '↑', count: () => counts.seeding },
    { key: 'stopped', label: 'STOPPED', mark: '■', count: () => counts.stopped },
    { key: 'error', label: 'ERROR', mark: '!', count: () => counts.error },
  ]

  const conn = $derived(globals.connection)
  const connDot = $derived(
    conn === 'live' ? 'bg-status-seed' : conn === 'reconnecting' ? 'bg-status-check' : 'bg-status-error',
  )
</script>

<ModeWatcher defaultMode="dark" />

<div class="flex h-svh flex-col">
  <!-- ───────── header (terminal) ───────── -->
  <header class="flex h-[54px] shrink-0 items-center gap-3 border-b border-line px-4">
    <div
      class="searchbar flex h-9 min-w-0 flex-1 cursor-text items-center gap-2 rounded-sm border border-line px-3 {searchActive || view.search ? 'active' : ''}"
      style="background:color-mix(in srgb, var(--primary) 3%, transparent)"
      onclick={() => searchEl?.focus()}
      role="searchbox"
      aria-label="filter torrents"
      tabindex="-1"
    >
      <span class="hidden text-[12.5px] text-dim sm:inline">~/torrents</span>
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
      <span class="glow-acc text-primary {searchActive || view.search ? 'caret-blink' : 'opacity-40'}">▋</span>
      {#if view.search}
        <span class="whitespace-nowrap text-[11.5px] text-dim">{visible.length} match{visible.length === 1 ? '' : 'es'}</span>
      {/if}
    </div>

    {#if selection.size > 0}
      <div class="flex shrink-0 items-center gap-1.5">
        <span class="text-[11.5px] text-dim">{selection.size} sel</span>
        <button class="tbtn" onclick={startSel} title="start">▶</button>
        <button class="tbtn" onclick={stopSel} title="stop">■</button>
        <button class="tbtn danger" onclick={removeSel} title="remove">✕</button>
        <button class="tbtn" onclick={() => selection.clear()} title="clear">⊘</button>
      </div>
    {/if}

    <div class="flex shrink-0 gap-2">
      <button class="tbtn {mainView === 'torrents' ? 'solid' : ''}" onclick={() => (mainView = 'torrents')}><span>≡</span> LIST</button>
      <button class="tbtn {mainView === 'insight' ? 'solid' : ''}" onclick={() => (mainView = 'insight')}><span>◫</span> INSIGHT</button>
      <button class="tbtn acc" onclick={() => (addOpen = true)}><span>+</span> ADD</button>
      <button class="tbtn" onclick={() => (throttleOpen = true)} title="rate limits"><span>⇅</span></button>
      <button class="tbtn" onclick={toggleMode} title="theme">
        <span class="hidden dark:inline">☀</span><span class="inline dark:hidden">☾</span>
      </button>
    </div>
  </header>

  {#if mainView === 'torrents'}
    <div class="flex min-h-0 flex-1">
      <!-- ───────── sidebar ───────── -->
      <aside class="hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-r border-line p-4 md:flex">
        <div class="brand">▚ TORUI<span class="ml-0.5 text-[13px] font-normal tracking-[0.04em] text-dim">::rtorrent</span></div>

        <div class="cap-box px-3 pb-3 pt-3.5">
          <div class="cap">transfer</div>
          <div class="flex gap-2.5">
            <div class="rate-box flex-1">
              <span class="text-[15px] leading-none">↓</span>
              <span class="glow-acc text-[16px] font-semibold">{short(globals.downRate)}<small>B/s</small></span>
            </div>
            <div class="rate-box up flex-1">
              <span class="text-[15px] leading-none">↑</span>
              <span class="glow-acc2 text-[16px] font-semibold">{short(globals.upRate)}<small>B/s</small></span>
            </div>
          </div>
          <div class="mt-3"><SpeedGraph dl={globals.dlHist} ul={globals.ulHist} /></div>
          <div class="mt-2 text-[10.5px] tracking-[0.04em] text-dim">Σ ↓{short(globals.downTotal)} &nbsp; ↑{short(globals.upTotal)}</div>
        </div>

        <div class="flex flex-col gap-px">
          <div class="mb-1.5 text-[10px] uppercase tracking-[0.16em] text-dim">// status</div>
          {#each statusFilters as f (f.key)}
            <div class="frow" class:on={view.status === f.key} onclick={() => (view.status = f.key)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.status = f.key)}>
              <span class="mk">{f.mark}</span>{f.label}<span class="ct">{f.count()}</span>
            </div>
          {/each}
        </div>

        {#if labels.length}
          <div class="flex flex-col gap-px">
            <div class="mb-1.5 text-[10px] uppercase tracking-[0.16em] text-dim">// labels</div>
            <div class="frow" class:on={view.label === null} onclick={() => (view.label = null)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.label = null)}>
              <span class="mk">·</span>all<span class="ct">{all.length}</span>
            </div>
            {#each labels as [name, count] (name)}
              <div class="frow" class:on={view.label === name} onclick={() => (view.label = name)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && (view.label = name)}>
                <span class="mk">·</span>{name}<span class="ct">{count}</span>
              </div>
            {/each}
          </div>
        {/if}
      </aside>

      <main class="min-w-0 flex-1"><TorrentTable rows={visible} /></main>
    </div>

    <DetailPanel />
  {:else}
    <div class="min-h-0 flex-1"><InsightView /></div>
  {/if}

  <!-- ───────── status line ───────── -->
  <footer class="flex h-7 shrink-0 items-center justify-between border-t border-line px-4 text-[11px] text-dim">
    <span class="flex items-center gap-2">
      <span class="inline-block size-1.5 rounded-full {connDot}" style="box-shadow:0 0 6px currentColor"></span>
      {conn} · {globals.torrentCount} torrents · {counts.seeding} seeding · {counts.downloading} downloading
    </span>
    <span>{rtVersion}</span>
  </footer>
</div>

<Toaster theme="dark" position="bottom-right" />
<AddTorrentDialog bind:open={addOpen} />
<ThrottleDialog bind:open={throttleOpen} />
