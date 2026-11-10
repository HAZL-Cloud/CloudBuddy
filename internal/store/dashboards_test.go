package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cloudbuddy/internal/dash"
)

func saveDash(t *testing.T, s *Store, id string, refreshSeconds int, sources ...dash.Source) {
	t.Helper()
	saveDashIn(t, s, id, "/w", refreshSeconds, sources...)
}

func saveDashIn(t *testing.T, s *Store, id, workdir string, refreshSeconds int, sources ...dash.Source) {
	t.Helper()
	b, _ := json.Marshal(dash.Spec{Title: "t", Refresh: "1h", Sources: sources})
	if err := s.SaveDashboard(context.Background(), Dashboard{ID: id, Title: "t", Workdir: workdir, RefreshSeconds: refreshSeconds, Spec: b}); err != nil {
		t.Fatal(err)
	}
}

// untilNext is how long until a dashboard's next refresh, on the store's
// clock, which the schedule uses.
func untilNext(t *testing.T, s *Store, id string) time.Duration {
	t.Helper()
	d, err := s.GetDashboard(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d.NextRunAt.Sub(s.now())
}

// The next refresh is the new interval after the last one began: a longer
// interval moves it later, a shorter one can make it due (Codex review,
// 2026-11-10: a longer interval kept the old, earlier time).
func TestDashboardScheduleFromLastRun(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	saveDash(t, s, "db_sched", 60, dash.Source{Key: "n", Command: "cat n"})
	if err := s.StartDashboardRun(ctx, "db_sched"); err != nil {
		t.Fatal(err)
	}
	if got := untilNext(t, s, "db_sched"); got < 59*time.Second || got > 61*time.Second {
		t.Errorf("a refresh starting moves the next one by the interval: in %v, want about 1m", got)
	}
	if err := s.SetDashboardSchedule(ctx, "db_sched", 3600, false); err != nil {
		t.Fatal(err)
	}
	if got := untilNext(t, s, "db_sched"); got < 59*time.Minute || got > 61*time.Minute {
		t.Errorf("1m -> 1h just after a run: next refresh in %v, want about 1h", got)
	}
	// a refresh starting after the change uses the interval stored now
	if err := s.StartDashboardRun(ctx, "db_sched"); err != nil {
		t.Fatal(err)
	}
	if got := untilNext(t, s, "db_sched"); got < 59*time.Minute || got > 61*time.Minute {
		t.Errorf("a refresh after 1m -> 1h: next refresh in %v, want about 1h", got)
	}

	// the last run was two hours ago: an hourly refresh is due now
	if _, err := s.db.Exec(ctx, `UPDATE dashboards SET last_run_at = $1 WHERE id='db_sched'`, s.now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDashboardSchedule(ctx, "db_sched", 3600, false); err != nil {
		t.Fatal(err)
	}
	due, err := s.DueDashboards(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Errorf("an hourly dashboard last run 2h ago should be due: %v (next in %v)", due, untilNext(t, s, "db_sched"))
	}

	// paused, then resumed: refreshes now
	if err := s.SetDashboardSchedule(ctx, "db_sched", 3600, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDashboardSchedule(ctx, "db_sched", 3600, false); err != nil {
		t.Fatal(err)
	}
	if got := untilNext(t, s, "db_sched"); got > time.Second {
		t.Errorf("resuming should refresh now: next in %v", got)
	}
}

// The schedule is on this computer's clock even when the database's is
// minutes off, so the timer neither refreshes on every tick nor stalls, and
// the page's countdown is right (Codex review of the fix, 2026-11-10).
func TestDashboardScheduleOnThisClock(t *testing.T) {
	for _, skew := range []time.Duration{10 * time.Minute, -10 * time.Minute} {
		t.Run(skew.String(), func(t *testing.T) { scheduleOnClock(t, skew) })
	}
}

func scheduleOnClock(t *testing.T, skew time.Duration) {
	s := testStore(t)
	ctx := context.Background()
	s.Now = func() time.Time { return time.Now().Add(skew) }
	id := "db_clock"
	saveDash(t, s, id, 60, dash.Source{Key: "n", Command: "cat n"})
	if due, err := s.DueDashboards(ctx); err != nil || len(due) != 1 {
		t.Errorf("skew %v: a new dashboard should be due: %v %v", skew, due, err)
	}
	if err := s.StartDashboardRun(ctx, id); err != nil {
		t.Fatal(err)
	}
	if due, err := s.DueDashboards(ctx); err != nil || len(due) != 0 {
		t.Errorf("skew %v: due right after it started: %v %v", skew, due, err)
	}
	if got := untilNext(t, s, id); got < 59*time.Second || got > 61*time.Second {
		t.Errorf("skew %v: next refresh in %v, want about 1m", skew, got)
	}
	if err := s.SetDashboardSchedule(ctx, id, 3600, false); err != nil {
		t.Fatal(err)
	}
	if got := untilNext(t, s, id); got < 59*time.Minute || got > 61*time.Minute {
		t.Errorf("skew %v: after 1m -> 1h, next refresh in %v", skew, got)
	}
	_ = s.SetDashboardSchedule(ctx, id, 3600, true)
	_ = s.SetDashboardSchedule(ctx, id, 3600, false)
	if due, err := s.DueDashboards(ctx); err != nil || len(due) != 1 {
		t.Errorf("skew %v: resumed but not due: %v %v", skew, due, err)
	}
}

// Runs and numbers of an earlier command under the same key are left out
// (Codex review, 2026-11-10).
func TestDashboardRunsAndPointsFollowTheCommand(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	saveDash(t, s, "db_rev", 3600, dash.Source{Key: "n", Command: "cat one"})
	old, cur := dash.SourceRev("/w", dash.Source{Key: "n", Command: "cat one"}), dash.SourceRev("/w", dash.Source{Key: "n", Command: "cat two"})
	if old == cur {
		t.Fatal("two commands have the same revision")
	}
	at := time.Now().Add(-time.Hour)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.AddDashboardRun(ctx, "db_rev", DashboardRun{SourceKey: "n", SourceRev: old, StartedAt: at, State: dash.RunOK, Output: "1"}))
	must(s.AddDashboardPoints(ctx, "db_rev", at, map[string]DashboardValue{"n|": {Value: 1, SourceRev: old}}))
	must(s.AddDashboardRun(ctx, "db_rev", DashboardRun{SourceKey: "n", SourceRev: cur, StartedAt: time.Now(), State: dash.RunFailed}))

	latest, lastOK, err := s.LatestDashboardRuns(ctx, "db_rev", map[string]string{"n": cur})
	must(err)
	if latest["n"].State != dash.RunFailed {
		t.Errorf("latest: %+v", latest["n"])
	}
	if r, ok := lastOK["n"]; ok {
		t.Errorf("the old command's output came back as the last run that worked: %+v", r)
	}
	pts, err := s.DashboardPoints(ctx, "db_rev", map[string]string{"n|": cur}, time.Now().Add(-24*time.Hour), 10)
	must(err)
	if len(pts["n|"]) != 0 {
		t.Errorf("the old command's numbers came back: %+v", pts)
	}
	m, err := s.DashboardMarks(ctx, "db_rev", "n|", cur, time.Now())
	must(err)
	if m.Oldest != nil || m.Previous != nil || m.Before[time.Hour] != nil || m.Before[24*time.Hour] != nil {
		t.Errorf("the old command's numbers came back as marks: %+v", m)
	}

	// the same command again: its history is its own
	latest, lastOK, err = s.LatestDashboardRuns(ctx, "db_rev", map[string]string{"n": old})
	must(err)
	if latest["n"].Output != "1" || lastOK["n"].Output != "1" {
		t.Errorf("old revision: %+v %+v", latest["n"], lastOK["n"])
	}
	pts, err = s.DashboardPoints(ctx, "db_rev", map[string]string{"n|": old}, time.Now().Add(-24*time.Hour), 10)
	must(err)
	if len(pts["n|"]) != 1 {
		t.Errorf("old revision points: %+v", pts)
	}
}

// Migration 004 tags rows stored before it with the revision dash.SourceRev
// gives the command each source has now: the SQL and the Go agree.
func TestMigration004MatchesSourceRev(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	srcs := []dash.Source{
		{Key: "plain", Command: "cat a.json"},
		{Key: "folder", Command: `jq -r '.users | length' u.json`, Cwd: "sub/dir", Format: "json"},
		{Key: "accents", Command: `echo "café ✓ <b> & \"q\""`, Cwd: "/abs/ü", Format: "text"},
	}
	const workdir = "/Users/me/projets/données"
	saveDashIn(t, s, "db_mig", workdir, 3600, srcs...)
	for _, src := range srcs {
		if _, err := s.db.Exec(ctx, `INSERT INTO dashboard_runs(dashboard_id,source_key,started_at,state) VALUES ('db_mig',$1,now(),'ok')`, src.Key); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(ctx, `INSERT INTO dashboard_points(dashboard_id,series,ts,value) VALUES ('db_mig',$1,now(),1)`, src.Key+"|.x"); err != nil {
			t.Fatal(err)
		}
	}
	sqlText, err := migrationFS.ReadFile("migrations/004_dashboard_source_rev.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, src := range srcs {
		want := dash.SourceRev(workdir, src)
		var run, point string
		if err := s.db.QueryRow(ctx, `SELECT source_rev FROM dashboard_runs WHERE dashboard_id='db_mig' AND source_key=$1`, src.Key).Scan(&run); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow(ctx, `SELECT source_rev FROM dashboard_points WHERE dashboard_id='db_mig' AND series=$1`, src.Key+"|.x").Scan(&point); err != nil {
			t.Fatal(err)
		}
		if run != want || point != want {
			t.Errorf("%s: run %q point %q, want %q", src.Key, run, point, want)
		}
	}
}
