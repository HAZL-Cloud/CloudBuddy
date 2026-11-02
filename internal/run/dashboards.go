package run

// Dashboards (D-067, D-068): an agent saves a dashboard once, through
// CloudBuddy's own MCP tools; after that CloudBuddy runs its commands itself
// on a timer. Every run decides again whether the command may run with
// nobody watching (dash.Classify), and runs it like cloudbuddy exec would:
// the user's environment, the folder's direnv setup, the same sandbox
// profile, and CLI shims that stop anything that would ask in a chat.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/dash"
	"cloudbuddy/internal/mask"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/policy"
	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/sandbox"
	"cloudbuddy/internal/shellenv"
	"cloudbuddy/internal/shim"
	"cloudbuddy/internal/store"
)

// DashServer is the name the agent sees CloudBuddy's own MCP server under.
const DashServer = "cloudbuddy"

// dashSlots limits how many dashboard commands run at the same time.
var dashSlots = make(chan struct{}, 4)

// dashExec is one dashboard command running now. Its shim token lets the
// CLI shims inside it ask CloudBuddy.
type dashExec struct {
	dashID, key string
	mu          sync.Mutex
	stopped     []string // CLI calls the shims stopped
}

// ---------- the timer ----------

// StartDashboards refreshes dashboards when they are due, checking every
// `every`, until ctx ends. History older than 30 days is dropped hourly.
func (e *Engine) StartDashboards(ctx context.Context, every time.Duration) {
	go func() {
		tick := time.NewTicker(every)
		defer tick.Stop()
		var pruned time.Time
		for {
			e.runDue(ctx)
			if time.Since(pruned) > time.Hour {
				_ = e.st.PruneDashboardPoints(ctx, time.Now())
				pruned = time.Now()
			}
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
}

func (e *Engine) runDue(ctx context.Context) {
	ids, err := e.st.DueDashboards(ctx, time.Now())
	if err != nil {
		return
	}
	for _, id := range ids {
		if e.dashBusy(id) {
			continue
		}
		go func(id string) { _ = e.RefreshDashboard(ctx, id) }(id)
	}
}

func (e *Engine) dashBusy(id string) bool {
	e.dashMu.Lock()
	defer e.dashMu.Unlock()
	_, busy := e.dashRunning[id]
	return busy
}

// RefreshDashboard runs every command of a dashboard once and stores what
// came back. If a refresh is already going, it waits for that one.
func (e *Engine) RefreshDashboard(ctx context.Context, id string) error {
	e.dashMu.Lock()
	if ch, busy := e.dashRunning[id]; busy {
		e.dashMu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return nil
	}
	ch := make(chan struct{})
	e.dashRunning[id] = ch
	e.dashMu.Unlock()
	defer func() {
		e.dashMu.Lock()
		delete(e.dashRunning, id)
		e.dashMu.Unlock()
		close(ch)
		e.bus.Dashboard(id)
	}()

	d, err := e.st.GetDashboard(ctx, id)
	if err != nil {
		return err
	}
	var spec dash.Spec
	if err := json.Unmarshal(d.Spec, &spec); err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := e.st.StartDashboardRun(ctx, id, now.Add(time.Duration(d.RefreshSeconds)*time.Second)); err != nil {
		return err
	}
	e.bus.Dashboard(id) // the page shows "Refreshing…"
	approvals, _ := e.st.DashboardApprovals(ctx, id)
	envs := &dashEnvs{e: e, m: map[string]dashEnv{}}
	runs := make([]store.DashboardRun, len(spec.Sources))
	var wg sync.WaitGroup
	for i, src := range spec.Sources {
		wg.Add(1)
		go func(i int, src dash.Source) {
			defer wg.Done()
			runs[i] = e.dashSource(ctx, d, src, approvals, envs)
		}(i, src)
	}
	wg.Wait()
	bg := context.WithoutCancel(ctx)
	byKey := map[string]store.DashboardRun{}
	for _, r := range runs {
		if err := e.st.AddDashboardRun(bg, id, r); err != nil {
			return err
		}
		byKey[r.SourceKey] = r
	}
	values := map[string]float64{}
	for series, sp := range spec.Tracked() {
		r, ok := byKey[sp[0]]
		if !ok || r.State != dash.RunOK || r.Truncated {
			continue
		}
		src, _ := spec.Source(sp[0])
		v, err := dash.Parse(r.Output, src.Format)
		if err == nil {
			v, err = dash.Lookup(v, sp[1])
		}
		if err != nil {
			continue
		}
		if n, ok := dash.Number(v); ok {
			values[series] = n
		}
	}
	return e.st.AddDashboardPoints(bg, id, now, values)
}

// dashEnv is a folder's environment: the login shell plus its direnv setup.
type dashEnv struct {
	env  map[string]string
	note string
}

// dashEnvs works out each folder's environment once per refresh.
type dashEnvs struct {
	e  *Engine
	mu sync.Mutex
	m  map[string]dashEnv
}

func (de *dashEnvs) get(cwd string) (map[string]string, string) {
	de.mu.Lock()
	defer de.mu.Unlock()
	if v, ok := de.m[cwd]; ok {
		return v.env, v.note
	}
	env := de.e.agentEnv()
	note := shellenv.Direnv(env, cwd, de.e.direnvBin, de.e.profile())
	de.m[cwd] = dashEnv{env: env, note: note}
	return env, note
}

// dashCwd is where a command runs: the dashboard's folder, or a folder
// inside its project folders.
func dashCwd(d store.Dashboard, rel string) (string, error) {
	cwd := d.Workdir
	if rel = strings.TrimSpace(rel); rel != "" {
		if strings.HasPrefix(rel, "~") {
			return "", fmt.Errorf("the folder %s must be inside the project: give it relative to %s", rel, d.Workdir)
		}
		if filepath.IsAbs(rel) {
			cwd = filepath.Clean(rel)
		} else {
			cwd = filepath.Join(d.Workdir, rel)
		}
	}
	real, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", fmt.Errorf("the folder %s doesn't exist", cwd)
	}
	for _, f := range append([]string{d.Workdir}, d.Folders...) {
		if real == f || strings.HasPrefix(real, f+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("the folder %s is outside the project's folders", cwd)
}

// dashRun is the Run a dashboard command is checked against: its project
// folders and the environment it runs with.
func dashRun(d store.Dashboard, env map[string]string) *Run {
	return &Run{Workdir: d.Workdir, Folders: existingDirs(d.Folders), Env: env}
}

// classifyDash decides whether a command may run on the timer (D-067).
func (e *Engine) classifyDash(ctx context.Context, d store.Dashboard, command, cwd string, env map[string]string) dash.Class {
	r := dashRun(d, env)
	folders := r.projectFolders()
	return dash.Classify(dash.Input{Command: command, Cwd: cwd, Env: e.env(r), Adapters: e.adapters,
		Rule: func(p string) string { return e.ruleFor(ctx, p, "") },
		LocalProgram: func(cmd string) string {
			for _, prog := range recognize.Programs(cmd) {
				p := prog
				if strings.Contains(p, "/") {
					if !filepath.IsAbs(p) {
						p = filepath.Join(cwd, p)
					}
				} else if p = lookIn(env["PATH"], prog); p == "" {
					continue
				}
				if real, err := filepath.EvalSymlinks(p); err == nil {
					p = real
				}
				for _, f := range folders {
					if p == f || strings.HasPrefix(p, f+string(filepath.Separator)) {
						return prog
					}
				}
			}
			return ""
		}})
}

// dashSource decides and runs one command.
func (e *Engine) dashSource(ctx context.Context, d store.Dashboard, src dash.Source, approvals map[string]store.DashboardApproval, envs *dashEnvs) store.DashboardRun {
	run := store.DashboardRun{SourceKey: src.Key, StartedAt: time.Now().UTC()}
	cwd, err := dashCwd(d, src.Cwd)
	if err != nil {
		run.State, run.Note = dash.RunBlocked, "It can't run: "+err.Error()+"."
		return run
	}
	env, envNote := envs.get(cwd)
	class := e.classifyDash(ctx, d, src.Command, cwd, env)
	switch class.Verdict {
	case dash.Never:
		run.State, run.Note = dash.RunBlocked, class.Reason
		return run
	case dash.NeedsOK:
		a, ok := approvals[src.Key]
		if !ok || a.Command != src.Command || a.Cwd != cwd {
			run.State, run.Note = dash.RunWaiting, class.Reason
			return run
		}
		if !a.Allowed {
			run.State, run.Note = dash.RunDeclined, class.Reason
			return run
		}
		if a.ExecDigest != filesDigest(src.Command, cwd) {
			run.State, run.Note = dash.RunWaiting, "The files this command runs changed after you allowed it, so it needs your OK again. "+class.Reason
			return run
		}
	}
	e.execDash(ctx, d, src, cwd, env, class, &run)
	if envNote != "" && run.State != dash.RunOK {
		run.Note = strings.TrimSpace(run.Note + "\n" + envNote)
	}
	return run
}

// execDash runs one allowed command, the way cloudbuddy exec runs a chat's
// commands: zsh -f under the same sandbox profile, in a process group that
// is killed when it ends or runs too long.
func (e *Engine) execDash(ctx context.Context, d store.Dashboard, src dash.Source, cwd string, base map[string]string, class dash.Class, run *store.DashboardRun) {
	select {
	case dashSlots <- struct{}{}:
	case <-ctx.Done():
		run.State, run.Note = dash.RunFailed, "CloudBuddy stopped before this command could run."
		return
	}
	defer func() { <-dashSlots }()
	run.StartedAt = time.Now().UTC()

	env := make(map[string]string, len(base)+10)
	for k, v := range base {
		if !strings.HasPrefix(k, "CB_") {
			env[k] = v
		}
	}
	// no pagers or colours: a pager is a program the CLI would start
	env["AWS_PAGER"], env["PAGER"], env["GIT_PAGER"], env["PSQL_PAGER"] = "", "cat", "cat", "cat"
	env["NO_COLOR"] = "1"
	if class.SQL == "psql" {
		// a read-only session: the server refuses any change (D-068)
		env["PGOPTIONS"] = strings.TrimSpace(env["PGOPTIONS"] + " -c default_transaction_read_only=on")
	}
	x := &dashExec{dashID: d.ID, key: src.Key}
	if dir := e.dashShims(env["PATH"]); dir != "" && e.shimSock != "" {
		token := randomToken()
		e.dashMu.Lock()
		e.dashByShim[token] = x
		e.dashMu.Unlock()
		defer func() {
			e.dashMu.Lock()
			delete(e.dashByShim, token)
			e.dashMu.Unlock()
		}()
		env["PATH"] = dir + string(os.PathListSeparator) + env["PATH"]
		env[shim.EnvDir], env[shim.EnvSock], env[shim.EnvToken] = dir, e.shimSock, token
	}

	timeout := time.Duration(src.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = dash.DefTimeout * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	shell, args := "/bin/zsh", []string{"-f", "-c", src.Command}
	if _, err := os.Stat(shell); err != nil {
		shell, args = "/bin/sh", []string{"-c", src.Command}
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" && sandbox.CanWrap() {
		cmd = exec.Command("/usr/bin/sandbox-exec", append([]string{"-p", e.profile(), shell}, args...)...)
	} else {
		cmd = exec.Command(shell, args...)
	}
	cmd.Dir = cwd
	cmd.Env = shellenv.List(env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second // a process that escaped the group can't hold the output open
	out := &capBuf{max: 1 << 20}
	errb := &capBuf{max: 64 << 10, keepEnd: true}
	cmd.Stdout, cmd.Stderr = out, errb
	if err := cmd.Start(); err != nil {
		run.State, run.Note = dash.RunFailed, "CloudBuddy couldn't start the command: "+err.Error()
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-cctx.Done():
		timedOut = ctx.Err() == nil
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // anything it left running
	run.DurationMs = int(time.Since(run.StartedAt).Milliseconds())
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			if code < 0 {
				code = 137
			}
		} else {
			code = 1
		}
	}
	run.ExitCode = &code
	o, _ := mask.String(out.String())
	o, cut := mask.Cap(o, mask.MaxEventBytes)
	run.Output, run.Truncated = o, cut || out.cut
	note, _ := mask.String(strings.TrimSpace(errb.String()))
	switch {
	case timedOut:
		run.State = dash.RunFailed
		note = strings.TrimSpace(fmt.Sprintf("It ran longer than %d seconds, so CloudBuddy stopped it.\n%s", src.TimeoutSeconds, note))
	case ctx.Err() != nil:
		run.State, note = dash.RunFailed, "CloudBuddy stopped while this command ran."
	case code != 0:
		run.State = dash.RunFailed
	default:
		run.State = dash.RunOK
	}
	x.mu.Lock()
	stopped := x.stopped
	x.mu.Unlock()
	if len(stopped) > 0 {
		note = strings.TrimSpace("CloudBuddy stopped " + strings.Join(stopped, ", ") + " inside this command, because dashboards run with nobody watching.\n" + note)
	}
	run.Note, _ = mask.Cap(note, 8<<10)
}

// capBuf keeps the first max bytes written (or the last, with keepEnd).
type capBuf struct {
	b       bytes.Buffer
	max     int
	cut     bool
	keepEnd bool
}

func (c *capBuf) Write(p []byte) (int, error) {
	if c.keepEnd {
		c.b.Write(p)
		if c.b.Len() > c.max {
			rest := c.b.Bytes()[c.b.Len()-c.max:]
			c.b = *bytes.NewBuffer(append([]byte(nil), rest...))
			c.cut = true
		}
		return len(p), nil
	}
	if room := c.max - c.b.Len(); room > 0 {
		if len(p) > room {
			c.b.Write(p[:room])
			c.cut = true
		} else {
			c.b.Write(p)
		}
	} else if len(p) > 0 {
		c.cut = true
	}
	return len(p), nil
}

func (c *capBuf) String() string { return c.b.String() }

// dashShims is a folder of CLI shims for a PATH: one per cloud CLI found on
// it, made once.
func (e *Engine) dashShims(path string) string {
	if e.shimRoot == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(path))
	dir := filepath.Join(e.shimRoot, "dash-"+hex.EncodeToString(sum[:6]))
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	tmp := dir + ".tmp" + randomToken()[:6]
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return ""
	}
	for _, name := range recognize.ShimCLIs() {
		if lookIn(path, name) == "" {
			continue
		}
		_ = os.WriteFile(filepath.Join(tmp, name), []byte(shim.Script(e.cfg.Bin, name, lookIn(e.loginPath, name))), 0o755)
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		if _, err := os.Stat(dir); err != nil {
			return ""
		}
	}
	return dir
}

// dashShimStart decides a CLI call made inside a dashboard command. Nobody
// is there to ask, so anything that would ask in a chat is stopped; the
// rest runs without a card. handled is false for tokens of chat commands.
func (e *Engine) dashShimStart(ctx context.Context, token, prog string, args []string, cwd string) (ok bool, reason string, handled bool) {
	e.dashMu.Lock()
	x := e.dashByShim[token]
	e.dashMu.Unlock()
	if x == nil {
		return false, "", false
	}
	d, err := e.st.GetDashboard(ctx, x.dashID)
	if err != nil {
		return false, "CloudBuddy doesn't know this dashboard any more.", true
	}
	if cwd == "" {
		cwd = d.Workdir
	}
	call := recognize.Call{Agent: "claude", Via: "shell", Command: shellJoin(prog, args), WantsNetwork: true, Cwd: cwd}
	dr := recognize.Recognize(call, e.env(dashRun(d, nil)), e.adapters)
	dec := policy.Decide(dr, policy.Context{Rule: e.ruleFor(ctx, dr.Provider, ""), CanWrapCode: true})
	if dec.Verdict == policy.Allow {
		return true, "", true
	}
	what := shellJoin(prog, args)
	if len(what) > 80 {
		what = what[:77] + "…"
	}
	x.mu.Lock()
	x.stopped = append(x.stopped, "“"+maskStr(what)+"”")
	x.mu.Unlock()
	return false, "dashboards run with nobody watching, so CloudBuddy only lets reads through. " + dec.Reason, true
}

// ---------- the agent's tools ----------

// DashToolResult is what one of CloudBuddy's own MCP tools returns.
type DashToolResult struct {
	Text    string `json:"text"`
	IsError bool   `json:"isError"`
}

func dashErr(f string, a ...any) DashToolResult {
	return DashToolResult{Text: fmt.Sprintf(f, a...), IsError: true}
}

// dashboardHook is the gate for CloudBuddy's own MCP tools. Saving only
// stores a definition; every command is checked again each time it runs, so
// the call is allowed. A save gets a card in a "CloudBuddy" lane.
func (e *Engine) dashboardHook(ctx context.Context, r *Run, in HookInput, call recognize.Call) HookAnswer {
	key := in.ToolUseID
	if key == "" {
		return HookAnswer{Decision: "deny", Reason: "CloudBuddy needs the tool call's id."}
	}
	switch call.Tool {
	case "save_dashboard":
		title, _ := in.ToolInput["title"].(string)
		id, _ := in.ToolInput["id"].(string)
		refresh, _ := in.ToolInput["refresh"].(string)
		srcs, _ := in.ToolInput["sources"].([]any)
		tiles, _ := in.ToolInput["tiles"].([]any)
		action, verb := model.ActionCreate, "Built"
		if strings.TrimSpace(id) != "" {
			action, verb = model.ActionChange, "Changed"
		}
		if strings.TrimSpace(title) == "" {
			title = "a dashboard"
		} else {
			title = "“" + oneLineTitle(title) + "”"
		}
		st, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, key, func(s *model.Step, _ bool) {
			s.Kind, s.Provider, s.Product, s.Action = "cloud", "CloudBuddy", "Dashboards", action
			s.Tags = []model.Tag{}
			s.Title, s.TitleSource = maskStr(verb+" the dashboard "+title), "adapter"
			s.Via, s.API = "mcp", "cloudbuddy · save_dashboard"
			s.Command = maskStr(fmt.Sprintf("save_dashboard(%s, %d commands, %d tiles)", title, len(srcs), len(tiles)))
			s.Facts = [][2]string{{"Commands", strconv.Itoa(len(srcs))}, {"Tiles", strconv.Itoa(len(tiles))}}
			if refresh != "" {
				s.Facts = append(s.Facts, [2]string{"Refreshes", "every " + maskStr(refresh)})
			}
			s.RecognizedBy = []string{"builtin"}
			s.Status = model.StatusRunning
		})
		if err != nil {
			return HookAnswer{Decision: "deny", Reason: "CloudBuddy couldn't record this step: " + err.Error()}
		}
		e.dashMu.Lock()
		e.dashSaveKey[r.ID] = key
		e.dashMu.Unlock()
		r.mu.Lock()
		r.gated[key] = true
		r.mu.Unlock()
		_ = e.st.AddStepToChat(ctx, r.ConvID, st.ID)
		e.bus.Conversation(r.ConvID)
		return HookAnswer{Decision: "allow", Reason: "Saving a dashboard only stores it; CloudBuddy checks each command every time it runs."}
	case "list_dashboards", "get_dashboard":
		_, err := e.st.UpsertStep(ctx, r.ConvID, r.ID, key, func(s *model.Step, _ bool) {
			s.Kind, s.Provider, s.Product, s.Action = "local", "CloudBuddy", "Dashboards", model.ActionRead
			s.Tags = []model.Tag{}
			s.Title, s.TitleSource = "Looked at the dashboards", "adapter"
			s.Via, s.API = "mcp", "cloudbuddy · "+call.Tool
			args, _ := json.Marshal(call.Args)
			s.Command = maskStr(call.Tool + "(" + string(args) + ")")
			s.RecognizedBy = []string{"builtin"}
			s.Status = model.StatusRunning
		})
		if err != nil {
			return HookAnswer{Decision: "deny", Reason: "CloudBuddy couldn't record this step: " + err.Error()}
		}
		r.mu.Lock()
		r.gated[key] = true
		r.mu.Unlock()
		return HookAnswer{Decision: "allow", Reason: "Reading dashboards changes nothing."}
	}
	return HookAnswer{Decision: "deny", Reason: "CloudBuddy has no tool called " + call.Tool + "."}
}

func oneLineTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 80 {
		s = string([]rune(s)[:77]) + "…"
	}
	return s
}

