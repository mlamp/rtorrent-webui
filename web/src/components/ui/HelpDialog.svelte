<script lang="ts">
  let { open = $bindable(false) }: { open?: boolean } = $props()

  const sections: { title: string; rows: { keys: string[]; label: string }[] }[] = [
    {
      title: 'navigate',
      rows: [
        { keys: ['j', '↓'], label: 'move cursor down' },
        { keys: ['k', '↑'], label: 'move cursor up' },
        { keys: ['/'], label: 'focus search' },
        { keys: ['o', '↵'], label: 'open / close detail' },
      ],
    },
    {
      title: 'select',
      rows: [
        { keys: ['x', '␣'], label: 'toggle row select' },
        { keys: ['*'], label: 'select all visible' },
        { keys: ['esc'], label: 'clear / close' },
      ],
    },
    {
      title: 'act',
      rows: [
        { keys: ['p'], label: 'pause selection' },
        { keys: ['r'], label: 'resume selection' },
        { keys: ['del'], label: 'remove selection' },
      ],
    },
    {
      title: 'view',
      rows: [
        { keys: ['v'], label: 'cycle list / grid / insight' },
        { keys: ['a'], label: 'add torrent' },
        { keys: ['?'], label: 'toggle this help' },
      ],
    },
  ]
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="modal-bd" onclick={() => (open = false)} role="presentation">
    <div class="modal" style="width:540px" onclick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" tabindex="-1">
      <div class="modal-top">
        <div class="modal-title"><span class="sd">?</span> keyboard</div>
        <button class="modal-x" onclick={() => (open = false)} aria-label="close">✕</button>
      </div>
      <div class="modal-body">
        <div class="kbd-grid">
          {#each sections as sec (sec.title)}
            <div class="kbd-sec">{sec.title}</div>
            {#each sec.rows as r (r.label)}
              <div class="kbd-row">
                <span class="kbd-keys">
                  {#each r.keys as k (k)}<span class="kbd">{k}</span>{/each}
                </span>
                {r.label}
              </div>
            {/each}
          {/each}
        </div>
      </div>
    </div>
  </div>
{/if}
