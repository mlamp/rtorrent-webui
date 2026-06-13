import { makeStalenessGauge, STALE_LAG_S } from '../src/lib/stale.ts'

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

// S1: healthy stream — server ts and client rx tick together, tiny delivery lag.
{
  const g = makeStalenessGauge()
  let allFresh = true
  for (let t = 1000; t < 1060; t++) allFresh = allFresh && g(t, t + 0.1) === 'fresh'
  ok('S1 healthy stream stays fresh', allFresh)
}

// S2: the hub's cached snapshot can be up to ~30s old when a subscriber joins.
// The first sample only sets the baseline; live deltas then correct the skew
// downward without ever producing a false positive.
{
  const g = makeStalenessGauge()
  const first = g(1000, 1030) // 30s-old cached snapshot
  let allFresh = first === 'fresh'
  for (let t = 1031; t < 1091; t++) allFresh = allFresh && g(t, t + 0.1) === 'fresh'
  ok('S2 stale cached-snapshot baseline never false-positives', allFresh)
}

// S3: resume flood — a calibrated connection drains hours-old queued messages.
{
  const g = makeStalenessGauge()
  g(1000, 1000.1)
  g(1001, 1001.1)
  ok('S3 resume flood reads stale', g(1002, 1002 + 3600) === 'stale')
}

// S4: client clock steps backward — negative lag, never a false positive.
{
  const g = makeStalenessGauge()
  g(1000, 1000.1)
  ok('S4 backward clock jump stays fresh', g(1001, 1001.1 - 60) === 'fresh')
}

// S5: client clock jumps forward — stale, and STAYS stale on this instance
// (the min baseline never corrects upward; the "one resync" property comes
// from the driver creating a fresh gauge per connection — see driver D10).
{
  const g = makeStalenessGauge()
  g(1000, 1000.1)
  const jumped = g(1001, 1001.1 + 60)
  const next = g(1002, 1002.1 + 60)
  ok('S5 forward jump is stale and stays stale on the same instance', jumped === 'stale' && next === 'stale')
}

// S6: boundary pair pins the strict `>` — excess lag exactly STALE_LAG_S is
// fresh; a hair past it is stale.
{
  const g1 = makeStalenessGauge()
  g1(1000, 1000) // skew baseline 0
  ok('S6a excess lag == STALE_LAG_S is fresh', g1(1001, 1001 + STALE_LAG_S) === 'fresh')
  const g2 = makeStalenessGauge()
  g2(1000, 1000)
  ok('S6b excess lag just past STALE_LAG_S is stale', g2(1001, 1001 + STALE_LAG_S + 0.001) === 'stale')
}

// S7: server clock back-step > STALE_LAG_S — ts regresses while rx advances,
// inflating apparent lag. One spurious resync is the accepted behavior
// (characterization: the new connection's gauge re-baselines after it).
{
  const g = makeStalenessGauge()
  g(1000, 1000.1)
  ok('S7 server-clock back-step reads stale (one accepted spurious resync)', g(980, 1001.1) === 'stale')
}

// S8: custom threshold argument is respected.
{
  const g = makeStalenessGauge(5)
  g(1000, 1000)
  ok('S8 custom staleLagS respected', g(1001, 1001 + 6) === 'stale')
}

console.log(`stale: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
