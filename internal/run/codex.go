package run

import (
	"context"
	"encoding/json"

	"cloudbuddy/internal/recognize"
)

// codexApproval answers Codex's approval requests through the gate (D-021).
// Only accept / decline are ever sent: never acceptForSession or amendments.
func (e *Engine) codexApproval(ctx context.Context, r *Run, method string, params []byte) (any, error) {
	decline := map[string]any{"decision": "decline"}
	switch method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		var p struct {
			ItemID  string          `json:"itemId"`
			CallID  string          `json:"callId"`
			Command any             `json:"command"`
			Cwd     string          `json:"cwd"`
			Reason  string          `json:"reason"`
			Kind    string          `json:"kind"`
			NetCtx  json.RawMessage `json:"networkApprovalContext"`
		}
		if json.Unmarshal(params, &p) != nil {
			return decline, nil
		}
		// Only plain command approvals: "writeStdin" types into a running
		// terminal, and a network-context approval can release other queued
		// commands to the same host (Phase 2 review).
		if (p.Kind != "" && p.Kind != "command") || (len(p.NetCtx) > 0 && string(p.NetCtx) != "null") {
			return decline, nil
		}
		cmd := ""
		switch c := p.Command.(type) {
		case string:
			cmd = c
		case []any: // older shape: argv
			for i, x := range c {
				if s, ok := x.(string); ok {
					if i > 0 {
						cmd += " "
					}
					cmd += s
				}
			}
		}
		key := p.ItemID
		if key == "" {
			key = p.CallID
		}
		cwd := p.Cwd
		if cwd == "" {
			cwd = r.Workdir
		}
		call := recognize.Call{Agent: "codex", Via: "shell", Command: unwrapShell(cmd), WantsNetwork: true, Cwd: cwd, Description: p.Reason}
		o := e.gate(ctx, r, key, call)
		if !o.Allowed {
			return decline, nil
		}
		if err := e.consume(ctx, r, o, key, call); err != nil {
			return decline, nil
		}
		r.mu.Lock()
		r.escalated[key] = true
		r.mu.Unlock()
		return map[string]any{"decision": "accept"}, nil
	case "item/fileChange/requestApproval", "applyPatchApproval":
		var p struct {
			ItemID string `json:"itemId"`
			CallID string `json:"callId"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(params, &p)
		key := p.ItemID
		if key == "" {
			key = p.CallID
		}
		r.mu.Lock()
		paths := r.codexPath[key]
		r.mu.Unlock()
		if len(paths) == 0 {
			return decline, nil // can't tell which files: don't approve blind
		}
		call := recognize.Call{Agent: "codex", Via: "file", Tool: "Write", Paths: paths, Cwd: r.Workdir, Description: p.Reason}
		o := e.gate(ctx, r, key, call)
		if !o.Allowed || e.consume(ctx, r, o, key, call) != nil {
			return decline, nil
		}
		return map[string]any{"decision": "accept"}, nil
	}
	return decline, nil
}
