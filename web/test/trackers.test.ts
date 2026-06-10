import { trackerFailing } from '../src/lib/trackers.ts'

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

// tr shape: { enabled, failed, failedAt, successAt }
const tr = (enabled: boolean, failed: number, failedAt: number, successAt: number) => ({
  enabled,
  failed,
  failedAt,
  successAt,
})

// Disabled trackers never read as failing, even with recorded failures.
ok('disabled is never failing', trackerFailing(tr(false, 9, 200, 0)) === false)

// No failures yet -> not failing.
ok('zero failures is not failing', trackerFailing(tr(true, 0, 0, 100)) === false)

// Last attempt failed (failedAt > successAt) -> failing.
ok('recent failure is failing', trackerFailing(tr(true, 3, 200, 100)) === true)

// Only ever failed (successAt == 0) -> failing.
ok('never-succeeded is failing', trackerFailing(tr(true, 1, 200, 0)) === true)

// Boundary: fail and ok in the same second -> failing (>= not strict >).
ok('same-second fail==ok is failing', trackerFailing(tr(true, 2, 150, 150)) === true)

// Recovered: a later success clears the failing state.
ok('later success clears failing', trackerFailing(tr(true, 5, 100, 200)) === false)

// Enabled but pristine (no counters) -> not failing.
ok('pristine is not failing', trackerFailing(tr(true, 0, 0, 0)) === false)

console.log(`trackers: ${pass} passed, ${fail} failed`)
if (fail > 0) process.exit(1)
