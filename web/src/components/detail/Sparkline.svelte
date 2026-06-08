<script lang="ts">
  let {
    down = [],
    up = [],
    height = 96,
  }: { down?: number[]; up?: number[]; height?: number } = $props()

  let canvas = $state<HTMLCanvasElement>()
  let width = $state(600)

  $effect(() => {
    const c = canvas
    if (!c) return
    const dpr = window.devicePixelRatio || 1
    c.width = width * dpr
    c.height = height * dpr
    const ctx = c.getContext('2d')!
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, width, height)

    const max = Math.max(1, ...down, ...up)
    const css = getComputedStyle(document.documentElement)

    const line = (data: number[], colorVar: string) => {
      if (data.length < 2) return
      const step = width / Math.max(1, data.length - 1)
      ctx.beginPath()
      data.forEach((v, i) => {
        const x = i * step
        const y = height - (v / max) * (height - 4) - 2
        if (i) ctx.lineTo(x, y)
        else ctx.moveTo(x, y)
      })
      ctx.strokeStyle = css.getPropertyValue(colorVar).trim() || '#888'
      ctx.lineWidth = 1.5
      ctx.stroke()
    }
    line(up, '--status-seed')
    line(down, '--status-download')
  })
</script>

<div bind:clientWidth={width} class="w-full">
  <canvas bind:this={canvas} style="width:{width}px;height:{height}px"></canvas>
</div>
