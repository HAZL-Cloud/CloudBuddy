import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { useCreateConversation } from '../queries'
import type { Agent, AgentName } from '../types'

export interface NewChatDialogProps {
  agents: Agent[]
  /** Pre-filled from the latest chat. */
  defaultWorkdir: string
  onClose: () => void
  onCreated: (id: string) => void
}

export function NewChatDialog({ agents, defaultWorkdir, onClose, onCreated }: NewChatDialogProps) {
  const create = useCreateConversation()
  const firstAvailable = agents.find((a) => a.available)?.name ?? 'claude'
  const [agent, setAgent] = useState<AgentName>(firstAvailable)
  const [workdir, setWorkdir] = useState(defaultWorkdir)
  const [text, setText] = useState('')
  const dialogRef = useRef<HTMLDivElement>(null)
  const opener = useRef<Element | null>(document.activeElement)

  useEffect(() => {
    dialogRef.current?.querySelector<HTMLElement>('select, input, textarea')?.focus()
    const from = opener.current
    return () => {
      if (from instanceof HTMLElement) from.focus()
    }
  }, [])

  const ready = workdir.trim() !== '' && text.trim() !== '' && !create.isPending
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!ready) return
    create.mutate({ agent, workdir: workdir.trim(), text: text.trim() }, { onSuccess: ({ id }) => onCreated(id) })
  }

  // Keep Tab inside the dialog while it is open.
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      onClose()
      return
    }
    if (e.key !== 'Tab') return
    const items = Array.from(dialogRef.current?.querySelectorAll<HTMLElement>('select, input, textarea, button:not(:disabled)') ?? [])
    const first = items[0]
    const last = items[items.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault()
      last?.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault()
      first?.focus()
    }
  }

  return (
    <div className="dialog-scrim" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="dialog" role="dialog" aria-modal="true" aria-labelledby="new-chat-title" ref={dialogRef} onKeyDown={onKeyDown}>
        <h2 id="new-chat-title">New chat</h2>
        <form onSubmit={submit}>
          <div className="field">
            <label htmlFor="nc-agent">Agent</label>
            <select id="nc-agent" value={agent} onChange={(e) => setAgent(e.target.value as AgentName)}>
              {agents.map((a) => (
                <option key={a.name} value={a.name} disabled={!a.available}>
                  {a.label}
                  {a.available ? ` ${a.version}` : ' (not found on this computer)'}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor="nc-folder">Working folder</label>
            <input
              id="nc-folder"
              className="mono"
              value={workdir}
              placeholder="~/code/my-project"
              autoComplete="off"
              spellCheck={false}
              onChange={(e) => setWorkdir(e.target.value)}
            />
          </div>
          <div className="field">
            <label htmlFor="nc-text">First message</label>
            <textarea
              id="nc-text"
              value={text}
              placeholder="What should the agent do?"
              onChange={(e) => setText(e.target.value)}
            />
          </div>
          {create.isError && (
            <div className="field-error" role="alert">
              Couldn&apos;t start the chat. Check the working folder and try again.
            </div>
          )}
          <div className="dialog-actions">
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn danger" disabled={!ready}>
              {create.isPending ? 'Starting…' : 'Start chat'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
