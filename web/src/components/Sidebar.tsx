import { relativeWhen, plural } from '../format'
import type { Theme } from '../theme'
import type { Agent, ConversationSummary, Project } from '../types'
import { AGENT_LABELS } from '../vocab'
import { Icon, type IconName } from './Icon'
import { Logo, Wordmark } from './Logo'

export type NavKey = 'chats' | 'approvals' | 'connections' | 'settings'
export type Page = 'chat' | 'connections' | 'settings'
const NAV: [NavKey, IconName, string][] = [
  ['chats', 'chat', 'Chats'],
  ['approvals', 'inbox', 'Approvals'],
  ['connections', 'plug', 'Connections'],
  ['settings', 'gear', 'Settings'],
]
const THEMES: [Theme, IconName, string][] = [
  ['system', 'monitor', 'Match this computer'],
  ['light', 'sun', 'Light'],
  ['dark', 'moon', 'Dark'],
]

export interface SidebarProps {
  /** The chats in the chosen project, or all of them. */
  conversations: ConversationSummary[]
  totalChats: number
  projects: Project[]
  /** The project the chat list is narrowed to; null for all chats. */
  scope: string | null
  activeId: string | null
  page: Page
  waitingTotal: number
  connectionCount: number | null
  agents: Agent[]
  theme: Theme
  onTheme: (theme: Theme) => void
  /** A narrow screen shows the sidebar as a drawer. While it is closed nothing in it can be tabbed to. */
  hidden: boolean
  open: boolean
  onOpenChat: (id: string) => void
  onNav: (key: NavKey) => void
  onNewChat: () => void
  onScope: (projectId: string | null) => void
  onNewProject: () => void
  onEditProject: (project: Project) => void
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

function Projects(p: SidebarProps) {
  return (
    <div className="side-projects">
      <div className="side-head">
        <span className="side-label" id="projects-label">
          Projects
        </span>
        <button type="button" className="side-add" aria-label="New project" title="New project" onClick={p.onNewProject}>
          <Icon name="plus" small />
        </button>
      </div>
      <ul className="projects" aria-labelledby="projects-label">
        <li className="project-row">
          <button type="button" className={`project ${p.scope === null ? 'on' : ''}`} aria-current={p.scope === null ? 'true' : undefined} onClick={() => p.onScope(null)}>
            <Icon name="chat" small />
            <span className="project-name">All chats</span>
            <span className="project-count">{p.totalChats}</span>
          </button>
        </li>
        {p.projects.map((pr) => (
          <li key={pr.id} className="project-row">
            <button
              type="button"
              className={`project ${p.scope === pr.id ? 'on' : ''}`}
              aria-current={p.scope === pr.id ? 'true' : undefined}
              title={pr.folders.join('\n')}
              onClick={() => p.onScope(pr.id)}
            >
              <Icon name="stack" small />
              <span className="project-name">{pr.name}</span>
              <span className="project-count">{pr.chatCount}</span>
            </button>
            <button type="button" className="project-edit" aria-label={`Edit ${pr.name}`} title="Edit project" onClick={() => p.onEditProject(pr)}>
              <Icon name="pencil" small />
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

export function Sidebar(p: SidebarProps) {
  const agentLabel = (name: ConversationSummary['agent']) => p.agents.find((a) => a.name === name)?.label ?? AGENT_LABELS[name]
  const current: NavKey = p.page === 'chat' ? 'chats' : p.page
  const scoped = p.scope ? p.projects.find((x) => x.id === p.scope) : undefined
  // `inert` is not in React 18's types, so it goes in through a spread.
  const inert = p.hidden && !p.open ? { inert: '' } : {}
  return (
    <aside className="sidebar" aria-label="Navigation" {...inert}>
      <div className="brand">
        <Logo className="brand-mark" />
        <span className="brand-name">
          <Wordmark />
        </span>
        {window.location.port && <span className="brand-host">:{window.location.port}</span>}
      </div>
      <button type="button" className="new-chat" aria-label={scoped ? `New chat in ${scoped.name}` : undefined} onClick={p.onNewChat}>
        <Icon name="plus" />
        New chat
        {scoped && <span className="new-chat-in">in {scoped.name}</span>}
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
      <Projects {...p} />
      <div className="side-label">{scoped ? `Chats in ${scoped.name}` : 'Recent'}</div>
      <ul className="recents">
        {p.conversations.map((c) => (
          <Recent key={c.id} c={c} on={c.id === p.activeId && p.page === 'chat'} agent={agentLabel(c.agent)} onOpen={() => p.onOpenChat(c.id)} />
        ))}
      </ul>
      {scoped && p.conversations.length === 0 && <p className="side-empty">No chats in this project yet.</p>}
      <div className="runtimes">
        <div className="side-label">Agents on this computer</div>
        {p.agents.map((a) => (
          <div key={a.name} className="runtime">
            <span className={`dot ${a.available ? 'ok' : 'off'}`} />
            {a.label}
            <span className="mono">{a.available ? a.version : 'not found'}</span>
          </div>
        ))}
        <div className="theme-row">
          <span id="theme-label">Theme</span>
          <div className="seg" role="group" aria-labelledby="theme-label">
            {THEMES.map(([key, icon, label]) => (
              <button
                key={key}
                type="button"
                className={p.theme === key ? 'on' : ''}
                aria-pressed={p.theme === key}
                aria-label={label}
                title={label}
                onClick={() => p.onTheme(key)}
              >
                <Icon name={icon} small />
              </button>
            ))}
          </div>
        </div>
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
