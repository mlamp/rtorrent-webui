<script lang="ts">
  import { onMount } from 'svelte'
  import { ModeWatcher, toggleMode } from 'mode-watcher'
  import { Toaster } from 'svelte-sonner'
  import {
    Sun, Moon, Plus, ArrowDown, ArrowUp, Circle, Search,
    Layers, Download, CheckCircle2, PauseCircle, AlertTriangle,
    Play, Square, Trash2, Gauge, X, Activity, List,
  } from '@lucide/svelte'
  import { torrents } from '$lib/stores/torrents.svelte'
  import { globals } from '$lib/stores/globals.svelte'
  import { view, matches, compare, type StatusFilter } from '$lib/stores/view.svelte'
  import { selection } from '$lib/stores/selection.svelte'
  import { connectSSE } from '$lib/api/sse'
  import { api, bulk } from '$lib/api/client'
  import { rate } from '$lib/format'
  import TorrentTable from './components/table/TorrentTable.svelte'
  import AddTorrentDialog from './components/toolbar/AddTorrentDialog.svelte'
  import ThrottleDialog from './components/toolbar/ThrottleDialog.svelte'
  import DetailPanel from './components/detail/DetailPanel.svelte'
  import InsightView from './components/insight/InsightView.svelte'

  let addOpen = $state(false)
  let throttleOpen = $state(false)
  let mainView = $state<'torrents' | 'insight'>('torrents')

  function startSel() {
    bulk(selection.list(), api.start, 'Started')
  }
  function stopSel() {
    bulk(selection.list(), api.stop, 'Stopped')
  }
  function removeSel() {
    const hs = selection.list()
    bulk(hs, api.remove, 'Removed').then(() => selection.clear())
  }

  let rtVersion = $state('')
  onMount(() => {
    const close = connectSSE()
    fetch('/api/version')
      .then((r) => r.json())
      .then((j) => (rtVersion = j?.data?.rtorrent ? `rtorrent ${j.data.rtorrent} · API ${j.data.api}` : ''))
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

  const LABEL_COLORS = ['#89b4fa', '#f9e2af', '#a6e3a1', '#f5c2e7', '#fab387', '#94e2d5', '#cba6f7']
  const labels = $derived.by(() => {
    const m = new Map<string, number>()
    for (const t of all) if (t.label) m.set(t.label, (m.get(t.label) ?? 0) + 1)
    return [...m.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  })

  const filters: { key: StatusFilter; label: string; icon: typeof Layers; count: () => number }[] = [
    { key: 'all', label: 'All', icon: Layers, count: () => counts.all },
    { key: 'downloading', label: 'Downloading', icon: Download, count: () => counts.downloading },
    { key: 'seeding', label: 'Seeding', icon: CheckCircle2, count: () => counts.seeding },
    { key: 'stopped', label: 'Stopped', icon: PauseCircle, count: () => counts.stopped },
    { key: 'error', label: 'Error', icon: AlertTriangle, count: () => counts.error },
  ]

  const conn = $derived(globals.connection)
  const connColor = $derived(
    conn === 'live' ? 'text-status-seed border-status-seed/40 bg-status-seed/10'
    : conn === 'reconnecting' ? 'text-status-check border-status-check/40 bg-status-check/10'
    : 'text-status-error border-status-error/40 bg-status-error/10',
  )
</script>

<ModeWatcher />

<div class="flex h-svh flex-col bg-background text-foreground">
  <header class="flex h-14 shrink-0 items-center gap-4 border-b bg-card px-4">
    <div class="flex items-center gap-2 font-semibold tracking-tight">
      <span class="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground">
        <Download class="size-4" />
      </span>
      <span>rtorrent<span class="text-primary">-webui</span></span>
    </div>

    <nav class="ml-2 flex gap-1 rounded-md bg-secondary p-1">
      <button
        onclick={() => (mainView = 'torrents')}
        class="flex items-center gap-1.5 rounded px-2.5 py-1 text-sm transition {mainView === 'torrents' ? 'bg-card font-medium text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}"
      ><List class="size-4" />Torrents</button>
      <button
        onclick={() => (mainView = 'insight')}
        class="flex items-center gap-1.5 rounded px-2.5 py-1 text-sm transition {mainView === 'insight' ? 'bg-card font-medium text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}"
      ><Activity class="size-4" />Insight</button>
    </nav>

    <button
      onclick={() => (addOpen = true)}
      class="ml-2 inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90"
    >
      <Plus class="size-4" /> Add
    </button>

    {#if selection.size > 0}
      <div class="flex items-center gap-1 border-l pl-2">
        <span class="px-1 text-sm text-muted-foreground">{selection.size} selected</span>
        <button onclick={startSel} title="Start" class="grid size-8 place-items-center rounded-md text-status-seed transition hover:bg-accent"><Play class="size-4" /></button>
        <button onclick={stopSel} title="Stop" class="grid size-8 place-items-center rounded-md text-muted-foreground transition hover:bg-accent"><Square class="size-4" /></button>
        <button onclick={removeSel} title="Remove" class="grid size-8 place-items-center rounded-md text-status-error transition hover:bg-accent"><Trash2 class="size-4" /></button>
        <button onclick={() => selection.clear()} title="Clear selection" class="grid size-8 place-items-center rounded-md text-muted-foreground transition hover:bg-accent"><X class="size-4" /></button>
      </div>
    {/if}

    <div class="relative ml-2 hidden md:block">
      <Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <input
        bind:value={view.search}
        placeholder="Filter torrents…"
        class="h-9 w-64 rounded-md border bg-background pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/50"
      />
    </div>

    <div class="ml-auto flex items-center gap-4 text-sm">
      <div class="hidden items-center gap-1.5 text-status-download sm:flex" title="Download rate">
        <ArrowDown class="size-4" /> <span class="font-medium tabular-nums">{rate(globals.downRate)}</span>
      </div>
      <div class="hidden items-center gap-1.5 text-status-seed sm:flex" title="Upload rate">
        <ArrowUp class="size-4" /> <span class="font-medium tabular-nums">{rate(globals.upRate)}</span>
      </div>

      <span class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium {connColor}">
        <Circle class="size-2 fill-current" /> {conn}
      </span>

      <button
        onclick={() => (throttleOpen = true)}
        title="Global rate limits"
        class="grid size-9 place-items-center rounded-md border bg-background text-muted-foreground transition hover:bg-accent hover:text-foreground"
      >
        <Gauge class="size-4" />
      </button>

      <button
        onclick={toggleMode}
        title="Toggle theme"
        class="grid size-9 place-items-center rounded-md border bg-background text-muted-foreground transition hover:bg-accent hover:text-foreground"
      >
        <Sun class="hidden size-4 dark:block" />
        <Moon class="block size-4 dark:hidden" />
      </button>
    </div>
  </header>

  {#if mainView === 'torrents'}
  <div class="flex min-h-0 flex-1">
    <aside class="hidden w-56 shrink-0 overflow-y-auto border-r bg-card/50 p-3 md:block">
      <p class="px-2 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Status</p>
      <nav class="flex flex-col">
        {#each filters as f (f.key)}
          {@const Icon = f.icon}
          <button
            onclick={() => (view.status = f.key)}
            class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition hover:bg-accent
              {view.status === f.key ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'}"
          >
            <Icon class="size-4" />
            <span class="flex-1 text-left">{f.label}</span>
            <span class="tabular-nums text-xs text-muted-foreground">{f.count()}</span>
          </button>
        {/each}
      </nav>

      {#if labels.length}
        <p class="px-2 pb-1 pt-4 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Labels</p>
        <nav class="flex flex-col">
          <button
            onclick={() => (view.label = null)}
            class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition hover:bg-accent {view.label === null ? 'text-foreground' : 'text-muted-foreground'}"
          >
            <span class="size-2.5 rounded-full bg-muted-foreground"></span>
            <span class="flex-1 text-left">All labels</span>
          </button>
          {#each labels as [name, count], i (name)}
            <button
              onclick={() => (view.label = name)}
              class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition hover:bg-accent {view.label === name ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'}"
            >
              <span class="size-2.5 rounded-full" style="background:{LABEL_COLORS[i % LABEL_COLORS.length]}"></span>
              <span class="flex-1 text-left">{name}</span>
              <span class="tabular-nums text-xs">{count}</span>
            </button>
          {/each}
        </nav>
      {/if}
    </aside>

    <main class="min-w-0 flex-1">
      <TorrentTable rows={visible} />
    </main>
  </div>

  <DetailPanel />
  {:else}
    <div class="min-h-0 flex-1"><InsightView /></div>
  {/if}

  <footer class="flex h-7 shrink-0 items-center justify-between border-t bg-card px-4 text-xs text-muted-foreground">
    <span>{globals.torrentCount} torrents · {counts.seeding} seeding · {counts.downloading} downloading</span>
    <span class="tabular-nums">{rtVersion}</span>
  </footer>
</div>

<Toaster richColors theme="system" position="bottom-right" />
<AddTorrentDialog bind:open={addOpen} />
<ThrottleDialog bind:open={throttleOpen} />
