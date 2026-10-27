package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/policy"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/sandbox"
	"cloudbuddy/internal/store"
)

// approvalWait is how long the gate holds a call waiting for the user. The
// Claude hook times out after 7200 s and then blocks (onFailure).
const approvalWait = 115 * time.Minute

type outcome struct {
	Allowed bool
	Reason  string
	Draft   recognize.Draft
	Step    model.Step
	// ApprovalID is set when the user approved this call.
	ApprovalID string
	// ExecDigest is the file hash taken when the user was asked; the release
	// must compare against it, never a fresh one (Phase 2 review).
	ExecDigest string
}

// gate is the single decision path for every agent tool call (section 4.2).
func (e *Engine) gate(ctx context.Context, r *Run, toolKey string, call recognize.Call) outcome {
	if r == nil || r.ended.Load() {
		return outcome{Reason: "This run has ended."}
	}
	draft := recognize.Recognize(call, e.env(r.Workdir), e.adapters)
	rule := e.ruleFor(ctx, draft.Provider, call.Server)
	dec := policy.Decide(draft, policy.Context{Rule: rule, CanWrapCode: r.Agent != "codex" && sandbox.CanWrap()})

	// hold new decisions while project code with network runs in this folder (D-029)
	lk := e.workLock(r.Workdir)
	lk.RLock()
	lk.RUnlock()

	uses := e.linksFor(ctx, r, toolKey, call)
	status := model.StatusRunning
	switch dec.Verdict {
	case policy.Deny:
		status = model.StatusDenied
	case policy.Ask:
		status = model.StatusWaiting
	}
	st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, toolKey, func(s *model.Step, isNew bool) {
		applyDraft(s, draft, call)
		s.Status = status
		if len(uses) > 0 {
			s.Uses = uses
		}
		if dec.Verdict == policy.Deny {
			now := time.Now().UTC()
			s.EndedAt = &now
			s.Output = "Not run: " + dec.Reason
		}
	})
	if err != nil {
		return outcome{Reason: "CloudBuddy couldn't record this step: " + err.Error()}
	}
	r.mu.Lock()
	r.gated[toolKey] = true
	r.mu.Unlock()
	_ = e.st.AppendEvent(ctx, r.ID, r.ConvID, "gate", "decision", map[string]any{
		"toolKey": toolKey, "verdict": dec.Verdict, "row": dec.Row, "reason": dec.Reason,
		"via": call.Via, "tool": call.Tool, "command": maskStr(call.Command)}, false)

	switch dec.Verdict {
	case policy.Allow:
		if !internalTool(call) {
			_ = e.st.AddStepToChat(ctx, r.ConvID, st.ID)
		}
		e.bus.Conversation(r.ConvID)
		return outcome{Allowed: true, Draft: draft, Step: st}
	case policy.Deny:
		_ = e.st.AddStepToChat(ctx, r.ConvID, st.ID)
		e.bus.Conversation(r.ConvID)
		return outcome{Reason: dec.Reason, Draft: draft, Step: st}
	}

	// ask: create the approval and wait for the user (D-024)
	ap := store.ApprovalRecord{ID: store.NewID("ap"), StepID: st.ID, ConversationID: r.ConvID, RunID: r.ID,
		LaunchEpoch: e.cfg.Epoch, RequestKey: toolKey, PayloadDigest: payloadDigest(toolKey, call),
		ExecDigest: execDigest(call, draft), Reason: dec.Reason, LossTitle: dec.LossTitle, Loss: dec.Loss, Warning: dec.Warning}
	if err := e.st.CreateApproval(ctx, ap); err != nil {
		return outcome{Reason: "CloudBuddy couldn't record the approval: " + err.Error()}
	}
	ch := make(chan bool, 1)
	e.mu.Lock()
	e.waiters[ap.ID] = ch
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.waiters, ap.ID)
		e.mu.Unlock()
	}()
	_ = e.st.AppendChat(ctx, r.ConvID, model.ChatItem{Type: "approval", StepID: st.ID})
	_ = e.st.SetConversationStatus(ctx, r.ConvID, model.ConvWaiting)
	e.bus.Conversation(r.ConvID)

	approved := false
	select {
	case approved = <-ch:
	case <-ctx.Done():
	case <-r.done:
	case <-time.After(approvalWait):
	}
	if !approved {
		_ = e.st.ExpireApproval(context.Background(), ap.ID)
	}
	bg := context.Background()
	if !r.ended.Load() {
		_ = e.st.SetConversationStatus(bg, r.ConvID, model.ConvRunning)
	}
	if !approved {
		st, _, _ = e.updateStepByID(bg, r, st.ID, func(s *model.Step) {
			s.Status = model.StatusDenied
			now := time.Now().UTC()
			s.EndedAt = &now
			s.Output = "Not run: you said no, or the approval expired."
		})
		_ = e.st.AddStepToChat(bg, r.ConvID, st.ID)
		e.bus.Conversation(r.ConvID)
		return outcome{Reason: "The user did not approve this in CloudBuddy. Don't try to do it another way; ask the user.", Draft: draft, Step: st}
	}
	st, _, _ = e.updateStepByID(bg, r, st.ID, func(s *model.Step) { s.Status = model.StatusRunning })
	_ = e.st.AddStepToChat(bg, r.ConvID, st.ID)
	e.bus.Conversation(r.ConvID)
	return outcome{Allowed: true, Draft: draft, Step: st, ApprovalID: ap.ID, ExecDigest: ap.ExecDigest}
}

