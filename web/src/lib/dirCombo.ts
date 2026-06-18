// Pure path helpers behind the Add dialog's save-to directory combobox. Kept out
// of the component so they're unit-testable (the component wires them to state +
// fetches). See components/toolbar/AddTorrentDialog.svelte.

export type DirSplit = { dir: string; leaf: string }

/**
 * Split a save-to value into the directory to LIST and the trailing leaf to
 * FILTER its children by. A trailing slash means "inside this dir" (leaf ""):
 * otherwise the final segment is a partial name filtering its parent's children.
 * dir "" means the configured roots (top level).
 */
export function splitDest(d: string): DirSplit {
  if (d === '') return { dir: '', leaf: '' }
  const i = d.lastIndexOf('/')
  if (i <= 0) return { dir: '', leaf: d.slice(i + 1) } // "/data" -> roots, leaf "data"
  return { dir: d.slice(0, i), leaf: d.slice(i + 1) }
}

/** Case-insensitive substring filter of directory entries by the typed leaf. */
export function filterDirs<T extends { name: string }>(entries: T[] | null | undefined, leaf: string): T[] {
  if (!entries) return []
  const l = leaf.toLowerCase()
  return l ? entries.filter((e) => e.name.toLowerCase().includes(l)) : entries
}

/**
 * The value actually submitted to rtorrent. Drill-in leaves a trailing slash
 * ("/data/dl/"), which we strip to the bare directory; empty becomes undefined
 * so the daemon uses its default download dir.
 */
export function cleanSaveTo(dest: string): string | undefined {
  return dest.trim().replace(/\/+$/, '') || undefined
}
