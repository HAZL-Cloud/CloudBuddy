import { useReactFlow, useViewport } from '@xyflow/react'
import { prefersReducedMotion } from '../hooks'
import { clamp } from '../camera'
import { CARD_W, type Column, type Lane } from '../layout'
import { countActions } from '../tally'
import type { Step } from '../types'
import { ACTIONS, TALLY_ORDER } from '../vocab'
import { Icon } from './Icon'

export function ZoomControls({ onFit }: { onFit: () => void }) {
  const rf = useReactFlow()
  const { zoom } = useViewport()
  const duration = prefersReducedMotion() ? 0 : 150
  const percent = Math.round(zoom * 100)
  return (
    <div className="zoom">
      <button type="button" aria-label="Zoom out" onClick={() => void rf.zoomOut({ duration })}>
        <Icon name="minus" />
      </button>
      <span className="zl" aria-label={`Zoom ${percent} percent`}>
        {percent}%
      </span>
      <button type="button" aria-label="Zoom in" onClick={() => void rf.zoomIn({ duration })}>
        <Icon name="plus" />
      </button>
      <button type="button" aria-label="Fit all steps" title="Fit all steps" onClick={onFit}>
        <Icon name="fit" />
      </button>
    </div>
  )
}

const RULER_H = 30

/** Step numbers pinned to the top edge, one per column. Left to right is the order. */
export function ColumnRuler({ columns, viewIndex }: { columns: Column[]; viewIndex: number | null }) {
  const { x, zoom } = useViewport()
  const first = columns[0]
  const last = columns[columns.length - 1]
  if (!first || !last) return null
  const left = x + first.x * zoom
  const right = x + (last.x + CARD_W) * zoom
  return (
    <div className="ruler" aria-hidden="true">
      <div className="ruler-line" style={{ transform: `translateX(${left}px)`, width: Math.max(0, right - left) }} />
      {columns.map((c) => (
        <span
          key={c.x}
          className={viewIndex !== null && c.index > viewIndex ? 'ruler-n fut' : 'ruler-n'}
          style={{ transform: `translateX(${x + (c.x + CARD_W / 2) * zoom}px) translateX(-50%)` }}
        >
          {c.label}
        </span>
      ))}
      <span className="ruler-h">Step</span>
    </div>
  )
}

/** Lane names pinned to the left edge. They follow vertical panning and zoom, but not horizontal panning. */
export function LaneRail({ lanes }: { lanes: Lane[] }) {
  const { y, zoom } = useViewport()
  return (
    <div className="rail" aria-hidden="true">
      {lanes.map((lane) => {
        const top = y + lane.top * zoom
        const bottom = y + (lane.top + lane.height) * zoom
        const hidden = bottom - top < 44 || bottom < 40
        return (
          <div
            key={lane.id}
            className="rail-l"
            style={{ transform: `translateY(${clamp(top + 10, RULER_H + 6, bottom - 40)}px)`, opacity: hidden ? 0 : 1 }}
          >
            <b>{lane.name}</b>
            <span>{lane.sub}</span>
          </div>
        )
      })}
    </div>
  )
}

export function TallyPills({ steps }: { steps: Step[] }) {
  const n = countActions(steps)
  return (
    <div className="tally">
      {TALLY_ORDER.filter((a) => n[a] > 0).map((a) => {
        const waiting = steps.filter((s) => s.status === 'waiting' && s.action === a).length
        const label = `${n[a]} ${ACTIONS[a].plural}${waiting ? `, ${waiting} waiting for you` : ''}`
        return (
          <span key={a} className={`pill risk-${a}${waiting ? ' waiting' : ''}`} title={label} aria-label={label}>
            <Icon name={ACTIONS[a].icon} />
            {n[a]}
            {waiting > 0 && ` · ${waiting} waiting`}
          </span>
        )
      })}
    </div>
  )
}
