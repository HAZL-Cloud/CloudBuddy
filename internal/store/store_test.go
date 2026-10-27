package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"cloudbuddy/internal/model"
)

// testStore opens a fresh database for each test run. It skips when the dev
// PostgreSQL isn't running (docker compose up -d).
func testStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	admin := os.Getenv("CB_TEST_ADMIN_URL")
	if admin == "" {
		admin = "postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/cloudbuddy?sslmode=disable"
	}
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("PostgreSQL not available: %v", err)
	}
	name := fmt.Sprintf("cb_test_%d", os.Getpid())
	_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)
	s, err := Open(ctx, fmt.Sprintf("postgres://cloudbuddy:cloudbuddy@127.0.0.1:54329/%s?sslmode=disable", name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		c, err := pgx.Connect(ctx, admin)
		if err == nil {
			_, _ = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			c.Close(ctx)
		}
	})
	return s
}

func seed(t *testing.T, s *Store) (convID, runID, stepID string) {
	ctx := context.Background()
	convID, runID = NewID("cv"), NewID("run")
	if err := s.CreateConversation(ctx, Conversation{ID: convID, Title: "t", Agent: "claude", Workdir: "/w", Status: model.ConvRunning}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRun(ctx, runID, convID, "epoch1", "2.1.311"); err != nil {
		t.Fatal(err)
	}
	st, err := s.UpsertStep(ctx, convID, runID, "tool-1", func(st *model.Step, isNew bool) { st.Title = "x" })
	if err != nil {
		t.Fatal(err)
	}
	return convID, runID, st.ID
}

func TestApprovalDecidedOnceAndConsumedOnce(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	conv, run, step := seed(t, s)
	a := ApprovalRecord{ID: NewID("ap"), StepID: step, ConversationID: conv, RunID: run, LaunchEpoch: "epoch1", RequestKey: "tool-1", PayloadDigest: "d1", Reason: "delete"}
	if err := s.CreateApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	// consuming before a decision fails
	if err := s.ConsumeApproval(ctx, a.ID, run, "epoch1", "d1"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("consumed an undecided approval: %v", err)
	}
	// two concurrent decisions: exactly one wins
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	for _, approve := range []bool{true, false} {
		wg.Add(1)
		go func(approve bool) {
			defer wg.Done()
			_, err := s.DecideApproval(ctx, a.ID, approve)
			wins <- err == nil
		}(approve)
	}
	wg.Wait()
	close(wins)
	n := 0
	for w := range wins {
		if w {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d decisions won, want 1", n)
	}
	// a second approval, decided deterministically, exercises the release step
	a = ApprovalRecord{ID: NewID("ap"), StepID: step, ConversationID: conv, RunID: run, LaunchEpoch: "epoch1", RequestKey: "tool-1b", PayloadDigest: "d1", Reason: "delete"}
	if err := s.CreateApproval(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideApproval(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	// mismatched digest, wrong run, wrong epoch: all rejected
	for _, c := range []struct{ run, epoch, digest string }{{run, "epoch1", "other"}, {"run_x", "epoch1", "d1"}, {run, "epoch2", "d1"}} {
		if err := s.ConsumeApproval(ctx, a.ID, c.run, c.epoch, c.digest); !errors.Is(err, ErrNotPending) {
			t.Fatalf("mismatch %+v accepted: %v", c, err)
		}
	}
	if err := s.ConsumeApproval(ctx, a.ID, run, "epoch1", "d1"); err != nil {
		t.Fatalf("matching consume failed: %v", err)
	}
	if err := s.ConsumeApproval(ctx, a.ID, run, "epoch1", "d1"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("consumed twice")
	}
}

func TestLateDecisionRejected(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	conv, run, step := seed(t, s)
	a := ApprovalRecord{ID: NewID("ap"), StepID: step, ConversationID: conv, RunID: run, LaunchEpoch: "epoch1", RequestKey: "k", PayloadDigest: "d", Reason: "r"}
	_ = s.CreateApproval(ctx, a)
	if err := s.ExpireRunApprovals(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideApproval(ctx, a.ID, true); !errors.Is(err, ErrNotPending) {
		t.Fatalf("approved after expiry: %v", err)
	}
}

func TestReleaseSingleUse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, run, step := seed(t, s)
	r := Release{TokenHash: "h1", StepID: step, RunID: run, LaunchEpoch: "epoch1", Command: "aws s3 ls", Cwd: "/w", NetworkKind: "A"}
	if err := s.CreateRelease(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeRelease(ctx, "h1", "epoch2"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("wrong epoch accepted")
	}
	got, err := s.ConsumeRelease(ctx, "h1", "epoch1")
	if err != nil || got.Command != "aws s3 ls" {
		t.Fatalf("consume: %v %+v", err, got)
	}
	if _, err := s.ConsumeRelease(ctx, "h1", "epoch1"); !errors.Is(err, ErrNotPending) {
		t.Fatalf("release used twice")
	}
}

func TestInterruptOpenRuns(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	conv, run, step := seed(t, s)
	_ = s.CreateApproval(ctx, ApprovalRecord{ID: NewID("ap"), StepID: step, ConversationID: conv, RunID: run, LaunchEpoch: "e", RequestKey: "k", PayloadDigest: "d", Reason: "r"})
	convs, err := s.InterruptOpenRuns(ctx)
	if err != nil || len(convs) != 1 {
		t.Fatalf("interrupt: %v %v", err, convs)
	}
	c, _ := s.GetConversation(ctx, conv)
	if c.Status != model.ConvInterrupted {
		t.Errorf("status %s", c.Status)
	}
	steps, _ := s.Steps(ctx, conv)
	if steps[0].Approval == nil || steps[0].Approval.State != "expired" {
		t.Errorf("approval not expired: %+v", steps[0].Approval)
	}
	if steps[0].Status != model.StatusOutcomeUnknown {
		t.Errorf("running step should become outcome_unknown, got %s", steps[0].Status)
	}
}

func TestChatGroupsSteps(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	conv, run, step := seed(t, s)
	st2, _ := s.UpsertStep(ctx, conv, run, "tool-2", func(st *model.Step, _ bool) {})
	_ = s.AppendChat(ctx, conv, model.ChatItem{Type: "user", Text: "hi"})
	_ = s.AddStepToChat(ctx, conv, step)
	_ = s.AddStepToChat(ctx, conv, st2.ID)
	_ = s.AddStepToChat(ctx, conv, st2.ID) // repeat is a no-op
	items, _ := s.Chat(ctx, conv)
	if len(items) != 2 || len(items[1].StepIDs) != 2 {
		t.Fatalf("chat: %+v", items)
	}
}
