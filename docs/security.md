# CloudBuddy safety model and threat model

CloudBuddy's job is to make sure an AI agent can't do anything irreversible to your cloud accounts without you seeing it and saying yes. This page says how, and where that promise ends. The full reasoning, with every decision and review finding, is in `plan_and_track.md`.

## What CloudBuddy protects against

**The agent itself going wrong:** a mistake, a misunderstanding, or a prompt-injection attack that turns it against you. The agent is treated as untrusted; CloudBuddy and you are trusted.

It protects against:
- **Irreversible cloud actions without approval:** deletes, moving money, sending to people, live-mode changes, and running project or third-party code with network.
- **The agent switching the gate off:** editing agent settings, calling CloudBuddy's own port, or approving its own request.
- **Web pages you visit** reaching CloudBuddy on localhost.
- **Secrets leaking into CloudBuddy's records** (known formats only).

## How each path is covered

| Path | Claude Code | Codex |
|---|---|---|
| Shell commands, no network | Claude Code's sandbox: no network, writes only in the project | Codex sandbox (`workspace-write`, network off) |
| Shell commands that need network | PreToolUse hook (`onFailure: block`) decides, then `cloudbuddy exec` runs the exact released command under CloudBuddy's macOS sandbox profile | Codex asks per command; CloudBuddy accepts only plain command approvals |
| MCP tool calls | Only through CloudBuddy's gateway (`--strict-mcp-config`) | Only through the gateway (isolated `CODEX_HOME`) |
| File edits | Hook; files outside the project ask; CloudBuddy's files and agent settings are denied | Codex approval for writes outside the workspace |
| Unknown tools | Asked or denied; the tool set is restricted (`--tools`) | Unknown request types declined |
| User or project settings | Not loaded (`--setting-sources ""`) | Not loaded (own `CODEX_HOME`, only the login is linked) |

## The rules (in order)

1. Anything aimed at CloudBuddy itself or the agents' settings: **refused**.
2. Background or detached work with network: **refused**.
3. Local work in the no-network sandbox: runs.
4. Connection set to Off: refused. Read-only: only reviewed reads run.
5. **Deletes ask.**
6. **Can't undo, Moves money, Reaches people ask.**
7. **Live mode asks.**
8. **Project or third-party code with network asks** (install scripts, `cdk deploy`, `terraform apply`), with a warning that CloudBuddy can't see inside.
9. A change where CloudBuddy can't tell the account asks.
10. "Ask before every change" asks.
11. Anything only guessed, opaque, or using files outside the project folder asks. That includes shell tricks, nested shells, unusual environment variables, curl config files and uploads.
12. **Only reviewed rules run without asking:**
    - reads that a reviewed rule or reviewed read verb covers (not ones that mint credentials or reveal secrets);
    - creates and changes that a hand-written adapter rule marks as reviewed: reversible, no money, no people.

    Everything else asks, even if CloudBuddy can name it.

**The project folders.** A chat outside a project has one folder. A chat in a project (D-054) has the project's folders, as they were when the chat started. All of them count as the project: the agent may change files in any of them (Claude Code gets `--add-dir`, Codex gets them as writable roots), uploads from them don't count as "files outside the project", and approved code with network may write in all of them. While approved code runs, every one of the chat's folders is locked, so no new step is decided until it finishes. CloudBuddy's own folder and the agents' settings stay refused inside a project folder too.

Approvals are **single use**. Each is tied to the exact command and the hash of the files it runs, consumed once, and cancelled if the run ends. Nothing is ever approved by waiting.

## Fail closed

- A hook that crashes, can't start, or times out blocks the call.
- If CloudBuddy is unreachable, nothing runs.
- If CloudBuddy restarts, pending approvals expire and running steps are marked "outcome unknown". They are never retried automatically.

## Local security

- The server listens on `127.0.0.1` only. It checks the `Host` header exactly (this blocks DNS rebinding) and sends no CORS headers.
- **Browser sign-in:**
  - CloudBuddy prints a one-time launch link that expires after 60 seconds.
  - The link becomes an HttpOnly, `SameSite=Strict` cookie.
  - Every change also needs a CSRF header and a matching `Origin`.
- **Helper programs** (hook, exec wrapper, MCP gateway) use a separate secret. It can submit requests, but it can't approve anything.
- The frontend is served with a strict Content Security Policy, and all agent and tool text is shown as plain text.

## Known limits (accepted risks)

- **Code you approve to run with network.** Approved code can do anything those credentials allow: an install script, a build, `cdk deploy`, `terraform apply`. CloudBuddy still asks first and shows a warning. It keeps its own files, your agent settings and config files protected, kills processes the command leaves behind (best effort on macOS), and refuses to run the command if its files changed after you approved it. But it can't see or stop what that code does in the cloud.
- **Inputs a script reads without naming them** can change between your approval and the run.
- **Linux:** `cloudbuddy exec`'s file rules use macOS `sandbox-exec`. On Linux, approved project code with network is refused, and plain cloud CLI calls run without CloudBuddy's file rules (D-042).
- **Accounts:** CloudBuddy works out the account from the command (`--profile`, environment variables). The CLI's own default is shown in Connections but not re-checked on every call.
- **Masking** hides known secret formats (AWS, Stripe, GitHub, JWTs, private keys, Authorization headers, secret-looking variables). Other formats can get through.
- **Outside the agent:** software already running on your computer, other people, and other tools can change your cloud without CloudBuddy seeing it.
- **MCP servers reached by URL** (often OAuth) aren't supported yet. Only stdio servers run through the gateway.

## Reporting a security problem

Please open a private security advisory on the repository rather than a public issue.
