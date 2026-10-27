package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cloudbuddy/internal/agent"
	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/picker"
	"cloudbuddy/internal/run"
	"cloudbuddy/internal/store"
)

type env struct {
	t      *testing.T
	base   string
	srv    *Server
	client *http.Client
	csrf   string
	work   string
}

func repoRoot() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..")
}

func newEnv(t *testing.T, scenario string) *env {
	t.Helper()
	ctx := context.Background()
	admin := "postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/cloudbuddy?sslmode=disable"
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	name := fmt.Sprintf("cb_srv_%d_%d", os.Getpid(), time.Now().UnixNano()%100000)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	st, err := store.Open(ctx, fmt.Sprintf("postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/%s?sslmode=disable", name))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	work := t.TempDir()
	cfg := config.Config{Port: ln.Addr().(*net.TCPAddr).Port, DataDir: data, Home: t.TempDir(), Bin: "/usr/local/bin/cloudbuddy",
		Epoch: config.Random(8), GateToken: config.Random(24)}
	b := bus.New()
	eng := run.New(cfg, st, b, nil)
	eng.UseFake(&agent.Fake{Scenario: scenario, Delay: 5 * time.Millisecond})
	srv := New(cfg, eng, b, nil, func() []Agent { return nil }, nil)
	hs := &http.Server{Handler: srv.Handler()}
	go hs.Serve(ln)
	t.Cleanup(func() {
		hs.Close()
		st.Close()
		c, err := pgx.Connect(ctx, admin)
		if err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			c.Close(ctx)
		}
	})
	jar, _ := cookiejar.New(nil)
	e := &env{t: t, base: fmt.Sprintf("http://127.0.0.1:%d", cfg.Port), srv: srv, work: work,
		client: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return e
}

