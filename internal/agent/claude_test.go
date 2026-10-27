package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The parser is checked against a real Claude Code 2.1.311 stream (S3).
func TestParseClaudeFixture(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	fx := filepath.Join(filepath.Dir(f), "..", "..", "testdata", "fixtures", "claude", "basic.stream.jsonl")
	fh, err := os.Open(fx)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	counts := map[string]int{}
	starts := map[string]bool{}
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		for _, e := range ParseClaudeLine(sc.Bytes()) {
			counts[e.Kind]++
			if e.Kind == "tool_start" {
				starts[e.ToolKey] = true
				if e.ToolName == "Bash" && e.Input["description"] == nil {
					t.Errorf("Bash tool_use without description (S5): %+v", e.Input)
				}
			}
			if e.Kind == "tool_end" && !starts[e.ToolKey] {
				t.Errorf("tool_end before tool_start: %s", e.ToolKey)
			}
		}
	}
	if counts["session"] != 1 || counts["done"] != 1 || counts["tool_start"] != 7 || counts["tool_end"] != 7 || counts["text"] < 1 {
		t.Errorf("event counts: %v", counts)
	}
}

func TestClaudeArgsAddProjectFolders(t *testing.T) {
	args := claudeArgs(StartOptions{Prompt: "hi", Workdir: "/code/app", Folders: []string{"/code/infra", "/code/docs"}}, "s.json", "m.json")
	got := 0
	for i, a := range args {
		if strings.HasPrefix(a, "--add-dir") {
			got++
			if a != "--add-dir=/code/infra" && a != "--add-dir=/code/docs" {
				t.Errorf("arg %d: %q", i, a)
			}
		}
	}
	if got != 2 {
		t.Errorf("want 2 --add-dir, got %d: %v", got, args)
	}
	for _, a := range claudeArgs(StartOptions{Prompt: "hi"}, "s.json", "m.json") {
		if strings.HasPrefix(a, "--add-dir") {
			t.Errorf("no folders, but %q", a)
		}
	}
}
