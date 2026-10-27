import { clock } from '../format'
import type { Step } from '../types'
import { Icon } from './Icon'

export interface ScrubberProps {
  /** The steps on the canvas right now, in order. */
  steps: Step[]
  /** Canvas numbers for the label. */
  numbers: Record<string, number>
  total: number
  /** null means the latest step. */
  viewIndex: number | null
  playing: boolean
  onGoTo: (index: number) => void
  onPlay: () => void
  onLatest: () => void
}

export function Scrubber({ steps, numbers, total, viewIndex, playing, onGoTo, onPlay, onLatest }: ScrubberProps) {
  const n = steps.length
  const at = viewIndex ?? n - 1
  const current = steps[at]
  return (
    <footer className="scrubber">
      <button type="button" className="btn play" onClick={onPlay} disabled={n === 0} aria-label={playing ? 'Pause replay' : 'Replay all steps'}>
        <Icon name={playing ? 'pause' : 'play'} />
        <span>{playing ? 'Pause' : 'Replay'}</span>
      </button>
      <div className="ticks" role="group" aria-label="Timeline of steps">
        {steps.map((s, i) => {
          const cls = ['tick', `risk-${s.action}`, `st-${s.status}`, i <= at ? 'on' : '', viewIndex !== null && i === at ? 'cur' : '']
            .filter(Boolean)
            .join(' ')
          const num = numbers[s.id] ?? i + 1
          return (
            <button
              key={s.id}
              type="button"
              className={cls}
              title={`Step ${num} · ${s.title}`}
              aria-label={`Go to step ${num}: ${s.title}`}
              aria-current={viewIndex !== null && i === at ? 'step' : undefined}
              onClick={() => onGoTo(i)}
            />
          )
        })}
      </div>
      <div className="scrub-label">
        {current ? (
          <>
            Step {numbers[current.id] ?? at + 1} of {total}
            <span className="long"> · {clock(current.startedAt)}</span>
          </>
        ) : (
          'No steps yet'
        )}
      </div>
      {viewIndex !== null && (
        <button type="button" className="btn" onClick={onLatest}>
          Latest
        </button>
      )}
    </footer>
  )
}