// DashboardTool runs one of CloudBuddy's own MCP tools for a chat's agent.
func (e *Engine) DashboardTool(ctx context.Context, runID, toolUseID, tool string, args json.RawMessage) DashToolResult {
	r := e.run(runID)
	if r == nil || r.ended.Load() {
		return dashErr("CloudBuddy doesn't know this chat's run; it may have ended.")
	}
	switch tool {
	case "save_dashboard":
		return e.dashSave(ctx, r, toolUseID, args)
	case "list_dashboards":
		return e.dashList(ctx)
	case "get_dashboard":
		var in struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(args, &in)
		return e.dashGet(ctx, strings.TrimSpace(in.ID))
	}
	return dashErr("CloudBuddy has no tool called %s.", tool)
}

func (e *Engine) dashSave(ctx context.Context, r *Run, toolUseID string, args json.RawMessage) DashToolResult {
	var in struct {
		ID string `json:"id"`
		dash.Spec
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return dashErr("The dashboard wasn't saved: its fields don't have the right types (%v).", err)
	}
	if err := dash.Check(&in.Spec); err != nil {
		return dashErr("%s", err.Error())
	}
	every, _ := dash.ParseRefresh(in.Spec.Refresh)
	where := store.Dashboard{Workdir: r.Workdir, Folders: r.Folders}
	var probs dash.Problems
	for _, src := range in.Spec.Sources {
		if _, err := dashCwd(where, src.Cwd); err != nil {
			probs = append(probs, fmt.Sprintf("source %q: %s", src.Key, err))
		}
	}
	if len(probs) > 0 {
		return dashErr("%s", probs.Error())
	}
	id := strings.TrimSpace(in.ID)
	isNew := id == ""
	if isNew {
		id = store.NewID("db")
	} else if _, err := e.st.GetDashboard(ctx, id); err != nil {
		return dashErr("There's no dashboard with id %q. Use list_dashboards to find it, or leave id out to make a new one.", id)
	}
	b, _ := json.Marshal(in.Spec)
	if err := e.st.SaveDashboard(ctx, store.Dashboard{ID: id, Title: in.Spec.Title, Description: in.Spec.Description,
		ConversationID: r.ConvID, Workdir: r.Workdir, Folders: r.Folders, RefreshSeconds: int(every / time.Second), Spec: b}); err != nil {
		return dashErr("CloudBuddy couldn't save the dashboard: %v", err)
	}
	var keys []string
	for _, src := range in.Spec.Sources {
		keys = append(keys, src.Key)
	}
	_ = e.st.KeepDashboardApprovals(ctx, id, keys)

	// the save's card on the canvas opens the dashboard
	key := toolUseID
	if key == "" {
		e.dashMu.Lock()
		key = e.dashSaveKey[r.ID]
		e.dashMu.Unlock()
	}
	if key != "" {
		if _, found, _ := e.st.StepByKey(ctx, r.ConvID, key); found {
			_, _ = e.st.UpsertStep(ctx, r.ConvID, r.ID, key, func(s *model.Step, _ bool) {
				s.App = &model.StepApp{Kind: "dashboard", ID: id, Title: in.Spec.Title}
				s.Resource = maskStr(in.Spec.Title)
			})
			e.bus.Conversation(r.ConvID)
		}
	}
	e.bus.Publish(bus.Event{Type: "dashboards"})

	// The first refresh runs now, so the agent sees what the page will show.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	_ = e.RefreshDashboard(rctx, id)
	det, err := e.DashboardDetail(ctx, id, "")
	if err != nil {
		return DashToolResult{Text: "Saved the dashboard (id " + id + "), but CloudBuddy couldn't read it back: " + err.Error()}
	}
	return DashToolResult{Text: dashReport(det, isNew)}
}

