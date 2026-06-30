import { createSseDriver, HIDDEN_GRACE_MS, HEALTH_STALE_MS, type ESLike, type ConnState, type RtHealth } from '../src/lib/sse-driver.ts'

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

// ── harness ──────────────────────────────────────────────────────────────────

// Models the WHATWG gate the flood-kill depends on: a CLOSED EventSource never
// dispatches queued events. `refused` counts the events the gate swallowed.
class FakeES implements ESLike {
  readyState = 0 // CONNECTING
  refused = 0
  url: string
  private listeners = new Map<string, ((e: { data?: string }) => void)[]>()
  constructor(url: string) {
    this.url = url // strip-only mode: no TS parameter properties
  }
  close() {
    this.readyState = 2
  }
  addEventListener(type: 'open' | 'snapshot' | 'delta' | 'error' | 'status', fn: (e: { data?: string }) => void) {
    const a = this.listeners.get(type) ?? []
    a.push(fn)
    this.listeners.set(type, a)
  }
  dispatch(type: string, data?: string) {
    if (this.readyState === 2) {
      this.refused++
      return
    }
    if (type === 'open') this.readyState = 1
    for (const fn of this.listeners.get(type) ?? []) fn({ data })
  }
}

// Virtual clock: timers fire in due order when advance() crosses them; a timer
// scheduled by a firing callback participates. `faulty` makes clearTimeout a
// no-op, simulating a late "cancelled" callback (zombie timer).
class VirtualClock {
  nowMs = 1_000_000_000_000
  private seq = 0
  pending = new Map<number, { at: number; fn: () => void }>()
  scheduled: number[] = [] // every setTimeout delay, for backoff assertions
  private faulty: boolean
  constructor(faulty = false) {
    this.faulty = faulty
  }
  now = () => this.nowMs
  setTimeout = (fn: () => void, ms: number) => {
    this.scheduled.push(ms)
    const id = ++this.seq
    this.pending.set(id, { at: this.nowMs + ms, fn })
    return id
  }
  clearTimeout = (id: unknown) => {
    if (!this.faulty) this.pending.delete(id as number)
  }
  advance(ms: number) {
    const target = this.nowMs + ms
    for (;;) {
      let dueId = -1
      let dueAt = Infinity
      for (const [id, t] of this.pending) if (t.at <= target && t.at < dueAt) ((dueAt = t.at), (dueId = id))
      if (dueId === -1) break
      const t = this.pending.get(dueId)!
      this.pending.delete(dueId)
      this.nowMs = Math.max(this.nowMs, t.at)
      t.fn()
    }
    this.nowMs = target
  }
  jump(ms: number) {
    this.nowMs += ms // time passes without the event loop running (suspend)
  }
}

function makeEnv(opts: { visible?: boolean; faulty?: boolean; health?: boolean } = {}) {
  const clock = new VirtualClock(opts.faulty)
  const created: FakeES[] = []
  let visible = opts.visible ?? true
  const connLog: ConnState[] = []
  const warns: string[] = []
  const healthLog: RtHealth[] = []
  let snapshots = 0
  let deltas = 0
  const driver = createSseDriver('/api/events', {
    createES: (u) => {
      const es = new FakeES(u)
      created.push(es)
      return es
    },
    setTimeout: clock.setTimeout,
    clearTimeout: clock.clearTimeout,
    now: clock.now,
    random: () => 0.5, // jitter factor exactly 1.0 → deterministic delays
    visible: () => visible,
    onConnection: (c) => connLog.push(c),
    applySnapshot: () => snapshots++,
    applyDelta: () => deltas++,
    warn: (m) => warns.push(m),
    onHealth: opts.health ? (h) => healthLog.push(h) : undefined,
  })
  const env = {
    clock,
    created,
    driver,
    connLog,
    warns,
    healthLog,
    setVisible: (v: boolean) => (visible = v),
    counts: () => ({ snapshots, deltas }),
    cur: () => created[created.length - 1],
    liveCount: () => created.filter((e) => e.readyState !== 2).length,
    conn: () => connLog[connLog.length - 1],
    health: () => driver._debug().health, // authoritative current health (initial 'unknown' is never re-emitted)
    // dispatch helpers on the CURRENT ES with server ts = client now + offset
    open: () => env.cur().dispatch('open'),
    snap: (seq = 1, tsOffsetS = 0) =>
      env.cur().dispatch('snapshot', JSON.stringify({ seq, ts: clock.now() / 1000 + tsOffsetS, globals: {}, torrents: [] })),
    delta: (seq: number, tsOffsetS = 0) =>
      env.cur().dispatch('delta', JSON.stringify({ seq, ts: clock.now() / 1000 + tsOffsetS, globals: {}, upserts: [], removed: null })),
    status: (rt: string, since = 0) => env.cur().dispatch('status', JSON.stringify({ rtorrent: rt, since })),
  }
  return env
}

