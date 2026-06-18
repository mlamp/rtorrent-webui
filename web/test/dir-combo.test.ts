import { splitDest, filterDirs, cleanSaveTo } from '../src/lib/dirCombo.ts'

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
function eq(name: string, got: unknown, want: unknown) {
  ok(name, JSON.stringify(got) === JSON.stringify(want), `got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}

// splitDest: the dir we LIST vs the leaf we FILTER by.
eq('empty -> roots, no leaf', splitDest(''), { dir: '', leaf: '' })
eq('bare slash -> roots, no leaf', splitDest('/'), { dir: '', leaf: '' })
eq('top-level partial -> roots, leaf', splitDest('/da'), { dir: '', leaf: 'da' })
eq('root path -> roots, leaf is its name', splitDest('/data'), { dir: '', leaf: 'data' })
eq('one level deep partial', splitDest('/data/dl'), { dir: '/data', leaf: 'dl' })
eq('trailing slash => list this dir, empty leaf', splitDest('/data/dl/'), { dir: '/data/dl', leaf: '' })
eq('deep partial', splitDest('/data/dl/Mov'), { dir: '/data/dl', leaf: 'Mov' })

// filterDirs: instant client-side typeahead, case-insensitive substring.
const dirs = [{ name: 'Movies' }, { name: 'Music' }, { name: 'books' }]
eq('empty leaf returns all', filterDirs(dirs, ''), dirs)
eq('case-insensitive substring', filterDirs(dirs, 'mu'), [{ name: 'Music' }])
eq(
  'substring matches multiple',
  filterDirs(dirs, 'm').map((d) => d.name),
  ['Movies', 'Music'],
)
eq('no match -> empty', filterDirs(dirs, 'zzz'), [])
eq('null entries -> empty (defensive)', filterDirs(null, ''), [])

// cleanSaveTo: strip drill-in's trailing slash; empty -> undefined (daemon default).
eq('bare dir unchanged', cleanSaveTo('/data/dl'), '/data/dl')
eq('trailing slash stripped', cleanSaveTo('/data/dl/'), '/data/dl')
eq('multiple trailing slashes stripped', cleanSaveTo('/data/dl///'), '/data/dl')
eq('empty -> undefined', cleanSaveTo(''), undefined)
eq('whitespace-only -> undefined', cleanSaveTo('   '), undefined)
eq('surrounding whitespace trimmed', cleanSaveTo('  /data/dl/  '), '/data/dl')

console.log(`dir-combo: ${pass} passed, ${fail} failed`)
if (fail) process.exit(1)
