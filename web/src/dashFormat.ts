import type { NumberFormat, Tile } from './types'

// How dashboard tiles write numbers (D-067). Pure, so it can be unit-tested.

type Fmt = Pick<Tile, 'format' | 'currency' | 'decimals' | 'unit'>

const fixed = (n: number, decimals: number) =>
  n.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals })

/** 1,284 · 12.9K · 4.2M · 1.1B: in full under 10,000, compact above. */
export function compact(n: number, decimals?: number): string {
  const abs = Math.abs(n)
  const units: [number, string][] = [
    [1e12, 'T'],
    [1e9, 'B'],
    [1e6, 'M'],
    [1e3, 'K'],
  ]
  if (abs >= 10_000) {
    for (const [size, suffix] of units) {
      if (abs >= size) {
        const v = n / size
        const d = Math.abs(v) >= 100 ? 0 : 1
        return `${fixed(v, d).replace(/\.0$/, '')}${suffix}`
      }
    }
  }
  if (decimals !== undefined) return fixed(n, decimals)
  if (Number.isInteger(n)) return fixed(n, 0)
  return fixed(n, abs >= 100 ? 0 : abs >= 1 ? 1 : 2)
}

function bytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = Math.abs(n)
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  const s = i === 0 ? fixed(v, 0) : fixed(v, v >= 100 ? 0 : 1)
  return `${n < 0 ? '-' : ''}${s} ${units[i]}`
}

/** Seconds as "45s", "3m 20s", "2.5 h", "3.1 days". */
function seconds(n: number): string {
  const abs = Math.abs(n)
  const sign = n < 0 ? '-' : ''
  if (abs < 60) return `${sign}${fixed(abs, abs < 10 && !Number.isInteger(abs) ? 1 : 0)}s`
  if (abs < 3600) {
    const m = Math.floor(abs / 60)
    const s = Math.round(abs % 60)
    return `${sign}${m}m${s ? ` ${String(s).padStart(2, '0')}s` : ''}`
  }
  if (abs < 86_400) return `${sign}${fixed(abs / 3600, 1).replace(/\.0$/, '')} h`
  return `${sign}${fixed(abs / 86_400, 1).replace(/\.0$/, '')} days`
}

/** A value the way its tile says: the big number of a number tile, a bar's value, a table cell. */
export function formatValue(n: number, t: Fmt = {}): string {
  const f: NumberFormat = t.format ?? 'number'
  let s: string
  switch (f) {
    case 'currency': {
      const code = (t.currency || 'USD').toUpperCase()
      const abs = Math.abs(n)
      if (abs >= 10_000) {
        let sym = code
        try {
          sym = (0).toLocaleString('en-US', { style: 'currency', currency: code, maximumFractionDigits: 0 }).replace(/[\d.,\s]/g, '')
        } catch {
          // unknown currency code: write the code
        }
        s = `${n < 0 ? '-' : ''}${sym}${compact(abs)}`
      } else {
        try {
          s = n.toLocaleString('en-US', {
            style: 'currency',
            currency: code,
            minimumFractionDigits: t.decimals ?? (Number.isInteger(n) ? 0 : 2),
            maximumFractionDigits: t.decimals ?? 2,
          })
        } catch {
          s = `${fixed(n, t.decimals ?? 2)} ${code}`
        }
      }
      break
    }
    case 'percent':
      s = `${fixed(n, t.decimals ?? (Number.isInteger(n) ? 0 : 1))}%`
      break
    case 'bytes':
      s = bytes(n)
      break
    case 'duration':
      s = seconds(n)
      break
    default:
      s = compact(n, t.decimals)
  }
  return s
}

export interface Delta {
  /** "+104", "−2.1%" */
  text: string
  /** "+9.5%" beside an absolute change, when it means something */
  pct?: string
  tone: 'good' | 'bad' | 'flat'
  direction: 'up' | 'down' | 'flat'
}

const MINUS = '−'

/** The change from an earlier value: signed, and good or bad by which way the tile says is good news. */
export function delta(now: number, before: number, t: Fmt & Pick<Tile, 'good'> = {}): Delta {
  const diff = now - before
  if (Math.abs(diff) < 1e-9) return { text: 'No change', tone: 'flat', direction: 'flat' }
  const direction = diff > 0 ? 'up' : 'down'
  const sign = diff > 0 ? '+' : MINUS
  const body = t.format === 'percent' ? `${fixed(Math.abs(diff), 1).replace(/\.0$/, '')} pts` : formatValue(Math.abs(diff), t)
  const out: Delta = { text: `${sign}${body}`, direction, tone: 'flat' }
  if (before !== 0 && t.format !== 'percent') {
    const p = (diff / Math.abs(before)) * 100
    out.pct = `${p > 0 ? '+' : MINUS}${fixed(Math.abs(p), Math.abs(p) < 10 ? 1 : 0)}%`
  }
  if (t.good === 'up') out.tone = diff > 0 ? 'good' : 'bad'
  if (t.good === 'down') out.tone = diff < 0 ? 'good' : 'bad'
  return out
}

