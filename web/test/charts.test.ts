import { monotonePath, timeSeriesPath, niceMax, type Pt } from '../src/lib/charts.ts'

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

// ── timeSeriesPath (time-based mapping shared by the charts) ──────────────────
// Degenerate windows draw nothing (caller falls back to a baseline).
eq('tsp empty', timeSeriesPath([], 'down', 0, 10, 0, 100, 0, 50, 1), '')
eq('tsp single point', timeSeriesPath([{ t: 5, down: 1 }], 'down', 0, 10, 0, 100, 0, 50, 1), '')
eq('tsp zero-span window', timeSeriesPath([{ t: 5, down: 1 }, { t: 5, down: 2 }], 'down', 5, 5, 0, 100, 0, 50, 1), '')

// An all-zero series over a valid window must draw a NON-empty flat line pinned to
// the baseline (y = y0 + plotH) — the "0 line" the chart must always show when idle,
// never a blank/absent path.
{
  const d = timeSeriesPath([{ t: 0, down: 0 }, { t: 5, down: 0 }, { t: 10, down: 0 }], 'down', 0, 10, 0, 100, 0, 50, 1)
  ok('tsp all-zero is drawn (non-empty)', d !== '', `got ${JSON.stringify(d)}`)
  const ys = samplePath(d).map(([, y]) => y)
  ok('tsp all-zero sits on the baseline', ys.every((y) => Math.abs(y - 50) < EPS), `ys [${Math.min(...ys)}, ${Math.max(...ys)}] want 50`)
}

// X is mapped by TIME, not index: an early then a late sample over [0,10] land at the
// left and right edges regardless of count.
{
  const d = timeSeriesPath([{ t: 1, down: 0 }, { t: 9, down: 0 }], 'down', 0, 10, 0, 100, 0, 50, 1)
  ok('tsp maps x by time', d.startsWith('M 10,') , `got ${d.slice(0, 24)}`) // t=1 → x = 100*(1-0)/10 = 10
}

// niceMax: round binary ceilings; 0 floors to 1 KiB; mid values round up to a step.
eq('niceMax 0 floors to 1KiB', niceMax(0), 1024)
eq('niceMax exact 1MiB', niceMax(1 << 20), 1 << 20)
ok('niceMax rounds 70MiB up to 128MiB', niceMax(70 * (1 << 20)) === 128 * (1 << 20), `got ${niceMax(70 * (1 << 20))}`)

console.log(`charts: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
