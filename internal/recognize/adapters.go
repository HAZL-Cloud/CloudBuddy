package recognize

// Adapters holds the loaded adapter files (Phase 4). A nil *Adapters means
// generic recognition only.
type Adapters struct {
	shell []shellRule
	mcp   []mcpRule
}

type shellRule struct{}
type mcpRule struct{}

func (a *Adapters) matchShell(s segment) (Draft, bool) { return Draft{}, false }
func (a *Adapters) matchMCP(c Call) (Draft, bool)      { return Draft{}, false }
