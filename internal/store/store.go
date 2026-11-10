// Package store keeps CloudBuddy's state in PostgreSQL. Queries are written
// by hand with pgx; migrations are embedded SQL files applied in order.
package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cloudbuddy/internal/model"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct {
	db *pgxpool.Pool
	// Now is this computer's clock, for times compared with it (a
	// dashboard's schedule). Tests set it to a clock that differs from the
	// database's.
	Now func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var ErrNotPending = errors.New("not_pending")
var ErrNotFound = errors.New("not_found")

func Open(ctx context.Context, url string) (*Store, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("can't reach PostgreSQL at %s: %w", redactURL(url), err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() { s.db.Close() }

func redactURL(u string) string {
	if i := strings.Index(u, "@"); i > 0 {
		if j := strings.Index(u, "://"); j > 0 && j < i {
			return u[:j+3] + "…" + u[i:]
		}
	}
	return u
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, n := range names {
		var done bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, n).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sqlText, _ := migrationFS.ReadFile("migrations/" + n)
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlText)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", n, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES ($1)`, n); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ---------- conversations ----------

type Conversation struct {
	ID, Title, Agent, Model, Workdir string
	ProjectID                        string   // "" outside a project
	ExtraFolders                     []string // the project's other folders when the chat started
	Status                           model.ConvStatus
	AgentSessionID, Subtitle         string
	Result                           *model.RunResult
	CreatedAt, UpdatedAt             time.Time
}

func (s *Store) CreateConversation(ctx context.Context, c Conversation) error {
	if c.ExtraFolders == nil {
		c.ExtraFolders = []string{}
	}
	_, err := s.db.Exec(ctx, `INSERT INTO conversations(id,title,agent,model,workdir,status,project_id,extra_folders) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)`,
		c.ID, c.Title, c.Agent, c.Model, c.Workdir, c.Status, c.ProjectID, c.ExtraFolders)
	return err
}

func (s *Store) GetConversation(ctx context.Context, id string) (Conversation, error) {
	var c Conversation
	var result []byte
	err := s.db.QueryRow(ctx, `SELECT id,title,agent,model,workdir,COALESCE(project_id,''),extra_folders,status,agent_session_id,subtitle,result,created_at,updated_at FROM conversations WHERE id=$1`, id).
		Scan(&c.ID, &c.Title, &c.Agent, &c.Model, &c.Workdir, &c.ProjectID, &c.ExtraFolders, &c.Status, &c.AgentSessionID, &c.Subtitle, &result, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if len(result) > 0 && string(result) != "null" {
		c.Result = &model.RunResult{}
		_ = json.Unmarshal(result, c.Result)
	}
	return c, err
}

func (s *Store) SetConversationStatus(ctx context.Context, id string, st model.ConvStatus) error {
	_, err := s.db.Exec(ctx, `UPDATE conversations SET status=$2, updated_at=now() WHERE id=$1`, id, st)
	return err
}

func (s *Store) SetSessionID(ctx context.Context, id, sessionID string) error {
	_, err := s.db.Exec(ctx, `UPDATE conversations SET agent_session_id=$2, updated_at=now() WHERE id=$1`, id, sessionID)
	return err
}

func (s *Store) SetModel(ctx context.Context, id, m string) error {
	_, err := s.db.Exec(ctx, `UPDATE conversations SET model=$2 WHERE id=$1 AND model=''`, id, m)
	return err
}

func (s *Store) SetResult(ctx context.Context, id string, r *model.RunResult, subtitle string) error {
	b, _ := json.Marshal(r)
	_, err := s.db.Exec(ctx, `UPDATE conversations SET result=$2, subtitle=$3, updated_at=now() WHERE id=$1`, id, b, subtitle)
	return err
}

func (s *Store) SetSubtitle(ctx context.Context, id, subtitle string) error {
	_, err := s.db.Exec(ctx, `UPDATE conversations SET subtitle=$2 WHERE id=$1`, id, subtitle)
	return err
}

func (s *Store) Touch(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) ListConversations(ctx context.Context) ([]model.ConversationSummary, error) {
	rows, err := s.db.Query(ctx, `
		SELECT c.id,c.title,c.agent,c.model,c.workdir,COALESCE(c.project_id,''),c.status,c.updated_at,
		  (SELECT count(*) FROM steps st WHERE st.conversation_id=c.id AND st.doc->>'kind'='cloud'),
		  (SELECT count(*) FROM approvals a WHERE a.conversation_id=c.id AND a.state='pending')
		FROM conversations c ORDER BY c.updated_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ConversationSummary{}
	for rows.Next() {
		var c model.ConversationSummary
		if err := rows.Scan(&c.ID, &c.Title, &c.Agent, &c.Model, &c.Workdir, &c.ProjectID, &c.Status, &c.UpdatedAt, &c.StepCount, &c.WaitingCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- runs and events ----------

func (s *Store) CreateRun(ctx context.Context, id, convID, epoch, agentVersion string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO runs(id,conversation_id,launch_epoch,agent_version) VALUES ($1,$2,$3,$4)`, id, convID, epoch, agentVersion)
	return err
}

func (s *Store) FinishRun(ctx context.Context, id, status, reason string) error {
	_, err := s.db.Exec(ctx, `UPDATE runs SET status=$2, exit_reason=$3, ended_at=now() WHERE id=$1 AND ended_at IS NULL`, id, status, reason)
	return err
}

// InterruptOpenRuns marks runs left open by a previous launch as interrupted,
// expires their pending approvals and returns the affected conversations.
func (s *Store) InterruptOpenRuns(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `UPDATE runs SET status='interrupted', exit_reason='CloudBuddy restarted', ended_at=now() WHERE ended_at IS NULL RETURNING conversation_id`)
	if err != nil {
		return nil, err
	}
	var convs []string
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		convs = append(convs, c)
	}
	rows.Close()
	if _, err := s.db.Exec(ctx, `UPDATE approvals SET state='expired', decided_at=now() WHERE state='pending'`); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `UPDATE releases SET consumed_at=now() WHERE consumed_at IS NULL`); err != nil {
		return nil, err
	}
	for _, c := range convs {
		_, _ = s.db.Exec(ctx, `UPDATE conversations SET status='interrupted' WHERE id=$1 AND status IN ('running','waiting')`, c)
		_, _ = s.db.Exec(ctx, `UPDATE steps SET doc = jsonb_set(doc, '{status}', to_jsonb(CASE WHEN doc->>'status'='waiting' THEN 'denied' ELSE 'outcome_unknown' END)) WHERE conversation_id=$1 AND doc->>'status' IN ('running','waiting')`, c)
	}
	return convs, nil
}

