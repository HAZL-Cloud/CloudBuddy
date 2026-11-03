package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"cloudbuddy/internal/config"
)

// The approval's file hash must change when the code a command runs
// changes, however the command names it (Codex review of D-067).
func TestDashDigestFollowsTheFilesThatRun(t *testing.T) {
	dir := t.TempDir()
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("scripts/count users.py", "print(1)")
	write("bin/stats", "#!/bin/sh\necho 1")
	write("conf/q.sql", "select 1")
	env := map[string]string{"PATH": filepath.Join(dir, "bin") + ":/usr/bin:/bin", "QUERY": "conf/q.sql"}
	folders := []string{dir}
	cases := []struct {
		cmd, file string
	}{
		{`python3 "scripts/count users.py"`, "scripts/count users.py"},
		{`stats --json`, "bin/stats"},
		{`psql -f "$QUERY"`, "conf/q.sql"},
		{`node --input=conf/q.sql`, "conf/q.sql"},
	}
	for _, c := range cases {
		before := dashDigest(c.cmd, dir, env, folders)
		if again := dashDigest(c.cmd, dir, env, folders); again != before {
			t.Fatalf("%s: the hash isn't stable", c.cmd)
		}
		write(c.file, "changed "+c.cmd)
		if after := dashDigest(c.cmd, dir, env, folders); after == before {
			t.Errorf("%s: changing %s didn't change the hash", c.cmd, c.file)
		}
	}
	// a different command or folder is a different approval
	if dashDigest("stats", dir, env, folders) == dashDigest("stats --all", dir, env, folders) {
		t.Error("the command isn't part of the hash")
	}
}

// A restart stops the dashboard commands an earlier launch left running,
// and nothing else (Codex review of D-067).
func TestKillDashLeftovers(t *testing.T) {
	e := &Engine{cfg: config.Config{DataDir: t.TempDir(), Epoch: "new-launch"}}
	start := func(mark string) *exec.Cmd {
		cmd := exec.Command("/bin/sleep", "30")
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		if mark != "" {
			cmd.Env = append(cmd.Env, "CB_DASH_MARK="+mark)
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
		return cmd
	}
	old := start("old-launch")     // left by the earlier launch
	reused := start("")            // a pid the earlier launch recorded, now someone else's
	current := start("new-launch") // this launch's own command
	_ = os.WriteFile(e.dashPidFile(old.Process.Pid), []byte("old-launch\n"+procStart(old.Process.Pid)), 0o600)
	_ = os.WriteFile(e.dashPidFile(reused.Process.Pid), []byte("old-launch\nMon Jan  1 00:00:00 2024"), 0o600)
	_ = os.WriteFile(e.dashPidFile(current.Process.Pid), []byte("new-launch\n"+procStart(current.Process.Pid)), 0o600)
	if n := e.killDashLeftovers(); n != 1 {
		t.Fatalf("stopped %d processes, want 1", n)
	}
	done := make(chan error, 1)
	go func() { done <- old.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the leftover command is still running")
	}
	for _, c := range []*exec.Cmd{reused, current} {
		if syscall.Kill(c.Process.Pid, 0) != nil {
			t.Errorf("pid %d was stopped but isn't a leftover", c.Process.Pid)
		}
	}
}
