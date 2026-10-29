import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ApiError } from './api'
import { CanvasPanel, type FocusRequest } from './components/CanvasPanel'
import { ChatPanel, type Draft } from './components/ChatPanel'
import { ConnectionsPage } from './components/ConnectionsPage'
import { Icon } from './components/Icon'
import { ProjectDialog } from './components/ProjectDialog'
import { SettingsPage } from './components/SettingsPage'
import { Sidebar, type NavKey, type Page } from './components/Sidebar'
import type { Deciding } from './components/StepParts'
import { useToast } from './components/Toast'
import { useMediaQuery, useStoredState } from './hooks'
import { useConnections, useConversation, useConversations, useDecide, useProjects, useSession, useSettings } from './queries'
import { connectStream } from './stream'
import { applyTheme, THEME_KEY, type Theme } from './theme'
import type { ConversationSummary, Decision, Project, Step } from './types'

const NO_CONVERSATIONS: ConversationSummary[] = []
const NO_PROJECTS: Project[] = []

export function App() {
  const toast = useToast()
  const client = useQueryClient()
  const narrow = useMediaQuery('(max-width: 1100px)')
  const mobile = useMediaQuery('(max-width: 760px)')

  const session = useSession()
  const list = useConversations()
  const connections = useConnections()
  const projectsQuery = useProjects()
  const settings = useSettings()
  const decide = useDecide()

  const [page, setPage] = useState<Page>('chat')
  const [view, setView] = useState<'chat' | 'canvas'>('canvas')
  const [navOpen, setNavOpen] = useState(false)
  const [chosen, setChosen] = useState<string | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [editing, setEditing] = useState<{ project: Project | null } | null>(null)
  const [storedScope, setScope] = useStoredState<string | null>('cloudbuddy.project', null)
  const [canvasOpen, setCanvasOpen] = useStoredState('cloudbuddy.canvasOpen', true)
  const [storedChoice, setTheme] = useStoredState<Theme>(THEME_KEY, 'system')
  const theme: Theme = storedChoice === 'light' || storedChoice === 'dark' ? storedChoice : 'system'
  const [focus, setFocus] = useState<FocusRequest | null>(null)
  const nonce = useRef(0)

  useEffect(() => connectStream(client), [client])
  useEffect(() => applyTheme(theme), [theme])

  const conversations = list.data ?? NO_CONVERSATIONS
  const projects = projectsQuery.data ?? NO_PROJECTS
  // A remembered project that has since been deleted means all chats.
  const scope = storedScope && (!projectsQuery.data || projects.some((p) => p.id === storedScope)) ? storedScope : null
  const visible = useMemo(() => (scope ? conversations.filter((c) => c.projectId === scope) : conversations), [conversations, scope])

  // The newest chat in view is open until you pick another. A chat you just started may not be in the list yet.
  const known = chosen !== null && (!list.data || list.isFetching || conversations.some((c) => c.id === chosen))
  const convId = draft ? null : known ? chosen : (visible[0]?.id ?? null)
  const detailQuery = useConversation(convId)
  const detail = detailQuery.data
  // With no chat to show, the middle panel is a new chat.
  const shownDraft = draft ?? (list.isSuccess && convId === null ? { projectId: scope } : null)
  const agents = session.data?.agents ?? []

  const waitingTotal = conversations.reduce((sum, c) => sum + c.waitingCount, 0)
  useEffect(() => {
    document.title = waitingTotal > 0 ? `(${waitingTotal}) Waiting for you · CloudBuddy` : 'CloudBuddy'
  }, [waitingTotal])

  useEffect(() => {
    if (!navOpen) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setNavOpen(false)
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [navOpen])
  useEffect(() => {
    if (!narrow) setNavOpen(false)
  }, [narrow])

  // Never optimistic: the buttons only wait. What changed is whatever the server says on the refetch.
  const { mutate: sendDecision, isPending: decisionPending, variables: decisionVars } = decide
  const deciding = useMemo<Deciding | null>(
    () => (decisionPending && decisionVars ? { approvalId: decisionVars.approvalId, decision: decisionVars.decision } : null),
    [decisionPending, decisionVars],
  )
  const onDecide = useCallback(
    (step: Step, decision: Decision) => {
      const approvalId = step.approval?.id
      if (!approvalId || decisionPending) return
      sendDecision(
        { approvalId, decision },
        {
          onSuccess: () => toast(decision === 'approve' ? 'Approved. The step is running now.' : 'Said no. The step did not run.'),
          onError: (err) =>
            toast(
              err instanceof ApiError && err.status === 409
                ? 'That was already decided or has expired. Showing the latest.'
                : "Couldn't send your decision, so nothing was approved. Try again.",
            ),
        },
      )
    },
    [sendDecision, decisionPending, toast],
  )

  const showOnCanvas = (stepId: string) => {
    if (!convId) return
    setCanvasOpen(true)
    setView('canvas')
    setFocus({ convId, target: { stepId }, nonce: ++nonce.current })
  }
  const onFocusHandled = useCallback((n: number) => setFocus((f) => (f?.nonce === n ? null : f)), [])

  const goToChat = () => {
    setNavOpen(false)
    setPage('chat')
  }
  const openChat = (id: string) => {
    goToChat()
    setDraft(null)
    setChosen(id)
  }
  // A new chat starts in the project you are looking at: the one the list is narrowed to, else the open chat's.
  const newChat = () => {
    goToChat()
    setView('chat')
    const pick = scope ?? (draft ? draft.projectId : (detail?.projectId ?? null))
    setDraft({ projectId: pick && projects.some((p) => p.id === pick) ? pick : null })
  }
  const pickScope = (projectId: string | null) => {
    goToChat()
    setScope(projectId)
    setDraft(null)
    setChosen(null)
  }

  const onNav = (key: NavKey) => {
    setNavOpen(false)
    if (key === 'chats') setPage('chat')
    else if (key === 'connections' || key === 'settings') setPage(key)
    else {
      const waiting = conversations.find((c) => c.waitingCount > 0)
      if (!waiting) return toast('Nothing is waiting for your approval.')
      if (scope && waiting.projectId !== scope) setScope(null)
      setDraft(null)
      setChosen(waiting.id)
      setPage('chat')
      setView('canvas')
      setCanvasOpen(true)
      setFocus({ convId: waiting.id, target: 'waiting', nonce: ++nonce.current })
    }
  }

  const onProjectSaved = (saved: Project) => {
    const isNew = editing?.project === null
    setEditing(null)
    // A new project is where you are going next: its chat list, with a new chat ready in it.
    if (isNew) {
      setScope(saved.id)
      setChosen(null)
      setDraft({ projectId: saved.id })
    }
  }
  const onProjectDeleted = (id: string) => {
    setEditing(null)
    if (storedScope === id) setScope(null)
    if (draft?.projectId === id) setDraft({ projectId: null })
  }

  const resetDemo =
    import.meta.env.VITE_MOCK === '1'
      ? () => {
          void import('./mock/mockApi').then((mock) => {
            mock.resetMock()
            setChosen(null)
            setDraft(null)
            setScope(null)
            void client.invalidateQueries()
            toast('Demo reset.')
          })
        }
      : undefined

  const hasWaiting = detail?.steps.some((s) => s.status === 'waiting') ?? false
  const shownView = shownDraft ? 'chat' : view
  const collapsed = !canvasOpen && !mobile

  return (
    <div className={`app ${navOpen ? 'nav-open' : ''}`} data-page={page} data-view={shownView} data-canvas={collapsed ? 'closed' : 'open'}>
      <Sidebar
        conversations={visible}
        totalChats={conversations.length}
        projects={projects}
        scope={scope}
        activeId={shownDraft ? null : convId}
        page={page}
        waitingTotal={waitingTotal}
        connectionCount={connections.data ? connections.data.cli.length + connections.data.mcp.length : null}
        agents={agents}
        theme={theme}
        onTheme={setTheme}
        hidden={narrow}
        open={navOpen}
        onOpenChat={openChat}
        onNav={onNav}
        onNewChat={newChat}
        onScope={pickScope}
        onNewProject={() => setEditing({ project: null })}
        onEditProject={(project) => setEditing({ project })}
        onResetDemo={resetDemo}
      />
      {navOpen && <div className="scrim" onClick={() => setNavOpen(false)} />}

      <div className="mobile-switch">
        <button type="button" className="icon-btn" aria-label="Open navigation" onClick={() => setNavOpen(true)}>
          <Icon name="menu" />
        </button>
        <div className="seg" role="group" aria-label="Choose panel">
          <button type="button" className={shownView === 'chat' ? 'on' : ''} aria-pressed={shownView === 'chat'} onClick={() => setView('chat')}>
            Chat
          </button>
          <button
            type="button"
            className={shownView === 'canvas' ? 'on' : ''}
            aria-pressed={shownView === 'canvas'}
            disabled={shownDraft !== null}
            onClick={() => setView('canvas')}
          >
            Canvas
            {hasWaiting && <span className="tab-dot" role="img" aria-label="Waiting for you" />}
          </button>
        </div>
      </div>

      <ChatPanel
        detail={shownDraft ? undefined : detail}
        draft={shownDraft}
        projects={projects}
        settings={settings.data}
        loading={list.isLoading || detailQuery.isLoading}
        failed={detailQuery.isError || list.isError}
        agents={agents}
        deciding={deciding}
        onDecide={onDecide}
        onShowOnCanvas={showOnCanvas}
        onOpenNav={() => setNavOpen(true)}
        onRetry={() => void client.invalidateQueries()}
        onDraftProject={(projectId) => setDraft({ projectId })}
        onNewProject={() => setEditing({ project: null })}
        onCreated={(id, projectId) => {
          setDraft(null)
          setChosen(id)
          setView('chat')
          if (scope !== null && scope !== projectId) setScope(projectId)
        }}
        onOpenSettings={() => setPage('settings')}
      />
      <CanvasPanel
        key={convId ?? 'none'}
        detail={shownDraft ? undefined : detail}
        mobile={mobile}
        focusRequest={focus}
        onFocusHandled={onFocusHandled}
        deciding={deciding}
        onDecide={onDecide}
        collapsed={collapsed}
        onCollapse={() => setCanvasOpen(false)}
        onExpand={() => setCanvasOpen(true)}
      />
      {page === 'connections' && <ConnectionsPage />}
      {page === 'settings' && (
        <SettingsPage
          agents={agents}
          projects={projects}
          onNewProject={() => setEditing({ project: null })}
          onEditProject={(project) => setEditing({ project })}
        />
      )}

      {editing && (
        <ProjectDialog project={editing.project} onClose={() => setEditing(null)} onSaved={onProjectSaved} onDeleted={onProjectDeleted} />
      )}
    </div>
  )
}
