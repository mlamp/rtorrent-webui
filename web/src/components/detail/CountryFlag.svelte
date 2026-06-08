<script lang="ts">
  let { code = '' }: { code?: string } = $props()

  const flag = $derived(
    code && code.length === 2
      ? String.fromCodePoint(
          ...[...code.toUpperCase()].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65),
        )
      : '🏳️',
  )
</script>

<span title={code || 'unknown'}>{flag}</span>
