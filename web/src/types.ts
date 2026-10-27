// The wire types from docs/api.md. Keep this file in step with that document.

export type AgentName = 'claude' | 'codex'
export interface Agent {
  name: AgentName
  label: string
  version: string
  available: boolean
}

export type Action = 'read' | 'create' | 'change' | 'send' | 'delete' | 'unknown'
export type Tag = 'live' | 'irreversible' | 'money' | 'people'
export type StepStatus = 'running' | 'done' | 'failed' | 'waiting' | 'denied' | 'outcome_unknown'
export type ConvStatus = 'idle' | 'running' | 'waiting' | 'done' | 'failed' | 'interrupted'

export interface Approval {
  id: string
  state: 'pending' | 'approved' | 'denied' | 'expired'
  reason: string
  lossTitle?: string
  loss?: string[]
  warning?: string
  yesLabel?: string
  noLabel?: string
  requestedAt: string
  decidedAt?: string
}

export type Via = 'cli' | 'mcp' | 'api' | 'file' | 'tool'

export interface Step {
  id: string
  seq: number
  kind: 'cloud' | 'local'
  provider: string
  product: string
  productSub?: string
  action: Action
  tags: Tag[]
  title: string
  titleSource: 'adapter' | 'agent' | 'generic'
  resource: string
  facts: [string, string][]
  via: Via
  recognizedBy: ('builtin' | 'generated' | 'mcp-hints' | 'generic' | 'local')[]
  generic: boolean
  guessWord?: string
  command: string
  api: string
  why: string
  status: StepStatus
  startedAt: string
  endedAt?: string
  durationMs?: number
  output: string
  outputTruncated: boolean
  maskedNote?: string
  uses: { fromStepId: string; what: string }[]
  approval?: Approval
}

export type ChatItem =
  | { id: string; type: 'user'; text: string; at: string }
  | { id: string; type: 'agent'; text: string; at: string }
  | { id: string; type: 'steps'; stepIds: string[]; at: string }
  | { id: string; type: 'approval'; stepId: string; at: string }
  | { id: string; type: 'notice'; level: 'info' | 'warning' | 'error'; text: string; at: string }

export interface RunResult {
  title: string
  line: string
  at: string
}

export interface ConversationSummary {
  id: string
  title: string
  agent: AgentName
  model: string
  workdir: string
  status: ConvStatus
  updatedAt: string
  stepCount: number
  waitingCount: number
}

export interface ConversationDetail extends ConversationSummary {
  subtitle: string
  chat: ChatItem[]
  steps: Step[]
  result?: RunResult
}

export type AdapterKind = 'builtin' | 'generated' | 'hints' | 'generic'
export type Rule = 'irreversible' | 'live' | 'every' | 'readonly' | 'off'
export interface Connection {
  id: string
  kind: 'cli' | 'mcp'
  name: string
  mark: string
  how: string
  account: string
  detail: string
  adapter: AdapterKind
  adapterNote?: string
  rule: Rule
  supported: boolean
}

export interface Session {
  csrfToken: string
  version: string
  agents: Agent[]
}

export interface ConnectionsResponse {
  scannedAt: string
  cli: Connection[]
  mcp: Connection[]
}

export type Decision = 'approve' | 'deny'

export type StreamEvent =
  | { type: 'conversation'; conversationId: string }
  | { type: 'conversations' }
  | { type: 'connections' }
