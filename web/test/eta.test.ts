import { nextSmoothRate, etaSecondsFor } from '../src/lib/eta.ts'
import { eta } from '../src/lib/format/index.ts'

let pass = 0, fail = 0
function eq(name: string, got: unknown, want: unknown) {
  if (JSON.stringify(got) === JSON.stringify(want)) { pass++; return }
  fail++
  console.error(`FAIL ${name}\n  got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}

// format.eta — unknown renders as '—' (not '∞'); known values format compactly
eq('eta Infinity', eta(Infinity), '—')
eq('eta NaN', eta(NaN), '—')
eq('eta 0', eta(0), '—')
eq('eta negative', eta(-5), '—')
eq('eta 45s', eta(45), '45s')
eq('eta 2m5s', eta(125), '2m 5s')
eq('eta 1h2m', eta(3725), '1h 2m')
eq('eta 1d1h', eta(90000), '1d 1h')

// EWMA — seeds from the first sample, then blends 70/30
eq('smooth seed', nextSmoothRate(0, 1000), 1000)
eq('smooth blend', nextSmoothRate(1000, 2000), 1300)

// etaSecondsFor — Infinity (→'—') when done or stalled; smoothed estimate otherwise
eq('eta done', etaSecondsFor(0, 1000, 1000), Infinity)
eq('eta stalled', etaSecondsFor(500, 0, 1000), Infinity)
eq('eta uses smoothRate', etaSecondsFor(1000, 5000, 500), 2)
eq('eta falls back to downRate', etaSecondsFor(1000, 250, 0), 4)

console.log(`\neta.ts/format.eta: ${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
