<script lang="ts">
  import { X } from '@lucide/svelte'
  import { detail, type DetailTab } from '$lib/stores/detail.svelte'
  import { torrents } from '$lib/stores/torrents.svelte'
  import { bytes, rate, ratio, percent } from '$lib/format'
  import Sparkline from './Sparkline.svelte'
  import CountryFlag from './CountryFlag.svelte'

  const t = $derived(detail.activeHash ? torrents.map.get(detail.activeHash) : undefined)

  // Sample the active torrent's rates into the Speed ring buffer on an interval.
  // (Doing this in a reactive $effect would self-invalidate: pushSpeed both reads
  // and writes speedDown.)
  $effect(() => {
    const h = detail.activeHash
    if (!h) return
    const id = setInterval(() => {
      const tt = torrents.map.get(h)
      if (tt) detail.pushSpeed(tt.downRate, tt.upRate)
    }, 1000)
    return () => clearInterval(id)
  })

  const tabs: { key: DetailTab; label: string }[] = [
    { key: 'general', label: 'General' },
    { key: 'files', label: 'Files' },
    { key: 'peers', label: 'Peers' },
    { key: 'trackers', label: 'Trackers' },
    { key: 'speed', label: 'Speed' },
  ]
  const PRIOS = [
    { v: 0, label: 'Off' },
    { v: 1, label: 'Normal' },
    { v: 2, label: 'High' },
  ]
</script>

{#if t}
  <section class="flex h-72 shrink-0 flex-col border-t bg-card">
    <!-- tab bar -->
    <div class="flex items-center gap-1 border-b px-2">
      {#each tabs as tab (tab.key)}
        <button
          onclick={() => detail.setTab(tab.key)}
          class="border-b-2 px-3 py-2 text-sm transition
            {detail.tab === tab.key ? 'border-primary font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'}"
        >
          {tab.label}
        </button>
      {/each}
      <div class="ml-auto flex items-center gap-2 pr-1">
        <span class="max-w-md truncate text-sm font-medium" title={t.name}>{t.name}</span>
        <button onclick={() => detail.close()} class="grid size-7 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Close">
          <X class="size-4" />
        </button>
      </div>
    </div>

    <!-- content -->
    <div class="min-h-0 flex-1 overflow-auto p-3 text-sm">
      {#if detail.tab === 'general'}
        <dl class="grid grid-cols-2 gap-x-8 gap-y-2 md:grid-cols-3">
          {#snippet row(k: string, v: string)}
            <div class="flex justify-between gap-2 border-b border-border/40 py-1">
              <dt class="text-muted-foreground">{k}</dt>
              <dd class="truncate text-right font-medium" title={v}>{v}</dd>
            </div>
          {/snippet}
          {@render row('Status', t.status)}
          {@render row('Size', bytes(t.size))}
          {@render row('Done', percent(t.done))}
          {@render row('Ratio', ratio(t.ratio))}
          {@render row('Down', rate(t.downRate))}
          {@render row('Up', rate(t.upRate))}
          {@render row('Uploaded', bytes(t.upTotal))}
          {@render row('Peers', `${t.peersConnected} / ${t.peersTotal}`)}
          {@render row('Seeds', `${t.seedsConnected}`)}
          {@render row('Label', t.label || '—')}
          {@render row('Directory', t.directory || '—')}
          {@render row('Hash', t.hash)}
          {#if t.message}{@render row('Message', t.message)}{/if}
        </dl>
      {:else if detail.tab === 'files'}
        {#if detail.loading && detail.files.length === 0}
          <p class="text-muted-foreground">Loading…</p>
        {:else}
          <div class="flex flex-col gap-1">
            {#each detail.files as f (f.index)}
              <div class="flex items-center gap-3 rounded px-2 py-1 hover:bg-accent/40">
                <span class="min-w-0 flex-1 truncate" title={f.path}>{f.path}</span>
                <span class="w-20 text-right tabular-nums text-muted-foreground">{bytes(f.size)}</span>
                <div class="h-1.5 w-24 overflow-hidden rounded-full bg-secondary">
                  <div class="h-full rounded-full bg-status-seed" style="width:{f.done * 100}%"></div>
                </div>
                <div class="flex gap-0.5">
                  {#each PRIOS as p (p.v)}
                    <button
                      onclick={() => detail.setFilePriority(f.index, p.v)}
                      class="rounded px-1.5 py-0.5 text-xs transition
                        {f.priority === p.v ? 'bg-primary text-primary-foreground' : 'bg-secondary text-muted-foreground hover:text-foreground'}"
                    >{p.label}</button>
                  {/each}
                </div>
              </div>
            {/each}
            {#if detail.files.length === 0}<p class="text-muted-foreground">No files.</p>{/if}
          </div>
        {/if}
      {:else if detail.tab === 'peers'}
        {#if detail.peers.length === 0}
          <p class="text-muted-foreground">{detail.loading ? 'Loading…' : 'No connected peers.'}</p>
        {:else}
          <table class="w-full text-sm">
            <thead class="text-xs uppercase text-muted-foreground">
              <tr class="border-b text-left">
                <th class="py-1 font-medium">Peer</th><th class="font-medium">Client</th>
                <th class="text-right font-medium">Down</th><th class="text-right font-medium">Up</th>
                <th class="text-right font-medium">Progress</th><th class="text-center font-medium">Enc</th>
              </tr>
            </thead>
            <tbody>
              {#each detail.peers as p (p.address + ':' + p.port)}
                <tr class="border-b border-border/40">
                  <td class="py-1"><CountryFlag code={p.country} /> {p.address}</td>
                  <td class="truncate text-muted-foreground">{p.client}</td>
                  <td class="text-right tabular-nums {p.downRate > 0 ? 'text-status-download' : ''}">{rate(p.downRate)}</td>
                  <td class="text-right tabular-nums {p.upRate > 0 ? 'text-status-seed' : ''}">{rate(p.upRate)}</td>
                  <td class="text-right tabular-nums">{p.progress}%</td>
                  <td class="text-center">{p.encrypted ? '🔒' : ''}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      {:else if detail.tab === 'trackers'}
        {#if detail.trackers.length === 0}
          <p class="text-muted-foreground">{detail.loading ? 'Loading…' : 'No trackers.'}</p>
        {:else}
          <div class="flex flex-col gap-1">
            {#each detail.trackers as tr (tr.index)}
              <div class="flex items-center gap-3 rounded px-2 py-1 hover:bg-accent/40">
                <label class="flex items-center gap-2">
                  <input type="checkbox" class="accent-primary" checked={tr.enabled} onchange={(e) => detail.toggleTracker(tr.index, e.currentTarget.checked)} />
                </label>
                <span class="min-w-0 flex-1 truncate" title={tr.url}>{tr.url}</span>
                <span class="text-xs text-muted-foreground">success: {tr.success}</span>
              </div>
            {/each}
          </div>
        {/if}
      {:else if detail.tab === 'speed'}
        <div class="flex items-center gap-6 pb-2">
          <span class="text-status-download">▼ {rate(t.downRate)}</span>
          <span class="text-status-seed">▲ {rate(t.upRate)}</span>
        </div>
        <Sparkline down={detail.speedDown} up={detail.speedUp} />
      {/if}
    </div>
  </section>
{/if}
