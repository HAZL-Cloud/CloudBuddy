package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/netlog"
	"cloudbuddy/internal/policy"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/shim"
	"cloudbuddy/internal/store"
)

// execState is one command running through cloudbuddy exec (D-065): its
// network log, its shim token, and the wrapper's process for Stop.
type execState struct {
	runID, stepID, tokenHash string
	shimToken                string
	pid                      int
	background               bool
	quiet                    bool   // a plain cloud CLI call: its step already shows what the shims would
	stopped                  string // who stopped it: "you" | "run"
	log                      *netlog.Log
	started                  time.Time
}

func (e *Engine) trackExec(x *execState) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	e.execByToken[x.tokenHash] = x
	e.execByShim[x.shimToken] = x
	e.execByStep[x.stepID] = x
}

func (e *Engine) untrackExec(x *execState) {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	delete(e.execByToken, x.tokenHash)
	delete(e.execByShim, x.shimToken)
	if e.execByStep[x.stepID] == x {
		delete(e.execByStep, x.stepID)
	}
}

// ExecFinished is called by cloudbuddy exec when the command ends. It adds
// where the command went to its card, and finishes a background step.
func (e *Engine) ExecFinished(token string, exitCode, leftovers int, output string) {
	e.execMu.Lock()
	x := e.execByToken[hashToken(token)]
	e.execMu.Unlock()
	if x == nil {
		return
	}
	e.untrackExec(x)
	var hits []netlog.Hit
	if x.log != nil {
		hits = x.log.Stop()
	}
	r := e.run(x.runID)
	ctx := context.Background()
	_, _, err := e.updateStepByID(ctx, r, x.stepID, func(s *model.Step) {
		if len(hits) > 0 {
			applyHits(s, hits)
		}
		if x.background || x.stopped != "" {
			now := time.Now().UTC()
			if s.Status == model.StatusRunning || s.Status == model.StatusDone || s.Status == model.StatusOutcomeUnknown {
				s.Status = model.StatusDone
				if exitCode != 0 && x.stopped == "" {
					s.Status = model.StatusFailed
				}
			}
			s.EndedAt = &now
			d := now.Sub(s.StartedAt).Milliseconds()
			s.DurationMs = &d
			if out, masked, trunc := mask.Clean(output); out != "" && x.background {
				s.Output, s.OutputTruncated = out, trunc
				if masked {
					s.MaskedNote = "CloudBuddy hid something that looked like a secret before saving this output."
				}
			}
			s.Output = withStopNote(s.Output, x.stopped)
		}
		_ = leftovers
	})
	if err != nil {
		return
	}
	if convID, _, err := e.st.StepConversation(ctx, x.stepID); err == nil {
		e.bus.Conversation(convID)
	}
}

// applyHits records the hosts a command talked to. A command CloudBuddy
// took for local work that did reach the internet becomes a cloud card.
func applyHits(s *model.Step, hits []netlog.Hit) {
	facts := s.Facts[:0:0]
	for _, f := range s.Facts {
		if f[0] != "Talked to" {
			facts = append(facts, f)
		}
	}
	s.Facts = append(facts, [2]string{"Talked to", netlog.Summary(hits, 4)})
	provider := ""
	for _, h := range hits {
		if p := recognize.ProviderForHost(h.Host); p != "" && p != "npm" && p != "PyPI" {
			provider = p
			break
		}
	}
	if s.Kind != "cloud" {
		s.Kind = "cloud"
		if provider == "" {
			provider = "Web"
		}
	}
	if provider != "" && (s.Provider == "Local" || s.Provider == "Unknown" || s.Provider == "Script" || s.Provider == "Web") {
		s.Provider = provider
	}
}

var ErrNotRunning = errors.New("that step isn't running")

