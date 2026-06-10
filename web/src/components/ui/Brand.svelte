<script lang="ts">
  // The TorUI wordmark. A configured instance name takes the prominent slot and
  // demotes the product name to the dim suffix ("▚ TV ::TorUI"); with no name it's
  // the default "▚ TORUI ::rtorrent".
  import { config } from '$lib/stores/config.svelte'

  let { class: cls = '' }: { class?: string } = $props()
</script>

<div class="brand {cls}">
  ▚ {config.name || 'TORUI'}<span class="ml-0.5 text-[13px] font-normal tracking-[0.04em] text-dim">::{config.name ? 'TorUI' : 'rtorrent'}</span>
</div>