type Env = ReturnType<typeof makeEnv>

// The cross-cutting safety properties, checked after every step in D7/D15.
function invariantViolation(env: Env, visible: boolean): string | null {
  const d = env.driver._debug()
  if (env.liveCount() > 1) return `live ES count ${env.liveCount()}`
  if (!visible && d.retryPending) return 'retry timer pending while hidden'
  if (d.conn === 'idle' && (d.hasES || d.retryPending || d.gracePending)) return `idle but hasES=${d.hasES} retry=${d.retryPending} grace=${d.gracePending}`
  if (d.gracePending && !d.hasES) return 'grace pending without an ES'
  // Health watchdog must never outlive the transport that armed it (invariant 9);
  // inert for the health-less fuzz envs (healthPending is always false there).
  if (d.conn === 'idle' && d.healthPending) return 'watchdog pending while idle'
  if (!visible && d.healthPending) return 'watchdog pending while hidden'
  return null
}

// ── D1: grace survives a recovered error (invariant 7 — round-2 regression #1)
{
  const env = makeEnv()
  env.driver.start()
  env.cur().dispatch('error') // transient blip
  env.clock.advance(1000) // retry fires → new ES
  env.open() // recovery succeeds
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide')
  ok('D1 hide after recovered error grants the grace (not instant idle)', env.driver._debug().gracePending && env.conn() === 'live')
  // ride out most of the grace with live traffic (grace-live keeps applying)
  let seq = 2
  for (let i = 0; i < 11; i++) {
    env.clock.advance(5000)
    env.delta(seq++)
  }
  ok('D1 ES kept through 55s of grace', env.cur().readyState === 1 && env.created.length === 2)
  env.setVisible(true)
  env.driver.signal('show')
  env.clock.advance(HIDDEN_GRACE_MS * 2)
  ok('D1 show cancels the grace; no late goIdle', env.cur().readyState === 1 && env.conn() === 'live' && env.created.length === 2)
}

// ── D2: duplicate hide must not orphan a grace timer (round-2 regression #2)
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide')
  env.driver.signal('hide') // duplicate (spec shouldn't emit it; we must absorb it)
  ok('D2 duplicate hide keeps exactly one pending grace', env.clock.pending.size === 1)
  env.delta(2) // keep lastMsgAt fresh so show keeps the ES
  env.setVisible(true)
  env.driver.signal('show')
  env.clock.advance(HIDDEN_GRACE_MS + 5000)
  ok('D2 no orphan timer closes a live visible connection', env.cur().readyState === 1 && env.conn() === 'live')
}

// ── D3: idle→show emits 'connecting' before any ES event (invariant 8)
{
  const env = makeEnv({ visible: false })
  env.driver.start()
  ok('D3 background-tab start is idle with no ES', env.conn() === 'idle' && env.created.length === 0)
  env.setVisible(true)
  env.driver.signal('show')
  ok('D3 show emits connecting before the ES opens', env.conn() === 'connecting' && env.created.length === 1)
  env.open()
  ok('D3 open completes to live', env.conn() === 'live')
}

// ── D4: dead-OPEN ES on show is recycled; a recently-talking one is kept
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide')
  env.clock.advance(16_000) // silent for 16s (> STALE_LAG_S) while hidden
  env.setVisible(true)
  env.driver.signal('show')
  ok('D4 silent OPEN ES is recycled on show', env.created.length === 2 && env.created[0].readyState === 2)
  ok('D4 the recycle announces connecting (not a stale live)', env.conn() === 'connecting')
}
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide')
  env.clock.advance(5000)
  env.delta(2) // talked 0s ago
  env.setVisible(true)
  env.driver.signal('show')
  ok('D4 recently-talking ES is kept on show (no churn)', env.created.length === 1 && env.cur().readyState === 1)
}

