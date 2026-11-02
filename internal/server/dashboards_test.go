package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"cloudbuddy/internal/dash"
	"cloudbuddy/internal/model"
)

func (e *env) dashboard(id string) dash.Detail {
	st, b := e.do("GET", "/api/dashboards/"+id, nil, nil)
	if st != 200 {
		e.t.Fatalf("dashboard %s: %d %s", id, st, b)
	}
	var d dash.Detail
	if err := json.Unmarshal(b, &d); err != nil {
		e.t.Fatalf("dashboard: %v %s", err, b)
	}
	return d
}

func (e *env) waitDash(id, what string, f func(dash.Detail) bool) dash.Detail {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		d := e.dashboard(id)
		if !d.Running && f(d) {
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	d := e.dashboard(id)
	b, _ := json.MarshalIndent(d.Sources, "", " ")
	e.t.Fatalf("timed out waiting for %s: %s", what, b)
	return d
}

func source(d dash.Detail, key string) dash.SourceView {
	for _, s := range d.Sources {
		if s.Key == key {
			return s
		}
	}
	return dash.SourceView{}
}

func value(d dash.Detail, key string) dash.TileValue {
	for _, v := range d.Values {
		if v.Key == key {
			return v
		}
	}
	return dash.TileValue{}
}

// A chat builds a dashboard; CloudBuddy runs its commands without the agent:
// reads run, a script waits for one OK, a delete never runs (D-067).
func TestDashboardEndToEnd(t *testing.T) {
	t.Setenv("CB_PLAIN_ENV", "1")
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "dashboard.json"))
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(e.work, "status.json"), []byte(`{"state":"healthy","regions":[{"name":"us-east-1","ok":true},{"name":"eu-west-1","ok":false}]}`), 0o644))
	must(os.WriteFile(filepath.Join(e.work, "count-users.sh"), []byte("#!/bin/sh\necho 1204\n"), 0o755))

	st, b := e.do("POST", "/api/conversations", map[string]string{"agent": "fake", "workdir": e.work, "text": "Make me a dashboard"}, nil)
	if st != 200 {
		t.Fatalf("create: %d %s", st, b)
	}
	var created struct{ ID string }
	_ = json.Unmarshal(b, &created)
	conv := e.waitFor(created.ID, "the run to end", func(d model.ConversationDetail) bool {
		return d.Status == model.ConvDone || d.Status == model.ConvFailed
	})
	if conv.Status != model.ConvDone {
		t.Fatalf("run status %s", conv.Status)
	}
	var app *model.StepApp
	for _, s := range conv.Steps {
		if s.App != nil {
			app = s.App
			if s.Provider != "CloudBuddy" || s.Kind != "cloud" || s.Action != model.ActionCreate {
				t.Errorf("save card: %+v", s)
			}
		}
	}
	if app == nil || app.Kind != "dashboard" || app.Title != "Project pulse" {
		t.Fatalf("the save step should point to the dashboard: %+v", conv.Steps)
	}

	st, b = e.do("GET", "/api/dashboards", nil, nil)
	var list []dash.Summary
	_ = json.Unmarshal(b, &list)
	if st != 200 || len(list) != 1 || list[0].ID != app.ID || list[0].ConversationID != created.ID {
		t.Fatalf("list: %d %s", st, b)
	}

	d := e.dashboard(app.ID)
	if s := source(d, "files"); s.Verdict != dash.Read || s.LastRun == nil || s.LastRun.State != dash.RunOK {
		t.Errorf("files: %+v %+v", s, s.LastRun)
	}
	if v := value(d, "files"); v.State != "ok" || v.Number == nil || *v.Number != 2 {
		t.Errorf("files tile: %+v", v)
	}
	if v := value(d, "app"); v.Status != "ok" || v.Text != "healthy" {
		t.Errorf("status tile: %+v", v)
	}
	if v := value(d, "regions"); len(v.Rows) != 2 {
		t.Errorf("table tile: %+v", v)
	}
	users := source(d, "users")
	if users.Verdict != dash.NeedsOK || users.LastRun.State != dash.RunWaiting {
		t.Fatalf("a script should wait for the OK: %+v %+v", users, users.LastRun)
	}
	if v := value(d, "users"); v.State != "waiting" {
		t.Errorf("users tile: %+v", v)
	}
	if s := source(d, "cleanup"); s.Verdict != dash.Never || !strings.Contains(s.Reason, "delete") {
		t.Errorf("a delete can never run: %+v", s)
	}
	if d.NeedsOK != 1 || d.Failing != 1 {
		t.Errorf("counts: needsOk=%d failing=%d", d.NeedsOK, d.Failing)
	}

	// an answer for a command the page didn't show is refused
	if st, _ := e.do("POST", "/api/dashboards/"+app.ID+"/sources/users/decision", map[string]string{"decision": "allow", "command": "./other.sh", "cwd": users.Cwd}, nil); st != 409 {
		t.Fatalf("allowing a command the page didn't show: %d", st)
	}
	// the user allows the script: it runs
	if st, b := e.do("POST", "/api/dashboards/"+app.ID+"/sources/users/decision", map[string]string{"decision": "allow", "command": users.Command, "cwd": users.Cwd}, nil); st != 200 {
		t.Fatalf("allow: %d %s", st, b)
	}
	d = e.waitDash(app.ID, "the script to run", func(d dash.Detail) bool {
		s := source(d, "users")
		return s.LastRun != nil && s.LastRun.State == dash.RunOK
	})
	if v := value(d, "users"); v.Number == nil || *v.Number != 1204 {
		t.Errorf("users after OK: %+v", v)
	}
	if s := source(d, "users"); s.Approval == nil || !s.Approval.Allowed {
		t.Errorf("approval: %+v", s.Approval)
	}
	// allowing can't make a delete run
	cleanup := source(d, "cleanup")
	before := cleanup.LastRun.At
	if st, _ := e.do("POST", "/api/dashboards/"+app.ID+"/sources/cleanup/decision", map[string]string{"decision": "allow", "command": cleanup.Command, "cwd": cleanup.Cwd}, nil); st != 200 {
		t.Fatalf("allow cleanup: %d", st)
	}
	d = e.waitDash(app.ID, "the refresh after the OK", func(d dash.Detail) bool { return source(d, "cleanup").LastRun.At.After(before) })
	if s := source(d, "cleanup"); s.LastRun.State != dash.RunBlocked {
		t.Errorf("a delete ran after an OK: %+v", s.LastRun)
	}

	// the script changes: it waits for a new OK
	must(os.WriteFile(filepath.Join(e.work, "count-users.sh"), []byte("#!/bin/sh\necho 9999\n"), 0o755))
	if st, _ := e.do("POST", "/api/dashboards/"+app.ID+"/refresh", nil, nil); st != 202 {
		t.Fatalf("refresh: %d", st)
	}
	d = e.waitDash(app.ID, "the changed script to wait", func(d dash.Detail) bool {
		return source(d, "users").LastRun.State == dash.RunWaiting
	})
	if s := source(d, "users"); s.Approval == nil || !s.Approval.Changed || !strings.Contains(s.Reason, "changed") {
		t.Errorf("changed script: %+v %+v", s, s.Approval)
	}
	if v := value(d, "users"); v.Number != nil {
		t.Errorf("a changed script's tile should wait, not show the old number: %+v", v)
	}

	// schedule, pause, delete
	if st, b := e.do("PUT", "/api/dashboards/"+app.ID+"/schedule", map[string]any{"refresh": "1h", "paused": true}, nil); st != 200 {
		t.Fatalf("schedule: %d %s", st, b)
	}
	if d := e.dashboard(app.ID); d.Refresh != "1h" || !d.Paused || d.NextRunAt != nil {
		t.Errorf("schedule: %s %v %v", d.Refresh, d.Paused, d.NextRunAt)
	}
	if st, _ := e.do("PUT", "/api/dashboards/"+app.ID+"/schedule", map[string]any{"refresh": "10s"}, nil); st != 400 {
		t.Errorf("too often accepted: %d", st)
	}
	if st, _ := e.do("POST", "/api/dashboards/"+app.ID+"/sources/users/decision", map[string]string{"decision": "allow", "command": users.Command, "cwd": users.Cwd}, map[string]string{"Origin": "http://evil.example"}); st != 403 {
		t.Errorf("another origin could allow a command: %d", st)
	}
	if st, _ := e.do("POST", "/gate/dashboards/tool", map[string]any{"tool": "list_dashboards"}, nil); st != 403 {
		t.Errorf("the gate endpoint needs the gate secret: %d", st)
	}
	if st, _ := e.do("DELETE", "/api/dashboards/"+app.ID, nil, nil); st != 204 {
		t.Fatalf("delete: %d", st)
	}
	if st, _ := e.do("GET", "/api/dashboards/"+app.ID, nil, nil); st != 404 {
		t.Errorf("deleted dashboard still there: %d", st)
	}
}

