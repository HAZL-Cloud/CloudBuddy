package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
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
