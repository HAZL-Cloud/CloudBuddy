import { Fragment } from 'react'
import { clock, duration } from '../format'
import type { Action, Decision, Step } from '../types'
import { ACTIONS, TAGS } from '../vocab'
import { Icon } from './Icon'

export function ActionChip({ action }: { action: Action }) {
  const a = ACTIONS[action]
  return (
    <span className="risk">
      <Icon name={a.icon} />
      {a.label}
    </span>
  )
}

export function TagList({ tags }: { tags: Step['tags'] }) {
  if (tags.length === 0) return null
  return (
    <div className="tags">
      {tags.map((t) => (
        <span key={t} className={`tag tag-${t}`}>
          <Icon name={TAGS[t].icon} />
          {TAGS[t].label}
        </span>
      ))}
    </div>
  )
}

export function StatusLine({ step }: { step: Step }) {
  switch (step.status) {
    case 'done':
      return (
        <span className="status ok">
          <Icon name="check" small />
          {step.durationMs === undefined ? 'Done' : `Done · ${duration(step.durationMs)}`}
        </span>
      )
    case 'running':
      return (
        <span className="status run">
          <span className="spin" />
          Running…
        </span>
      )
    case 'waiting':
      return (
        <span className="status wait">
          <Icon name="clock" small />
          Waiting since {clock(step.approval?.requestedAt ?? step.startedAt)}
        </span>
      )
    case 'failed':
      return (
        <span className="status bad">
          <Icon name="x" small />
          Failed
        </span>
      )
    case 'outcome_unknown':
      return (
        <span className="status unk" title="CloudBuddy lost track of this step. Check the service to see what happened.">
          <Icon name="question" small />
          Outcome unknown
        </span>
      )
    case 'denied':
      return (
        <span className="status no">
          <Icon name="x" small />
          Not run
        </span>
      )
  }
}

/** The ink stamp on a decided approval, e.g. "Approved by you · 20:38:44". */
export function Stamp({ step }: { step: Step }) {
  const a = step.approval
  if (!a || a.state === 'pending') return null
  if (a.state === 'approved') {
    return (
      <span className="stamp ok">
        <Icon name="check" small />
        Approved by you · {clock(a.decidedAt)}
      </span>
    )
  }
  if (a.state === 'denied') {
    return (
      <span className="stamp no">
        <Icon name="x" small />
        You said no · {clock(a.decidedAt)}
      </span>
    )
  }
  return (
    <span className="stamp no">
      <Icon name="clock" small />
      Expired · {clock(a.decidedAt ?? a.requestedAt)}
    </span>
  )
}

export function FactList({ facts }: { facts: Step['facts'] }) {
  if (facts.length === 0) return null
  return (
    <dl className="facts">
      {facts.map(([k, v], i) => (
        <Fragment key={`${k}-${i}`}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </Fragment>
      ))}
    </dl>
  )
}

export function usesText(u: Step['uses'][number], numbers: Record<string, number>): string {
  const n = numbers[u.fromStepId]
  return n === undefined ? `the ${u.what} from a local step` : `the ${u.what} from step ${n}`
}

export function UsesLines({ step, numbers }: { step: Step; numbers: Record<string, number> }) {
  return (
    <>
      {step.uses.map((u, i) => (
        <div key={`${u.fromStepId}-${i}`} className="uses">
          <Icon name="link" small />
          <span>Uses {usesText(u, numbers)}</span>
        </div>
      ))}
    </>
  )
}

/** Said on the card when no adapter matched and CloudBuddy guessed from a word in the command. */
export function AdapterNote({ step }: { step: Step }) {
  if (!step.generic) return null
  return (
    <div className="generic">
      <Icon name="question" small />
      <span>
        No {step.provider} adapter.{' '}
        {step.guessWord ? `Guessed from the word “${step.guessWord}”.` : "Asks first unless it's clearly read-only."}
      </span>
    </div>
  )
}

export function Warning({ text }: { text?: string }) {
  if (!text) return null
  return (
    <div className="warn">
      <Icon name="question" small />
      <span>{text}</span>
    </div>
  )
}

export interface Deciding {
  approvalId: string
  decision: Decision
}

/** No / Approve. Both stay disabled while any decision is on its way to the server. */
export function DecisionButtons({
  step,
  deciding,
  onDecide,
}: {
  step: Step
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
}) {
  const mine = deciding?.approvalId === step.approval?.id ? deciding?.decision : undefined
  return (
    <>
      <button type="button" className="btn" disabled={deciding !== null} onClick={() => onDecide(step, 'deny')}>
        {mine === 'deny' ? 'Saying no…' : (step.approval?.noLabel ?? 'No')}
      </button>
      <button type="button" className="btn danger" disabled={deciding !== null} onClick={() => onDecide(step, 'approve')}>
        {mine === 'approve' ? 'Approving…' : (step.approval?.yesLabel ?? 'Approve')}
      </button>
    </>
  )
}
