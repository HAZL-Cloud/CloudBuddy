package run

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/store"
)

// MCPServer is a user's stdio MCP server as CloudBuddy starts it behind the
// gateway. Servers reached by URL (often OAuth) aren't supported yet (Q-06).
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// userMCPServers reads the stdio MCP servers the user set up for the agent:
// Claude Code's ~/.claude.json and Codex's ~/.codex/config.toml.
func (e *Engine) userMCPServers(agentName string) map[string]MCPServer {
	out := map[string]MCPServer{}
	if os.Getenv("CB_NO_USER_MCP") == "1" { // tests: never start the user's real servers
		goto extra
	}
	// each agent only gets the servers from its own config
	if agentName != "claude" {
		goto codex
	}
	if b, err := os.ReadFile(filepath.Join(e.cfg.Home, ".claude.json")); err == nil {
		var cfg struct {
			MCPServers map[string]struct {
				Type    string            `json:"type"`
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Env     map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(b, &cfg) == nil {
			for name, s := range cfg.MCPServers {
				if s.Command != "" && (s.Type == "" || s.Type == "stdio") {
					out[name] = MCPServer{Command: s.Command, Args: s.Args, Env: s.Env}
				}
			}
		}
	}
codex:
	if agentName != "codex" {
		goto extra
	}
	if b, err := os.ReadFile(filepath.Join(e.cfg.Home, ".codex", "config.toml")); err == nil {
		var cfg struct {
			MCPServers map[string]struct {
				Command string            `toml:"command"`
				Args    []string          `toml:"args"`
				Env     map[string]string `toml:"env"`
			} `toml:"mcp_servers"`
		}
		if toml.Unmarshal(b, &cfg) == nil {
			for name, s := range cfg.MCPServers {
				if _, dup := out[name]; !dup && s.Command != "" {
					out[name] = MCPServer{Command: s.Command, Args: s.Args, Env: s.Env}
				}
			}
		}
	}
extra:
	if extra := os.Getenv("CB_EXTRA_MCP"); extra != "" { // tests: {"name": {command,args,env}}
		var m map[string]MCPServer
		if json.Unmarshal([]byte(extra), &m) == nil {
			for k, v := range m {
				out[k] = v
			}
		}
	}
	return out
}

// MCPTool is one tool as the gateway saw it in tools/list.
type MCPTool struct {
	Name        string           `json:"name"`
	Annotations *recognize.Hints `json:"annotations,omitempty"`
}

func (e *Engine) MCPTools(runID, server string, tools []MCPTool) {
	e.hintsMu.Lock()
	defer e.hintsMu.Unlock()
	for _, t := range tools {
		if t.Annotations != nil {
			e.hints[runID+"/"+server+"/"+t.Name] = *t.Annotations
		}
	}
}

func (e *Engine) hintsFor(runID, server, tool string) *recognize.Hints {
	e.hintsMu.Lock()
	defer e.hintsMu.Unlock()
	if h, ok := e.hints[runID+"/"+server+"/"+tool]; ok {
		return &h
	}
	return nil
}

// MCPCall decides one tools/call arriving at the gateway.
func (e *Engine) MCPCall(ctx context.Context, runID, server, tool string, args map[string]any, meta map[string]any) (bool, string, string) {
	r := e.run(runID)
	if r == nil {
		return false, "CloudBuddy doesn't know this run; it may have ended.", ""
	}
	key := ""
	if v, ok := meta["claudecode/toolUseId"].(string); ok {
		key = v
	} else if v, ok := meta["callId"].(string); ok {
		key = "mcp-" + v
	} else {
		key = store.NewID("mcp")
	}
	call := recognize.Call{Agent: r.Agent, Via: "mcp", Server: server, Tool: tool, Args: args, Cwd: r.Workdir,
		Hints: e.hintsFor(runID, server, tool)}
	o := e.gate(ctx, r, key, call)
	if !o.Allowed {
		return false, o.Reason, key
	}
	if err := e.consume(ctx, r, o, key, call); err != nil {
		return false, "This approval was already used or no longer matches.", key
	}
	return true, "", key
}

// MCPResult records what a gateway call returned.
func (e *Engine) MCPResult(ctx context.Context, runID, key, output string, isError bool) {
	r := e.run(runID)
	if r == nil || key == "" {
		return
	}
	out, masked, trunc := mask.Clean(output)
	st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, key, func(s *model.Step, isNew bool) {
		if isNew {
			return
		}
		now := time.Now().UTC()
		if s.Status != model.StatusDenied {
			s.Status = model.StatusDone
			if isError {
				s.Status = model.StatusFailed
			}
		}
		if s.EndedAt == nil {
			s.EndedAt = &now
			d := now.Sub(s.StartedAt).Milliseconds()
			s.DurationMs = &d
		}
		s.Output, s.OutputTruncated = out, trunc
		if masked {
			s.MaskedNote = "CloudBuddy hid something that looked like a secret before saving this output."
		}
	})
	if err == nil && !isError {
		e.recordLinks(ctx, r, st.ID, output)
	}
	e.bus.Conversation(r.ConvID)
}
