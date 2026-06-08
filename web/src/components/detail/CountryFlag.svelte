<script lang="ts">
  import { flagEmojiSupported, codeToFlag } from '$lib/flags'

  let { code = '' }: { code?: string } = $props()

  const cc = $derived(code && code.length === 2 ? code.toUpperCase() : '')
  const flag = $derived(codeToFlag(cc))
</script>

{#if cc && flagEmojiSupported}
  <span class="cc-flag" title={cc}>{flag}</span>
{:else}
  <span class="cc-code" title={cc || 'unknown'}>{cc || '··'}</span>
{/if}
