package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Codex drives `codex app-server` with the settings pinned by D-021 and an
// isolated CODEX_HOME (D-037).
type Codex struct {
	DataDir string
	Home    string
}

func (c *Codex) Name() string { return "codex" }

// Home prepares CloudBuddy's own Codex home: only the login is shared.
func (c *Codex) prepareHome() (string, error) {
	dir := filepath.Join(c.DataDir, "codex-home")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	link := filepath.Join(dir, "auth.json")
	target := filepath.Join(c.Home, ".codex", "auth.json")
	if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
		if err := os.Symlink(target, link); err != nil {
			return "", err
		}
	}
	return dir, nil
}

type rpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type codexConn struct {
	w       io.Writer
	wmu     sync.Mutex
	next    int
	pmu     sync.Mutex
	pending map[string]chan rpcMsg
	log     io.Writer
}

func (c *codexConn) send(v any) error {
	b, _ := json.Marshal(v)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.log != nil {
		fmt.Fprintf(c.log, "> %s\n", b)
	}
	_, err := c.w.Write(append(b, '\n'))
	return err
}

func (c *codexConn) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.pmu.Lock()
	c.next++
	id := c.next
	ch := make(chan rpcMsg, 1)
	c.pending[fmt.Sprint(id)] = ch
	c.pmu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if len(m.Error) > 0 && string(m.Error) != "null" {
			return nil, fmt.Errorf("codex %s: %s", method, m.Error)
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(90 * time.Second):
		return nil, fmt.Errorf("codex %s: no answer", method)
	}
}

// policyParams are sent on thread/start, thread/resume and every turn/start
// so neither user config nor a resumed thread can change them (D-021).
func policyParams() map[string]any {
	return map[string]any{"approvalPolicy": "on-request", "approvalsReviewer": "user"}
}

func (c *Codex) Run(ctx context.Context, o StartOptions, out chan<- Event) error {
	home, err := c.prepareHome()
	if err != nil {
		return err
	}
	env := map[string]string{"CODEX_HOME": home}
	for k, v := range o.Env {
		env[k] = v
	}
	cmd := exec.Command("codex", "app-server", "--listen", "stdio://")
	cmd.Dir = o.Workdir
	cmd.Env = baseEnv(env)
	stderr, _ := os.Create(filepath.Join(o.RunDir, "codex.stderr"))
	defer stderr.Close()
	cmd.Stderr = stderr
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := startGroup(cmd); err != nil {
		return fmt.Errorf("couldn't start Codex: %w", err)
	}
	protoLog, _ := os.Create(filepath.Join(o.RunDir, "codex.protocol.log"))
	defer protoLog.Close()
	conn := &codexConn{w: stdin, pending: map[string]chan rpcMsg{}, log: protoLog}
	turnDone := make(chan string, 1)
	readerDone := make(chan struct{})
	gatewayNames := map[string]bool{}
	for name := range o.MCP {
		gatewayNames[name] = true
	}

	go func() {
		defer close(readerDone)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			fmt.Fprintf(protoLog, "< %s\n", line)
			var m rpcMsg
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			switch {
			case m.Method != "" && len(m.ID) > 0:
				go c.answer(ctx, conn, m, o.Approve, gatewayNames)
			case m.Method != "":
				for _, e := range parseCodexNotification(m.Method, m.Params, gatewayNames) {
					out <- e
				}
				if m.Method == "turn/completed" {
					var p struct {
						Turn struct {
							Status string `json:"status"`
							Error  *struct {
								Message string `json:"message"`
							} `json:"error"`
						} `json:"turn"`
					}
					_ = json.Unmarshal(m.Params, &p)
					st := p.Turn.Status
					if p.Turn.Error != nil {
						st += ": " + p.Turn.Error.Message
					}
					select {
					case turnDone <- st:
					default:
					}
				}
			default:
				conn.pmu.Lock()
				ch := conn.pending[string(m.ID)]
				delete(conn.pending, string(m.ID))
				conn.pmu.Unlock()
				if ch != nil {
					ch <- m
				}
			}
		}
	}()

	finish := func(status, text string) error {
		out <- Event{Kind: "done", Status: status, Text: text}
		stdin.Close()
		StopGroup(cmd)
		_ = cmd.Wait()
		<-readerDone
		return nil
	}

	if _, err := conn.request(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "cloudbuddy", "title": "CloudBuddy", "version": "0.1.0"},
		"capabilities": map[string]any{"experimentalApi": true},
	}); err != nil {
		return finish("failed", err.Error())
	}
	_ = conn.send(map[string]any{"jsonrpc": "2.0", "method": "initialized"})

	threadParams := policyParams()
	threadParams["cwd"] = o.Workdir
	threadParams["sandbox"] = "workspace-write"
	threadParams["developerInstructions"] = o.Brief
	mcp := map[string]any{}
	for name, s := range o.MCP {
		mcp[name] = map[string]any{"command": s.Command, "args": s.Args, "env": s.Env}
	}
	threadParams["config"] = map[string]any{"mcp_servers": mcp}
	if o.Model != "" {
		threadParams["model"] = o.Model
	}
	var res json.RawMessage
	threadID := o.SessionID
	if threadID != "" {
		threadParams["threadId"] = threadID
		res, err = conn.request(ctx, "thread/resume", threadParams)
	} else {
		res, err = conn.request(ctx, "thread/start", threadParams)
	}
	if err != nil {
		return finish("failed", err.Error())
	}
	var ts struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Model string `json:"model"`
	}
	_ = json.Unmarshal(res, &ts)
	if ts.Thread.ID != "" {
		threadID = ts.Thread.ID
	}
	out <- Event{Kind: "session", SessionID: threadID, Model: ts.Model}

	turn := policyParams()
	turn["threadId"] = threadID
	turn["cwd"] = o.Workdir
	turn["sandboxPolicy"] = map[string]any{"type": "workspaceWrite", "networkAccess": false}
	turn["input"] = []map[string]any{{"type": "text", "text": o.Prompt}}
	if o.Model != "" {
		turn["model"] = o.Model
	}
	if os.Getenv("CB_CODEX_EFFORT") != "" {
		turn["effort"] = os.Getenv("CB_CODEX_EFFORT")
	}
	if _, err := conn.request(ctx, "turn/start", turn); err != nil {
		return finish("failed", err.Error())
	}
	select {
	case st := <-turnDone:
		status, text := "completed", ""
		if !strings.HasPrefix(st, "completed") {
			status, text = "failed", st
		}
		return finish(status, text)
	case <-ctx.Done():
		return finish("interrupted", "")
	case <-readerDone:
		return finish("failed", "Codex stopped unexpectedly")
	}
}

