import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react'
import { delta, formatValue, niceTicks, tickLabel, timeLabel } from '../dashFormat'
import { relativeWhen } from '../format'
import type { Bar, Point, Tile, TileColumn, TileValue } from '../types'
import { Icon } from './Icon'

// CloudBuddy's own widget kit for dashboards (D-067). Everything the agent
// or a command wrote is shown as text, never as HTML.

const SERIES = 6

/** Width of an element, kept up to date. Falls back to a fixed width where ResizeObserver is missing (tests). */
function useWidth<T extends HTMLElement>(fallback = 480) {
  const ref = useRef<T>(null)
  const [width, setWidth] = useState(fallback)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w && w > 0) setWidth(Math.round(w))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width] as const
}

// ---------- sparkline ----------

export function Sparkline({ points, label }: { points: Point[]; label: string }) {
  const [ref, width] = useWidth<HTMLDivElement>(160)
  const h = 32
  if (points.length < 2) return null
  const xs = points.map((p) => new Date(p.t).getTime())
  const ys = points.map((p) => p.v)
  const x0 = Math.min(...xs)
  const x1 = Math.max(...xs)
  const y0 = Math.min(...ys)
  const y1 = Math.max(...ys)
  const sx = (x: number) => (x1 === x0 ? width / 2 : 4 + ((x - x0) / (x1 - x0)) * (width - 8))
  const sy = (y: number) => (y1 === y0 ? h / 2 : h - 5 - ((y - y0) / (y1 - y0)) * (h - 10))
  const d = points.map((p, i) => `${i ? 'L' : 'M'}${sx(xs[i] ?? 0).toFixed(1)} ${sy(p.v).toFixed(1)}`).join(' ')
  const last = points[points.length - 1] as Point
  return (
    <div className="spark" ref={ref}>
      <svg width={width} height={h} role="img" aria-label={label}>
        <path className="spark-line" d={d} />
        <circle className="spark-dot" cx={sx(xs[xs.length - 1] ?? 0)} cy={sy(last.v)} r={3.5} />
      </svg>
    </div>
  )
}

// ---------- line chart ----------

interface LineSeries {
  label: string
  points: { x: number; y: number; label?: string }[]
}

