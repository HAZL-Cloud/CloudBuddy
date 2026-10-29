import { useEffect, useRef, type ReactNode } from 'react'
import { api } from '../api'
import { clock } from '../format'
import type { Decision, Step } from '../types'
import { ACTIONS, VIA_LABELS } from '../vocab'
import { Icon } from './Icon'
import {
  ActionChip,
  DecisionButtons,
  FactList,
  StatusLine,
  TagList,
  Warning,
  usesText,
  type Deciding,
} from './StepParts'
import { useToast } from './Toast'

function Section({ title, extra, className = '', children }: { title: string; extra?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <section className={`ins-sec ${className}`}>
      <h4>
        <span>{title}</span>
        {extra}
      </h4>
      {children}
    </section>
  )
}

function genericExplanation(step: Step): string {
  const kind =
    step.action === 'unknown' ? "something it can't be sure of" : `a ${ACTIONS[step.action].label.toLowerCase()}`
  const guess = step.guessWord ? `: the word “${step.guessWord}” means this is ${kind}.` : '.'
  const title = step.titleSource === 'adapter' ? '' : " The title is the agent's own description."
  return `There's no ${step.provider} adapter yet, so CloudBuddy guessed from the command${guess}${title} Steps like this run on their own unless they look like a delete, a payment or a message to people.`
}

const capitalise = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

function outputBody(step: Step): ReactNode {
  if (step.output) {
    return (
      <>
        <pre className="code">{step.output}</pre>
        {step.maskedNote && <p className="note">{step.maskedNote}</p>}
        {step.outputTruncated && <p className="note">The output was cut short to keep it a reasonable size.</p>}
      </>
    )
  }
  switch (step.status) {
    case 'waiting':
      return <p>Nothing yet. This step has not run.</p>
    case 'denied':
      return <p>Not run. You said no.</p>
    case 'running':
      return <p>Still running.</p>
    default:
      return <p>No output.</p>
  }
}

export interface InspectorProps {
  step: Step
  number: number
  numbers: Record<string, number>
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  onClose: () => void
  /** Move focus into the panel when it opens from the keyboard. */
  focusOnOpen?: boolean
}

export function Inspector({ step, number, numbers, deciding, onDecide, onClose, focusOnOpen }: InspectorProps) {
  const toast = useToast()
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    if (focusOnOpen) ref.current?.focus()
  }, [focusOnOpen, step.id])

  const approval = step.approval
  const asking = step.status === 'waiting' && approval?.state === 'pending'
  const mcp = step.via === 'mcp'

  const stop = () => {
    api.stopStep(step.id).then(
      () => toast('Stopping…'),
      () => toast('It already finished.'),
    )
  }

  const copy = () => {
    const done = () => toast('Copied.')
    const fallback = () => toast('Could not copy. Select the text and press Ctrl+C.')
    if (!navigator.clipboard) return fallback()
    navigator.clipboard.writeText(step.command).then(done, fallback)
  }

  return (
    <aside className="inspector" aria-label="Step details" tabIndex={-1} ref={ref}>
      <div className={`ins-head risk-${step.action}`}>
        <ActionChip action={step.action} />
        <span>
          Step {number} · {clock(step.startedAt)}
        </span>
        <button type="button" className="icon-btn" aria-label="Close details" onClick={onClose}>
          <Icon name="x" />
        </button>
      </div>
      <h3 className="ins-title">{step.title}</h3>
      <div className="ins-where">
        {step.provider} · {step.product} · via {VIA_LABELS[step.via]}
      </div>
      <div>
        <StatusLine step={step} />
      </div>
      <TagList tags={step.tags} />
      {asking && (
        <div className="gate-actions">
          <DecisionButtons step={step} deciding={deciding} onDecide={onDecide} />
        </div>
      )}
      {step.status === 'running' && step.via !== 'mcp' && step.via !== 'file' && (
        <div className="gate-actions">
          <button type="button" className="btn" onClick={stop}>
            Stop this command
          </button>
        </div>
      )}
      {approval && (
        <Section title="Why CloudBuddy asked" className="asked">
          <p>{approval.reason}</p>
          <Warning text={approval.warning} />
        </Section>
      )}
      {step.generic && (
        <Section title="How CloudBuddy read this">
          <p>{genericExplanation(step)}</p>
        </Section>
      )}
      <Section title="Why the agent did this">
        <p>{step.why}</p>
      </Section>
      {step.uses.length > 0 && (
        <Section title="Uses">
          <ul>
            {step.uses.map((u, i) => (
              <li key={`${u.fromStepId}-${i}`}>{capitalise(usesText(u, numbers))}</li>
            ))}
          </ul>
        </Section>
      )}
      {approval?.loss && approval.loss.length > 0 && (
        <Section title={asking ? (approval.lossTitle ?? 'What will be lost') : 'What was at stake'}>
          <ul>
            {approval.loss.map((l, i) => (
              <li key={i}>{l}</li>
            ))}
          </ul>
        </Section>
      )}
      {approval && approval.state !== 'pending' && (
        <Section title="Your decision">
          <p>
            {approval.state === 'approved' && `Approved by you at ${clock(approval.decidedAt)}.`}
            {approval.state === 'denied' && `Denied by you at ${clock(approval.decidedAt)}.`}
            {approval.state === 'expired' && `The request expired at ${clock(approval.decidedAt ?? approval.requestedAt)}. Nothing ran.`}
          </p>
        </Section>
      )}
      {step.facts.length > 0 && (
        <Section title="Details">
          <FactList facts={step.facts} />
        </Section>
      )}
      <Section
        title={mcp ? 'Tool call' : 'Command'}
        extra={
          <button type="button" className="copy" onClick={copy}>
            <Icon name="copy" small />
            Copy
          </button>
        }
      >
        <pre className="code">{step.command}</pre>
      </Section>
      <Section title={mcp ? 'MCP tool' : 'API call'}>
        <p className="mono">{step.api}</p>
        <p className="note">
          {mcp ? `The tool the agent called on the ${step.provider} MCP server.` : 'The operation behind this command.'}
        </p>
      </Section>
      <Section title="Output">{outputBody(step)}</Section>
    </aside>
  )
}
