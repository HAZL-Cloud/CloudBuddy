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
