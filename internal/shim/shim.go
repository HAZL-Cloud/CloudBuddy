// Package shim is `cloudbuddy shim <cli> [args]`, which stands in for a cloud
// CLI (aws, supabase, stripe, …) on the PATH of every command CloudBuddy
// runs (D-065). When a script calls the CLI, the shim tells CloudBuddy, which
// shows the call as its own card and asks first if it can't be undone. Then
// the shim runs the real CLI unchanged.
//
// It is a view into scripts, not a wall: a script can still call a CLI by its
// full path. If CloudBuddy can't be reached the call runs anyway, with a
// warning; only an explicit "no" stops it.
package shim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Env names the shim reads; cloudbuddy exec sets them.
const (
	EnvSock  = "CB_SHIM_SOCK"
	EnvToken = "CB_SHIM_TOKEN"
	EnvDir   = "CB_SHIM_DIR"
)

const maxOutput = 64 << 10

// Script is the file CloudBuddy writes for one CLI in the shim folder.
func Script(bin, name string) string {
	return fmt.Sprintf("#!/bin/sh\nexec '%s' shim '%s' \"$@\"\n", strings.ReplaceAll(bin, "'", `'\''`), name)
}

type startAnswer struct {
	Decision string `json:"decision"` // allow | deny
	Reason   string `json:"reason"`
	Key      string `json:"key"`
}

func Main(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "cloudbuddy shim: which CLI?")
		return 2
	}
	name, rest := args[0], args[1:]
	real := findReal(name)
	if real == "" {
		fmt.Fprintf(os.Stderr, "%s: command not found\n", name)
		return 127
	}
	sock, token := os.Getenv(EnvSock), os.Getenv(EnvToken)
	if sock == "" || token == "" {
		return run(real, rest, nil)
	}
	c := client(sock)
	cwd, _ := os.Getwd()
	var ans startAnswer
	if err := post(c, "/shim/start", map[string]any{"token": token, "prog": name, "args": rest, "cwd": cwd}, &ans); err != nil {
		fmt.Fprintf(os.Stderr, "[CloudBuddy couldn't record this %s call (%v); it runs without a card.]\n", name, err)
		return run(real, rest, nil)
	}
	if ans.Decision != "allow" {
		fmt.Fprintf(os.Stderr, "CloudBuddy stopped this %s call: %s\n", name, ans.Reason)
		return 126
	}
	if ans.Key == "" { // the step that ran this command already shows the call
		return run(real, rest, nil)
	}
	var out tail
	code := run(real, rest, &out)
	_ = post(c, "/shim/end", map[string]any{"token": token, "key": ans.Key, "exitCode": code, "output": out.String()}, nil)
	return code
}

// findReal finds the CLI on PATH, skipping CloudBuddy's shim folder.
func findReal(name string) string {
	shimDir := os.Getenv(EnvDir)
	self, _ := os.Executable()
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d == "" || (shimDir != "" && filepath.Clean(d) == filepath.Clean(shimDir)) {
			continue
		}
		p := filepath.Join(d, name)
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 {
			continue
		}
		if r, err := filepath.EvalSymlinks(p); err == nil && self != "" && r == self {
			continue
		}
		if b, err := os.ReadFile(p); err == nil && len(b) < 512 && bytes.Contains(b, []byte(" shim '")) {
			continue // another CloudBuddy shim
		}
		return p
	}
	return ""
}

// run starts the real CLI with the same input and output, passes signals on,
// and keeps the end of its output for the card.
func run(real string, args []string, out *tail) int {
	cmd := exec.Command(real, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if out != nil {
		cmd.Stdout = io.MultiWriter(os.Stdout, out)
		cmd.Stderr = io.MultiWriter(os.Stderr, out)
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", filepath.Base(real), err)
		return 127
	}
	go func() {
		for s := range sigs {
			_ = cmd.Process.Signal(s)
		}
	}()
	err := cmd.Wait()
	if ee, ok := err.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	if err != nil {
		return 1
	}
	return 0
}

func client(sock string) *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", sock)
	}}}
}

func post(c *http.Client, path string, body, out any) error {
	b, _ := json.Marshal(body)
	resp, err := c.Post("http://cloudbuddy"+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%s", strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// tail keeps the last maxOutput bytes written to it.
type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > maxOutput {
		t.b = t.b[len(t.b)-maxOutput:]
	}
	return len(p), nil
}

func (t *tail) String() string { return string(t.b) }