// dashReport tells the agent what the user will see, so it can fix mistakes.
func dashReport(d dash.Detail, isNew bool) string {
	var b strings.Builder
	verb := "Saved the new"
	if !isNew {
		verb = "Updated the"
	}
	fmt.Fprintf(&b, "%s dashboard “%s” (id %s). CloudBuddy refreshes it every %s while it runs. The user opens it from the card on the canvas or from Dashboards in the sidebar.\n\n", verb, d.Title, d.ID, d.Refresh)
	b.WriteString("Commands, first run:\n")
	waiting, broken := 0, 0
	for _, s := range d.Sources {
		fmt.Fprintf(&b, "- %s: ", s.Key)
		switch {
		case s.LastRun == nil:
			b.WriteString("not run yet.\n")
		case s.LastRun.State == dash.RunOK:
			fmt.Fprintf(&b, "OK in %.1fs. Output: %s\n", float64(s.LastRun.DurationMs)/1000, preview(s.LastRun.Output, 300))
		case s.LastRun.State == dash.RunWaiting:
			waiting++
			fmt.Fprintf(&b, "waiting for the user's OK on the dashboard page. %s\n", s.Reason)
		case s.LastRun.State == dash.RunBlocked:
			broken++
			fmt.Fprintf(&b, "can't run from a dashboard. %s Change it to a command that only reads.\n", s.Reason)
		case s.LastRun.State == dash.RunDeclined:
			fmt.Fprintf(&b, "the user didn't allow this command.\n")
		default:
			broken++
			code := ""
			if s.LastRun.ExitCode != nil {
				code = fmt.Sprintf(" (exit code %d)", *s.LastRun.ExitCode)
			}
			fmt.Fprintf(&b, "failed%s. %s Output: %s\n", code, preview(s.LastRun.Note, 400), preview(s.LastRun.Output, 200))
		}
	}
	b.WriteString("\nTiles:\n")
	for i, t := range d.Tiles {
		v := d.Values[i]
		fmt.Fprintf(&b, "- %s (%s): ", t.Title, t.Type)
		switch v.State {
		case "ok":
			b.WriteString(tileSummary(v))
		case "error":
			broken++
			b.WriteString("ERROR: " + v.Message)
		default:
			b.WriteString(v.State + ". " + v.Message)
		}
		b.WriteString("\n")
	}
	switch {
	case broken > 0:
		fmt.Fprintf(&b, "\nFix what failed and save again with id %q. Test commands in the shell first.", d.ID)
	case waiting > 0:
		b.WriteString("\nTell the user that some commands wait for their OK on the dashboard page: CloudBuddy can't tell they only read, so they don't run on the timer until the user allows them.")
	default:
		b.WriteString("\nEverything worked. Tell the user where to find the dashboard.")
	}
	return b.String()
}

