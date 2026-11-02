// Package dashmcp is `cloudbuddy dashboards-mcp`: CloudBuddy's own MCP
// server, which Claude Code starts for every chat (D-068). Its tools let the
// agent save, list and read dashboards; there is no delete tool. Every call
// goes to the CloudBuddy server with the run's gate secret, which can't
// approve anything.
package dashmcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"cloudbuddy/internal/config"
	"cloudbuddy/internal/gateclient"
)

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func Main(args []string) int {
	c, err := gateclient.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cloudbuddy dashboards-mcp:", err)
		return 2
	}
	s := &server{call: func(tool, toolUseID string, args json.RawMessage) (string, bool) {
		var ans struct {
			Text    string `json:"text"`
			IsError bool   `json:"isError"`
		}
		err := c.Post("/gate/dashboards/tool", map[string]any{"runId": c.RunID, "toolUseId": toolUseID, "tool": tool, "args": args}, &ans)
		if err != nil {
			return "CloudBuddy couldn't do this: " + err.Error(), true
		}
		return ans.Text, ans.IsError
	}, out: os.Stdout}
	s.serve(os.Stdin)
	return 0
}

type server struct {
	call  func(tool, toolUseID string, args json.RawMessage) (string, bool)
	out   io.Writer
	outMu sync.Mutex
	wg    sync.WaitGroup
}

func (s *server) serve(in io.Reader) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var req request
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		s.handle(req)
	}
	s.wg.Wait()
}

func (s *server) send(v any) {
	b, _ := json.Marshal(v)
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.out.Write(append(b, '\n'))
}

func (s *server) reply(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) handle(req request) {
	if len(req.ID) == 0 {
		return // a notification (initialized, cancelled): nothing to answer
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-06-18"
		}
		s.reply(req.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "cloudbuddy", "version": config.Version},
			"instructions":    instructions,
		})
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": Tools()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      map[string]any  `json:"_meta"`
		}
		_ = json.Unmarshal(req.Params, &p)
		toolUseID, _ := p.Meta["claudecode/toolUseId"].(string)
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		s.wg.Add(1)
		go func() { // a save runs the commands once and can take a while
			defer s.wg.Done()
			text, isErr := s.call(p.Name, toolUseID, p.Arguments)
			s.reply(req.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isErr})
		}()
	default:
		s.send(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "unknown method " + req.Method}})
	}
}

const instructions = `CloudBuddy's own tools. Use save_dashboard when the user wants numbers, charts or tables they can keep open and that stay fresh (user counts, signups, revenue, errors, queue sizes, deploy status). CloudBuddy reruns the dashboard's commands itself on a timer, without you.`