// Deleting a dashboard stops the command it is running (Codex review of D-067).
func TestDashboardDeleteStopsCommands(t *testing.T) {
	t.Setenv("CB_PLAIN_ENV", "1")
	e := newEnv(t, filepath.Join(repoRoot(), "testdata", "scenarios", "dashboard.json"))
	pidFile := filepath.Join(e.work, "slow.pid")
	if err := os.WriteFile(filepath.Join(e.work, "status.json"), []byte(`{"state":"ok","regions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.work, "count-users.sh"), []byte("#!/bin/sh\necho $$ > slow.pid\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, b := e.do("POST", "/api/conversations", map[string]string{"agent": "fake", "workdir": e.work, "text": "Make me a dashboard"}, nil)
	if st != 200 {
		t.Fatalf("create: %d %s", st, b)
	}
	var created struct{ ID string }
	_ = json.Unmarshal(b, &created)
	e.waitFor(created.ID, "the run to end", func(d model.ConversationDetail) bool { return d.Status == model.ConvDone })
	_, b = e.do("GET", "/api/dashboards", nil, nil)
	var list []dash.Summary
	_ = json.Unmarshal(b, &list)
	if len(list) != 1 {
		t.Fatalf("list: %s", b)
	}
	id := list[0].ID
	users := source(e.dashboard(id), "users")
	if st, _ := e.do("POST", "/api/dashboards/"+id+"/sources/users/decision", map[string]string{"decision": "allow", "command": users.Command, "cwd": users.Cwd}, nil); st != 200 {
		t.Fatalf("allow: %d", st)
	}
	var pid int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &pid); err == nil && pid > 0 {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("the slow command never started")
	}
	if st, _ := e.do("DELETE", "/api/dashboards/"+id, nil, nil); st != 204 {
		t.Fatalf("delete: %d", st)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatal("the command kept running after the dashboard was deleted")
}
