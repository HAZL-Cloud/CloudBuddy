import { relativeWhen, plural } from '../format'
import type { Agent, ConversationSummary } from '../types'
import { AGENT_LABELS } from '../vocab'
import { Icon, type IconName } from './Icon'

export type NavKey = 'chats' | 'approvals' | 'connections' | 'settings'
const NAV: [NavKey, IconName, string][] = [
  ['chats', 'chat', 'Chats'],
  ['approvals', 'inbox', 'Approvals'],
  ['connections', 'plug', 'Connections'],
  ['settings', 'gear', 'Settings'],
]

export interface SidebarProps {
  conversations: ConversationSummary[]
  activeId: string | null
  page: 'chat' | 'connections'
  waitingTotal: number
  connectionCount: number | null
  agents: Agent[]
  /** A narrow screen shows the sidebar as a drawer. While it is closed nothing in it can be tabbed to. */
  hidden: boolean
  open: boolean
  onOpenChat: (id: string) => void
  onNav: (key: NavKey) => void
  onNewChat: () => void
  /** Only in mock mode. */
  onResetDemo?: () => void
}

function Recent({ c, on, agent, onOpen }: { c: ConversationSummary; on: boolean; agent: string; onOpen: () => void }) {
  let sub: string
  let dot = 'ok'
  let tone = ''
  if (c.status === 'waiting') {
    sub = 'Waiting for your OK'
    dot = 'wait'
    tone = 'wait'
  } else if (c.status === 'running') {
    sub = 'Working…'
    dot = 'run'
  } else {
    const when = `${plural(c.stepCount, 'step')} · ${relativeWhen(c.updatedAt)}`
    if (c.status === 'failed' || c.status === 'interrupted') {
      sub = `${c.status === 'failed' ? 'Failed' : 'Stopped'} · ${when}`
      dot = 'bad'
      tone = 'bad'
    } else {
      sub = when
    }
  }
  return (
    <li>
      <button type="button" className={`recent ${on ? 'on' : ''}`} aria-current={on ? 'page' : undefined} onClick={onOpen}>
        <span className="recent-title">{c.title}</span>
        <span className={`recent-sub ${tone}`}>
          <span className={`dot ${dot}`} />
          <span>
            {agent} · {sub}
          </span>
        </span>
      </button>
    </li>
  )
}

export function Sidebar(p: SidebarProps) {
  const agentLabel = (name: ConversationSummary['agent']) => p.agents.find((a) => a.name === name)?.label ?? AGENT_LABELS[name]
  const current = p.page === 'chat' ? 'chats' : 'connections'
  // `inert` is not in React 18's types, so it goes in through a spread.
  const inert = p.hidden && !p.open ? { inert: '' } : {}
  return (
    <aside className="sidebar" aria-label="Navigation" {...inert}>
      <div className="brand">
        <span className="brand-mark">
          <Icon name="cloud" />
        </span>
        <span className="brand-name">CloudBuddy</span>
        {window.location.port && <span className="brand-host">:{window.location.port}</span>}
      </div>
      <button type="button" className="new-chat" onClick={p.onNewChat}>
        <Icon name="plus" />
        New chat
      </button>
      <nav className="nav" aria-label="Main">
        {NAV.map(([key, icon, label]) => (
          <button
            key={key}
            type="button"
            className={`nav-item ${key === current ? 'on' : ''}`}
            aria-current={key === current ? 'page' : undefined}
            onClick={() => p.onNav(key)}
          >
            <Icon name={icon} />
            <span>{label}</span>
            {key === 'approvals' && p.waitingTotal > 0 && (
              <span className="badge" aria-label={`${p.waitingTotal} waiting`}>
                {p.waitingTotal}
              </span>
            )}
            {key === 'connections' && p.connectionCount !== null && <span className="badge quiet">{p.connectionCount}</span>}
          </button>
        ))}
      </nav>
      <div className="side-label">Recent</div>
      <ul className="recents">
        {p.conversations.map((c) => (
          <Recent key={c.id} c={c} on={c.id === p.activeId && p.page === 'chat'} agent={agentLabel(c.agent)} onOpen={() => p.onOpenChat(c.id)} />
        ))}
      </ul>
      <div className="runtimes">
        <div className="side-label">Agents on this computer</div>
        {p.agents.map((a) => (
          <div key={a.name} className="runtime">
            <span className={`dot ${a.available ? 'ok' : 'off'}`} />
            {a.label}
            <span className="mono">{a.available ? a.version : 'not found'}</span>
          </div>
        ))}
        {p.onResetDemo && (
          <button type="button" className="reset" onClick={p.onResetDemo}>
            <Icon name="reset" small />
            Reset this demo
          </button>
        )}
      </div>
    </aside>
  )
}
