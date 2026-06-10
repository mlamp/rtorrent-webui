/**
 * Monotone cubic interpolation (Fritsch–Carlson, the algorithm behind d3's
 * curveMonotoneX): as smooth as Catmull-Rom, but the curve is guaranteed to
 * stay within the y-range of its neighbouring data points. A rate dropping
 * 20 MB/s → 0 flattens onto the axis instead of bouncing below zero the way
 * Catmull-Rom control points do.
 *
 * Points must be in ascending-x order (chart columns always are).
 */

export type Pt = [number, number]

const sgn = (x: number) => (x < 0 ? -1 : 1)

// Tangent at b limited so the b→a and b→c segments stay monotone (no overshoot).
// Opposite-sign secants (a local extremum) force a flat tangent.
function slope3(a: Pt, b: Pt, c: Pt): number {
  const h0 = b[0] - a[0]
  const h1 = c[0] - b[0]
  const s0 = h0 ? (b[1] - a[1]) / h0 : 0
  const s1 = h1 ? (c[1] - b[1]) / h1 : 0
  const p = (s0 * h1 + s1 * h0) / (h0 + h1)
  return (sgn(s0) + sgn(s1)) * Math.min(Math.abs(s0), Math.abs(s1), 0.5 * Math.abs(p)) || 0
}

// One-sided endpoint tangent; stays within the Fritsch–Carlson monotone region
// [0, 3·secant] given the interior tangent t, so end segments don't overshoot.
function slope2(a: Pt, b: Pt, t: number): number {
  const h = b[0] - a[0]
  return h ? (3 * (b[1] - a[1])) / h / 2 - t / 2 : t
}

/**
 * "Nice" round Y-axis maximum in binary (1024) units: the smallest of
 * {1,2,4,…,1024}×1024^k that covers `m`. Shared by every rate chart so the
 * vertical scale reads the same everywhere. 0/negative → 1 KiB floor.
 */
export function niceMax(m: number): number {
  if (m <= 0) return 1024
  const k = Math.floor(Math.log(m) / Math.log(1024))
  const unit = Math.pow(1024, k)
  const f = m / unit
  const steps = [1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024]
  return (steps.find((s) => f <= s) ?? 1024) * unit
}

/**
 * Smooth (monotone-cubic) path for one `{t, …}` series over a fixed time window
 * [start, end], scaled into the plot box at offset (x0, y0) sized plotW×plotH.
 * X is mapped by TIME (not array index), so an idle/sparse series sits at its true
 * position; Y is clamped to maxVal. Returns '' for < 2 points or a zero-width
 * window. The single implementation shared by the sidebar sparkline and the
 * full traffic chart so the two can't diverge.
 */
export function timeSeriesPath(
  points: { t: number; [k: string]: number }[],
  key: 'down' | 'up' | 'v',
  start: number,
  end: number,
  x0: number,
  plotW: number,
  y0: number,
  plotH: number,
  maxVal: number,
): string {
  const span = end - start
  if (points.length < 2 || span <= 0 || maxVal <= 0) return ''
  const pts: Pt[] = points.map((p) => [
    x0 + (plotW * (p.t - start)) / span,
    y0 + plotH - (Math.min(p[key], maxVal) / maxVal) * plotH,
  ])
  return monotonePath(pts)
}

/** SVG path ("M … C …") through the points; '' for fewer than 2 points. */
export function monotonePath(p: Pt[]): string {
  const n = p.length
  if (n < 2) return ''
  if (n === 2) return `M ${p[0][0]},${p[0][1]} L ${p[1][0]},${p[1][1]}`
  const m = new Array<number>(n)
  for (let i = 1; i < n - 1; i++) m[i] = slope3(p[i - 1], p[i], p[i + 1])
  m[0] = slope2(p[0], p[1], m[1])
  m[n - 1] = slope2(p[n - 2], p[n - 1], m[n - 2])
  let d = `M ${p[0][0]},${p[0][1]}`
  for (let i = 0; i < n - 1; i++) {
    const dx = (p[i + 1][0] - p[i][0]) / 3
    d += ` C ${p[i][0] + dx},${p[i][1] + dx * m[i]} ${p[i + 1][0] - dx},${p[i + 1][1] - dx * m[i + 1]} ${p[i + 1][0]},${p[i + 1][1]}`
  }
  return d
}
