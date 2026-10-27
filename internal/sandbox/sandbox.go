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
	Kind    string // A: plain cloud CLI (no approval needed) | B: approved code
}

// cliStateDirs are cache and token folders cloud CLIs and package managers
// write while they run (S6). Config files that can run code or change which
// account is used are denied below.
var cliStateDirs = []string{".aws/cli/cache", ".aws/sso/cache", ".config/gcloud", ".config/stripe", ".cache", ".npm",
	".pnpm-store", ".yarn/cache", ".wrangler", ".terraform.d/plugin-cache", ".pulumi", ".cdk", ".kube/cache",
	".azure", ".fly", ".netlify", ".vercel", ".supabase", ".stripe", "Library/Caches",
	"Library/Preferences/.wrangler", "Library/Application Support/com.vercel.cli", "Library/Application Support/stripe",
	".local/share/pnpm", "go/pkg/mod"}

// configFiles can make a later command run code or use another account
// (credential_process, aliases, credential helpers, rc files).
var configFiles = []string{".aws/config", ".aws/credentials", ".config/gcloud/configurations", ".config/gh", ".gitconfig",
	".config/git", ".npmrc", ".yarnrc", ".curlrc", ".config/.curlrc", ".wgetrc", ".netrc", ".ssh", ".docker/config.json",
	".kube/config", ".zshrc", ".zshenv", ".zprofile", ".bashrc", ".bash_profile", ".profile", ".config/stripe/config.toml"}

// secretDirs aren't needed by any cloud CLI: commands that run without the
// user's approval (kind A) can't read them.
var secretDirs = []string{".ssh", ".gnupg", "Library/Keychains", "Library/Application Support/Google/Chrome",
	"Library/Application Support/Firefox", "Library/Safari", "Library/Cookies", "Library/Application Support/Arc",
	".password-store", ".1password", "Library/Group Containers"}

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
	for _, f := range configFiles {
		fmt.Fprintf(&b, "(deny file-write* (literal %s) (subpath %s))\n", quote(filepath.Join(p.Home, f)), quote(filepath.Join(p.Home, f)))
	}
	if p.Kind != "B" {
		for _, d := range secretDirs {
			fmt.Fprintf(&b, "(deny file-read* (subpath %s))\n", quote(filepath.Join(p.Home, d)))
		}
	}
	// CloudBuddy's own port
	if p.Port != 0 {
		fmt.Fprintf(&b, "(deny network-outbound (remote ip \"localhost:%d\"))\n", p.Port)
	}
	return b.String()
}
