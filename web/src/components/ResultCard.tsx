import { clock } from '../format'
import { countActions } from '../tally'
import type { RunResult, Step } from '../types'
import { Icon } from './Icon'

export function ResultCard({
  result,
  steps,
  current,
  future,
  enter,
}: {
  result: RunResult
  steps: Step[]
  current: boolean
  future: boolean
  enter: boolean
}) {
  const n = countActions(steps)
  const cells: [string, number][] = [
    ['Created', n.create],
    ['Changed', n.change],
  ]
  if (n.send) cells.push(['Sent', n.send])
  if (n.delete || !n.send) cells.push(['Deleted', n.delete])
  cells.push(['Stopped by you', n.denied])
  const cls = ['card', 'result', 'risk-result', current ? 'current' : '', future ? 'future' : '', enter ? 'enter' : '']
    .filter(Boolean)
    .join(' ')
  return (
    <article className={cls} aria-label="Run finished" data-step-id="result">
      <div className="card-top">
        <span className="risk">
          <Icon name="check" />
          Finished
        </span>
        <span className="time mono">{clock(result.at)}</span>
      </div>
      <div className="card-title">{result.title}</div>
      <div className="card-res">{result.line}</div>
      <div className="tallies">
        {cells.map(([label, value]) => (
          <div key={label}>
            <b>{value}</b>
            <span>{label}</span>
          </div>
        ))}
      </div>
    </article>
  )
}
