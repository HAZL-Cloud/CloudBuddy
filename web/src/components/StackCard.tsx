import { useState } from 'react'
import { clock } from '../format'
import { STACK_ROWS, rangeLabel } from '../layout'
import type { Step } from '../types'
import { ActionChip } from './StepParts'

export interface StackCardProps {
  /** Back-to-back reads in one lane, in order. */
  steps: Step[]
  indices: number[]
  numbers: Record<string, number>
  selectedId: string | null
  viewIndex: number | null
  traced: boolean
  enter: boolean
  onSelect: (id: string, viaKeyboard?: boolean) => void
}

/** Several reads in a row, as one card. Each row opens its step's details. */
export function StackCard({ steps, indices, numbers, selectedId, viewIndex, traced, enter, onSelect }: StackCardProps) {
  const [open, setOpen] = useState(false)
  const shown = open ? steps : steps.slice(0, STACK_ROWS)
  const label = rangeLabel(steps.map((s) => numbers[s.id] ?? 0))
  const future = (i: number) => viewIndex !== null && (indices[i] ?? 0) > viewIndex
  const cls = [
    'card',
    'stack',
    'risk-read',
    steps.some((s) => s.id === selectedId) ? 'selected' : '',
    viewIndex !== null && indices.includes(viewIndex) ? 'current' : '',
    future(0) ? 'future' : '',
    enter ? 'enter' : '',
    traced ? 'traced' : '',
  ]
    .filter(Boolean)
    .join(' ')
  const first = steps[0]

  return (
    <article className={cls} aria-label={`Steps ${label}: ${steps.length} reads (${first?.provider ?? ''})`}>
      <div className="card-top">
        <ActionChip action="read" />
        <span>Steps {label}</span>
        {first && <span className="time mono">{clock(first.startedAt)}</span>}
      </div>
      <div className="card-title">{steps.length} reads</div>
      <ol className="stack-rows">
        {shown.map((s, i) => {
          const n = numbers[s.id] ?? 0
          const rowCls = ['stack-row', s.id === selectedId || viewIndex === indices[i] ? 'on' : '', future(i) ? 'future' : '']
            .filter(Boolean)
            .join(' ')
          return (
            <li key={s.id}>
              <button
                type="button"
                className={rowCls}
                data-step-id={s.id}
                title={s.title}
                aria-label={`Step ${n}: ${s.title}`}
                onClick={(e) => onSelect(s.id, e.detail === 0)}
              >
                <span className="n mono">{n}</span>
                <span className="t">{s.title}</span>
                {s.status === 'running' && <span className="spin" aria-label="Running" />}
              </button>
            </li>
          )
        })}
      </ol>
      {steps.length > STACK_ROWS && (
        <button type="button" className="btn link stack-more" aria-expanded={open} onClick={() => setOpen(!open)}>
          {open ? 'Show fewer' : `Show ${steps.length - STACK_ROWS} more`}
        </button>
      )}
    </article>
  )
}
