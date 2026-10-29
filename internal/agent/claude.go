package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Claude drives `claude -p` with the launch flags from D-039.
type Claude struct {
	Exe       string // claude, found once at start-up ("" means "claude" on PATH)
	Bin       string // the cloudbuddy binary (hook and exec wrapper)
	DataDir   string
	Home      string
	ExtraDeny []string
}

func (c *Claude) Name() string { return "claude" }

// AllowedTools is the tool set CloudBuddy gives Claude Code: what a native
// session uses for project work, including subagents, skills and watching
// background commands (D-065). Scheduling, remote triggers and worktrees
// stay out; they act outside the chat. Every call still passes the hook.
const AllowedTools = "Bash,Read,Edit,MultiEdit,Write,Glob,Grep,NotebookEdit,WebFetch,WebSearch,TodoWrite,ToolSearch,Task,Skill,Monitor,TaskStop,TaskOutput,BashOutput,KillShell"

// Settings returns the --settings document: the gate hook on every tool,
// the SessionStart canary, and the no-network sandbox whose only way out is
// the cloudbuddy exec wrapper (D-020, D-022, D-036, D-039).
func (c *Claude) Settings() map[string]any {
	deny := []string{c.DataDir,
		filepath.Join(c.Home, ".claude", ".credentials.json"),
		filepath.Join(c.Home, ".codex", "auth.json")}
	deny = append(deny, c.ExtraDeny...)
	return map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{"matcher": "*", "hooks": []any{map[string]any{
				"type": "command", "command": c.Bin, "args": []string{"hook"}, "timeout": 7200, "onFailure": "block"}}}},
			"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{
				"type": "command", "command": c.Bin, "args": []string{"hook", "--event", "session-start"}, "timeout": 30}}}},
		},
		"sandbox": map[string]any{
			"enabled":                  true,
			"autoAllowBashIfSandboxed": true,
			"allowUnsandboxedCommands": false,
			"excludedCommands":         []string{c.Bin + " exec *"},
			"network":                  map[string]any{"allowedDomains": []string{}, "allowLocalBinding": false},
			"filesystem":               map[string]any{"denyRead": deny},
		},
	}
}

func claudeArgs(o StartOptions, settingsPath, mcpPath string) []string {
	args := []string{"-p", o.Prompt,
		"--output-format", "stream-json", "--verbose",
		"--setting-sources", "",
		"--settings", settingsPath,
		"--permission-prompts", "none",
		"--mcp-config", mcpPath,
		"--tools", AllowedTools,
		"--append-system-prompt", o.Brief,
	}
	// One flag per folder: --add-dir takes several values, so a bare list
	// could swallow whatever comes next.
	for _, f := range o.Folders {
		args = append(args, "--add-dir="+f)
	}
	if o.SessionID != "" {
		args = append(args, "--resume", o.SessionID)
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	return args
}

func (c *Claude) Run(ctx context.Context, o StartOptions, out chan<- Event) error {
	settingsPath := filepath.Join(o.RunDir, "claude-settings.json")
	mcpPath := filepath.Join(o.RunDir, "claude-mcp.json")
	if err := writeJSON(settingsPath, c.Settings()); err != nil {
		return err
	}
	servers := map[string]any{}
	for name, s := range o.MCPServers {
		servers[name] = s
	}
	for name, s := range o.MCP {
		servers[name] = map[string]any{"type": "stdio", "command": s.Command, "args": s.Args, "env": s.Env}
	}
	if err := writeJSON(mcpPath, map[string]any{"mcpServers": servers}); err != nil {
		return err
	}
	exe := c.Exe
	if exe == "" {
		exe = "claude"
	}
	cmd := exec.Command(exe, claudeArgs(o, settingsPath, mcpPath)...)
	cmd.Dir = o.Workdir
	cmd.Env = baseEnv(o.BaseEnv, o.Env)
	stderr, _ := os.Create(filepath.Join(o.RunDir, "claude.stderr"))
	defer stderr.Close()
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := startGroup(cmd); err != nil {
		return fmt.Errorf("couldn't start Claude Code: %w", err)
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			StopGroup(cmd)
		case <-stop:
		}
	}()
	defer close(stop)

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	sawResult := false
	for sc.Scan() {
		for _, e := range ParseClaudeLine(sc.Bytes()) {
			if e.Kind == "done" {
				sawResult = true
			}
			out <- e
		}
	}
	err = cmd.Wait()
	if !sawResult {
		status := "failed"
		if ctx.Err() != nil {
			status = "interrupted"
		}
		msg := ""
		if b, _ := os.ReadFile(filepath.Join(o.RunDir, "claude.stderr")); len(b) > 0 {
			msg = tail(string(b), 600)
		}
		out <- Event{Kind: "done", Status: status, Text: msg}
	}
	return err
}

// ParseClaudeLine turns one stream-json line into events.
func ParseClaudeLine(line []byte) []Event {
	var m struct {
		Type      string          `json:"type"`
		Subtype   string          `json:"subtype"`
		SessionID string          `json:"session_id"`
		Model     string          `json:"model"`
		Message   json.RawMessage `json:"message"`
		Result    string          `json:"result"`
		IsError   bool            `json:"is_error"`
	}
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	var evs []Event
	switch m.Type {
	case "system":
		if m.Subtype == "init" {
			evs = append(evs, Event{Kind: "session", SessionID: m.SessionID, Model: m.Model})
		}
	case "assistant", "user":
		var msg struct {
			Content []struct {
				Type      string         `json:"type"`
				Text      string         `json:"text"`
				ID        string         `json:"id"`
				Name      string         `json:"name"`
				Input     map[string]any `json:"input"`
				ToolUseID string         `json:"tool_use_id"`
				Content   any            `json:"content"`
				IsError   bool           `json:"is_error"`
			} `json:"content"`
		}
		_ = json.Unmarshal(m.Message, &msg)
		for _, c := range msg.Content {
			switch c.Type {
			case "text":
				if m.Type == "assistant" && strings.TrimSpace(c.Text) != "" {
					evs = append(evs, Event{Kind: "text", Text: c.Text})
				}
			case "tool_use":
				evs = append(evs, Event{Kind: "tool_start", ToolKey: c.ID, ToolName: c.Name, Input: c.Input})
			case "tool_result":
				evs = append(evs, Event{Kind: "tool_end", ToolKey: c.ToolUseID, Output: textOf(c.Content), IsError: c.IsError})
			}
		}
	case "result":
		status := "completed"
		if m.IsError {
			status = "failed"
		}
		evs = append(evs, Event{Kind: "done", Status: status, SessionID: m.SessionID})
	}
	return evs
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func tail(s string, n int) string {
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}
