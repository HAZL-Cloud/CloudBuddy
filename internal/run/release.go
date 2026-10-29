package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"cloudbuddy/internal/config"
	"cloudbuddy/internal/model"
	"cloudbuddy/internal/netlog"
	"cloudbuddy/internal/shim"
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
	Profile string `json:"profile"` // sandbox-exec profile: network on, CloudBuddy's own files off (D-065)
	Kind    string `json:"kind"`    // "" | A | B, as recognized
	StepID  string `json:"stepId"`
	// PathPrefix goes in front of PATH: the folder of CLI shims (D-065).
	PathPrefix string            `json:"pathPrefix,omitempty"`
	Env        map[string]string `json:"env,omitempty"` // shim socket and token, network log proxy
	Background bool              `json:"background,omitempty"`
}

var ErrStale = errors.New("this release is unknown, used, stale, or its files changed")

// Release is the atomic release step (D-024, D-036): it uses the token once,
// consumes the approval once, and for approved project code checks the files
// it runs haven't changed since the user said yes. It also starts the
// command's network log and shim access (D-065).
func (e *Engine) Release(ctx context.Context, token, _ string, pid int) (ReleaseInfo, error) {
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
		if rel.NetworkKind == "B" && (rel.ExecDigest == "" || filesDigest(rel.Command, rel.Cwd) != rel.ExecDigest) {
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
	r.mu.Lock()
	bg := r.bgSteps[rel.StepID]
	r.mu.Unlock()
	x := &execState{runID: r.ID, stepID: rel.StepID, tokenHash: hashToken(token), pid: pid, background: bg,
		quiet: rel.NetworkKind == "A", shimToken: randomToken(), started: time.Now()}
	info := ReleaseInfo{Command: rel.Command, Cwd: rel.Cwd, Kind: rel.NetworkKind, StepID: rel.StepID, Background: bg,
		Profile: e.profile(), Env: map[string]string{}}
	if r.shimDir != "" && e.shimSock != "" {
		info.PathPrefix = r.shimDir
		info.Env[shim.EnvDir] = r.shimDir
		info.Env[shim.EnvSock] = e.shimSock
		info.Env[shim.EnvToken] = x.shimToken
	}
	if e.netlogOn(r) {
		if l, err := netlog.Start(); err == nil {
			x.log = l
			for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
				info.Env[k] = l.URL()
			}
			// keep the user's own exceptions (Codex review of D-065)
			noProxy := "localhost,127.0.0.1,::1,.local,.localhost"
			for _, k := range []string{"NO_PROXY", "no_proxy"} {
				if v := strings.TrimSpace(r.Env[k]); v != "" {
					noProxy += "," + v
				}
			}
			info.Env["NO_PROXY"], info.Env["no_proxy"] = noProxy, noProxy
			info.Env["NODE_USE_ENV_PROXY"] = "1"
		}
	}
	e.trackExec(x)
	return info, nil
}

// netlogOn: the network log is on unless the user already routes traffic
// through a proxy of their own, or turned it off.
func (e *Engine) netlogOn(r *Run) bool {
	if os.Getenv("CB_NO_NETLOG") == "1" {
		return false
	}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if r.Env[k] != "" {
			return false
		}
	}
	return true
}

var _ = store.ErrNotPending
