package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Dashboard is one stored dashboard (D-067, D-068).
type Dashboard struct {
	ID, Title, Description string
	ConversationID         string // "" when the chat is gone
	Workdir                string
	Folders                []string
	RefreshSeconds         int
	Paused                 bool
	Spec                   json.RawMessage
	NextRunAt              time.Time
	LastRunAt              *time.Time
	CreatedAt, UpdatedAt   time.Time
}

const dashCols = `id, title, description, COALESCE(conversation_id,''), workdir, folders, refresh_seconds, paused, spec, next_run_at, last_run_at, created_at, updated_at`

func scanDashboard(row pgx.Row) (Dashboard, error) {
	var d Dashboard
	err := row.Scan(&d.ID, &d.Title, &d.Description, &d.ConversationID, &d.Workdir, &d.Folders, &d.RefreshSeconds, &d.Paused,
		&d.Spec, &d.NextRunAt, &d.LastRunAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return d, err
}

// SaveDashboard creates a dashboard or replaces its definition. A changed
// definition refreshes right away.
func (s *Store) SaveDashboard(ctx context.Context, d Dashboard) error {
	if d.Folders == nil {
		d.Folders = []string{}
	}
	_, err := s.db.Exec(ctx, `INSERT INTO dashboards(id,title,description,conversation_id,workdir,folders,refresh_seconds,spec,next_run_at)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,now())
		ON CONFLICT (id) DO UPDATE SET title=$2, description=$3, conversation_id=COALESCE(NULLIF($4,''), dashboards.conversation_id),
			workdir=$5, folders=$6, refresh_seconds=$7, spec=$8, next_run_at=now(), updated_at=now()`,
		d.ID, d.Title, d.Description, d.ConversationID, d.Workdir, d.Folders, d.RefreshSeconds, []byte(d.Spec))
	return err
}

func (s *Store) GetDashboard(ctx context.Context, id string) (Dashboard, error) {
	return scanDashboard(s.db.QueryRow(ctx, `SELECT `+dashCols+` FROM dashboards WHERE id=$1`, id))
}

func (s *Store) Dashboards(ctx context.Context) ([]Dashboard, error) {
	rows, err := s.db.Query(ctx, `SELECT `+dashCols+` FROM dashboards ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dashboard{}
	for rows.Next() {
		d, err := scanDashboard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteDashboard(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM dashboards WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// SetDashboardSchedule changes how often a dashboard refreshes, or pauses it.
func (s *Store) SetDashboardSchedule(ctx context.Context, id string, refreshSeconds int, paused bool) error {
	tag, err := s.db.Exec(ctx, `UPDATE dashboards SET refresh_seconds=$2, paused=$3,
		next_run_at = CASE WHEN paused AND NOT $3 THEN now() ELSE LEAST(next_run_at, now() + make_interval(secs => $4)) END,
		updated_at=now() WHERE id=$1`, id, refreshSeconds, paused, float64(refreshSeconds))
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// DueDashboards lists dashboards whose next refresh is due.
func (s *Store) DueDashboards(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT id FROM dashboards WHERE NOT paused AND next_run_at <= $1 ORDER BY next_run_at`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// StartDashboardRun moves the next refresh on before a refresh begins, so a
// slow one isn't started twice.
func (s *Store) StartDashboardRun(ctx context.Context, id string, next time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE dashboards SET next_run_at=$2, last_run_at=now() WHERE id=$1`, id, next)
	return err
}

// ---------- approvals ----------

// DashboardApproval is the user's answer for one command that isn't a read.
type DashboardApproval struct {
	SourceKey, Command, Cwd, ExecDigest string
	Allowed                             bool
	DecidedAt                           time.Time
}

func (s *Store) DashboardApprovals(ctx context.Context, id string) (map[string]DashboardApproval, error) {
	rows, err := s.db.Query(ctx, `SELECT source_key, command, cwd, exec_digest, allowed, decided_at FROM dashboard_approvals WHERE dashboard_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]DashboardApproval{}
	for rows.Next() {
		var a DashboardApproval
		if err := rows.Scan(&a.SourceKey, &a.Command, &a.Cwd, &a.ExecDigest, &a.Allowed, &a.DecidedAt); err != nil {
			return nil, err
		}
		out[a.SourceKey] = a
	}
	return out, rows.Err()
}

func (s *Store) SetDashboardApproval(ctx context.Context, id string, a DashboardApproval) error {
	_, err := s.db.Exec(ctx, `INSERT INTO dashboard_approvals(dashboard_id,source_key,command,cwd,exec_digest,allowed) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (dashboard_id, source_key) DO UPDATE SET command=$3, cwd=$4, exec_digest=$5, allowed=$6, decided_at=now()`,
		id, a.SourceKey, a.Command, a.Cwd, a.ExecDigest, a.Allowed)
	return err
}

// KeepDashboardApprovals forgets answers for commands no longer in the dashboard.
func (s *Store) KeepDashboardApprovals(ctx context.Context, id string, keys []string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM dashboard_approvals WHERE dashboard_id=$1 AND NOT (source_key = ANY($2))`, id, keys)
	return err
}

// ---------- runs ----------

// DashboardRun is one run of one command.
type DashboardRun struct {
	SourceKey  string
	StartedAt  time.Time
	DurationMs int
	State      string
	ExitCode   *int
	Output     string
	Truncated  bool
	Note       string
	ExecDigest string
}

// keepRuns is how many runs of each command are kept.
const keepRuns = 30

func (s *Store) AddDashboardRun(ctx context.Context, id string, r DashboardRun) error {
	_, err := s.db.Exec(ctx, `INSERT INTO dashboard_runs(dashboard_id,source_key,started_at,duration_ms,state,exit_code,output,truncated,note,exec_digest)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, r.SourceKey, r.StartedAt, r.DurationMs, r.State, r.ExitCode, r.Output, r.Truncated, r.Note, r.ExecDigest)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `DELETE FROM dashboard_runs WHERE dashboard_id=$1 AND source_key=$2 AND id NOT IN
		(SELECT id FROM dashboard_runs WHERE dashboard_id=$1 AND source_key=$2 ORDER BY id DESC LIMIT $3)
		AND id NOT IN (SELECT id FROM dashboard_runs WHERE dashboard_id=$1 AND source_key=$2 AND state='ok' ORDER BY id DESC LIMIT 1)`,
		id, r.SourceKey, keepRuns)
	return err
}

