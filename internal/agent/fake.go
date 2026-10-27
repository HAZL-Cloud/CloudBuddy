package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Fake replays a scenario file as if an agent were working (D-028: no live
// agent before the gate exists; also used by tests and the dev demo). Every
// tool call goes through the same gate as a real Claude Code hook call.
type Fake struct {
	Scenario string        // path to a scenario JSON file
	Delay    time.Duration // pause between steps
}

type FakeScenario struct {
	Model string `json:"model"`
	Steps []struct {
		Say    string         `json:"say,omitempty"`
		Tool   string         `json:"tool,omitempty"`
		Input  map[string]any `json:"input,omitempty"`
		Output string         `json:"output,omitempty"`
		Error  bool           `json:"error,omitempty"`
	} `json:"steps"`
}

// FakeGate is how the fake agent asks the gate, like the Claude hook does.
type FakeGate func(ctx context.Context, runID, toolKey, toolName string, input map[string]any) (allowed bool, reason string)

func (f *Fake) Name() string { return "fake" }

func (f *Fake) Run(ctx context.Context, o StartOptions, out chan<- Event) error {
	return fmt.Errorf("fake agent needs RunWithGate")
}

func (f *Fake) RunWithGate(ctx context.Context, o StartOptions, gate FakeGate, out chan<- Event) error {
	b, err := os.ReadFile(f.Scenario)
	if err != nil {
		out <- Event{Kind: "done", Status: "failed", Text: err.Error()}
		return err
	}
	var sc FakeScenario
	if err := json.Unmarshal(b, &sc); err != nil {
		out <- Event{Kind: "done", Status: "failed", Text: err.Error()}
		return err
	}
	out <- Event{Kind: "session", SessionID: "fake-" + o.RunID, Model: sc.Model}
	for i, s := range sc.Steps {
		if ctx.Err() != nil {
			out <- Event{Kind: "done", Status: "interrupted"}
			return nil
		}
		if s.Say != "" {
			out <- Event{Kind: "text", Text: s.Say}
		}
		if s.Tool != "" {
			key := fmt.Sprintf("fake-%s-%d", o.RunID, i)
			out <- Event{Kind: "tool_start", ToolKey: key, ToolName: s.Tool, Input: s.Input}
			ok, reason := gate(ctx, o.RunID, key, s.Tool, s.Input)
			if ok {
				out <- Event{Kind: "tool_end", ToolKey: key, ToolName: s.Tool, Output: s.Output, IsError: s.Error}
			} else {
				out <- Event{Kind: "tool_end", ToolKey: key, ToolName: s.Tool, Output: "Denied: " + reason, IsError: true}
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(f.Delay):
		}
	}
	out <- Event{Kind: "done", Status: "completed"}
	return nil
}
