import { useEffect, useLayoutEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { ApiError } from '../api'
import { useCreateConversation, useSendMessage, useStopConversation } from '../queries'
import type { Agent, AgentName, ConversationDetail, Decision, Project, Settings, Step } from '../types'
import { AGENT_LABELS } from '../vocab'
import { ChatItem } from './ChatItem'
import { Icon } from './Icon'
import { Logo } from './Logo'
import type { Deciding } from './StepParts'
import { useToast } from './Toast'

/** A chat that hasn't started yet: it starts when the first message is sent. */
export interface Draft {
  projectId: string | null
}

export interface ChatPanelProps {
  detail: ConversationDetail | undefined
  draft: Draft | null
  projects: Project[]
  settings: Settings | undefined
  loading: boolean
  failed: boolean
  agents: Agent[]
  deciding: Deciding | null
  onDecide: (step: Step, decision: Decision) => void
  onShowOnCanvas: (stepId: string) => void
  onOpenNav: () => void
  onRetry: () => void
  onDraftProject: (projectId: string | null) => void
  onNewProject: () => void
  onCreated: (id: string, projectId: string | null) => void
  onOpenSettings: () => void
}

export function ChatPanel(props: ChatPanelProps) {
  const { detail, draft, loading, failed, onOpenNav, onRetry } = props
  const settingsAgent = props.settings?.agent
  const title = draft ? 'New chat' : detail ? detail.title : 'CloudBuddy'
  return (
    <section className="chat" aria-label="Chat">
      <header className="chat-head">
        <button type="button" className="icon-btn menu-btn" aria-label="Open navigation" onClick={onOpenNav}>
          <Icon name="menu" />
        </button>
        <div className="chat-head-main">
          <h1>{title}</h1>
          {draft
            ? settingsAgent && (
                <div className="chat-meta">
                  <AgentChip name={settingsAgent} agents={props.agents} />
                </div>
              )
            : detail && <ChatMeta detail={detail} agents={props.agents} projects={props.projects} />}
        </div>
      </header>
      {draft ? (
        <NewChat {...props} draft={draft} />
      ) : detail ? (
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
          ) : null}
        </div>
      )}
    </section>
  )
}

const agentName = (name: AgentName, agents: Agent[]) => agents.find((a) => a.name === name)?.label ?? AGENT_LABELS[name]

function AgentChip({ name, agents }: { name: AgentName; agents: Agent[] }) {
  return (
    <span className="meta-chip">
      <Icon name="bot" small />
      {agentName(name, agents)}
    </span>
  )
}

function FolderChip({ path, first }: { path: string; first?: boolean }) {
  return (
    <span className="meta-chip mono folder-chip" title={first ? `${path} (the agent starts here)` : path}>
      <Icon name="folder" small />
      <span>{path}</span>
    </span>
  )
}

function ChatMeta({ detail, agents, projects }: { detail: ConversationDetail; agents: Agent[]; projects: Project[] }) {
  const project = detail.projectId ? projects.find((p) => p.id === detail.projectId) : undefined
  const extra = detail.extraFolders ?? []
  return (
    <div className="chat-meta">
      <span className="meta-chip">
        <Icon name="bot" small />
        {agentName(detail.agent, agents)}
        {detail.model ? ` · ${detail.model}` : ''}
      </span>
      {project && (
        <span className="meta-chip project-chip">
          <Icon name="stack" small />
          {project.name}
        </span>
      )}
      <FolderChip path={detail.workdir} first={extra.length > 0} />
      {extra.map((f) => (
        <FolderChip key={f} path={f} />
      ))}
    </div>
  )
}

// ---- Composer --------------------------------------------------------------------------------------------------

interface ComposerProps {
  agentLabel: string
  value: string
  onChange: (text: string) => void
  onSubmit: () => void
  placeholder: string
  /** True while the message is on its way. */
  pending: boolean
  /** Nothing can be sent yet, for example when no folder is set. */
  blocked?: boolean
  autoFocus?: boolean
}