func tileSummary(v dash.TileValue) string {
	switch {
	case v.Status != "":
		return "status " + v.Status + " (" + v.Text + ")"
	case v.Number != nil:
		return strconv.FormatFloat(*v.Number, 'f', -1, 64)
	case len(v.Bars) > 0:
		return fmt.Sprintf("%d bars, first %q = %v", len(v.Bars), v.Bars[0].Label, v.Bars[0].Value)
	case len(v.Rows) > 0:
		return fmt.Sprintf("%d rows, %d columns", len(v.Rows)+v.More, len(v.Columns))
	case len(v.Series) > 0:
		n := 0
		for _, s := range v.Series {
			n += len(s.Points) + len(s.Bars)
		}
		return fmt.Sprintf("%d lines, %d points so far (a line over time fills in as CloudBuddy refreshes)", len(v.Series), n)
	case v.Text != "":
		return "text: " + preview(v.Text, 120)
	}
	return "OK"
}

func preview(s string, n int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(nothing)"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n]) + "…"
	}
	return s
}

func (e *Engine) dashList(ctx context.Context) DashToolResult {
	all, err := e.st.Dashboards(ctx)
	if err != nil {
		return dashErr("CloudBuddy couldn't list the dashboards: %v", err)
	}
	if len(all) == 0 {
		return DashToolResult{Text: "There are no dashboards yet. Use save_dashboard to make one."}
	}
	var b strings.Builder
	for _, d := range all {
		var spec dash.Spec
		_ = json.Unmarshal(d.Spec, &spec)
		fmt.Fprintf(&b, "- id %s: “%s”, refreshes every %s, folder %s\n", d.ID, d.Title, dash.RefreshLabel(time.Duration(d.RefreshSeconds)*time.Second), d.Workdir)
		for _, s := range spec.Sources {
			fmt.Fprintf(&b, "    source %s: %s\n", s.Key, maskStr(s.Command))
		}
		for _, t := range spec.Tiles {
			fmt.Fprintf(&b, "    tile %s: %s “%s”\n", t.Key, t.Type, t.Title)
		}
	}
	b.WriteString("\nUse get_dashboard with an id for the full definition, then save_dashboard with that id to change it.")
	return DashToolResult{Text: b.String()}
}

