package run

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cloudbuddy/internal/recognize"
	"cloudbuddy/internal/sandbox"
	"cloudbuddy/internal/shim"
)

// Protect sets what commands may not change, worked out once at start-up
// (Codex review of D-065): every folder on the user's PATH and on
// CloudBuddy's own, the real homes of the programs CloudBuddy runs outside
// the sandbox (claude, direnv), and direnv's list of allowed .envrc files.
// A command that could put a program in any of these would get it run
// outside the sandbox later.
func (e *Engine) Protect(paths []string, programs []string) {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		d = filepath.Clean(d)
		if d == "" || d == "/" || d == "." || seen[d] {
			return
		}
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			return
		}
		seen[d] = true
		out = append(out, d)
	}
	for _, p := range paths {
		for _, d := range filepath.SplitList(p) {
			add(d)
		}
	}
	for _, prog := range programs {
		if prog == "" {
			continue
		}
		add(filepath.Dir(prog))
		if r, err := filepath.EvalSymlinks(prog); err == nil {
			add(filepath.Dir(r))
		}
	}
	add(filepath.Join(e.cfg.Home, ".local", "share", "direnv"))
	sort.Strings(out)
	e.denyWrite = out
	if len(paths) > 0 {
		e.loginPath = paths[0]
	}
}

// profile is the sandbox profile for commands and for the folder's direnv
// setup.
func (e *Engine) profile() string {
	return sandbox.MacProfile(sandbox.Paths{DataDir: e.cfg.DataDir, Home: e.cfg.Home, Bin: e.cfg.Bin, Port: e.cfg.Port,
		ShimDir: e.shimRoot, DenyWrite: e.denyWrite})
}

// runShims writes one shim per cloud CLI found on this run's PATH (after
// direnv, so a toolchain the project adds is covered too), with the path of
// the installed copy, so the shim can tell when another one runs.
func (e *Engine) runShims(r *Run) string {
	if e.shimRoot == "" {
		return ""
	}
	dir := filepath.Join(e.shimRoot, r.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	for _, name := range recognize.ShimCLIs() {
		if lookIn(r.Env["PATH"], name) == "" {
			continue
		}
		// the installed copy (on the login PATH); a different one the run
		// would use is shown on its card (Codex review of D-065)
		_ = os.WriteFile(filepath.Join(dir, name), []byte(shim.Script(e.cfg.Bin, name, lookIn(e.loginPath, name))), 0o755)
	}
	return dir
}

func lookIn(path, name string) string {
	for _, d := range filepath.SplitList(path) {
		if d == "" {
			continue
		}
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			if b, err := os.ReadFile(p); err == nil && len(b) < 512 && strings.Contains(string(b), " shim '") {
				continue // a CloudBuddy shim
			}
			return p
		}
	}
	return ""
}