func (s *Store) AppendEvent(ctx context.Context, runID, convID, source, kind string, payload any, truncated bool) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO events(run_id,conversation_id,source,kind,payload,truncated) VALUES ($1,$2,$3,$4,$5,$6)`, runID, convID, source, kind, b, truncated)
	return err
}

// ---------- steps ----------

// UpsertStep stores a step, creating it under toolKey if new. It returns the
// stored step (with its id and seq).
func (s *Store) UpsertStep(ctx context.Context, convID, runID, toolKey string, mutate func(st *model.Step, isNew bool)) (model.Step, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return model.Step{}, err
	}
	defer tx.Rollback(ctx)
	var doc []byte
	var st model.Step
	isNew := false
	err = tx.QueryRow(ctx, `SELECT doc FROM steps WHERE conversation_id=$1 AND tool_key=$2 FOR UPDATE`, convID, toolKey).Scan(&doc)
	if errors.Is(err, pgx.ErrNoRows) {
		isNew = true
		var seq int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(seq),0)+1 FROM steps WHERE conversation_id=$1`, convID).Scan(&seq); err != nil {
			return st, err
		}
		st = model.Step{ID: newID("st"), Seq: seq, StartedAt: time.Now().UTC(), Status: model.StatusRunning,
			Tags: []model.Tag{}, Facts: [][2]string{}, Uses: []model.Use{}, RecognizedBy: []string{}}
	} else if err != nil {
		return st, err
	} else {
		_ = json.Unmarshal(doc, &st)
	}
	mutate(&st, isNew)
	st.Approval = nil // approvals live in their own table
	b, _ := json.Marshal(st)
	if isNew {
		_, err = tx.Exec(ctx, `INSERT INTO steps(id,conversation_id,run_id,tool_key,seq,doc) VALUES ($1,$2,$3,$4,$5,$6)`, st.ID, convID, runID, toolKey, st.Seq, b)
	} else {
		_, err = tx.Exec(ctx, `UPDATE steps SET doc=$3, updated_at=now() WHERE conversation_id=$1 AND tool_key=$2`, convID, toolKey, b)
	}
	if err != nil {
		return st, err
	}
	return st, tx.Commit(ctx)
}

