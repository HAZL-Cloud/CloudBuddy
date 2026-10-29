// Package shellenv builds the environment an agent starts with: the user's
// own login shell environment plus the project folder's direnv setup, so its
// commands see the same CLIs, logins and accounts as in the user's terminal
// (D-065).
package shellenv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const marker = "\n__CLOUDBUDDY_ENV__\n"

var (
	loginOnce sync.Once
	loginEnv  map[string]string
)

// Login returns the environment of the user's interactive login shell,
// captured once from the home folder (so no project's direnv leaks in). If
// that fails, it falls back to CloudBuddy's own environment.
func Login() map[string]string {
	loginOnce.Do(func() {
		loginEnv = capture()
		if loginEnv == nil {
			loginEnv = parse(os.Environ())
		}
	})
	out := make(map[string]string, len(loginEnv))
	for k, v := range loginEnv {
		out[k] = v
	}
	return out
}

func capture() map[string]string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", `printf '`+strings.ReplaceAll(marker, "\n", `\n`)+`'; env -0`)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.Env = os.Environ()
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return nil
	}
	i := bytes.LastIndex(out.Bytes(), []byte(marker))
	if i < 0 {
		return nil
	}
	env := parse(strings.Split(string(out.Bytes()[i+len(marker):]), "\x00"))
	if env["PATH"] == "" || env["HOME"] == "" {
		return nil
	}
	// direnv's bookkeeping from the home folder isn't the project's
	for k := range env {
		if strings.HasPrefix(k, "DIRENV_") {
			delete(env, k)
		}
	}
	return env
}

func parse(kvs []string) map[string]string {
	env := map[string]string{}
	for _, kv := range kvs {
		if i := strings.IndexByte(kv, '='); i > 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	return env
}

// Direnv applies the folder's direnv setup to env, as the user's terminal
// does on cd. It returns a note for the user when the folder has an .envrc
// that direnv won't load (not allowed yet), and nothing when there is no
// direnv or no .envrc.
//
// bin is direnv's full path (found once at start-up, "" for none). With a
// sandbox profile, direnv and the .envrc it runs work inside it, like any
// command: a command may have changed the .envrc or a file it sources.
func Direnv(env map[string]string, dir, bin, profile string) string {
	if bin == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "export", "json")
	if profile != "" {
		cmd = exec.CommandContext(ctx, "/usr/bin/sandbox-exec", "-p", profile, bin, "export", "json")
	}
	cmd.Dir = dir
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	if strings.Contains(errb.String(), "is blocked") {
		return "This folder's .envrc isn't allowed in direnv yet, so its settings weren't loaded. Run `direnv allow` in the folder to use them."
	}
	if runErr != nil || out.Len() == 0 {
		return ""
	}
	var diff map[string]*string
	if json.Unmarshal(out.Bytes(), &diff) != nil {
		return ""
	}
	for k, v := range diff {
		if v == nil {
			delete(env, k)
		} else {
			env[k] = *v
		}
	}
	return ""
}

// Find looks for a program on a PATH value.
func Find(path, name string) string { return lookIn(path, name) }

func lookIn(path, name string) string {
	for _, d := range strings.Split(path, ":") {
		p := d + "/" + name
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// List turns env into KEY=value pairs.
func List(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