// ── D4b: rtorrent-outage silence → exactly one reconnect per focus, no loop
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.clock.advance(20_000) // healthy-but-silent stream (poll errors broadcast nothing)
  env.setVisible(false)
  env.driver.signal('hide')
  env.setVisible(true)
  env.driver.signal('show')
  ok('D4b one reconnect for a silent stream', env.created.length === 2)
  env.open()
  env.snap(10)
  env.clock.advance(5000)
  ok('D4b no reconnect loop afterwards', env.created.length === 2 && env.conn() === 'live')
}

// ── D5: backoff discipline + the gen-capture bug (invariants 2 & 6)
{
  const env = makeEnv()
  env.driver.start()
  env.cur().dispatch('error')
  env.clock.advance(1000)
  ok('D5 retry actually reconnects (gen captured after closeES)', env.created.length === 2, `created ${env.created.length}, want 2`)
  env.cur().dispatch('error')
  env.clock.advance(2000)
  env.cur().dispatch('error')
  env.clock.advance(4000)
  env.cur().dispatch('error') // 4th failure → retry scheduled at 8000
  env.setVisible(false)
  env.driver.signal('hide')
  ok('D5 hide kills the pending retry', env.conn() === 'idle' && !env.driver._debug().retryPending)
  env.setVisible(true)
  env.driver.signal('show') // immediate attempt, grown backoff preserved
  env.cur().dispatch('error')
  env.clock.advance(16_000)
  env.open() // success at last
  env.cur().dispatch('error') // next failure starts over at 1s
  ok(
    'D5 backoff sequence 1s,2s,4s,8s,16s preserved across idle, reset only on open',
    JSON.stringify(env.clock.scheduled.filter((ms) => ms >= 1000)) === JSON.stringify([1000, 2000, 4000, 8000, 16000, 1000]),
    `scheduled ${JSON.stringify(env.clock.scheduled)}`,
  )
}

// ── D6: never retry hidden (invariant 3)
{
  const env = makeEnv()
  env.driver.start()
  env.setVisible(false)
  env.cur().dispatch('error') // error while hidden
  ok('D6 hidden error goes idle with no timer', env.conn() === 'idle' && !env.driver._debug().retryPending && env.created.length === 1)
}
{
  const env = makeEnv()
  env.driver.start()
  env.cur().dispatch('error') // visible → retry scheduled
  env.setVisible(false) // tab hides without a signal reaching us in time
  env.clock.advance(1000) // retry fires anyway…
  ok('D6 retry firing while hidden goes idle, not open', env.conn() === 'idle' && env.created.length === 1)
}

// ── D7: hostile scripted interleave — single-connection invariant throughout
{
  const env = makeEnv()
  let visible = true
  const show = () => ((visible = true), env.setVisible(true), env.driver.signal('show'))
  const hide = () => ((visible = false), env.setVisible(false), env.driver.signal('hide'))
  const term = () => ((visible = false), env.setVisible(false), env.driver.signal('terminate'))
  const err = () => env.cur()?.dispatch('error')
  const steps: [string, () => void][] = [
    ['start', () => env.driver.start()],
    ['open', () => env.open()],
    ['snap', () => env.snap(1)],
    ['hide', hide],
    ['show', show],
    ['err', err],
    ['hide-during-backoff', hide],
    ['show2', show],
    ['err2', err],
    ['advance-retry', () => env.clock.advance(3000)],
    ['open2', () => env.open()],
    ['term', term],
    ['show3', show],
    ['open3', () => env.open()],
    ['delta', () => env.delta(50)],
    ['hide2', hide],
    ['grace-expiry', () => env.clock.advance(HIDDEN_GRACE_MS + 1000)],
    ['show4', show],
    ['open4', () => env.open()],
    ['stop', () => env.driver.stop()],
  ]
  let bad: string | null = null
  for (const [name, fn] of steps) {
    fn()
    const v = invariantViolation(env, visible)
    if (v && !bad) bad = `${name}: ${v}`
  }
  ok('D7 hostile interleave holds every invariant', bad === null, bad ?? '')
}

// ── D8: flood kill — close() during the first stale message gates the rest
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.delta(2) // calibrated, 2 applied messages
  const before = env.counts()
  const es1 = env.created[0]
  for (let i = 0; i < 500; i++) es1.dispatch('delta', JSON.stringify({ seq: 3 + i, ts: env.clock.now() / 1000 - 3600, globals: {}, upserts: [], removed: null }))
  const after = env.counts()
  ok('D8 zero flood messages applied', after.deltas === before.deltas, `applied ${after.deltas - before.deltas}`)
  ok('D8 first stale message closed the ES; the gate refused the rest', es1.readyState === 2 && es1.refused === 499, `refused ${es1.refused}`)
  ok('D8 exactly one replacement connection', env.created.length === 2)
}

