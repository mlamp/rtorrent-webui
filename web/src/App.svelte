<script lang="ts">
  import { ModeWatcher, toggleMode } from 'mode-watcher'
  import {
    Sun,
    Moon,
    Plus,
    ArrowDown,
    ArrowUp,
    HardDrive,
    Circle,
    Layers,
    Download,
    CheckCircle2,
    PauseCircle,
    AlertTriangle,
    Search,
  } from '@lucide/svelte'

  // --- placeholder data: a static skeleton until the SSE store lands (M1/M2) ---
  type Status = 'download' | 'seed' | 'stopped' | 'error' | 'check'
  const statusMeta: Record<Status, { label: string; color: string }> = {
    download: { label: 'Downloading', color: 'text-status-download' },
    seed: { label: 'Seeding', color: 'text-status-seed' },
    stopped: { label: 'Stopped', color: 'text-status-stopped' },
    error: { label: 'Error', color: 'text-status-error' },
    check: { label: 'Checking', color: 'text-status-check' },
  }

  const torrents = [
    { name: 'debian-12.5.0-amd64-netinst.iso', size: '658 MB', done: 100, down: '0 B/s', up: '1.2 MB/s', ratio: '4.81', peers: '0 / 38', status: 'seed' as Status, label: 'linux' },
    { name: 'Ubuntu 24.04.2 Desktop (amd64)', size: '5.9 GB', done: 62, down: '11.4 MB/s', up: '320 KB/s', ratio: '0.18', peers: '24 / 211', status: 'download' as Status, label: 'linux' },
    { name: 'Sintel.2010.2160p.UHD.BluRay.x265', size: '14.2 GB', done: 100, down: '0 B/s', up: '4.7 MB/s', ratio: '11.02', peers: '6 / 144', status: 'seed' as Status, label: 'movies' },
    { name: 'archlinux-2026.06.01-x86_64.iso', size: '1.1 GB', done: 100, down: '0 B/s', up: '0 B/s', ratio: '2.34', peers: '0 / 0', status: 'stopped' as Status, label: 'linux' },
    { name: 'Big Buck Bunny (1080p, h264)', size: '355 MB', done: 38, down: '0 B/s', up: '0 B/s', ratio: '0.00', peers: '0 / 12', status: 'error' as Status, label: '' },
    { name: 'NASA Voyager Mission Archive [2025]', size: '88.6 GB', done: 7, down: '0 B/s', up: '0 B/s', ratio: '0.00', peers: '0 / 0', status: 'check' as Status, label: 'science' },
    { name: 'fedora-workstation-40-x86_64.iso', size: '2.1 GB', done: 100, down: '0 B/s', up: '880 KB/s', ratio: '6.13', peers: '3 / 57', status: 'seed' as Status, label: 'linux' },
    { name: 'Cosmos Laundromat (4K, AV1)', size: '7.8 GB', done: 91, down: '6.0 MB/s', up: '210 KB/s', ratio: '0.44', peers: '18 / 96', status: 'download' as Status, label: 'movies' },
  ]

  const filters = [
    { label: 'All', icon: Layers, count: 1071 },
    { label: 'Downloading', icon: Download, count: 2 },
    { label: 'Seeding', icon: CheckCircle2, count: 1063 },
    { label: 'Stopped', icon: PauseCircle, count: 5 },
    { label: 'Error', icon: AlertTriangle, count: 1 },
  ]
  const labels = [
    { name: 'linux', count: 412, color: '#89b4fa' },
    { name: 'movies', count: 318, color: '#f9e2af' },
    { name: 'science', count: 67, color: '#a6e3a1' },
    { name: 'music', count: 211, color: '#f5c2e7' },
  ]

  let active = $state('All')
</script>

<ModeWatcher />

