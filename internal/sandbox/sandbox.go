// Package sandbox builds the profile cloudbuddy-exec runs every command
// under: network on, CloudBuddy's own files and the agents' settings
// off-limits (D-027, D-036, D-065).
package sandbox

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// CanWrap reports whether this machine can run approved project code with
// network while keeping file rules. macOS uses sandbox-exec; Linux support
// (bubblewrap) is not built yet, so there such code is denied (D-038 rule).
func CanWrap() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

// Paths lists what the profile protects.
type Paths struct {
	DataDir string
	Home    string
	Bin     string
	Port    int
	ShimDir string // CloudBuddy's CLI shims (D-065): commands may run them, not change them
}

// MacProfile returns a sandbox-exec (Seatbelt) profile. Since D-065 commands
// run as they would in a terminal: network on, every file the user can reach.
// Only self-protection remains (D-027): the agent can't read or change
// CloudBuddy's data, the agents' settings and logins, CloudBuddy's binary and
// shims, or call CloudBuddy's port (where it could approve its own requests).
func MacProfile(p Paths) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	fmt.Fprintf(&b, "(deny file-read* file-write* (subpath %s))\n", quote(real(p.DataDir)))
	for _, f := range []string{".claude/settings.json", ".claude/settings.local.json", ".claude.json", ".codex/config.toml", ".codex/rules"} {
		fmt.Fprintf(&b, "(deny file-write* (literal %s) (subpath %s))\n", quote(filepath.Join(p.Home, f)), quote(filepath.Join(p.Home, f)))
	}
	for _, f := range []string{".claude/.credentials.json", ".codex/auth.json"} {
		fmt.Fprintf(&b, "(deny file-read* file-write* (literal %s))\n", quote(filepath.Join(p.Home, f)))
	}
	if p.Bin != "" {
		fmt.Fprintf(&b, "(deny file-write* (literal %s))\n", quote(real(p.Bin)))
	}
	if p.ShimDir != "" {
		fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quote(real(p.ShimDir)))
	}
	if p.Port != 0 {
		fmt.Fprintf(&b, "(deny network-outbound (remote ip \"localhost:%d\"))\n", p.Port)
	}
	return b.String()
}

func quote(p string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(p, `\`, `\\`), `"`, `\"`) + `"`
}

func real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
