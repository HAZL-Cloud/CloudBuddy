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
	write(shims, Script("/opt/cb/cloudbuddy", "aws"))
	write(other, Script("/old/cloudbuddy", "aws")) // a shim from another launch
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
	if got := Script("/a/it's/cloudbuddy", "aws"); !strings.Contains(got, `exec '/a/it'\''s/cloudbuddy' shim 'aws' "$@"`) {
		t.Errorf("script: %s", got)
	}
}
