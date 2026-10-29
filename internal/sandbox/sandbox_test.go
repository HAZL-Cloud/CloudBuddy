package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// D-065: commands run like in a terminal; only self-protection is denied.
func TestProfileKeepsOnlySelfProtection(t *testing.T) {
	p := MacProfile(Paths{DataDir: "/Users/test/.cloudbuddy", Home: "/Users/test", Bin: "/opt/cb/cloudbuddy", Port: 7420, ShimDir: "/tmp/cb/shims"})
	for _, want := range []string{
		`(deny file-read* file-write* (subpath "/Users/test/.cloudbuddy"))`,
		`(deny file-write* (literal "/Users/test/.claude/settings.json")`,
		`(deny file-read* file-write* (literal "/Users/test/.claude/.credentials.json"))`,
		`(deny file-write* (literal "/opt/cb/cloudbuddy"))`,
		`(deny file-write* (subpath "/tmp/cb/shims"))`,
		`(deny network-outbound (remote ip "localhost:7420"))`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %s in\n%s", want, p)
		}
	}
	// no blanket write limit, no secret-folder or config-file rules any more
	for _, gone := range []string{"(deny file-write*)\n", ".ssh", ".config/stripe", "Library/Keychains"} {
		if strings.Contains(p, gone) {
			t.Errorf("profile still has %q", gone)
		}
	}
}

// The profile really works under sandbox-exec: ordinary writes run, while
// startup files, PATH folders, CloudBuddy's data and the escape programs
// are refused (Codex review of D-065).
func TestProfileUnderSandboxExec(t *testing.T) {
	if !CanWrap() {
		t.Skip("needs macOS sandbox-exec")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "bin")
	data := filepath.Join(home, ".cloudbuddy")
	for _, d := range []string{bin, data, filepath.Join(home, "project")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := MacProfile(Paths{DataDir: data, Home: home, Port: 7420, DenyWrite: []string{bin}})
	run := func(script string) error {
		return exec.Command("/usr/bin/sandbox-exec", "-p", p, "/bin/sh", "-c", script).Run()
	}
	if err := run("echo ok > " + filepath.Join(home, "project", "out.txt")); err != nil {
		t.Errorf("an ordinary write was refused: %v", err)
	}
	for name, script := range map[string]string{
		"shell startup file": "echo evil >> " + filepath.Join(home, ".zshrc"),
		"PATH folder":        "echo evil > " + filepath.Join(bin, "ps"),
		"LaunchAgents":       "mkdir -p " + filepath.Join(home, "Library", "LaunchAgents") + " && echo x > " + filepath.Join(home, "Library", "LaunchAgents", "x.plist"),
		"CloudBuddy data":    "cat " + filepath.Join(data, "x") + " 2>/dev/null || ls " + data,
		"open":               "/usr/bin/open -h",
		"osascript":          "/usr/bin/osascript -e 'return 1'",
	} {
		if err := run(script); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
}