func (e *Engine) dashGet(ctx context.Context, id string) DashToolResult {
	d, err := e.st.GetDashboard(ctx, id)
	if err != nil {
		return dashErr("There's no dashboard with id %q. Use list_dashboards to find it.", id)
	}
	var spec map[string]any
	_ = json.Unmarshal(d.Spec, &spec)
	spec["id"] = d.ID
	spec["refresh"] = dash.RefreshLabel(time.Duration(d.RefreshSeconds) * time.Second)
	b, _ := json.MarshalIndent(spec, "", "  ")
	det, err := e.DashboardDetail(ctx, id, "")
	text := "Definition (send it back to save_dashboard, changed, to update the dashboard):\n" + maskStr(string(b))
	if err == nil {
		text += "\n\nWhat it shows now:\n" + dashReport(det, false)
	}
	return DashToolResult{Text: text}
}

// ---------- the browser's side ----------

// Dashboards lists every dashboard for the Dashboards page.
func (e *Engine) Dashboards(ctx context.Context) ([]dash.Summary, error) {
	all, err := e.st.Dashboards(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dash.Summary, 0, len(all))
	for _, d := range all {
		det, err := e.dashDetail(ctx, d, "")
		if err != nil {
			return nil, err
		}
		out = append(out, det.Summary)
	}
	return out, nil
}

