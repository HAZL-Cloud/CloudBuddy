package run

import (
	"context"

	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
)

// Detail builds the API's ConversationDetail.
func (e *Engine) Detail(ctx context.Context, id string) (model.ConversationDetail, error) {
	c, err := e.st.GetConversation(ctx, id)
	if err != nil {
		return model.ConversationDetail{}, err
	}
	steps, err := e.st.Steps(ctx, id)
	if err != nil {
		return model.ConversationDetail{}, err
	}
	chat, err := e.st.Chat(ctx, id)
	if err != nil {
		return model.ConversationDetail{}, err
	}
	d := model.ConversationDetail{
		ConversationSummary: model.ConversationSummary{ID: c.ID, Title: c.Title, Agent: c.Agent, Model: c.Model,
			Workdir: c.Workdir, Status: c.Status, UpdatedAt: c.UpdatedAt},
		Subtitle: c.Subtitle, Chat: chat, Steps: steps, Result: c.Result,
	}
	for _, s := range steps {
		if s.Kind == "cloud" {
			d.StepCount++
		}
		if s.Approval != nil && s.Approval.State == "pending" {
			d.WaitingCount++
		}
	}
	if d.Subtitle == "" {
		d.Subtitle = "No cloud steps yet"
	}
	return d, nil
}

// fakeGate lets the fake agent ask the gate exactly as a Claude hook would.
func (e *Engine) fakeGate(ctx context.Context, runID, toolKey, toolName string, input map[string]any) (bool, string) {
	in := HookInput{ToolName: toolName, ToolInput: input, ToolUseID: toolKey}
	r := e.run(runID)
	if r == nil {
		return false, "run ended"
	}
	in.Cwd = r.Workdir
	call := CallFromClaude(in)
	var o outcome
	if call.Via == "mcp" {
		ok, reason, _ := e.MCPCall(ctx, runID, call.Server, call.Tool, call.Args, map[string]any{"claudecode/toolUseId": toolKey})
		return ok, reason
	}
	o = e.gate(ctx, r, toolKey, call)
	if !o.Allowed {
		return false, o.Reason
	}
	if err := e.consume(ctx, r, o, toolKey, call); err != nil {
		return false, "approval already used"
	}
	return true, ""
}

var _ = recognize.Call{}
