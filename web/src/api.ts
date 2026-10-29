import type {
  ConnectionsResponse,
  Connection,
  ConversationDetail,
  ConversationSummary,
  Decision,
  Project,
  ProjectInput,
  Rule,
  Session,
  Settings,
  Step,
} from './types'

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  constructor(status: number, code: string) {
    super(code)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE'

// No sign-in (D-063): the server only checks that writes come from its own page (the Origin header the browser adds).
async function call<T>(method: Method, path: string, body?: unknown): Promise<T> {
  // The check sits right at the import so a production build drops the mock code entirely.
  if (import.meta.env.VITE_MOCK === '1') {
    const { mockRequest } = await import('./mock/mockApi')
    return (await mockRequest({ method, path, body })) as T
  }
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  if (!res.ok) {
    let code = `http_${res.status}`
    try {
      const data = (await res.json()) as { error?: unknown }
      if (typeof data.error === 'string') code = data.error
    } catch {
      // The body was not JSON; the status code is enough.
    }
    throw new ApiError(res.status, code)
  }
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

const enc = encodeURIComponent

export const api = {
  session: () => call<Session>('GET', '/api/session'),
  conversations: () => call<ConversationSummary[]>('GET', '/api/conversations'),
  conversation: (id: string) => call<ConversationDetail>('GET', `/api/conversations/${enc(id)}`),
  /** The agent comes from Settings; the folders from the project, or from Settings outside a project. */
  createConversation: (input: { text: string; projectId?: string }) => call<{ id: string }>('POST', '/api/conversations', input),
  sendMessage: (id: string, text: string) => call<void>('POST', `/api/conversations/${enc(id)}/messages`, { text }),
  stop: (id: string) => call<void>('POST', `/api/conversations/${enc(id)}/stop`),
  /** Stops a running command, such as a log tail (D-065). */
  stopStep: (stepId: string) => call<void>('POST', `/api/steps/${enc(stepId)}/stop`),
  decide: (approvalId: string, decision: Decision) =>
    call<Step>('POST', `/api/approvals/${enc(approvalId)}`, { decision }),
  settings: () => call<Settings>('GET', '/api/settings'),
  saveSettings: (input: Settings) => call<Settings>('PUT', '/api/settings', input),
  /** Opens the computer's folder chooser (D-058). Waits until a folder is picked or the chooser is closed. */
  pickFolder: (start?: string) => call<{ path?: string; cancelled?: boolean }>('POST', '/api/pick-folder', { start: start ?? '' }),
  projects: () => call<Project[]>('GET', '/api/projects'),
  createProject: (input: ProjectInput) => call<Project>('POST', '/api/projects', input),
  updateProject: (id: string, input: ProjectInput) => call<Project>('PUT', `/api/projects/${enc(id)}`, input),
  deleteProject: (id: string) => call<void>('DELETE', `/api/projects/${enc(id)}`),
  connections: () => call<ConnectionsResponse>('GET', '/api/connections'),
  scanConnections: () => call<ConnectionsResponse>('POST', '/api/connections/scan'),
  setRule: (id: string, rule: Rule) => call<Connection>('PUT', `/api/connections/${enc(id)}/rule`, { rule }),
}
