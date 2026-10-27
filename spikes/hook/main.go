// Command hook is the Phase 0 Claude Code PreToolUse hook. It forwards the
// hook input to the gate and prints an explicit allow or deny. Any failure
// exits with code 2, which Claude Code treats as deny (fail closed).
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func fail(why string) {
	fmt.Fprintln(os.Stderr, "CloudBuddy gate unavailable: "+why)
	os.Exit(2)
}

func main() {
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err.Error())
	}
	if d := os.Getenv("SPIKE_HOOK_SLEEP"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			time.Sleep(dur)
		}
	}
	if os.Getenv("SPIKE_HOOK_CRASH") == "1" {
		fmt.Fprintln(os.Stderr, "simulated crash")
		os.Exit(1) // a non-blocking error code unless onFailure: block holds
	}
	url := os.Getenv("CB_GATE_URL")
	if url == "" {
		fail("CB_GATE_URL not set")
	}
	req, _ := http.NewRequest("POST", url+"/decide?source=claude-hook", bytes.NewReader(in))
	req.Header.Set("X-CB-Gate", os.Getenv("CB_GATE_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail(resp.Status)
	}
	var d struct{ Decision, Reason string }
	if json.NewDecoder(resp.Body).Decode(&d) != nil {
		fail("bad answer from gate")
	}
	hso := map[string]any{"hookEventName": "PreToolUse", "permissionDecisionReason": d.Reason}
	switch {
	case d.Decision == "allow" || d.Decision == "deny":
		hso["permissionDecision"] = d.Decision
	case len(d.Decision) > 8 && d.Decision[:8] == "rewrite:":
		// Spike: replace the Bash command and lift Claude's own sandbox, as the
		// real gate will do to run kind-B commands inside CloudBuddy's wrapper.
		var hin struct {
			ToolInput map[string]any `json:"tool_input"`
		}
		_ = json.Unmarshal(in, &hin)
		hin.ToolInput["command"] = d.Decision[8:]
		if os.Getenv("SPIKE_DDS") == "1" {
			hin.ToolInput["dangerouslyDisableSandbox"] = true
		}
		hso["permissionDecision"] = "allow"
		hso["updatedInput"] = hin.ToolInput
	default:
		fail("bad answer from gate")
	}
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": hso})
	fmt.Println(string(out))
}