// Decide is the user's answer from the browser.
func (e *Engine) Decide(ctx context.Context, approvalID string, approve bool) error {
	a, err := e.st.DecideApproval(ctx, approvalID, approve)
	if err != nil {
		return err
	}
	if a.LaunchEpoch != e.cfg.Epoch {
		return store.ErrNotPending
	}
	e.mu.Lock()
	ch := e.waiters[approvalID]
	e.mu.Unlock()
	if ch == nil {
		// nobody is waiting (the run ended): the decision is recorded but
		// nothing runs
		return store.ErrNotPending
	}
	select {
	case ch <- approve:
	default:
	}
	e.bus.Conversation(a.ConversationID)
	return nil
}

// consume marks an approved call as used, once (the release step of D-024).
func (e *Engine) consume(ctx context.Context, r *Run, o outcome, toolKey string, call recognize.Call) error {
	if o.ApprovalID == "" {
		return nil
	}
	return e.st.ConsumeApproval(ctx, o.ApprovalID, r.ID, e.cfg.Epoch, payloadDigest(toolKey, call))
}

func applyDraft(s *model.Step, d recognize.Draft, call recognize.Call) {
	s.Kind, s.Provider, s.Product, s.Action = d.Kind, d.Provider, d.Product, d.Action
	s.Tags = d.Tags
	if s.Tags == nil {
		s.Tags = []model.Tag{}
	}
	s.Title, s.TitleSource = maskStr(d.Title), d.TitleSource
	s.Resource, s.API = maskStr(d.Resource), d.API
	s.Facts = d.Facts
	if s.Facts == nil {
		s.Facts = [][2]string{}
	}
	if d.Identity.Label != "" && d.Kind == "cloud" {
		s.Facts = append(s.Facts, [2]string{"Account", d.Identity.Label})
	}
	s.RecognizedBy = d.RecognizedBy
	s.Generic, s.GuessWord = d.Generic, d.GuessWord
	switch call.Via {
	case "shell":
		s.Via = "cli"
		if strings.Contains(d.API, " ") && (strings.HasPrefix(d.API, "GET") || strings.HasPrefix(d.API, "POST") || strings.HasPrefix(d.API, "PUT") || strings.HasPrefix(d.API, "DELETE") || strings.HasPrefix(d.API, "PATCH")) {
			s.Via = "api"
		}
		s.Command = maskStr(call.Command)
	case "mcp":
		s.Via = "mcp"
		args, _ := json.Marshal(call.Args)
		s.Command = maskStr(fmt.Sprintf("%s(%s)", call.Tool, args))
	case "file":
		s.Via = "file"
		s.Command = maskStr(call.Tool + " " + strings.Join(call.Paths, " "))
	default:
		s.Via = "tool"
		args, _ := json.Marshal(call.Args)
		s.Command = maskStr(call.Tool + " " + string(args))
	}
	s.Why = maskStr(call.Description)
}

