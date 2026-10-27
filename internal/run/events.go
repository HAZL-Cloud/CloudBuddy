package run

import (
	"context"
	"strings"
	"time"

	"cloudbuddy/internal/agent"
	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

// handle applies one agent event to the conversation.
func (e *Engine) handle(ctx context.Context, r *Run, ev agent.Event) {
	bg := context.Background()
	payload, _ := mask.String(evSummary(ev))
	payload, trunc := mask.Cap(payload, mask.MaxEventBytes)
	_ = e.st.AppendEvent(bg, r.ID, r.ConvID, "agent", ev.Kind, map[string]any{"summary": payload, "toolKey": ev.ToolKey, "tool": ev.ToolName}, trunc)

	switch ev.Kind {
	case "session":
		if ev.SessionID != "" {
			_ = e.st.SetSessionID(bg, r.ConvID, ev.SessionID)
		}
		if ev.Model != "" {
			_ = e.st.SetModel(bg, r.ConvID, ev.Model)
		}
	case "text":
		t, _ := mask.String(ev.Text)
		t, _ = mask.Cap(t, 64*1024)
		_ = e.st.AppendChat(bg, r.ConvID, model.ChatItem{Type: "agent", Text: t})
	case "tool_start":
		e.toolStart(bg, r, ev)
	case "tool_end":
		e.toolEnd(bg, r, ev)
	}
	e.bus.Conversation(r.ConvID)
}

func evSummary(ev agent.Event) string {
	switch ev.Kind {
	case "text":
		return ev.Text
	case "tool_start":
		if ev.Command != "" {
			return ev.ToolName + ": " + ev.Command
		}
		return ev.ToolName
	case "tool_end":
		return ev.Output
	}
	return ev.Kind
}

func (e *Engine) toolStart(ctx context.Context, r *Run, ev agent.Event) {
	switch r.Agent {
	case "claude":
		// Every Claude tool call must pass the hook. If the SessionStart
		// canary never arrived, hooks aren't running: stop (D-039).
		if e.ClaudeCanary && !r.canary.Load() {
			e.bypass(r, "CloudBuddy's safety hook isn't running in this session, so the run was stopped before the tool could be used.")
		}
	case "codex":
		// Codex reports sandboxed commands without asking; record them as
		// local steps. A later approval for the same item updates the step.
		var call recognize.Call
		switch ev.ToolName {
		case "shell":
			call = recognize.Call{Agent: "codex", Via: "shell", Command: unwrapShell(ev.Command), Cwd: r.Workdir}
		case "file_change":
			r.mu.Lock()
			r.codexPath[ev.ToolKey] = ev.Paths
			r.mu.Unlock()
			call = recognize.Call{Agent: "codex", Via: "file", Tool: "Write", Paths: ev.Paths, Cwd: r.Workdir}
		default:
			call = recognize.Call{Agent: "codex", Via: "tool", Tool: ev.ToolName}
		}
		d := recognize.Recognize(call, e.env(r), e.adapters)
		st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, ev.ToolKey, func(s *model.Step, isNew bool) {
			if !isNew {
				return
			}
			applyDraft(s, d, call)
			s.Kind = "local" // it runs in Codex's no-network sandbox unless approved
		})
		if err == nil {
			_ = e.st.AddStepToChat(ctx, r.ConvID, st.ID)
		}
	}
}

func (e *Engine) toolEnd(ctx context.Context, r *Run, ev agent.Event) {
	_, found, _ := e.st.StepByKey(ctx, r.ConvID, ev.ToolKey)
	if !found {
		if r.Agent == "claude" {
			// a tool finished that the gate never saw: something bypassed it
			e.bypass(r, "A tool ran without passing CloudBuddy's gate, so the run was stopped. Check this computer's Claude Code settings.")
			return
		}
		return
	}
	out, masked, trunc := mask.Clean(ev.Output)
	st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, ev.ToolKey, func(s *model.Step, _ bool) {
		now := time.Now().UTC()
		if s.Status != model.StatusDenied {
			if ev.IsError {
				s.Status = model.StatusFailed
			} else {
				s.Status = model.StatusDone
			}
		}
		if s.EndedAt == nil {
			s.EndedAt = &now
			d := now.Sub(s.StartedAt).Milliseconds()
			s.DurationMs = &d
		}
		if out != "" || s.Output == "" {
			s.Output = out
		}
		s.OutputTruncated = trunc
		if masked {
			s.MaskedNote = "CloudBuddy hid something that looked like a secret before saving this output."
		}
		if r.Agent == "codex" {
			r.mu.Lock()
			esc, gated := r.escalated[ev.ToolKey], r.gated[ev.ToolKey]
			r.mu.Unlock()
			if !esc && !gated {
				s.Kind = "local" // it ran in Codex's no-network sandbox
			}
		}
	})
	if err == nil && st.Status == model.StatusDone {
		e.recordLinks(ctx, r, st.ID, ev.Output)
	}
}

func (e *Engine) bypass(r *Run, msg string) {
	if r.ended.Load() {
		return
	}
	_ = e.st.AppendChat(context.Background(), r.ConvID, model.ChatItem{Type: "notice", Level: "error", Text: msg})
	r.cancel()
}

// unwrapShell strips Codex's `/bin/zsh -lc '...'` wrapper for display.
func unwrapShell(cmd string) string {
	for _, pre := range []string{"/bin/zsh -lc ", "/bin/bash -lc ", "/bin/sh -c ", "bash -lc ", "zsh -lc "} {
		if strings.HasPrefix(cmd, pre) {
			rest := strings.TrimPrefix(cmd, pre)
			if len(rest) >= 2 && (rest[0] == '\'' || rest[0] == '"') && rest[len(rest)-1] == rest[0] {
				rest = rest[1 : len(rest)-1]
				rest = strings.ReplaceAll(rest, `'\''`, `'`)
			}
			return rest
		}
	}
	return cmd
}
