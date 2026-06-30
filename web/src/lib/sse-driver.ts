// .ts extension so plain node (--experimental-strip-types) can load this module
// directly in test/sse-driver.test.ts; the type-only import below is erased.
import { makeStalenessGauge, STALE_LAG_S } from './stale.ts'
import type { SnapshotMsg, DeltaMsg } from './types/torrent'

export const HIDDEN_GRACE_MS = 60_000 // hidden this long → disconnect (knob for glanceable titles vs idle disconnect)
export const BACKOFF_MIN_MS = 1_000
export const BACKOFF_MAX_MS = 30_000
const FIRST_MSG_AGE_MS = 30_000 // connect-age rule: first message of a connection older than this = resume backlog

export const HEALTH_STALE_MS = 20_000 // ES open+visible but no snapshot/delta this long → rtorrent likely stale.
                                      // Client BACKSTOP only; > pollTimeout(15s) so one slow poll never flaps it.

export type ConnState = 'connecting' | 'live' | 'reconnecting' | 'offline' | 'idle'
export type RtHealth = 'up' | 'down' | 'stale' | 'unknown'
export type LifecycleSignal = 'show' | 'hide' | 'terminate'

export type ESLike = {
  readyState: number
  close(): void
  addEventListener(type: 'open' | 'snapshot' | 'delta' | 'error' | 'status', fn: (e: { data?: string }) => void): void
}

export type DriverDeps = {
  createES(url: string): ESLike
  setTimeout(fn: () => void, ms: number): unknown
  clearTimeout(id: unknown): void
  now(): number // ms epoch
  random(): number // retry jitter
  visible(): boolean
  onConnection(c: ConnState): void
  applySnapshot(d: SnapshotMsg): void
  applyDelta(d: DeltaMsg): void
  warn(msg: string): void
  // OPTIONAL: presence enables the rtorrent-health axis (status listener + silence
  // watchdog). Omit it (existing tests do) and the driver behaves exactly as before.
  onHealth?(h: RtHealth): void
}

/**
 * Visibility-aware EventSource lifecycle, dependency-injected so the whole
 * thing runs under plain-node tests with a fake EventSource and virtual clock
 * (test/sse-driver.test.ts checks every invariant below).
 *
 * Layers of defense against the frozen-tab resume flood (a hidden tab whose
 * queued SSE events all dispatch at once on reactivation):
 *  1. graceful idle — hidden ≥ HIDDEN_GRACE_MS closes the stream;
 *  2. synchronous close on terminate (freeze/pagehide), before the handler returns;
 *  3. stale-burst guard — skew-calibrated message lag + a connect-age rule for
 *     the first message + a last-message-age check on show. On detection we
 *     close and reconnect (the server resyncs us with its cached snapshot);
 *     we NEVER skip an individual delta — deltas are diffs, dropping one
 *     corrupts torrent state. es.close() is the load-bearing flood killer
 *     (queued tasks check readyState CLOSED before dispatching, per spec);
 *     the generation counter is insurance for late timers only.
 *
 * Invariants (each machine-checked):
 *  1. at most one live EventSource — only open() creates one, closeES-first
 *  2. backoff resets ONLY in the 'open' listener (a connection succeeded)
 *  3. never a retry timer while hidden
 *  4. every entry to idle cancels both timers
 *  5. stale handling closes before anything else
 *  6. the retry callback re-checks gen (captured AFTER closeES bumped it),
 *     stopped, and visibility; the grace callback needs none — every
 *     invalidating transition clears it
 *  7. a timer variable is non-null iff its timer is pending (the hide handler
 *     branches on retryTimer truthiness, so this is load-bearing)
 *  8. the emitted ConnState never lies to a visible tab
 *  9. rtorrent-health is a SEPARATE axis (onHealth), never folded into ConnState;
 *     the silence watchdog only signals — it NEVER opens/closes an ES; obeys
 *     invariant 7 (var non-null iff pending) and 4 (idle/terminate/stop cancels it).
 *     All health code is gated on deps.onHealth so the transport machine is
 *     unchanged when absent.
 */
