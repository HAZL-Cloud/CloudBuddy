import type { IconName } from './components/Icon'
import type { Action, AdapterKind, AgentName, Rule, Tag, Via } from './types'

// D-011: five action types and four warning tags. "unknown" is the actions the server could not classify.
export const ACTIONS: Record<Action, { label: string; icon: IconName; plural: string }> = {
  read: { label: 'Read', icon: 'eye', plural: 'reads' },
  create: { label: 'Create', icon: 'plus', plural: 'creates' },
  change: { label: 'Change', icon: 'pencil', plural: 'changes' },
  send: { label: 'Send', icon: 'plane', plural: 'sends' },
  delete: { label: 'Delete', icon: 'trash', plural: 'deletes' },
  unknown: { label: 'Unknown', icon: 'question', plural: 'unknown actions' },
}

export const TALLY_ORDER: Action[] = ['read', 'create', 'change', 'send', 'delete', 'unknown']

export const TAGS: Record<Tag, { label: string; icon: IconName }> = {
  live: { label: 'Live', icon: 'bolt' },
  irreversible: { label: "Can't undo", icon: 'lock' },
  money: { label: 'Moves money', icon: 'dollar' },
  people: { label: 'Reaches people', icon: 'people' },
}

export const VIA_LABELS: Record<Via, string> = {
  cli: 'CLI',
  mcp: 'MCP',
  api: 'API',
  file: 'File',
  tool: 'Tool',
}

export const ADAPTERS: Record<AdapterKind, { label: string; note: string }> = {
  builtin: { label: 'Built-in adapter', note: 'Hand-tuned titles and risks.' },
  generated: { label: 'Generated adapter', note: "Built from the tool's published command list or API spec." },
  hints: { label: 'MCP hints', note: 'Uses the read and destroy hints the tools declare about themselves.' },
  generic: { label: 'Generic checks', note: "Guesses from the command and asks unless it's clearly read-only." },
}

export const RULES: [Rule, string][] = [
  ['irreversible', "Ask before anything I can't undo"],
  ['live', 'Always ask in live mode'],
  ['every', 'Ask before every change'],
  ['readonly', 'Read-only'],
  ['off', 'Off for all chats'],
]

export const AGENT_LABELS: Record<AgentName, string> = { claude: 'Claude Code', codex: 'Codex' }
