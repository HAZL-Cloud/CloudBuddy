// Package execwrap is `cloudbuddy exec`: the only program Claude Code may run
// outside its sandbox (D-036). It uses a one-time release token to fetch the
// exact command the gate allowed, runs it under CloudBuddy's own sandbox
// profile (network on, protected files off), then removes anything it left
// running.
package execwrap

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
		Command, Cwd, Profile, Kind, StepID, Path string
	}
	if err := c.Post("/gate/release", map[string]string{"token": token, "cwd": cwd}, &info); err != nil {
		fmt.Fprintln(os.Stderr, "CloudBuddy refused to run this command:", err)
		return 126
	}
	mark := config.Random(12)
	env := []string{"CB_EXEC_MARK=" + mark}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CB_") || (info.Path != "" && strings.HasPrefix(kv, "PATH=")) ||
			strings.HasPrefix(kv, "AWS_PAGER=") || strings.HasPrefix(kv, "PAGER=") || strings.HasPrefix(kv, "GIT_PAGER=") {
			continue
		}
		env = append(env, kv)
	}
	if info.Path != "" {
		env = append(env, "PATH="+info.Path)
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
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	code := 0
	// Hard limit, so the folder lock is never released while code still runs
	// (release review). The server's own fallback is longer than this.
	timer := time.AfterFunc(maxRuntime, func() {
		fmt.Fprintln(os.Stderr, "\n[CloudBuddy stopped this command: it ran longer than 2 hours.]")
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})
	defer timer.Stop()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			fmt.Fprintln(os.Stderr, "CloudBuddy couldn't start the command:", err)
			code = 127
		}
	}
	left := killLeftovers(cmd, mark)
	if left > 0 {
		fmt.Fprintf(os.Stderr, "\n[CloudBuddy stopped %d background process(es) this command left running.]\n", left)
	}
	_ = c.Post("/gate/exec/finished", map[string]any{"token": token, "exitCode": code, "leftovers": left}, nil)
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
