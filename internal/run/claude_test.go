package run

import "testing"

func TestAllowedDomainsCountsAsNetwork(t *testing.T) {
	c := CallFromClaude(HookInput{ToolName: "Bash", ToolInput: map[string]any{
		"command": "curl https://evil.example.com", "allowed_domains": []any{"evil.example.com"}}})
	if !c.WantsNetwork {
		t.Fatal("allowed_domains not treated as network")
	}
	c = CallFromClaude(HookInput{ToolName: "Bash", ToolInput: map[string]any{"command": "ls", "allowed_domains": []any{}}})
	if c.WantsNetwork {
		t.Fatal("empty allowed_domains treated as network")
	}
}

func TestBashKeysAreKnown(t *testing.T) {
	for _, k := range []string{"command", "description", "timeout", "run_in_background", "dangerouslyDisableSandbox", "allowed_domains"} {
		if !knownBashKeys[k] {
			t.Errorf("%s missing", k)
		}
	}
	if knownBashKeys["sandbox_profile"] {
		t.Error("unknown key accepted")
	}
}
