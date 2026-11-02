package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
			Workdir: c.Workdir, ProjectID: c.ProjectID, Status: c.Status, UpdatedAt: c.UpdatedAt},
		ExtraFolders: c.ExtraFolders, Subtitle: c.Subtitle, Chat: chat, Steps: steps, Result: c.Result,
	}
	for i := range steps {
		decorate(&steps[i])
	}
	for _, s := range steps {
		if s.Kind == "cloud" {
			d.StepCount++
		}
		if s.Approval != nil && s.Approval.State == "pending" {
			d.WaitingCount++
		}
	}
	d.Subtitle = subtitleFor(steps)
	return d, nil
}

// subtitleFor names the providers a conversation touched so far.
func subtitleFor(steps []model.Step) string {
	var providers []string
	seen := map[string]bool{}
	for _, s := range steps {
		if s.Kind == "cloud" && !seen[s.Provider] {
			seen[s.Provider] = true
			providers = append(providers, s.Provider)
		}
	}
	switch len(providers) {
	case 0:
		return "No cloud steps yet"
	case 1:
		return providers[0]
	}
	return fmt.Sprintf("%d providers · %s", len(providers), strings.Join(providers, ", "))
}

var productSubs = map[string]string{"S3": "Storage", "EC2": "Servers", "Account": "STS", "CloudFront": "CDN",
	"CloudWatch": "Monitoring", "CloudWatch Logs": "Logs", "SNS": "Alerts", "SQS": "Queues", "IAM": "Access",
	"Lambda": "Functions", "RDS": "Databases", "DynamoDB": "Database", "Route 53": "DNS", "ECS": "Containers",
	"EKS": "Kubernetes", "ECR": "Images", "Secrets Manager": "Secrets", "CloudFormation": "Stacks", "SES": "Email",
	"KMS": "Keys", "ACM": "Certificates", "API Gateway": "APIs", "Budgets": "Billing", "Systems Manager": "Config"}

// decorate adds display-only fields the browser needs.
func decorate(s *model.Step) {
	if s.ProductSub == "" {
		s.ProductSub = productSubs[s.Product]
	}
	a := s.Approval
	if a == nil {
		return
	}
	switch {
	case a.Warning != "":
		a.YesLabel, a.NoLabel = "Run it", "Don't run"
	case s.Action == model.ActionDelete:
		a.YesLabel, a.NoLabel = "Delete", "Keep it"
	case s.Action == model.ActionSend:
		a.YesLabel, a.NoLabel = "Send", "Not yet"
	default:
		a.YesLabel, a.NoLabel = "Approve", "Deny"
	}
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
	if call.Via == "mcp" && call.Server == DashServer {
		// the demo agent's dashboard calls: the hook, then the tool itself
		if ans := e.dashboardHook(ctx, r, in, call); ans.Decision != "allow" {
			return false, ans.Reason
		}
		args, _ := json.Marshal(input)
		res := e.DashboardTool(ctx, runID, toolKey, call.Tool, args)
		return !res.IsError, res.Text
	}
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