/** A line chart with a crosshair: hover or use the arrow keys to read every line at one time. */
export function LineChart({ series, tile, categories, title }: { series: LineSeries[]; tile: Tile; categories?: string[]; title: string }) {
  const [ref, width] = useWidth<HTMLDivElement>()
  const [hover, setHover] = useState<number | null>(null)
  const tipId = useId()
  const h = 190
  const pad = { top: 12, right: 18, bottom: 26, left: 52 }
  const all = series.flatMap((s) => s.points)
  const xs = all.map((p) => p.x)
  const ys = all.map((p) => p.y)
  const x0 = Math.min(...xs)
  const x1 = Math.max(...xs)
  const ticks = niceTicks(Math.min(...ys), Math.max(...ys), 4)
  const y0 = ticks[0] ?? 0
  const y1 = ticks[ticks.length - 1] ?? 1
  const iw = Math.max(40, width - pad.left - pad.right)
  const ih = h - pad.top - pad.bottom
  const sx = (x: number) => pad.left + (x1 === x0 ? iw / 2 : ((x - x0) / (x1 - x0)) * iw)
  const sy = (y: number) => pad.top + (y1 === y0 ? ih / 2 : ih - ((y - y0) / (y1 - y0)) * ih)
  const span = x1 - x0
  const xLabel = (x: number, long = false) => {
    if (categories) return categories[Math.round(x)] ?? ''
    return timeLabel(new Date(x).toISOString(), span, long)
  }
  // the x positions a crosshair can stop at: every point of every line
  const stops = useMemo(() => Array.from(new Set(xs)).sort((a, b) => a - b), [xs.join(',')]) // eslint-disable-line react-hooks/exhaustive-deps
  // As many labels as fit without touching: always the first and the last, evenly between.
  const xTicks = useMemo(() => {
    if (stops.length <= 1) return stops
    const longest = Math.max(...stops.map((x) => xLabel(x).length))
    const labelW = longest * 6.6 + 18
    const n = Math.min(stops.length, Math.max(2, Math.floor(iw / labelW) + 1))
    const picked = Array.from({ length: n }, (_, i) => stops[Math.round((i * (stops.length - 1)) / (n - 1))] as number)
    const last = picked[picked.length - 1] as number
    const kept: number[] = []
    for (const x of picked) {
      const prev = kept[kept.length - 1]
      if (prev !== undefined && sx(x) - sx(prev) < labelW) continue
      if (x !== last && sx(last) - sx(x) < labelW) continue
      kept.push(x)
    }
    return kept
  }, [stops, iw]) // eslint-disable-line react-hooks/exhaustive-deps

  const at = hover === null ? null : (stops[hover] ?? null)
  const nearest = (s: LineSeries, x: number) =>
    s.points.reduce<LineSeries['points'][number] | null>((best, p) => (!best || Math.abs(p.x - x) < Math.abs(best.x - x) ? p : best), null)

  const onMove = (e: PointerEvent<SVGRectElement>) => {
    const box = (e.currentTarget.ownerSVGElement as SVGSVGElement).getBoundingClientRect()
    const px = e.clientX - box.left
    let best = 0
    stops.forEach((x, i) => {
      if (Math.abs(sx(x) - px) < Math.abs(sx(stops[best] ?? 0) - px)) best = i
    })
    setHover(best)
  }
  const onKey = (e: KeyboardEvent) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight' && e.key !== 'Home' && e.key !== 'End') return
    e.preventDefault()
    const last = stops.length - 1
    setHover((cur) => {
      if (e.key === 'Home') return 0
      if (e.key === 'End') return last
      if (cur === null) return last
      return Math.max(0, Math.min(last, cur + (e.key === 'ArrowLeft' ? -1 : 1)))
    })
  }
  const single = series.length === 1
  const lastOf = (s: LineSeries) => s.points[s.points.length - 1]
  const tipLeft = at === null ? 0 : Math.min(Math.max(sx(at) + 12, pad.left), width - 170)

  return (
    <div className="chart" ref={ref}>
      {!single && (
        <ul className="legend" aria-label="Lines">
          {series.map((s, i) => (
            <li key={s.label}>
              <span className={`key line s${(i % SERIES) + 1}`} aria-hidden="true" />
              {s.label}
            </li>
          ))}
        </ul>
      )}
      <svg
        width={width}
        height={h}
        role="img"
        tabIndex={0}
        aria-label={`${title}: line chart. Use the left and right arrow keys to read the values.`}
        aria-describedby={at !== null ? tipId : undefined}
        onKeyDown={onKey}
        onBlur={() => setHover(null)}
      >
        {ticks.map((t) => (
          <g key={t}>
            <line className="grid" x1={pad.left} x2={pad.left + iw} y1={sy(t)} y2={sy(t)} />
            <text className="tick y" x={pad.left - 8} y={sy(t)} dy="0.32em">
              {tickLabel(t, y1, tile)}
            </text>
          </g>
        ))}
        {xTicks.map((x, i) => (
          <text key={i} className="tick x" x={sx(x)} y={h - 6} textAnchor={i === 0 && xTicks.length > 1 ? 'start' : i === xTicks.length - 1 && xTicks.length > 1 ? 'end' : 'middle'}>
            {xLabel(x)}
          </text>
        ))}
        {series.map((s, i) => {
          const d = s.points.map((p, j) => `${j ? 'L' : 'M'}${sx(p.x).toFixed(1)} ${sy(p.y).toFixed(1)}`).join(' ')
          const first = s.points[0]
          const last = lastOf(s)
          const cls = `s${(i % SERIES) + 1}`
          return (
            <g key={s.label}>
              {single && first && last && s.points.length > 1 && (
                <path className={`area ${cls}`} d={`${d} L${sx(last.x).toFixed(1)} ${pad.top + ih} L${sx(first.x).toFixed(1)} ${pad.top + ih} Z`} />
              )}
              <path className={`line ${cls}`} d={d} />
              {last && <circle className={`dot ${cls}`} cx={sx(last.x)} cy={sy(last.y)} r={4} />}
            </g>
          )
        })}
        {single && series[0] && lastOf(series[0]) && at === null && (
          <text className="end-label" x={Math.min(sx(lastOf(series[0])!.x), pad.left + iw) - 6} y={sy(lastOf(series[0])!.y) - 10} textAnchor="end">
            {formatValue(lastOf(series[0])!.y, tile)}
          </text>
        )}
        {at !== null && (
          <g>
            <line className="crosshair" x1={sx(at)} x2={sx(at)} y1={pad.top} y2={pad.top + ih} />
            {series.map((s, i) => {
              const p = nearest(s, at)
              return p ? <circle key={s.label} className={`dot s${(i % SERIES) + 1}`} cx={sx(p.x)} cy={sy(p.y)} r={4.5} /> : null
            })}
          </g>
        )}
        <rect className="hit" x={pad.left - 8} y={0} width={iw + 16} height={h} onPointerMove={onMove} onPointerLeave={() => setHover(null)} />
      </svg>
      {at !== null && (
        <div className="tip" id={tipId} role="status" style={{ left: tipLeft, top: pad.top }}>
          <div className="tip-when">{xLabel(at, true)}</div>
          {series.map((s, i) => {
            const p = nearest(s, at)
            if (!p) return null
            return (
              <div key={s.label} className="tip-row">
                <span className={`key line s${(i % SERIES) + 1}`} aria-hidden="true" />
                <b>{formatValue(p.y, tile)}</b>
                <span>{s.label}</span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// ---------- bars ----------

/** Horizontal bars, value at the tip. */
export function BarsChart({ bars, tile }: { bars: Bar[]; tile: Tile }) {
  const max = Math.max(0, ...bars.map((b) => b.value))
  const min = Math.min(0, ...bars.map((b) => b.value))
  const span = max - min || 1
  return (
    <ul className="bars">
      {bars.map((b, i) => {
        const left = ((0 - min) / span) * 100
        const w = (Math.abs(b.value) / span) * 100
        const start = b.value < 0 ? left - w : left
        return (
          <li key={`${b.label}-${i}`} title={`${b.label}: ${formatValue(b.value, tile)}`}>
            <span className="bar-label">{b.label}</span>
            <span className="bar-track">
              <span className={`bar-fill ${b.value < 0 ? 'neg' : ''}`} style={{ left: `${start}%`, width: `max(${w}%, 2px)` }} />
            </span>
            <span className="bar-value">{formatValue(b.value, tile)}</span>
          </li>
        )
      })}
    </ul>
  )
}

// ---------- table ----------

function cellText(v: unknown, col?: TileColumn): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'number') return formatValue(v, { format: col?.format })
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}

export function DataTable({ rows, columns, more, caption }: { rows: Record<string, unknown>[]; columns: TileColumn[]; more?: number; caption: string }) {
  return (
    <div className="dtable-wrap">
      <table className="dtable">
        <caption className="sr-only">{caption}</caption>
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={c.field} scope="col">
                {c.label || c.field}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              {columns.map((c) => {
                const v = r[c.field]
                return (
                  <td key={c.field} className={typeof v === 'number' ? 'num' : ''}>
                    {cellText(v, c)}
                  </td>
                )
              })}
            </tr>
          ))}
        </tbody>
      </table>
      {more ? <div className="dtable-more">{more === 1 ? '1 more row' : `${more} more rows`} not shown</div> : null}
    </div>
  )
}

// ---------- tiles ----------

const STATE_TEXT: Record<string, { icon: 'hand' | 'clock' | 'lock' | 'x' | 'question'; title: string }> = {
  pending: { icon: 'clock', title: 'Not fetched yet' },
  waiting: { icon: 'hand', title: 'Waiting for your OK' },
  blocked: { icon: 'lock', title: "Can't run from a dashboard" },
  declined: { icon: 'x', title: "You didn't allow this command" },
  error: { icon: 'question', title: "Couldn't show this" },
}

function NumberBody({ tile, value }: { tile: Tile; value: TileValue }) {
  const n = value.number as number
  const d = value.compare ? delta(n, value.compare.value, tile) : null
  return (
    <>
      <div className="big">
        <span className="big-num">{formatValue(n, tile)}</span>
        {tile.unit && <span className="big-unit">{tile.unit}</span>}
      </div>
      {d && value.compare && (
        <div className={`delta ${d.tone}`}>
          {d.direction !== 'flat' && <span aria-hidden="true">{d.direction === 'up' ? '▲' : '▼'}</span>}
          <b>{d.text}</b>
          {d.pct && <span>({d.pct})</span>}
          <span className="delta-when">{value.compare.label}</span>
        </div>
      )}
      {value.spark && value.spark.length > 1 && <Sparkline points={value.spark} label={`${tile.title} over time`} />}
    </>
  )
}

const STATUS = {
  ok: { icon: 'check', label: 'OK' },
  warn: { icon: 'question', label: 'Warning' },
  fail: { icon: 'x', label: 'Failing' },
} as const

function StatusBody({ value }: { value: TileValue }) {
  const s = STATUS[value.status ?? 'warn']
  const text = value.text && value.text !== s.label && value.text !== 'OK' && value.text !== 'Failing' ? value.text : ''
  return (
    <div className={`status-big ${value.status ?? 'warn'}`}>
      <span className="status-mark" aria-hidden="true">
        <Icon name={s.icon} />
      </span>
      <span className="status-label">{s.label}</span>
      {text && <span className="status-value">{text}</span>}
    </div>
  )
}

function chartSeries(tile: Tile, value: TileValue): { series: LineSeries[]; categories?: string[] } | null {
  const vs = value.series ?? []
  if (vs.some((s) => s.bars && s.bars.length > 0)) {
    const bars = vs[0]?.bars ?? []
    return { categories: bars.map((b) => b.label), series: [{ label: tile.title, points: bars.map((b, i) => ({ x: i, y: b.value })) }] }
  }
  const series = vs
    .filter((s) => (s.points?.length ?? 0) > 0)
    .map((s) => ({ label: s.label, points: (s.points ?? []).map((p) => ({ x: new Date(p.t).getTime(), y: p.v })) }))
  return series.length > 0 ? { series } : null
}

/** A chart's numbers as a table: the view that needs no colour or hovering. */
function chartRows(tile: Tile, value: TileValue, span: number): { rows: Record<string, unknown>[]; columns: TileColumn[] } {
  if (tile.type === 'bars') {
    return {
      rows: (value.bars ?? []).map((b) => ({ label: b.label, value: b.value })),
      columns: [{ field: 'label', label: tile.labelField || 'Label' }, { field: 'value', label: tile.valueField || 'Value', format: tile.format }],
    }
  }
  const cs = chartSeries(tile, value)
  if (!cs) return { rows: [], columns: [] }
  if (cs.categories) {
    return {
      rows: cs.series[0]?.points.map((p) => ({ x: cs.categories?.[p.x] ?? '', y: p.y })) ?? [],
      columns: [{ field: 'x', label: tile.labelField || 'Label' }, { field: 'y', label: tile.valueField || tile.title, format: tile.format }],
    }
  }
  const byTime = new Map<number, Record<string, unknown>>()
  cs.series.forEach((s, i) =>
    s.points.forEach((p) => {
      const row = byTime.get(p.x) ?? { when: timeLabel(new Date(p.x).toISOString(), span, true) }
      row[`s${i}`] = p.y
      byTime.set(p.x, row)
    }),
  )
  const rows = Array.from(byTime.entries())
    .sort((a, b) => b[0] - a[0])
    .map(([, r]) => r)
  return { rows, columns: [{ field: 'when', label: 'When' }, ...cs.series.map((s, i) => ({ field: `s${i}`, label: s.label, format: tile.format }))] }
}

export interface TileViewProps {
  tile: Tile
  value: TileValue | undefined
  /** The chart time range in milliseconds, for time labels. */
  span: number
  onReview?: () => void
}

export function TileView({ tile, value, span, onReview }: TileViewProps) {
  const [asTable, setAsTable] = useState(false)
  const chart = tile.type === 'line' || tile.type === 'bars'
  const v = value ?? { key: tile.key, state: 'pending' as const }
  const ok = v.state === 'ok'
  const cs = tile.type === 'line' && ok ? chartSeries(tile, v) : null
  const fewPoints = tile.type === 'line' && ok && (!cs || cs.series.every((s) => s.points.length < 2))

  let body
  if (!ok) {
    const st = STATE_TEXT[v.state] ?? STATE_TEXT.error!
    body = (
      <div className={`tile-state ${v.state}`}>
        <span className="tile-state-h">
          <Icon name={st.icon} small />
          {st.title}
        </span>
        {v.message && <p>{v.message}</p>}
        {v.state === 'waiting' && onReview && (
          <button type="button" className="btn link inline" onClick={onReview}>
            Review the command
          </button>
        )}
      </div>
    )
  } else if (chart && asTable) {
    const t = chartRows(tile, v, span)
    body = <DataTable rows={t.rows} columns={t.columns} caption={tile.title} />
  } else {
    switch (tile.type) {
      case 'number':
        body = v.number !== undefined ? <NumberBody tile={tile} value={v} /> : null
        break
      case 'status':
        body = <StatusBody value={v} />
        break
      case 'bars':
        body = (v.bars?.length ?? 0) > 0 ? <BarsChart bars={v.bars ?? []} tile={tile} /> : <p className="tile-empty">Nothing to show yet.</p>
        break
      case 'table':
        body =
          (v.rows?.length ?? 0) > 0 ? (
            <DataTable rows={v.rows ?? []} columns={v.columns ?? []} more={v.more} caption={tile.title} />
          ) : (
            <p className="tile-empty">No rows.</p>
          )
        break
      case 'text':
        body = <pre className="tile-text">{v.text || '(empty)'}</pre>
        break
      case 'line':
        body = fewPoints ? (
          <div className="tile-empty">
            {v.number !== undefined && <div className="big small">{formatValue(v.number, tile)}</div>}
            <p>The line fills in as CloudBuddy refreshes this dashboard.</p>
          </div>
        ) : cs ? (
          <LineChart series={cs.series} categories={cs.categories} tile={tile} title={tile.title} />
        ) : null
        break
    }
  }
  if (tile.type === 'bars' && ok && v.more) {
    body = (
      <>
        {body}
        <div className="dtable-more">{v.more} more not shown</div>
      </>
    )
  }

  return (
    <article className={`tile w${Math.min(4, Math.max(1, tile.width || 1))} t-${tile.type} st-${v.state} ${v.stale ? 'stale' : ''}`} aria-label={tile.title}>
      <header className="tile-head">
        <h3>{tile.title}</h3>
        {chart && ok && !fewPoints && (
          <button type="button" className="tile-toggle" aria-pressed={asTable} onClick={() => setAsTable((x) => !x)} title={asTable ? 'Show the chart' : 'Show as a table'}>
            {asTable ? 'Chart' : 'Table'}
          </button>
        )}
      </header>
      <div className="tile-body">{body}</div>
      {(tile.note || (ok && v.message) || (ok && v.updatedAt)) && (
        <footer className="tile-foot">
          {ok && v.stale && v.message && (
            <span className="tile-warn" title={v.message}>
              <Icon name="question" small />
              Last refresh failed
            </span>
          )}
          {ok && !v.stale && v.message && (
            <span className="tile-warn">
              <Icon name="question" small />
              {v.message}
            </span>
          )}
          {tile.note && <span>{tile.note}</span>}
          {ok && v.updatedAt && !v.stale && <span className="tile-when">{relativeWhen(v.updatedAt)}</span>}
        </footer>
      )}
    </article>
  )
}