// DashboardDetail is one dashboard's page, with charts over rangeKey.
func (e *Engine) DashboardDetail(ctx context.Context, id, rangeKey string) (dash.Detail, error) {
	d, err := e.st.GetDashboard(ctx, id)
	if err != nil {
		return dash.Detail{}, err
	}
	return e.dashDetail(ctx, d, rangeKey)
}

func (e *Engine) dashDetail(ctx context.Context, d store.Dashboard, rangeKey string) (dash.Detail, error) {
	var spec dash.Spec
	if err := json.Unmarshal(d.Spec, &spec); err != nil {
		return dash.Detail{}, err
	}
	latest, lastOK, err := e.st.LatestDashboardRuns(ctx, d.ID)
	if err != nil {
		return dash.Detail{}, err
	}
	approvals, err := e.st.DashboardApprovals(ctx, d.ID)
	if err != nil {
		return dash.Detail{}, err
	}
	runs := map[string]dash.Runs{}
	for _, src := range spec.Sources {
		var r dash.Runs
		if x, ok := latest[src.Key]; ok {
			r.Latest = runView(x, 0)
		}
		if x, ok := lastOK[src.Key]; ok {
			r.LastOK = runView(x, 0)
		}
		runs[src.Key] = r
	}
	rg := dash.RangeFor(rangeKey)
	now := time.Now()
	var series []string
	for k := range spec.Tracked() {
		series = append(series, k)
	}
	sort.Strings(series)
	pts, err := e.st.DashboardPoints(ctx, d.ID, series, now.Add(-rg.Span), 120)
	if err != nil {
		return dash.Detail{}, err
	}
	h := &dashHistory{ctx: ctx, st: e.st, id: d.ID, now: now, points: pts, marks: map[string]store.DashboardMarks{}}
	values := dash.Values(spec, runs, h, now)

	det := dash.Detail{Workdir: d.Workdir, Range: rg.Key, Tiles: spec.Tiles, Values: values, Choices: dash.RefreshChoices, Ranges: dash.Ranges}
	det.Summary = dash.Summary{ID: d.ID, Title: d.Title, Description: d.Description, ConversationID: d.ConversationID,
		Refresh: dash.RefreshLabel(time.Duration(d.RefreshSeconds) * time.Second), Paused: d.Paused, Running: e.dashBusy(d.ID),
		LastRunAt: d.LastRunAt, UpdatedAt: d.UpdatedAt, TileCount: len(spec.Tiles), Headlines: []dash.Headline{}}
	if !d.Paused {
		next := d.NextRunAt
		det.NextRunAt = &next
	}
	for _, src := range spec.Sources {
		sv := dash.SourceView{Key: src.Key, Label: src.Label, Command: maskStr(src.Command), Format: src.Format}
		sv.Cwd, _ = dashCwd(d, src.Cwd)
		if sv.Cwd == "" {
			sv.Cwd = src.Cwd
		}
		a, hasA := approvals[src.Key]
		sameCmd := hasA && a.Command == src.Command && a.Cwd == sv.Cwd
		if x, ok := latest[src.Key]; ok {
			sv.LastRun = runView(x, 16<<10)
			switch x.State {
			case dash.RunBlocked:
				sv.Verdict, sv.Reason = dash.Never, x.Note
				det.Failing++
			case dash.RunWaiting:
				sv.Verdict, sv.Reason = dash.NeedsOK, x.Note
				det.NeedsOK++
				if hasA {
					sv.Approval = &dash.Approval{Allowed: a.Allowed, DecidedAt: a.DecidedAt, Changed: true}
				}
			case dash.RunDeclined:
				sv.Verdict, sv.Reason = dash.NeedsOK, x.Note
				sv.Approval = &dash.Approval{Allowed: false, DecidedAt: a.DecidedAt}
			default:
				sv.Verdict = dash.Read
				if sameCmd && a.Allowed {
					sv.Verdict = dash.NeedsOK
					sv.Approval = &dash.Approval{Allowed: true, DecidedAt: a.DecidedAt}
				}
				if x.State == dash.RunFailed {
					det.Failing++
				}
			}
		}
		det.Sources = append(det.Sources, sv)
	}
	for i, t := range spec.Tiles {
		if t.Type == "number" && len(det.Headlines) < 3 {
			det.Headlines = append(det.Headlines, dash.Headline{Tile: t, Value: values[i]})
		}
	}
	return det, nil
}