func maskStr(s string) string {
	out, _ := mask.String(s)
	return out
}

func payloadDigest(toolKey string, c recognize.Call) string {
	h := sha256.New()
	args, _ := json.Marshal(c.Args)
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%v\x00%s", toolKey, c.Via, c.Tool, c.Server, c.Command, c.WantsNetwork, args)
	fmt.Fprintf(h, "\x00%s", strings.Join(c.Paths, "\x00"))
	return hex.EncodeToString(h.Sum(nil))
}

// execDigest hashes the program and script files a kind-B command would run,
// so an edit while the approval waits voids it (D-024).
func execDigest(c recognize.Call, d recognize.Draft) string {
	if d.Network != "B" || c.Via != "shell" {
		return ""
	}
	return filesDigest(c.Command, c.Cwd)
}

var manifests = []string{"package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "requirements.txt",
	"pyproject.toml", "go.mod", "Makefile", "cdk.json", "Pulumi.yaml", "serverless.yml", "wrangler.toml", "Gemfile", "Cargo.toml"}

func filesDigest(command, cwd string) string {
	var files []string
	for _, w := range strings.Fields(command) {
		w = strings.Trim(w, `"'`)
		p := w
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			files = append(files, p)
		}
	}
	for _, m := range manifests {
		p := filepath.Join(cwd, m)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			files = append(files, p)
		}
	}
	tf, _ := filepath.Glob(filepath.Join(cwd, "*.tf"))
	files = append(files, tf...)
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(h, "%s %x\n", f, sum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (e *Engine) workLock(dir string) *sync.RWMutex {
	e.workMu.Lock()
	defer e.workMu.Unlock()
	lk := e.workLk[dir]
	if lk == nil {
		lk = &sync.RWMutex{}
		e.workLk[dir] = lk
	}
	return lk
}

func (e *Engine) updateStepByID(ctx context.Context, r *Run, stepID string, f func(*model.Step)) (model.Step, bool, error) {
	convID, key, err := e.st.StepConversation(ctx, stepID)
	if err != nil {
		return model.Step{}, false, err
	}
	runID := ""
	if r != nil {
		runID = r.ID
	}
	st, err := e.st.UpsertStep(ctx, convID, runID, key, func(s *model.Step, _ bool) { f(s) })
	return st, err == nil, err
}

// ruleFor finds the user's Connections rule for a provider (Phase 6). An MCP
// server's own rule wins over its provider's.
func (e *Engine) ruleFor(ctx context.Context, provider, server string) string {
	rules, err := e.st.Rules(ctx)
	if err != nil || len(rules) == 0 {
		return ""
	}
	conns, _, err := e.st.Connections(ctx)
	if err != nil {
		return ""
	}
	best := ""
	for _, c := range conns {
		r := rules[c.ID]
		if r == "" {
			continue
		}
		if server != "" && c.Kind == "mcp" && strings.EqualFold(c.ID, "mcp:"+server) {
			return r
		}
		if strings.EqualFold(c.Name, provider) {
			// several connections for one provider: the strictest rule wins
			if strictness(r) > strictness(best) {
				best = r
			}
		}
	}
	return best
}

func strictness(r string) int {
	switch r {
	case model.RuleOff:
		return 5
	case model.RuleReadOnly:
		return 4
	case model.RuleEvery:
		return 3
	case model.RuleLive:
		return 2
	case model.RuleIrreversible:
		return 1
	}
	return 0
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// internalTool marks agent bookkeeping (loading tool schemas, to-do lists)
// that would only clutter the chat.
func internalTool(c recognize.Call) bool {
	return c.Via == "tool" && (c.Tool == "ToolSearch" || c.Tool == "TodoWrite")
}
