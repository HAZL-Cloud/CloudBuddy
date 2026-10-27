import { clock } from '../format'
import type { ChatItem as Item, ConversationDetail, Decision, Step } from '../types'
import { ACTIONS } from '../vocab'
import { ApprovalBlock } from './ApprovalBlock'
import { Icon } from './Icon'
import { InlineCode } from './InlineCode'
import type { Deciding } from './StepParts'

function StepRow({ step, onShow }: { step: Step; onShow: (id: string) => void }) {
  const action = ACTIONS[step.action]
  const cls = `step-row risk-${step.action} st-${step.status}`
  const meta =
    step.status === 'running' ? (
      <span className="spin" />
    ) : step.status === 'denied' ? (
      <span className="rm">Not run</span>
    ) : (
      <span className="rm">{clock(step.startedAt)}</span>
    )
  const body = (
    <>
      <span className="rk">
        <Icon name={action.icon} />
      </span>
      <span className="rt">{step.title}</span>
      {meta}
    </>
  )
  // Local steps ran on this computer and are not on the canvas, so there is nothing to jump to.
  if (step.kind === 'local') {
    return (
      <div className={cls} title="Ran on this computer. Local steps are not on the canvas.">
        {body}
      </div>
    )
  }
  return (
    <button type="button" className={cls} onClick={() => onShow(step.id)} title="Show this step on the canvas">
      {body}
    </button>
  )
}

export interface ChatItemProps {
  item: Item
  detail: ConversationDetail
  agentInitials: string
  agentLabel: string
  /** True when the item before this one was the user's message, or this is the first item. */
  startsAgentTurn: boolean
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  onShowOnCanvas: (stepId: string) => void
}

export function ChatItem({ item, detail, agentInitials, agentLabel, startsAgentTurn, deciding, onDecide, onShowOnCanvas }: ChatItemProps) {
  switch (item.type) {
    case 'user':
      return (
        <div className="msg-user">
          <InlineCode text={item.text} />
        </div>
      )
    case 'agent':
      return (
        <div className="msg-agent">
          {startsAgentTurn && (
            <div className="agent-name">
              <span className="agent-av">{agentInitials}</span>
              {agentLabel}
            </div>
          )}
          <p>
            <InlineCode text={item.text} />
          </p>
        </div>
      )
    case 'steps': {
      const rows = item.stepIds
        .map((id) => detail.steps.find((s) => s.id === id))
        .filter((s): s is Step => s !== undefined)
      if (rows.length === 0) return null
      return (
        <div className="step-list">
          {rows.map((s) => (
            <StepRow key={s.id} step={s} onShow={onShowOnCanvas} />
          ))}
        </div>
      )
    }
    case 'approval': {
      const step = detail.steps.find((s) => s.id === item.stepId)
      return step ? <ApprovalBlock step={step} deciding={deciding} onDecide={onDecide} onShowOnCanvas={onShowOnCanvas} /> : null
    }
    case 'notice':
      return (
        <div className={`notice ${item.level}`} role={item.level === 'error' ? 'alert' : undefined}>
          <Icon name={item.level === 'info' ? 'shield' : 'question'} small />
          <span>{item.text}</span>
        </div>
      )
  }
}
