package dashmcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestProtocol(t *testing.T) {
	var got struct {
		tool, id string
		args     string
	}
	var out bytes.Buffer
	s := &server{out: &out, call: func(tool, id string, args json.RawMessage) (string, bool) {
		got.tool, got.id, got.args = tool, id, string(args)
		return "Saved.", false
	}}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"save_dashboard","arguments":{"title":"T"},"_meta":{"claudecode/toolUseId":"toolu_1"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
	}, "\n")
	s.serve(strings.NewReader(in))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 answers (no answer to a notification), got %d:\n%s", len(lines), out.String())
	}
	byID := map[string]map[string]any{}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		byID[string(mustJSON(m["id"]))] = m
	}
	if v := byID["1"]["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("protocol version: %v", v)
	}
	tools := byID["2"]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Errorf("tools: %d", len(tools))
	}
	for _, tl := range tools {
		if name := tl.(map[string]any)["name"]; name == "delete_dashboard" {
			t.Error("the agent must not get a delete tool")
		}
	}
	res := byID["3"]["result"].(map[string]any)
	if res["isError"] != false || got.tool != "save_dashboard" || got.id != "toolu_1" || got.args != `{"title":"T"}` {
		t.Errorf("call: %+v %+v", res, got)
	}
	if byID["4"]["error"] == nil {
		t.Error("unknown method should be an error")
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
