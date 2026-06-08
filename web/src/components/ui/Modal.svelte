<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    open = $bindable(false),
    title = '',
    sd = '',
    width = 580,
    children,
    footer,
  }: {
    open?: boolean
    title?: string
    sd?: string
    width?: number
    children?: Snippet
    footer?: Snippet
  } = $props()
</script>

{#if open}
  <div
    class="modal-bd"
    onclick={(e) => {
      if (e.target === e.currentTarget) open = false
    }}
    role="presentation"
  >
    <div class="modal" style="width:{width}px" role="dialog" aria-modal="true" tabindex="-1">
      <div class="modal-top">
        <div class="modal-title">{#if sd}<span class="sd">{sd}</span>{/if}{title}</div>
        <button class="modal-x" onclick={() => (open = false)} aria-label="Close">✕</button>
      </div>
      <div class="modal-body">{@render children?.()}</div>
      {#if footer}<div class="modal-foot">{@render footer()}</div>{/if}
    </div>
  </div>
{/if}
