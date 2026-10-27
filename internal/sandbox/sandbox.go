// Package sandbox builds the profile cloudbuddy-exec uses for commands that
// get network: network on, CloudBuddy's own files and the agents' settings
// off-limits (D-029, D-036).
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
	Workdir string // the only project folder the command may write
}

// cliStateDirs are folders cloud CLIs and package managers write their own
// caches and logins to (S6).
var cliStateDirs = []string{".aws", ".config", ".cache", ".npm", ".pnpm-store", ".yarn", ".wrangler", ".terraform.d",
	".pulumi", ".cdk", ".kube", ".docker", ".azure", ".fly", ".netlify", ".vercel", ".supabase", ".stripe",
	"Library/Caches", "Library/Preferences/.wrangler", "Library/Application Support/com.vercel.cli",
	"Library/Application Support/stripe", ".local/share", ".local/state", "go/pkg/mod"}

func quote(p string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(p, `\`, `\\`), `"`, `\"`) + `"`
}

func real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// MacProfile returns a sandbox-exec (Seatbelt) profile.
func MacProfile(p Paths) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n")
	// Writes: only the project folder, temp folders and CLI state folders.
	// Later rules win, so the protected paths below are denied last.
	b.WriteString("(deny file-write*)\n(allow file-write* (subpath \"/dev\") (subpath \"/private/tmp\") (subpath \"/private/var/folders\") (subpath \"/tmp\")")
	if p.Workdir != "" {
		fmt.Fprintf(&b, " (subpath %s)", quote(real(p.Workdir)))
	}
	for _, d := range cliStateDirs {
		fmt.Fprintf(&b, " (subpath %s)", quote(filepath.Join(p.Home, d)))
	}
	b.WriteString(")\n")
	// CloudBuddy's data: no reading or writing
	fmt.Fprintf(&b, "(deny file-read* file-write* (subpath %s))\n", quote(real(p.DataDir)))
	// the agents' settings and logins: no writing; the logins: no reading
	for _, f := range []string{".claude/settings.json", ".claude/settings.local.json", ".claude.json", ".codex/config.toml", ".codex/rules"} {
		fmt.Fprintf(&b, "(deny file-write* (literal %s) (subpath %s))\n", quote(filepath.Join(p.Home, f)), quote(filepath.Join(p.Home, f)))
	}
	for _, f := range []string{".claude/.credentials.json", ".codex/auth.json"} {
		fmt.Fprintf(&b, "(deny file-read* file-write* (literal %s))\n", quote(filepath.Join(p.Home, f)))
	}
	if p.Bin != "" {
		fmt.Fprintf(&b, "(deny file-write* (literal %s))\n", quote(real(p.Bin)))
	}
	// CloudBuddy's own port
	if p.Port != 0 {
		fmt.Fprintf(&b, "(deny network-outbound (remote ip \"localhost:%d\"))\n", p.Port)
	}
	return b.String()
}
