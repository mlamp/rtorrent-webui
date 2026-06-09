import { short } from '../src/lib/format/index.ts'

let pass = 0, fail = 0
function eq(name: string, got: unknown, want: unknown) {
  if (JSON.stringify(got) === JSON.stringify(want)) { pass++; return }
  fail++
  console.error(`FAIL ${name}\n  got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}

const KiB = 1024, MiB = 1024 * 1024, GiB = 1024 * 1024 * 1024

// short() contract: binary (1024) units, max 3 significant figures / 3 chars.
//   < 10  -> one decimal (1.0, 1.1, 9.9)
//   10..999 -> whole number, no dot (10, 110, 999)
//   >= 1000 -> carry to the next unit (999K -> 1.0M)
eq('zero', short(0), '0')
eq('negative/guard', short(-5), '0')

// raw bytes stay whole (no ".0" on a byte count)
eq('5 B', short(5), '5')
eq('512 B', short(512), '512')

// below ten -> one decimal
eq('1.0K', short(1 * KiB), '1.0K')
eq('1.1K', short(1126), '1.1K')
eq('1.5K', short(1536), '1.5K')
eq('9.9K', short(Math.round(9.9 * KiB)), '9.9K')

// ten and up -> integer, never a decimal (the "no 10.1" rule)
eq('10K', short(10 * KiB), '10K')
eq('110K (was 109.9)', short(112537), '110K')
eq('999K', short(999 * KiB), '999K')
eq('10M not 10.1M', short(Math.round(10.1 * MiB)), '10M')
eq('34G not 34.15G', short(Math.round(34.15 * GiB)), '34G')

// carry: a value that would print a 4th digit rolls up a unit
eq('1023K -> 1.0M', short(1023 * KiB), '1.0M')
eq('1.0M', short(MiB), '1.0M')
eq('999M', short(999 * MiB), '999M')
eq('1.0G', short(GiB), '1.0G')

// width invariant: the numeric part is never more than 3 characters
for (let p = 0; p < 40; p++) {
  const n = Math.round(2 ** p * 1.3)
  const s = short(n)
  const num = s.replace(/[KMGTP]$/, '')
  if (num.length > 3) { fail++; console.error(`FAIL width ${n} -> ${s} (${num.length} chars)`) }
}

console.log(`\nformat.short: ${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
