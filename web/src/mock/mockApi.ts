import { ApiError } from '../api'
import type {
  Agent,
  AgentName,
  ConnectionsResponse,
  ConversationDetail,
  ConversationSummary,
  Decision,
  Rule,
  Step,
  StreamEvent,
} from '../types'
import { seedConnections, seedConversations } from './data'

// A stand-in for the Go server, used when VITE_MOCK=1 and by the tests. It answers the same paths with the same
// status codes as docs/api.md, so the real client code (CSRF header, 401, 409) is exercised too.

export const MOCK_CSRF = 'mock-csrf-token'

const AGENTS: Agent[] = [
  { name: 'claude', label: 'Claude Code', version: '2.1.311', available: true },
  { name: 'codex', label: 'Codex', version: '0.163.0', available: true },
]

interface MockState {
  convs: ConversationDetail[]
  connections: ConnectionsResponse
  signedIn: boolean
  /** Multiplies every delay. Tests set it to 0. */
  speed: number
  counter: number
}

const state: MockState = {
  convs: seedConversations(),
  connections: seedConnections(),
  signedIn: true,
  speed: 1,
  counter: 0,
}
const timers = new Set<ReturnType<typeof setTimeout>>()
const listeners = new Set<(ev: StreamEvent) => void>()

export function configureMock(opts: { speed?: number; signedIn?: boolean }): void {
  if (opts.speed !== undefined) state.speed = opts.speed
  if (opts.signedIn !== undefined) state.signedIn = opts.signedIn
}

