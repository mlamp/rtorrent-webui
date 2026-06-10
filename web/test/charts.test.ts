import { monotonePath, type Pt } from '../src/lib/charts.ts'

let pass = 0, fail = 0
function ok(name: string, cond: boolean, detail = '') {
  if (cond) { pass++; return }
  fail++
  console.error(`FAIL ${name}${detail ? `\n  ${detail}` : ''}`)
}
function eq(name: string, got: unknown, want: unknown) {
  ok(name, JSON.stringify(got) === JSON.stringify(want), `got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}

// Parse "M x,y C c1x,c1y c2x,c2y x,y …" and densely sample every cubic segment.
function samplePath(d: string): Pt[] {
  const nums = d.match(/-?[\d.]+(?:e-?\d+)?/g)!.map(Number)
  const out: Pt[] = [[nums[0], nums[1]]]
  let [x0, y0] = [nums[0], nums[1]]
  for (let i = 2; i + 5 < nums.length; i += 6) {
    const [c1x, c1y, c2x, c2y, x1, y1] = nums.slice(i, i + 6)
    for (let s = 1; s <= 64; s++) {
      const t = s / 64, u = 1 - t
      out.push([
        u * u * u * x0 + 3 * u * u * t * c1x + 3 * u * t * t * c2x + t * t * t * x1,
        u * u * u * y0 + 3 * u * u * t * c1y + 3 * u * t * t * c2y + t * t * t * y1,
      ])
    }
    ;[x0, y0] = [x1, y1]
  }
  return out
}

// Chart mapping used by SpeedGraph/TrafficChart: y = h - (v/max)*h, so v=0 is
// y=h (the baseline) and "below zero" means a sampled y > h.
function chartPts(values: number[], h: number): Pt[] {
  const max = Math.max(1, ...values) * 1.15
  return values.map((v, i) => [i * 10, h - (v / max) * h])
}

const H = 58
const EPS = 1e-6

// The reported bug: 20 MB/s collapsing to a flat zero tail. Catmull-Rom bounces
// below the baseline here; monotone interpolation must not.
const drop = chartPts([20e6, 20e6, 20e6, 0, 0, 0, 0], H)
for (const [x, y] of samplePath(monotonePath(drop))) {
  if (y > H + EPS) { ok('drop-to-zero stays above baseline', false, `y=${y} at x=${x} exceeds h=${H}`); break }
}
ok('drop-to-zero sampled', true)

// A lone spike between zeros: no undershoot before/after, no overshoot above the peak.
const spike = chartPts([0, 0, 20e6, 0, 0], H)
{
  const ys = samplePath(monotonePath(spike)).map(([, y]) => y)
  ok('spike never below baseline', ys.every((y) => y <= H + EPS), `maxY=${Math.max(...ys)}`)
  const top = Math.min(...spike.map(([, y]) => y)) // peak data point (smallest y)
  ok('spike never above its peak', ys.every((y) => y >= top - EPS), `minY=${Math.min(...ys)} top=${top}`)
}

// Random walks: the curve must stay inside the data's overall y-range.
{
  let seed = 42
  const rnd = () => (seed = (seed * 1103515245 + 12345) % 2 ** 31) / 2 ** 31
  for (let run = 0; run < 50; run++) {
    const vals = Array.from({ length: 30 }, () => (rnd() < 0.3 ? 0 : rnd() * 30e6))
    const p = chartPts(vals, H)
    const lo = Math.min(...p.map(([, y]) => y))
    const ys = samplePath(monotonePath(p)).map(([, y]) => y)
    if (!ys.every((y) => y <= H + EPS && y >= lo - EPS)) {
      ok(`random walk ${run} in range`, false, `[${Math.min(...ys)}, ${Math.max(...ys)}] vs [${lo}, ${H}]`)
      break
    }
  }
  ok('random walks sampled', true)
}

// Degenerate inputs.
eq('empty', monotonePath([]), '')
eq('single point', monotonePath([[5, 5]]), '')
eq('two points is a line', monotonePath([[0, 10], [10, 0]]), 'M 0,10 L 10,0')

// The curve interpolates (passes through) every data point. Sample the emitted
// cubics and assert a sampled point coincides with each data point within EPS —
// numeric, not the brittle "does the coordinate substring appear in the path".
{
  const p = chartPts([3e6, 9e6, 1e6, 0, 14e6], H)
  const sampled = samplePath(monotonePath(p))
  const through = p.every(([x, y]) =>
    sampled.some(([sx, sy]) => Math.abs(sx - x) < EPS && Math.abs(sy - y) < EPS),
  )
  ok('passes through all points (numeric)', through)
}

// The emitted path must never carry NaN/Infinity coordinates — SVG rejects them,
// and an unguarded division (e.g. a zero-width x segment) is how they'd appear.
function finitePath(name: string, d: string) {
  ok(name, d !== '' && !/NaN|Infinity/.test(d), d)
}
finitePath('drop path is finite', monotonePath(drop))
finitePath('spike path is finite', monotonePath(spike))
finitePath('interp path is finite', monotonePath(chartPts([3e6, 9e6, 1e6, 0, 14e6], H)))

// Duplicate consecutive x (h == 0 in slope3/slope2) must not divide-by-zero into
// NaN coordinates — exercises the ternary guards on a vertical segment.
finitePath('duplicate-x path is finite', monotonePath([[0, 0], [0, 10], [1, 15]]))

console.log(`charts: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
