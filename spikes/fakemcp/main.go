// Command fakemcp is a tiny stdio MCP server used by the Phase 0 spikes.
// It offers three tools with different risk hints so the gateway and the
// agents' handling of MCP calls can be observed without touching a real service.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type msg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func reply(id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	fmt.Println(string(b))
}

func main() {
	tools := []map[string]any{
		{"name": "list_items", "description": "List items in the store.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			"annotations": map[string]any{"readOnlyHint": true}},
		{"name": "delete_item", "description": "Delete an item by id. Cannot be undone.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}, "required": []string{"id"}},
			"annotations": map[string]any{"destructiveHint": true, "readOnlyHint": false}},
		{"name": "send_email", "description": "Send an email to a list of people.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"to": map[string]any{"type": "string"}, "subject": map[string]any{"type": "string"}}}},
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var m msg
		if json.Unmarshal(sc.Bytes(), &m) != nil || len(m.ID) == 0 {
			continue // notifications and junk need no answer
		}
		switch m.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(m.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			reply(m.ID, map[string]any{"protocolVersion": p.ProtocolVersion,
				"capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo":   map[string]any{"name": "fakemcp", "version": "0.0.1"}})
		case "tools/list":
			reply(m.ID, map[string]any{"tools": tools})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(m.Params, &p)
			text := fmt.Sprintf("fakemcp ran %s with %s", p.Name, string(p.Arguments))
			if f, err := os.OpenFile(os.Getenv("FAKEMCP_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				fmt.Fprintln(f, text)
				f.Close()
			}
			reply(m.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
		case "ping":
			reply(m.ID, map[string]any{})
		default:
			b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "method not found"}})
			fmt.Println(string(b))
		}
	}
}
