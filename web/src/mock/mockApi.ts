import { ApiError } from '../api'
import type {
  Agent,
  AgentName,
  ConnectionsResponse,
  ConversationDetail,
  ConversationSummary,
  Decision,
  Project,
  ProjectInput,
  Rule,
  Settings,
  Step,
  StreamEvent,
} from '../types'
import { seedConnections, seedConversations, seedProjects, seedSettings } from './data'

// A stand-in for the Go server, used when VITE_MOCK=1 and by the tests. It answers the same paths with the same
// status codes as docs/api.md, so the real client code (CSRF header, 401, 409) is exercised too.

export const MOCK_CSRF = 'mock-csrf-token'

const AGENTS: Agent[] = [
  { name: 'claude', label: 'Claude Code', version: '2.1.311', available: true },
  { name: 'codex', label: 'Codex', version: '0.163.0', available: true },
]

interface MockState {
  convs: ConversationDetail[]
  projects: Project[]
  settings: Settings
  connections: ConnectionsResponse
  signedIn: boolean
  /** Multiplies every delay. Tests set it to 0. */
  speed: number
  counter: number
  /** What the folder chooser does: picks a folder, is cancelled, or doesn't exist on this computer. */
  picker: 'pick' | 'cancel' | 'none'
  picks: number
}

const state: MockState = {
  convs: seedConversations(),
  projects: seedProjects(),
  settings: seedSettings(),
  connections: seedConnections(),
  signedIn: true,
  speed: 1,
  counter: 0,
  picker: 'pick',
  picks: 0,
}
const timers = new Set<ReturnType<typeof setTimeout>>()
const listeners = new Set<(ev: StreamEvent) => void>()

export function configureMock(opts: { speed?: number; signedIn?: boolean; picker?: MockState['picker'] }): void {
  if (opts.speed !== undefined) state.speed = opts.speed
  if (opts.signedIn !== undefined) state.signedIn = opts.signedIn
  if (opts.picker !== undefined) state.picker = opts.picker
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
  state.projects = seedProjects()
  state.settings = seedSettings()
  state.connections = seedConnections()
  state.signedIn = true
  state.picker = 'pick'
  state.picks = 0
  emit({ type: 'conversations' })
  emit({ type: 'connections' })
  emit({ type: 'projects' })
  emit({ type: 'settings' })
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
  const { id, title, agent, model, workdir, projectId, status, updatedAt, stepCount, waitingCount } = c
  return { id, title, agent, model, workdir, projectId, status, updatedAt, stepCount, waitingCount }
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

// The mock can't look at the disk, so it only checks that a folder is a full path.
function checkFolders(folders: unknown): string[] {
  const list = Array.isArray(folders) ? folders.map((f) => String(f).trim()).filter(Boolean) : []
  const out = [...new Set(list)]
  if (out.length === 0) throw new ApiError(400, 'add at least one folder')
  const bad = out.find((f) => !f.startsWith('/') && !f.startsWith('~/'))
  if (bad) throw new ApiError(400, 'use a full folder path, like /Users/you/code/project')
  if (out.length > 12) throw new ApiError(400, 'a project can have up to 12 folders')
  return out
}

function createConversation(body: { text?: string; projectId?: string }): { id: string } {
  const text = String(body.text ?? '').trim()
  if (!text) throw new ApiError(400, 'the message is empty')
  let folders: string[]
  if (body.projectId) {
    const p = state.projects.find((x) => x.id === body.projectId)
    if (!p) throw new ApiError(400, 'that project no longer exists')
    folders = p.folders
  } else if (state.settings.folder) {
    folders = [state.settings.folder]
  } else {
    throw new ApiError(400, 'no_folder')
  }
  const id = nextId('chat-')
  const c: ConversationDetail = {
    id,
    title: text.length > 60 ? `${text.slice(0, 57)}…` : text,
    agent: state.settings.agent,
    model: '',
    workdir: folders[0] as string,
    projectId: body.projectId || undefined,
    extraFolders: folders.slice(1),
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
  emit({ type: 'projects' })
  replyLater(c, 900)
  return { id }
}

// ---- Projects and settings -------------------------------------------------------------------------------------

function projectList(): Project[] {
  return clone(
    [...state.projects]
      .map((p) => ({ ...p, chatCount: state.convs.filter((c) => c.projectId === p.id).length }))
      .sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase())),
  )
}

function saveProject(id: string | null, body: ProjectInput): Project {
  const name = String(body.name ?? '').trim().replace(/\s+/g, ' ')
  if (!name) throw new ApiError(400, 'give the project a name')
  if (name.length > 80) throw new ApiError(400, 'use a name of 80 characters or fewer')
  const folders = checkFolders(body.folders)
  let p = id ? state.projects.find((x) => x.id === id) : undefined
  if (id && !p) throw new ApiError(404, 'not_found')
  if (!p) {
    p = { id: nextId('pj-'), name, folders, chatCount: 0, updatedAt: now() }
    state.projects.push(p)
  } else {
    Object.assign(p, { name, folders, updatedAt: now() })
  }
  emit({ type: 'projects' })
  const saved = p
  return projectList().find((x) => x.id === saved.id) as Project
}

function deleteProject(id: string): void {
  if (!state.projects.some((p) => p.id === id)) throw new ApiError(404, 'not_found')
  state.projects = state.projects.filter((p) => p.id !== id)
  for (const c of state.convs) if (c.projectId === id) c.projectId = undefined
  emit({ type: 'projects' })
  emit({ type: 'conversations' })
}

function saveSettings(body: Partial<Settings>): Settings {
  if (!AGENTS.some((a) => a.name === body.agent)) throw new ApiError(400, `unknown agent "${String(body.agent)}"`)
  const folder = String(body.folder ?? '').trim()
  if (folder) checkFolders([folder])
  state.settings = { agent: body.agent as AgentName, folder }
  emit({ type: 'settings' })
  return clone(state.settings)
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
  if (route === 'POST /api/conversations') return createConversation(b as { text?: string; projectId?: string })
  if (route === 'GET /api/settings') return clone(state.settings)
  if (route === 'PUT /api/settings') return saveSettings(b as Partial<Settings>)
  // The mock can't open a real chooser, so it "picks" made-up folders in turn.
  if (route === 'POST /api/pick-folder') {
    if (state.picker === 'none') throw new ApiError(501, 'no_picker')
    if (state.picker === 'cancel') return { cancelled: true }
    return { path: `/Users/you/code/${['shop-web', 'shop-infra', 'shop-docs'][state.picks++ % 3]}` }
  }
  if (route === 'GET /api/projects') return projectList()
  if (route === 'POST /api/projects') return saveProject(null, b as unknown as ProjectInput)
  if ((m = path.match(/^\/api\/projects\/([^/]+)$/))) {
    const id = decodeURIComponent(m[1] as string)
    if (method === 'PUT') return saveProject(id, b as unknown as ProjectInput)
    if (method === 'DELETE') return deleteProject(id)
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
