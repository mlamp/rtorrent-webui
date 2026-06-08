<script lang="ts">
  import { bytes } from '$lib/format'

  type Point = { t: number; down: number; up: number }
  let { points = [], height = 240 }: { points?: Point[]; height?: number } = $props()

  let canvas = $state<HTMLCanvasElement>()
  let width = $state(800)

  $effect(() => {
    const c = canvas
    if (!c) return
    const dpr = window.devicePixelRatio || 1
    c.width = width * dpr
    c.height = height * dpr
    const ctx = c.getContext('2d')!
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.clearRect(0, 0, width, height)

    const css = getComputedStyle(document.documentElement)
    const cssv = (n: string) => css.getPropertyValue(n).trim() || '#888'
    const pad = { l: 8, r: 8, t: 8, b: 8 }
    const w = width - pad.l - pad.r
    const h = height - pad.t - pad.b
    const max = Math.max(1, ...points.map((p) => Math.max(p.down, p.up)))

    ctx.strokeStyle = cssv('--border')
    ctx.lineWidth = 1
    for (let i = 0; i <= 4; i++) {
      const y = pad.t + (h * i) / 4
      ctx.beginPath()
      ctx.moveTo(pad.l, y)
      ctx.lineTo(pad.l + w, y)
      ctx.stroke()
    }

    if (points.length >= 2) {
      const x = (i: number) => pad.l + (w * i) / (points.length - 1)
      const y = (v: number) => pad.t + h - (v / max) * h
      const area = (key: 'down' | 'up', colorVar: string) => {
        const col = cssv(colorVar)
        ctx.beginPath()
        points.forEach((p, i) => (i ? ctx.lineTo(x(i), y(p[key])) : ctx.moveTo(x(i), y(p[key]))))
        ctx.strokeStyle = col
        ctx.lineWidth = 1.5
        ctx.stroke()
        ctx.lineTo(x(points.length - 1), pad.t + h)
        ctx.lineTo(x(0), pad.t + h)
        ctx.closePath()
        ctx.globalAlpha = 0.12
        ctx.fillStyle = col
        ctx.fill()
        ctx.globalAlpha = 1
      }
      area('up', '--status-seed')
      area('down', '--status-download')
    }

    ctx.fillStyle = cssv('--muted-foreground')
    ctx.font = '11px ui-sans-serif, system-ui'
    ctx.fillText(`${bytes(max)}/s`, pad.l + 4, pad.t + 12)
  })
</script>

<div bind:clientWidth={width} class="w-full">
  {#if points.length < 2}
    <div class="grid text-sm text-muted-foreground" style="height:{height}px; place-items:center">
      Collecting data…
    </div>
  {:else}
    <canvas bind:this={canvas} style="width:{width}px;height:{height}px"></canvas>
  {/if}
</div>
