const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

/** Human-readable binary size. */
export function bytes(n: number, digits = 1): string {
  if (!n || n <= 0) return '0 B'
  // Clamp the exponent to >= 0 so sub-1 inputs don't index UNITS[-1] (undefined).
  const i = Math.min(UNITS.length - 1, Math.max(0, Math.floor(Math.log(n) / Math.log(1024))))
  const v = n / Math.pow(1024, i)
  return `${i === 0 ? v : v.toFixed(digits)} ${UNITS[i]}`
}

/** Transfer rate (bytes/s). */
export function rate(n: number): string {
  return n > 0 ? `${bytes(n)}/s` : '0 B/s'
}

/**
 * Compact magnitude in binary (1024) units, capped at 3 significant figures so
 * the number is at most 3 characters wide: below ten keeps one decimal
 * (1.0, 1.1, 9.9); 10..999 are whole numbers with no dot (10, 99, 110, 999);
 * then it carries to the next unit — 999K -> 1.0M. e.g. 4128768 -> "3.9M".
 */
export function short(n: number): string {
  if (!n || n <= 0) return '0'
  const u = ['', 'K', 'M', 'G', 'T', 'P']
  let v = Math.abs(n)
  let i = 0
  // Step up a unit while the value would round to a 4th digit (>= 1000), so we
  // never exceed 3 sig figs — 999 rolls over to "1.0" of the next unit.
  while (v >= 999.5 && i < u.length - 1) {
    v /= 1024
    i++
  }
  // One decimal below ten (KiB and up); whole numbers from 10 up and for raw bytes.
  const decimals = i > 0 && v < 9.95 ? 1 : 0
  return `${v.toFixed(decimals)}${u[i]}`
}

/** rtorrent stores ratio ×1000. */
export function ratio(permille: number): string {
  return (permille / 1000).toFixed(2)
}

/** Fraction (0..1) → percent. */
export function percent(frac: number): string {
  const p = frac * 100
  return `${p >= 100 ? 100 : p.toFixed(p < 10 ? 1 : 0)}%`
}

/** Seconds remaining → compact ETA. '—' when unknown (done, stalled, or idle). */
export function eta(seconds: number): string {
  if (!isFinite(seconds) || seconds <= 0) return '—'
  let s = Math.floor(seconds)
  const d = Math.floor(s / 86400)
  s %= 86400
  const h = Math.floor(s / 3600)
  s %= 3600
  const m = Math.floor(s / 60)
  s %= 60
  if (d) return `${d}d ${h}h`
  if (h) return `${h}h ${m}m`
  if (m) return `${m}m ${s}s`
  return `${s}s`
}

/** Host of a tracker announce URL ("https://bgp.technology/announce" → "bgp.technology").
 *  Falls back to the raw string for non-URLs; "" stays "". */
export function trackerHost(u: string): string {
  if (!u) return ''
  const m = u.replace(/^[a-z]+:\/\//i, '').match(/^([^/:]+)/)
  return m ? m[1] : u
}

/** Epoch seconds → "3d ago" style. */
export function relativeTime(epoch: number): string {
  if (!epoch) return '—'
  const diff = Date.now() / 1000 - epoch
  if (diff < 60) return 'just now'
  const m = Math.floor(diff / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  const mo = Math.floor(d / 30)
  if (mo < 12) return `${mo}mo ago`
  return `${Math.floor(mo / 12)}y ago`
}