// answer replies to one Codex server request. Unknown request types are
// declined (coverage, section 4.3).
func (c *Codex) answer(ctx context.Context, conn *codexConn, m rpcMsg, approve Approver, gateways map[string]bool) {
	reply := func(result any) {
		_ = conn.send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
	}
	fail := func(msg string) {
		_ = conn.send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": msg}})
	}
	switch m.Method {
	case "item/commandExecution/requestApproval", "execCommandApproval",
		"item/fileChange/requestApproval", "applyPatchApproval":
		if approve == nil {
			reply(map[string]any{"decision": "decline"})
			return
		}
		res, err := approve(ctx, m.Method, m.Params)
		if err != nil {
			reply(map[string]any{"decision": "decline"})
			return
		}
		reply(res)
	case "item/permissions/requestApproval":
		// Grant nothing: a grant covers the rest of the turn without review.
		reply(map[string]any{"permissions": map[string]any{}, "scope": "turn"})
	case "mcpServer/elicitation/request":
		var p struct {
			ServerName string `json:"serverName"`
			Meta       struct {
				Kind string `json:"codex_approval_kind"`
			} `json:"_meta"`
		}
		_ = json.Unmarshal(m.Params, &p)
		// The gateway decides MCP calls; Codex's own prompt for them is
		// accepted only for CloudBuddy's gateway servers, never persisted.
		if gateways[p.ServerName] && p.Meta.Kind == "mcp_tool_call" {
			reply(map[string]any{"action": "accept", "content": nil, "_meta": nil})
			return
		}
		reply(map[string]any{"action": "decline", "content": nil, "_meta": nil})
	default:
		fail("CloudBuddy declined " + m.Method)
	}
}

// parseCodexNotification turns app-server notifications into events. MCP
// calls through CloudBuddy's gateway are skipped: the gateway reports them.
func parseCodexNotification(method string, params json.RawMessage, gateways map[string]bool) []Event {
	if method != "item/started" && method != "item/completed" {
		return nil
	}
	var p struct {
		Item map[string]any `json:"item"`
	}
	if json.Unmarshal(params, &p) != nil || p.Item == nil {
		return nil
	}
	it := p.Item
	typ, _ := it["type"].(string)
	id, _ := it["id"].(string)
	switch typ {
	case "agentMessage":
		if method == "item/completed" {
			if t, _ := it["text"].(string); strings.TrimSpace(t) != "" {
				return []Event{{Kind: "text", Text: t}}
			}
		}
	case "commandExecution":
		cmdline, _ := it["command"].(string)
		if method == "item/started" {
			return []Event{{Kind: "tool_start", ToolKey: id, ToolName: "shell", Command: cmdline}}
		}
		outTxt, _ := it["aggregatedOutput"].(string)
		status, _ := it["status"].(string)
		isErr := status != "completed"
		if ec, ok := it["exitCode"].(float64); ok && ec != 0 {
			isErr = true
		}
		return []Event{{Kind: "tool_end", ToolKey: id, ToolName: "shell", Command: cmdline, Output: outTxt, IsError: isErr, Status: status}}
	case "fileChange":
		var paths []string
		if ch, ok := it["changes"].([]any); ok {
			for _, x := range ch {
				if m, ok := x.(map[string]any); ok {
					if s, ok := m["path"].(string); ok {
						paths = append(paths, s)
					}
				}
			}
		}
		if method == "item/started" {
			return []Event{{Kind: "tool_start", ToolKey: id, ToolName: "file_change", Paths: paths}}
		}
		status, _ := it["status"].(string)
		return []Event{{Kind: "tool_end", ToolKey: id, ToolName: "file_change", Paths: paths, IsError: status != "completed", Status: status}}
	case "mcpToolCall":
		server, _ := it["server"].(string)
		if gateways[server] {
			return nil
		}
		tool, _ := it["tool"].(string)
		if method == "item/started" {
			return []Event{{Kind: "tool_start", ToolKey: id, ToolName: "mcp__" + server + "__" + tool}}
		}
		return []Event{{Kind: "tool_end", ToolKey: id, ToolName: "mcp__" + server + "__" + tool, IsError: it["status"] != "completed"}}
	case "webSearch":
		if method == "item/started" {
			return []Event{{Kind: "tool_start", ToolKey: id, ToolName: "WebSearch"}}
		}
		return []Event{{Kind: "tool_end", ToolKey: id, ToolName: "WebSearch"}}
	}
	return nil
}
