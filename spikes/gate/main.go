// Command gate is the Phase 0 stand-in for CloudBuddy's gate. It receives
// permission requests from the Claude hook, the Codex driver and the MCP
// gateway, logs them, and answers with a scripted policy: anything mentioning
// DENYME, delete_item or send_email is denied, everything else is allowed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	logPath := flag.String("log", "gate.jsonl", "request log")
	portFile := flag.String("portfile", "gate.port", "where to write the chosen port")
	delay := flag.Duration("delay", 0, "wait before answering (simulates a person deciding)")
	flag.Parse()
	token := os.Getenv("CB_GATE_TOKEN")

	lf, err := os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = os.WriteFile(*portFile, []byte(fmt.Sprint(port)), 0o600)

	http.HandleFunc("/decide", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("X-CB-Gate") != token {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		body, _ := io.ReadAll(r.Body)
		decision, reason := "allow", "spike policy: allowed"
		s := string(body)
		bads := []string{"DENYME"}
		if r.URL.Query().Get("source") == "mcp-gateway" || os.Getenv("SPIKE_STRICT") == "1" {
			bads = append(bads, "delete_item", "send_email")
		}
		for _, bad := range bads {
			if strings.Contains(s, bad) {
				decision, reason = "deny", "spike policy: contains "+bad
			}
		}
		if rw := os.Getenv("SPIKE_REWRITE"); rw != "" && strings.Contains(s, "REWRITEME") {
			decision, reason = "rewrite:"+rw, "spike policy: rewritten"
		}
		if *delay > 0 {
			time.Sleep(*delay)
		}
		rec, _ := json.Marshal(map[string]any{"ts": time.Now().Format(time.RFC3339Nano), "source": r.URL.Query().Get("source"),
			"decision": decision, "request": json.RawMessage(body)})
		fmt.Fprintln(lf, string(rec))
		_ = json.NewEncoder(w).Encode(map[string]string{"decision": decision, "reason": reason})
	})
	log.Printf("gate listening on %d", port)
	log.Fatal(http.Serve(ln, nil))
}