func (s *Store) StepByKey(ctx context.Context, convID, toolKey string) (model.Step, bool, error) {
	var doc []byte
	err := s.db.QueryRow(ctx, `SELECT doc FROM steps WHERE conversation_id=$1 AND tool_key=$2`, convID, toolKey).Scan(&doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Step{}, false, nil
	}
	var st model.Step
	_ = json.Unmarshal(doc, &st)
	return st, err == nil, err
}

func (s *Store) StepConversation(ctx context.Context, stepID string) (convID, toolKey string, err error) {
	err = s.db.QueryRow(ctx, `SELECT conversation_id, tool_key FROM steps WHERE id=$1`, stepID).Scan(&convID, &toolKey)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}

func (s *Store) Steps(ctx context.Context, convID string) ([]model.Step, error) {
	rows, err := s.db.Query(ctx, `SELECT doc FROM steps WHERE conversation_id=$1 ORDER BY seq`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Step{}
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		var st model.Step
		_ = json.Unmarshal(doc, &st)
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	approvals, err := s.approvalsFor(ctx, convID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if a, ok := approvals[out[i].ID]; ok {
			out[i].Approval = a
		}
	}
	return out, nil
}

// ---------- chat ----------

func (s *Store) AppendChat(ctx context.Context, convID string, item model.ChatItem) error {
	if item.ID == "" {
		item.ID = newID("ci")
	}
	if item.At.IsZero() {
		item.At = time.Now().UTC()
	}
	b, _ := json.Marshal(item)
	_, err := s.db.Exec(ctx, `INSERT INTO chat_items(id,conversation_id,doc) VALUES ($1,$2,$3)`, item.ID, convID, b)
	return err
}

// AddStepToChat appends stepID to the last chat item if it is a steps group,
// otherwise starts a new group.
func (s *Store) AddStepToChat(ctx context.Context, convID, stepID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, convID); err != nil {
		return err
	}
	var id string
	var doc []byte
	err = tx.QueryRow(ctx, `SELECT id, doc FROM chat_items WHERE conversation_id=$1 ORDER BY seq DESC LIMIT 1`, convID).Scan(&id, &doc)
	var last model.ChatItem
	if err == nil {
		_ = json.Unmarshal(doc, &last)
	}
	if err == nil && last.Type == "steps" {
		for _, x := range last.StepIDs {
			if x == stepID {
				return tx.Commit(ctx)
			}
		}
		last.StepIDs = append(last.StepIDs, stepID)
		b, _ := json.Marshal(last)
		if _, err := tx.Exec(ctx, `UPDATE chat_items SET doc=$2 WHERE id=$1`, id, b); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	item := model.ChatItem{ID: newID("ci"), Type: "steps", StepIDs: []string{stepID}, At: time.Now().UTC()}
	b, _ := json.Marshal(item)
	if _, err := tx.Exec(ctx, `INSERT INTO chat_items(id,conversation_id,doc) VALUES ($1,$2,$3)`, item.ID, convID, b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Chat(ctx context.Context, convID string) ([]model.ChatItem, error) {
	rows, err := s.db.Query(ctx, `SELECT doc FROM chat_items WHERE conversation_id=$1 ORDER BY seq`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ChatItem{}
	for rows.Next() {
		var doc []byte
		if err := rows.Scan(&doc); err != nil {
			return nil, err
		}
		var it model.ChatItem
		_ = json.Unmarshal(doc, &it)
		out = append(out, it)
	}
	return out, rows.Err()
}

// ---------- approvals (D-024) ----------

type ApprovalRecord struct {
	ID, StepID, ConversationID, RunID, LaunchEpoch, RequestKey, PayloadDigest, ExecDigest string
	Reason, LossTitle, Warning                                                            string
	Loss                                                                                  []string
	State                                                                                 string
}

func (s *Store) CreateApproval(ctx context.Context, a ApprovalRecord) error {
	loss, _ := json.Marshal(a.Loss)
	_, err := s.db.Exec(ctx, `INSERT INTO approvals(id,step_id,conversation_id,run_id,launch_epoch,request_key,payload_digest,exec_digest,reason,loss_title,loss,warning)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, a.StepID, a.ConversationID, a.RunID, a.LaunchEpoch, a.RequestKey, a.PayloadDigest, a.ExecDigest, a.Reason, a.LossTitle, loss, a.Warning)
	return err
}

// DecideApproval moves a pending approval to approved or denied in one
// atomic update. It fails with ErrNotPending for anything late or repeated.
func (s *Store) DecideApproval(ctx context.Context, id string, approve bool) (ApprovalRecord, error) {
	state := "denied"
	if approve {
		state = "approved"
	}
	var a ApprovalRecord
	err := s.db.QueryRow(ctx, `UPDATE approvals SET state=$2, decided_at=now() WHERE id=$1 AND state='pending'
		RETURNING id,step_id,conversation_id,run_id,launch_epoch,request_key,payload_digest,exec_digest,state`, id, state).
		Scan(&a.ID, &a.StepID, &a.ConversationID, &a.RunID, &a.LaunchEpoch, &a.RequestKey, &a.PayloadDigest, &a.ExecDigest, &a.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotPending
	}
	return a, err
}

// ExpireApproval marks a still-pending approval expired (run ended, timeout).
func (s *Store) ExpireApproval(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE approvals SET state='expired', decided_at=now() WHERE id=$1 AND state='pending'`, id)
	return err
}

func (s *Store) ExpireRunApprovals(ctx context.Context, runID string) error {
	_, err := s.db.Exec(ctx, `UPDATE approvals SET state='expired', decided_at=now() WHERE run_id=$1 AND state='pending'`, runID)
	return err
}

// ConsumeApproval is the release step: it succeeds once, only for an approved,
// unconsumed approval whose run, epoch and digests still match.
func (s *Store) ConsumeApproval(ctx context.Context, id, runID, epoch, payloadDigest string) error {
	tag, err := s.db.Exec(ctx, `UPDATE approvals SET consumed_at=now()
		WHERE id=$1 AND state='approved' AND consumed_at IS NULL AND run_id=$2 AND launch_epoch=$3 AND payload_digest=$4`,
		id, runID, epoch, payloadDigest)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotPending
	}
	return nil
}

// ConsumeApprovalByID is the release step for commands that run later through
// cloudbuddy exec; the payload was already matched when the release was made.
func (s *Store) ConsumeApprovalByID(ctx context.Context, id, runID, epoch string) error {
	tag, err := s.db.Exec(ctx, `UPDATE approvals SET consumed_at=now()
		WHERE id=$1 AND state='approved' AND consumed_at IS NULL AND run_id=$2 AND launch_epoch=$3`, id, runID, epoch)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotPending
	}
	return nil
}

func (s *Store) approvalsFor(ctx context.Context, convID string) (map[string]*model.Approval, error) {
	rows, err := s.db.Query(ctx, `SELECT id,step_id,state,reason,loss_title,loss,warning,requested_at,decided_at FROM approvals WHERE conversation_id=$1 ORDER BY requested_at`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*model.Approval{}
	for rows.Next() {
		var a model.Approval
		var stepID string
		var loss []byte
		if err := rows.Scan(&a.ID, &stepID, &a.State, &a.Reason, &a.LossTitle, &loss, &a.Warning, &a.RequestedAt, &a.DecidedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(loss, &a.Loss)
		out[stepID] = &a // the latest approval for a step wins
	}
	return out, rows.Err()
}

// ---------- releases (D-036) ----------

type Release struct {
	TokenHash, StepID, RunID, LaunchEpoch, ApprovalID, Command, Cwd, NetworkKind, ExecDigest string
}

func (s *Store) CreateRelease(ctx context.Context, r Release) error {
	var appr any
	if r.ApprovalID != "" {
		appr = r.ApprovalID
	}
	_, err := s.db.Exec(ctx, `INSERT INTO releases(token_hash,step_id,run_id,launch_epoch,approval_id,command,cwd,network_kind,exec_digest) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.TokenHash, r.StepID, r.RunID, r.LaunchEpoch, appr, r.Command, r.Cwd, r.NetworkKind, r.ExecDigest)
	return err
}

// ConsumeRelease uses a release token once. It returns ErrNotPending for an
// unknown, used or stale token.
func (s *Store) ConsumeRelease(ctx context.Context, tokenHash, epoch string) (Release, error) {
	var r Release
	var appr *string
	err := s.db.QueryRow(ctx, `UPDATE releases SET consumed_at=now() WHERE token_hash=$1 AND consumed_at IS NULL AND launch_epoch=$2 AND created_at > now() - interval '10 minutes'
		RETURNING token_hash,step_id,run_id,launch_epoch,approval_id,command,cwd,network_kind,exec_digest`, tokenHash, epoch).
		Scan(&r.TokenHash, &r.StepID, &r.RunID, &r.LaunchEpoch, &appr, &r.Command, &r.Cwd, &r.NetworkKind, &r.ExecDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotPending
	}
	if appr != nil {
		r.ApprovalID = *appr
	}
	return r, err
}

// ---------- link tokens ----------

func (s *Store) AddLinkTokens(ctx context.Context, convID, stepID string, tokens map[string]string) error {
	for h, what := range tokens {
		if _, err := s.db.Exec(ctx, `INSERT INTO link_tokens(conversation_id,step_id,token_hash,what) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, convID, stepID, h, what); err != nil {
			return err
		}
	}
	return nil
}

// FindLinks returns, for each value, the earlier step where it first appeared.
// A value echoed again by later outputs still points to that first step.
func (s *Store) FindLinks(ctx context.Context, convID, stepID string, hashes []string) ([]model.Use, error) {
	if len(hashes) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (o.step_id) o.step_id, o.what FROM (
			SELECT DISTINCT ON (l.token_hash) l.token_hash, l.step_id, l.what FROM link_tokens l JOIN steps s ON s.id=l.step_id
			WHERE l.conversation_id=$1 AND l.token_hash = ANY($2) AND l.step_id <> $3
				AND s.seq < COALESCE((SELECT seq FROM steps WHERE id=$3), 2147483647)
			ORDER BY l.token_hash, s.seq
		) o ORDER BY o.step_id`, convID, hashes, stepID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Use
	for rows.Next() {
		var u model.Use
		if err := rows.Scan(&u.FromStepID, &u.What); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// FirstProducers maps each hash to the step where that value first appeared.
func (s *Store) FirstProducers(ctx context.Context, convID string, hashes []string) (map[string]string, error) {
	out := map[string]string{}
	if len(hashes) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT ON (l.token_hash) l.token_hash, l.step_id FROM link_tokens l JOIN steps s ON s.id=l.step_id
		WHERE l.conversation_id=$1 AND l.token_hash = ANY($2) ORDER BY l.token_hash, s.seq`, convID, hashes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h, id string
		if err := rows.Scan(&h, &id); err != nil {
			return nil, err
		}
		out[h] = id
	}
	return out, rows.Err()
}

// ---------- connections ----------

func (s *Store) SaveConnections(ctx context.Context, conns []model.Connection) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM connections`); err != nil {
		return err
	}
	for _, c := range conns {
		c.Rule = ""
		b, _ := json.Marshal(c)
		if _, err := tx.Exec(ctx, `INSERT INTO connections(id,doc) VALUES ($1,$2)`, c.ID, b); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Connections(ctx context.Context) ([]model.Connection, time.Time, error) {
	rows, err := s.db.Query(ctx, `SELECT c.doc, COALESCE(r.rule,''), c.scanned_at FROM connections c LEFT JOIN connection_rules r ON r.connection_id=c.id ORDER BY c.doc->>'kind', c.doc->>'name'`)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	var out []model.Connection
	var scanned time.Time
	for rows.Next() {
		var doc []byte
		var rule string
		var c model.Connection
		if err := rows.Scan(&doc, &rule, &scanned); err != nil {
			return nil, scanned, err
		}
		_ = json.Unmarshal(doc, &c)
		c.Rule = rule
		if c.Rule == "" {
			c.Rule = model.RuleIrreversible
		}
		out = append(out, c)
	}
	return out, scanned, rows.Err()
}

func (s *Store) SetRule(ctx context.Context, connID, rule string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO connection_rules(connection_id,rule) VALUES ($1,$2) ON CONFLICT (connection_id) DO UPDATE SET rule=$2`, connID, rule)
	return err
}