func (e *env) do(method, path string, body any, hdr map[string]string) (int, []byte) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequest(method, e.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if method != "GET" {
		req.Header.Set("Origin", e.base)
		req.Header.Set("X-CB-CSRF", e.csrf)
	}
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := e.client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func (e *env) signIn() {
	code := e.srv.NewLaunchCode()
	st, _ := e.do("GET", "/launch/"+code, nil, nil)
	if st != http.StatusFound {
		e.t.Fatalf("launch: %d", st)
	}
	st, b := e.do("GET", "/api/session", nil, nil)
	if st != 200 {
		e.t.Fatalf("session: %d %s", st, b)
	}
	var s struct{ CsrfToken string }
	_ = json.Unmarshal(b, &s)
	e.csrf = s.CsrfToken
}

func (e *env) detail(id string) model.ConversationDetail {
	_, b := e.do("GET", "/api/conversations/"+id, nil, nil)
	var d model.ConversationDetail
	if err := json.Unmarshal(b, &d); err != nil {
		e.t.Fatalf("detail: %v %s", err, b)
	}
	return d
}

// waitFor polls until f returns true.
func (e *env) waitFor(id string, what string, f func(model.ConversationDetail) bool) model.ConversationDetail {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		d := e.detail(id)
		if f(d) {
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	d := e.detail(id)
	b, _ := json.MarshalIndent(d.Steps, "", " ")
	e.t.Fatalf("timed out waiting for %s; status=%s steps=%s", what, d.Status, b)
	return d
}

func waiting(d model.ConversationDetail) *model.Step {
	for i := range d.Steps {
		if d.Steps[i].Status == model.StatusWaiting && d.Steps[i].Approval != nil && d.Steps[i].Approval.State == "pending" {
			return &d.Steps[i]
		}
	}
	return nil
}

func TestDemoRunEndToEnd(t *testing.T) {
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "demo.json"))
	e.signIn()
	st, b := e.do("POST", "/api/conversations", map[string]string{"agent": "fake", "workdir": e.work, "text": "Deploy the site and clean up"}, nil)
	if st != 200 {
		t.Fatalf("create: %d %s", st, b)
	}
	var created struct{ ID string }
	_ = json.Unmarshal(b, &created)
	id := created.ID

	answers := map[string]string{
		"aws s3api create-bucket": "approve",
		"aws s3 sync":             "approve",
		"aws cloudfront create":   "approve",
		"aws s3 rb":               "deny",
		"stripe prices create":    "approve",
		"curl -s https://api.stripe.com/v1/prices/price_1QxR8sKj2LmN7pQ": "approve",
	}
	seen := map[string]bool{}
	for {
		d := e.waitFor(id, "a waiting step or the end", func(d model.ConversationDetail) bool {
			return waiting(d) != nil || d.Status == model.ConvDone || d.Status == model.ConvFailed
		})
		w := waiting(d)
		if w == nil {
			break
		}
		decision := ""
		for prefix, ans := range answers {
			if strings.HasPrefix(w.Command, prefix) || strings.Contains(w.Command, prefix) {
				decision = ans
			}
		}
		if decision == "" {
			t.Fatalf("unexpected approval for %q (%s)", w.Command, w.Approval.Reason)
		}
		seen[w.Command] = true
		st, b := e.do("POST", "/api/approvals/"+w.Approval.ID, map[string]string{"decision": decision}, nil)
		if st != 200 {
			t.Fatalf("decide %s: %d %s", w.Command, st, b)
		}
		// a second answer to the same approval is refused
		if st, _ := e.do("POST", "/api/approvals/"+w.Approval.ID, map[string]string{"decision": "approve"}, nil); st != 409 {
			t.Fatalf("second decision accepted: %d", st)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("expected 6 approvals, saw %d: %v", len(seen), seen)
	}
	d := e.detail(id)
	if d.Status != model.ConvDone {
		t.Fatalf("status %s", d.Status)
	}
	for _, s := range d.Steps {
		t.Logf("step %d %-6s %-8s %-9s %-40.40s row-reason=%v", s.Seq, s.Kind, s.Action, s.Status, s.Command, s.Approval != nil)
	}
	byCmd := map[string]model.Step{}
	for _, s := range d.Steps {
		byCmd[s.Command] = s
	}
	check := func(prefix string, kind string, status model.StepStatus) model.Step {
		for c, s := range byCmd {
			if strings.HasPrefix(c, prefix) {
				if s.Kind != kind || s.Status != status {
					t.Errorf("%s: kind=%s status=%s, want %s %s", prefix, s.Kind, s.Status, kind, status)
				}
				return s
			}
		}
		t.Errorf("no step for %s", prefix)
		return model.Step{}
	}
	check("aws sts get-caller-identity", "cloud", model.StatusDone)
	check("npm run build", "local", model.StatusDone)
	check("aws s3 rb", "cloud", model.StatusDenied)
	price := check("stripe prices create", "cloud", model.StatusDone)
	hasLive := false
	for _, tg := range price.Tags {
		if tg == model.TagLive {
			hasLive = true
		}
	}
	if !hasLive {
		t.Errorf("live price not tagged Live: %v", price.Tags)
	}
	curl := check("curl -s https://api.stripe.com", "cloud", model.StatusDone)
	if strings.Contains(curl.Command, "sk_live_51Hx9EXAMPLE") {
		t.Errorf("secret not masked in command: %s", curl.Command)
	}
	if len(curl.Uses) == 0 || curl.Uses[0].FromStepID != price.ID {
		t.Errorf("value link from the price step missing: %+v", curl.Uses)
	}
	if d.Result == nil || !strings.Contains(d.Result.Line, "stopped by you") {
		t.Errorf("result: %+v", d.Result)
	}
	if !strings.Contains(d.Subtitle, "AWS") || !strings.Contains(d.Subtitle, "Stripe") {
		t.Errorf("subtitle: %q", d.Subtitle)
	}
	// chat has the user message, agent text, step groups and approvals
	types := map[string]int{}
	for _, c := range d.Chat {
		types[c.Type]++
	}
	if types["user"] != 1 || types["agent"] < 2 || types["steps"] < 1 || types["approval"] < 5 {
		t.Errorf("chat items: %v", types)
	}
}

func TestLocalSecurity(t *testing.T) {
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "demo.json"))
	// no cookie
	if st, _ := e.do("GET", "/api/conversations", nil, nil); st != 401 {
		t.Errorf("no cookie: %d", st)
	}
	// launch code works once
	code := e.srv.NewLaunchCode()
	if st, _ := e.do("GET", "/launch/"+code, nil, nil); st != 302 {
		t.Errorf("launch: %d", st)
	}
	if st, _ := e.do("GET", "/launch/"+code, nil, nil); st != 403 {
		t.Errorf("launch code reused: %d", st)
	}
	e.signIn()
	// wrong Host (DNS rebinding)
	req, _ := http.NewRequest("GET", e.base+"/api/conversations", nil)
	req.Host = "evil.example.com"
	resp, _ := e.client.Do(req)
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("wrong host: %d", resp.StatusCode)
	}
	// cross-origin POST, missing Origin, bad CSRF
	body := map[string]string{"decision": "approve"}
	if st, _ := e.do("POST", "/api/approvals/x", body, map[string]string{"Origin": "https://evil.example.com"}); st != 403 {
		t.Errorf("cross-origin post: %d", st)
	}
	if st, _ := e.do("POST", "/api/approvals/x", body, map[string]string{"Origin": ""}); st != 403 {
		t.Errorf("post without origin: %d", st)
	}
	if st, _ := e.do("POST", "/api/approvals/x", body, map[string]string{"X-CB-CSRF": "wrong"}); st != 403 {
		t.Errorf("bad csrf: %d", st)
	}
	// gate endpoints need the gate secret, and the browser session isn't it
	if st, _ := e.do("POST", "/gate/release", map[string]string{"token": "x"}, nil); st != 403 {
		t.Errorf("gate without secret: %d", st)
	}
	// no CORS headers
	_, _ = e.do("GET", "/api/session", nil, nil)
	req, _ = http.NewRequest("OPTIONS", e.base+"/api/session", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, _ = e.client.Do(req)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("CORS header sent")
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Errorf("no CSP")
	}
	_ = url.URL{}
}

