// Package run is CloudBuddy's engine: it starts agent runs, turns their
// events into steps and chat, and makes every gate decision.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloudbuddy/internal/agent"
	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/shellenv"
	"cloudbuddy/internal/store"
)

type Engine struct {
	cfg      config.Config
	st       *store.Store
	bus      *bus.Bus
	adapters *recognize.Adapters
	claude   *agent.Claude
	codex    *agent.Codex
	fake     *agent.Fake

	mu      sync.Mutex
	runs    map[string]*Run // by run id
	active  map[string]*Run // by conversation id
	waiters map[string]chan bool

	hintsMu sync.Mutex
	hints   map[string]recognize.Hints // runID/server/tool

	// commands running through cloudbuddy exec (D-065)
	execMu      sync.Mutex
	execByToken map[string]*execState // release token hash
	execByShim  map[string]*execState // shim token
	execByStep  map[string]*execState
	stoppedBy   map[string]string // step id -> "you" | "run", for commands CloudBuddy stopped
	shimRoot    string            // a folder of CLI shims per run, first on every command's PATH
	shimSock    string            // where the shims reach CloudBuddy
	denyWrite   []string          // folders commands may not change (Protect)
	direnvBin   string            // direnv, found once at start-up
	loginPath   string            // the user's login-shell PATH (Protect)

	pinOnce sync.Once
	pinned  map[string]string

	// ClaudeCanary is true once Phase 2 confirmed SessionStart fires in -p
	// mode; only then is a missing canary treated as a bypass.
	ClaudeCanary bool
}

type Run struct {
	ID, ConvID, Agent, Workdir string
	Folders                    []string          // the project's other folders that still exist
	Env                        map[string]string // the agent's environment: login shell plus direnv (D-065)
	shimDir                    string            // this run's CLI shims
	cancel                     context.CancelFunc
	canary                     atomic.Bool
	ended                      atomic.Bool
	done                       chan struct{}
	gateways                   map[string]bool

	mu        sync.Mutex
	gated     map[string]bool     // tool keys the gate decided
	bgSteps   map[string]bool     // step ids of background commands
	codexPath map[string][]string // Codex file-change item -> paths
	escalated map[string]bool     // Codex items that got network
}

func New(cfg config.Config, st *store.Store, b *bus.Bus, adapters *recognize.Adapters) *Engine {
	return &Engine{cfg: cfg, st: st, bus: b, adapters: adapters,
		claude: &agent.Claude{Bin: cfg.Bin, DataDir: cfg.DataDir, Home: cfg.Home},
		codex:  &agent.Codex{DataDir: cfg.DataDir, Home: cfg.Home},
		runs:   map[string]*Run{}, active: map[string]*Run{}, waiters: map[string]chan bool{},
		hints: map[string]recognize.Hints{}, execByToken: map[string]*execState{}, execByShim: map[string]*execState{},
		execByStep: map[string]*execState{}, stoppedBy: map[string]string{}}
}

// SetClaude pins the claude program CloudBuddy starts.
func (e *Engine) SetClaude(exe string) { e.claude.Exe = exe }

// UseFake makes the "fake" agent available (dev demo and tests).
func (e *Engine) UseFake(f *agent.Fake) { e.fake = f }

func (e *Engine) Store() *store.Store { return e.st }

// Recover runs once at start-up: anything left running by a previous launch
// is marked interrupted and its approvals expire (R-10).
func (e *Engine) Recover(ctx context.Context) error {
	convs, err := e.st.InterruptOpenRuns(ctx)
	for _, c := range convs {
		_ = e.st.AppendChat(ctx, c, model.ChatItem{Type: "notice", Level: "warning",
			Text: "CloudBuddy restarted while this run was going. It was stopped, and anything waiting for approval was cancelled."})
	}
	return err
}

// projectFolders is the working folder and the project's other folders.
func (r *Run) projectFolders() []string {
	return append([]string{r.Workdir}, r.Folders...)
}

