import { useLayoutEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { ApiError } from '../api'
import { useSendMessage, useStopConversation } from '../queries'
import type { Agent, ConversationDetail, Decision, Step } from '../types'
import { AGENT_LABELS } from '../vocab'
import { ChatItem } from './ChatItem'
import { Icon } from './Icon'
import type { Deciding } from './StepParts'
import { useToast } from './Toast'

export interface ChatPanelProps {
  detail: ConversationDetail | undefined
  loading: boolean
  failed: boolean
  agents: Agent[]
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  onShowOnCanvas: (stepId: string) => void
  onOpenNav: () => void
  onNewChat: () => void
  onRetry: () => void
}

export function ChatPanel(props: ChatPanelProps) {
  const { detail, loading, failed, onOpenNav, onNewChat, onRetry } = props
  return (
    <section className="chat" aria-label="Chat">
      <header className="chat-head">
        <button type="button" className="icon-btn menu-btn" aria-label="Open navigation" onClick={onOpenNav}>
          <Icon name="menu" />
        </button>
        <div className="chat-head-main">
          <h1>{detail ? detail.title : 'CloudBuddy'}</h1>
          {detail && <ChatMeta detail={detail} agents={props.agents} />}
        </div>
      </header>
      {detail ? (
        <Conversation key={detail.id} {...props} detail={detail} />
      ) : (
        <div className="empty">
          {failed ? (
            <>
              <b>Could not load this chat</b>
              <p>Check that CloudBuddy is still running in your terminal.</p>
              <button type="button" className="btn" onClick={onRetry}>
                Try again
              </button>
            </>
          ) : loading ? (
            <p>Loading…</p>
          ) : (
            <>
              <b>No chats yet</b>
              <p>Start one and ask your agent to do some cloud work. Every step it takes will show up here.</p>
              <button type="button" className="btn danger" onClick={onNewChat}>
                New chat
              </button>
            </>
          )}
        </div>
      )}
    </section>
  )
}

const agentName = (name: ConversationDetail['agent'], agents: Agent[]) =>
  agents.find((a) => a.name === name)?.label ?? AGENT_LABELS[name]

function ChatMeta({ detail, agents }: { detail: ConversationDetail; agents: Agent[] }) {
  return (
    <div className="chat-meta">
      <span className="meta-chip">
        <Icon name="bot" small />
        {agentName(detail.agent, agents)}
        {detail.model ? ` · ${detail.model}` : ''}
      </span>
      <span className="meta-chip mono">
        <Icon name="folder" small />
        {detail.workdir}
      </span>
    </div>
  )
}

function Conversation({ detail, agents, deciding, onDecide, onShowOnCanvas }: ChatPanelProps & { detail: ConversationDetail }) {
  const toast = useToast()
  const send = useSendMessage(detail.id)
  const stop = useStopConversation(detail.id)
  const [text, setText] = useState('')
  const listRef = useRef<HTMLDivElement>(null)
  const stick = useRef(true)

  // Follow new messages, unless you have scrolled up to read.
  useLayoutEffect(() => {
    const el = listRef.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [detail.chat, detail.status])

  const label = agentName(detail.agent, agents)
  const initials = detail.agent === 'codex' ? 'CX' : 'CC'
  const running = detail.status === 'running'

  const submit = (e?: FormEvent) => {
    e?.preventDefault()
    const message = text.trim()
    if (!message || send.isPending) return
    send.mutate(message, {
      onSuccess: () => {
        setText('')
        stick.current = true
      },
      onError: (err) => {
        toast(
          err instanceof ApiError && err.status === 409
            ? `${label} is still working. Wait for it to finish, or stop it.`
            : "Couldn't send that message. Try again.",
        )
      },
    })
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <>
      <div
        className="messages"
        ref={listRef}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80
        }}
      >
        {detail.chat.map((item, i) => (
          <ChatItem
            key={item.id}
            item={item}
            detail={detail}
            agentInitials={initials}
            agentLabel={label}
            startsAgentTurn={i === 0 || detail.chat[i - 1]?.type === 'user'}
            deciding={deciding}
            onDecide={onDecide}
            onShowOnCanvas={onShowOnCanvas}
          />
        ))}
        {running && (
          <div className="working" role="status">
            <span className="spin" />
            {label} is working…
            <button type="button" className="btn link" disabled={stop.isPending} onClick={() => stop.mutate()}>
              Stop
            </button>
          </div>
        )}
      </div>
      <form className="composer" onSubmit={submit}>
        <textarea
          rows={2}
          aria-label="Message"
          placeholder={`Reply to ${label}…`}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
        />
        <div className="composer-row">
          <span className="chip">
            <Icon name="bot" small />
            {label}
          </span>
          <span
            className="chip safe"
            title="Reads run on their own. Creates and changes show up as they happen. Anything you can't undo waits for you: deletes, moving money, reaching people, live mode."
          >
            <Icon name="shield" small />
            Asks before anything you can&apos;t undo
          </span>
          <button type="submit" className="send" aria-label="Send message" disabled={!text.trim() || send.isPending}>
            <Icon name="send" />
          </button>
        </div>
      </form>
    </>
  )
}