// ── D9: connect-age rule — fresh-ts first message arriving 31s after connect
{
  const env = makeEnv()
  env.driver.start()
  env.clock.advance(31_000) // renderer was suspended between connect and first dispatch
  env.snap(1) // ts is FRESH (the gauge alone would pass it) — age rule must catch it
  ok('D9 over-age first message triggers resync without applying', env.counts().snapshots === 0 && env.created.length === 2)
  env.open()
  env.snap(2)
  ok('D9 connection 2 applies promptly — no loop', env.counts().snapshots === 1 && env.created.length === 2 && env.conn() === 'live')
}

// ── D10: forward clock jump → one resync; connection 2 re-baselines
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.delta(2)
  env.clock.jump(60_000) // suspend: client clock leaps, server ts now reads 60s behind
  env.delta(3, -60)
  ok('D10 post-jump delta triggers resync, not applied', env.counts().deltas === 1 && env.created.length === 2)
  env.open()
  env.snap(10, -60) // all server ts still 60s behind the jumped clock
  env.delta(11, -60)
  env.clock.advance(2000)
  ok('D10 connection 2 re-baselines and stays live', env.counts().deltas === 2 && env.created.length === 2 && env.conn() === 'live')
}

// ── D11: terminate closes synchronously and cancels everything (invariant 4)
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide') // grace pending
  env.driver.signal('terminate')
  ok('D11 terminate: ES CLOSED, idle, zero pending timers', env.created[0].readyState === 2 && env.conn() === 'idle' && env.clock.pending.size === 0)
}

// ── D12: zombie timer (broken clearTimeout) is inert (invariant 6)
{
  const env = makeEnv({ faulty: true })
  env.driver.start()
  env.cur().dispatch('error') // retry scheduled
  env.driver.signal('terminate') // "cancels" the retry — but FaultyClock keeps it
  env.clock.advance(1000) // zombie fires
  ok('D12 zombie retry callback creates no ES', env.created.length === 1 && env.conn() === 'idle')
}

// ── D13: stop() is terminal
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.driver.stop()
  ok('D13 stop closes and reports offline', env.conn() === 'offline' && env.created[0].readyState === 2)
  env.driver.signal('show')
  env.clock.advance(120_000)
  ok('D13 post-stop signals and timers are no-ops', env.created.length === 1 && env.conn() === 'offline')
}

// ── D14: seq tripwire warns on in-connection gaps only
{
  const env = makeEnv()
  env.driver.start()
  env.open()
  env.snap(5)
  env.delta(6)
  ok('D14 contiguous delta: no warn', env.warns.length === 0)
  env.delta(8)
  ok('D14 in-connection gap warns once', env.warns.length === 1)
  env.delta(8) // duplicate (subscribe race shape): not a gap
  ok('D14 duplicate seq does not warn', env.warns.length === 1)
  env.cur().dispatch('error')
  env.clock.advance(1000)
  env.open()
  env.snap(100) // poller advanced while we were away
  env.delta(101)
  ok('D14 cross-connection seq advance does not warn', env.warns.length === 1)
}

// ── D15: seeded fuzz — invariants after every random op
{
  const env = makeEnv()
  let visible = true
  env.driver.start()
  let lcg = 42
  const rnd = (n: number) => {
    lcg = (lcg * 1103515245 + 12345) % 2147483648
    return lcg % n
  }
  let seq = 1
  let bad: string | null = null
  for (let i = 0; i < 500 && !bad; i++) {
    const op = rnd(8)
    if (op === 0) ((visible = true), env.setVisible(true), env.driver.signal('show'))
    else if (op === 1) ((visible = false), env.setVisible(false), env.driver.signal('hide'))
    else if (op === 2) ((visible = false), env.setVisible(false), env.driver.signal('terminate'))
    else if (op === 3) env.cur()?.dispatch('error')
    else if (op === 4) env.open()
    else if (op === 5) env.delta(seq++)
    else if (op === 6) env.delta(seq++, -3600)
    else env.clock.advance(rnd(70) * 1000)
    const v = invariantViolation(env, visible)
    if (v) bad = `op ${i} (${op}): ${v}`
  }
  ok('D15 500-op seeded fuzz holds every invariant', bad === null, bad ?? '')
}