func runView(r store.DashboardRun, maxOut int) *dash.RunView {
	v := &dash.RunView{State: r.State, ExitCode: r.ExitCode, Output: r.Output, Truncated: r.Truncated, Note: r.Note, At: r.StartedAt, DurationMs: r.DurationMs}
	if maxOut > 0 && len(v.Output) > maxOut {
		v.Output = v.Output[:maxOut] + "\n…"
	}
	return v
}

// dashHistory reads a dashboard's numbers for dash.Values.
type dashHistory struct {
	ctx    context.Context
	st     *store.Store
	id     string
	now    time.Time
	points map[string][]store.DashboardPoint
	marks  map[string]store.DashboardMarks
}

func (h *dashHistory) Points(series string) []dash.Point {
	src := h.points[series]
	out := make([]dash.Point, len(src))
	for i, p := range src {
		out[i] = dash.Point{T: p.T, V: p.V}
	}
	return out
}

func (h *dashHistory) mark(series string) store.DashboardMarks {
	if m, ok := h.marks[series]; ok {
		return m
	}
	m, _ := h.st.DashboardMarks(h.ctx, h.id, series, h.now)
	h.marks[series] = m
	return m
}

func point(p *store.DashboardPoint) *dash.Point {
	if p == nil {
		return nil
	}
	return &dash.Point{T: p.T, V: p.V}
}