// A Connections rule changes what happens to the next step (Phase 6 exit).
func TestConnectionRuleApplies(t *testing.T) {
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "demo.json"))
	e.signIn()
	st := e.srv.eng.Store()
	if err := st.SaveConnections(context.Background(), []model.Connection{
		{ID: "cli:aws", Kind: "cli", Name: "AWS", Mark: "AW", How: "aws", Adapter: "builtin", Supported: true},
		{ID: "cli:stripe", Kind: "cli", Name: "Stripe", Mark: "St", How: "stripe", Adapter: "builtin", Supported: true},
	}); err != nil {
		t.Fatal(err)
	}
	if code, b := e.do("PUT", "/api/connections/cli:aws/rule", map[string]string{"rule": "readonly"}, nil); code != 200 {
		t.Fatalf("set rule: %d %s", code, b)
	}
	if code, _ := e.do("PUT", "/api/connections/cli:aws/rule", map[string]string{"rule": "anything"}, nil); code != 400 {
		t.Fatalf("bad rule accepted")
	}
	if code, b := e.do("PUT", "/api/connections/cli:stripe/rule", map[string]string{"rule": "off"}, nil); code != 200 {
		t.Fatalf("set rule: %d %s", code, b)
	}
	_, b := e.do("POST", "/api/conversations", map[string]string{"agent": "fake", "workdir": e.work, "text": "go"}, nil)
	var created struct{ ID string }
	_ = json.Unmarshal(b, &created)
	// read-only AWS: creates are denied without asking; reads still run.
	// Stripe off: denied. Only the curl to api.stripe.com and the AWS delete
	// would have asked; answer no to anything that does.
	for {
		d := e.waitFor(created.ID, "end", func(d model.ConversationDetail) bool {
			return waiting(d) != nil || d.Status == model.ConvDone || d.Status == model.ConvFailed
		})
		w := waiting(d)
		if w == nil {
			break
		}
		if strings.HasPrefix(w.Command, "aws s3api create-bucket") || strings.HasPrefix(w.Command, "stripe") {
			t.Fatalf("asked for something the rules deny: %s", w.Command)
		}
		e.do("POST", "/api/approvals/"+w.Approval.ID, map[string]string{"decision": "deny"}, nil)
	}
	d := e.detail(created.ID)
	for _, s := range d.Steps {
		switch {
		case strings.HasPrefix(s.Command, "aws s3api create-bucket"), strings.HasPrefix(s.Command, "stripe prices"):
			if s.Status != model.StatusDenied || s.Approval != nil {
				t.Errorf("%s: status %s approval %v, want denied without asking", s.Command, s.Status, s.Approval != nil)
			}
		case strings.HasPrefix(s.Command, "aws sts get-caller-identity"):
			if s.Status != model.StatusDone {
				t.Errorf("read under read-only rule: %s", s.Status)
			}
		}
	}
}

