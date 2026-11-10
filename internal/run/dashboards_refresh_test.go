package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cloudbuddy/internal/bus"
	"cloudbuddy/internal/config"
	"cloudbuddy/internal/dash"
	"cloudbuddy/internal/store"
)

// dashEngine is an engine on a fresh database, and a project folder.
func dashEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	t.Setenv("CB_PLAIN_ENV", "1")
	ctx := context.Background()
	admin := "postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/cloudbuddy?sslmode=disable"
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	name := fmt.Sprintf("cb_run_%d_%d", os.Getpid(), time.Now().UnixNano()%100000)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	st, err := store.Open(ctx, fmt.Sprintf("postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/%s?sslmode=disable", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		st.Close()
		if c, err := pgx.Connect(ctx, admin); err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			c.Close(ctx)
		}
	})
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Port: 1, DataDir: t.TempDir(), Home: t.TempDir(), Bin: "/usr/local/bin/cloudbuddy",
		Epoch: config.Random(8), GateToken: config.Random(24)}
	return New(cfg, st, bus.New(), nil), work
}

// dashSpec is a dashboard with one number tile per source.
func dashSpec(sources ...dash.Source) json.RawMessage {
	spec := dash.Spec{Title: "T", Refresh: "1h", Sources: sources}
	for _, s := range sources {
		spec.Tiles = append(spec.Tiles, dash.Tile{Key: s.Key, Type: "number", Title: s.Key, Source: s.Key, Compare: "previous"})
	}
	b, _ := json.Marshal(spec)
	return b
}

func saveSpec(t *testing.T, e *Engine, r *Run, id string, sources ...dash.Source) string {
	t.Helper()
	args := dashSpec(sources...)
	if id != "" {
		var m map[string]any
		_ = json.Unmarshal(args, &m)
		m["id"] = id
		args, _ = json.Marshal(m)
	}
	res := e.dashSave(context.Background(), r, "", args)
	if res.IsError {
		t.Fatalf("save: %s", res.Text)
	}
	all, err := e.st.Dashboards(context.Background())
	if err != nil || len(all) != 1 {
		t.Fatalf("dashboards: %v %v", all, err)
	}
	return all[0].ID
}

func tile(d dash.Detail, key string) dash.TileValue {
	for _, v := range d.Values {
		if v.Key == key {
			return v
		}
	}
	return dash.TileValue{}
}