<div class="flex h-svh flex-col bg-background text-foreground">
  <!-- ───────────── top bar ───────────── -->
  <header class="flex h-14 shrink-0 items-center gap-4 border-b bg-card px-4">
    <div class="flex items-center gap-2 font-semibold tracking-tight">
      <span class="grid size-7 place-items-center rounded-md bg-primary text-primary-foreground">
        <Download class="size-4" />
      </span>
      <span>rtorrent<span class="text-primary">-webui</span></span>
    </div>

    <button
      class="ml-2 inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition hover:opacity-90"
    >
      <Plus class="size-4" /> Add
    </button>

    <div class="relative ml-2 hidden md:block">
      <Search class="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
      <input
        placeholder="Filter torrents…"
        class="h-9 w-64 rounded-md border bg-background pl-8 pr-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring/50"
      />
    </div>

    <div class="ml-auto flex items-center gap-4 text-sm">
      <div class="hidden items-center gap-1.5 text-status-download sm:flex" title="Download rate">
        <ArrowDown class="size-4" /> <span class="font-medium tabular-nums">17.4 MB/s</span>
      </div>
      <div class="hidden items-center gap-1.5 text-status-seed sm:flex" title="Upload rate">
        <ArrowUp class="size-4" /> <span class="font-medium tabular-nums">7.3 MB/s</span>
      </div>
      <div class="hidden items-center gap-1.5 text-muted-foreground lg:flex" title="Free disk">
        <HardDrive class="size-4" /> <span class="tabular-nums">3.1 TB free</span>
      </div>

      <span class="inline-flex items-center gap-1.5 rounded-full border border-status-seed/40 bg-status-seed/10 px-2 py-0.5 text-xs font-medium text-status-seed">
        <Circle class="size-2 fill-current" /> live
      </span>

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

  <!-- ───────────── body ───────────── -->
  <div class="flex min-h-0 flex-1">
    <!-- sidebar -->
    <aside class="hidden w-56 shrink-0 overflow-y-auto border-r bg-card/50 p-3 md:block">
      <p class="px-2 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Status</p>
      <nav class="flex flex-col">
        {#each filters as f (f.label)}
          {@const Icon = f.icon}
          <button
            onclick={() => (active = f.label)}
            class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition hover:bg-accent
              {active === f.label ? 'bg-accent font-medium text-foreground' : 'text-muted-foreground'}"
          >
            <Icon class="size-4" />
            <span class="flex-1 text-left">{f.label}</span>
            <span class="tabular-nums text-xs text-muted-foreground">{f.count}</span>
          </button>
        {/each}
      </nav>

      <p class="px-2 pb-1 pt-4 text-xs font-semibold uppercase tracking-wider text-muted-foreground">Labels</p>
      <nav class="flex flex-col">
        {#each labels as l (l.name)}
          <button class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition hover:bg-accent">
            <span class="size-2.5 rounded-full" style="background:{l.color}"></span>
            <span class="flex-1 text-left">{l.name}</span>
            <span class="tabular-nums text-xs">{l.count}</span>
          </button>
        {/each}
      </nav>
    </aside>

    <!-- torrent table -->
    <main class="min-w-0 flex-1 overflow-auto">
      <table class="w-full border-collapse text-sm">
        <thead class="sticky top-0 z-10 bg-card text-xs uppercase tracking-wide text-muted-foreground">
          <tr class="border-b">
            <th class="w-8 px-3 py-2 text-left font-medium"></th>
            <th class="w-full px-3 py-2 text-left font-medium">Name</th>
            <th class="px-3 py-2 text-right font-medium">Size</th>
            <th class="w-48 px-3 py-2 text-left font-medium">Progress</th>
            <th class="px-3 py-2 text-right font-medium">Down</th>
            <th class="px-3 py-2 text-right font-medium">Up</th>
            <th class="px-3 py-2 text-right font-medium">Ratio</th>
            <th class="px-3 py-2 text-right font-medium">Seeds/Peers</th>
            <th class="px-3 py-2 text-left font-medium">Status</th>
            <th class="px-3 py-2 text-left font-medium">Label</th>
          </tr>
        </thead>
        <tbody>
          {#each torrents as t, i (t.name)}
            <tr class="border-b border-border/60 transition hover:bg-accent/50 {i % 2 ? 'bg-card/40' : ''}">
              <td class="px-3 py-2"><input type="checkbox" class="accent-primary" /></td>
              <td class="max-w-0 truncate px-3 py-2 font-medium" title={t.name}>{t.name}</td>
              <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums text-muted-foreground">{t.size}</td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-secondary">
                    <div
                      class="h-full rounded-full"
                      style="width:{t.done}%; background:var(--status-{t.status === 'download' ? 'download' : t.status === 'error' ? 'error' : t.status === 'check' ? 'check' : 'seed'})"
                    ></div>
                  </div>
                  <span class="w-9 text-right text-xs tabular-nums text-muted-foreground">{t.done}%</span>
                </div>
              </td>
              <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums {t.down !== '0 B/s' ? 'text-status-download' : 'text-muted-foreground'}">{t.down}</td>
              <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums {t.up !== '0 B/s' ? 'text-status-seed' : 'text-muted-foreground'}">{t.up}</td>
              <td class="px-3 py-2 text-right tabular-nums">{t.ratio}</td>
              <td class="whitespace-nowrap px-3 py-2 text-right tabular-nums text-muted-foreground">{t.peers}</td>
              <td class="whitespace-nowrap px-3 py-2">
                <span class="inline-flex items-center gap-1.5 font-medium {statusMeta[t.status].color}">
                  <Circle class="size-2 fill-current" />{statusMeta[t.status].label}
                </span>
              </td>
              <td class="px-3 py-2">
                {#if t.label}
                  <span class="rounded-full bg-secondary px-2 py-0.5 text-xs text-secondary-foreground">{t.label}</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </main>
  </div>

  <!-- ───────────── status bar ───────────── -->
  <footer class="flex h-7 shrink-0 items-center justify-between border-t bg-card px-4 text-xs text-muted-foreground">
    <span>1071 torrents · 1063 seeding · 2 downloading</span>
    <span class="tabular-nums">rtorrent 0.16.10 · API 20 · webui dev</span>
  </footer>
</div>
