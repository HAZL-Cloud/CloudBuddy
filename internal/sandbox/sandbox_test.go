package sandbox

import (
	"fmt"
	"net"
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
	home, err := os.MkdirTemp("/tmp", "cbh") // short: unix socket paths are limited
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	bin := filepath.Join(home, ".local", "bin")
	data := filepath.Join(home, ".cloudbuddy")
	for _, d := range []string{bin, data, filepath.Join(home, "project")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	db, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dbPort := db.Addr().(*net.TCPAddr).Port
	sockDir := filepath.Join(home, ".local", "state", "cmux")
	if err := os.MkdirAll(sockDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ctl, err := net.Listen("unix", filepath.Join(sockDir, "c.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer ctl.Close()
	p := MacProfile(Paths{DataDir: data, Home: home, Port: 7420, DBPort: dbPort, DenyWrite: []string{bin}})
	run := func(script string) error {
		return exec.Command("/usr/bin/sandbox-exec", "-p", p, "/bin/sh", "-c", script).Run()
	}
	// control: the port and socket checks work outside the sandbox
	for _, c := range []string{fmt.Sprintf("/usr/bin/nc -z -G 2 127.0.0.1 %d", dbPort), sockConnect(filepath.Join(sockDir, "c.sock"))} {
		if err := exec.Command("/bin/sh", "-c", c).Run(); err != nil {
			t.Fatalf("control failed outside the sandbox: %s: %v", c, err)
		}
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
		"defaults":           "/usr/bin/defaults read NSGlobalDomain AppleLocale",
		"Claude's folder":    "mkdir -p " + filepath.Join(home, ".claude", "plugins") + " && echo x > " + filepath.Join(home, ".claude", "plugins", "hook.sh"),
		"git config":         "echo x >> " + filepath.Join(home, ".gitconfig"),
		"database port":      fmt.Sprintf("/usr/bin/nc -z -G 2 127.0.0.1 %d", dbPort),
		"terminal socket":    sockConnect(filepath.Join(sockDir, "c.sock")),
	} {
		if err := run(script); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
}

// sockConnect is a shell command that connects to a unix socket and fails
// if it can't.
func sockConnect(path string) string {
	return `python3 -I -c "import socket,sys; s=socket.socket(socket.AF_UNIX); s.connect(sys.argv[1])" ` + path
}
