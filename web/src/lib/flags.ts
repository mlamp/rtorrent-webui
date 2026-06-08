// Country-flag emoji render unevenly across platforms: macOS, Windows, iOS,
// Android and Firefox (bundled Twemoji) draw real flags, but Chrome/Chromium on
// Linux just paints the two regional-indicator letters — and some headless
// envs draw nothing (tofu). Rather than ship a few MB of SVG flag assets, we
// detect support once and let CountryFlag fall back to a styled code badge
// where flags don't render, so the peer list stays tidy everywhere.
//
// Detection: paint 🇬🇧 (a multi-colour flag) to a canvas in solid black. A real
// flag glyph is colour emoji and ignores fillStyle, so it leaves saturated
// pixels; the letter fallback (and tofu) stay monochrome. If we find any
// clearly-coloured pixel, flags render here.
function detectFlagEmoji(): boolean {
  if (typeof document === 'undefined') return false
  try {
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 16
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    if (!ctx) return false
    ctx.textBaseline = 'top'
    ctx.font = '16px sans-serif'
    ctx.fillStyle = '#000'
    ctx.fillText('\u{1F1EC}\u{1F1E7}', 0, 0) // 🇬🇧
    const { data } = ctx.getImageData(0, 0, 16, 16)
    for (let i = 0; i < data.length; i += 4) {
      if (data[i + 3] === 0) continue // transparent
      const r = data[i],
        g = data[i + 1],
        b = data[i + 2]
      if (Math.abs(r - g) > 24 || Math.abs(g - b) > 24 || Math.abs(r - b) > 24) return true
    }
    return false
  } catch {
    return false
  }
}

// Computed once at module load (client-only SPA, so `document` is available).
export const flagEmojiSupported = detectFlagEmoji()

// Two-letter ISO code → flag emoji (regional indicator pair). Empty if invalid.
export function codeToFlag(code: string): string {
  if (!code || code.length !== 2) return ''
  return String.fromCodePoint(...[...code.toUpperCase()].map((ch) => 0x1f1e6 + ch.charCodeAt(0) - 65))
}