// Rules returns every rule the user set, keyed by connection id.
func (s *Store) Rules(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT connection_id, rule FROM connection_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, r string
		if err := rows.Scan(&id, &r); err != nil {
			return nil, err
		}
		out[id] = r
	}
	return out, rows.Err()
}

// ---------- projects and settings ----------

const projectCols = `p.id, p.name, p.folders, p.updated_at, (SELECT count(*) FROM conversations c WHERE c.project_id=p.id)`

func scanProject(row pgx.Row) (model.Project, error) {
	var p model.Project
	err := row.Scan(&p.ID, &p.Name, &p.Folders, &p.UpdatedAt, &p.ChatCount)
	return p, err
}

// Projects lists every project by name.
func (s *Store) Projects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.Query(ctx, `SELECT `+projectCols+` FROM projects p ORDER BY lower(p.name), p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (model.Project, error) {
	p, err := scanProject(s.db.QueryRow(ctx, `SELECT `+projectCols+` FROM projects p WHERE p.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// SaveProject creates the project, or updates it when the id exists.
func (s *Store) SaveProject(ctx context.Context, p model.Project) error {
	_, err := s.db.Exec(ctx, `INSERT INTO projects(id,name,folders) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET name=$2, folders=$3, updated_at=now()`, p.ID, p.Name, p.Folders)
	return err
}

// DeleteProject removes the project. Its chats stay, outside any project.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM projects WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) Settings(ctx context.Context) (model.Settings, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return model.Settings{}, err
	}
	defer rows.Close()
	var out model.Settings
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "agent":
			out.Agent = v
		case "folder":
			out.Folder = v
		}
	}
	return out, rows.Err()
}

func (s *Store) SaveSettings(ctx context.Context, v model.Settings) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for k, val := range map[string]string{"agent": v.Agent, "folder": v.Folder} {
		if _, err := tx.Exec(ctx, `INSERT INTO settings(key,value) VALUES ($1,$2) ON CONFLICT (key) DO UPDATE SET value=$2`, k, val); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
