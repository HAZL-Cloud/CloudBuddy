import type { KeyboardEvent, MouseEvent } from 'react'
import { clock } from '../format'
import type { Decision, Step } from '../types'
import { Icon } from './Icon'
import {
  ActionChip,
  AdapterNote,
  DecisionButtons,
  FactList,
  StatusLine,
  Stamp,
  TagList,
  UsesLines,
  Warning,
  type Deciding,
} from './StepParts'

export interface StepCardProps {
  step: Step
  /** The step's number on the canvas. */
  number: number
  numbers: Record<string, number>
  selected: boolean
  current: boolean
  future: boolean
  enter: boolean
  deciding: Deciding | null
  onSelect: (id: string, viaKeyboard?: boolean) => void
  onDecide: (step: Step, decision: Decision) => void
}

export function StepCard({ step, number, numbers, selected, current, future, enter, deciding, onSelect, onDecide }: StepCardProps) {
  const approval = step.approval
  const asking = step.status === 'waiting' && approval?.state === 'pending'
  const cls = [
    'card',
    `risk-${step.action}`,
    `st-${step.status}`,
    approval ? 'gate' : '',
    selected ? 'selected' : '',
    current ? 'current' : '',
    future ? 'future' : '',
    enter ? 'enter' : '',
  ]
    .filter(Boolean)
    .join(' ')

  // Clicks on the buttons inside the card are theirs; anything else opens the details.
  const onClick = (e: MouseEvent) => {
    if (!(e.target as Element).closest('button')) onSelect(step.id)
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if ((e.key === 'Enter' || e.key === ' ') && e.target === e.currentTarget) {
      e.preventDefault()
      onSelect(step.id, true)
    }
  }

  return (
    <article
      className={cls}
      tabIndex={0}
      aria-label={`Step ${number}: ${step.title} (${step.provider})`}
      aria-current={current ? 'step' : undefined}
      data-step-id={step.id}
      onClick={onClick}
      onKeyDown={onKeyDown}
    >
      {asking && (
        <div className="gate-banner">
          <Icon name="hand" small />
          Waiting for your approval
        </div>
      )}
      <div className="card-top">
        <ActionChip action={step.action} />
        <span>Step {number}</span>
        <span className="time mono">{clock(step.startedAt)}</span>
      </div>
      <div className="card-title">{step.title}</div>
      {step.resource && <div className="card-res">{step.resource}</div>}
      <TagList tags={step.tags} />
      <AdapterNote step={step} />
      <UsesLines step={step} numbers={numbers} />
      <FactList facts={step.facts} />
      {asking && approval ? (
        <>
          {approval.loss && approval.loss.length > 0 && (
            <div className="loss">
              <div className="loss-h">{approval.lossTitle ?? 'What will be lost'}</div>
              <ul>
                {approval.loss.map((l, i) => (
                  <li key={i}>{l}</li>
                ))}
              </ul>
            </div>
          )}
          <Warning text={approval.warning} />
          <div className="why">
            <span>Agent&apos;s reason</span>
            {step.why}
          </div>
          <div className="gate-actions">
            <DecisionButtons step={step} deciding={deciding} onDecide={onDecide} />
          </div>
        </>
      ) : (
        <Stamp step={step} />
      )}
      {step.status === 'running' && <div className="bar" />}
      <div className="card-foot">
        <StatusLine step={step} />
        <span className="api mono" title={step.api}>
          {step.api}
        </span>
      </div>
    </article>
  )
}
