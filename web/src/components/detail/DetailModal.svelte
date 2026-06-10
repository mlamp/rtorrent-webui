<script lang="ts">
  import type { TorrentRow } from '$lib/stores/torrents.svelte'
  import type { Status } from '$lib/types/torrent'
  import { detail } from '$lib/stores/detail.svelte'
  import RowDetail from './RowDetail.svelte'

  let { t }: { t: TorrentRow } = $props()

  const MARK: Record<Status, string> = {
    downloading: '▶',
    seeding: '↑',
    stopped: '■',
    paused: '⏸',
    hashing: '⟳',
    error: '!',
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="modal-bd" onclick={() => detail.close()} role="presentation">
  <div class="modal" style="width:880px" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
    <div class="modal-top">
      <div class="modal-title">
        <span class="sd">{MARK[t.status]}</span>
        <span class="truncate" style="color:var(--foreground)">{t.name}</span>
      </div>
      <button class="modal-x" onclick={() => detail.close()} aria-label="close">✕</button>
    </div>
    <!-- The body sizes to its content (capped by .modal's max-height); RowDetail's
         tab area carries its own max-height scroll, so short torrents render a
         compact popup instead of a tall, mostly-empty one. -->
    <div class="modal-body" style="padding:0">
      <RowDetail {t} />
    </div>
  </div>
</div>