// stopRunExecs stops the commands a run left running when it ends, so a log
// tail doesn't outlive its chat.
func (e *Engine) stopRunExecs(runID string) {
	e.execMu.Lock()
	var pids []int
	for _, x := range e.execByStep {
		if x.runID == runID && x.pid > 0 {
			x.stopped = "run"
			e.stoppedBy[x.stepID] = "run"
			pids = append(pids, x.pid)
		}
	}
	e.execMu.Unlock()
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

// StopStep stops a running command (a log tail, a long deploy) from the
// card: cloudbuddy exec passes the stop on to the whole command.
func (e *Engine) StopStep(ctx context.Context, stepID string) error {
	e.execMu.Lock()
	x := e.execByStep[stepID]
	if x != nil {
		x.stopped = "you"
		e.stoppedBy[x.stepID] = "you"
	}
	e.execMu.Unlock()
	if x == nil || x.pid <= 0 {
		return ErrNotRunning
	}
	if err := syscall.Kill(x.pid, syscall.SIGUSR1); err != nil { // cloudbuddy exec: "the user stopped this"
		return ErrNotRunning
	}
	if r := e.run(x.runID); r != nil {
		_ = e.st.AppendChat(ctx, r.ConvID, model.ChatItem{Type: "notice", Level: "info", Text: "You stopped a running command."})
		e.bus.Conversation(r.ConvID)
	}
	return nil
}

// ---------- shims (D-065) ----------

// SetShims tells the engine where per-run shim folders go and where the
// shims reach CloudBuddy (D-065).
func (e *Engine) SetShims(root, sock string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	e.shimRoot, e.shimSock = root, sock
	return nil
}

// SetDirenv pins the direnv CloudBuddy runs for each folder.
func (e *Engine) SetDirenv(bin string) { e.direnvBin = bin }

// ShimStart decides one CLI call a script made. It shows as its own card
// under the command that ran the script, and asks first like any call.
func (e *Engine) ShimStart(ctx context.Context, token, prog string, args []string, cwd, copy string) (bool, string, string) {
	if ok, reason, handled := e.dashShimStart(ctx, token, prog, args, cwd); handled {
		return ok, reason, ""
	}
	e.execMu.Lock()
	x := e.execByShim[token]
	e.execMu.Unlock()
	if x == nil {
		return false, "CloudBuddy doesn't know the command this came from; it may have ended.", ""
	}
	r := e.run(x.runID)
	if r == nil || r.ended.Load() {
		return false, "This run has ended.", ""
	}
	call := recognize.Call{Agent: r.Agent, Via: "shell", Command: shellJoin(prog, args), WantsNetwork: true, Cwd: cwd}
	if cwd == "" {
		call.Cwd = r.Workdir
	}
	// Quiet: the command's own card already shows this installed CLI call.
	// It only skips the card for a call that runs anyway; anything that
	// would ask goes through the gate (Codex review of D-065).
	if x.quiet && copy == "" {
		d := recognize.Recognize(call, e.env(r), e.adapters)
		if policy.Decide(d, policy.Context{Rule: e.ruleFor(ctx, d.Provider, ""), CanWrapCode: true}).Verdict == policy.Allow {
			return true, "", ""
		}
	}
	facts := [][2]string{}
	if copy != "" {
		facts = append(facts, [2]string{"Program", copy})
	}
	parent := ""
	if convID, key, err := e.st.StepConversation(ctx, x.stepID); err == nil {
		if p, ok, _ := e.st.StepByKey(ctx, convID, key); ok {
			parent = p.Title
		}
	}
	key := store.NewID("shim")
	o := e.gate(ctx, r, key, call, append(facts, [2]string{"Run by", parent})...)
	if !o.Allowed {
		return false, o.Reason, ""
	}
	if err := e.consume(ctx, r, o, key, call); err != nil {
		return false, "This approval was already used or no longer matches.", ""
	}
	return true, "", key
}

// ShimEnd records how a script's CLI call ended.
func (e *Engine) ShimEnd(token, key string, exitCode int, output string) {
	e.execMu.Lock()
	x := e.execByShim[token]
	e.execMu.Unlock()
	if x == nil || key == "" {
		return
	}
	r := e.run(x.runID)
	if r == nil {
		return
	}
	ctx := context.Background()
	out, masked, trunc := mask.Clean(output)
	st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, key, func(s *model.Step, isNew bool) {
		if isNew {
			return
		}
		now := time.Now().UTC()
		if s.Status != model.StatusDenied {
			s.Status = model.StatusDone
			if exitCode != 0 {
				s.Status = model.StatusFailed
			}
		}
		s.EndedAt = &now
		d := now.Sub(s.StartedAt).Milliseconds()
		s.DurationMs = &d
		s.Output, s.OutputTruncated = out, trunc
		if masked {
			s.MaskedNote = "CloudBuddy hid something that looked like a secret before saving this output."
		}
	})
	if err == nil && st.Status == model.StatusDone {
		e.recordLinks(ctx, r, st.ID, output)
	}
	e.bus.Conversation(r.ConvID)
}

// shellJoin quotes a CLI call so the recognizer reads back the same words.
func shellJoin(prog string, args []string) string { return shim.Join(prog, args) }

var _ = fmt.Sprintf

// stopNote is added to the output of a command CloudBuddy stopped.
func withStopNote(out, who string) string {
	note := ""
	switch who {
	case "you":
		note = "[You stopped this command.]"
	case "run":
		note = "[CloudBuddy stopped this command when the run ended.]"
	default:
		return out
	}
	if strings.Contains(out, note) {
		return out
	}
	return strings.TrimSpace(out + "\n\n" + note)
}

// stoppedStep says who stopped a step's command, if CloudBuddy did.
func (e *Engine) stoppedStep(stepID string) string {
	e.execMu.Lock()
	defer e.execMu.Unlock()
	return e.stoppedBy[stepID]
}