func (e *Engine) env(r *Run) recognize.Env {
	e.pinOnce.Do(func() {
		e.pinned = map[string]string{}
		for _, bin := range recognize.CloudCLINames() {
			if p, err := exec.LookPath(bin); err == nil {
				e.pinned[bin] = p
			}
		}
	})
	return recognize.Env{Home: e.cfg.Home, DataDir: e.cfg.DataDir, Port: e.cfg.Port, Workdir: r.Workdir, Folders: r.Folders,
		CloudBuddyBin: e.cfg.Bin, Pinned: e.pinned, AgentEnv: r.Env}
}

// ---------- conversations ----------

var ErrBusy = errors.New("a run is already going in this chat")

// ErrCodexSoon: Codex chats are paused while CloudBuddy works like native
// Claude Code (D-066).
var ErrCodexSoon = errors.New("Codex support is coming soon. Start a new chat with Claude Code.")

func titleFrom(text string) string {
	t := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if len(t) > 64 {
		t = t[:61] + "…"
	}
	if t == "" {
		t = "New chat"
	}
	return t
}

// Agents lists the agents a chat can use. Codex is coming soon (D-066).
func (e *Engine) Agents() []string {
	out := []string{"claude"}
	if e.fake != nil {
		out = append(out, "fake")
	}
	return out
}

func (e *Engine) validWorkdir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(e.cfg.Home, dir[2:])
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("use a full folder path, like /Users/you/code/project")
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("folder not found: %s", dir)
	}
	fi, err := os.Stat(real)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("not a folder: %s", dir)
	}
	if real == e.cfg.DataDir || strings.HasPrefix(real, e.cfg.DataDir+string(filepath.Separator)) || real == "/" {
		return "", fmt.Errorf("pick a project folder, not %s", real)
	}
	return real, nil
}

func (e *Engine) SendMessage(ctx context.Context, convID, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("the message is empty")
	}
	c, err := e.st.GetConversation(ctx, convID)
	if err != nil {
		return err
	}
	if c.Agent == "codex" {
		return ErrCodexSoon
	}
	e.mu.Lock()
	if _, busy := e.active[convID]; busy {
		e.mu.Unlock()
		return ErrBusy
	}
	r := &Run{ID: store.NewID("run"), ConvID: convID, Agent: c.Agent, Workdir: c.Workdir, Folders: existingDirs(c.ExtraFolders), done: make(chan struct{}),
		gated: map[string]bool{}, codexPath: map[string][]string{}, escalated: map[string]bool{}, gateways: map[string]bool{},
		bgSteps: map[string]bool{}}
	e.active[convID] = r
	e.runs[r.ID] = r
	e.mu.Unlock()

	masked, _ := mask.String(text)
	_ = e.st.AppendChat(ctx, convID, model.ChatItem{Type: "user", Text: masked})
	if err := e.st.CreateRun(ctx, r.ID, convID, e.cfg.Epoch, ""); err != nil {
		e.forget(r)
		return err
	}
	_ = e.st.SetConversationStatus(ctx, convID, model.ConvRunning)
	_ = e.st.SetResult(ctx, convID, nil, c.Subtitle)
	e.bus.Conversation(convID)

	runCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go e.drive(runCtx, r, c, text)
	return nil
}

func (e *Engine) forget(r *Run) {
	e.mu.Lock()
	delete(e.active, r.ConvID)
	delete(e.runs, r.ID)
	e.mu.Unlock()
}

func (e *Engine) Stop(ctx context.Context, convID string) error {
	e.mu.Lock()
	r := e.active[convID]
	e.mu.Unlock()
	if r == nil {
		return nil
	}
	_ = e.st.AppendChat(ctx, convID, model.ChatItem{Type: "notice", Level: "info", Text: "You stopped this run."})
	r.cancel()
	return nil
}

func (e *Engine) run(id string) *Run {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runs[id]
}

