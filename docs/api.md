# CloudBuddy HTTP API (v0)

The contract between the Go server (`internal/server`) and the React app (`web/`).
All JSON. Times are RFC 3339 strings. IDs are strings.

## Access

- The server listens on `127.0.0.1:7420`.
- On start it opens the browser at `http://127.0.0.1:7420/`, the landing page. The app itself is at `/app` (D-060); any other path serves the same `index.html`, and the page picks what to show from the path.
- No sign-in, cookie or token (D-063). Requests must name the server's own host (`Host` is checked exactly, which blocks DNS rebinding), and every non-GET request must carry a matching `Origin`, so other websites can't change anything.
- `GET /api/session` returns `{ version, agents: Agent[] }`.
- In dev, Vite runs on `127.0.0.1:5173` and proxies `/api` and `/stream` to `127.0.0.1:7420`. The server accepts both origins only when started with `--dev`.

## Types

```ts
type AgentName = "claude" | "codex";
interface Agent { name: AgentName; label: string; version: string; available: boolean; }

type Action = "read" | "create" | "change" | "send" | "delete" | "unknown";
type Tag = "live" | "irreversible" | "money" | "people";
type StepStatus = "running" | "done" | "failed" | "waiting" | "denied" | "outcome_unknown";
type ConvStatus = "idle" | "running" | "waiting" | "done" | "failed" | "interrupted";

interface Approval {
  id: string;
  state: "pending" | "approved" | "denied" | "expired";
  reason: string;              // "Why CloudBuddy asked", plain text
  lossTitle?: string;          // e.g. "What will be lost" / "What will happen"
  loss?: string[];
  warning?: string;            // kind-B warning ("CloudBuddy can't see what this does inside…")
  yesLabel?: string;           // button text, e.g. "Delete", "Send", "Run it" (default "Approve")
  noLabel?: string;            // e.g. "Keep it", "Not yet" (default "Deny")
  requestedAt: string;
  decidedAt?: string;
}

interface Step {
  id: string;
  seq: number;                 // 1-based position in the conversation
  kind: "cloud" | "local";     // only "cloud" steps go on the canvas
  provider: string;            // "AWS", "Stripe", "Local", "Unknown", …
  product: string;             // "S3", "Prices", "Shell", …
  productSub?: string;         // short sub-label for single-provider lanes ("Storage")
  action: Action;
  tags: Tag[];
  title: string;
  titleSource: "adapter" | "agent" | "generic";
  resource: string;
  facts: [string, string][];
  via: "cli" | "mcp" | "api" | "file" | "tool";
  recognizedBy: ("builtin" | "generated" | "mcp-hints" | "generic" | "local")[];
  generic: boolean;            // true when no adapter matched
  guessWord?: string;          // the word the generic layer keyed on
  command: string;             // command line or tool call, masked
  api: string;                 // "s3:DeleteBucket", "resend · send_broadcast", …
  why: string;                 // the agent's own description of the step
  status: StepStatus;
  startedAt: string;
  endedAt?: string;
  durationMs?: number;
  output: string;              // masked, capped
  outputTruncated: boolean;
  maskedNote?: string;         // e.g. "CloudBuddy hid a secret before saving this output."
  uses: { fromStepId: string; what: string }[];  // the earlier step where each value first appeared
  approval?: Approval;
}

type ChatItem =
  | { id: string; type: "user"; text: string; at: string }
  | { id: string; type: "agent"; text: string; at: string }
  | { id: string; type: "steps"; stepIds: string[]; at: string }
  | { id: string; type: "approval"; stepId: string; at: string }
  | { id: string; type: "notice"; level: "info" | "warning" | "error"; text: string; at: string };

interface RunResult { title: string; line: string; at: string; }

interface ConversationSummary {
  id: string; title: string; agent: AgentName; model: string;
  workdir: string;             // where the agent starts
  projectId?: string;          // missing outside a project
  status: ConvStatus; updatedAt: string; stepCount: number; waitingCount: number;
}

interface ConversationDetail extends ConversationSummary {
  extraFolders: string[];      // the project's other folders when the chat started; the agent may change files there too
  subtitle: string;            // e.g. "4 providers · Resend, Cloudflare, Stripe, Google Cloud"
  chat: ChatItem[];
  steps: Step[];
  result?: RunResult;
}

// D-054. The first folder is where the agent starts. Folders are full paths, resolved by the server.
interface Project { id: string; name: string; folders: string[]; chatCount: number; updatedAt: string; }

// D-055. The agent for new chats, and the folder for chats outside a project ("" = not set).
interface Settings { agent: AgentName; folder: string; }

type AdapterKind = "builtin" | "generated" | "hints" | "generic";
type Rule = "irreversible" | "live" | "every" | "readonly" | "off";
interface Connection {
  id: string; kind: "cli" | "mcp"; name: string; mark: string;   // mark: 2-letter tile text
  how: string;                 // "aws 2.34.61" or "MCP · local · 7 tools"
  account: string; detail: string;
  adapter: AdapterKind; adapterNote?: string;
  rule: Rule; supported: boolean;
}
```

