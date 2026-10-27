const pad = (n: number) => String(n).padStart(2, '0')

function parse(iso: string | undefined): Date | null {
  if (!iso) return null
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? null : d
}

/** 24-hour local time with seconds, e.g. "20:38:44". */
export function clock(iso: string | undefined): string {
  const d = parse(iso)
  return d ? `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}` : ''
}

/** Local time without seconds, e.g. "20:30". */
export function clockShort(iso: string | undefined): string {
  const d = parse(iso)
  return d ? `${pad(d.getHours())}:${pad(d.getMinutes())}` : ''
}

/** "0.4s", "21.4s", "1m 05s". */
export function duration(ms: number | undefined): string {
  if (ms === undefined) return ''
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const total = Math.round(ms / 1000)
  return `${Math.floor(total / 60)}m ${pad(total % 60)}s`
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

/** The sidebar's "when": minutes and hours for today, then "Yesterday", then a date. */
export function relativeWhen(iso: string, now: Date = new Date()): string {
  const d = parse(iso)
  if (!d) return ''
  const mins = Math.floor((now.getTime() - d.getTime()) / 60_000)
  if (mins < 1) return 'Just now'
  if (mins < 60) return `${mins} min ago`
  const startOfDay = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime()
  const days = Math.round((startOfDay(now) - startOfDay(d)) / 86_400_000)
  if (days <= 0) return `${Math.floor(mins / 60)} h ago`
  if (days === 1) return 'Yesterday'
  const date = `${MONTHS[d.getMonth()]} ${d.getDate()}`
  return d.getFullYear() === now.getFullYear() ? date : `${date}, ${d.getFullYear()}`
}

export function plural(n: number, one: string, many: string = `${one}s`): string {
  return n === 1 ? `${n} ${one}` : `${n} ${many}`
}

export interface TextPart {
  text: string
  code: boolean
}

/** Splits agent text on `backtick` spans. The pieces are rendered as React elements, never as HTML. */
export function splitCode(text: string): TextPart[] {
  const parts: TextPart[] = []
  const re = /`([^`]+)`/g
  let last = 0
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) parts.push({ text: text.slice(last, m.index), code: false })
    parts.push({ text: m[1] ?? '', code: true })
    last = m.index + m[0].length
  }
  if (last < text.length) parts.push({ text: text.slice(last), code: false })
  return parts
}