// drive starts the agent and consumes its events until it exits.
func (e *Engine) drive(ctx context.Context, r *Run, c store.Conversation, prompt string) {
	defer close(r.done)
	runDir := e.cfg.RunDir(r.ID)
	_ = os.MkdirAll(runDir, 0o700)
	events := make(chan agent.Event, 64)
	// the same setup as the user's terminal in this folder (D-065)
	// The .envrc runs inside the commands' sandbox: a command could have
	// changed it, or a file it sources (Codex review of D-065).
	env := e.agentEnv()
	if note := shellenv.Direnv(env, r.Workdir, e.direnvBin, e.profile()); note != "" {
		_ = e.st.AppendChat(context.Background(), r.ConvID, model.ChatItem{Type: "notice", Level: "warning", Text: note})
	}
	r.Env = env
	r.shimDir = e.runShims(r)
	opts := agent.StartOptions{RunID: r.ID, Workdir: r.Workdir, Folders: r.Folders, Prompt: prompt, SessionID: c.AgentSessionID,
		RunDir: runDir, Brief: agent.Brief, BaseEnv: env,
		Env: map[string]string{"CB_GATE_URL": e.cfg.GateURL(), "CB_GATE_TOKEN": e.cfg.GateToken, "CB_RUN_ID": r.ID}}
	if c.Agent == "claude" {
		opts.MCPServers = e.claudeMCPServers(r)
	} else {
		opts.MCP = e.gatewaySpecs(r, runDir)
	}
	if c.Model != "" && c.Agent != "fake" {
		opts.Model = c.Model
	}
	if m := os.Getenv("CB_" + strings.ToUpper(c.Agent) + "_MODEL"); m != "" && opts.Model == "" {
		opts.Model = m // cheap models for live tests (D-040)
	}
	opts.Approve = func(actx context.Context, method string, params json.RawMessage) (any, error) {
		return e.codexApproval(actx, r, method, params)
	}

	go func() {
		defer close(events)
		var err error
		switch c.Agent {
		case "claude":
			err = e.claude.Run(ctx, opts, events)
		case "codex":
			err = e.codex.Run(ctx, opts, events)
		case "fake":
			err = e.fake.RunWithGate(ctx, opts, e.fakeGate, events)
		}
		if err != nil && ctx.Err() == nil {
			events <- agent.Event{Kind: "done", Status: "failed", Text: err.Error()}
		}
	}()

	final := "failed"
	finalText := ""
	for ev := range events {
		if ev.Kind == "done" {
			if final == "failed" || ev.Status != "failed" {
				final, finalText = ev.Status, ev.Text
			}
			continue
		}
		e.handle(ctx, r, ev)
	}
	if ctx.Err() != nil {
		final = "interrupted"
	}
	e.finish(r, final, finalText)
}

func (e *Engine) finish(r *Run, status, text string) {
	ctx := context.Background()
	r.ended.Store(true)
	e.stopRunExecs(r.ID)
	if r.shimDir != "" {
		defer os.RemoveAll(r.shimDir)
	}
	_ = e.st.ExpireRunApprovals(ctx, r.ID)
	e.mu.Lock()
	for id, ch := range e.waiters {
		_ = id
		select {
		case ch <- false:
		default:
		}
	}
	e.mu.Unlock()
	conv := model.ConvDone
	switch status {
	case "completed":
	case "interrupted":
		conv = model.ConvInterrupted
	default:
		conv = model.ConvFailed
		if text != "" {
			masked, _ := mask.String(text)
			_ = e.st.AppendChat(ctx, r.ConvID, model.ChatItem{Type: "notice", Level: "error", Text: "The agent stopped with an error: " + masked})
		}
	}
	// running steps of a run that ended without a result: outcome unknown, never retried
	steps, _ := e.st.Steps(ctx, r.ConvID)
	for _, s := range steps {
		if s.Status == model.StatusRunning || s.Status == model.StatusWaiting {
			_, _, _ = e.updateStepByID(ctx, r, s.ID, func(st *model.Step) {
				if st.Status == model.StatusWaiting {
					st.Status = model.StatusDenied
				} else {
					st.Status = model.StatusOutcomeUnknown
				}
			})
		}
	}
	_ = e.st.FinishRun(ctx, r.ID, status, text)
	_ = e.st.SetConversationStatus(ctx, r.ConvID, conv)
	e.writeResult(ctx, r.ConvID, conv)
	e.forget(r)
	e.bus.Conversation(r.ConvID)
}