export function createSseDriver(url: string, deps: DriverDeps) {
  let es: ESLike | null = null
  let gen = 0
  let graceTimer: unknown
  let retryTimer: unknown
  let backoffMs = BACKOFF_MIN_MS
  let gauge = makeStalenessGauge()
  let gotFirstMsg = false
  let connectAtMs = 0
  let lastMsgAtMs = 0
  let lastSeq = 0
  let conn: ConnState = 'connecting'
  let stopped = false

  // ── rtorrent-health axis (inert unless deps.onHealth is provided) ──
  const healthOn = deps.onHealth !== undefined
  let healthTimer: unknown
  let srvDown = false   // last `status` event said rtorrent is unreachable (authoritative)
  let seenData = false  // a snapshot/delta arrived on THIS connection
  let dataStale = false // silence watchdog tripped on THIS connection
  let health: RtHealth = 'unknown'

  // down (server) > watchdog stale > data > unknown. srvDown is checked FIRST so a
  // replayed cached snapshot can never mask a cached status:unreachable for a joiner.
  const resolveHealth = (): RtHealth =>
    srvDown ? 'down' : dataStale ? 'stale' : seenData ? 'up' : 'unknown'
  const setHealth = (h: RtHealth) => {
    if (!healthOn || h === health) return
    health = h
    deps.onHealth!(h)
  }
  const clearHealthTimer = () => {
    if (healthTimer !== undefined) deps.clearTimeout(healthTimer)
    healthTimer = undefined
  }
  const armHealth = () => {
    if (!healthOn) return
    clearHealthTimer()
    healthTimer = deps.setTimeout(() => {
      healthTimer = undefined
      dataStale = true
      setHealth(resolveHealth())
    }, HEALTH_STALE_MS)
  }
  const onDatum = () => { // fresh poll proves reachable+fresh; does NOT clear srvDown (resolveHealth orders it)
    seenData = true
    dataStale = false
    armHealth()
    setHealth(resolveHealth())
  }

  const emit = (c: ConnState) => {
    conn = c
    deps.onConnection(c)
  }
  const clearRetry = () => {
    if (retryTimer !== undefined) deps.clearTimeout(retryTimer)
    retryTimer = undefined
  }
  const clearGrace = () => {
    if (graceTimer !== undefined) deps.clearTimeout(graceTimer)
    graceTimer = undefined
  }
  const closeES = () => {
    gen++
    clearRetry()
    clearHealthTimer()
    es?.close()
    es = null
  }
  const goIdle = () => {
    clearGrace()
    closeES()
    emit('idle')
  }

  // Shared stale check for both message listeners. Returns true when the
  // message was a backlog marker and the connection has been recycled.
  const staleAndRecycled = (ts: number): boolean => {
    const tooOldFirst = !gotFirstMsg && deps.now() - connectAtMs > FIRST_MSG_AGE_MS
    gotFirstMsg = true
    if (!tooOldFirst && gauge(ts, deps.now() / 1000) === 'fresh') return false
    closeES() // kills the rest of the flood synchronously: CLOSED gates dispatch per spec
    if (deps.visible()) open()
    else goIdle()
    return true
  }

  const open = () => {
    closeES() // invariant 1
    gauge = makeStalenessGauge()
    gotFirstMsg = false
    lastSeq = 0
    srvDown = false
    seenData = false
    dataStale = false
    connectAtMs = lastMsgAtMs = deps.now()
    if (conn === 'idle') emit('connecting') // invariant 8
    setHealth('unknown') // never claim green across a reconnect until a datum/status confirms
    const myGen = gen
    es = deps.createES(url)

    es.addEventListener('open', () => {
      if (myGen !== gen || stopped) return
      backoffMs = BACKOFF_MIN_MS // invariant 2 — the only reset site
      emit('live')
      armHealth() // transport live → start watching for data silence
    })

    es.addEventListener('snapshot', (e) => {
      if (myGen !== gen || stopped) return
      const d: SnapshotMsg = JSON.parse(e.data!)
      lastMsgAtMs = deps.now()
      if (staleAndRecycled(d.ts)) return
      lastSeq = d.seq
      deps.applySnapshot(d)
      emit('live')
      onDatum()
    })

    es.addEventListener('delta', (e) => {
      if (myGen !== gen || stopped) return
      const d: DeltaMsg = JSON.parse(e.data!)
      lastMsgAtMs = deps.now()
      if (staleAndRecycled(d.ts)) return
      // Tripwire only — in-connection gaps are impossible by server construction
      // (single poller goroutine, FIFO per-sub channel, kill-don't-drop). The
      // subscribe race can deliver snapshot N then delta N; the duplicate applies
      // harmlessly (absolute field values), so seq <= last is NOT a gap.
      if (lastSeq && d.seq > lastSeq + 1) deps.warn(`sse: seq gap ${lastSeq}→${d.seq}`)
      lastSeq = d.seq
      deps.applyDelta(d)
      emit('live')
      onDatum()
    })

    if (healthOn)
      es.addEventListener('status', (e) => {
        if (myGen !== gen || stopped) return
        // Health-only frame: must NOT touch lastSeq, gauge, lastMsgAtMs, or connectAtMs.
        const d = JSON.parse(e.data!) as { rtorrent?: string }
        srvDown = d.rtorrent === 'unreachable' // 'up' clears it; next datum → green
        setHealth(resolveHealth())
      })

    es.addEventListener('error', () => {
      if (myGen !== gen || stopped) return
      closeES() // bumps gen — the retry callback must compare against the NEW value
      if (!deps.visible()) {
        goIdle() // invariant 3
        return
      }
      emit('reconnecting')
      const g = gen // NOT myGen (invariant 6): myGen is stale after the closeES above
      retryTimer = deps.setTimeout(
        () => {
          retryTimer = undefined // invariant 7
          if (g !== gen || stopped) return
          if (deps.visible()) open()
          else goIdle()
        },
        backoffMs * (0.8 + deps.random() * 0.4),
      )
      backoffMs = Math.min(backoffMs * 2, BACKOFF_MAX_MS)
    })
  }

  const signal = (s: LifecycleSignal) => {
    if (stopped) return
    if (s === 'terminate') {
      goIdle() // freeze/pagehide: close synchronously, inside the event handler
      return
    }
    if (s === 'hide') {
      if (retryTimer !== undefined) {
        goIdle() // a hidden tab never runs the backoff loop (invariant 3)
        return
      }
      clearHealthTimer() // hidden: dot not shown; resume the silence watchdog on show
      // Keep the ES (even an in-flight connect) so quick alt-tabs cause zero churn.
      if (es && graceTimer === undefined)
        graceTimer = deps.setTimeout(() => {
          graceTimer = undefined
          goIdle()
        }, HIDDEN_GRACE_MS)
      return
    }
    // 'show'
    clearGrace()
    // Alive AND recently talking → keep it. The age check covers silent TCP
    // death during machine sleep (no error/message ever dispatches; the gauge
    // can't see a message that never arrives). A live stream gets a delta every
    // successful poll tick (~1s); during an rtorrent outage the stream is
    // legitimately silent and this costs one redundant, harmless reconnect per
    // focus — accepted; don't weaken the check, that reopens the sleep hole.
    if (es && es.readyState !== 2 /* CLOSED */ && deps.now() - lastMsgAtMs < STALE_LAG_S * 1000) {
      armHealth() // resume the silence watchdog for the now-visible, still-live stream
      return
    }
    if (es) emit('connecting') // invariant 8: a dead-ES reconnect must not keep claiming 'live'
    open() // keeps grown backoffMs (invariant 2)
  }

  return {
    start() {
      if (deps.visible()) open()
      else emit('idle') // opened in a background tab: connect on first show
    },
    signal,
    stop() {
      stopped = true
      clearGrace()
      closeES()
      emit('offline')
    },
    /** test-only introspection — cheaper and more honest than inferring timer state */
    _debug() {
      return {
        hasES: es !== null,
        esState: es ? es.readyState : null,
        retryPending: retryTimer !== undefined,
        gracePending: graceTimer !== undefined,
        backoffMs,
        gen,
        conn,
        health,
        healthPending: healthTimer !== undefined,
      }
    },
  }
}
