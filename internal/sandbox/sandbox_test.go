package sandbox

import (
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
