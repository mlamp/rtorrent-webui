<script lang="ts">
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import { view } from '$lib/stores/view.svelte'
  import GridCard from './GridCard.svelte'

  let { rows }: { rows: TorrentRow[] } = $props()
</script>

<div class="cardgrid" data-list>
  {#each rows as t (t.hash)}
    <GridCard {t} />
  {/each}
  {#if rows.length === 0}
    <div class="empty" style="grid-column:1/-1">
      // no torrents match <b>grep {view.search}</b>
    </div>
  {/if}
</div>
