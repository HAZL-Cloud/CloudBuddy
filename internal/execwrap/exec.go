// Package execwrap is `cloudbuddy exec`: the only program Claude Code may run
// outside its sandbox (D-036). It uses a one-time release token to fetch the
// exact command the gate allowed, runs it under CloudBuddy's own sandbox
// profile (network on, only CloudBuddy's own files off, D-065) with the CLI
// shims and the network log, then removes anything it left running.
package execwrap

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloudbuddy/internal/config"
	"cloudbuddy/internal/gateclient"
)

const maxRuntime = 2 * time.Hour

func Main(args []string) int {
	token := ""
	for i, a := range args {
		if a == "--release" && i+1 < len(args) {
			token = args[i+1]
		}
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "CloudBuddy: cloudbuddy exec only runs commands the gate released.")
		return 126
	}
	c, err := gateclient.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "CloudBuddy:", err)
		return 126
	}
	cwd, _ := os.Getwd()
	var info struct {
		Command, Cwd, Profile, Kind, StepID, PathPrefix string
		Env                                             map[string]string
		Background                                      bool
	}
	if err := c.Post("/gate/release", map[string]any{"token": token, "cwd": cwd, "pid": os.Getpid()}, &info); err != nil {
		fmt.Fprintln(os.Stderr, "CloudBuddy refused to run this command:", err)
		return 126
	}
	mark := config.Random(12)
	env := []string{"CB_EXEC_MARK=" + mark}
	path := os.Getenv("PATH")
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CB_") || strings.HasPrefix(kv, "PATH=") ||
			strings.HasPrefix(kv, "AWS_PAGER=") || strings.HasPrefix(kv, "PAGER=") || strings.HasPrefix(kv, "GIT_PAGER=") {
			continue
		}
		if k := kv[:max(strings.IndexByte(kv, '='), 0)]; info.Env[k] != "" {
			continue
		}
		env = append(env, kv)
	}
	if info.PathPrefix != "" {
		path = info.PathPrefix + string(os.PathListSeparator) + path
	}
	env = append(env, "PATH="+path)
	for k, v := range info.Env {
		env = append(env, k+"="+v)
	}
	// no pagers: a pager is a program the CLI would start
	env = append(env, "AWS_PAGER=", "PAGER=cat", "GIT_PAGER=cat")
	shell := "/bin/zsh"
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}
	var cmd *exec.Cmd
	// zsh -f reads no startup files, so nothing can redefine a CLI as a shell
	// function or alias before the approved command runs. Plain sh reads
	// none in non-interactive mode either.
	shArgs := []string{"-c", info.Command}
	if shell == "/bin/zsh" {
		shArgs = []string{"-f", "-c", info.Command}
	}
	if runtime.GOOS == "darwin" && info.Profile != "" {
		cmd = exec.Command("/usr/bin/sandbox-exec", append([]string{"-p", info.Profile, shell}, shArgs...)...)
	} else {
		cmd = exec.Command(shell, shArgs...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	var out tail
	if info.Background { // the agent reads the output elsewhere; the card keeps the end
		cmd.Stdout, cmd.Stderr = io.MultiWriter(os.Stdout, &out), io.MultiWriter(os.Stderr, &out)
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	code := 0
	// Hard limit, so a forgotten command doesn't run for ever.
	timer := time.AfterFunc(maxRuntime, func() {
		fmt.Fprintln(os.Stderr, "\n[CloudBuddy stopped this command: it ran longer than 2 hours.]")
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})
	defer timer.Stop()
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "CloudBuddy couldn't start the command:", err)
		_ = c.Post("/gate/exec/finished", map[string]any{"token": token, "exitCode": 127}, nil)
		return 127
	}
	// Stop from the card, the agent's timeout or the end of the run: pass it
	// on to the whole command, then make sure it ends.
	// SIGUSR1 is the Stop button: say so, so the agent knows who stopped it.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)
	defer signal.Stop(sigs)
	go func() {
		for s := range sigs {
			sig := s.(syscall.Signal)
			if sig == syscall.SIGUSR1 {
				fmt.Fprintln(os.Stderr, "\n[The user stopped this command in CloudBuddy.]")
				sig = syscall.SIGTERM
			}
			_ = syscall.Kill(-cmd.Process.Pid, sig)
			time.AfterFunc(5*time.Second, func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
		}
	}()
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
			if code < 0 {
				code = 128 + int(syscall.SIGTERM)
			}
		} else {
			code = 1
		}
	}
	left := killLeftovers(cmd, mark)
	if left > 0 {
		fmt.Fprintf(os.Stderr, "\n[CloudBuddy stopped %d background process(es) this command left running.]\n", left)
	}
	_ = c.Post("/gate/exec/finished", map[string]any{"token": token, "exitCode": code, "leftovers": left, "output": out.String()}, nil)
	return code
}

// killLeftovers stops the command's process group and any process carrying
// its environment mark (which catches setsid and double-fork escapes on
// macOS, best effort: a process can scrub its own environment).
func killLeftovers(cmd *exec.Cmd, mark string) int {
	n := 0
	if cmd.Process != nil {
		if syscall.Kill(-cmd.Process.Pid, 0) == nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			n++
		}
	}
	out, err := exec.Command("ps", "-Ewwax", "-o", "pid=,command=").Output()
	if err != nil {
		return n
	}
	self := os.Getpid()
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.Contains(line, "CB_EXEC_MARK="+mark) {
			continue
		}
		f := strings.Fields(line)
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid == self {
			continue
		}
		if syscall.Kill(pid, syscall.SIGKILL) == nil {
			n++
		}
	}
	return n
}

// tail keeps the last 64 KB of a background command's output for its card.
type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > 64<<10 {
		t.b = t.b[len(t.b)-64<<10:]
	}
	return len(p), nil
}

func (t *tail) String() string { return string(t.b) }
