# CloudBuddy safety model and threat model

CloudBuddy lets an AI agent work in your projects the way Claude Code does in your own terminal, shows every step it takes as a card, and stops for your yes before anything that can't be undone. This page says how, and where that promise ends.

## What CloudBuddy protects against

**The agent itself going wrong:** a mistake, a misunderstanding, or a prompt-injection attack that turns it against you.

It protects against:
- **Irreversible cloud actions without your yes:** deletes, moving money, sending to people, and changes in live mode, including when the agent hides them in a compound command, a wrapper or a script that calls a cloud CLI.
- **The agent switching the gate off:** editing agent settings, calling CloudBuddy's own port, or approving its own request.
- **Web pages you visit** reaching CloudBuddy on localhost.
- **Secrets leaking into CloudBuddy's records** (known formats only).

It does **not** try to stop what a normal terminal allows: reading your files, sending data to websites, running project code with network. CloudBuddy shows those steps; it doesn't block them (D-065).

## How each path is covered

| Path | Claude Code |
|---|---|
| Shell commands | The PreToolUse hook (`onFailure: block`) decides every command. `cloudbuddy exec` then runs the exact released command with network, your login-shell environment and the folder's direnv setup, under a macOS sandbox profile (see "What commands can't do" below). |
| Cloud CLIs inside scripts | Shims for the installed cloud CLIs (`aws`, `supabase`, `stripe`, `gh`, …) come first on every command's `PATH`. Each call a script makes shows as its own card and asks first if it can't be undone. |
| Where a command went | A small proxy per command records the hosts it talked to ("Talked to api.supabase.com ×4"). It never decrypts anything and never connects to this computer. |
| MCP tool calls | Your own MCP servers from `~/.claude.json` load (URL and OAuth ones too); the hook decides every call. A project's `.mcp.json` and the claude.ai connectors aren't loaded. |
| File edits | Hook; CloudBuddy's files and the agents' settings are refused. |
| Background commands | Run like in a terminal (log tails); the card stays live until the command ends, and **Stop** ends it. A run's commands stop when the run ends. |
| User or project settings | Not loaded (`--setting-sources ""`), so they can't switch the hook off. Claude Code starts with your login environment only; the folder's direnv changes reach commands, not Claude Code itself. |

## What commands can't do

Commands run like in your terminal, with these exceptions. They keep the agent from changing the gate, or from getting code run later outside the sandbox (two Codex reviews of D-065):
- read or change CloudBuddy's folder, or reach CloudBuddy's port or its database port;
- change the agents' folders and settings (`~/.claude`, `~/.codex`, `~/.claude.json`) or read their logins;
- change any folder on your PATH, the folders of `claude`, `direnv` and your cloud CLIs, or direnv's list of allowed `.envrc` files;
- change shell startup files, LaunchAgents, global git config, or Terminal and iTerm profiles;
- run `open`, `osascript`, `launchctl`, `crontab`, `at`, `screen`, `tmux` or `defaults`, send Apple events, or connect to the control sockets of tmux, screen, cmux, iTerm, WezTerm or Zellij.

The folder's `.envrc` itself runs inside the same sandbox when CloudBuddy loads it.

Codex is paused for now (D-066).

## The rules (in order)

1. Anything aimed at CloudBuddy itself or the agents' settings: **refused**.
2. Connection set to **Off**: refused.
3. Connection set to **Read-only**: only reviewed reads run.
4. **Deletes ask.** That includes a delete CloudBuddy can't read exactly: a command with shell tricks that reaches a cloud CLI and names a delete, a project program that calls a cloud CLI with a delete (`./env aws s3 rb …`, `pnpm exec wrangler … DROP TABLE`), and a script or make target whose name says delete (`make db-drop`, `./scripts/delete-prod.sh`).
5. **Can't undo, Moves money, Reaches people ask.**
6. **Changes in live mode ask.** Reads in live mode run.
7. "Ask before every change" asks before anything but a read.
8. **Everything else runs**, as it would in your terminal.

Approvals are **single use**. Each is tied to the exact command (and, for approved project code, the hash of the files it runs), consumed once, and cancelled if the run ends. Nothing is ever approved by waiting.

## Dashboards (D-067)

A chat can save a **dashboard**: commands plus tiles. CloudBuddy then reruns the commands itself on a timer, with nobody watching, so the rules are stricter than in a chat. CloudBuddy decides again on **every run**:

1. **Plain shell only.** CloudBuddy parses dashboard commands itself, with a small grammar: words, quotes, `$NAME` variables, `|`, `&&`, `;`, and only the redirections `2>/dev/null`, `2>&1` and `>&2`. A command with anything else (`$(…)`, `${…}`, backticks, globs, `~`, subshells, redirections to files, zsh glob qualifiers like `*(e:…:)`, several lines) **never runs**: CloudBuddy can't tell what it runs or hold it to your OK. Bigger steps go in a script file in the project. (Three Codex review rounds kept finding shell syntax that a guess-based reader got wrong; the grammar replaced the guesses.)
2. **Never, not even with your OK:** anything aimed at CloudBuddy itself, background commands, a connection turned **Off**, deletes, Can't undo, Moves money, Reaches people, sends, changes in live mode, and database queries that change data (`DELETE`, `UPDATE`, `DROP`, `SELECT … INTO`, `COPY`…), however the query is written.
3. **Reads run by themselves:** every piece of the command must be a read CloudBuddy recognizes: a cloud CLI read, a text tool on project files (`cat`, `grep`, `wc`, `jq`…, without options that write or print the environment, and without variables, which could hold an option), or one database read: `psql -X` with one `SELECT` in `-c` (CloudBuddy runs it with `default_transaction_read_only=on` and no psqlrc, not through a service file or a connection string that sets options), or `sqlite3 -readonly -safe` with one `SELECT` (safe mode turns off `writefile()` and `.shell`).
4. **Everything else waits for your OK once** on the dashboard page: scripts and programs, web requests, programs that live in the project folder, anything CloudBuddy doesn't know only reads. The OK is for exactly what the page showed (the command, its folder and the hash of the files it runs, as of the last refresh); if the agent changed it or the files changed since, nothing is allowed. It is checked again right before each run, and asks again when anything changes.
5. A chat's agent can only see and change dashboards it built or that belong to its project's folders, and has no tool to delete one. **Pause** and **Delete** stop a refresh that is running. A restart stops dashboard commands an earlier CloudBuddy left running.

Dashboard commands run like chat commands: your login environment plus the folder's direnv setup, `zsh -f`, the same sandbox profile, a time limit (60 seconds unless the dashboard says otherwise, 5 minutes at most), and output masked and capped before it is stored. Inside a command, the CLI shims stop any call that would ask in a chat, because nobody is there to answer. Numbers are kept for 30 days.

Limits: an allowed script can still do things CloudBuddy can't see (SDK calls, full-path CLIs, files it imports that the command doesn't name), every time it runs, like any approved project code. Approvals for dashboards last until the command or its files change, unlike a chat's single-use approvals.

## Fail closed

- A hook that crashes, can't start, or times out blocks the call.
- If CloudBuddy is unreachable, nothing runs.
- If CloudBuddy restarts, pending approvals expire and running steps are marked "outcome unknown". They are never retried automatically.
- A shim that can't reach CloudBuddy lets the call run with a warning; only a "no" stops it. The shims are a view into scripts, not a wall.

## Local security

- The server listens on `127.0.0.1` only. It checks the `Host` header exactly (this blocks DNS rebinding) and sends no CORS headers.
- **No browser sign-in** (D-063). Every change needs a matching `Origin`, so other websites can't approve or change anything. Other programs running on your computer could call the API. The agent can't: its commands can't reach CloudBuddy's port, directly or through the network-log proxy, or its data folder.
- **Helper programs** (hook, exec wrapper) use a separate secret, and the shims a per-command token on a local socket. Neither can approve anything.
- The frontend is served with a strict Content Security Policy, and all agent and tool text is shown as plain text.

## Known limits (accepted risks)

- **What CloudBuddy can't see, it can't stop.** A script that calls a cloud API through an SDK or by full path, or a CLI the project runs from `node_modules`, shows as one card with the hosts it talked to, not each operation. If it deletes something, CloudBuddy only asks when the script's name or command says so. This is the trade-off for working like a terminal (D-065).
- **Data can leave like in a terminal.** Commands can read your files and send them anywhere.
- **A deliberately hostile agent may still find a way out.** The sandbox lists what it blocks; macOS has more ways to run code outside it than any list. Two that stay open on purpose: **Docker** (a container can mount any folder, but blocking it would break container builds and local development) and **ssh to this computer** (only if Remote Login is on). A project's own `.git` folder also stays writable (`git push -u` writes it), so a command can leave a git hook that runs later, outside the sandbox. Since D-063 approvals need no secret, so anything that gets out could approve requests. Pairing the browser once (a secret only your browser holds) would close this; it's the user's call (D-065 work log).
- **Accounts:** CloudBuddy works out the account from the command and the environment (`--profile`, `AWS_PROFILE`, direnv). A config file can still switch it.
- **Linux:** `cloudbuddy exec`'s file rules use macOS `sandbox-exec`. On Linux commands run without them (D-042).
- **Masking** hides known secret formats (AWS, Stripe, GitHub, JWTs, private keys, Authorization headers, secret-looking variables). Other formats can get through.
- **Outside the agent:** software already running on your computer, other people, and other tools can change your cloud without CloudBuddy seeing it.

## Reporting a security problem

Please open a private security advisory on the repository rather than a public issue.