// ── D16: green needs a datum — transport open alone is not 'up'
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  ok('D16 transport open is not green until a datum', env.health() === 'unknown')
  env.snap(1)
  ok('D16 first snapshot turns health up', env.health() === 'up')
}

// ── D17: silence watchdog → stale, then recovery, with NO reconnect
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.snap(1)
  ok('D17 up after snapshot', env.health() === 'up')
  env.clock.advance(HEALTH_STALE_MS + 1000)
  ok('D17 silence trips stale without recycling the ES', env.health() === 'stale' && env.created.length === 1 && env.cur().readyState === 1)
  env.delta(2)
  ok('D17 a fresh datum clears stale back to up', env.health() === 'up')
}

// ── D18: no flap below the staleness window
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.snap(1)
  let seq = 2
  for (let i = 0; i < 5; i++) {
    env.clock.advance(HEALTH_STALE_MS - 1000)
    env.delta(seq++)
  }
  ok('D18 steady traffic stays up', env.health() === 'up' && !env.healthLog.includes('stale'))
}

// ── D19: joiner-while-down beats a cached snapshot, regardless of arrival order
{
  const a = makeEnv({ health: true })
  a.driver.start()
  a.open()
  a.snap(1)
  a.status('unreachable')
  ok('D19 snapshot-then-status resolves down', a.health() === 'down')

  const b = makeEnv({ health: true })
  b.driver.start()
  b.open()
  b.status('unreachable')
  b.snap(1)
  ok('D19 status-then-snapshot resolves down (snapshot cannot mask it)', b.health() === 'down')
}

// ── D20: recovery sequence — down → unknown (de-escalated) → up only on a datum
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.status('unreachable')
  ok('D20 status:unreachable → down', env.health() === 'down')
  env.status('up')
  ok('D20 status:up de-escalates to unknown, not straight to green', env.health() === 'unknown')
  env.delta(2)
  ok('D20 next datum confirms up', env.health() === 'up')
}

// ── D21: transport wins; the watchdog is cleared on a transport drop and the
// cached status is re-shown after reconnect
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.status('unreachable')
  ok('D21 down while live', env.health() === 'down' && env.conn() === 'live')
  env.cur().dispatch('error') // visible → reconnecting
  ok('D21 transport drop → reconnecting, watchdog cleared', env.conn() === 'reconnecting' && env.driver._debug().healthPending === false)
  env.clock.advance(2000) // retry fires → new ES
  env.open()
  env.status('unreachable')
  env.snap(1)
  ok('D21 re-shows down from the replayed status', env.health() === 'down')
}

// ── D22: hide clears the watchdog, show (recently talking) re-arms it
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.snap(1)
  env.setVisible(false)
  env.driver.signal('hide')
  ok('D22 hide clears the watchdog, leaving only the grace timer', env.driver._debug().healthPending === false && env.clock.pending.size === 1)
  env.setVisible(true)
  env.driver.signal('show') // recently talking → ES kept, watchdog re-armed
  ok('D22 show re-arms the watchdog on the kept stream', env.driver._debug().healthPending === true)
}

// ── D23: no-onHealth parity — the gate keeps the transport machine pristine
{
  const env = makeEnv() // no health
  env.driver.start()
  env.open()
  env.snap(1)
  const pendingBefore = env.clock.pending.size
  env.clock.advance(HEALTH_STALE_MS + 5000)
  ok('D23 no HEALTH_STALE_MS timer is ever scheduled', !env.clock.scheduled.includes(HEALTH_STALE_MS))
  ok('D23 pending-timer count matches the health-less baseline', env.clock.pending.size === pendingBefore && env.driver._debug().healthPending === false)
}

// ── D24: green→amber→green on ONE connection (the empirically-validated bug)
{
  const env = makeEnv({ health: true })
  env.driver.start()
  env.open()
  env.snap(1)
  ok('D24 starts live+up', env.conn() === 'live' && env.health() === 'up')
  env.status('unreachable') // rtorrent dies; SSE transport stays warm
  ok('D24 rtorrent down does NOT drop the transport', env.health() === 'down' && env.conn() === 'live')
  env.status('up')
  env.delta(2)
  ok('D24 recovers to up on the same connection', env.health() === 'up')
  ok('D24 transport never flapped while rtorrent died and recovered', !env.connLog.includes('reconnecting') && !env.connLog.includes('offline'))
}

console.log(`sse-driver: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