/** Round tick values for an axis from min to max (0 / 1,000 / 2,000). */
export function niceTicks(min: number, max: number, count = 4): number[] {
  if (!Number.isFinite(min) || !Number.isFinite(max)) return [0]
  if (min === max) {
    const pad = Math.abs(min) || 1
    min -= pad / 2
    max += pad / 2
  }
  const raw = (max - min) / Math.max(1, count)
  const mag = 10 ** Math.floor(Math.log10(raw))
  const norm = raw / mag
  const step = (norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 2.5 ? 2.5 : norm <= 5 ? 5 : 10) * mag
  const start = Math.floor(min / step) * step
  const end = Math.ceil(max / step) * step
  const out: number[] = []
  for (let v = start; v <= end + step / 2; v += step) out.push(Math.round(v / step) * step)
  return out
}

/** An axis tick: in full under 10,000, compact on an axis that reaches it. */
export function tickLabel(v: number, axisMax: number, t: Fmt = {}): string {
  if (t.format && t.format !== 'number') return formatValue(v, { ...t, decimals: undefined })
  if (Math.abs(axisMax) >= 10_000) return compact(v)
  return Number.isInteger(v) ? fixed(v, 0) : fixed(v, 1)
}

const pad = (n: number) => String(n).padStart(2, '0')
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** A time on a chart's axis or tooltip, as much as the range needs. */
export function timeLabel(iso: string, spanMs: number, long = false): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const day = `${MONTHS[d.getMonth()]} ${d.getDate()}`
  if (long) return `${day}, ${hm}`
  return spanMs <= 36 * 3600_000 ? hm : day
}

/** "every 15 min", "every hour", "every day". */
export function refreshLabel(r: string): string {
  const m = /^(\d+)([smhd])$/.exec(r)
  if (!m) return `every ${r}`
  const n = Number(m[1])
  const unit = { s: 'second', m: 'minute', h: 'hour', d: 'day' }[m[2] as 's' | 'm' | 'h' | 'd']
  if (n === 1) return `every ${unit}`
  return `every ${n} ${unit === 'minute' ? 'min' : `${unit}s`}`
}

/** "in 4 min", "in 1 h", "now". */
export function untilLabel(iso: string | undefined, now: Date = new Date()): string {
  if (!iso) return ''
  const ms = new Date(iso).getTime() - now.getTime()
  if (Number.isNaN(ms) || ms <= 30_000) return 'soon'
  const mins = Math.round(ms / 60_000)
  if (mins < 60) return `in ${mins} min`
  const h = Math.round(mins / 60)
  if (h < 48) return `in ${h} h`
  return `in ${Math.round(h / 24)} days`
}

/** A chart category on an axis: a plain date ("2026-10-16") as "Oct 16" ("Oct 16, 2026" long); anything else as it is. */
export function categoryLabel(s: string, long = false): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s)
  const month = m ? MONTHS[Number(m[2]) - 1] : undefined
  if (!m || !month) return s
  const day = `${month} ${Number(m[3])}`
  return long ? `${day}, ${m[1]}` : day
}

/**
 * Where to draw x-axis labels: always the first and the last point, and as
 * many evenly spaced between as fit with `gap` pixels between labels. Each
 * label is centred on its point but kept inside [left, right]. Returns each
 * drawn label's point index and its left edge.
 */
export function placeLabels(xs: number[], widths: number[], left: number, right: number, gap = 12): { i: number; x: number }[] {
  const n = xs.length
  if (n === 0) return []
  const box = (i: number) => {
    const w = widths[i] ?? 0
    return Math.max(left, Math.min((xs[i] ?? 0) - w / 2, right - w))
  }
  if (n === 1) return [{ i: 0, x: box(0) }]
  const avg = widths.reduce((a, b) => a + b, 0) / n
  for (let k = Math.min(n, Math.floor((right - left + gap) / (avg + gap))); k >= 2; k--) {
    const picked = Array.from({ length: k }, (_, j) => Math.round((j * (n - 1)) / (k - 1)))
    const placed = picked.map((i) => ({ i, x: box(i) }))
    const fits = placed.every((p, j) => j === 0 || p.x - ((placed[j - 1] as { x: number }).x + (widths[(placed[j - 1] as { i: number }).i] ?? 0)) >= gap)
    if (fits) return placed
  }
  return [{ i: 0, x: box(0) }]
}

/**
 * The line of a failed command's error output that says the most: one that
 * starts with "Error"/"[ERROR]"/"fatal", else one that names a failure (not
 * a stack frame), else the last line.
 */
export function errorLine(note: string, max = 200): string {
  const lines = note
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  const head = lines.find((l) => /^(✘\s*)?\[?(error|fatal)\]?\b/i.test(l) && /\w/.test(l.replace(/^(✘\s*)?\[?(error|fatal)\]?:?/i, '')))
  const said = lines.find(
    (l) => !l.startsWith('at ') && !/new Error\(/.test(l) && /\b(error|failed|denied|expired|refused|not found|no such|permission|timed out)\b/i.test(l),
  )
  const line = head ?? said ?? lines[lines.length - 1] ?? ''
  return line.length > max ? `${line.slice(0, max - 1)}…` : line
}
