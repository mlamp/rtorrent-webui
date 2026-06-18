import {
  headerText,
  summaryText,
  primaryLabel,
  removeURL,
  reduceRemoveTargets,
  requestDeletesData,
  noCheckboxNote,
  namesPreview,
  summarizeRemoval,
  type RemoveOutcome,
} from '../src/lib/removeDialog.logic.ts'

let pass = 0,
  fail = 0
function eq(name: string, got: unknown, want: unknown) {
  if (JSON.stringify(got) === JSON.stringify(want)) {
    pass++
    return
  }
  fail++
  console.error(`FAIL ${name}\n  got  ${JSON.stringify(got)}\n  want ${JSON.stringify(want)}`)
}
function ok(name: string, cond: boolean) {
  if (cond) {
    pass++
    return
  }
  fail++
  console.error(`FAIL ${name}`)
}

// headers
eq('header single', headerText(1), 'Remove this torrent?')
eq('header bulk', headerText(3), 'Remove 3 torrents?')

// summary copy must make the disk consequence unmistakable
ok('summary keep says KEPT', summaryText(false).includes('KEPT'))
ok('summary delete says PERMANENTLY DELETED', summaryText(true).includes('PERMANENTLY DELETED'))

// primary button label tracks (deleteData, busy); busy wins
eq('label remove', primaryLabel(false, false), 'REMOVE')
eq('label delete', primaryLabel(true, false), 'DELETE FILES')
eq('label busy', primaryLabel(false, true), 'REMOVING…')
eq('label busy+delete', primaryLabel(true, true), 'REMOVING…')

// URL builder: ?data=true only when deletion is requested
eq('url keep', removeURL('ABCDEF', false), '/api/torrents/ABCDEF')
eq('url delete', removeURL('ABCDEF', true), '/api/torrents/ABCDEF?data=true')

// initial state reducer: empty ignored, deleteData never sticky
eq('reduce empty', reduceRemoveTargets([]), null)
eq('reduce one', reduceRemoveTargets([{ hash: 'h', name: 'n' }]), { deleteData: false })

// the delete gate: server-allowed AND user-checked (all four combinations)
eq('gate off+off', requestDeletesData(false, false), false)
eq('gate off+on', requestDeletesData(false, true), false)
eq('gate on+off', requestDeletesData(true, false), false)
eq('gate on+on', requestDeletesData(true, true), true)

// no-capability copy distinguishes every config state (never mislabels WHY)
ok('note idle = checking', noCheckboxNote('idle').includes('checking server capabilities'))
ok('note loaded = disabled by config', noCheckboxNote('loaded').includes('disabled by this server'))
ok('note failed = unknown', noCheckboxNote('failed').includes('capabilities unknown'))
ok('note states are distinct', new Set([noCheckboxNote('idle'), noCheckboxNote('loaded'), noCheckboxNote('failed')]).size === 3)

// names preview caps the list and keeps full targets (so the view keys on the
// unique hash, never the non-unique name)
const many = Array.from({ length: 10 }, (_, i) => ({ hash: `h${i}`, name: `n${i}` }))
eq('names cap shown', namesPreview(many, 8).shown.length, 8)
eq('names cap more', namesPreview(many, 8).more, 2)
eq('names under cap more', namesPreview(many.slice(0, 3), 8).more, 0)
eq('names preview carries hashes', namesPreview(many, 8).shown[0].hash, 'h0')
// two torrents that share a display name must BOTH survive with distinct hashes
// (deduping by name, or keying on name, would drop/crash one of them)
const dup = [
  { hash: 'aaa', name: 'Movie (2024)' },
  { hash: 'bbb', name: 'Movie (2024)' },
]
eq('duplicate names both kept', namesPreview(dup).shown.length, 2)
eq('duplicate names keep distinct hashes', new Set(namesPreview(dup).shown.map((t) => t.hash)).size, 2)

// toast summary counts only the server-reported truth
const F = (erased: boolean, dataDeleted: boolean): RemoveOutcome => ({ status: 'fulfilled', erased, dataDeleted })
const R: RemoveOutcome = { status: 'rejected' }
eq('summary two erased', summarizeRemoval([F(true, false), F(true, false)]), 'Removed 2 torrents')
eq('summary erased+deleted', summarizeRemoval([F(true, true), F(true, false)]), 'Removed 2 torrents · deleted files from 1')
eq('summary single deleted', summarizeRemoval([F(true, true)]), 'Removed 1 torrent · deleted files from 1')
eq('summary all rejected', summarizeRemoval([R, R]), null)
eq('summary nothing happened', summarizeRemoval([F(false, false)]), 'No torrents were removed')
eq('summary partial success ignores rejected count', summarizeRemoval([F(true, false), R]), 'Removed 1 torrent')

console.log(`\nremove-dialog.logic: ${pass} passed, ${fail} failed`)
process.exit(fail ? 1 : 0)