func writeFile(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

func waitBusy(t *testing.T, e *Engine, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !e.dashBusy(id) {
		if time.Now().After(deadline) {
			t.Fatal("the refresh never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A save while the old version is refreshing runs the new version, and the
// agent's report is about it (Codex review, 2026-11-10: the save's refresh
// joined the old one and returned without running the new commands).
func TestDashSaveDuringRefreshRunsTheNewVersion(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	if err := syscall.Mkfifo(filepath.Join(work, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(work, "two.txt"), "2\n", 0o644)
	r := &Run{ID: "run_t", Workdir: work}
	// cat of a pipe nobody writes to waits until its timeout
	slow := dash.Source{Key: "n", Command: "cat pipe", Format: "auto", TimeoutSeconds: 3}
	id := saveSpec(t, e, r, "", slow)
	if d, _ := e.DashboardDetail(ctx, id, ""); len(d.Sources) != 1 || d.Sources[0].Verdict != dash.Read {
		t.Fatalf("cat of a project file should be a read: %+v", d.Sources)
	}

	done := make(chan struct{})
	go func() { _ = e.refresh(ctx, id, refreshScheduled); close(done) }()
	waitBusy(t, e, id)

	start := time.Now()
	saveSpec(t, e, r, id, dash.Source{Key: "n", Command: "cat two.txt"})
	if took := time.Since(start); took > 2500*time.Millisecond {
		t.Errorf("the save waited %v for the old version's refresh", took)
	}
	d, err := e.DashboardDetail(ctx, id, "")
	if err != nil {
		t.Fatal(err)
	}
	if v := tile(d, "n"); v.State != "ok" || v.Number == nil || *v.Number != 2 {
		t.Errorf("after the save the tile should show the new command's 2: %+v", v)
	}
	<-done
	// the old version's refresh was stopped, and says why
	latest, _, err := e.st.LatestDashboardRuns(ctx, id, map[string]string{"n": dash.SourceRev(work, slow)})
	if err != nil {
		t.Fatal(err)
	}
	if n := latest["n"].Note; !strings.Contains(n, "saved again") {
		t.Errorf("the stopped run's note: %q", n)
	}
}

// A changed command under the same key starts with no results: a failing new
// command doesn't show the old command's number, and its history doesn't
// mix with the old one's (Codex review, 2026-11-10).
func TestDashChangedCommandStartsClean(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	writeFile(t, filepath.Join(work, "one.txt"), "1\n", 0o644)
	writeFile(t, filepath.Join(work, "two.txt"), "2\n", 0o644)
	r := &Run{ID: "run_t", Workdir: work}

	id := saveSpec(t, e, r, "", dash.Source{Key: "n", Command: "cat one.txt"})
	d, _ := e.DashboardDetail(ctx, id, "")
	if v := tile(d, "n"); v.Number == nil || *v.Number != 1 {
		t.Fatalf("first version: %+v", v)
	}

	saveSpec(t, e, r, id, dash.Source{Key: "n", Command: "cat missing.txt"})
	d, _ = e.DashboardDetail(ctx, id, "")
	if v := tile(d, "n"); v.State != "error" || v.Number != nil || v.Stale {
		t.Errorf("a failing new command showed the old command's number: %+v", v)
	}

	saveSpec(t, e, r, id, dash.Source{Key: "n", Command: "cat two.txt"})
	d, _ = e.DashboardDetail(ctx, id, "")
	v := tile(d, "n")
	if v.Number == nil || *v.Number != 2 {
		t.Fatalf("third version: %+v", v)
	}
	if v.Compare != nil {
		t.Errorf("the new command was compared with the old command's number: %+v", v.Compare)
	}
}

// Allowing a command while a refresh is going runs it once that refresh
// ends, not at the next scheduled one (the same mistake as the save's).
func TestDashAllowDuringRefreshRunsIt(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	if err := syscall.Mkfifo(filepath.Join(work, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(work, "count.sh"), "#!/bin/sh\necho 7\n", 0o755)
	r := &Run{ID: "run_t", Workdir: work}
	id := saveSpec(t, e, r, "", dash.Source{Key: "slow", Command: "cat pipe", TimeoutSeconds: 2}, dash.Source{Key: "users", Command: "./count.sh"})
	d, _ := e.DashboardDetail(ctx, id, "")
	var rev string
	for _, s := range d.Sources {
		if s.Key == "users" {
			rev = s.Revision
		}
	}
	if rev == "" {
		t.Fatalf("the script should wait for an OK: %+v", d.Sources)
	}

	go func() { _ = e.refresh(ctx, id, refreshScheduled) }()
	waitBusy(t, e, id)
	if err := e.AllowDashboardSource(ctx, id, "users", true, rev); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		d, _ = e.DashboardDetail(ctx, id, "")
		if v := tile(d, "users"); v.Number != nil && *v.Number == 7 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the allowed script didn't run after the refresh that was going: %+v", tile(d, "users"))
}

// Pausing stops a refresh that was waiting for the running one to end
// (Codex review of the fix, 2026-11-10: an answer's refresh ran after the
// pause).
func TestDashPauseStopsAWaitingRefresh(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	if err := syscall.Mkfifo(filepath.Join(work, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(work, "count.sh"), "#!/bin/sh\necho 7\n", 0o755)
	r := &Run{ID: "run_t", Workdir: work}
	users := dash.Source{Key: "users", Command: "./count.sh", Format: "auto"}
	id := saveSpec(t, e, r, "", dash.Source{Key: "slow", Command: "cat pipe", TimeoutSeconds: 2}, users)
	d, _ := e.DashboardDetail(ctx, id, "")
	var rev string
	for _, s := range d.Sources {
		if s.Key == "users" {
			rev = s.Revision
		}
	}
	if rev == "" {
		t.Fatalf("the script should wait for an OK: %+v", d.Sources)
	}

	go func() { _ = e.refresh(ctx, id, refreshScheduled) }()
	waitBusy(t, e, id)
	if err := e.AllowDashboardSource(ctx, id, "users", true, rev); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // the answer's refresh is waiting
	if err := e.SetDashboardSchedule(ctx, id, "1h", true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2500 * time.Millisecond) // past the slow command's timeout
	latest, lastOK, err := e.st.LatestDashboardRuns(ctx, id, map[string]string{"users": dash.SourceRev(work, users)})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := lastOK["users"]; ok {
		t.Errorf("the script ran after the dashboard was paused: %+v", r)
	}
	if latest["users"].State != dash.RunWaiting {
		t.Errorf("latest: %+v", latest["users"])
	}
}

// A dashboard saved again from a chat in a parent folder runs in that
// folder: its sources start clean, as with a changed command.
func TestDashMovedDashboardStartsClean(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	sub := filepath.Join(work, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "n.txt"), "1\n", 0o644)
	src := dash.Source{Key: "n", Command: "cat n.txt"}
	id := saveSpec(t, e, &Run{ID: "run_a", Workdir: sub}, "", src)
	d, _ := e.DashboardDetail(ctx, id, "")
	if v := tile(d, "n"); v.Number == nil || *v.Number != 1 {
		t.Fatalf("first save: %+v", v)
	}
	saveSpec(t, e, &Run{ID: "run_b", Workdir: work}, id, src)
	d, _ = e.DashboardDetail(ctx, id, "")
	if d.Workdir != work {
		t.Fatalf("the save didn't move the dashboard: %s", d.Workdir)
	}
	if v := tile(d, "n"); v.State != "error" || v.Number != nil {
		t.Errorf("the moved dashboard showed the old folder's number: %+v", v)
	}
}

// A refresh asked for before a pause doesn't run after it, however late it
// gets going (Codex review of the fix, 2026-11-10: the pause count was read
// when the goroutine started).
func TestDashRefreshAskedBeforePauseDoesntRun(t *testing.T) {
	e, work := dashEngine(t)
	ctx := context.Background()
	writeFile(t, filepath.Join(work, "one.txt"), "1\n", 0o644)
	src := dash.Source{Key: "n", Command: "cat one.txt", Format: "auto"}
	id := saveSpec(t, e, &Run{ID: "run_t", Workdir: work}, "", src)
	revs := map[string]string{"n": dash.SourceRev(work, src)}
	before, _, err := e.st.LatestDashboardRuns(ctx, id, revs)
	if err != nil || before["n"].State != dash.RunOK {
		t.Fatalf("first refresh: %+v %v", before["n"], err)
	}
	ask := e.askRefresh(id)
	if err := e.SetDashboardSchedule(ctx, id, "1h", true); err != nil {
		t.Fatal(err)
	}
	if err := e.refreshAsked(ctx, id, refreshChanged, ask); err != nil {
		t.Fatal(err)
	}
	after, _, _ := e.st.LatestDashboardRuns(ctx, id, revs)
	if !after["n"].StartedAt.Equal(before["n"].StartedAt) {
		t.Errorf("a refresh asked before the pause ran after it: %v then %v", before["n"].StartedAt, after["n"].StartedAt)
	}
	// asked after the pause: it runs once, as before (the user or agent asked)
	if err := e.refresh(ctx, id, refreshChanged); err != nil {
		t.Fatal(err)
	}
	again, _, _ := e.st.LatestDashboardRuns(ctx, id, revs)
	if !again["n"].StartedAt.After(before["n"].StartedAt) {
		t.Errorf("a refresh asked after the pause didn't run")
	}
}
