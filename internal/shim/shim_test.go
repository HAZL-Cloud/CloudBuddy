package shim

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shim finds the real CLI behind it on PATH, never itself or another
// CloudBuddy shim.
func TestFindRealSkipsShims(t *testing.T) {
	shims, other, real := t.TempDir(), t.TempDir(), t.TempDir()
	write := func(dir, body string) {
		if err := os.WriteFile(filepath.Join(dir, "aws"), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(shims, Script("/opt/cb/cloudbuddy", "aws", "/x/aws"))
	write(other, Script("/old/cloudbuddy", "aws", "/x/aws")) // a shim from another launch
	write(real, "#!/bin/sh\necho real aws\n")
	t.Setenv(EnvDir, shims)
	t.Setenv("PATH", strings.Join([]string{shims, other, real}, string(os.PathListSeparator)))
	if got := findReal("aws"); got != filepath.Join(real, "aws") {
		t.Fatalf("found %q", got)
	}
	if got := findReal("gcloud"); got != "" {
		t.Fatalf("found a CLI that isn't installed: %q", got)
	}
}

func TestScriptQuotesTheBinary(t *testing.T) {
	if got := Script("/a/it's/cloudbuddy", "aws", "/opt/homebrew/bin/aws"); !strings.Contains(got, `CB_SHIM_EXPECTED='/opt/homebrew/bin/aws' exec '/a/it'\''s/cloudbuddy' shim 'aws' "$@"`) {
		t.Errorf("script: %s", got)
	}
}

// Without its token (a script cleared it) the shim can't ask CloudBuddy. It
// then decides with the gate's rules: a delete doesn't run, a read does.
func TestUnaskedCallsDecideSafely(t *testing.T) {
	realDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	if err := os.WriteFile(filepath.Join(realDir, "aws"), []byte("#!/bin/sh\necho \"$@\" >> "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", realDir)
	t.Setenv(EnvToken, "")
	t.Setenv(EnvSock, "")
	t.Setenv(EnvExpected, filepath.Join(realDir, "aws"))
	Decide = func(name string, args []string, cwd string) bool { return !strings.Contains(strings.Join(args, " "), " rb ") && args[1] != "rb" }
	defer func() { Decide = nil }()
	if code := Main([]string{"aws", "s3", "rb", "s3://x"}); code != 126 {
		t.Errorf("delete without a token: exit %d", code)
	}
	if code := Main([]string{"aws", "s3", "ls"}); code != 0 {
		t.Errorf("read without a token: exit %d", code)
	}
	b, _ := os.ReadFile(marker)
	if string(b) != "s3 ls\n" {
		t.Errorf("real CLI ran with: %q", b)
	}
}