### Dashboards (D-067)

A chat's agent saves a dashboard through CloudBuddy's own MCP tools (`cloudbuddy dashboards-mcp`: `save_dashboard`, `list_dashboards`, `get_dashboard`). After that CloudBuddy reruns its commands on a timer; the browser only reads it, changes its schedule, answers for commands that aren't reads, and deletes it. The full shapes are in `web/src/types.ts`.

```ts
interface Tile {
  key: string; type: "number" | "line" | "bars" | "table" | "text" | "status"; title: string;
  source?: string; path?: string;          // which command, and where the value is in its output
  width: number;                           // 1–4 of a 4-column grid
  format?: "number" | "currency" | "percent" | "bytes" | "duration"; unit?: string; good?: "up" | "down"; …
}
interface TileValue {                      // what a tile shows now, worked out by the server
  key: string; state: "ok" | "pending" | "waiting" | "blocked" | "declined" | "error";
  message?: string; stale?: boolean;       // stale: the last refresh failed, this is the last good value
  number?: number; compare?: { value, at, label }; spark?: Point[];
  series?: { label, points?: Point[], bars?: Bar[] }[]; bars?: Bar[];
  rows?: object[]; columns?: TileColumn[]; more?: number; text?: string; status?: "ok" | "warn" | "fail";
}
interface DashboardSource {                // one command
  key: string; label?: string; command: string; cwd: string; format: string;
  verdict: "" | "read" | "needs_ok" | "never"; reason?: string;
  approval?: { allowed: boolean; decidedAt: string; changed?: boolean };
  lastRun?: { state: "ok" | "failed" | "waiting" | "blocked" | "declined"; exitCode?: number; output: string; note?: string; at: string; durationMs: number };
}
interface DashboardSummary {
  id: string; title: string; description: string; conversationId?: string;
  refresh: string; paused: boolean; running: boolean; lastRunAt?: string; nextRunAt?: string; updatedAt: string;
  needsOk: number; failing: number; tileCount: number; headlines: { tile: Tile; value: TileValue }[];
}
interface DashboardDetail extends DashboardSummary {
  workdir: string; range: string; tiles: Tile[]; values: TileValue[]; sources: DashboardSource[];
  refreshChoices: string[]; rangeChoices: { key: string; label: string }[];
}
```

## Endpoints

