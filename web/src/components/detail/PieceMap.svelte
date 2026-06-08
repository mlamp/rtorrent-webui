<script lang="ts">
  // Completion map — a grid of cells, `done` fraction filled. rtorrent doesn't
  // expose a real piece bitfield over RPC, so this is a sequential approximation
  // (have / in-flight near the edge while downloading / missing).
  let {
    done = 0,
    count = 280,
    downloading = false,
  }: { done?: number; count?: number; downloading?: boolean } = $props()

  const cells = $derived.by(() => {
    const filled = Math.floor(done * count)
    const out: number[] = []
    for (let i = 0; i < count; i++) {
      if (i < filled) out.push(1)
      else if (downloading && i < filled + 8 && i % 2 === 0) out.push(2)
      else out.push(0)
    }
    return out
  })
</script>

<div class="flex flex-wrap gap-[2px]">
  {#each cells as c, i (i)}
    <i
      class="size-[10px] rounded-[1px]"
      style="background:{c === 1 ? 'var(--primary)' : c === 2 ? 'var(--warn)' : 'color-mix(in srgb, var(--primary) 15%, transparent)'}; {c === 1 ? 'box-shadow:0 0 3px color-mix(in srgb,var(--primary) 50%,transparent)' : ''}"
    ></i>
  {/each}
</div>
