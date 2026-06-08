<script lang="ts">
  import type { Snippet } from 'svelte'
  import { X } from '@lucide/svelte'

  let {
    open = $bindable(false),
    title = '',
    children,
  }: { open?: boolean; title?: string; children?: Snippet } = $props()

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') open = false
  }
</script>

<svelte:window {onkeydown} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-center bg-black/50 p-4 backdrop-blur-sm"
    onclick={(e) => {
      if (e.target === e.currentTarget) open = false
    }}
    role="presentation"
  >
    <div class="w-full max-w-md rounded-sm border border-line bg-card text-card-foreground shadow-2xl">
      <div class="flex items-center justify-between border-b border-line px-4 py-3">
        <h2 class="text-[13px] font-semibold uppercase tracking-[0.08em] text-primary">{title}</h2>
        <button
          onclick={() => (open = false)}
          class="grid size-7 place-items-center rounded-md text-muted-foreground transition hover:bg-accent hover:text-foreground"
          aria-label="Close"
        >
          <X class="size-4" />
        </button>
      </div>
      <div class="p-4">{@render children?.()}</div>
    </div>
  </div>
{/if}
