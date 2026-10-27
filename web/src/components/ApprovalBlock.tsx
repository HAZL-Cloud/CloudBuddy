import { clock } from '../format'
import type { Decision, Step } from '../types'
import { Icon } from './Icon'
import { DecisionButtons, Warning, type Deciding } from './StepParts'

export interface ApprovalBlockProps {
  step: Step
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  /** Only cloud steps are on the canvas. */
  onShowOnCanvas?: (stepId: string) => void
}

/** The red card in the chat when a step needs your OK, and its quiet version once decided. */
export function ApprovalBlock({ step, deciding, onDecide, onShowOnCanvas }: ApprovalBlockProps) {
  const approval = step.approval
  if (!approval) return null
  const pending = approval.state === 'pending' && step.status === 'waiting'
  const after = step.action === 'delete' ? ' Nothing was deleted.' : step.action === 'send' ? ' Nothing was sent.' : ''
  return (
    <div className={`approval ${pending ? '' : 'done'}`}>
      <div className="ap-head">
        <Icon name={pending ? 'hand' : 'shield'} small />
        {pending ? 'Needs your approval' : 'Approval'}
      </div>
      <div className="ap-body">
        <div className="ap-title">{step.title}</div>
        <div className="ap-why">{approval.reason}</div>
        <Warning text={approval.warning} />
        <div className="ap-cmd">{step.command}</div>
        {pending && (
          <div className="ap-actions">
            {step.kind === 'cloud' && onShowOnCanvas && (
              <button type="button" className="btn link" onClick={() => onShowOnCanvas(step.id)}>
                Review on canvas
              </button>
            )}
            <DecisionButtons step={step} deciding={deciding} onDecide={onDecide} />
          </div>
        )}
        {approval.state === 'approved' && !pending && (
          <div className="ap-result ok">
            <Icon name="check" small />
            You approved this at {clock(approval.decidedAt)}
          </div>
        )}
        {approval.state === 'denied' && (
          <div className="ap-result no">
            <Icon name="x" small />
            You said no at {clock(approval.decidedAt)}.{after}
          </div>
        )}
        {approval.state === 'expired' && (
          <div className="ap-result no">
            <Icon name="clock" small />
            This approval expired. Nothing ran.
          </div>
        )}
      </div>
    </div>
  )
}