// LatestDashboardRuns returns each command's latest run and latest run that
// worked.
func (s *Store) LatestDashboardRuns(ctx context.Context, id string) (latest, lastOK map[string]DashboardRun, err error) {
	latest, lastOK = map[string]DashboardRun{}, map[string]DashboardRun{}
	rows, err := s.db.Query(ctx, `
		(SELECT DISTINCT ON (source_key) 'latest', source_key, started_at, duration_ms, state, exit_code, output, truncated, note, exec_digest
			FROM dashboard_runs WHERE dashboard_id=$1 ORDER BY source_key, id DESC)
		UNION ALL
		(SELECT DISTINCT ON (source_key) 'ok', source_key, started_at, duration_ms, state, exit_code, output, truncated, note, exec_digest
			FROM dashboard_runs WHERE dashboard_id=$1 AND state='ok' ORDER BY source_key, id DESC)`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var which string
		var r DashboardRun
		if err := rows.Scan(&which, &r.SourceKey, &r.StartedAt, &r.DurationMs, &r.State, &r.ExitCode, &r.Output, &r.Truncated, &r.Note, &r.ExecDigest); err != nil {
			return nil, nil, err
		}
		if which == "latest" {
			latest[r.SourceKey] = r
		} else {
			lastOK[r.SourceKey] = r
		}
	}
	return latest, lastOK, rows.Err()
}

// ---------- number history ----------

// DashboardPoint is one number at one time.
type DashboardPoint struct {
	T time.Time
	V float64
}

// historyDays is how long numbers are kept.
const historyDays = 30

func (s *Store) AddDashboardPoints(ctx context.Context, id string, at time.Time, values map[string]float64) error {
	if len(values) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for series, v := range values {
		batch.Queue(`INSERT INTO dashboard_points(dashboard_id,series,ts,value) VALUES ($1,$2,$3,$4)`, id, series, at, v)
	}
	return s.db.SendBatch(ctx, batch).Close()
}

// DashboardPoints returns each series' points since a time, averaged into
// at most n buckets, oldest first.
func (s *Store) DashboardPoints(ctx context.Context, id string, series []string, since time.Time, n int) (map[string][]DashboardPoint, error) {
	out := map[string][]DashboardPoint{}
	if len(series) == 0 {
		return out, nil
	}
	span := time.Since(since)
	bucket := span / time.Duration(max(n, 1))
	if bucket < time.Second {
		bucket = time.Second
	}
	rows, err := s.db.Query(ctx, `SELECT series, date_bin(make_interval(secs => $4), ts, $3::timestamptz) AS b, max(ts), avg(value)
		FROM dashboard_points WHERE dashboard_id=$1 AND series = ANY($2) AND ts >= $3
		GROUP BY series, b ORDER BY series, b`, id, series, since, bucket.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var b, last time.Time
		var v float64
		if err := rows.Scan(&name, &b, &last, &v); err != nil {
			return nil, err
		}
		out[name] = append(out[name], DashboardPoint{T: last, V: v})
	}
	return out, rows.Err()
}

// DashboardMarks are the points a number tile compares against: the latest
// point at least 1 hour, 24 hours and 7 days old, the oldest, and the one
// before the latest.
type DashboardMarks struct {
	Before   map[time.Duration]*DashboardPoint
	Oldest   *DashboardPoint
	Previous *DashboardPoint
}

func (s *Store) DashboardMarks(ctx context.Context, id, series string, now time.Time) (DashboardMarks, error) {
	m := DashboardMarks{Before: map[time.Duration]*DashboardPoint{}}
	one := func(q string, args ...any) (*DashboardPoint, error) {
		var p DashboardPoint
		err := s.db.QueryRow(ctx, q, args...).Scan(&p.T, &p.V)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	for _, d := range []time.Duration{time.Hour, 24 * time.Hour, 7 * 24 * time.Hour} {
		p, err := one(`SELECT ts, value FROM dashboard_points WHERE dashboard_id=$1 AND series=$2 AND ts <= $3 ORDER BY ts DESC LIMIT 1`, id, series, now.Add(-d))
		if err != nil {
			return m, err
		}
		m.Before[d] = p
	}
	var err error
	if m.Oldest, err = one(`SELECT ts, value FROM dashboard_points WHERE dashboard_id=$1 AND series=$2 ORDER BY ts LIMIT 1`, id, series); err != nil {
		return m, err
	}
	m.Previous, err = one(`SELECT ts, value FROM dashboard_points WHERE dashboard_id=$1 AND series=$2 ORDER BY ts DESC OFFSET 1 LIMIT 1`, id, series)
	return m, err
}

// PruneDashboardPoints drops numbers older than the history window.
func (s *Store) PruneDashboardPoints(ctx context.Context, now time.Time) error {
	_, err := s.db.Exec(ctx, `DELETE FROM dashboard_points WHERE ts < $1`, now.Add(-historyDays*24*time.Hour))
	return err
}
