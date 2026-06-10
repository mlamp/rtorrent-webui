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
