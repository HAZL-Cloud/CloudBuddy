// Command codexdrv is the Phase 0 Codex spike (S2): it drives
// `codex app-server` over stdio JSON-RPC, pins the approval settings, sends
// every server request to the spike gate, and logs everything to a JSONL file.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type msg struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

var (
	stdin   io.Writer
	wmu     sync.Mutex
	logf    *os.File
	nextID  = 1
	pending = map[string]chan msg{}
	pmu     sync.Mutex
)

func send(v any) {
	b, _ := json.Marshal(v)
	wmu.Lock()
	stdin.Write(append(b, '\n'))
	wmu.Unlock()
	fmt.Fprintf(logf, "{\"dir\":\"out\",\"msg\":%s}\n", b)
}

func request(method string, params any) msg {
	pmu.Lock()
	id := nextID
	nextID++
	ch := make(chan msg, 1)
	pending[fmt.Sprint(id)] = ch
	pmu.Unlock()
	send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	select {
	case m := <-ch:
		return m
	case <-time.After(120 * time.Second):
		fmt.Println("TIMEOUT waiting for", method)
		os.Exit(1)
	}
	return msg{}
}

func gate(body []byte) (string, string) {
	req, _ := http.NewRequest("POST", os.Getenv("CB_GATE_URL")+"/decide?source=codex", bytes.NewReader(body))
	req.Header.Set("X-CB-Gate", os.Getenv("CB_GATE_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "deny", "gate unavailable"
	}
	defer resp.Body.Close()
	var d struct{ Decision, Reason string }
	_ = json.NewDecoder(resp.Body).Decode(&d)
	if d.Decision != "allow" {
		return "deny", d.Reason
	}
	return "allow", d.Reason
}

func handleServerRequest(m msg) {
	fmt.Printf("SERVER REQUEST %s %s\n", m.Method, truncate(string(m.Params), 400))
	var result any
	switch m.Method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		if d, _ := gate(m.Params); d == "allow" {
			result = map[string]any{"decision": "accept"}
		} else {
			result = map[string]any{"decision": "decline"}
		}
	case "item/fileChange/requestApproval", "applyPatchApproval":
		if d, _ := gate(m.Params); d == "allow" {
			result = map[string]any{"decision": "accept"}
		} else {
			result = map[string]any{"decision": "decline"}
		}
	case "item/permissions/requestApproval":
		// Grant nothing: a turn-wide grant would skip later reviews.
		result = map[string]any{"permissions": map[string]any{}, "scope": "turn"}
	case "mcpServer/elicitation/request":
		result = map[string]any{"action": "decline", "content": nil}
	default:
		send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": -32601, "message": "declined by CloudBuddy spike: " + m.Method}})
		return
	}
	send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func main() {
	cwd := flag.String("cwd", ".", "work folder")
	prompt := flag.String("prompt", "", "user prompt")
	logPath := flag.String("log", "codex.jsonl", "protocol log")
	model := flag.String("model", "", "model override")
	policy := flag.String("policy", "on-request", "approval policy")
	mcp := flag.String("mcp", "", "JSON object for config.mcp_servers")
	flag.Parse()
	logf, _ = os.Create(*logPath)

	cmd := exec.Command("codex", "app-server", "--listen", "stdio://")
	cmd.Dir = *cwd
	cmd.Stderr, _ = os.Create(*logPath + ".stderr")
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	stdin = in
	if err := cmd.Start(); err != nil {
		fmt.Println("start:", err)
		os.Exit(1)
	}
	done := make(chan msg, 1)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			line := sc.Bytes()
			fmt.Fprintf(logf, "{\"dir\":\"in\",\"msg\":%s}\n", line)
			var m msg
			if json.Unmarshal(line, &m) != nil {
				continue
			}
			switch {
			case m.Method != "" && len(m.ID) > 0:
				go handleServerRequest(m)
			case m.Method != "":
				if strings.HasPrefix(m.Method, "item/") && !strings.Contains(m.Method, "delta") {
					var p struct {
						Item map[string]any `json:"item"`
					}
					_ = json.Unmarshal(m.Params, &p)
					if p.Item != nil {
						fmt.Printf("%s %v %s\n", m.Method, p.Item["type"], truncate(fmt.Sprint(p.Item["command"], " ", p.Item["status"], " ", p.Item["server"], " ", p.Item["tool"], " ", p.Item["exitCode"]), 200))
					}
				}
				if m.Method == "turn/completed" {
					done <- m
				}
			default:
				pmu.Lock()
				ch := pending[string(m.ID)]
				pmu.Unlock()
				if ch != nil {
					ch <- m
				}
			}
		}
	}()

	r := request("initialize", map[string]any{"clientInfo": map[string]any{"name": "cloudbuddy-spike", "version": "0.0.1"},
		"capabilities": map[string]any{"experimentalApi": true}})
	fmt.Println("initialize:", truncate(string(r.Result)+string(r.Error), 300))
	send(map[string]any{"jsonrpc": "2.0", "method": "initialized"})

	tp := map[string]any{"cwd": *cwd, "approvalPolicy": *policy, "approvalsReviewer": "user", "sandbox": "workspace-write"}
	if *model != "" {
		tp["model"] = *model
	}
	if *mcp != "" {
		var servers map[string]any
		_ = json.Unmarshal([]byte(*mcp), &servers)
		tp["config"] = map[string]any{"mcp_servers": servers}
	}
	r = request("thread/start", tp)
	fmt.Println("thread/start:", truncate(string(r.Result)+string(r.Error), 300))
	var ts struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	_ = json.Unmarshal(r.Result, &ts)
	r = request("turn/start", map[string]any{"threadId": ts.Thread.ID, "approvalPolicy": *policy, "approvalsReviewer": "user",
		"sandboxPolicy": map[string]any{"type": "workspaceWrite", "networkAccess": false},
		"effort":        "low", "input": []map[string]any{{"type": "text", "text": *prompt}}})
	fmt.Println("turn/start:", truncate(string(r.Result)+string(r.Error), 200))
	select {
	case m := <-done:
		fmt.Println("turn/completed:", truncate(string(m.Params), 300))
	case <-time.After(6 * time.Minute):
		fmt.Println("turn TIMEOUT")
	}
	in.Close()
	cmd.Process.Kill()
}
