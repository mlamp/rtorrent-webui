import { latestOnly } from '../src/lib/latest.ts'

let pass = 0,
  fail = 0
function ok(name: string, cond: boolean, detail = '') {
  if (cond) {
    pass++
    return
  }
  fail++
  console.error(`FAIL ${name}${detail ? `\n  ${detail}` : ''}`)
}

// Manually-resolved promise so the test controls response arrival order.
function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((res) => (resolve = res))
  return { promise, resolve }
}

type Win = { start: number; end: number }

// Loader whose fetches resolve only when the test says so; apply writes the
// window into `win` exactly like InsightView's loadHistory does.
function makeLoader() {
  const pending: { promise: Promise<Win | null>; resolve: (v: Win | null) => void }[] = []
  let win: Win | null = null
  const load = latestOnly<Win>(
    () => {
      const d = deferred<Win | null>()
      pending.push(d)
      return d.promise
    },
    (d) => (win = d),
  )
  return { load, pending, win: () => win }
}

// Out-of-order responses: user clicks 7d, then 15m; the 15m response arrives
// first and the slow 7d response last. The stale 7d data must be dropped —
// the chart window stays at the currently-selected 15m (900s) range.
{
  const l = makeLoader()
  const p7d = l.load()
  const p15m = l.load()
  l.pending[1].resolve({ start: 1000, end: 1900 }) // 15m answers immediately
  l.pending[0].resolve({ start: 0, end: 604800 }) // slow 7d answers last
  await Promise.all([p7d, p15m])
  const w = l.win()
  ok('stale response is dropped, latest range wins', w !== null && w.end - w.start === 900, `window ${w ? w.end - w.start : 'null'}s, want 900s`)
}

// In-order responses still apply normally.
{
  const l = makeLoader()
  const p1 = l.load()
  l.pending[0].resolve({ start: 0, end: 3600 })
  await p1
  const p2 = l.load()
  l.pending[1].resolve({ start: 0, end: 900 })
  await p2
  const w = l.win()
  ok('sequential responses apply in order', w !== null && w.end - w.start === 900, `window ${w ? w.end - w.start : 'null'}s, want 900s`)
}

// A failed fetch (silentGet returns null) never clobbers existing state.
{
  const l = makeLoader()
  const p1 = l.load()
  l.pending[0].resolve({ start: 0, end: 900 })
  await p1
  const p2 = l.load()
  l.pending[1].resolve(null)
  await p2
  const w = l.win()
  ok('null response leaves state untouched', w !== null && w.end - w.start === 900, `window ${w ? w.end - w.start : 'null'}s, want 900s`)
}

// Interval-poll race: a poll for the old range is in flight when the user
// clicks a new range; the click's response lands, then the old poll's. The
// late poll result must not roll the window back.
{
  const l = makeLoader()
  const pPoll = l.load() // 3s interval fires for old range (6h)
  const pClick = l.load() // user clicks 15m
  l.pending[1].resolve({ start: 100, end: 1000 }) // click answers
  await pClick
  l.pending[0].resolve({ start: 0, end: 21600 }) // stale poll answers after
  await pPoll
  const w = l.win()
  ok('late interval poll cannot roll back a newer click', w !== null && w.end - w.start === 900, `window ${w ? w.end - w.start : 'null'}s, want 900s`)
}

// The newest request applies even when an older one never resolves at all.
{
  const l = makeLoader()
  l.load() // hangs forever
  const p2 = l.load()
  l.pending[1].resolve({ start: 0, end: 900 })
  await p2
  const w = l.win()
  ok('latest applies while an older request hangs', w !== null && w.end - w.start === 900, `window ${w ? w.end - w.start : 'null'}s, want 900s`)
}

console.log(`latest: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
