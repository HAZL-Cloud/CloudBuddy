package run

import (
	"os"
	"path/filepath"
	"testing"
)

// The approval's file hash must change when the code a command runs
// changes, however the command names it (Codex review of D-067).
func TestDashDigestFollowsTheFilesThatRun(t *testing.T) {
	dir := t.TempDir()
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		dir = r
	}
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("scripts/count users.py", "print(1)")
	write("bin/stats", "#!/bin/sh\necho 1")
	write("conf/q.sql", "select 1")
	env := map[string]string{"PATH": filepath.Join(dir, "bin") + ":/usr/bin:/bin", "QUERY": "conf/q.sql"}
	folders := []string{dir}
	cases := []struct {
		cmd, file string
	}{
		{`python3 "scripts/count users.py"`, "scripts/count users.py"},
		{`stats --json`, "bin/stats"},
		{`psql -f "$QUERY"`, "conf/q.sql"},
		{`node --input=conf/q.sql`, "conf/q.sql"},
	}
	for _, c := range cases {
		before := dashDigest(c.cmd, dir, env, folders)
		if again := dashDigest(c.cmd, dir, env, folders); again != before {
			t.Fatalf("%s: the hash isn't stable", c.cmd)
		}
		write(c.file, "changed "+c.cmd)
		if after := dashDigest(c.cmd, dir, env, folders); after == before {
			t.Errorf("%s: changing %s didn't change the hash", c.cmd, c.file)
		}
	}
	// a different command or folder is a different approval
	if dashDigest("stats", dir, env, folders) == dashDigest("stats --all", dir, env, folders) {
		t.Error("the command isn't part of the hash")
	}
}
