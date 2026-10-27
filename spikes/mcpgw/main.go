// Command mcpgw is the Phase 0 MCP gateway: a stdio proxy between an agent
// and one downstream stdio MCP server. Every tools/call is sent to the gate
// first; a denied call never reaches the downstream server. Anything the gate
// can't answer is treated as denied.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

type rpc struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func ask(line []byte) (string, string) {
	req, _ := http.NewRequest("POST", os.Getenv("CB_GATE_URL")+"/decide?source=mcp-gateway", bytes.NewReader(line))
	req.Header.Set("X-CB-Gate", os.Getenv("CB_GATE_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "deny", "CloudBuddy gate unavailable"
	}
	defer resp.Body.Close()
	var d struct{ Decision, Reason string }
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&d) != nil || d.Decision != "allow" {
		if d.Reason == "" {
			d.Reason = "CloudBuddy gate refused"
		}
		return "deny", d.Reason
	}
	return "allow", d.Reason
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mcpgw <downstream command> [args...]")
		os.Exit(2)
	}
	down := exec.Command(os.Args[1], os.Args[2:]...)
	down.Stderr = os.Stderr
	dIn, _ := down.StdinPipe()
	dOut, _ := down.StdoutPipe()
	if err := down.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var outMu sync.Mutex
	write := func(b []byte) {
		outMu.Lock()
		os.Stdout.Write(append(b, '\n'))
		outMu.Unlock()
	}
	logf, _ := os.OpenFile(os.Getenv("MCPGW_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)

	go func() { // downstream -> agent
		sc := bufio.NewScanner(dOut)
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			if logf != nil {
				fmt.Fprintf(logf, "down> %s\n", sc.Bytes())
			}
			write(append([]byte(nil), sc.Bytes()...))
		}
		os.Exit(0)
	}()

	sc := bufio.NewScanner(os.Stdin) // agent -> downstream
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		if logf != nil {
			fmt.Fprintf(logf, "agent> %s\n", line)
		}
		var m rpc
		if json.Unmarshal(line, &m) == nil && m.Method == "tools/call" && len(m.ID) > 0 {
			if d, reason := ask(line); d != "allow" {
				res, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": map[string]any{
					"isError": true,
					"content": []map[string]any{{"type": "text", "text": "Denied by CloudBuddy: " + reason}},
				}})
				write(res)
				continue
			}
		}
		if _, err := io.Copy(dIn, bytes.NewReader(append(line, '\n'))); err != nil {
			break
		}
	}
	dIn.Close()
	down.Wait()
}