export function subscribeMock(fn: (ev: StreamEvent) => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

const emit = (ev: StreamEvent) => listeners.forEach((fn) => fn(ev))

export function resetMock(): void {
  timers.forEach(clearTimeout)
  timers.clear()
  state.convs = seedConversations()
  state.connections = seedConnections()
  state.signedIn = true
  emit({ type: 'conversations' })
  emit({ type: 'connections' })
}

function later(ms: number, fn: () => void): void {
  const id = setTimeout(() => {
    timers.delete(id)
    fn()
  }, ms * state.speed)
  timers.add(id)
}

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T
const nextId = (prefix: string) => `${prefix}${++state.counter}`
const now = () => new Date().toISOString()
/** A chat that just changed sorts first, as on the server. Seeded chats use clock times that can still be ahead today. */
const newest = () => {
  const latest = state.convs.reduce((m, c) => (c.updatedAt > m ? c.updatedAt : m), '')
  return latest >= now() ? new Date(Date.parse(latest) + 1000).toISOString() : now()
}

function summary(c: ConversationDetail): ConversationSummary {
  const { id, title, agent, model, workdir, status, updatedAt, stepCount, waitingCount } = c
  return { id, title, agent, model, workdir, status, updatedAt, stepCount, waitingCount }
}

// Keeps the summary fields in step with the steps after every change.
function touch(c: ConversationDetail): void {
  c.stepCount = c.steps.length
  c.waitingCount = c.steps.filter((s) => s.status === 'waiting').length
  c.updatedAt = newest()
  emit({ type: 'conversation', conversationId: c.id })
}

const say = (c: ConversationDetail, text: string) => c.chat.push({ id: nextId('m'), type: 'agent', text, at: now() })

function findConv(id: string): ConversationDetail {
  const c = state.convs.find((x) => x.id === id)
  if (!c) throw new ApiError(404, 'not_found')
  return c
}

const agentLabel = (name: AgentName) => AGENTS.find((a) => a.name === name)?.label ?? name

function replyLater(c: ConversationDetail, delay: number): void {
  later(delay, () => {
    say(c, `This is mock mode, so nothing runs here. In the real app, ${agentLabel(c.agent)} would pick up your message and every step it takes would land on the canvas.`)
    c.status = 'done'
    touch(c)
  })
}

function createConversation(body: { agent: AgentName; workdir: string; text: string }): { id: string } {
  const id = nextId('chat-')
  const text = body.text.trim()
  const c: ConversationDetail = {
    id,
    title: text.length > 60 ? `${text.slice(0, 57)}…` : text,
    agent: body.agent,
    model: '',
    workdir: body.workdir,
    status: 'running',
    updatedAt: newest(),
    stepCount: 0,
    waitingCount: 0,
    subtitle: 'No cloud steps yet',
    chat: [{ id: nextId('m'), type: 'user', text, at: now() }],
    steps: [],
  }
  state.convs.unshift(c)
  emit({ type: 'conversations' })
  replyLater(c, 900)
  return { id }
}

function sendMessage(id: string, text: string): void {
  const c = findConv(id)
  if (c.status === 'running' || c.status === 'waiting') throw new ApiError(409, 'run_active')
  c.chat.push({ id: nextId('m'), type: 'user', text, at: now() })
  c.status = 'running'
  touch(c)
  replyLater(c, 700)
}

function stop(id: string): void {
  const c = findConv(id)
  if (c.status !== 'running') return
  c.status = 'interrupted'
  c.chat.push({ id: nextId('m'), type: 'notice', level: 'info', text: 'You stopped this run.', at: now() })
  touch(c)
}

// ---- Approvals -------------------------------------------------------------------------------------------------

function findByApproval(approvalId: string): { conv: ConversationDetail; step: Step } | null {
  for (const conv of state.convs) {
    const step = conv.steps.find((s) => s.approval?.id === approvalId)
    if (step) return { conv, step }
  }
  return null
}

function decide(approvalId: string, decision: Decision): Step {
  const found = findByApproval(approvalId)
  if (!found) throw new ApiError(404, 'not_found')
  const { conv, step } = found
  if (!step.approval || step.approval.state !== 'pending') throw new ApiError(409, 'not_pending')
  step.approval = { ...step.approval, state: decision === 'approve' ? 'approved' : 'denied', decidedAt: now() }
  if (decision === 'approve') approve(conv, step)
  else deny(conv, step)
  return clone(step)
}

const stillWaiting = (conv: ConversationDetail) => conv.steps.some((s) => s.status === 'waiting')

// The newsletter's send step has a scripted ending, as in the mockup: sent, then a delivery check, then the result.
// Any other pending step just runs and finishes.
function approve(conv: ConversationDetail, step: Step): void {
  step.status = 'running'
  conv.status = 'running'
  touch(conv)
  if (step.id !== 'n9') {
    later(1200, () => {
      step.status = 'done'
      step.durationMs = 1200
      step.endedAt = now()
      conv.status = stillWaiting(conv) ? 'waiting' : 'done'
      touch(conv)
    })
    return
  }
  say(conv, 'Sending now.')
  later(1700, () => {
    step.status = 'done'
    step.durationMs = 2100
    step.endedAt = now()
    step.output = '{\n  "id": "bc_7e2a91f4",\n  "status": "sent",\n  "recipients": 1240\n}'
    const check: Step = {
      ...step,
      id: `${step.id}-check`,
      seq: conv.steps.length + 1,
      action: 'read',
      tags: [],
      title: 'Checked delivery',
      facts: [['Delivered', '1,236'], ['Bounced', '4']],
      api: 'resend · get_broadcast',
      command: 'get_broadcast({ "id": "bc_7e2a91f4" })',
      why: 'Makes sure the email actually went out.',
      status: 'running',
      startedAt: now(),
      endedAt: undefined,
      durationMs: undefined,
      output: '',
      uses: [],
      approval: undefined,
    }
    conv.steps.push(check)
    conv.chat.push(
      { id: nextId('m'), type: 'steps', stepIds: [step.id], at: now() },
      { id: nextId('m'), type: 'agent', text: "Sent. I'll check how many were delivered.", at: now() },
      { id: nextId('m'), type: 'steps', stepIds: [check.id], at: now() },
    )
    touch(conv)
    later(1900, () => {
      check.status = 'done'
      check.durationMs = 800
      check.endedAt = now()
      check.output = '{\n  "status": "sent",\n  "delivered": 1236,\n  "bounced": 4\n}'
      conv.status = stillWaiting(conv) ? 'waiting' : 'done'
      conv.result = { title: 'Newsletter is live', line: '1,236 delivered · $8/month plan on sale', at: now() }
      say(conv, "Done. `news@shipyard.dev` is verified, the $8/month plan is on sale, and the launch email reached 1,236 of 1,240 people (4 bounced). Every step across all four services is on the canvas.")
      touch(conv)
    })
  })
}

function deny(conv: ConversationDetail, step: Step): void {
  step.status = 'denied'
  step.output = ''
  conv.status = stillWaiting(conv) ? 'waiting' : 'done'
  if (step.id === 'n9') {
    conv.result = { title: 'Newsletter is set up', line: 'Launch email saved as a draft', at: now() }
    say(conv, "Okay, I didn't send it. The draft is saved in Resend, so you can send it from there or ask me later.")
  }
  touch(conv)
}

// ---- Connections -----------------------------------------------------------------------------------------------

function setRule(id: string, rule: Rule) {
  const conn = [...state.connections.cli, ...state.connections.mcp].find((c) => c.id === id)
  if (!conn) throw new ApiError(404, 'not_found')
  conn.rule = rule
  emit({ type: 'connections' })
  return clone(conn)
}

// ---- Router ----------------------------------------------------------------------------------------------------

export interface MockRequest {
  method: string
  path: string
  body: unknown
  csrf: string | null
}

export async function mockRequest({ method, path, body, csrf }: MockRequest): Promise<unknown> {
  await new Promise((resolve) => setTimeout(resolve, 40 * state.speed))
  if (!state.signedIn) throw new ApiError(401, 'not_signed_in')
  if (method !== 'GET' && csrf !== MOCK_CSRF) throw new ApiError(403, 'bad_csrf')
  const b = (body ?? {}) as Record<string, unknown>
  const route = `${method} ${path}`
  let m: RegExpMatchArray | null

  if (route === 'GET /api/session') return { csrfToken: MOCK_CSRF, version: '0.1.0-mock', agents: clone(AGENTS) }
  if (route === 'GET /api/conversations') {
    return clone([...state.convs].sort((a, c) => c.updatedAt.localeCompare(a.updatedAt)).map(summary))
  }
  if (route === 'POST /api/conversations') {
    return createConversation(b as { agent: AgentName; workdir: string; text: string })
  }
  if ((m = path.match(/^\/api\/conversations\/([^/]+)$/)) && method === 'GET') return clone(findConv(decodeURIComponent(m[1] as string)))
  if ((m = path.match(/^\/api\/conversations\/([^/]+)\/messages$/)) && method === 'POST') {
    sendMessage(decodeURIComponent(m[1] as string), String(b.text ?? ''))
    return undefined
  }
  if ((m = path.match(/^\/api\/conversations\/([^/]+)\/stop$/)) && method === 'POST') {
    stop(decodeURIComponent(m[1] as string))
    return undefined
  }
  if ((m = path.match(/^\/api\/approvals\/([^/]+)$/)) && method === 'POST') {
    return decide(decodeURIComponent(m[1] as string), b.decision as Decision)
  }
  if (route === 'GET /api/connections') return clone(state.connections)
  if (route === 'POST /api/connections/scan') {
    state.connections.scannedAt = now()
    emit({ type: 'connections' })
    return clone(state.connections)
  }
  if ((m = path.match(/^\/api\/connections\/([^/]+)\/rule$/)) && method === 'PUT') {
    return setRule(decodeURIComponent(m[1] as string), b.rule as Rule)
  }
  throw new ApiError(404, 'not_found')
}
