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
	DBPort  int    // CloudBuddy's database: commands can't reach it (D-027)
	// DenyWrite are folders commands may not change: every folder on the
	// user's PATH and where the programs CloudBuddy itself runs live (claude,
	// direnv, the cloud CLIs). A command that could put a program there
	// would get it run outside the sandbox later, by CloudBuddy or by the
	// user's own terminal (Codex review of D-065).
	DenyWrite []string
}

// startupFiles run code in the user's next shell or log-in session: a
// command that changed them would get code run outside the sandbox.
var startupFiles = []string{".zshrc", ".zshenv", ".zprofile", ".zlogin", ".zlogout", ".bashrc", ".bash_profile",
	".bash_login", ".profile", ".config/fish", "Library/LaunchAgents", ".config/direnv",
	// the agents' own folders: plugins, hooks, skills and settings Claude Code
	// loads outside the sandbox
	".claude", ".codex",
	// run by the user's next git command or terminal window
	".gitconfig", ".config/git", "Library/Preferences/com.apple.Terminal.plist",
	"Library/Preferences/com.googlecode.iterm2.plist", "Library/Application Support/iTerm2"}

// controlSockets belong to terminal multiplexers and terminal apps that can
// open a window running any command, outside the sandbox.
var controlSockets = []string{".local/state/cmux", "Library/Application Support/iTerm2", ".local/share/wezterm",
	".screen", ".local/share/zellij"}

// escapes start programs outside the sandbox (through launchd, Apple
// events, cron or a terminal multiplexer), so commands can't run them.
var escapes = []string{"/usr/bin/open", "/usr/bin/osascript", "/bin/launchctl", "/usr/bin/crontab", "/usr/bin/at",
	"/usr/bin/batch", "/usr/bin/screen", "/opt/homebrew/bin/tmux", "/usr/local/bin/tmux", "/usr/bin/defaults"}

// MacProfile returns a sandbox-exec (Seatbelt) profile. Since D-065 commands
// run as they would in a terminal: network on, the user's files readable and
// mostly writable. Only self-protection remains (D-027): the agent can't read
// or change CloudBuddy's data, the agents' settings and logins, CloudBuddy's
// binary and shims, or call CloudBuddy's port (where it could approve its own
// requests), and it can't plant code that something outside the sandbox
// would run later (PATH folders, shell startup files, launchd, Apple events).
func MacProfile(p Paths) string {
	var b strings.Builder
	home := real(p.Home) // the sandbox matches resolved paths (/var -> /private/var)
	b.WriteString("(version 1)\n(allow default)\n")
	fmt.Fprintf(&b, "(deny file-read* file-write* (subpath %s))\n", quote(real(p.DataDir)))
	for _, f := range []string{".claude/settings.json", ".claude/settings.local.json", ".claude.json", ".codex/config.toml", ".codex/rules"} {
		fmt.Fprintf(&b, "(deny file-write* (literal %s) (subpath %s))\n", quote(filepath.Join(home, f)), quote(filepath.Join(home, f)))
	}
	for _, f := range []string{".claude/.credentials.json", ".codex/auth.json"} {
		fmt.Fprintf(&b, "(deny file-read* file-write* (literal %s))\n", quote(filepath.Join(home, f)))
	}
	if p.Bin != "" {
		fmt.Fprintf(&b, "(deny file-write* (literal %s))\n", quote(real(p.Bin)))
	}
	if p.ShimDir != "" {
		fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quote(real(p.ShimDir)))
	}
	for _, d := range p.DenyWrite {
		if d != "" && d != "/" {
			fmt.Fprintf(&b, "(deny file-write* (subpath %s))\n", quote(real(d)))
		}
	}
	for _, f := range startupFiles {
		fmt.Fprintf(&b, "(deny file-write* (literal %s) (subpath %s))\n", quote(filepath.Join(home, f)), quote(filepath.Join(home, f)))
	}
	for _, x := range escapes {
		fmt.Fprintf(&b, "(deny process-exec (literal %s))\n", quote(x))
	}
	b.WriteString("(deny appleevent-send)\n")
	if p.Port != 0 {
		fmt.Fprintf(&b, "(deny network-outbound (remote ip \"localhost:%d\"))\n", p.Port)
	}
	if p.DBPort != 0 {
		fmt.Fprintf(&b, "(deny network-outbound (remote ip \"localhost:%d\"))\n", p.DBPort)
	}
	for _, s := range controlSockets {
		fmt.Fprintf(&b, "(deny network-outbound (remote unix-socket (subpath %s)))\n", quote(filepath.Join(home, s)))
	}
	b.WriteString("(deny network-outbound (remote unix-socket (path-regex #\"^/private/tmp/tmux-\")))\n")
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
