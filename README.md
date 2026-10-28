# CloudBuddy

**See what your AI agent does to your cloud, and stop it before anything you can't undo.**

CloudBuddy is a local web app that runs the AI coding agents already on your computer (Claude Code and Codex) for cloud work: deploying to AWS, setting up Stripe, adding DNS records in Cloudflare, sending email with Resend. Every step the agent takes becomes a card on a timeline, and CloudBuddy **stops the agent before anything that can't be undone**:
- deletes
- moving money
- sending to real people
- live mode

You approve or deny right on the canvas.

![The CloudBuddy canvas](docs/screenshot.jpg)

- **Chat** with the agent, like the Claude desktop app. Its replies are formatted (lists, tables, code blocks you can copy).
- **Projects**: a name and the folders that belong together, such as an app and its infrastructure code. Start a chat in a project and the agent has all of them.
- **Canvas** beside the chat, which you can fold away to give the chat the room: one card per cloud step, left to right in order, one lane per provider. Back-to-back reads stack into one card. A line runs from something a step created to each later step that builds on it; a value found earlier is a chip you can hover to trace.
- **Approvals** before deletes, payments, sends and live-mode changes, with what will be lost and the agent's reason.
- **History**: every chat keeps its canvas. Reopen it, replay it step by step.
- **Connections**: which CLIs and MCP servers the agent can reach, who they are signed in as, and a rule per connection (Off, Read-only, Ask before every change…).

It runs only on your computer (`127.0.0.1`), uses your existing agent logins and cloud CLIs, and never stores your keys.

## Status

`v0.1.0` is the first release. It works on **macOS** with **Claude Code** and **Codex**. Linux runs, but approved project code with network is refused there until the Linux sandbox is built (see [Safety](docs/security.md)). Windows isn't supported yet.

## Quick start

You need: Go 1.24+, Node 20+, Docker, and Claude Code and/or Codex installed and signed in.

```bash
git clone <this repo> cloudbuddy && cd cloudbuddy
make db-up          # PostgreSQL in Docker on 127.0.0.1:54329
make release        # builds the frontend and a single binary: bin/cloudbuddy
bin/cloudbuddy      # opens your browser
```

CloudBuddy prints a one-time link and opens it. The link signs your browser in, works once, and opens the landing page; **Open CloudBuddy** takes you to the app at `/app`. To open CloudBuddy again later, restart it for a new link.

Pick your agent in **Settings**, add a project (its name and folders, picked with **Choose…**) with **+** next to Projects, then click **New chat** and describe the job. The first message starts the chat.

### Try it without an agent

```bash
bin/cloudbuddy --fake testdata/scenarios/demo.json
```

This adds a **Demo agent** that replays a deploy run, so you can try approvals without touching any cloud account.

## How it works

```
 browser ── CloudBuddy (Go, 127.0.0.1:7420) ── claude / codex CLI ── your cloud CLIs and MCP servers
                     │
               gate: recognizer → policy → ask you → release once
```

1. CloudBuddy starts the agent CLI with its own settings:
   - **Claude Code** gets a hook on every tool call, a no-network sandbox, and no user or project settings.
   - **Codex** gets pinned approval settings and its own home folder.
2. Before any tool call runs, the **recognizer** works out what it does:
   - provider, action and identity;
   - warning tags: Can't undo, Moves money, Reaches people, Live.
3. The **policy** decides: run it, ask you, or refuse.
4. Local work (editing files, building) runs inside a sandbox with **no network**.
5. Anything that needs network goes through the gate. Approved commands run through `cloudbuddy exec` under a sandbox profile that protects CloudBuddy's own files, your agent settings and secret folders.
6. MCP servers are reached only through CloudBuddy's MCP gateway, which checks every tool call.

Every decision fails closed: if CloudBuddy can't decide, can't reach you, or crashes, the action doesn't run.

## Adapters

CloudBuddy knows AWS (every service, generated from the AWS CLI models), Stripe, Cloudflare and Resend. Anything else that has a CLI or an MCP server still works: CloudBuddy guesses from the command and asks unless it's clearly read-only.

Adapters are YAML files. See [docs/adapters.md](docs/adapters.md) to add one.

## Safety and its limits

Read [docs/security.md](docs/security.md) before trusting CloudBuddy with production accounts. In short, CloudBuddy can't see inside code you approve to run with network (an install script, `cdk deploy`). It also can't stop cloud changes made outside the agent. Secret masking covers known formats only.

## Development

```bash
make db-up
make build && bin/cloudbuddy serve --dev --no-open   # API on :7420
cd web && npm run dev                                 # UI on :5173 (or npm run dev:mock without a server)
make test                                             # Go and frontend tests
```

- `plan_and_track.md`: the plan, every decision, and the work log.
- `docs/api.md`: the HTTP API between server and browser.
- `spikes/`: the Phase 0 experiments that proved the agent integration.

## License

Apache-2.0. See [LICENSE](LICENSE).