// gatewaySpecs builds the MCP server entries the agent gets: one CloudBuddy
// gateway per MCP server the user has set up (D-023).
func (e *Engine) gatewaySpecs(r *Run, runDir string) map[string]agent.MCPSpec {
	servers := e.userMCPServers(r.Agent)
	rules, _ := e.st.Rules(context.Background())
	out := map[string]agent.MCPSpec{}
	for name, spec := range servers {
		if rules["mcp:"+name] == model.RuleOff {
			continue // turned off in Connections: not started at all
		}
		specPath := filepath.Join(runDir, "mcp-"+safeName(name)+".json")
		if err := writeJSONFile(specPath, spec); err != nil {
			continue
		}
		out[name] = agent.MCPSpec{Command: e.cfg.Bin, Args: []string{"mcp-gateway", "--server", name, "--spec", specPath},
			Env: map[string]string{"CB_GATE_URL": e.cfg.GateURL(), "CB_GATE_TOKEN": e.cfg.GateToken, "CB_RUN_ID": r.ID}}
		r.gateways[name] = true
	}
	return out
}

func safeName(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// ---------- results ----------

func (e *Engine) writeResult(ctx context.Context, convID string, status model.ConvStatus) {
	steps, _ := e.st.Steps(ctx, convID)
	var created, changed, deleted, sent, denied int
	providers := []string{}
	seen := map[string]bool{}
	for _, s := range steps {
		if s.Kind != "cloud" {
			continue
		}
		if !seen[s.Provider] {
			seen[s.Provider] = true
			providers = append(providers, s.Provider)
		}
		if s.Status == model.StatusDenied {
			denied++
			continue
		}
		if s.Status != model.StatusDone {
			continue
		}
		switch s.Action {
		case model.ActionCreate:
			created++
		case model.ActionChange:
			changed++
		case model.ActionDelete:
			deleted++
		case model.ActionSend:
			sent++
		}
	}
	subtitle := "No cloud steps yet"
	switch len(providers) {
	case 0:
	case 1:
		subtitle = providers[0]
	default:
		subtitle = fmt.Sprintf("%d providers · %s", len(providers), strings.Join(providers, ", "))
	}
	if len(seen) == 0 {
		_ = e.st.SetResult(ctx, convID, nil, subtitle)
		return
	}
	var parts []string
	add := func(n int, word string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, word))
		}
	}
	add(created, "created")
	add(changed, "changed")
	add(deleted, "deleted")
	add(sent, "sent")
	add(denied, "stopped by you")
	line := strings.Join(parts, " · ")
	if line == "" {
		line = "Looked around; nothing was changed"
	}
	title := "Run finished"
	switch status {
	case model.ConvInterrupted:
		title = "Run stopped"
	case model.ConvFailed:
		title = "Run ended with an error"
	}
	_ = e.st.SetResult(ctx, convID, &model.RunResult{Title: title, Line: line, At: time.Now().UTC()}, subtitle)
}

// agentEnv is the user's login shell environment, or CloudBuddy's own in
// tests (CB_PLAIN_ENV), so test runs never pick up the real one.
func (e *Engine) agentEnv() map[string]string {
	if os.Getenv("CB_PLAIN_ENV") == "1" {
		env := map[string]string{}
		for _, kv := range os.Environ() {
			if i := strings.IndexByte(kv, '='); i > 0 {
				env[kv[:i]] = kv[i+1:]
			}
		}
		return env
	}
	return shellenv.Login()
}
