package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/store"
)

// HookInput is what Claude Code sends a PreToolUse hook.
type HookInput struct {
	SessionID string         `json:"session_id"`
	Cwd       string         `json:"cwd"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
}

// HookAnswer goes back to the hook program, which prints it for Claude Code.
type HookAnswer struct {
	Decision     string         `json:"decision"` // allow | deny
	Reason       string         `json:"reason"`
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
}

var knownBashKeys = map[string]bool{"command": true, "description": true, "timeout": true,
	"run_in_background": true, "dangerouslyDisableSandbox": true, "allowed_domains": true}

var fileTools = map[string]string{"Read": "file_path", "Edit": "file_path", "MultiEdit": "file_path", "Write": "file_path",
	"NotebookEdit": "notebook_path", "NotebookRead": "notebook_path", "Glob": "path", "Grep": "path"}

// CallFromClaude maps a Claude Code tool call onto a recognizer call.
func CallFromClaude(in HookInput) recognize.Call {
	c := recognize.Call{Agent: "claude", Tool: in.ToolName, Args: in.ToolInput, Cwd: in.Cwd}
	str := func(k string) string { s, _ := in.ToolInput[k].(string); return s }
	switch {
	case in.ToolName == "Bash":
		c.Via = "shell"
		c.Command = str("command")
		c.Description = str("description")
		c.WantsNetwork, _ = in.ToolInput["dangerouslyDisableSandbox"].(bool)
		// allowed_domains opens network from inside Claude Code's sandbox, so
		// it counts as asking for network (found in the Phase 2 live run).
		if d, ok := in.ToolInput["allowed_domains"]; ok && d != nil {
			if list, isList := d.([]any); !isList || len(list) > 0 {
				c.WantsNetwork = true
			}
		}
		c.Background, _ = in.ToolInput["run_in_background"].(bool)
		c.Args = nil
	case fileTools[in.ToolName] != "":
		c.Via = "file"
		if p := str(fileTools[in.ToolName]); p != "" {
			c.Paths = []string{p}
		}
	case strings.HasPrefix(in.ToolName, "mcp__"):
		parts := strings.SplitN(strings.TrimPrefix(in.ToolName, "mcp__"), "__", 2)
		c.Via, c.Server = "mcp", parts[0]
		if len(parts) == 2 {
			c.Tool = parts[1]
		}
	default:
		c.Via = "tool"
	}
	return c
}

// ClaudeSessionStart is the canary: hooks are running in this session.
func (e *Engine) ClaudeSessionStart(runID string) {
	if r := e.run(runID); r != nil {
		r.canary.Store(true)
	}
}

// ClaudePreTool decides one Claude Code tool call.
func (e *Engine) ClaudePreTool(ctx context.Context, runID string, in HookInput) HookAnswer {
	r := e.run(runID)
	if r == nil {
		return HookAnswer{Decision: "deny", Reason: "CloudBuddy doesn't know this run; it may have ended."}
	}
	r.canary.Store(true) // a hook call proves hooks run
	call := CallFromClaude(in)
	if call.Cwd == "" {
		call.Cwd = r.Workdir
	}
	// A Bash option CloudBuddy doesn't know might change how the sandbox
	// treats the command: deny rather than guess (fail closed).
	if in.ToolName == "Bash" {
		for k := range in.ToolInput {
			if !knownBashKeys[k] {
				return HookAnswer{Decision: "deny", Reason: "CloudBuddy doesn't know the Bash option \"" + k + "\" yet, so it can't check this command. Run it without that option."}
			}
		}
	}
	// MCP calls through CloudBuddy's gateway are decided by the gateway.
	if call.Via == "mcp" && r.gateways[call.Server] {
		return HookAnswer{Decision: "allow", Reason: "CloudBuddy's MCP gateway checks this call."}
	}
	o := e.gate(ctx, r, in.ToolUseID, call)
	if !o.Allowed {
		return HookAnswer{Decision: "deny", Reason: o.Reason}
	}
	// Network commands run through cloudbuddy exec, the only program
	// allowed out of Claude Code's sandbox (D-036).
	if call.Via == "shell" && o.Draft.Network != "" {
		token := randomToken()
		rel := store.Release{TokenHash: hashToken(token), StepID: o.Step.ID, RunID: r.ID, LaunchEpoch: e.cfg.Epoch,
			ApprovalID: o.ApprovalID, Command: call.Command, Cwd: call.Cwd, NetworkKind: o.Draft.Network, ExecDigest: execDigest(call, o.Draft)}
		if err := e.st.CreateRelease(ctx, rel); err != nil {
			return HookAnswer{Decision: "deny", Reason: "CloudBuddy couldn't prepare the command: " + err.Error()}
		}
		upd := map[string]any{}
		for k, v := range in.ToolInput {
			upd[k] = v
		}
		delete(upd, "dangerouslyDisableSandbox")
		delete(upd, "allowed_domains") // the wrapper runs outside Claude's sandbox
		upd["command"] = fmt.Sprintf("%s exec --release %s", e.cfg.Bin, token)
		return HookAnswer{Decision: "allow", Reason: "Approved in CloudBuddy.", UpdatedInput: upd}
	}
	if err := e.consume(ctx, r, o, in.ToolUseID, call); err != nil {
		return HookAnswer{Decision: "deny", Reason: "This approval was already used or no longer matches."}
	}
	return HookAnswer{Decision: "allow", Reason: "Allowed by CloudBuddy."}
}

func decodeHook(raw []byte) (HookInput, error) {
	var in HookInput
	err := json.Unmarshal(raw, &in)
	return in, err
}
