<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    open = $bindable(false),
    title = '',
    sd = '',
    width = 580,
    // bdClass adds a class to the backdrop so a stacked dialog (e.g. the remove
    // confirm over the detail modal) can raise its own z-index.
    bdClass = '',
    // dismissible gates backdrop-click and ✕ close; set false to block dismissal
    // mid-operation (e.g. while a destructive action is in flight).
    dismissible = true,
    // trapFocus, when set, makes this modal own initial focus + a Tab focus-trap.
    // Off by default so existing dialogs are unchanged; the destructive remove
    // dialog opts in. The element with [data-autofocus] gets initial focus
    // (falling back to the first focusable, then the dialog box itself).
    trapFocus = false,
    children,
    footer,
  }: {
    open?: boolean
    title?: string
    sd?: string
    width?: number
    bdClass?: string
    dismissible?: boolean
    trapFocus?: boolean
    children?: Snippet
    footer?: Snippet
  } = $props()

  let modalEl = $state<HTMLElement>()

  function focusables(): HTMLElement[] {
    if (!modalEl) return []
    const sel = 'a[href],button:not([disabled]),input:not([disabled]),textarea:not([disabled]),select:not([disabled]),[tabindex]:not([tabindex="-1"])'
    return [...modalEl.querySelectorAll<HTMLElement>(sel)].filter((el) => el.offsetParent !== null || el === document.activeElement)
  }

  // Initial focus when the trap-enabled modal opens. Depends only on open/modalEl
  // so toggling inner controls never steals focus back to the default.
  $effect(() => {
    if (open && trapFocus && modalEl) {
      const target = modalEl.querySelector<HTMLElement>('[data-autofocus]') ?? focusables()[0] ?? modalEl
      target.focus()
    }
  })

  // Tab focus-trap: recomputes the focusable set live (so the primary button
  // going disabled mid-flight is handled) and wraps at both ends.
  function onKeydown(e: KeyboardEvent) {
    if (!trapFocus || e.key !== 'Tab') return
    const f = focusables()
    if (f.length === 0) {
      e.preventDefault()
      modalEl?.focus()
      return
    }
    const first = f[0]
    const last = f[f.length - 1]
    const active = document.activeElement as HTMLElement | null
    const inside = active && modalEl?.contains(active)
    if (e.shiftKey) {
      if (active === first || !inside) {
        e.preventDefault()
        last.focus()
      }
    } else if (active === last || !inside) {
      e.preventDefault()
      first.focus()
    }
  }
</script>

{#if open}
  <div
    class="modal-bd {bdClass}"
    onclick={(e) => {
      if (dismissible && e.target === e.currentTarget) open = false
    }}
    role="presentation"
  >
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <!-- title is the accessible name (no aria-labelledby — the visible .modal-title is decorative text) -->
    <div bind:this={modalEl} class="modal" style="width:{width}px" role="dialog" aria-modal="true" aria-label={title || undefined} tabindex="-1" onkeydown={onKeydown}>
      <div class="modal-top">
        <div class="modal-title">{#if sd}<span class="sd">{sd}</span>{/if}{title}</div>
        <button class="modal-x" onclick={() => dismissible && (open = false)} aria-label="Close">✕</button>
      </div>
      <div class="modal-body">{@render children?.()}</div>
      {#if footer}<div class="modal-foot">{@render footer()}</div>{/if}
    </div>
  </div>
{/if}
