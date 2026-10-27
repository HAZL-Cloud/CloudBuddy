// Package mcpgw is `cloudbuddy mcp-gateway`: a stdio proxy between an agent
// and one of the user's MCP servers (D-023). Every tools/call is sent to the
// gate first; a denied call never reaches the server. If CloudBuddy can't be
// reached, the call is denied.
package mcpgw

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"cloudbuddy/internal/gateclient"
)

type msg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type spec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func Main(args []string) int {
	var server, specPath string
	for i, a := range args {
		if a == "--server" && i+1 < len(args) {
			server = args[i+1]
		}
		if a == "--spec" && i+1 < len(args) {
			specPath = args[i+1]
		}
	}
	c, err := gateclient.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cloudbuddy mcp-gateway:", err)
		return 2
	}
	b, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cloudbuddy mcp-gateway: no server spec:", err)
		return 2
	}
	var sp spec
	if err := json.Unmarshal(b, &sp); err != nil || sp.Command == "" {
		fmt.Fprintln(os.Stderr, "cloudbuddy mcp-gateway: bad server spec")
		return 2
	}
	down := exec.Command(sp.Command, sp.Args...)
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CB_") {
			env = append(env, kv)
		}
	}
	for k, v := range sp.Env {
		env = append(env, k+"="+v)
	}
	down.Env = env
	down.Stderr = os.Stderr
	dIn, _ := down.StdinPipe()
	dOut, _ := down.StdoutPipe()
	if err := down.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "cloudbuddy mcp-gateway: can't start", server+":", err)
		return 1
	}
	g := &gateway{c: c, server: server, pending: map[string]pend{}, out: os.Stdout, dIn: dIn}
	go func() {
		sc := bufio.NewScanner(dOut)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			g.fromServer(append([]byte(nil), sc.Bytes()...))
		}
		os.Exit(0)
	}()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		g.fromAgent(append([]byte(nil), sc.Bytes()...))
	}
	dIn.Close()
	_ = down.Wait()
	return 0
}

type pend struct {
	kind string // tools/list | tools/call
	key  string
}

type gateway struct {
	c      *gateclient.Client
	server string
	mu     sync.Mutex
	outMu  sync.Mutex
	// requests forwarded to the server, by id
	pending map[string]pend
	out     io.Writer
	dIn     io.Writer
}

func (g *gateway) toAgent(b []byte) {
	g.outMu.Lock()
	g.out.Write(append(b, '\n'))
	g.outMu.Unlock()
}

func (g *gateway) toServer(b []byte) {
	g.mu.Lock()
	g.dIn.Write(append(b, '\n'))
	g.mu.Unlock()
}

func (g *gateway) fromAgent(line []byte) {
	var m msg
	if json.Unmarshal(line, &m) != nil || len(m.ID) == 0 {
		g.toServer(line)
		return
	}
	switch m.Method {
	case "tools/list":
		g.mu.Lock()
		g.pending[string(m.ID)] = pend{kind: "tools/list"}
		g.mu.Unlock()
		g.toServer(line)
	case "tools/call":
		// decide in the background: the user may take minutes to answer
		go g.call(m, line)
	default:
		g.toServer(line)
	}
}

func (g *gateway) call(m msg, line []byte) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		Meta      map[string]any `json:"_meta"`
	}
	_ = json.Unmarshal(m.Params, &p)
	var ans struct {
		Decision, Reason, Key string
	}
	err := g.c.Post("/gate/mcp/call", map[string]any{"runId": g.c.RunID, "server": g.server, "tool": p.Name,
		"args": p.Arguments, "meta": p.Meta}, &ans)
	if err != nil || ans.Decision != "allow" {
		reason := ans.Reason
		if err != nil {
			reason = err.Error()
		}
		res, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
			"isError": true, "content": []map[string]any{{"type": "text", "text": "Denied by CloudBuddy: " + reason}}}})
		g.toAgent(res)
		return
	}
	g.mu.Lock()
	g.pending[string(m.ID)] = pend{kind: "tools/call", key: ans.Key}
	g.mu.Unlock()
	g.toServer(line)
}

func (g *gateway) fromServer(line []byte) {
	var m msg
	if json.Unmarshal(line, &m) == nil && len(m.ID) > 0 && m.Method == "" {
		g.mu.Lock()
		p, ok := g.pending[string(m.ID)]
		delete(g.pending, string(m.ID))
		g.mu.Unlock()
		if ok {
			switch p.kind {
			case "tools/list":
				var res struct {
					Tools []json.RawMessage `json:"tools"`
				}
				_ = json.Unmarshal(m.Result, &res)
				var tools []map[string]any
				for _, t := range res.Tools {
					var tool map[string]any
					if json.Unmarshal(t, &tool) == nil {
						tools = append(tools, map[string]any{"name": tool["name"], "annotations": tool["annotations"]})
					}
				}
				go g.c.Post("/gate/mcp/tools", map[string]any{"runId": g.c.RunID, "server": g.server, "tools": tools}, nil)
			case "tools/call":
				var res struct {
					IsError bool `json:"isError"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}
				_ = json.Unmarshal(m.Result, &res)
				var sb strings.Builder
				for _, c := range res.Content {
					if c.Type == "text" {
						sb.WriteString(c.Text)
						sb.WriteString("\n")
					}
				}
				isErr := res.IsError || (len(m.Error) > 0 && string(m.Error) != "null")
				if len(m.Error) > 0 && string(m.Error) != "null" {
					sb.Write(m.Error)
				}
				go g.c.Post("/gate/mcp/result", map[string]any{"runId": g.c.RunID, "key": p.key, "output": sb.String(), "isError": isErr}, nil)
			}
		}
	}
	g.toAgent(line)
}
