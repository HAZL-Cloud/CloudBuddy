// Package agent runs the agent CLIs (Claude Code, Codex) and turns their
// output into a small set of events. Gate decisions don't happen here: Claude
// Code asks through its hook (HTTP), Codex through the Approver callback.
package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Event is one normalized thing an agent did.
type Event struct {
	Kind      string // session | text | tool_start | tool_end | done | error
	SessionID string
	Model     string
	Text      string
	ToolKey   string // the agent's id for the tool call
	ToolName  string // Bash, Read, mcp__x__y, shell, file_change, …
	Input     map[string]any
	Command   string // shell command (Codex)
	Paths     []string
	Output    string
	IsError   bool
	Status    string // done: completed | failed | interrupted
	Network   bool   // Codex: ran outside the sandbox after approval
}

// Approver answers Codex's server requests (approvals, elicitations).
type Approver func(ctx context.Context, method string, params json.RawMessage) (any, error)

type StartOptions struct {
	RunID     string
	Workdir   string
	Folders   []string // the project's other folders: the agent may read and write them too
	Prompt    string
	SessionID string // continue this agent session
	Model     string
	RunDir    string            // per-run folder for settings files
	BaseEnv   map[string]string // the environment to start from (D-065); empty means CloudBuddy's own
	Env       map[string]string // added to the agent's environment
	MCP       map[string]MCPSpec
	// MCPServers are the user's own MCP servers, passed to Claude Code as they
	// are (stdio, http, sse); its hook gates every call to them (D-065).
	MCPServers map[string]any
	Brief      string
	Approve    Approver
}

// MCPSpec is how an agent starts one MCP server (always the CloudBuddy gateway).
type MCPSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

type Driver interface {
	Name() string
	// Run starts the agent, sends events to out, and returns when it exits.
	Run(ctx context.Context, opts StartOptions, out chan<- Event) error
}

// Brief is the instruction given to every agent (plan section 4.3). It is
// guidance; the gate and sandbox enforce what matters.
const Brief = `You are running inside CloudBuddy, which shows every step you take as a card and asks the user before anything that can't be undone (deletes, payments, messages to people, changes in live mode).
- Work as you would in the user's own terminal: commands have network, the project's environment and the user's CLI logins. Use the project's own scripts and make targets when they fit.
- Give every shell command a short plain-English description of what it does; it becomes the card's title.
- Prefer one cloud command per tool call when it's easy, so each shows as its own card.
- If CloudBuddy or the user denies a step, don't try to do the same thing another way. Explain and ask the user.
- Never touch ~/.cloudbuddy, CloudBuddy's port, or the agents' own settings files.`

// baseEnv is the user's environment (base, or CloudBuddy's own when base is
// empty) without variables that would confuse a nested agent, plus extra.
func baseEnv(base, extra map[string]string) []string {
	src := os.Environ()
	if len(base) > 0 {
		src = src[:0:0]
		for k, v := range base {
			src = append(src, k+"="+v)
		}
	}
	var env []string
	for _, kv := range src {
		k := kv[:strings.IndexByte(kv, '=')]
		switch k {
		case "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_CODE_SSE_PORT", "CODEX_HOME":
			continue
		}
		if strings.HasPrefix(k, "CB_") {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// startGroup starts cmd in its own process group so the whole tree can be
// stopped together.
func startGroup(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}

// StopGroup asks the process group to stop, then kills it.
func StopGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 30; i++ {
			if syscall.Kill(-pgid, 0) != nil {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		close(done)
	}()
	<-done
}

// textOf flattens Claude tool_result content (a string or a list of blocks).
func textOf(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var b strings.Builder
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				if s, ok := m["text"].(string); ok {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(s)
				}
			}
		}
		return b.String()
	}
	if v == nil {
		return ""
	}
	bs, _ := json.Marshal(v)
	return string(bs)
}