function Composer({ agentLabel, value, onChange, onSubmit, placeholder, pending, blocked = false, autoFocus = false }: ComposerProps) {
  const boxRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (autoFocus) boxRef.current?.focus()
  }, [autoFocus])
  const submit = (e?: FormEvent) => {
    e?.preventDefault()
    if (!value.trim() || pending || blocked) return
    onSubmit()
  }
  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      submit()
    }
  }
  return (
    <form className="composer" onSubmit={submit}>
      <textarea
        ref={boxRef}
        rows={2}
        aria-label="Message"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
      />
      <div className="composer-row">
        <span className="chip">
          <Icon name="bot" small />
          {agentLabel}
        </span>
        <span
          className="chip safe"
          title="Reads run on their own. Creates and changes show up as they happen. Anything you can't undo waits for you: deletes, moving money, reaching people, live mode."
        >
          <Icon name="shield" small />
          Asks before anything you can&apos;t undo
        </span>
        <button type="submit" className="send" aria-label="Send message" disabled={!value.trim() || pending || blocked}>
          <Icon name="send" />
        </button>
      </div>
    </form>
  )
}

// ---- A new chat ------------------------------------------------------------------------------------------------

const NEW_PROJECT = '__new'
const NO_PROJECT = ''

function NewChat({ draft, projects, settings, agents, onDraftProject, onNewProject, onCreated, onOpenSettings }: ChatPanelProps & { draft: Draft }) {
  const toast = useToast()
  const create = useCreateConversation()
  const [text, setText] = useState('')
  const project = draft.projectId ? projects.find((p) => p.id === draft.projectId) : undefined
  const label = settings ? agentName(settings.agent, agents) : 'the agent'
  const folders = project ? project.folders : settings?.folder ? [settings.folder] : []
  const noFolder = settings !== undefined && folders.length === 0

  const send = () => {
    const projectId = project?.id ?? null
    create.mutate(
      { text: text.trim(), projectId: projectId ?? undefined },
      {
        onSuccess: ({ id }) => onCreated(id, projectId),
        onError: (err) =>
          toast(
            err instanceof ApiError && err.code === 'no_folder'
              ? 'Choose a folder in Settings, or pick a project.'
              : err instanceof ApiError && err.status === 400
                ? `Couldn't start the chat: ${err.code}.`
                : "Couldn't start the chat. Try again.",
          ),
      },
    )
  }

  return (
    <>
      <div className="messages">
        <div className="start">
          <Logo className="start-mark" />
          <h2>What should {label} do?</h2>
          <div className="start-where">
            <label htmlFor="nc-project">Project</label>
            <select
              id="nc-project"
              value={project?.id ?? NO_PROJECT}
              onChange={(e) => {
                if (e.target.value === NEW_PROJECT) onNewProject()
                else onDraftProject(e.target.value || null)
              }}
            >
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
              <option value={NO_PROJECT}>No project</option>
              <option value={NEW_PROJECT}>New project…</option>
            </select>
          </div>
          {folders.length > 0 && (
            <ul className="start-folders" aria-label="Folders">
              {folders.map((f, i) => (
                <li key={f}>
                  <Icon name="folder" small />
                  <span className="mono">{f}</span>
                  {i === 0 && folders.length > 1 && <span className="start-here">starts here</span>}
                </li>
              ))}
            </ul>
          )}
          {noFolder ? (
            <div className="notice warning start-note" role="status">
              <Icon name="question" small />
              <span>
                Outside a project, the agent needs a folder to work in.{' '}
                <button type="button" className="btn link inline" onClick={onOpenSettings}>
                  Choose one in Settings
                </button>{' '}
                or pick a project.
              </span>
            </div>
          ) : (
            <p className="start-note">
              {project
                ? folders.length > 1
                  ? `${label} can read and change files in all ${folders.length} folders.`
                  : `${label} works in this project's folder.`
                : 'Outside a project, it works in the folder from Settings.'}
            </p>
          )}
        </div>
      </div>
      <Composer
        agentLabel={label}
        value={text}
        onChange={setText}
        onSubmit={send}
        placeholder={project ? `Ask ${label} to work on ${project.name}…` : `Ask ${label} to do some cloud work…`}
        pending={create.isPending}
        blocked={noFolder || settings === undefined}
        autoFocus
      />
    </>
  )
}

// ---- A chat that has started ------------------------------------------------------------------------------------

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

  const submit = () => {
    send.mutate(text.trim(), {
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
      <Composer
        agentLabel={label}
        value={text}
        onChange={setText}
        onSubmit={submit}
        placeholder={`Reply to ${label}…`}
        pending={send.isPending}
      />
    </>
  )
}
