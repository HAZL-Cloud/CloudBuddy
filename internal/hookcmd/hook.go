// Package hookcmd is `cloudbuddy hook`, the program Claude Code runs before
// every tool call (D-020). Any failure exits with code 2, which Claude Code
// treats as deny; the settings also set onFailure: "block".
package hookcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"cloudbuddy/internal/gateclient"
)

func fail(why string) int {
	fmt.Fprintln(os.Stderr, "CloudBuddy blocked this: "+why)
	return 2
}

func Main(args []string) int {
	event := "pretool"
	for i, a := range args {
		if a == "--event" && i+1 < len(args) {
			event = args[i+1]
		}
	}
	in, err := io.ReadAll(io.LimitReader(os.Stdin, 8<<20))
	if err != nil {
		return fail(err.Error())
	}
	c, err := gateclient.FromEnv()
	if err != nil {
		return fail(err.Error())
	}
	if event == "session-start" {
		_ = c.Post("/gate/claude/session", map[string]string{"runId": c.RunID}, nil)
		return 0
	}
	var input json.RawMessage = in
	var ans struct {
		Decision     string         `json:"decision"`
		Reason       string         `json:"reason"`
		UpdatedInput map[string]any `json:"updatedInput"`
	}
	if err := c.Post("/gate/claude/pretool", map[string]any{"runId": c.RunID, "input": input}, &ans); err != nil {
		return fail(err.Error())
	}
	if ans.Decision != "allow" && ans.Decision != "deny" {
		return fail("unexpected answer from CloudBuddy")
	}
	out := map[string]any{"hookEventName": "PreToolUse", "permissionDecision": ans.Decision, "permissionDecisionReason": ans.Reason}
	if ans.Decision == "allow" && ans.UpdatedInput != nil {
		out["updatedInput"] = ans.UpdatedInput
	}
	b, _ := json.Marshal(map[string]any{"hookSpecificOutput": out})
	fmt.Println(string(b))
	return 0
}