func TestProjectsAndSettings(t *testing.T) {
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "demo.json"))
	e.signIn()
	real := func(p string) string { r, _ := filepath.EvalSymlinks(p); return r }
	other := t.TempDir()

	st, b := e.do("GET", "/api/settings", nil, nil)
	if st != 200 || !strings.Contains(string(b), `"agent":"claude"`) || !strings.Contains(string(b), `"folder":""`) {
		t.Fatalf("default settings: %d %s", st, b)
	}
	// no folder yet and no project: the chat can't start
	if st, b := e.do("POST", "/api/conversations", map[string]string{"text": "hi"}, nil); st != 400 || !strings.Contains(string(b), "no_folder") {
		t.Fatalf("chat with no folder: %d %s", st, b)
	}
	if st, b := e.do("PUT", "/api/settings", map[string]string{"agent": "nope", "folder": e.work}, nil); st != 400 {
		t.Fatalf("unknown agent saved: %d %s", st, b)
	}
	if st, _ := e.do("PUT", "/api/settings", map[string]string{"agent": "fake", "folder": e.work}, map[string]string{"X-CB-CSRF": ""}); st != 403 {
		t.Fatalf("settings saved without the CSRF header: %d", st)
	}
	if st, b := e.do("PUT", "/api/settings", map[string]string{"agent": "fake", "folder": e.work}, nil); st != 200 || !strings.Contains(string(b), real(e.work)) {
		t.Fatalf("save settings: %d %s", st, b)
	}

	// projects: folders are checked and resolved
	if st, _ := e.do("POST", "/api/projects", map[string]any{"name": "Shop", "folders": []string{"relative/path"}}, nil); st != 400 {
		t.Fatalf("relative folder accepted: %d", st)
	}
	if st, _ := e.do("POST", "/api/projects", map[string]any{"name": " ", "folders": []string{e.work}}, nil); st != 400 {
		t.Fatalf("empty name accepted: %d", st)
	}
	st, b = e.do("POST", "/api/projects", map[string]any{"name": "Shop", "folders": []string{e.work, other, e.work}}, nil)
	if st != 200 {
		t.Fatalf("create project: %d %s", st, b)
	}
	var p model.Project
	_ = json.Unmarshal(b, &p)
	if len(p.Folders) != 2 || p.Folders[0] != real(e.work) || p.Folders[1] != real(other) {
		t.Fatalf("project folders: %+v", p)
	}

	// a chat in the project starts in its first folder and can use the others
	st, b = e.do("POST", "/api/conversations", map[string]string{"projectId": p.ID, "text": "Deploy the site"}, nil)
	if st != 200 {
		t.Fatalf("chat in project: %d %s", st, b)
	}
	var created struct{ ID string }
	_ = json.Unmarshal(b, &created)
	d := e.detail(created.ID)
	if d.Agent != "fake" || d.ProjectID != p.ID || d.Workdir != real(e.work) || len(d.ExtraFolders) != 1 || d.ExtraFolders[0] != real(other) {
		t.Fatalf("chat: agent=%s project=%s workdir=%s extra=%v", d.Agent, d.ProjectID, d.Workdir, d.ExtraFolders)
	}
	if st, _ := e.do("POST", "/api/conversations", map[string]string{"projectId": p.ID, "workdir": e.work, "text": "x"}, nil); st != 400 {
		t.Fatalf("project and folder together accepted: %d", st)
	}

	// rename, then delete: the chat stays, outside any project
	if st, b := e.do("PUT", "/api/projects/"+p.ID, map[string]any{"name": "Shop site", "folders": []string{e.work}}, nil); st != 200 || !strings.Contains(string(b), `"chatCount":1`) {
		t.Fatalf("rename: %d %s", st, b)
	}
	if st, _ := e.do("PUT", "/api/projects/pj_missing", map[string]any{"name": "x", "folders": []string{e.work}}, nil); st != 404 {
		t.Fatalf("update of a missing project: %d", st)
	}
	if st, _ := e.do("DELETE", "/api/projects/"+p.ID, nil, map[string]string{"X-CB-CSRF": ""}); st != 403 {
		t.Fatalf("delete without the CSRF header: %d", st)
	}
	if st, _ := e.do("DELETE", "/api/projects/"+p.ID, nil, map[string]string{"Origin": "http://evil.example"}); st != 403 {
		t.Fatalf("delete from another origin: %d", st)
	}
	if st, b := e.do("DELETE", "/api/projects/"+p.ID, nil, nil); st != 204 {
		t.Fatalf("delete: %d %s", st, b)
	}
	if d := e.detail(created.ID); d.ProjectID != "" || d.Workdir != real(e.work) {
		t.Fatalf("chat after delete: project=%s workdir=%s", d.ProjectID, d.Workdir)
	}
	// with no project, a new chat uses the folder from Settings
	st, b = e.do("POST", "/api/conversations", map[string]string{"text": "Check the bucket"}, nil)
	if st != 200 {
		t.Fatalf("chat from settings: %d %s", st, b)
	}
	_ = json.Unmarshal(b, &created)
	if d := e.detail(created.ID); d.Workdir != real(e.work) || len(d.ExtraFolders) != 0 || d.Agent != "fake" {
		t.Fatalf("chat from settings: workdir=%s extra=%v agent=%s", d.Workdir, d.ExtraFolders, d.Agent)
	}
}

