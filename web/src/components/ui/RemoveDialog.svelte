<script lang="ts">
  import Modal from './Modal.svelte'
  import { removeDialog } from '$lib/stores/removeDialog.svelte'
  import { headerText, summaryText, primaryLabel, namesPreview, noCheckboxNote, requestDeletesData } from '$lib/removeDialog.logic'

  const n = $derived(removeDialog.targets.length)
  const names = $derived(namesPreview(removeDialog.targets))
  // The destructive intent is the AND of "server allows it" and "user checked the
  // box" — the same helper the confirm action uses, so label/colour/summary/request
  // can never diverge.
  const arming = $derived(requestDeletesData(removeDialog.capable, removeDialog.deleteData))

  // Close-transition teardown (focus restore + busy reset). This fires for every
  // close path, including Modal's backdrop/✕ which write open=false directly.
  let wasOpen = false
  $effect(() => {
    if (removeDialog.open) wasOpen = true
    else if (wasOpen) {
      wasOpen = false
      removeDialog.afterClose()
    }
  })
</script>

<Modal
  bind:open={removeDialog.open}
  title="rtorrent.remove"
  sd="✕"
  width={460}
  bdClass="rd-bd"
  trapFocus
  dismissible={!removeDialog.busy}
>
  <div class="rd-confirm">
    <div class="rd-q">{headerText(n)}</div>

    <div class="rd-names">
      <!-- key on the unique hash: torrent names are NOT unique (cross-seed /
           re-added releases) and a duplicate {#each} key throws at render -->
      {#each names.shown as t (t.hash)}
        <div class="rd-name" title={t.name}>{t.name}</div>
      {/each}
      {#if names.more > 0}<div class="rd-more">+{names.more} more</div>{/if}
    </div>

    {#if removeDialog.capable}
      <!-- locked mid-flight: confirm() already captured the delete flag, so the
           box (and the armed copy it drives) must not change after submit -->
      <label class="rd-check" class:disabled={removeDialog.busy}>
        <input type="checkbox" bind:checked={removeDialog.deleteData} disabled={removeDialog.busy} />
        <span>Also delete downloaded files from disk</span>
      </label>
      {#if removeDialog.deleteData}
        <div class="rd-warn">// permanent — files are erased from disk, not just removed from rtorrent</div>
      {/if}
    {:else}
      <div class="rd-note">{noCheckboxNote(removeDialog.configState)}</div>
    {/if}

    <!-- aria-live so a screen reader hears the consequence change as the box is
         toggled, at the action rather than only in passive copy -->
    <div class="rd-summary" class:armed={arming} aria-live="polite">{summaryText(arming)}</div>
  </div>

  {#snippet footer()}
    <!-- CANCEL is the default-focused, safe choice (data-autofocus); locked mid-flight -->
    <button class="rd-btn sp" data-autofocus disabled={removeDialog.busy} onclick={() => removeDialog.cancel()}>CANCEL</button>
    <button
      class="tbtn {arming ? 'danger' : 'acc'}"
      class:armed={arming}
      disabled={removeDialog.busy}
      onclick={() => removeDialog.confirm()}
    >{primaryLabel(arming, removeDialog.busy)}</button>
  {/snippet}
</Modal>
