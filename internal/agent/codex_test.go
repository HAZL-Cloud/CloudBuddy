package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The Codex notification parser is checked against a real Codex 0.163.0
// app-server log (S2/S3): commands, the gateway MCP call (skipped), text.
func TestParseCodexFixture(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	fh, err := os.Open(filepath.Join(filepath.Dir(f), "..", "..", "testdata", "fixtures", "codex", "basic.protocol.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	counts := map[string]int{}
	var cmds []string
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var line struct {
			Dir string `json:"dir"`
			Msg struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			} `json:"msg"`
		}
		if json.Unmarshal(sc.Bytes(), &line) != nil || line.Dir != "in" {
			continue
		}
		for _, e := range parseCodexNotification(line.Msg.Method, line.Msg.Params, map[string]bool{"cb": true}) {
			counts[e.Kind]++
			if e.Kind == "tool_end" {
				cmds = append(cmds, e.Command)
			}
		}
	}
	// 3 shell commands; the 2 MCP calls went through the "cb" gateway and are
	// reported by it, not by the driver
	if counts["tool_start"] != 3 || counts["tool_end"] != 3 || counts["text"] < 1 {
		t.Fatalf("counts %v cmds %v", counts, cmds)
	}
	if unwrap := cmds[0]; unwrap == "" {
		t.Fatal("no command text")
	}
}