func TestPickFolder(t *testing.T) {
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "demo.json"))
	e.signIn()
	var gotStart string
	answer := func(path string, err error) {
		e.srv.pick = func(_ context.Context, _, start string) (string, error) { gotStart = start; return path, err }
	}

	answer("/code/shop", nil)
	if st, b := e.do("POST", "/api/pick-folder", map[string]string{"start": "~/code"}, nil); st != 200 || !strings.Contains(string(b), `"path":"/code/shop"`) {
		t.Fatalf("pick: %d %s", st, b)
	}
	if gotStart != filepath.Join(e.srv.cfg.Home, "code") {
		t.Errorf("start folder: %q", gotStart)
	}
	if st, _ := e.do("POST", "/api/pick-folder", map[string]string{"start": "relative"}, nil); st != 200 || gotStart != e.srv.cfg.Home {
		t.Errorf("relative start should open at home, got %q", gotStart)
	}
	answer("", picker.ErrCancelled)
	if st, b := e.do("POST", "/api/pick-folder", nil, nil); st != 200 || !strings.Contains(string(b), `"cancelled":true`) {
		t.Fatalf("cancel: %d %s", st, b)
	}
	answer("", picker.ErrUnavailable)
	if st, b := e.do("POST", "/api/pick-folder", nil, nil); st != 501 || !strings.Contains(string(b), "no_picker") {
		t.Fatalf("no chooser: %d %s", st, b)
	}
	// a page can't open the chooser without the CSRF header, or from another origin
	answer("/code/shop", nil)
	if st, _ := e.do("POST", "/api/pick-folder", nil, map[string]string{"X-CB-CSRF": ""}); st != 403 {
		t.Fatalf("no CSRF header: %d", st)
	}
	if st, _ := e.do("POST", "/api/pick-folder", nil, map[string]string{"Origin": "http://evil.example"}); st != 403 {
		t.Fatalf("other origin: %d", st)
	}

	// one chooser at a time
	opened, release := make(chan struct{}), make(chan struct{})
	e.srv.pick = func(context.Context, string, string) (string, error) {
		close(opened)
		<-release
		return "/code/a", nil
	}
	done := make(chan int)
	go func() { st, _ := e.do("POST", "/api/pick-folder", nil, nil); done <- st }()
	<-opened
	if st, _ := e.do("POST", "/api/pick-folder", nil, nil); st != 409 {
		t.Errorf("second chooser while one is open: %d", st)
	}
	close(release)
	if st := <-done; st != 200 {
		t.Errorf("first chooser: %d", st)
	}
}