func (h *dashHistory) Before(series string, d time.Duration) *dash.Point {
	return point(h.mark(series).Before[d])
}
func (h *dashHistory) Oldest(series string) *dash.Point   { return point(h.mark(series).Oldest) }
func (h *dashHistory) Previous(series string) *dash.Point { return point(h.mark(series).Previous) }

// SetDashboardSchedule changes how often a dashboard refreshes, or pauses it.
func (e *Engine) SetDashboardSchedule(ctx context.Context, id, refresh string, paused bool) error {
	every, err := dash.ParseRefresh(refresh)
	if err != nil {
		return err
	}
	if err := e.st.SetDashboardSchedule(ctx, id, int(every/time.Second), paused); err != nil {
		return err
	}
	e.bus.Dashboard(id)
	return nil
}

// AllowDashboardSource records the user's answer for a command that isn't
// a read, tied to the command, its folder and the files it runs now. It
// can't make a command that can never run from a dashboard run: that is
// decided again on every run.
func (e *Engine) AllowDashboardSource(ctx context.Context, id, key string, allow bool) error {
	d, err := e.st.GetDashboard(ctx, id)
	if err != nil {
		return err
	}
	var spec dash.Spec
	if err := json.Unmarshal(d.Spec, &spec); err != nil {
		return err
	}
	src, ok := spec.Source(key)
	if !ok {
		return store.ErrNotFound
	}
	cwd, err := dashCwd(d, src.Cwd)
	if err != nil {
		return err
	}
	if err := e.st.SetDashboardApproval(ctx, id, store.DashboardApproval{SourceKey: key, Command: src.Command, Cwd: cwd,
		ExecDigest: filesDigest(src.Command, cwd), Allowed: allow}); err != nil {
		return err
	}
	e.bus.Dashboard(id)
	e.RefreshDashboardSoon(id)
	return nil
}

// RefreshDashboardSoon starts a refresh in the background.
func (e *Engine) RefreshDashboardSoon(id string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_ = e.RefreshDashboard(ctx, id)
	}()
}

// DeleteDashboard removes a dashboard and its history.
func (e *Engine) DeleteDashboard(ctx context.Context, id string) error {
	if err := e.st.DeleteDashboard(ctx, id); err != nil {
		return err
	}
	e.bus.Publish(bus.Event{Type: "dashboards"})
	return nil
}

// dashMCPServer is how Claude Code starts CloudBuddy's own MCP server.
func (e *Engine) dashMCPServer(r *Run) map[string]any {
	return map[string]any{"type": "stdio", "command": e.cfg.Bin, "args": []string{"dashboards-mcp"},
		"env": map[string]string{"CB_GATE_URL": e.cfg.GateURL(), "CB_GATE_TOKEN": e.cfg.GateToken, "CB_RUN_ID": r.ID}}
}
