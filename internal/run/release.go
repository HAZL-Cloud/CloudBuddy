package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"cloudbuddy/internal/config"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/sandbox"
	"cloudbuddy/internal/store"
)

func randomToken() string { return config.Random(24) }

func hashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

// ReleaseInfo is what cloudbuddy exec needs to run one allowed command.
type ReleaseInfo struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	Profile string `json:"profile"` // sandbox-exec profile (network on, protected files off)
	Kind    string `json:"kind"`    // A | B
	StepID  string `json:"stepId"`
	Path    string `json:"path"` // PATH CloudBuddy was started with, so project folders can't shadow a CLI
}

var ErrStale = errors.New("this release is unknown, used, stale, or its files changed")

// Release is the atomic release step (D-024, D-036): it uses the token once,
// consumes the approval once, and checks the files the command runs haven't
// changed since the user approved it.
func (e *Engine) Release(ctx context.Context, token, _ string) (ReleaseInfo, error) {
	rel, err := e.st.ConsumeRelease(ctx, hashToken(token), e.cfg.Epoch)
	if err != nil {
		return ReleaseInfo{}, ErrStale
	}
	r := e.run(rel.RunID)
	if r == nil || r.ended.Load() {
		return ReleaseInfo{}, ErrStale
	}
	if rel.ApprovalID != "" {
		// the payload digest was checked when the approval was granted; here
		// only single use and the run binding matter
		if err := e.st.ConsumeApprovalByID(ctx, rel.ApprovalID, rel.RunID, e.cfg.Epoch); err != nil {
			return ReleaseInfo{}, ErrStale
		}
	}
	if rel.NetworkKind == "B" {
		// Reserve the project folders before checking the files, and keep
		// them until cloudbuddy exec reports the command finished (D-029).
		lks := e.workLocks(r.projectFolders())
		for _, lk := range lks {
			lk.Lock()
		}
		key := hashToken(token)
		e.mu.Lock()
		e.heldLocks[key] = lks
		e.mu.Unlock()
		go func() { // last resort if the wrapper never reports back
			time.Sleep(2*time.Hour + 10*time.Minute) // after cloudbuddy exec's own 2-hour kill
			e.releaseHold(key)
		}()
		if rel.ExecDigest == "" || filesDigest(rel.Command, rel.Cwd) != rel.ExecDigest {
			e.releaseHold(key)
			_, _, _ = e.updateStepByID(context.Background(), r, rel.StepID, func(s *model.Step) {
				s.Status = model.StatusDenied
				now := time.Now().UTC()
				s.EndedAt = &now
				s.Output = "Not run: the files this command runs changed after you approved it."
			})
			e.bus.Conversation(r.ConvID)
			return ReleaseInfo{}, ErrStale
		}
	}
	return ReleaseInfo{Command: rel.Command, Cwd: rel.Cwd, Kind: rel.NetworkKind, StepID: rel.StepID, Path: os.Getenv("PATH"),
		Profile: sandbox.MacProfile(sandbox.Paths{DataDir: e.cfg.DataDir, Home: e.cfg.Home, Bin: e.cfg.Bin, Port: e.cfg.Port, Workdir: r.Workdir, Folders: r.Folders, Kind: rel.NetworkKind})}, nil
}

// ExecFinished is called by cloudbuddy exec when the command ends.
func (e *Engine) ExecFinished(token string, exitCode int, leftovers int) {
	key := hashToken(token)
	e.releaseHold(key)
	_ = exitCode
	_ = leftovers
}

func (e *Engine) releaseHold(key string) {
	e.mu.Lock()
	lks := e.heldLocks[key]
	delete(e.heldLocks, key)
	e.mu.Unlock()
	for _, lk := range lks {
		lk.Unlock()
	}
}

var _ = store.ErrNotPending
