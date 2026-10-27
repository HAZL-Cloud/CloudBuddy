import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ApiError } from './api'
import { useSignedOut } from './auth'
import { CanvasPanel, type FocusRequest } from './components/CanvasPanel'
import { ChatPanel } from './components/ChatPanel'
import { ConnectionsPage } from './components/ConnectionsPage'
import { Icon } from './components/Icon'
import { NewChatDialog } from './components/NewChatDialog'
import { NotSignedIn } from './components/NotSignedIn'
import { Sidebar, type NavKey } from './components/Sidebar'
import type { Deciding } from './components/StepParts'
import { useToast } from './components/Toast'
import { useMediaQuery } from './hooks'
import { useConnections, useConversation, useConversations, useDecide, useSession } from './queries'
import { connectStream } from './stream'
import type { ConversationSummary, Decision, Step } from './types'

const NO_CONVERSATIONS: ConversationSummary[] = []

export function App() {
  return useSignedOut() ? <NotSignedIn /> : <Shell />
}

function Shell() {
  const toast = useToast()
  const client = useQueryClient()
  const narrow = useMediaQuery('(max-width: 1100px)')
  const mobile = useMediaQuery('(max-width: 760px)')

  const session = useSession()
  const list = useConversations()
  const connections = useConnections()
  const decide = useDecide()

  const [page, setPage] = useState<'chat' | 'connections'>('chat')
  const [view, setView] = useState<'chat' | 'canvas'>('canvas')
  const [navOpen, setNavOpen] = useState(false)
  const [chosen, setChosen] = useState<string | null>(null)
  const [newChatOpen, setNewChatOpen] = useState(false)
  const [focus, setFocus] = useState<FocusRequest | null>(null)
  const nonce = useRef(0)

  useEffect(() => connectStream(client), [client])

  const conversations = list.data ?? NO_CONVERSATIONS
  // The newest chat is open until you pick another. A chat that no longer exists falls back to the newest.
  const convId = chosen && (!list.data || list.data.some((c) => c.id === chosen)) ? chosen : (conversations[0]?.id ?? null)
  const detailQuery = useConversation(convId)
  const detail = detailQuery.data
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
    setView('canvas')
    setFocus({ convId, target: { stepId }, nonce: ++nonce.current })
  }
  const onFocusHandled = useCallback((n: number) => setFocus((f) => (f?.nonce === n ? null : f)), [])

  const onNav = (key: NavKey) => {
    setNavOpen(false)
    if (key === 'chats') setPage('chat')
    else if (key === 'connections') setPage('connections')
    else if (key === 'settings') toast('Settings are not available yet.')
    else {
      const waiting = conversations.find((c) => c.waitingCount > 0)
      if (!waiting) return toast('Nothing is waiting for your approval.')
      setChosen(waiting.id)
      setPage('chat')
      setView('canvas')
      setFocus({ convId: waiting.id, target: 'waiting', nonce: ++nonce.current })
    }
  }

  const resetDemo =
    import.meta.env.VITE_MOCK === '1'
      ? () => {
          void import('./mock/mockApi').then((mock) => {
            mock.resetMock()
            setChosen(null)
            void client.invalidateQueries()
            toast('Demo reset.')
          })
        }
      : undefined

  const hasWaiting = detail?.steps.some((s) => s.status === 'waiting') ?? false

  return (
    <div className="app" data-page={page} data-view={view}>
      <Sidebar
        conversations={conversations}
        activeId={convId}
        page={page}
        waitingTotal={waitingTotal}
        connectionCount={connections.data ? connections.data.cli.length + connections.data.mcp.length : null}
        agents={agents}
        hidden={narrow}
        open={navOpen}
        onOpenChat={(id) => {
          setChosen(id)
          setPage('chat')
          setNavOpen(false)
        }}
        onNav={onNav}
        onNewChat={() => {
          setNavOpen(false)
          setNewChatOpen(true)
        }}
        onResetDemo={resetDemo}
      />
      {navOpen && <div className="scrim" onClick={() => setNavOpen(false)} />}

      <div className="mobile-switch">
        <button type="button" className="icon-btn" aria-label="Open navigation" onClick={() => setNavOpen(true)}>
          <Icon name="menu" />
        </button>
        <div className="seg" role="group" aria-label="Choose panel">
          <button type="button" className={view === 'chat' ? 'on' : ''} aria-pressed={view === 'chat'} onClick={() => setView('chat')}>
            Chat
          </button>
          <button type="button" className={view === 'canvas' ? 'on' : ''} aria-pressed={view === 'canvas'} onClick={() => setView('canvas')}>
            Canvas
            {hasWaiting && <span className="tab-dot" role="img" aria-label="Waiting for you" />}
          </button>
        </div>
      </div>

      <ChatPanel
        detail={detail}
        loading={list.isLoading || detailQuery.isLoading}
        failed={detailQuery.isError || list.isError}
        agents={agents}
        deciding={deciding}
        onDecide={onDecide}
        onShowOnCanvas={showOnCanvas}
        onOpenNav={() => setNavOpen(true)}
        onNewChat={() => setNewChatOpen(true)}
        onRetry={() => void client.invalidateQueries()}
      />
      <CanvasPanel
        key={convId ?? 'none'}
        detail={detail}
        mobile={mobile}
        focusRequest={focus}
        onFocusHandled={onFocusHandled}
        deciding={deciding}
        onDecide={onDecide}
      />
      {page === 'connections' && <ConnectionsPage />}

      {newChatOpen && (
        <NewChatDialog
          agents={agents}
          defaultWorkdir={conversations[0]?.workdir ?? ''}
          onClose={() => setNewChatOpen(false)}
          onCreated={(id) => {
            setNewChatOpen(false)
            setChosen(id)
            setPage('chat')
            setView('chat')
          }}
        />
      )}
    </div>
  )
}
