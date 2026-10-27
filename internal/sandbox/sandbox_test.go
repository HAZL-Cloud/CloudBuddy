package sandbox

import (
	"strings"
	"testing"
)

func TestProfileWritesEveryProjectFolder(t *testing.T) {
	p := MacProfile(Paths{DataDir: "/Users/test/.cloudbuddy", Home: "/Users/test", Workdir: "/code/app", Folders: []string{"/code/infra"}, Kind: "B"})
	var allow string
	for _, line := range strings.Split(p, "\n") {
		if strings.HasPrefix(line, "(allow file-write*") {
			allow = line
		}
	}
	for _, d := range []string{"/code/app", "/code/infra"} {
		if !strings.Contains(allow, `(subpath "`+d+`")`) {
			t.Errorf("%s not writable: %s", d, allow)
		}
	}
	// CloudBuddy's folder is denied after the allow list, so the deny wins
	deny := strings.Index(p, `(deny file-read* file-write* (subpath "/Users/test/.cloudbuddy"))`)
	if deny < strings.Index(p, allow) {
		t.Errorf("data folder not denied after the project folders")
	}
}