| Method | Path | Body | Returns |
|---|---|---|---|
| GET | `/api/session` | | `{ version, agents }` |
| GET | `/api/conversations` | | `ConversationSummary[]` (newest first) |
| POST | `/api/conversations` | `{ text, projectId? }` | `{ id }` (starts the first run). The agent comes from Settings. With `projectId`, the folders are the project's; without it, the folder from Settings. `400 {"error":"no_folder"}` when there is neither. `agent` and `workdir` may also be given, to override Settings (tests); `workdir` together with `projectId` is a `400`. |
| GET | `/api/conversations/:id` | | `ConversationDetail` |
| POST | `/api/conversations/:id/messages` | `{ text }` | `202` (starts a run) / `409` if a run is active, or with a message when the chat uses Codex, which is coming soon (D-066) |
| POST | `/api/conversations/:id/stop` | | `202` |
| POST | `/api/approvals/:id` | `{ decision: "approve" \| "deny" }` | `200 Step` / `409 {"error":"not_pending"}` |
| POST | `/api/steps/:id/stop` | | `202` stops the step's running command, such as a log tail (D-065) / `409 {"error":"not_running"}` |
| GET | `/api/settings` | | `Settings` |
| PUT | `/api/settings` | `Settings` | `Settings` (the folder resolved) / `400` with a message for people |
| POST | `/api/pick-folder` | `{ start? }` | Opens the computer's own folder chooser (D-058) and waits: `{ path }`, or `{ cancelled: true }`. `409 busy` while one is open; `501 no_picker` when this computer has none (Linux without zenity or kdialog). Nothing is saved; the path is checked where it is used. |
| GET | `/api/projects` | | `Project[]` (by name) |
| POST | `/api/projects` | `{ name, folders }` | `Project` / `400` with a message for people |
| PUT | `/api/projects/:id` | `{ name, folders }` | `Project`. Chats already in the project keep their folders. |
| DELETE | `/api/projects/:id` | | `204`. Its chats stay, outside any project. |
| GET | `/api/connections` | | `{ scannedAt, cli: Connection[], mcp: Connection[] }` |
| POST | `/api/connections/scan` | | same as GET |
| PUT | `/api/connections/:id/rule` | `{ rule }` | `Connection` |
| GET | `/api/dashboards` | | `DashboardSummary[]` (latest changed first) |
| GET | `/api/dashboards/:id?range=1h\|24h\|7d\|30d` | | `DashboardDetail`, charts over the range (default 24h) / `404` |
| PUT | `/api/dashboards/:id/schedule` | `{ refresh, paused }` | `DashboardDetail` / `400` with a message (refresh from 1m to 1d) |
| POST | `/api/dashboards/:id/refresh` | | `202`, runs every command once now |
| POST | `/api/dashboards/:id/sources/:key/decision` | `{ decision: "allow" \| "deny" }` | `DashboardDetail`. Records the user's answer for a command that isn't a read, tied to the command, its folder and its files. It can't make a command that can never run from a dashboard run. |
| DELETE | `/api/dashboards/:id` | | `204`; its history goes too |
| GET | `/stream` | | Server-Sent Events, see below |

## Live updates (`GET /stream`)

Each SSE message is one JSON object in `data:`:

```ts
type StreamEvent =
  | { type: "conversation"; conversationId: string }   // refetch that conversation + the list
  | { type: "conversations" }                           // refetch the list
  | { type: "connections" }                             // refetch connections
  | { type: "projects" }                                // refetch projects
  | { type: "settings" }                                // refetch settings
  | { type: "dashboards" }                              // refetch the dashboards list
  | { type: "dashboard"; dashboardId: string };         // refetch that dashboard + the list
```

The app treats every event as "refetch this" (TanStack Query invalidation). The server sends a comment ping every 15 seconds.

## Rendering rules
- All text from agents and tools is rendered as **plain text**. No `dangerouslySetInnerHTML`, no Markdown-to-HTML on tool output. Agent chat messages are formatted as Markdown by CloudBuddy's own parser (`web/src/markdown.ts`), which builds React elements only: raw HTML shows as text, images show as links, and only `http:`/`https:` links become links (opened in a new tab with `rel="noopener noreferrer"`).
- Approve/Deny buttons call `POST /api/approvals/:id`. A `409` means it was already decided or expired: refetch.
